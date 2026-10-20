package httpapi

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

// resourceAdmitDocumentRequest 文档入库/刷新请求
// 以「documents/原文件名」为业务身份，同一路径复用同一句柄
type resourceAdmitDocumentRequest struct {
	Path string `json:"path"` // 相对 LocalDir 的路径，如 documents/foo.md
	Name string `json:"name"` // 原始文件名
}

// memoryAddRequest 记忆添加请求（文本 / 图片 / 音频共用，type 指定类型）
type memoryAddRequest struct {
	Type                   string   `json:"type,omitempty"`                    // 记忆类型：text / image / audio（空=按字段推断）
	Role                   string   `json:"role,omitempty"`                    // 消息角色，文本记忆使用
	Content                string   `json:"content,omitempty"`                 // 文本内容，文本记忆使用
	Base64                 string   `json:"base64,omitempty"`                  // 图片/音频 base64 数据
	Description            string   `json:"description,omitempty"`             // 音频记忆描述（用于嵌入与归组）
	Tags                   []string `json:"tags,omitempty"`                    // 文本记忆显式标签（以标签文本嵌入）
	Source                 string   `json:"source,omitempty"`                  // 写入来源标记（如 action）
	RecognitionOrientation string   `json:"recognition_orientation,omitempty"` // 图片识别取向标识（auto/emotion/text/color/appearance/species/posture/custom）
	RecognitionCustom      string   `json:"recognition_custom,omitempty"`      // 自定义识别取向参考文本（仅 custom 使用）
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
// 集合是内容自动归入的分组；创建时按集合名调用嵌入模型生成名称向量
type memoryCollectionRequest struct {
	Name string `json:"name"`
}

// memoryMessageData 记忆条目（文本/图片/音频统一，客户端依据 type 取用对应内容）
type memoryMessageData struct {
	ID         string  `json:"id"`                   // 消息 ID
	Type       string  `json:"type"`                 // 记忆类型：text / image / audio
	Role       string  `json:"role"`                 // 消息角色，图片记忆为 "image"
	Content    string  `json:"content"`              // 文本内容，文本记忆使用；音频记忆为描述
	URL        string  `json:"url,omitempty"`        // 媒体读取地址（multimedia|audios/<集合名> 下的文件）
	Base64     string  `json:"base64,omitempty"`     // 旧版图片内联数据（仅历史数据兼容）
	Collection string  `json:"collection,omitempty"` // 内容所属分组集合名
	Source     string  `json:"source,omitempty"`     // 写入来源标记
	Similarity float32 `json:"similarity"`           // 内容向量余弦相似度 [0, 1]
	Timestamp  int64   `json:"timestamp,omitempty"`  // 入库时间 Unix 秒级时间戳，旧数据无此字段
}

type memoryStatsData struct {
	DocumentCount int  `json:"document_count"`
	Initialized   bool `json:"initialized"`
	EntryCount    int  `json:"entry_count"`
	SyncMismatch  bool `json:"sync_mismatch"` // 维度不符文档存在标记
	TextCount     int  `json:"text_count"`
	ImageCount    int  `json:"image_count"`
	AudioCount    int  `json:"audio_count"`
}

type memoryQueryData struct {
	Query      string              `json:"query"`
	Type       string              `json:"type"`
	TopK       int                 `json:"top_k"`
	Results    []memoryMessageData `json:"results"`
	TotalFound int                 `json:"total_found"`
}

// memoryCollectionInfo 集合信息（分组）
type memoryCollectionInfo struct {
	Name                   string `json:"name"`
	EmbeddingModel         string `json:"embedding_model"`
	Dimension              int    `json:"dimension"`
	Count                  int    `json:"count"`
	Version                int    `json:"version"`
	HasNameEmbedding       bool   `json:"has_name_embedding"`
	NameEmbeddingDimension int    `json:"name_embedding_dimension"`
	NameEmbeddingModel     string `json:"name_embedding_model,omitempty"`
}
