package YaraLTP

// ==== LTP3 引擎管理器：包扫描、虚拟机装载/卸载、事件与钩子分发 ====

import (
	"fmt"
	"os"
	"time"

	"LunarSubsystem/LoggerGeneral"
)

// newEngine 构造引擎管理器。
func newEngine() *engine {
	return &engine{
		plugins: map[string]*plugin{},
		byDir:   map[string]string{},
	}
}

// LoadAll 扫描包根目录并加载全部 LTP3 包（启动时调用）。
func (e *engine) LoadAll() {
	e.root = packageRoot()
	e.mu.Lock()
	e.running = true
	e.mu.Unlock()
	e.reconcile()
}

// fingerprint 计算磁盘上包目录集合的指纹（仅 LTP3，目录名 + metadata 变更时间戳）。
func (e *engine) fingerprint() string {
	entries, err := os.ReadDir(e.root)
	if err != nil {
		return ""
	}
	sig := ""
	idToDir := map[string]string{}
	for _, ent := range entries {
		if !ent.IsDir() {
			continue
		}
		dir := ent.Name()
		root := e.root + "/" + dir
		id, _, _, is, _ := readMeta(root)
		if !is {
			continue
		}
		sig += dir + "|"
		idToDir[id] = dir
	}
	// 结合已加载集合，避免同 id 换目录名导致漏算
	e.mu.RLock()
	for id, p := range e.plugins {
		if _, ok := idToDir[id]; !ok {
			sig += "!!" + id + "@" + p.DirName + "!!"
		}
	}
	e.mu.RUnlock()
	return sig
}

// reconcile 对账：新增/移除 LTP3 包 → 加载/卸载对应虚拟机。
func (e *engine) reconcile() {
	entries, err := os.ReadDir(e.root)
	if err != nil {
		return
	}
	seen := map[string]string{} // id → dirName
	for _, ent := range entries {
		if !ent.IsDir() {
			continue
		}
		dir := ent.Name()
		root := e.root + "/" + dir
		id, title, _, is, _ := readMeta(root)
		if !is || id == "" {
			continue
		}
		seen[id] = dir
		e.mu.RLock()
		exists := e.plugins[id] != nil
		e.mu.RUnlock()
		if !exists {
			e.loadPackage(id, dir, root, title)
		}
	}
	// 卸载磁盘上已不存在的插件（含标签被移除的包）
	e.mu.RLock()
	toRemove := []string{}
	for id, p := range e.plugins {
		if seen[id] != p.DirName {
			toRemove = append(toRemove, id)
		}
	}
	e.mu.RUnlock()
	for _, id := range toRemove {
		e.unloadPackage(id)
	}
}

// loadPackage 加载一个插件（若已加载则跳过）。
func (e *engine) loadPackage(id, dir, root, title string) {
	e.mu.Lock()
	if e.plugins[id] != nil {
		e.mu.Unlock()
		return
	}
	p := newPlugin(dir, root, id, title)
	e.plugins[id] = p
	e.byDir[dir] = id
	e.mu.Unlock()

	if err := p.load(); err != nil {
		LoggerGeneral.Warn(ServiceName, "LTP3 插件加载失败 %s: %v", id, err)
	}
}

// unloadPackage 卸载一个插件并释放虚拟机。
func (e *engine) unloadPackage(id string) {
	e.mu.Lock()
	p := e.plugins[id]
	if p == nil {
		e.mu.Unlock()
		return
	}
	delete(e.plugins, id)
	delete(e.byDir, p.DirName)
	e.mu.Unlock()
	p.unload()
}

// reloadPackage 重载一个插件。
func (e *engine) reloadPackage(id string) error {
	e.mu.RLock()
	p := e.plugins[id]
	e.mu.RUnlock()
	if p == nil {
		return fmt.Errorf("插件 %s 未加载", id)
	}
	p.unload()
	e.loadPackage(id, p.DirName, p.Root, p.Title)
	return nil
}

// states 返回所有插件状态。
func (e *engine) states() []manageState {
	e.mu.RLock()
	defer e.mu.RUnlock()
	out := make([]manageState, 0, len(e.plugins))
	for _, p := range e.plugins {
		st := manageState{ID: p.ID, DirName: p.DirName, Title: p.Title, Loaded: p.loaded}
		if p.loadErr != "" {
			st.Error = p.loadErr
		}
		out = append(out, st)
	}
	return out
}

// DispatchHook 分派钩子：把所有订阅该钩子点的插件回调执行一遍，返回结果与汇总。
func (e *engine) DispatchHook(hookType string, payload any, ctx map[string]any, requestID string) ([]hookOutcome, dispatchSummary) {
	e.mu.RLock()
	snap := make([]*plugin, 0, len(e.plugins))
	for _, p := range e.plugins {
		if len(p.hooks[hookType]) > 0 {
			snap = append(snap, p)
		}
	}
	e.mu.RUnlock()

	summary := dispatchSummary{AllowContinue: true}
	all := []hookOutcome{}
	for _, p := range snap {
		p.mu.Lock()
		old := p.currentRequestID
		p.currentRequestID = requestID
		outs := p.runHook(hookType, payload, ctx)
		p.currentRequestID = old
		p.mu.Unlock()
		for _, oc := range outs {
			all = append(all, oc)
			if oc.Error != "" {
				summary.Errored++
				summary.Subscribed++
				continue
			}
			summary.Subscribed++
			if m, ok := oc.Result.(map[string]any); ok {
				if m["action"] == "abort" {
					summary.Aborted = true
					summary.AllowContinue = false
					continue
				}
				if ac, ok := m["allowContinue"].(bool); ok && !ac {
					summary.AllowContinue = false
				}
			}
		}
	}
	return all, summary
}

// PublishEvent 发布事件到所有订阅该事件的插件订阅者，返回调用次数。
func (e *engine) PublishEvent(topic string, payload any) int {
	e.mu.RLock()
	snap := make([]*plugin, 0, len(e.plugins))
	for _, p := range e.plugins {
		if len(p.events[topic]) > 0 {
			snap = append(snap, p)
		}
	}
	e.mu.RUnlock()

	cnt := 0
	for _, p := range snap {
		p.mu.Lock()
		subs := len(p.events[topic])
		p.fireEventLocal(topic, payload)
		p.mu.Unlock()
		cnt += subs
	}
	return cnt
}

// callCrossPlugin 跨插件 API 调用：qualified 形如 "插件id.方法名"。
func (e *engine) callCrossPlugin(from *plugin, qualified string, params map[string]any) (any, error) {
	dot := -1
	for i := len(qualified) - 1; i >= 0; i-- {
		if qualified[i] == '.' {
			dot = i
			break
		}
	}
	if dot <= 0 {
		return nil, fmt.Errorf("API 限定名格式错误: %s（应为 插件ID.方法名）", qualified)
	}
	targetID := qualified[:dot]
	method := qualified[dot+1:]
	if targetID == from.ID {
		// 本插件内部调用：调用方已持有 from.mu，不可再上锁（避免自锁死锁）
		api := from.apis[method]
		if api == nil {
			return nil, fmt.Errorf("API %s 未注册", method)
		}
		return from.callFn(api.handler, params)
	}
	e.mu.RLock()
	target := e.plugins[targetID]
	e.mu.RUnlock()
	if target == nil {
		return nil, fmt.Errorf("目标插件 %s 未加载", targetID)
	}
	target.mu.Lock()
	api := target.apis[method]
	if api == nil || !api.public {
		target.mu.Unlock()
		return nil, fmt.Errorf("目标插件 %s 的 API %s 未注册或非 public", targetID, method)
	}
	res, err := target.callFn(api.handler, params)
	target.mu.Unlock()
	return res, err
}

// startReconcile 启动周期性对账循环（增删包 → 加载/卸载虚拟机）。
func (e *engine) startReconcile() {
	reconcileStop = make(chan struct{})
	reconcileWG.Add(1)
	go func() {
		defer reconcileWG.Done()
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
				LoggerGeneral.Info(ServiceName, "检测到 LTP3 包目录变化，执行对账")
				e.reconcile()
				last = e.fingerprint()
			case <-reconcileStop:
				return
			}
		}
	}()
}

// stopReconcile 停止对账循环。
func (e *engine) stopReconcile() {
	if reconcileStop != nil {
		close(reconcileStop)
		reconcileStop = nil
	}
	reconcileWG.Wait()
}

// shutdown 卸载全部插件。
func (e *engine) shutdown() {
	e.mu.RLock()
	ids := make([]string, 0, len(e.plugins))
	for id := range e.plugins {
		ids = append(ids, id)
	}
	e.mu.RUnlock()
	for _, id := range ids {
		e.unloadPackage(id)
	}
}