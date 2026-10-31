package config

// ==== 文献解析智能体（lunar_config.json 的 document_reader 节） ====
//
// 仅用于「大文件轻量扫描通道」：主模型产出扫描计划，轻量模型逐片摘要，
// 主模型归并并按需回读原文。指针字段区分「未配置」与「显式关闭」。

var (
	// DocumentScanModel 轻量扫描使用的模型名（默认 light_reasoning）
	DocumentScanModel = "light_reasoning"
	// DocumentScanSwap 是否在扫描前卸载主模型并加载轻量模型（nil=默认开启）
	DocumentScanSwap *bool
	// DocumentScanThreshold 启用轻量扫描的字符阈值（低于此值仍走原直读流程）
	DocumentScanThreshold = 50000
	// DocumentScanMaxChunks 单次扫描最多处理的片数（超出按相关度抽样）
	DocumentScanMaxChunks = 40
	// DocumentScanChunkRunes 扫描单片字符上限（受轻量模型上下文约束）
	DocumentScanChunkRunes = 10000
)
