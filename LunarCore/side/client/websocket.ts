import { WebSocketMessage } from './types';

/**
 * WebSocket客户端类
 *
 * 提供WebSocket连接管理、自动重连、消息处理等功能
 */
export class WebSocketClient {
	private url: string;
	private ws: WebSocket | null = null;
	private messageHandlers: ((message: WebSocketMessage) => void)[] = [];
	private connectionHandlers: (() => void)[] = [];
	private errorHandlers: ((error: Event) => void)[] = [];
	private reconnectAttempts = 0;
	private readonly maxReconnectAttempts = 5;
	private readonly reconnectDelay = 3000;
	private reconnectTimer: number | null = null;

	/**
	 * 创建WebSocket客户端实例
	 *
	 * @param {string} url - WebSocket服务器地址
	 */
	constructor(url: string) {
		this.url = url;
	}

	/**
	 * 建立WebSocket连接
	 */
	connect(): void {
		if (this.ws?.readyState === WebSocket.OPEN) {
			return;
		}

		try {
			this.ws = new WebSocket(this.url);
			this.setupEventListeners();
		} catch (error) {
			console.error('WebSocket connection error:', error);
			this.scheduleReconnect();
		}
	}

	/**
	 * 设置WebSocket事件监听器
	 */
	private setupEventListeners(): void {
		if (!this.ws) return;

		this.ws.onopen = () => {
			console.log('WebSocket connected');
			this.reconnectAttempts = 0;
			this.connectionHandlers.forEach(handler => handler());
		};

		this.ws.onmessage = (event: MessageEvent) => {
			try {
				const message = JSON.parse(event.data) as WebSocketMessage;
				this.messageHandlers.forEach(handler => handler(message));
			} catch (error) {
				console.error('Failed to parse WebSocket message:', error);
			}
		};

		this.ws.onerror = (error: Event) => {
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
	private scheduleReconnect(): void {
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
	disconnect(): void {
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
	send(data: string | object): void {
		if (this.ws?.readyState === WebSocket.OPEN) {
			const message = typeof data === 'string' ? data : JSON.stringify(data);
			this.ws.send(message);
		} else {
			console.warn('WebSocket is not connected');
		}
	}

	/**
	 * 注册消息处理器
	 *
	 * @param {(message: WebSocketMessage) => void} handler - 消息回调函数
	 */
	onMessage(handler: (message: WebSocketMessage) => void): void {
		this.messageHandlers.push(handler);
	}

	/**
	 * 注册连接成功处理器
	 *
	 * @param {() => void} handler - 连接成功回调函数
	 */
	onConnect(handler: () => void): void {
		this.connectionHandlers.push(handler);
	}

	/**
	 * 注册错误处理器
	 *
	 * @param {(error: Event) => void} handler - 错误回调函数
	 */
	onError(handler: (error: Event) => void): void {
		this.errorHandlers.push(handler);
	}

	/**
	 * 检查是否已连接
	 *
	 * @returns {boolean} - 是否已连接
	 */
	isConnected(): boolean {
		return this.ws?.readyState === WebSocket.OPEN;
	}
}
