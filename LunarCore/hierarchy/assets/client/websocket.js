/**
 * WebSocket客户端类
 *
 * 提供WebSocket连接管理、自动重连、消息处理等功能
 */
export class WebSocketClient {
    url;
    ws = null;
    messageHandlers = [];
    connectionHandlers = [];
    errorHandlers = [];
    reconnectAttempts = 0;
    maxReconnectAttempts = 5;
    reconnectDelay = 3000;
    reconnectTimer = null;

    /**
     * 创建WebSocket客户端实例
     *
     * @param {string} url - WebSocket服务器地址
     */
    constructor(url) {
        this.url = url;
    }

    /**
     * 建立WebSocket连接
     */
    connect() {
        if (this.ws?.readyState === WebSocket.OPEN) {
            return;
        }
        try {
            this.ws = new WebSocket(this.url);
            this.setupEventListeners();
        }
        catch (error) {
            console.error('WebSocket connection error:', error);
            this.scheduleReconnect();
        }
    }

    /**
     * 设置WebSocket事件监听器
     */
    setupEventListeners() {
        if (!this.ws) return;

        this.ws.onopen = () => {
            console.log('WebSocket connected');
            this.reconnectAttempts = 0;
            this.connectionHandlers.forEach(handler => handler());
        };

        this.ws.onmessage = (event) => {
            try {
                const message = JSON.parse(event.data);
                this.messageHandlers.forEach(handler => handler(message));
            }
            catch (error) {
                console.error('Failed to parse WebSocket message:', error);
            }
        };

        this.ws.onerror = (error) => {
            console.error('WebSocket error:', error);
            this.errorHandlers.forEach(handler => handler(error));
        };

        this.ws.onclose = () => {
            console.log('WebSocket closed');
            this.scheduleReconnect();
        };
    }

    /**
     * 安排自动重连
     */
    scheduleReconnect() {
        if (this.reconnectAttempts >= this.maxReconnectAttempts) {
            console.error('Max reconnection attempts reached');
            return;
        }
        if (this.reconnectTimer) {
            clearTimeout(this.reconnectTimer);
        }
        this.reconnectTimer = window.setTimeout(() => {
            this.reconnectAttempts++;
            console.log(`Reconnecting... attempt ${this.reconnectAttempts}`);
            this.connect();
        }, this.reconnectDelay);
    }

    /**
     * 断开WebSocket连接
     */
    disconnect() {
        if (this.reconnectTimer) {
            clearTimeout(this.reconnectTimer);
            this.reconnectTimer = null;
        }
        if (this.ws) {
            this.ws.close();
            this.ws = null;
        }
    }

    /**
     * 发送消息到服务器
     *
     * @param {string | object} data - 消息内容
     */
    send(data) {
        if (this.ws?.readyState === WebSocket.OPEN) {
            const message = typeof data === 'string' ? data : JSON.stringify(data);
            this.ws.send(message);
        }
        else {
            console.warn('WebSocket is not connected');
        }
    }

    /**
     * 注册消息处理器
     *
     * @param {(message: WebSocketMessage) => void} handler - 消息回调函数
     */
    onMessage(handler) {
        this.messageHandlers.push(handler);
    }

    /**
     * 注册连接成功处理器
     *
     * @param {() => void} handler - 连接成功回调函数
     */
    onConnect(handler) {
        this.connectionHandlers.push(handler);
    }

    /**
     * 注册错误处理器
     *
     * @param {(error: Event) => void} handler - 错误回调函数
     */
    onError(handler) {
        this.errorHandlers.push(handler);
    }

    /**
     * 检查是否已连接
     *
     * @returns {boolean} - 是否已连接
     */
    isConnected() {
        return this.ws?.readyState === WebSocket.OPEN;
    }
}