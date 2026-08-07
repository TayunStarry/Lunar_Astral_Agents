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

// memoryAddImageRequest 添加图片文档请求（image 类型集合专用）
type memoryAddImageRequest struct {
	Image          string `json:"image"`            // 图片 base64 编码数据
	EmotionDesc    string `json:"emotion_desc"`     // 情绪描述
	ColorStyleDesc string `json:"color_style_desc"` // 色彩风格描述
	ContentDesc    string `json:"content_desc"`     // 主要内容描述
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
	ModelName      string `json:"model_name"`
	CollectionType string `json:"collection_type,omitempty"` // 集合类型："text" 或 "image"，空值默认为 text
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

// memoryCollectionInfo 集合信息（含模型、维度与类型）
type memoryCollectionInfo struct {
	Name      string `json:"name"`
	Model     string `json:"model"`
	Dimension int    `json:"dimension"`
	Count     int    `json:"count"`
	Type      string `json:"type,omitempty"` // 集合类型："text" 或 "image"
}

// memoryImageQueryData 图片查询结果数据
type memoryImageQueryData struct {
	Query      string                   `json:"query"`
	TopK       int                      `json:"top_k"`
	Results    []memoryImageQueryResult `json:"results"`
	TotalFound int                      `json:"total_found"`
}

// memoryImageQueryResult 单条图片查询结果
type memoryImageQueryResult struct {
	ID         string  `json:"id"`          // 文档 ID
	Image      string  `json:"image"`       // 图片 base64 编码数据
	BaseScore  float32 `json:"base_score"`  // 基础评分（三个向量相似度平均值）
	FinalScore float32 `json:"final_score"` // 最终评分（tok5 加权后）
	BoostLevel int     `json:"boost_level"` // 加权等级：0/1/2/3
}
