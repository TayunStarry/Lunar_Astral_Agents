// 媒体工具类型定义（GGUF 元数据解析 + 图片格式转换）
package module

// GGUFMetadataRequest GGUF 元数据请求
type GGUFMetadataRequest struct {
	FilePath string `json:"filePath"` // 文件路径（绝对路径或相对 LocalDir 的路径）
}

// GGUFMetadataResponse GGUF 元数据响应
type GGUFMetadataResponse struct {
	Success  bool              `json:"success"`
	Error    string            `json:"error,omitempty"`
	FileName string            `json:"filename,omitempty"`
	FilePath string            `json:"filePath,omitempty"`
	Summary  map[string]string `json:"summary,omitempty"`
	Metadata map[string]string `json:"metadata,omitempty"`
	Count    int               `json:"count,omitempty"`
}

// ConvertImageRequest 单张图片转换请求
type ConvertImageRequest struct {
	Path         string `json:"path"`          // 文件路径（绝对路径或相对 LocalDir 的路径）
	TargetFormat string `json:"target_format"` // 目标格式: png / jpeg / webp
	DeleteSource bool   `json:"delete_source"` // 是否删除源文件
	Quality      int    `json:"quality"`       // 编码质量（jpeg/webp），默认 90
}

// ConvertImageResponse 单张图片转换响应
type ConvertImageResponse struct {
	Success    bool   `json:"success"`
	OutputPath string `json:"output_path,omitempty"`
	Error      string `json:"error,omitempty"`
}

// BatchConvertRequest 批量转换请求
type BatchConvertRequest struct {
	Folder       string `json:"folder"`        // 文件夹路径（绝对路径或相对 LocalDir 的路径）
	SourceFormat string `json:"source_format"` // 源格式: all / png / jpeg / webp
	TargetFormat string `json:"target_format"` // 目标格式: png / jpeg / webp
	DeleteSource bool   `json:"delete_source"` // 是否删除源文件
	Quality      int    `json:"quality"`       // 编码质量（jpeg/webp），默认 90
}

// BatchConvertResult 批量转换单条结果
type BatchConvertResult struct {
	Path       string `json:"path"`
	Success    bool   `json:"success"`
	OutputPath string `json:"output_path,omitempty"`
	Error      string `json:"error,omitempty"`
}

// BatchConvertResponse 批量转换响应
type BatchConvertResponse struct {
	Success      bool                 `json:"success"`
	Results      []BatchConvertResult `json:"results"`
	Total        int                  `json:"total"`
	SuccessCount int                  `json:"success_count"`
	FailCount    int                  `json:"fail_count"`
	Error        string               `json:"error,omitempty"`
}

// ImageFileInfo 图片文件信息
type ImageFileInfo struct {
	Name   string `json:"name"`
	Path   string `json:"path"`
	Format string `json:"format"`
}

// ListImagesResponse 图片列表响应
type ListImagesResponse struct {
	Success bool            `json:"success"`
	Files   []ImageFileInfo `json:"files"`
	Folder  string          `json:"folder"`
	Error   string          `json:"error,omitempty"`
}
