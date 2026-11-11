package resourcehub

// type.go 统一资源聚合工具的类型定义。
//
// 定位、句柄登记与内容取回三件事从此只有一个入口：
//   - Query 描述一次查询（type 选择模式，query 承载句柄 / 路径 / 关键词）；
//   - Result 是模型可读文本 + 本次涉及的句柄数组；
//   - Hooks 把「解读能力」从 engine 剥离，由装配层接上具体智能体实现，
//     避免 engine 反向依赖 agents。

// Query 一次统一资源查询。
type Query struct {
	// Types 查询模式数组：memory / handle / path / list，可一次传入多个模式
	Types []string
	// Query 目标列表：memory 为关键词，handle 为句柄，path 为文件名或路径，list 为类型码 + 过滤词
	Query []string
	// Limit 返回数量上限（<=0 时按模式默认值）
	Limit int
}

// Result 统一查询输出。
type Result struct {
	// Text 模型可读的结果文本
	Text string
	// Handle 本次实际涉及的句柄数组（无涉及句柄时为空数组）
	Handle []string
	// Error 失败原因（无错误时为空串；部分失败写在 Text 里，不置 Error）
	Error string
}

// Hooks 资源解读能力注入点。
//
// 每个字段都允许为 nil：未注入时对应类型的资源只返回已缓存的解析文本或纯文本内容，
// 不会因为缺少智能体而阻塞其他模式。
type Hooks struct {
	// DescribeMedia 图片 / 动图的视觉描述
	DescribeMedia func(handle, ask string) string
	// DescribeVideo 本机视频地址的观看管线描述
	DescribeVideo func(rawURL, ask string) (string, error)
	// InspectText 文本句柄的定向探查（文献解析智能体）
	InspectText func(handle, ask string) string
	// ReadPage 指定网页的现场读取
	ReadPage func(rawURL, ask string) (title, summary string, err error)
	// BilibiliImpression B 站地址的直驱印象
	BilibiliImpression func(rawURL, ask string) (string, error)
	// IsArchiveExt 判断扩展名是否属于归档类（压缩包 / GGUF）
	IsArchiveExt func(ext string) bool
	// InspectArchive 归档实体的确定性解析
	InspectArchive func(blobPath, ask string) (string, error)
	// IsVideoExpired 判断视频描述错误是否为「远程地址已失效」
	IsVideoExpired func(err error) bool
}
