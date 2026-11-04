package toolreg

// toolreg 工具注册表：把「工具声明」放在智能体包，「工具实现」留在装配层。
//
// 分层约定：
//   - 智能体包在 init() 中调用 Register / RegisterProvider 声明工具（schema + 说明 + 所属智能体 ID）；
//   - 各链与 app 在装配时调用 RegisterExecutor 注入执行函数；
//   - 链只通过 LookupSchema / LookupExecutor 查表，不反向依赖智能体之外的实现细节。
//
// 本包只做登记与查找，不执行任何模型调度或业务逻辑。

import (
	"context"
	"sort"
	"sync"
)

// Schema 一个工具的声明：名称、说明与 JSON Schema 参数。
type Schema struct {
	Name        string
	Description string
	Parameters  map[string]any
}

// Executor 工具执行函数：输入解析后的参数，返回工具输出原语。
type Executor func(ctx context.Context, args map[string]any) (any, error)

var (
	mu        sync.RWMutex
	staticOf  = map[string][]Schema{}
	providers = map[string][]func() []Schema{}
	executors = map[string]Executor{}
)

// Register 登记某个智能体（owner）的静态工具声明。
func Register(owner string, schemas ...Schema) {
	mu.Lock()
	defer mu.Unlock()
	staticOf[owner] = append(staticOf[owner], schemas...)
}

// RegisterProvider 登记动态工具声明：每次查询都会重新调用 provider。
// 适用于枚举值依赖运行时状态的工具（例如通讯桥接就绪后才出现的子工具）。
func RegisterProvider(owner string, provider func() []Schema) {
	mu.Lock()
	defer mu.Unlock()
	providers[owner] = append(providers[owner], provider)
}

// RegisterExecutor 由装配层注入工具执行函数；同名后写覆盖先写。
func RegisterExecutor(name string, exec Executor) {
	mu.Lock()
	defer mu.Unlock()
	executors[name] = exec
}

// Schemas 返回某个智能体的全部工具声明（静态 + 动态），按名称去重并排序。
func Schemas(owner string) []Schema {
	mu.RLock()
	static := append([]Schema(nil), staticOf[owner]...)
	dynamic := make([]func() []Schema, len(providers[owner]))
	copy(dynamic, providers[owner])
	mu.RUnlock()

	for _, provider := range dynamic {
		static = append(static, provider()...)
	}

	merged := make(map[string]Schema, len(static))
	for _, schema := range static {
		if schema.Name == "" {
			continue
		}
		merged[schema.Name] = schema
	}
	out := make([]Schema, 0, len(merged))
	for _, schema := range merged {
		out = append(out, schema)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// LookupSchema 按名称查找工具声明（静态优先，其次动态）。
func LookupSchema(name string) (Schema, bool) {
	mu.RLock()
	defer mu.RUnlock()
	for _, list := range staticOf {
		for _, schema := range list {
			if schema.Name == name {
				return schema, true
			}
		}
	}
	for _, list := range providers {
		for _, provider := range list {
			for _, schema := range provider() {
				if schema.Name == name {
					return schema, true
				}
			}
		}
	}
	return Schema{}, false
}

// LookupExecutor 按名称查找工具执行函数。
func LookupExecutor(name string) (Executor, bool) {
	mu.RLock()
	defer mu.RUnlock()
	exec, ok := executors[name]
	return exec, ok
}

// UnregisterExecutor 移除工具执行函数（仅供测试）。
func UnregisterExecutor(name string) {
	mu.Lock()
	defer mu.Unlock()
	delete(executors, name)
}

// Owners 返回已登记工具声明或执行器的全部智能体 ID。
func Owners() []string {
	mu.RLock()
	defer mu.RUnlock()
	seen := map[string]struct{}{}
	for owner := range staticOf {
		seen[owner] = struct{}{}
	}
	for owner := range providers {
		seen[owner] = struct{}{}
	}
	out := make([]string, 0, len(seen))
	for owner := range seen {
		out = append(out, owner)
	}
	sort.Strings(out)
	return out
}

// Reset 清空注册表（仅供测试）。
func Reset() {
	mu.Lock()
	defer mu.Unlock()
	staticOf = map[string][]Schema{}
	providers = map[string][]func() []Schema{}
	executors = map[string]Executor{}
}
