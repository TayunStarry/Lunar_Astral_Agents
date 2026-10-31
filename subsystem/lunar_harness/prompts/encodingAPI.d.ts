// ═══════════════════════════════════════════════════════════
// 编解码与哈希签名（encoding.* / hash.*）
//
// 可失败方法统一 Result 风格（返回 { success, text?, error? }），不抛异常。
// ═══════════════════════════════════════════════════════════

/** 编解码工具 */
export const encoding: {
	/** base64 编码；data 为 UTF-8 字符串 */
	base64Encode(data: any): string
	/** base64 解码；返回 { success, text?, error? } */
	base64Decode(s: string): { success: boolean; text?: string; error?: string }
	/** URL 编码（查询转义） */
	urlEncode(s: string): string
	/** URL 解码；返回 { success, text?, error? } */
	urlDecode(s: string): { success: boolean; text?: string; error?: string }
	/**
	 * 加密
	 *
	 * @param key 加解密密钥字符串
	 * @param content 要加密的明文
	 * @returns { success, text: 密文, error? }
	 */
	lunarEncoder(key: string, content: string): { success: boolean; text?: string; error?: string }
	/**
	 * 解密
	 *
	 * @param key 加解密密钥字符串
	 * @param cipher 要解密的密文
	 * @returns { success, text: 原文, error? }
	 */
	lunarDecoder(key: string, cipher: string): { success: boolean; text?: string; error?: string }
}

/** 哈希工具（签名类方法统一 Result 风格） */
export const hash: {
	/**
	 * 生成 JWT 令牌
	 *
	 * @param claims 令牌负载对象
	 * @param secret HS256 时为其 HMAC 密钥；EdDSA 时为 Ed25519 PEM(PKCS8) 私钥
	 * @param algorithm 算法，默认 HS256；可选 HS256 / EdDSA(Ed25519) / none
	 * @param kid 可选；非空时写入 JWT 头（多密钥按 Key ID 选择签名密钥）
	 * @returns { success, text: JWT 字符串, error? }
	 */
	signJWT(claims: Record<string, unknown>, secret: string, algorithm?: string, kid?: string): { success: boolean; text?: string; error?: string }
	/** MD5 摘要；@param data 输入字符串；@returns 小写十六进制摘要 */
	md5(data: string): string
	/** SHA-1 摘要；@param data 输入字符串；@returns 小写十六进制摘要 */
	sha1(data: string): string
	/** SHA-256 摘要；@param data 输入字符串；@returns 小写十六进制摘要 */
	sha256(data: string): string
	/** HMAC-SHA1；@param key 密钥；@param data 输入字符串；@returns 小写十六进制 MAC */
	hmacSha1(key: string, data: string): string
	/** HMAC-SHA256；@param key 密钥；@param data 输入字符串；@returns 小写十六进制 MAC */
	hmacSha256(key: string, data: string): string
	/**
	 * Ed25519 签名
	 *
	 * @param privKeyPemOrSeed Ed25519 私钥（PEM(PKCS8) 或 32/64 字节原始字节）
	 * @param data 待签名数据
	 * @returns { success, text: base64url 签名, error? }
	 */
	ed25519Sign(privKeyPemOrSeed: string, data: string): { success: boolean; text?: string; error?: string }
	/**
	 * 生成 EdDSA (Ed25519) JWT
	 *
	 * @param claims 令牌负载对象
	 * @param privKeyPemOrSeed Ed25519 私钥（PEM(PKCS8) 或 32/64 字节原始字节）
	 * @param kid 可选；非空时写入 JWT 头
	 * @returns { success, text: JWT 字符串, error? }
	 */
	generateJWT(claims: Record<string, unknown>, privKeyPemOrSeed: string, kid?: string): { success: boolean; text?: string; error?: string }
}
