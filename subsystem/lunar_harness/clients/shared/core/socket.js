/**
 * 星月智能 · 桌面端 / 移动端共享 WebSocket 客户端
 *
 * 统一连接、自动重连、消息分发与状态回调。
 * 重连次数、退避曲线由各端通过 options 注入（桌面 3 次递增、移动 5 次固定）。
 */

export class WebSocketClient {
	/**
	 * @param {string} url
	 * @param {object} [options]
	 * @param {number} [options.maxReconnectAttempts=5]
	 * @param {number} [options.reconnectDelay=3000] 固定重连间隔（ms）
	 * @param {(attempt:number)=>number} [options.getReconnectDelay] 自定义退避（优先于 reconnectDelay）
	 * @param {(status:string, detail:object)=>void} [options.onStatus] 状态：connecting/open/reconnecting/closed/failed
	 * @param {(socket:WebSocket)=>void} [options.onSocketChange]
	 * @param {(raw:string)=>void} [options.onParseError] JSON 解析失败时的兜底
	 */
	constructor(url, options = {}) {
		this.url = url;
		this.ws = null;
		this.messageHandlers = [];
		this.connectionHandlers = [];
		this.errorHandlers = [];
		this.statusHandlers = [];
		this.parseErrorHandlers = [];
		this.reconnectAttempts = 0;
		this.maxReconnectAttempts = options.maxReconnectAttempts ?? 5;
		this.reconnectDelay = options.reconnectDelay ?? 3000;
		this.getReconnectDelay = options.getReconnectDelay || null;
		this.reconnectTimer = null;
		this.closedByUser = false;
		if (options.onStatus) this.onStatus(options.onStatus);
		if (options.onParseError) this.onParseError(options.onParseError);
		this.onSocketChange = options.onSocketChange || null;
	}

	/** 当前连接是否可用 */
	isConnected() {
		return this.ws?.readyState === WebSocket.OPEN;
	}

	/** 建立连接（OPEN / CONNECTING 时直接返回） */
	connect() {
		if (this.ws && (this.ws.readyState === WebSocket.OPEN || this.ws.readyState === WebSocket.CONNECTING)) return;
		this.emitStatus('connecting');
		try {
			this.ws = new WebSocket(this.url);
			this.onSocketChange?.(this.ws);
			this.setupEventListeners();
		} catch (error) {
			console.error('WebSocket connection error:', error);
			this.scheduleReconnect();
		}
	}

	setupEventListeners() {
		if (!this.ws) return;
		this.ws.onopen = () => {
			this.reconnectAttempts = 0;
			this.emitStatus('open');
			this.connectionHandlers.forEach(handler => handler());
		};
		this.ws.onmessage = (event) => {
			try {
				const message = JSON.parse(event.data);
				this.messageHandlers.forEach(handler => handler(message));
			} catch (error) {
				console.error('Failed to parse WebSocket message:', error);
				if (this.parseErrorHandlers.length) {
					this.parseErrorHandlers.forEach(handler => handler(event.data));
				}
			}
		};
		this.ws.onerror = (error) => {
			console.error('WebSocket error:', error);
			this.errorHandlers.forEach(handler => handler(error));
			this.emitStatus('error');
		};
		this.ws.onclose = () => {
			this.emitStatus('closed');
			if (!this.closedByUser) this.scheduleReconnect();
		};
	}

	scheduleReconnect() {
		if (this.closedByUser) return;
		if (this.reconnectAttempts >= this.maxReconnectAttempts) {
			this.emitStatus('failed', { attempts: this.reconnectAttempts });
			return;
		}
		if (this.reconnectTimer) clearTimeout(this.reconnectTimer);
		const delay = this.getReconnectDelay
			? this.getReconnectDelay(this.reconnectAttempts + 1)
			: this.reconnectDelay;
		this.reconnectTimer = window.setTimeout(() => {
			this.reconnectAttempts++;
			this.connect();
		}, delay);
		this.emitStatus('reconnecting', { attempt: this.reconnectAttempts + 1, delay });
	}

	/** 主动断开，不再触发自动重连 */
	disconnect() {
		this.closedByUser = true;
		if (this.reconnectTimer) {
			clearTimeout(this.reconnectTimer);
			this.reconnectTimer = null;
		}
		if (this.ws) {
			this.ws.onclose = null;
			this.ws.close();
			this.ws = null;
			this.onSocketChange?.(null);
		}
	}

	/** 发送消息（对象自动 JSON 序列化） */
	send(data) {
		if (this.isConnected()) {
			this.ws.send(typeof data === 'string' ? data : JSON.stringify(data));
			return true;
		}
		console.warn('WebSocket is not connected');
		return false;
	}

	onMessage(handler) { this.messageHandlers.push(handler); }
	onConnect(handler) { this.connectionHandlers.push(handler); }
	onError(handler) { this.errorHandlers.push(handler); }
	onStatus(handler) { this.statusHandlers.push(handler); }
	onParseError(handler) { this.parseErrorHandlers.push(handler); }

	emitStatus(status, detail = {}) {
		this.statusHandlers.forEach(handler => handler(status, detail));
	}
}
