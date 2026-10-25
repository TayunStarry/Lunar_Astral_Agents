// Package agents 收纳琉璃（Crystal）的全部内部智能体，Go 与 JS 实现同处一层：
//
//	agents/desktop/        桌面操纵智能体（Go：UIA / 输入 / 进程 / 窗口）
//	agents/WebSearch/         网络搜索智能体（Go：引擎 / 过滤 / 文本摘要 / 报告 / 缓存）
//	agents/Descriptor/     描述者（Go：句柄系统的影像描述与文件内容探查）
//	agents/BilibiliViewer/   观影者智能体（Go：真实浏览器刷视频 / 评论 / 日记）
//	agents/QQMessenger/      QQ 投递智能体（Go：向群/好友发送文本、图片、文件）
//	agents/*.js            页面注入型智能体（JS：Mini-LTP / Self-LTP / SharedInput）
//
// 之所以把 JS 脚本从"前端静态资源"里搬出来：它们不是前端页面的一部分，而是
// 由三个 handler 单独下发、注入到 iframe 里运行的智能体本体。放在前端目录里时，
// 资源在 web/、而读取方写的是 "assets/xxx"，编译器与 tsc 都不报错，只有真跑
// 一遍 handler 才暴露 500（见 crystal/asset_serve_test.go 覆盖的历史缺陷）。
// 现在"脚本"与"内嵌脚本的代码"同处 agents/ 下，前缀错配在结构上不再可能发生。
package agents

import (
	"embed"
	"fmt"
	"strings"
)

// scripts 内嵌的页面注入型智能体脚本。
//
// 模式 `*.js` 只匹配本目录下的脚本，不会把 desktop/、WebSearch/ 子目录卷进来。
//
//go:embed *.js
var scripts embed.FS

// ReadScript 读取内嵌的智能体脚本。
//
// name 为 agents/ 下的文件名（如 "mini_ltp_agent.js"），也接受带 "agents/" 前缀的
// 写法，避免调用方再猜一次前缀。任何取不到的名字都返回错误而非空内容——空内容
// 会让前端拿到 200 空脚本、注入静默失效，比直接 500 难排查得多。
func ReadScript(name string) ([]byte, error) {
	if name == "" {
		return nil, fmt.Errorf("智能体脚本名称为空")
	}
	base := strings.TrimPrefix(name, "agents/")
	data, err := scripts.ReadFile(base)
	if err != nil {
		return nil, fmt.Errorf("读取内嵌智能体脚本 %s 失败: %w", base, err)
	}
	if len(data) == 0 {
		return nil, fmt.Errorf("内嵌智能体脚本 %s 为空", base)
	}
	return data, nil
}
