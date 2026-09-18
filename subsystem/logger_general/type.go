package LoggerGeneral

// LogRecord 单条日志记录（日志文本已完成色彩参数处理，转为 Markdown 风格的加粗/色彩标注）
type LogRecord struct {
	Level  string `json:"level"`  // 日志级别：INFO / WARN / ERROR / FATAL
	Module string `json:"module"` // 模块名
	Sub    string `json:"sub"`    // 子模块名（无子模块时固定为占位 not）
	Text   string `json:"text"`   // Markdown 风格渲染文本（HTML 色彩标注 + 加粗）
}
