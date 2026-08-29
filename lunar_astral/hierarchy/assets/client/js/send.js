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
            if (category === 'image' || category === 'video') {
                try {
                    const fileUrl = await saveFile(pf.file);
                    contentBlocks.push({ type: 'image_url', image_url: { url: fileUrl } });
                    attachments.push({ type: category, src: fileUrl.replace(window.location.origin, ''), label: pf.name });
                    categories.add('image');
                } catch (err) {
                    showToast(`无法上传 ${pf.name}`, 'error');
                }
            } else if (category === 'audio') {
                // 音频：wav/mp3 以 input_audio 形式推送，其余仅本地展示
                try {
                    const base64Data = await fileToRawBase64(pf.file);
                    const format = getAudioFormat(pf.file);
                    if (format) {
                        contentBlocks.push({ type: 'input_audio', input_audio: { data: base64Data, format } });
                    } else {
                        showToast(`音频 ${pf.name} 仅支持 wav/mp3，已跳过发送`, 'error');
                    }
                } catch (err) {
                    showToast(`无法读取音频 ${pf.name}`, 'error');
                }
                // 历史记录使用独立 blob URL，避免被清理撤销
                attachments.push({ type: 'audio', src: URL.createObjectURL(pf.file), label: pf.name });
                categories.add('voice');
            } else if (category === 'text') {
                try {
                    // 文本文件 → 构造阅读者可解析的围栏块 ```fileName\n全文\n```（发送全文入库）
                    const rawText = await readFileAsText(pf.file);
                    if (rawText.trim()) {
                        textFiles.push({ name: pf.name, block: `\`\`\`${pf.name}\n${rawText}\n\`\`\`` });
                        categories.add('text');
                    }
                } catch (err) {
                    showToast(`无法读取文件 ${pf.name}`, 'error');
                }
            } else {
                try {
                    const fileUrl = await saveFile(pf.file);
                    const block = `【文件 ${pf.name}】访问链接：${fileUrl}`;
                    contentBlocks.push({ type: 'text', text: block });
                    attachments.push({ type: 'other', src: fileUrl.replace(window.location.origin, ''), label: pf.name });
                    categories.add('text');
                } catch (err) {
                    showToast(`无法上传文件 ${pf.name}`, 'error');
                }
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
        const sendPayload = [];
        for (const tf of textFiles) sendPayload.push({ role: 'user', content: tf.block });
        if (userText || contentBlocks.length) {
            const mainContent = contentBlocks.length
                ? [...(userText ? [{ type: 'text', text: userText }] : []), ...contentBlocks]
                : userText;
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
        schedulePersist();
        showToast('已清空消息', 'info');
    });
}
