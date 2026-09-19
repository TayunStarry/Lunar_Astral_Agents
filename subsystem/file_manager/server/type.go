package server

// PackageInstallResponse 包安装响应结构体
type PackageInstallResponse struct {
	Success      bool   `json:"success"`
	Message      string `json:"message"`
	PackageName  string `json:"package_name,omitempty"`
	PackageID    string `json:"package_id,omitempty"`
	PackageTitle string `json:"package_title,omitempty"`
}

// ExportPackageRequest 包导出请求结构体
type ExportPackageRequest struct {
	PackageName string `json:"package_name"`
	Action      string `json:"action"` // "download" 或 "save"
	SavePath    string `json:"save_path,omitempty"`
}

// memoryAddRequest v5 统一添加请求（文本和图片共用）
// 若 Base64 字段非空，则视为图片记忆；否则为文本记忆
type memoryAddRequest struct {
	Role                   string `json:"role,omitempty"`                    // 消息角色，文本记忆使用
	Content                string `json:"content,omitempty"`                 // 文本内容，文本记忆使用
	Base64                 string `json:"base64,omitempty"`                  // 图片 base64 数据，图片记忆使用
	RecognitionOrientation string `json:"recognition_orientation,omitempty"` // 图片识别取向标识（auto/emotion/text/color/appearance/species/posture/custom）
	RecognitionCustom      string `json:"recognition_custom,omitempty"`      // 自定义识别取向参考文本（仅 custom 使用）
}

type memoryDeleteRequest struct {
	ID string `json:"id"`
}

// memoryInitRequest v2 实例初始化请求
// 模型配置已迁移至 config 模块（lunar_config.json），此请求体不再需要模型参数
// 保留结构体以兼容前端可能发送的空请求体
type memoryInitRequest struct {
}

// memoryCollectionRequest 创建/打开集合请求
// v5: 文本与图片记忆混合存储，不再需要集合类型；模型名从 config 模块读取
type memoryCollectionRequest struct {
}

// memoryMessageData v5 查询结果单条记忆（文本和图片统一，客户端依据 content/base64 区分）
type memoryMessageData struct {
	ID         string  `json:"id"`                  // 消息 ID
	Role       string  `json:"role"`                // 消息角色，图片记忆为 "image"
	Content    string  `json:"content"`             // 文本内容，文本记忆使用；图片记忆为空
	Base64     string  `json:"base64,omitempty"`    // 图片 base64 数据，图片记忆使用；文本记忆为空
	Similarity float32 `json:"similarity"`          // 内容向量余弦相似度 [0, 1]
	Timestamp  int64   `json:"timestamp,omitempty"` // 入库时间 Unix 秒级时间戳，旧数据无此字段
}

type memoryStatsData struct {
	DocumentCount int  `json:"document_count"`
	Initialized   bool `json:"initialized"`
	EntryCount    int  `json:"entry_count"`
	SyncMismatch  bool `json:"sync_mismatch"` // 维度不符文档存在标记
}

type memoryQueryData struct {
	Query      string              `json:"query"`
	TopK       int                 `json:"top_k"`
	Results    []memoryMessageData `json:"results"`
	TotalFound int                 `json:"total_found"`
}

// memoryCollectionInfo v2 集合信息
type memoryCollectionInfo struct {
	Name            string `json:"name"`
	EmbeddingModel  string `json:"embedding_model"`
	Dimension       int    `json:"dimension"`
	Count           int    `json:"count"`
	Type            string `json:"type"`
	MultimodalModel string `json:"multimodal_model,omitempty"`
	Version         int    `json:"version"`
	TagCount        int    `json:"tag_count"`
}
