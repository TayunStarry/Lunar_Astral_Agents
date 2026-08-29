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

        // 文件导入通知：渲染为可点击的文件图标（按钮），点击载入输入框
        const fileNotice = /^\[文件已导入\]\s*(#[\w.-]+)$/.exec(content.trim());
        if (fileNotice) {
            addMessage({
                id: generateId(), role: 'assistant', categories: ['text', 'file'],
                content: fileNotice[0], fileRef: fileNotice[1], timestamp: Date.now()
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
            const src = (img.startsWith('data:') || img.startsWith('http')) ? img : ('data:image/jpeg;base64,' + img);
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
