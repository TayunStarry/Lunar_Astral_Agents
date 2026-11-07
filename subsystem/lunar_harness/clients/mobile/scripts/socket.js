/**
 * 移动端 WebSocket：连接 / 重连 / 消息分发统一由共享层实现。
 * 移动端保留 5 次、固定 3 秒的重连参数（共享层默认值）。
 */
export { WebSocketClient } from '/shared/core/socket.js';
