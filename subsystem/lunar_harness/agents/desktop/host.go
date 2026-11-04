//go:build windows

// Package desktop 桌面操纵：多角色协作的桌面自动化智能体。
//
// 编纂者先把自然语言请求收敛成结构化任务，启动者负责把目标应用带到前台，
// 之后由 UIA 理解者 / 规划者 / 单一操作者（键盘、鼠标或 UIA）循环推进，
// 直到达成验收标准或判定停滞。提示词统一放在 prompts/desktop*.md。
package desktop

// ==== 宿主循环辅助 ====
// 编排循环（初始构建 + 执行循环）已上移到 chain/desktop；
// 本包只提供各阶段模型调用与确定性辅助函数。

import (
	"encoding/base64"
	"strings"
)

// extractJSON 从模型输出文本中提取第一组大括号包裹的 JSON 片段。
func extractJSON(s string) []byte {
	clean := strings.TrimSpace(s)
	start := strings.IndexByte(clean, '{')
	end := strings.LastIndexByte(clean, '}')
	if start < 0 || end <= start {
		return []byte(s)
	}
	return []byte(clean[start : end+1])
}

// base64Encode 将字节数据编码为标准 base64 字符串（用于内嵌图片）。
func base64Encode(b []byte) string {
	return base64.StdEncoding.EncodeToString(b)
}
