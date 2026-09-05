package main

// ==== 类型定义集中区（LTP3 权限密钥生成器） ====

// genResponse 密钥生成结果响应。
type genResponse struct {
	Success     bool     `json:"success"`
	Error       string   `json:"error,omitempty"`
	Hash        string   `json:"hash,omitempty"`        // 脚本拼接哈希（hex，128 位截断）
	Key         string   `json:"key,omitempty"`         // 生成的权限密钥内容
	Length      int      `json:"length,omitempty"`      // 密钥长度
	Permissions []string `json:"permissions,omitempty"` // 已加密的权限名
	FileCount   int      `json:"file_count,omitempty"`  // 参与哈希的脚本数
	Files       []string `json:"files,omitempty"`       // 参与哈希的脚本文件名（排序后）
}

// verifyRequest 密钥校验请求。
type verifyRequest struct {
	Key    string `json:"key"`
	Cipher string `json:"cipher"`
}

// verifyBlock 单条密钥字符串的解码信息。
type verifyBlock struct {
	Index int    `json:"index"`          // 第几条
	Len   int    `json:"len"`            // 该条长度
	Text  string `json:"text"`           // 该条原文（含填充）
	Perm  string `json:"perm,omitempty"` // 去掉填充后恢复的权限名
}

// verifyResponse 密钥校验响应。
type verifyResponse struct {
	Success bool          `json:"success"`
	Error   string        `json:"error,omitempty"`
	Plain   string        `json:"plain,omitempty"`   // 整体解码出的明文报文
	Blocks  []verifyBlock `json:"blocks,omitempty"`   // 分割后的各条
	Perms   []string      `json:"perms,omitempty"`    // 恢复出的权限名
}