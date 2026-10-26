package agentdump

// type.go 数据导出提供者的公共类型。
//
// 每个 Go 智能体包在 init() 里注册一个 Provider，把自己运行期持有的上下文/状态
// 暴露给导出端点。这样控制台与 HTTP 端点都不需要认识具体智能体，也不会出现
// "新增智能体忘记接导出" 的分散实现。

// Provider 一个导出目标
type Provider struct {
	ID          string // 导出标识（agent 查询参数）
	Label       string // 显示名
	Group       string // 分组（控制台清单用）
	Description string // 说明
	Export      func(includeImages bool) (any, error)
}
