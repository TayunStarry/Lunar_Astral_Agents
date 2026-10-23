package config

import "flag"

var (
	LocalDir = flag.String("local-dir", "local_data", "本地目录路径，用于存储资源文件")
	// CertFile  证书文件路径，用于HTTPS加密通信
	CertFile = flag.String("cert-file", *LocalDir+"/cert/localhost.pem", "证书文件路径, 用于HTTPS加密通信")
	// KeyFile  私钥文件路径，用于HTTPS加密通信
	KeyFile = flag.String("key-file", *LocalDir+"/cert/localhost-key.pem", "私钥文件路径, 用于HTTPS加密通信")
	// KnowledgeDBPath  通用知识库文件路径（用户 / 插件自建表；内建缓存在各自领域库）
	KnowledgeDBPath = flag.String("knowledge-db", *LocalDir+"/database/knowledge.db", "知识库文件路径")
	// ArchiveDBPath  统一资源目录文件路径（句柄 / 元数据 / 段落结构，见 docs/adr/0001）
	ArchiveDBPath = flag.String("archive-db", *LocalDir+"/database/archive.db", "统一资源目录数据库路径")
	// MediaDBPath  媒体派生数据（动图/视频/音频理解摘要、视觉副本簿记）
	MediaDBPath = flag.String("media-db", *LocalDir+"/database/media.db", "媒体派生数据库路径")
	// WebDBPath  网络检索缓存（页面摘要缓存）
	WebDBPath = flag.String("web-db", *LocalDir+"/database/web.db", "网络检索缓存数据库路径")
	// TextDBPath  文本内容索引（文件 → 分块集合映射）
	TextDBPath = flag.String("text-db", *LocalDir+"/database/text.db", "文本内容索引数据库路径")
	// CharacterDBPath  自定义角色库（绘图角色档案）
	CharacterDBPath = flag.String("character-db", *LocalDir+"/database/character.db", "自定义角色库数据库路径")
	// ArchiveQuotaMB  内容仓库容量上限（MB）：超限按「等级 → 最久未用」回收 blob
	ArchiveQuotaMB = flag.Int("archive-quota-mb", 2048, "统一资源内容仓库容量上限(MB)")
	// ArchiveMaxBlobMB  单个资源入仓体积上限（MB）：超限拒绝入仓并回落为直读来源
	ArchiveMaxBlobMB = flag.Int("archive-max-blob-mb", 20, "单个资源入仓体积上限(MB)")
	// ArchiveGCIntervalMin  资源仓库回收周期（分钟），0 表示关闭周期回收
	ArchiveGCIntervalMin = flag.Int("archive-gc-interval-min", 30, "资源仓库回收周期(分钟)")
	// MemoryDBDir  记忆库文件夹路径，统一数据库存储
	MemoryDBDir = flag.String("memory-db", *LocalDir+"/database/memory", "记忆库文件夹路径")
)
