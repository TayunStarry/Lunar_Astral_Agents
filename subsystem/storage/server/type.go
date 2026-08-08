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

// memoryAddRequest v2 统一添加请求（text 和 image 共用）
// 若 Image 字段非空，则视为图片文档；否则为文本文档
type memoryAddRequest struct {
	Role    string `json:"role,omitempty"`    // 消息角色，text 文档使用
	Content string `json:"content,omitempty"` // 文本内容，text 文档使用
	Image   string `json:"image,omitempty"`   // 图片 base64 数据，image 文档使用
}

type memoryDeleteRequest struct {
	ID string `json:"id"`
}

// memoryInitRequest v2 实例初始化请求（嵌入服务 + LLM 标签生成服务）
type memoryInitRequest struct {
	BaseURL         string `json:"base_url"`          // 嵌入服务 base_url
	APIKey          string `json:"api_key"`           // 嵌入服务 API Key
	LLMBaseURL      string `json:"llm_base_url"`      // LLM 标签生成服务 base_url
	LLMAPIKey       string `json:"llm_api_key"`       // LLM 标签生成服务 API Key
	MultimodalModel string `json:"multimodal_model"`  // 多模态模型名
}

// memoryCollectionRequest 创建/打开集合请求（集合级锁定模型）
type memoryCollectionRequest struct {
	ModelName      string `json:"model_name"`
	CollectionType string `json:"collection_type,omitempty"` // 集合类型："text" 或 "image"
}

// memoryMessageData v2 查询结果单条消息（text 和 image 统一）
type memoryMessageData struct {
	ID         string  `json:"id"`                   // 消息 ID
	Role       string  `json:"role"`                 // 消息角色，image 文档为 "image"
	Content    string  `json:"content"`              // 消息内容，image 文档为空
	Image      string  `json:"image,omitempty"`      // 图片 base64 数据，仅 image 文档
	Similarity float32 `json:"similarity"`           // 标签匹配频次得分 [0, 1]
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
	Name              string `json:"name"`
	EmbeddingModel    string `json:"embedding_model"`
	Dimension         int    `json:"dimension"`
	Count             int    `json:"count"`
	Type              string `json:"type"`
	MultimodalModel   string `json:"multimodal_model,omitempty"`
	Version           int    `json:"version"`
	TagCount          int    `json:"tag_count"`
}
