// Package agents 收纳月华与琉璃两个人格局及其内部智能体，Go 实现集中在本目录：
//
//	agents/Screenshoter/  截图智能体（Go：显示器/窗口枚举、区域截图、多帧推送与主帧回传）
//	agents/desktop/        桌面操纵智能体（Go：UIA / 输入 / 进程 / 窗口）
//	agents/SearchPlanner/    搜索者-规划智能体（Go：结构化管线决策）
//	agents/SearchSummarizer/ 搜索者-摘要智能体（Go：多页正文一次提炼）
//	executors/WebCrawler/   网页抓取执行体（Go：引擎降级 / 详情页采集 / 缓存；不发起模型调用）
//	agents/Vision/           视觉理解智能体（Go：图片 / 动图 / 视频的视觉副本、观看与描述）
//	agents/DocumentReader/   文献解析智能体（Go：格式识别、按行 / 分批解析超大规模文本）
//	executors/ArchiveReader/ 归档解析执行体（Go：压缩包结构 / 条目读取 / GGUF 元数据；不发起模型调用）
//	agents/ArchiveOps/       归档写操作工具库（Go：压缩包创建 / 删除 / 条目新增 / 移除；不发起模型调用）
//	agents/DialogueBridging/ 通讯桥接智能体（Go：向群/好友发送文本、图片、文件）
//	agents/MutualForum/      互助论坛智能体（Go：论坛浏览/发帖/回复的多轮工具执行）
//	agents/FileEditor/       文档库读写工具库（Go：/document 读写与项目运行；不发起模型调用）
//	agents/WishManager/      愿望管理智能体（Go：愿望新增 / 改写 / 完成 / 删除 / 优先级 / 查询）
//	agents/SQLWriter/        语句生成智能体（Go：自然语言 → 一条只读 SQL，不执行）
//
// 跨多轮 / 跨多智能体的系统级链路编排不在本目录，统一收纳于 chain/：
//
//	chain/thinking/ 思维链（对话者 Dialogue + 单 Tick 循环与特许响应车道）
//	chain/search/   搜索链（规划 → 抓取 → 摘要三段调度）
//	chain/life/     生活链路（生活事件编排）
//	chain/viewer/   观影链路（哔哩观影编排）
//
// 页面注入型智能体脚本（Mini-LTP / Self-LTP / SharedInput）与下发它们的 LTPX 宿主
// 同处 server/ltpxhost（脚本在 server/ltpxhost/scripts，经 go:embed 内嵌）：脚本不是
// 前端页面的一部分，而是由三个 handler 单独下发、注入到 iframe 里运行的智能体本体，
// 因此与宿主放在一起，前缀错配在结构上不再可能发生。
package agents
