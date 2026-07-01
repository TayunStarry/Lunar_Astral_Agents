package config

import "flag"

var (
	LocalDir = flag.String("local-dir", "local_data", "本地目录路径，用于存储资源文件")
	// CertFile  证书文件路径，用于HTTPS加密通信
	CertFile = flag.String("cert-file", *LocalDir+"/certs/localhost.pem", "证书文件路径, 用于HTTPS加密通信")
	// KeyFile  私钥文件路径，用于HTTPS加密通信
	KeyFile = flag.String("key-file", *LocalDir+"/certs/localhost-key.pem", "私钥文件路径, 用于HTTPS加密通信")
	// KnowledgeDBPath  知识库文件路径，统一数据库存储
	KnowledgeDBPath = flag.String("knowledge-db", *LocalDir+"/database/knowledge.db", "知识库文件路径")
	// MemoryDBDir  记忆库文件夹路径，统一数据库存储
	MemoryDBDir = flag.String("memory-db", *LocalDir+"/database/memory", "记忆库文件夹路径")
)
