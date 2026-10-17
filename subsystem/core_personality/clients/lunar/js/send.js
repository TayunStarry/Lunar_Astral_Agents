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
        // 引用文法已统一为资源句柄 @D.003（文档）/ @I.007（图片）/ @V.001（视频）/ @A.002（音频）。
        // 前端在发送时还不知道文件名对应的句柄序号（序号由服务端资源目录分配），
        // 因此这里下发的是**文件名引用标记** `@文件名.ext`：服务端收录文件后会把标记
        // 规范化为真正的句柄并在正文中整段展开（见 reader.ts 的 processReferencesInText）。
        const rawRefIds = [...referencedFiles];
        if (text && textFiles.length) {
            for (const tf of textFiles) rawRefIds.push(tf.name);
        }
        const refNames = [...new Set(rawRefIds.map(id => String(id).replace(/^[#@]/, '')))];
        // 引用写作 `@句柄` 或 `@文件名.ext` 形态：已经是句柄的直接下发，文件名形态由服务端在
        // 收录后规范化为句柄（序号由服务端资源目录分配，前端无法预知）并整段展开。
        const refText = refNames.map(name => `@${name}`).join(' ');
        // 主用户文本：
        //   - 有文字 → 带引用（此时是"就这份资料提问"的意图，服务端需展开正文）
        //   - 只发文件不提问 → 正文留空，让该气泡由服务端回推的收录回执来承载
        //     （此前写成「已将文件x交给月华」，会在回执之外额外留一个空气泡）
        let userText;
        if (text) userText = (refText ? refText + ' ' : '') + text;
        else userText = '';
        if (userText || textFiles.length) categories.add('text');
        // 纯文件消息标记为 file 类别：气泡里会显示「文件」徽标而不是空的「文本」
        if (!text && textFiles.length) categories.add('file');
        if (!text && !textFiles.length && referencedFiles.length) categories.add('file');

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
