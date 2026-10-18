// ============================================================
//  星月智能 · 消息终端 — 消息发送与输入事件
// ============================================================

// ---------- 消息发送 ----------
async function sendMessages(payload) {
    const res = await fetch('/write/message', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ messages: payload })
    });
    if (!res.ok) throw new Error('发送失败');
    return res.json();
}

function setSendingState(sending) {
    isSending = sending;
    if (sendButton) {
        sendButton.disabled = sending;
        sendButton.innerHTML = sending ? '<i class="fas fa-spinner fa-pulse"></i>' : '<i class="fas fa-paper-plane"></i>';
    }
}

async function handleSend() {
    if (isSending) return;
    const text = messageInput.value.trim();
    const hasPending = pendingFiles.length > 0;
    if (!text && referencedFiles.length === 0 && !hasPending) return;

    setSendingState(true);

    try {
        const contentBlocks = [];      // 多模态文本块（img/video/audio/other）
        const attachments = [];        // 本地附件预览
        const categories = new Set();
        const documentRefs = [];       // 已入库文档 {handle, name}

        for (const pf of pendingFiles) {
            const category = pf.category;
            switch (category) {
                // 多模态消息 -> 图片
                case 'image':
                    try {
                        const fileUrl = await saveFile(pf.file);
                        contentBlocks.push({ type: 'image_url', image_url: { url: fileUrl } });
                        attachments.push({ type: category, src: fileUrl.replace(window.location.origin, ''), label: pf.name });
                        categories.add('image');
                    }
                    catch (err) { showToast(`无法上传 ${pf.name}`, 'error'); }
                    break;
                // 多模态消息 -> 视频
                case 'video':
                    try {
                        const fileUrl = await saveFile(pf.file);
                        contentBlocks.push({ type: 'text', text: '[视频] ' });
                        contentBlocks.push({ type: 'video_url', video_url: { url: fileUrl } });
                        attachments.push({ type: 'video', src: fileUrl.replace(window.location.origin, ''), label: pf.name });
                        categories.add('image');
                    }
                    catch (err) { showToast(`无法上传 ${pf.name}`, 'error'); }
                    break;
                // 多模态消息 -> 语音/音频
                case 'audio':
                    try {
                        const base64Data = await fileToRawBase64(pf.file);
                        const format = getAudioFormat(pf.file);
                        contentBlocks.push({ type: 'text', text: '[语音] ' });
                        if (format) {
                            contentBlocks.push({ type: 'input_audio', input_audio: { data: base64Data, format } });
                        }
                        else {
                            const fileUrl = await saveFile(pf.file);
                            contentBlocks.push({ type: 'audio_url', audio_url: { url: fileUrl } });
                        }
                    }
                    catch (err) { showToast(`无法读取音频 ${pf.name}`, 'error'); }
                    // 历史记录使用独立 blob URL，避免被清理撤销
                    attachments.push({ type: 'audio', src: URL.createObjectURL(pf.file), label: pf.name });
                    categories.add('voice');
                    break;
                // 文本文件 -> 服务端入库并拿到句柄（正文不进上下文）
                case 'text':
                    try {
                        documentRefs.push(await admitDocumentFile(pf.file));
                        categories.add('file');
                    }
                    catch (err) { showToast(`无法上传文件 ${pf.name}`, 'error'); }
                    break;
                // 多模态消息 -> 其他文件
                default:
                    try {
                        const fileUrl = await saveFile(pf.file);
                        const block = `[文件] 名称: ${pf.name} 大小: ${formatFileSize(pf.file.size)} 访问链接: ${fileUrl}`;
                        contentBlocks.push({ type: 'text', text: block });
                        attachments.push({ type: 'other', src: fileUrl.replace(window.location.origin, ''), label: pf.name });
                        categories.add('text');
                    }
                    catch (err) { showToast(`无法上传文件 ${pf.name}`, 'error'); }
                    break;
            }
        }
        // ---- 组装引用与主用户文本（一次导入/引用只显示一个气泡）----
        // 文档引用统一为规范句柄 @D.003：文件在 Go 侧或前端上传后入库时即已分配，
        // 消息里只传句柄，正文由月华按需用 read_resource 读取。
        const handleRefs = [
            ...referencedFiles,
            ...documentRefs.map(item => item.handle)
        ].map(id => String(id).trim().replace(/^[#@]/, ''))
            .filter(Boolean);
        const refText = [...new Set(handleRefs)].map(id => `@${id}`).join(' ');
        // 主用户文本：有文字时带引用，只发文件时正文就是句柄
        const userText = text ? (refText ? refText + ' ' : '') + text : refText;
        if (text) categories.add('text');
        if (documentRefs.length || referencedFiles.length) categories.add('file');

        // ---- 组装发送给后端的消息数组 ----
        // 发送给月华的主文本带 QQ 适配器同款用户名前缀；本地展示（addMessage）仍用不带前缀的 userText
        const payloadText = userText ? buildUserNamePrefix() + userText : userText;
        const sendPayload = [];
        if (userText || contentBlocks.length) {
            const mainContent = contentBlocks.length
                ? [...(payloadText ? [{ type: 'text', text: payloadText }] : []), ...contentBlocks]
                : payloadText;
            sendPayload.push({ role: 'user', content: mainContent });
        }

        // ---- 本地历史展示（单个气泡）----
        addMessage({
            id: generateId(),
            role: 'user',
            categories: categories.size ? Array.from(categories) : ['text'],
            content: userText,
            attachments: attachments.length ? attachments : undefined,
            timestamp: Date.now()
        });
        // 用户发送消息时触发持久化（AI 消息不再自动写盘）
        schedulePersist();

        // 推送到后端
        if (backendConnected) {
            if (sendPayload.length) await sendMessages(sendPayload);
        } else {
            showToast('离线模式：内容仅本地渲染', 'info');
        }
    } catch (err) {
        showToast('发送失败：' + (err.message || err), 'error');
    } finally {
        // 无论成功或失败都清理输入与待发送附件、文件引用（消息已进入历史记录）
        messageInput.value = '';
        autoResizeTextarea();
        clearPendingFiles();
        referencedFiles = [];
        renderFileRefChips();
        messageInput.focus();
        setSendingState(false);
    }
}

function autoResizeTextarea() {
    messageInput.style.height = 'auto';
    messageInput.style.height = Math.min(messageInput.scrollHeight, 200) + 'px';
}

function setupInputEvents() {
    messageInput.addEventListener('input', autoResizeTextarea);
    messageInput.addEventListener('keydown', (e) => {
        if (e.key === 'Enter' && !e.shiftKey) {
            e.preventDefault();
            handleSend();
        }
    });
    sendButton.addEventListener('click', handleSend);
    attachBtn.addEventListener('click', () => fileInput.click());
    fileInput.addEventListener('change', (e) => {
        const files = e.target.files;
        if (files && files.length) addPendingFiles(Array.from(files));
        fileInput.value = '';
    });
    clearBtn.addEventListener('click', () => {
        if (messages.length === 0) return;
        messageArea.querySelectorAll('.message').forEach(el => el.remove());
        messages = [];
        updateEmptyState();
        // 立即落盘：清空必须当场写入存储文件，不能等 1 分钟周期判定
        schedulePersist(true);
        showToast('已清空消息', 'info');
    });
}
