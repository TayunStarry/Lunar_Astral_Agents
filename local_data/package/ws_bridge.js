// ==== ws_bridge.js — 引擎/工作室统一通信客户端 ====
// 用途：替代 BroadcastChannel，采用与常规前端一致的联络策略：
//   - 接收：WebSocket 连接 /ws，后端无差别广播 {type,data} 格式消息，客户端自行过滤
//   - 发送：HTTP POST /write/engine（格式与 /write/message 同构），后端本地广播 + 转发智能体侧
// 用法：const channel = new WsBridge('integrated-studio-bus');
//       channel.postMessage(msg);  // 发送消息（POST /write/engine）
//       channel.onmessage = (event) => { ... };  // 接收消息（ws /ws 下行）
// 兼容性：API 与 BroadcastChannel 完全一致，迁移时无需修改业务逻辑
//
// 特性：
//   - 自动重连（指数退避，最大延迟 30 秒）
//   - 连接状态跟踪（readyState 属性）
//   - 发送不依赖 ws 连接状态（走 HTTP，连接未就绪时仍可发送）

class WsBridge {
    /** 频道名称（保留参数，兼容 BroadcastChannel API） */
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
    /** WebSocket URL（自动从当前页面 host 推导，连接同源 /ws） */
    #url;
    /** onmessage 设置前收到的入站消息缓冲队列 */
    #incomingBuffer = [];

    /**
     * 构造函数
     * @param {string} name - 频道名称（保留参数，兼容 BroadcastChannel API）
     * 连接地址默认同源 /ws；可通过 URL 参数 ?ws=host:port 覆盖目标后端（用于嵌入页指定智能体所在端口）
     */
    constructor(name) {
        this.#name = name || 'integrated-studio-bus';
        // WebSocket 连接地址：默认同源 /ws（与常规前端一致的标准通道）
        // 支持 URL 参数 ?ws=host[:port] 覆盖目标后端（如消息终端嵌入引擎时指向智能体所在端口 36789）
        const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
        const wsParam = new URLSearchParams(window.location.search).get('ws');
        if (wsParam) {
            this.#url = /^wss?:\/\//i.test(wsParam) ? wsParam : `${protocol}//${wsParam}/ws`;
        } else {
            this.#url = `${protocol}//${window.location.host}/ws`;
        }
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
     * 通过 HTTP POST /write/engine 提交：后端广播给所有 /ws 客户端（模块间互通）并转发智能体侧
     * @param {*} message - 任意可序列化的消息对象
     */
    postMessage(message) {
        const payload = JSON.stringify(message);
        fetch('/write/engine', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: payload,
        }).catch(err => {
            console.warn('[WsBridge] 消息发送失败:', err);
        });
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
