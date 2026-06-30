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

type memoryAddRequest struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type memoryDeleteRequest struct {
	ID string `json:"id"`
}

// memoryInitRequest 实例初始化请求（仅配置嵌入服务连接，不创建集合）
type memoryInitRequest struct {
	BaseURL string `json:"base_url"`
	APIKey  string `json:"api_key"`
}

// memoryCollectionRequest 创建/打开集合请求（集合级锁定模型）
type memoryCollectionRequest struct {
	ModelName string `json:"model_name"`
}

type memoryMessageData struct {
	ID         string  `json:"id"`         // 消息ID
	Role       string  `json:"role"`       // 消息角色，user/assistant/system
	Content    string  `json:"content"`    // 消息内容
	Similarity float32 `json:"similarity"` // 余弦相似度分数 [-1, 1]，越高越相关
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

// memoryCollectionInfo 集合信息（含模型与维度）
type memoryCollectionInfo struct {
	Name      string `json:"name"`
	Model     string `json:"model"`
	Dimension int    `json:"dimension"`
	Count     int    `json:"count"`
}
