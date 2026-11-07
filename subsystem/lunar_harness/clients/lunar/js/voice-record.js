// ============================================================
//  星月智能 · 消息终端 — 连续语音对话（共享层适配）
//  AEC + VAD 录音与编码由 /shared/core/voice-chat.js 提供，
//  这里只注入桌面端专属的本地历史、引用块与离线提示。
// ============================================================

// ---------- 语音按钮状态 ----------
// idle 待机 / listening 持续监听 / capturing 正在采集用户语音
function updateVoiceUI(state) {
    if (!voiceBtn) return;
    voiceBtn.classList.toggle('listening', state === 'listening');
    voiceBtn.classList.toggle('recording', state === 'capturing');
    if (state === 'capturing') {
        voiceBtn.title = '正在聆听你的声音…';
        voiceBtn.innerHTML = '<i class="fas fa-microphone-lines"></i>';
    } else if (state === 'listening') {
        voiceBtn.title = '连续语音对话中（点击关闭）';
        voiceBtn.innerHTML = '<i class="fas fa-headset"></i>';
    } else {
        voiceBtn.title = '开启连续语音对话';
        voiceBtn.innerHTML = '<i class="fas fa-microphone"></i>';
    }
}

// ---------- 共享语音管理器 ----------
const desktopVoiceChat = window.LunarShared ? new window.LunarShared.VoiceChatManager({
    audioQueue: AudioQueue,
    onStatusChange: updateVoiceUI,
    onError: (type, message) => {
        if (type === 'rate-limited') {
            showToast('月华还没回应上一条语音，稍后再说吧', 'info');
        } else if (type === 'send-failed') {
            showToast('语音发送失败：' + (message || '未知错误'), 'error');
        } else {
            showToast(message || '当前环境不支持连续语音', 'error');
        }
    },
    onUtterance: async ({ file, base64Data }) => {
        // 本地历史展示（独立 blob URL，避免被清理撤销）
        const voiceMessageId = generateId();
        addMessage({
            id: voiceMessageId,
            role: 'user',
            categories: ['voice'],
            content: '',
            attachments: [{ type: 'audio', src: URL.createObjectURL(file), label: file.name }],
            timestamp: Date.now()
        });
        // 前端不再落盘：聊天记录由思维链统一写入缓存文件

        if (!backendConnected) {
            showToast('离线模式：语音仅本地渲染', 'info');
            return false;
        }
        // QQ 适配器标准格式：[用户: xxx]: 前缀 + 音频内容块（句柄与转写由服务端生成）
        const quoteBlock = buildQuoteContentBlock();
        const audioBlock = window.LunarShared.media.audioEnvelope(file, base64Data, 'wav');
        const voiceContent = quoteBlock
            ? [quoteBlock, { type: 'text', text: buildUserNamePrefix() }, audioBlock]
            : [
                { type: 'text', text: buildUserNamePrefix() },
                audioBlock
            ];
        const result = await sendMessages([{ role: 'user', content: voiceContent }]);
        applyAudioHandlesToMessage(voiceMessageId, result && result.handles);
        clearQuotedMessages();
        return true;
    }
}) : null;

// ---------- 语音输入入口 ----------
function setupVoiceInput() {
    if (!voiceBtn || !desktopVoiceChat) return;
    voiceBtn.addEventListener('click', () => {
        if (desktopVoiceChat.enabled) stopVoiceListening(true);
        else startVoiceListening();
    });
}

// 开启连续语音：申请带 AEC 的麦克风并常驻，之后自动检测用户语音
async function startVoiceListening() {
    if (!desktopVoiceChat || desktopVoiceChat.enabled) return;
    await desktopVoiceChat.enable();
    if (desktopVoiceChat.enabled) showToast('连续语音已开启，直接说话即可', 'success');
}

// 关闭连续语音：释放采集链路与麦克风
async function stopVoiceListening(notify) {
    if (!desktopVoiceChat) return;
    const wasEnabled = desktopVoiceChat.enabled;
    await desktopVoiceChat.disable();
    if (notify && wasEnabled) showToast('连续语音已关闭', 'info');
}

// 月华回应感知：WebSocket 收到助手消息时调用，解除语音发送限制
function notifyVoiceAssistantReply() {
    desktopVoiceChat?.notifyAssistantReply();
}
