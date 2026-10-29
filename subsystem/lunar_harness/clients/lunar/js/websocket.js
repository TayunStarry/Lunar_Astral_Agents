// ============================================================
//  星月智能 · 消息终端 — WebSocket 消息处理与连接管理
// ============================================================

function updateConnectionStatusUI(state) {
    if (state === 'connected') {
        sendButton.classList.add('connected');
    } else {
        sendButton.classList.remove('connected');
    }
}

// ---------- WebSocket 消息处理 ----------
function handleWebSocketMessage(msg) {
    const type = msg.type || '';
    const data = msg.data || {};

    // 月华回应感知：助手侧消息到达即解除连续语音的发送限制
    if (type === 'context' || type === 'image') {
        notifyVoiceAssistantReply();
    }

    if (type === 'context') {
        const subType = data.type || 'text';
        const content = data.content || '';
        const audio = data.audio || '';

        if (subType === 'music') {
            addMessage({ id: generateId(), role: 'assistant', categories: ['music'], content: '', abcNotation: content, timestamp: Date.now() });
            return;
        }
        if (subType === 'music_audio') {
            try {
                const audioData = JSON.parse(content || '{}');
                if (audioData.type === 'audio_ready' && audioData.audio_url) {
                    playRenderedAudio(audioData.audio_url, audioData.file_name);
                }
            } catch (e) {
                console.warn('乐谱音频数据解析失败', e);
            }
            return;
        }
        if (subType === 'action') {
            addMessage({ id: generateId(), role: 'assistant', categories: ['action'], content, actionType: subType, timestamp: Date.now() });
            return;
        }

        // 资源收录回执：渲染为可点击的资源图标（按钮），点击把该资源句柄载入输入框。
        // 模型可能在回复中回显 `[已收录] @D.003 文件名.md`，按回执渲染以保持可点击。
        const fileNotice = /^\[已收录\]\s*(@[DIVAGTU]\.\d{3,})\s*(.*)$/.exec(content.trim());
        if (fileNotice) {
            addMessage({
                id: generateId(), role: 'assistant', categories: ['text', 'file'],
                content: fileNotice[0], fileRef: fileNotice[1], fileLabel: fileNotice[2] || fileNotice[1],
                timestamp: Date.now()
            });
            return;
        }
        // 历史回执兼容：旧的 `[文件已导入] #文件名.ext` 形态（迁移期内仍可能出现）
        const legacyNotice = /^\[文件已导入\]\s*(#[\w.-]+)$/.exec(content.trim());
        if (legacyNotice) {
            addMessage({
                id: generateId(), role: 'assistant', categories: ['text', 'file'],
                content: legacyNotice[0], fileRef: legacyNotice[1], fileLabel: legacyNotice[1],
                timestamp: Date.now()
            });
            return;
        }

        // 文本 / 思考 / 代码等上下文消息（可能携带 TTS 音频）
        const categories = ['text'];
        if (audio) categories.push('voice');
        addMessage({ id: generateId(), role: 'assistant', categories, content, audio: audio || '', timestamp: Date.now() });
        if (audio && autoPlayVoice) AudioQueue.enqueue(audio);
        return;
    }

    if (type === 'image') {
        const images = data.images || [];
        const isSticker = !!data.sticker;
        images.forEach(img => {
            // 图片统一以 URL 传递：/file/read/... 与 http(s):// 直接使用；data: 仅兼容历史消息
            const src = (img.startsWith('data:') || img.startsWith('http') || img.startsWith('/'))
                ? img
                : ('/file/read/' + img.replace(/^\.?\//, ''));
            addMessage({ id: generateId(), role: 'assistant', categories: ['image'], content: '', imageSrc: src, imageLabel: isSticker ? '表情包' : '图片', timestamp: Date.now() });
        });
        return;
    }

    // 引擎控制消息（动作/移动指令等）：仅供引擎侧消费，不在聊天区展示原文
    if (type === 'engine') {
        return;
    }

    // 未知格式：以 JSON 文本兜底展示
    addMessage({ id: generateId(), role: 'assistant', categories: ['text'], content: '```json\n' + JSON.stringify(msg, null, 2) + '\n```', timestamp: Date.now() });
}

// ---------- WebSocket 连接管理 ----------
function connectWebSocket() {
    if (ws && (ws.readyState === WebSocket.OPEN || ws.readyState === WebSocket.CONNECTING)) return;
    try {
        ws = new WebSocket(WS_URL);
    } catch (err) {
        scheduleReconnect();
        return;
    }

    ws.onopen = () => {
        reconnectAttempts = 0;
        backendConnected = true;
        updateConnectionStatusUI('connected');
    };

    ws.onmessage = (event) => {
        try {
            const parsed = JSON.parse(event.data);
            handleWebSocketMessage(parsed);
        } catch {
            addMessage({ id: generateId(), role: 'assistant', categories: ['text'], content: event.data, timestamp: Date.now() });
        }
    };

    ws.onerror = () => {
        updateConnectionStatusUI('disconnected');
    };

    ws.onclose = () => {
        backendConnected = false;
        updateConnectionStatusUI('disconnected');
        if (!manualClose) scheduleReconnect();
    };
}

function scheduleReconnect() {
    if (manualClose) return;
    if (reconnectAttempts >= MAX_RECONNECT_ATTEMPTS) {
        backendConnected = false;
        updateConnectionStatusUI('failed');
        showToast('后端连接失败，已进入本地模式', 'error');
        return;
    }
    const delay = RECONNECT_BASE_DELAY * Math.pow(1.5, reconnectAttempts);
    reconnectTimer = setTimeout(() => {
        reconnectAttempts++;
        connectWebSocket();
    }, delay);
}
