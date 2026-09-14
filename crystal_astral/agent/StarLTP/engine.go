package StarLTP

// ==== LTP9 引擎管理器：包扫描、插件加载/卸载、公开查询 ====

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"LunarSubsystem/LoggerGeneral"
)

// newEngine 构造引擎管理器。
func newEngine() *engine {
	return &engine{plugins: map[string]*plugin{}}
}

// root scan a single package directory for LTP9 metadata.
type pkgMeta struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Tags  []string `json:"tags"`
}

// readMeta 读取包 metadata.json，判断是否为 LTP9 插件。
func readMeta(root string) (pkgMeta, bool) {
	var m pkgMeta
	raw, err := os.ReadFile(root + "/metadata.json")
	if err != nil {
		return m, false
	}
	if json.Unmarshal(raw, &m) != nil {
		return m, false
	}
	for _, t := range m.Tags {
		if strings.EqualFold(t, LTP9Tag) {
			return m, true
		}
	}
	return m, false
}

// LoadAll 扫描包根目录并加载全部 LTP9 插件（启动时调用，幂等），并启动周期对账实现热加载/卸载。
func (e *engine) LoadAll() {
	e.root = packageRoot()
	e.mu.Lock()
	if e.running {
		e.mu.Unlock()
		return
	}
	e.running = true
	e.reconcileStop = make(chan struct{})
	e.mu.Unlock()
	e.reconcile()
	go e.reconcileLoop()
}

// reconcileLoop 周期对账：包目录指纹（目录名 + metadata/脚本/密钥/配置的 mtime 与大小）
// 发生变化时重新扫描，实现插件热加载/卸载与失败重载（与 LTP3 YaraLTP 的对账机制对齐）。
func (e *engine) reconcileLoop() {
	e.mu.RLock()
	stop := e.reconcileStop
	e.mu.RUnlock()
	ticker := time.NewTicker(reconcileInterval)
	defer ticker.Stop()
	last := e.fingerprint()
	for {
		select {
		case <-ticker.C:
			cur := e.fingerprint()
			if cur == last {
				continue
			}
			LoggerGeneral.Info(ServiceName, "检测到 LTP9 包目录变化，执行对账")
			e.reconcile()
			last = e.fingerprint()
		case <-stop:
			return
		}
	}
}

// fingerprint 计算包目录指纹；读取失败返回空串（下次与有效指纹比对必不相等，触发对账）。
func (e *engine) fingerprint() string {
	entries, err := os.ReadDir(e.root)
	if err != nil {
		return ""
	}
	names := make([]string, 0, len(entries))
	for _, ent := range entries {
		if ent.IsDir() {
			names = append(names, ent.Name())
		}
	}
	sort.Strings(names)
	var b strings.Builder
	for _, dir := range names {
		b.WriteString(dir)
		b.WriteByte('|')
		for _, f := range []string{"metadata.json", DefaultMain, KeyFileName, DefaultConfigFile} {
			if info, ierr := os.Stat(filepath.Join(e.root, dir, f)); ierr == nil {
				fmt.Fprintf(&b, "%s=%d:%d,", f, info.ModTime().UnixNano(), info.Size())
			}
		}
		b.WriteByte(';')
	}
	return b.String()
}

// reconcile 对账：扫描包目录，新增/移除插件。
func (e *engine) reconcile() {
	entries, err := os.ReadDir(e.root)
	if err != nil {
		return
	}
	seen := map[string]string{} // id → dirName
	names := []string{}
	for _, ent := range entries {
		if !ent.IsDir() {
			continue
		}
		names = append(names, ent.Name())
	}
	sort.Strings(names)
	for _, dir := range names {
		root := e.root + string(os.PathSeparator) + dir
		m, is := readMeta(root)
		if !is {
			continue
		}
		id := m.ID
		if id == "" {
			id = dir
		}
		seen[id] = dir
		e.mu.RLock()
		existing := e.plugins[id]
		e.mu.RUnlock()
		if existing != nil {
			// 已存在的插件：此前加载失败的随目录变化重试（重建实例，避免向旧沙箱重复注册）
			if !existing.loaded && existing.loadErr != "" {
				if lerr := e.reloadPackage(id); lerr != nil {
					LoggerGeneral.Warn(ServiceName, "LTP9 插件重试加载失败 %s: %v", id, lerr)
				}
			}
			continue
		}
		p := newPlugin(dir, root, id, m.Title)
		e.mu.Lock()
		e.plugins[id] = p
		e.mu.Unlock()
		if err := p.load(); err != nil {
			LoggerGeneral.Warn(ServiceName, "LTP9 插件加载失败 %s: %v", id, err)
		}
	}
	// 卸载磁盘上已不存在的插件
	e.mu.RLock()
	var rm []string
	for id, p := range e.plugins {
		if seen[id] != p.DirName {
			rm = append(rm, id)
		}
	}
	e.mu.RUnlock()
	for _, id := range rm {
		e.unloadPackage(id)
	}
}

// unloadPackage 卸载插件并释放其事件循环。
func (e *engine) unloadPackage(id string) {
	e.mu.Lock()
	p := e.plugins[id]
	if p == nil {
		e.mu.Unlock()
		return
	}
	delete(e.plugins, id)
	e.mu.Unlock()
	p.unload()
}

// reloadPackage 重载插件。
func (e *engine) reloadPackage(id string) error {
	e.mu.Lock()
	p := e.plugins[id]
	if p == nil {
		e.mu.Unlock()
		return nil
	}
	dir, root, title := p.DirName, p.Root, p.Title
	delete(e.plugins, id)
	e.mu.Unlock()
	p.unload()
	np := newPlugin(dir, root, id, title)
	e.mu.Lock()
	e.plugins[id] = np
	e.mu.Unlock()
	return np.load()
}

// pluginStates 返回全部插件状态（含已注册事件主题与导出函数，供前端下拉填充）。
func (e *engine) pluginStates() []PluginState {
	e.mu.RLock()
	defer e.mu.RUnlock()
	out := make([]PluginState, 0, len(e.plugins))
	for _, p := range e.plugins {
		st := PluginState{ID: p.ID, Title: p.Title, Loaded: p.loaded, Error: p.loadErr}
		if len(p.granted) > 0 {
			for n := range p.granted {
				st.Granted = append(st.Granted, n)
			}
		}
		// 事件/导出为运行时可变数据，读取时加锁；仅列出当前非空的订阅主题与已导出函数
		p.mu.Lock()
		for t, subs := range p.events {
			if len(subs) > 0 {
				st.Events = append(st.Events, t)
			}
		}
		for n := range p.exports {
			st.Exports = append(st.Exports, n)
		}
		for n := range p.tools {
			st.Tools = append(st.Tools, n)
		}
		p.mu.Unlock()
		sort.Strings(st.Events)
		sort.Strings(st.Exports)
		sort.Strings(st.Tools)
		out = append(out, st)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// pluginFor 按 ID 取插件（已锁外部使用）。
func (e *engine) pluginFor(id string) *plugin {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.plugins[id]
}

// snapshotPlugins 返回全部插件副本（用于分发）。
func (e *engine) snapshotPlugins() []*plugin {
	e.mu.RLock()
	defer e.mu.RUnlock()
	out := make([]*plugin, 0, len(e.plugins))
	for _, p := range e.plugins {
		out = append(out, p)
	}
	return out
}

// setOutbound 注入插件单向通报的接收函数（由外部 Go 程序提供）。
func (e *engine) setOutbound(fn func(topic string, payload any)) {
	e.mu.Lock()
	e.outbound = fn
	e.mu.Unlock()
}

// shutdown 停止对账循环并卸载全部插件。
func (e *engine) shutdown() {
	e.mu.Lock()
	if e.reconcileStop != nil {
		close(e.reconcileStop)
		e.reconcileStop = nil
	}
	e.mu.Unlock()
	e.mu.RLock()
	ids := make([]string, 0, len(e.plugins))
	for id := range e.plugins {
		ids = append(ids, id)
	}
	e.mu.RUnlock()
	for _, id := range ids {
		e.unloadPackage(id)
	}
	LoggerGeneral.Info(ServiceName, "LTP9 引擎已关闭")
}