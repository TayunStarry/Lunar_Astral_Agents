// package runtime 实现月华系统的私有 JavaScript 运行时：
// 每次运行在独立 goja 沙箱内执行脚本，能力以顶层全局（console/file/memory/http/…）注入。
//
// 该模块不再是独立的「插件引擎」：不再扫描扩展包目录、不再按 LTP9 标签识别与载入插件，
// 也不再有任何权限限制 / 加密凭证（permissions.key）机制。运行实例只能由月华系统内部
// 显式创建（当前唯一调用方是「月华-生活系统-创作」及其文件编辑智能体）。
package runtime

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"LunarSubsystem/LunarHarness/engine/config"
	"LunarSubsystem/LunarHarness/engine/console/logger"
)

// ==== 私有运行时注册表 ====

var (
	registryMu sync.RWMutex
	// runtimes 正在运行的私有运行时（id → 实例），供同一进程内的跨运行时调用/指令分发。
	runtimes = map[string]*plugin{}
)

// Version 返回运行时构建标记（供调用方自证版本）。
func Version() string { return EngineBuild }

// Run 在私有沙箱中执行一段 JavaScript，返回可继续调用的运行时句柄。
// id 用于日志与跨运行时标识；为空时自动生成。
func Run(id, title, code string) (*Runtime, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		id = fmt.Sprintf("js-%d", nowNano())
	}
	if strings.TrimSpace(code) == "" {
		return nil, fmt.Errorf("脚本内容为空")
	}

	// 同名实例先卸载，避免同一 id 残留两套事件循环。
	CloseRuntime(id)

	p := newPrivate(id, title)
	if err := p.loadSource(code); err != nil {
		p.unload()
		return nil, err
	}

	registryMu.Lock()
	runtimes[id] = p
	registryMu.Unlock()
	return &Runtime{plugin: p}, nil
}

// CloseRuntime 按 id 关闭并卸载运行时。
func CloseRuntime(id string) {
	registryMu.Lock()
	p := runtimes[id]
	delete(runtimes, id)
	registryMu.Unlock()
	if p != nil {
		p.unload()
	}
}

// Close 关闭全部私有运行时（进程退出前调用）。
func Close() {
	registryMu.Lock()
	all := make([]*plugin, 0, len(runtimes))
	for id, p := range runtimes {
		all = append(all, p)
		delete(runtimes, id)
	}
	registryMu.Unlock()
	for _, p := range all {
		p.unload()
	}
	logger.Info(ServiceName, "私有 js 运行时已关闭")
}

// Lookup 按 id 取运行时句柄（未找到时 ok=false）。
func Lookup(id string) (*Runtime, bool) {
	p := runtimeFor(id)
	if p == nil {
		return nil, false
	}
	return &Runtime{plugin: p}, true
}

// runtimeFor 按 id 取运行时（未找到返回 nil）。
func runtimeFor(id string) *plugin {
	registryMu.RLock()
	defer registryMu.RUnlock()
	return runtimes[id]
}

// snapshotRuntimes 返回运行时副本（供指令/事件分发）。
func snapshotRuntimes() []*plugin {
	registryMu.RLock()
	defer registryMu.RUnlock()
	out := make([]*plugin, 0, len(runtimes))
	for _, p := range runtimes {
		out = append(out, p)
	}
	return out
}

// ==== 运行时句柄 ====

// Runtime 一个已加载的私有 JavaScript 运行时。
type Runtime struct {
	plugin *plugin
}

// Call 调用运行时导出的函数（exportFunction 注册）。
func (r *Runtime) Call(fnName string, args ...any) (any, error) {
	if r == nil || r.plugin == nil {
		return nil, fmt.Errorf("运行时已关闭")
	}
	return r.plugin.callExport(fnName, args)
}

// Exports 返回已导出的函数名（排序后）。
func (r *Runtime) Exports() []string {
	if r == nil || r.plugin == nil {
		return nil
	}
	r.plugin.mu.Lock()
	defer r.plugin.mu.Unlock()
	out := make([]string, 0, len(r.plugin.exports))
	for name := range r.plugin.exports {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// Loaded 运行时是否已成功加载。
func (r *Runtime) Loaded() bool {
	if r == nil || r.plugin == nil {
		return false
	}
	return r.plugin.loaded
}

// Error 加载失败原因（成功时为空串）。
func (r *Runtime) Error() string {
	if r == nil || r.plugin == nil {
		return "运行时已关闭"
	}
	return r.plugin.loadErr
}

// Close 卸载当前运行时。
func (r *Runtime) Close() {
	if r == nil || r.plugin == nil {
		return
	}
	CloseRuntime(r.plugin.ID)
}

// privateDataDir 私有运行时的数据目录（file.* 写入隔离区）。
func privateDataDir(id string) string {
	execDir := "."
	if exe, err := os.Executable(); err == nil {
		execDir = filepath.Dir(exe)
	}
	safe := strings.Map(func(r rune) rune {
		switch r {
		case '/', '\\', ':', '*', '?', '"', '<', '>', '|':
			return '_'
		}
		return r
	}, id)
	return filepath.Join(execDir, config.LocalDir, "runtime", safe, "data")
}
