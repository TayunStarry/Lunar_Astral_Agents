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

type chromemAddRequest struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chromemDeleteRequest struct {
	ID string `json:"id"`
}

type chromemInitRequest struct {
	BaseURL   string `json:"base_url"`
	APIKey    string `json:"api_key"`
	ModelName string `json:"model_name"`
}

type chromemMessageData struct {
	ID         string  `json:"id"`         // 消息ID
	Role       string  `json:"role"`       // 消息角色，user/assistant/system
	Content    string  `json:"content"`    // 消息内容
	Similarity float32 `json:"similarity"` // 余弦相似度分数 [-1, 1]，越高越相关
}

type chromemStatsData struct {
	DocumentCount int  `json:"document_count"`
	Initialized   bool `json:"initialized"`
	EntryCount    int  `json:"entry_count"`
	SyncMismatch  bool `json:"sync_mismatch"`
}

type chromemQueryData struct {
	Query      string               `json:"query"`
	TopK       int                  `json:"top_k"`
	Results    []chromemMessageData `json:"results"`
	TotalFound int                  `json:"total_found"`
}
