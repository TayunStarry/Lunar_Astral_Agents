// Package agents 收纳月华与琉璃两个人格局及其内部智能体，Go 与 JS 实现同处一层：
//
//	agents/Screenshoter/  截图智能体（Go：显示器/窗口枚举、区域截图、多帧推送与主帧回传）
//	agents/ltpxhost/       琉璃协议宿主（Go：LTPX 协调 / 模块创建 / WebView 会话 / 前端守卫）
//	agents/desktop/        桌面操纵智能体（Go：UIA / 输入 / 进程 / 窗口）
//	agents/SearchPlanner/    搜索者-规划智能体（Go：结构化管线决策）
//	agents/SearchSummarizer/ 搜索者-摘要智能体（Go：多页正文一次提炼）
//	chain/search/crawler/ 搜索抓取执行体（Go：引擎降级 / 详情页采集 / 缓存；不发起模型调用）
//	agents/Vision/           视觉理解智能体（Go：图片 / 动图 / 视频的视觉副本、观看与描述）
//	agents/DocumentReader/   文献解析智能体（Go：格式识别、按行 / 分批解析超大规模文本）
//	executors/ArchiveReader/ 归档解析执行体（Go：压缩包结构 / 条目读取 / GGUF 元数据；不发起模型调用）
//	agents/ArchiveOps/       归档操作智能体（Go：压缩包创建 / 删除 / 条目新增 / 移除）
//	agents/DialogueBridging/ 通讯桥接智能体（Go：向群/好友发送文本、图片、文件）
//	agents/MutualForum/      互助论坛智能体（Go：论坛浏览/发帖/回复的多轮工具执行）
//	agents/FileEditor/       文件编辑智能体（Go：/document 文档库读写与项目运行）
//	agents/PlanManager/      计划管理智能体（Go：计划表新增 / 删除 / 改写 / 查询）
//	agents/*.js            页面注入型智能体（JS：Mini-LTP / Self-LTP / SharedInput）
//
// 跨多轮 / 跨多智能体的系统级链路编排不在本目录，统一收纳于 chain/：
//
//	chain/thinking/ 思维链（对话者 Dialogue + 两组 Tick 循环）
//	chain/search/   搜索链（规划 → 抓取 → 摘要三段调度）
//	chain/life/     生活链路（生活事件编排）
//	chain/viewer/   观影链路（哔哩观影编排）
//	chain/creation/ 创作链路（自由创作编排）
//
// 之所以把 JS 脚本从"前端静态资源"里搬出来：它们不是前端页面的一部分，而是
// 由三个 handler 单独下发、注入到 iframe 里运行的智能体本体。放在前端目录里时，
// 资源在 web/、而读取方写的是 "assets/xxx"，编译器与 tsc 都不报错，只有真跑
// 一遍 handler 才暴露 500（见 agents/ltpxhost/asset_serve_test.go 覆盖的历史缺陷）。
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
