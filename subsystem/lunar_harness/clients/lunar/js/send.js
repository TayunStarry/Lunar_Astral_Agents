// ============================================================
//  星月智能 · 消息终端 — 消息发送与输入事件
// ============================================================

// ---------- 消息发送（共享层适配） ----------
function sendMessages(payload) {
    return window.LunarShared.api.sendMessages(payload);
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
        const envelopes = [];          // 内容信封（六类：图片/动图/视频/音频/文件/网页）
        const attachments = [];        // 本地附件预览
        const categories = new Set();
        const quoteSnapshots = quotedMessages.map(quoteSnapshotOfMessage);
        const quoteBlock = buildQuoteContentBlock();

        for (const pf of pendingFiles) {
            const category = pf.category;
            switch (category) {
                // 多模态消息 -> 图片
                case 'image':
                    try {
                        const fileUrl = await saveFile(pf.file);
                        envelopes.push(window.LunarShared.media.imageEnvelope(pf.file, fileUrl));
                        attachments.push({ type: category, src: fileUrl.replace(window.location.origin, ''), label: pf.name });
                        categories.add('image');
                    }
                    catch (err) { showToast(`无法上传 ${pf.name}`, 'error'); }
                    break;
                // 多模态消息 -> 视频（句柄由服务端入库后生成，不在正文写占位符）
                case 'video':
                    try {
                        const fileUrl = await saveFile(pf.file);
                        envelopes.push(window.LunarShared.media.videoEnvelope(pf.file, fileUrl));
                        attachments.push({ type: 'video', src: fileUrl.replace(window.location.origin, ''), label: pf.name });
                        categories.add('image');
                    }
                    catch (err) { showToast(`无法上传 ${pf.name}`, 'error'); }
                    break;
                // 多模态消息 -> 语音/音频（统一内联下发，服务端转写，不落盘）
                case 'audio':
                    try {
                        const base64Data = await fileToRawBase64(pf.file);
                        const format = getAudioFormat(pf.file);
                        envelopes.push(window.LunarShared.media.audioEnvelope(pf.file, base64Data, format));
                    }
                    catch (err) { showToast(`无法读取音频 ${pf.name}`, 'error'); }
                    // 历史记录使用独立 blob URL，避免被清理撤销
                    attachments.push({ type: 'audio', src: URL.createObjectURL(pf.file), label: pf.name });
                    categories.add('voice');
                    break;
                // 文本 / 其他文件 -> file 信封（服务端按内容哈希去重并生成句柄）
                case 'text':
                default:
                    try {
                        const fileUrl = await saveFile(pf.file);
                        envelopes.push(window.LunarShared.media.fileEnvelope(pf.file, fileUrl));
                        // 本地气泡保留原始文件名与大小；信封内对应 name / bytes
                        attachments.push({ type: 'file', label: pf.name, size: formatFileSize(pf.file.size) });
                        categories.add('file');
                    }
                    catch (err) {
                        // 上传失败时退回文本，至少让月华知道用户带了什么文件
                        envelopes.push({ type: 'text', text: `${pf.name}（${formatFileSize(pf.file.size)}）` });
                        categories.add('text');
                    }
                    break;
            }
        }
        // 输入框里的 URL 由前端抽成 webpage 信封，正文保持纯文本
        const textSplit = await window.LunarShared.media.extractWebpageEnvelopes(text);
        envelopes.push(...textSplit.envelopes);
        // ---- 组装引用与主用户文本（一次导入/引用只显示一个气泡）----
        // 手动引用的文档统一为规范句柄 @D.003，正文由月华按需用 fetch_resources 读取；
        // 待发送附件不再预入库，统一由 file 信封在服务端按内容哈希入库。
        const handleRefs = referencedFiles
            .map(id => String(id).trim().replace(/^[#@]/, ''))
            .filter(Boolean);
        const refText = [...new Set(handleRefs)].map(id => `@${id}`).join(' ');
        // 主用户文本：有文字时带引用，只发引用时正文就是句柄
        const userText = text ? (refText ? refText + ' ' : '') + text : refText;
        const backendText = textSplit.text ? (refText ? refText + ' ' : '') + textSplit.text : refText;
        if (text) categories.add('text');
        if (referencedFiles.length) categories.add('file');

        // ---- 组装发送给后端的消息数组 ----
        // 发送给月华的主文本带 QQ 适配器同款用户名前缀；本地展示（addMessage）仍用不带前缀的 userText
        const payloadText = backendText ? buildUserNamePrefix() + backendText : backendText;
        const sendPayload = [];
        if (userText || envelopes.length || quoteBlock) {
            const blocks = [];
            if (quoteBlock) blocks.push(quoteBlock);
            if (payloadText) blocks.push({ type: 'text', text: payloadText });
            blocks.push(...envelopes);
            sendPayload.push({ role: 'user', content: envelopes.length ? blocks : (quoteBlock ? blocks : payloadText) });
        }

        // ---- 本地历史展示（单个气泡）----
        const localMessageId = generateId();
        addMessage({
            id: localMessageId,
            role: 'user',
            categories: categories.size ? Array.from(categories) : ['text'],
            content: userText,
            attachments: attachments.length ? attachments : undefined,
            quotes: quoteSnapshots.length ? quoteSnapshots : undefined,
            timestamp: Date.now()
        });
        // 用户发送消息仅做内存展示；聊天记录由思维链统一写入缓存文件

        // 推送到后端
        if (backendConnected) {
            if (sendPayload.length) {
                const result = await sendMessages(sendPayload);
                applyAudioHandlesToMessage(localMessageId, result && result.handles);
            }
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
        clearQuotedMessages();
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
}
