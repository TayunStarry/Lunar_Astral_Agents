package imagecache

// ImageEntry 图片缓存条目：一张图片在宿主侧的注册信息与元数据
//
// 缓存是图片数据在宿主侧的权威索引，QQ 合并转发（Go 侧）与对话者上下文
// （goja 侧经适配器）共用同一命名空间：模型上下文中只出现 [图片.00x] 标签，
// 真实图片按需经 Source 交给图片处理管线取回
type ImageEntry struct {
	// Seq 注册序号（全局单调递增，用于排序）
	Seq int64
	// Tag 图片标签（形如 图片.003，上下文中以 [图片.003] 占位出现）
	Tag string
	// Source 图片来源地址（http(s) URL / data URI / 本地路径），取回时交给 resizeImage / upscaleImage 等管线
	Source string
	// Desc 内容描述（降级时由视觉模型生成一次，供标签清单展示；可为空）
	Desc string
	// Pinned 是否钉住（多模态窗口内的图片钉住防 FIFO 淘汰，降级后解除）
	Pinned bool
	// Timestamp 注册 Unix 秒级时间戳
	Timestamp int64
}
