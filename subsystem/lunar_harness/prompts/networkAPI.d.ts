// ═══════════════════════════════════════════════════════════
// 网络能力（http.* / network.*）
//
// 全局 fetch / WebSocket 见 runtimeAPI.d.ts。此处为同步 HTTP 与裸套接字。
// ═══════════════════════════════════════════════════════════

/** 裸套接字（network.tcpConnect / udpConnect / udpListen 返回；方法均为同步阻塞调用） */
interface NetSocket {
	/**
	 * 发送文本数据
	 *
	 * TCP 直写；已连接 UDP（udpConnect）发往固定对端；udpListen 监听套接字需用 sendTo
	 */
	send(data: string): { success: boolean; error?: string }
	/**
	 * 阻塞读取一段文本（带超时，秒；<=0 时用默认 5 秒）
	 *
	 * @returns { success, data }；udpListen 监听套接字额外携带对端 host / port
	 */
	receive(timeoutSec?: number): { success: boolean; data?: string; host?: string; port?: number; error?: string }
	/**
	 * 向指定目标发送 UDP 数据（仅 udpListen 监听套接字；已连接 UDP 请用 send）
	 */
	sendTo(host: string, port: number, data: string): { success: boolean; error?: string }
	/** 关闭套接字 */
	close(): { success: boolean; error?: string }
}

/** 同步 HTTP 请求 */
export const http: {
	/**
	 * 同步 GET；阻塞直到返回
	 *
	 * @param url 请求地址
	 * @param headers 可选请求头
	 * @returns { success, status, body }；传输失败时 success=false 含 error
	 */
	get(url: string, headers?: Record<string, string>): { success: boolean; status: number; body: string; error?: string }
	/**
	 * 同步 POST；阻塞直到返回
	 *
	 * @param url 请求地址
	 * @param body 请求体（JSON 字符串）
	 * @param headers 可选请求头
	 * @returns { success, status, body }；传输失败时 success=false 含 error
	 */
	post(url: string, body: string, headers?: Record<string, string>): { success: boolean; status: number; body: string; error?: string }
	/**
	 * 同步下载任意文件到数据目录并落盘
	 *
	 * @param url 文件直链
	 * @param savePath 相对数据目录的保存路径；为空时按 URL 末段命名
	 * @returns { success:true, path: 相对路径, size: 字节数 }；失败返回 { success:false, error }
	 */
	download(url: string, savePath?: string): { success: boolean; path?: string; size?: number; error?: string }
}

/** 裸 TCP/UDP/DNS */
export const network: {
	/** 解析主机名的全部地址 */
	resolveDNS(hostname: string, timeoutSec?: number): { success: boolean; addresses?: string[]; error?: string }
	/** 解析 SRV 记录 */
	resolveSRV(service: string, proto: string, hostname: string, timeoutSec?: number): { success: boolean; targets?: Array<{ target: string; port: number }>; error?: string }
	/** 建立 TCP 连接，返回套接字 */
	tcpConnect(host: string, port: number, timeoutSec?: number): NetSocket
	/** 建立已「连接」的 UDP 套接字 */
	udpConnect(host: string, port: number, timeoutSec?: number): NetSocket
	/** 在指定端口监听 UDP */
	udpListen(host?: string, port?: number): NetSocket
}
