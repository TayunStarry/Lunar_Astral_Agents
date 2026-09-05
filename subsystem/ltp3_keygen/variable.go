package main

// ==== 常量与变量集中区（LTP3 权限密钥生成器） ====

import "flag"

// 权限密钥相关常量（与引擎 agent/YaraLTP 保持完全一致）。
const (
	// permSeparator 权限密钥字符串之间的分隔符（取自 base64 字符集；权限名不含它）。
	permSeparator = "+"
	// permPadding 每条密钥字符串的填充字符（同样取自 base64 字符集；权限名不含它）。
	permPadding = "/"
	// perPermissionKeyLen 每条权限密钥字符串的长度。
	perPermissionKeyLen = 32
)

// serverPort 本地服务端口（默认 0 = 随机可用端口）。
var serverPort = flag.Int("port", 0, "本地服务端口，0 表示随机可用端口")

// allPermissions 权限名全集（下拉框候选，与引擎 AllPermissionNames 保持一致）。
var allPermissions = []string{
	"event.subscribe", "event.publish",
	"hook.register",
	"command.register",
	"tool.register",
	"event_handler.register",
	"llm_provider.register",
	"api.register", "api.call",
	"send.text", "send.image", "send.emoji", "send.hybrid",
	"http.request",
	"network.tcp", "network.udp",
	"platform.command",
	"encoding.use", "time.use", "crypto.use",
	"model.access",
	"plugin.config.read", "plugin.config.write",
	"plugin.file.read", "plugin.file.write",
	"data.directory.read", "data.directory.write",
	"database.read",
	"knowledge.search",
	"async_task.execute",
	"emoji.access",
}

// permIndex 权限名 → 是否合法。
var permIndex = func() map[string]bool {
	m := map[string]bool{}
	for _, p := range allPermissions {
		m[p] = true
	}
	return m
}()