package ltpxhost

import (
	"embed"
	"fmt"
	"strings"
)

// scripts 内嵌的页面注入型智能体脚本。
//
// 脚本随宿主一并迁移到本包：/mini-ltp-agent.js、/self-ltp-agent.js、/shared-input.js
// 三个 handler 与注入逻辑都在这里，源码与下发方同包，前缀错配在结构上不再可能发生。
//
//go:embed scripts/*.js
var scripts embed.FS

// ReadScript 读取内嵌的智能体脚本。
//
// name 为脚本文件名（如 "mini_ltp_agent.js"），也接受带 "agents/" 前缀的历史写法，
// 避免迁移前的调用方再猜一次前缀。任何取不到的名字都返回错误而非空内容——空内容
// 会让前端拿到 200 空脚本、注入静默失效，比直接 500 难排查得多。
func ReadScript(name string) ([]byte, error) {
	if name == "" {
		return nil, fmt.Errorf("智能体脚本名称为空")
	}
	base := strings.TrimPrefix(name, "agents/")
	data, err := scripts.ReadFile("scripts/" + base)
	if err != nil {
		return nil, fmt.Errorf("读取内嵌智能体脚本 %s 失败: %w", base, err)
	}
	if len(data) == 0 {
		return nil, fmt.Errorf("内嵌智能体脚本 %s 为空", base)
	}
	return data, nil
}
