package thinking

// wire_dialogue.go 对话者工具执行器所需的宿主能力实现。
//
// 工具执行器归属 agents/Dialogue；这些能力只在链侧实现，因此由装配层（app）
// 在启动时显式写入 agents/Dialogue 的注入点，本包不在 init() 里改全局状态。
// 创作链能力（RunFileOperations）同样由 app 装配层注入，
// 以保持「thinking 不直接 import creation」。

// SaveImage 落盘图片并返回可读地址。
func SaveImage(name string, data []byte) (string, error) {
	if _, err := persistenceSaveBytes(name, data); err != nil {
		return "", err
	}
	return localPathToReadURL(name), nil
}
