package config

var (
	// LocalDir 本地目录路径，用于存储资源文件
	LocalDir = "local_data"
	// CertFile 证书文件路径，用于 HTTPS 加密通信
	CertFile = LocalDir + "/cert/localhost.pem"
	// KeyFile 私钥文件路径，用于 HTTPS 加密通信
	KeyFile = LocalDir + "/cert/localhost-key.pem"
	// KnowledgeDBPath 通用知识库文件路径（用户 / 插件自建表；内建缓存在各自领域库）
	KnowledgeDBPath = LocalDir + "/database/knowledge.db"
	// ArchiveDBPath 统一句柄库文件路径（URL 主表 / 六类句柄 / 段落 / 缓存）
	ArchiveDBPath = LocalDir + "/database/handles.db"
	// MediaDBPath 历史媒体派生库路径（新体系不再使用，启动时会被清除）
	MediaDBPath = LocalDir + "/database/media.db"
	// WebDBPath 历史网络检索缓存路径（新体系不再使用，启动时会被清除）
	WebDBPath = LocalDir + "/database/web.db"
	// TextDBPath 文本内容索引（文件 → 分块集合映射）
	TextDBPath = LocalDir + "/database/text.db"
	// CharacterDBPath 自定义角色库（绘图角色档案）
	CharacterDBPath = LocalDir + "/database/character.db"
	// ArchiveQuotaMB 内容仓库容量上限（MB）：超限按「等级 → 最久未用」回收 blob
	ArchiveQuotaMB = 2048
	// ArchiveMaxBlobMB 单个资源入仓体积上限（MB）：超限拒绝入仓并回落为直读来源
	ArchiveMaxBlobMB = 20
	// ArchiveGCIntervalMin 资源仓库回收周期（分钟），0 表示关闭周期回收
	ArchiveGCIntervalMin = 30
	// MemoryDBDir 记忆库文件夹路径，统一数据库存储
	MemoryDBDir = LocalDir + "/database/memory"
)
