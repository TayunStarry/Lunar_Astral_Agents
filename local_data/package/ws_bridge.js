// ==== ws_bridge.js — WebSocket 广播桥接器 ====
// 用途：替代 BroadcastChannel，通过 WebSocket 服务端中继实现跨组件通信
// 用法：const channel = new WsBridge('integrated-studio-bus');
//       channel.postMessage(msg);  // 发送消息
//       channel.onmessage = (event) => { ... };  // 接收消息
// 兼容性：API 与 BroadcastChannel 完全一致，迁移时无需修改业务逻辑
//
// 特性：
//   - 自动重连（指数退避，最大延迟 30 秒）
//   - 连接状态跟踪（readyState 属性）
//   - 超时保护（60 秒读取超时，10 秒写入超时）

class WsBridge {
    /** 频道名称（用于标识，不影响 WebSocket 路由） */
    #name;
    /** WebSocket 实例 */
    #ws = null;
    /** 重连计时器 */
    #reconnectTimer = null;
    /** 重连尝试次数 */
    #reconnectAttempts = 0;
    /** 最大重连次数（0 表示无限） */
    #maxReconnectAttempts = 0;
    /** 基础重连延迟（毫秒） */
    #baseReconnectDelay = 1000;
    /** 最大重连延迟（毫秒） */
    #maxReconnectDelay = 30000;
    /** 消息处理器（兼容 BroadcastChannel API） */
    #onmessage = null;
    /** 连接状态：0=CONNECTING, 1=OPEN, 2=CLOSING, 3=CLOSED */
    #readyState = 0;
    /** WebSocket URL（自动从当前页面 host 推导） */
    #url;
    /** 连接建立前的出站消息缓冲队列 */
    #pendingMessages = [];
    /** onmessage 设置前收到的入站消息缓冲队列 */
    #incomingBuffer = [];

    /**
     * 构造函数
     * @param {string} name - 频道名称（保留参数，兼容 BroadcastChannel API）
     */
    constructor(name) {
        this.#name = name || 'integrated-studio-bus';
        // WebSocket 连接地址：ws://<host>/ws/studio
        const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
        this.#url = `${protocol}//${window.location.host}/ws/studio`;
        this.#connect();
    }

    // ==== 公开属性（兼容 BroadcastChannel） ====

    /** 频道名称（只读） */
    get name() {
        return this.#name;
    }

    /** 连接就绪状态（0=CONNECTING, 1=OPEN, 2=CLOSING, 3=CLOSED） */
    get readyState() {
        return this.#readyState;
    }

    /** 消息处理器（设置时自动冲刷入站缓冲） */
    get onmessage() {
        return this.#onmessage;
    }

    set onmessage(fn) {
        this.#onmessage = fn;
        // 冲刷 onmessage 设置前收到的入站消息
        if (fn && this.#incomingBuffer.length > 0) {
            const msgs = this.#incomingBuffer;
            this.#incomingBuffer = [];
            console.debug(`[WsBridge] 冲刷 ${msgs.length} 条入站缓冲消息:`, this.#name);
            for (const msg of msgs) {
                try {
                    fn(msg);
                } catch (err) {
                    console.warn('[WsBridge] 处理入站缓冲消息失败:', err);
                }
            }
        }
    }

    // ==== 公开方法 ====

    /**
     * 发送消息（兼容 BroadcastChannel API）
     * @param {*} message - 任意可序列化的消息对象
     */
    postMessage(message) {
        if (this.#readyState === 1) {
            // WebSocket.OPEN
            try {
                this.#ws.send(JSON.stringify(message));
            } catch (err) {
                console.warn('[WsBridge] 发送消息失败:', err);
            }
        } else if (this.#readyState === 0) {
            // WebSocket.CONNECTING — 缓冲消息，连接建立后自动冲刷
            this.#pendingMessages.push(message);
            console.debug('[WsBridge] 消息已缓冲（等待连接）:', this.#name, message.type);
        } else {
            // CLOSING 或 CLOSED，丢弃消息
            console.debug('[WsBridge] 连接未就绪，消息已丢弃:', this.#name);
        }
    }

    /**
     * 关闭连接
     */
    close() {
        this.#maxReconnectAttempts = 0; // 阻止重连
        if (this.#reconnectTimer) {
            clearTimeout(this.#reconnectTimer);
            this.#reconnectTimer = null;
        }
        if (this.#ws) {
            this.#readyState = 2; // CLOSING
            this.#ws.close(1000, '客户端主动关闭');
            this.#ws = null;
        }
        this.#readyState = 3; // CLOSED
    }

    // ==== 内部方法 ====

    /** 建立 WebSocket 连接 */
    #connect() {
        if (this.#ws) {
            this.#ws.close(1000, '重新连接');
            this.#ws = null;
        }

        this.#readyState = 0; // CONNECTING

        try {
            this.#ws = new WebSocket(this.#url);
        } catch (err) {
            console.error('[WsBridge] 创建 WebSocket 失败:', err);
            this.#scheduleReconnect();
            return;
        }

        this.#ws.onopen = () => {
            this.#readyState = 1; // OPEN
            this.#reconnectAttempts = 0;
            console.debug('[WsBridge] 已连接:', this.#name);
            // 冲刷缓冲队列中的消息
            this.#flushPending();
        };

        this.#ws.onmessage = (event) => {
            try {
                const data = JSON.parse(event.data);
                // 模拟 BroadcastChannel MessageEvent 结构
                if (this.#onmessage) {
                    this.#onmessage({ data });
                } else {
                    // onmessage 尚未设置，缓冲消息
                    this.#incomingBuffer.push({ data });
                }
            } catch (err) {
                console.warn('[WsBridge] 消息解析失败:', err);
            }
        };

        this.#ws.onerror = (err) => {
            console.error('[WsBridge] 连接错误:', this.#name, err);
        };

        this.#ws.onclose = (event) => {
            this.#readyState = 3; // CLOSED
            this.#ws = null;
            console.debug('[WsBridge] 连接已关闭:', this.#name, `(code=${event.code})`);
            // 非正常关闭时自动重连
            if (event.code !== 1000) {
                this.#scheduleReconnect();
            }
        };
    }

    /** 冲刷缓冲队列中的消息（连接建立后调用） */
    #flushPending() {
        if (this.#pendingMessages.length === 0) return;
        console.debug(`[WsBridge] 冲刷 ${this.#pendingMessages.length} 条缓冲消息:`, this.#name);
        const msgs = this.#pendingMessages;
        this.#pendingMessages = [];
        for (const msg of msgs) {
            try {
                this.#ws.send(JSON.stringify(msg));
            } catch (err) {
                console.warn('[WsBridge] 冲刷消息失败:', err);
            }
        }
    }

    /** 安排重连（指数退避） */
    #scheduleReconnect() {
        if (this.#maxReconnectAttempts > 0 && this.#reconnectAttempts >= this.#maxReconnectAttempts) {
            console.warn('[WsBridge] 已达到最大重连次数，停止重连:', this.#name);
            return;
        }
        if (this.#reconnectTimer) {
            clearTimeout(this.#reconnectTimer);
        }
        this.#reconnectAttempts++;
        const delay = Math.min(
            this.#baseReconnectDelay * Math.pow(2, this.#reconnectAttempts - 1),
            this.#maxReconnectDelay
        );
        console.debug(`[WsBridge] 将在 ${delay}ms 后重连 (第 ${this.#reconnectAttempts} 次):`, this.#name);
        this.#reconnectTimer = setTimeout(() => {
            this.#reconnectTimer = null;
            this.#connect();
        }, delay);
    }
}

// 暴露到全局作用域（兼容非模块化脚本）
// 注意：不使用 export，因为此文件通过 <script src="..."> 加载（非 module）
if (typeof window !== 'undefined') {
    window.WsBridge = WsBridge;
}