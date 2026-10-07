package main

// fileRecord 枚举到的目标文件记录
type fileRecord struct {
	// Path 文件绝对路径
	Path string
	// Ext 小写扩展名（含点）
	Ext string
	// Size 文件字节数
	Size int64
}

// warehouseStats 处理统计
type warehouseStats struct {
	// Scanned 枚举到的目标文件总数
	Scanned int64
	// Moved 首次入库（内容移入 source_data 并在原位置留硬链接）的文件数
	Moved int64
	// Linked 以硬链接替换的重复文件数
	Linked int64
	// Skipped 已是正确硬链接而跳过的文件数
	Skipped int64
	// Failed 处理失败的文件数
	Failed int64
	// SavedBytes 去重节省的重复内容字节数
	SavedBytes int64
}
