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
        const textFiles = [];          // 文本文件围栏块 {name, block}

        for (const pf of pendingFiles) {
            const category = pf.category;
            switch (category) {
                // 多模态消息 -> 图片
                case 'image':
                    try {
                        const fileUrl = await saveFile(pf.file);
                        contentBlocks.push({ type: 'image_url', image_url: { url: fileUrl } });
                        // 参考图（律令 <参考图>）：图片附件的标签置为「参考图」，便于在消息记录中识别
                        const label = (category === 'image' && text.includes('<参考图>')) ? '参考图' : pf.name;
                        attachments.push({ type: category, src: fileUrl.replace(window.location.origin, ''), label });
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
                // 多模态消息 -> 文本文件
                case 'text':
                    try {
                        const rawText = await readFileAsText(pf.file);
                        if (rawText.trim()) {
                            textFiles.push({ name: pf.name, block: `\`\`\`${pf.name}\n${rawText}\n\`\`\`` });
                            categories.add('text');
                        }
                    }
                    catch (err) { showToast(`无法读取文件 ${pf.name}`, 'error'); }
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
        // 引用来源：手动载入（referencedFiles）+ 导入文本文件且带文字时的自动引用
        // 统一规范化为 `[#fileName.ext]:` 引用块（reader 仅识别带 # 的引用）
        const rawRefIds = [...referencedFiles];
        if (text && textFiles.length) {
            for (const tf of textFiles) rawRefIds.push(tf.name);
        }
        const refNames = [...new Set(rawRefIds.map(id => id.replace(/^#/, '')))];
        const refText = refNames.map(name => `(#${name}):`).join('');
        // 主用户文本：有引用 → 「[文件按钮] 用户输入」；仅导入无文字 → 「已将文件x交给月华」；否则普通文字
        let userText;
        if (refText) userText = refText + text;
        else if (text) userText = text;
        else if (textFiles.length) userText = `已将文件${textFiles.map(t => t.name).join('、')}交给月华`;
        else userText = '';
        if (userText || textFiles.length) categories.add('text');

        // ---- 组装发送给后端的消息数组（顺序：围栏块 → 主用户消息）----
        // 前端在发送前即已知文件ID，直接自构造引用，无需等待后端推送
        // 发送给月华的主文本带 QQ 适配器同款用户名前缀；本地展示（addMessage）仍用不带前缀的 userText
        const payloadText = userText ? buildUserNamePrefix() + userText : userText;
        const sendPayload = [];
        for (const tf of textFiles) sendPayload.push({ role: 'user', content: tf.block });
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
