package StarLTP

// ==== 蓝图执行器（调度循环，自前端 execute.js 迁移） ====
// 前端 planning 页面仅负责图编辑/状态渲染/参数编辑/持久化与浏览器节点协作执行；
// 扁平执行池调度在本文件：ltp9/bp_run 提交图数据 → 逐节点执行 → ltp9/bp_node
// 逐节点推送状态 → ltp9/bp_done 收尾。浏览器专属节点（microphone/speaker/audio_eq/
// image_confuse/video_keyframe）经 ltp9/bp_exec 请求前端执行，前端回报 ltp9/bp_exec_result。
//
// 调度语义对齐前端扁平执行池：连线仅定义先后顺序，池内不同节点并行，同一节点多次激活
// 经节点锁串行；单节点失败仅自身出池不阻断池；callCount/maxCalls 防死循环；
// 终止条件三选一：池空无源 / 上限耗尽 / 手动停止（时钟存在则保持运行直至手动停止）。

import (
	"encoding/json"
	"fmt"
	"time"

	"LunarSubsystem/LoggerGeneral"
)

// ==== WS 入口（HandleInbound 分发） ====

// handleBlueprintRun 处理 ltp9/bp_run：校验单实例互斥 → 受理回执 → 后台 goroutine 跑调度循环
// （Inbound 消费者是单 goroutine 串行，绝不能阻塞，否则 bp_exec_result 回报无法被消费）。
func handleBlueprintRun(m map[string]any) {
	reqID, _ := m["request_id"].(string)
	raw, err := json.Marshal(m)
	if err != nil {
		bpAck(reqID, false, "", "蓝图数据解析失败: "+err.Error())
		return
	}
	var req bpRunRequest
	if err := json.Unmarshal(raw, &req); err != nil || req.Graph == nil {
		bpAck(reqID, false, "", "蓝图数据无效（缺少 graph）")
		return
	}
	mode := req.Mode
	if mode != "single" {
		mode = "all"
	}
	if mode == "single" && req.NodeID == "" {
		bpAck(reqID, false, "", "单节点运行需提供 node_id")
		return
	}
	r := newBpRunner(mode, req.Graph)
	if mode == "single" && r.nodes[req.NodeID] == nil {
		bpAck(reqID, false, "", "节点不存在: "+req.NodeID)
		return
	}

	bpMu.Lock()
	if bpCurrent != nil {
		bpMu.Unlock()
		bpAck(reqID, false, "", "已有蓝图运行中，请先停止当前运行")
		return
	}
	bpCurrent = r
	bpMu.Unlock()

	SetEmitObserver(r.notifyEventWaiters) // event_start 事件来源接入
	bpAck(reqID, true, r.runID, "")
	LoggerGeneral.Info(ServiceName, "蓝图开始运行 run=%s mode=%s nodes=%d links=%d", r.runID, mode, len(r.nodes), len(r.links))
	go r.run(mode, req.NodeID)
}

// handleBlueprintStop 处理 ltp9/bp_stop：立即收尾（reason=stopped）。
// 正在执行的阻塞节点不强杀（goja 沙箱与 HTTP 调用无法安全中断），仅停止派发后续节点。
func handleBlueprintStop(m map[string]any) {
	bpMu.Lock()
	r := bpCurrent
	bpMu.Unlock()
	if r == nil {
		return
	}
	if rid, _ := m["run_id"].(string); rid != "" && rid != r.runID {
		return
	}
	r.finish("stopped")
}

// handleBlueprintExecResult 处理 ltp9/bp_exec_result：按 exec_id 投递到等待中的节点执行器。
func handleBlueprintExecResult(m map[string]any) {
	raw, err := json.Marshal(m)
	if err != nil {
		return
	}
	var res bpExecResult
	if json.Unmarshal(raw, &res) != nil || res.ExecID == "" {
		return
	}
	bpMu.Lock()
	r := bpCurrent
	bpMu.Unlock()
	if r == nil {
		return
	}
	if rid, _ := m["run_id"].(string); rid != "" && rid != r.runID {
		return
	}
	r.execMu.Lock()
	ch := r.execWait[res.ExecID]
	r.execMu.Unlock()
	if ch != nil {
		select {
		case ch <- &res:
		default:
		}
	}
}

// bpAck 受理/拒绝回执（ltp9/bp_ack）。
func bpAck(reqID string, ok bool, runID, errMsg string) {
	res := map[string]any{"type": "ltp9/bp_ack", "ok": ok}
	if reqID != "" {
		res["request_id"] = reqID
	}
	if runID != "" {
		res["run_id"] = runID
	}
	if errMsg != "" {
		res["error"] = errMsg
	}
	if b, err := json.Marshal(res); err == nil {
		debugEmit(b)
	}
}

// ==== 运行实例 ====

// newBpRunner 构造运行实例：登记扁平节点命名空间并清洗无效连线（端点不存在的历史残留）。
func newBpRunner(mode string, g *bpGraph) *bpRunner {
	r := &bpRunner{
		runID:    fmt.Sprintf("bp%d", time.Now().UnixNano()),
		mode:     mode,
		graph:    g,
		nodes:    map[string]*bpNodeRun{},
		refs:     map[string]int{},
		comps:    map[string]*bpCompState{},
		execWait: map[string]chan *bpExecResult{},
		haltCh:   make(chan struct{}),
		maxCalls: bpDefaultMaxCalls,
		startAt:  time.Now(),
	}
	for i := range g.Nodes {
		n := g.Nodes[i]
		if n.ID == "" || n.Type == "" {
			continue
		}
		if n.Params == nil {
			n.Params = map[string]any{}
		}
		r.nodes[n.ID] = &bpNodeRun{spec: n}
		r.order = append(r.order, n.ID)
	}
	for _, l := range g.Links {
		if l.From.Node == "" || l.To.Node == "" {
			continue
		}
		if r.nodes[l.From.Node] == nil || r.nodes[l.To.Node] == nil {
			continue
		}
		r.links = append(r.links, l)
	}
	return r
}

// run 调度入口：筛选种子并触发，随后由 finishCheck 判定瞬时结束。
func (r *bpRunner) run(mode, singleID string) {
	defer func() {
		if e := recover(); e != nil {
			LoggerGeneral.Error(ServiceName, "蓝图执行器异常 run=%s: %v", r.runID, e)
			r.finish("error")
		}
	}()
	if mode == "single" {
		r.triggerSeed(r.nodes[singleID]) // 单节点执行：仅运行该节点本身，不向后续传播
		r.finishCheck()
		return
	}
	// 种子：触发型启动节点同时触发；无启动节点时以无入线节点为入口（对齐前端 runAll）
	hasIncoming := map[string]bool{}
	for _, l := range r.linksSnapshot() {
		hasIncoming[l.To.Node] = true
	}
	seeds := []*bpNodeRun{}
	for _, id := range r.order {
		n := r.nodes[id]
		if bpEnabled(n.spec) && (n.spec.Type == "start" || n.spec.Type == "event_start" || n.spec.Type == "clock_start") {
			seeds = append(seeds, n)
		}
	}
	if len(seeds) == 0 {
		for _, id := range r.order {
			n := r.nodes[id]
			if bpEnabled(n.spec) && !hasIncoming[id] {
				seeds = append(seeds, n)
			}
		}
	}
	for _, s := range seeds {
		r.triggerSeed(s)
	}
	r.finishCheck() // 无触发/瞬时结束时收尾
}

// triggerSeed 触发种子节点：先同步占位 active 计数（防止 run 末尾的 finishCheck 误判「池空无源」提前收尾），
// 再后台执行；成功后向后续传播（single 模式不传播）。
func (r *bpRunner) triggerSeed(n *bpNodeRun) {
	r.stMu.Lock()
	r.active++
	r.stMu.Unlock()
	go func() {
		out := r.execNode(n)
		if out.status == "ok" && r.mode != "single" {
			r.dispatchSuccessors(n.spec.ID)
		}
		r.stMu.Lock()
		if r.active > 0 {
			r.active--
		}
		r.stMu.Unlock()
		r.finishCheck()
	}()
}

// enterPool 节点加入执行池：每次激活一个 goroutine，同节点经节点锁串行，池内不同节点完全并行。
// comp 非 nil 表示该激活由复合节点派生（计入复合实例的完成判定）。
func (r *bpRunner) enterPool(nodeID string, comp *bpCompState) {
	if r.halted() {
		return
	}
	n := r.nodes[nodeID]
	if n == nil || !bpEnabled(n.spec) {
		return
	}
	if !r.acquireCall() {
		return // 最大节点执行数量耗尽 → 不再入池
	}
	r.stMu.Lock()
	r.refs[nodeID]++
	r.active++
	if comp != nil {
		comp.active++
	}
	r.stMu.Unlock()
	go func() {
		out := r.execNode(n)
		if out.status == "ok" {
			r.dispatchSuccessors(nodeID) // 成功 → 后续立即入池；失败 → 仅自身出池，不激活后续
		}
		r.finishActivation(nodeID, comp)
	}()
}

// acquireCall 执行数量上限判定：耗尽后不再入池（收尾时以 reason=maxcalls 告知前端）。
func (r *bpRunner) acquireCall() bool {
	r.stMu.Lock()
	defer r.stMu.Unlock()
	if r.callCount >= r.maxCalls {
		r.limitedNotified = true
		return false
	}
	r.callCount++
	return true
}

// dispatchSuccessors 节点成功出池后其所有连线指向的后续节点立即入池（同一后续只入池一次）。
func (r *bpRunner) dispatchSuccessors(nodeID string) {
	seen := map[string]bool{}
	for _, l := range r.linksSnapshot() {
		if l.From.Node != nodeID || seen[l.To.Node] {
			continue
		}
		seen[l.To.Node] = true
		t := r.nodes[l.To.Node]
		if t != nil {
			r.enterPool(l.To.Node, t.comp)
		}
	}
}

// finishActivation 激活出池：引用计数递减、复合派生计数递减（归零唤醒等待中的复合节点），随后判定收尾。
func (r *bpRunner) finishActivation(nodeID string, comp *bpCompState) {
	var done chan struct{}
	r.stMu.Lock()
	if c := r.refs[nodeID]; c > 1 {
		r.refs[nodeID] = c - 1
	} else {
		delete(r.refs, nodeID)
	}
	if r.active > 0 {
		r.active--
	}
	if comp != nil {
		if comp.active > 0 {
			comp.active--
		}
		if comp.active == 0 && !comp.closed {
			comp.closed = true
			done = comp.doneCh
		}
	}
	r.stMu.Unlock()
	if done != nil {
		close(done)
	}
	r.finishCheck()
}

// finishCheck 终止条件（三选一即终）：a) 执行池清空且无时钟持续投递；b) 上限耗尽；c) 手动停止（停止路径自行收尾）。
func (r *bpRunner) finishCheck() {
	r.stMu.Lock()
	if r.active > 0 {
		r.stMu.Unlock()
		return
	}
	maxed := r.callCount >= r.maxCalls
	r.stMu.Unlock()
	if maxed {
		r.finish("maxcalls")
		return
	}
	r.clockMu.Lock()
	nclocks := len(r.clocks)
	r.clockMu.Unlock()
	if nclocks > 0 {
		return // 时钟启动持续投递新节点，运行保持直至手动停止
	}
	r.finish("finished")
}

// finish 收尾运行（幂等，以首个原因为准）：关闭 haltCh 唤醒全部阻塞点、停时钟、撤销观察者、清理实例、推送 bp_done。
func (r *bpRunner) finish(reason string) {
	r.doneMu.Lock()
	if r.doneReason != "" {
		r.doneMu.Unlock()
		return
	}
	r.doneReason = reason
	r.doneMu.Unlock()

	r.stopOnce.Do(func() { close(r.haltCh) })

	r.clockMu.Lock()
	tickers := r.clocks
	r.clocks = nil
	r.clockMu.Unlock()
	for _, t := range tickers {
		t.Stop()
	}

	SetEmitObserver(nil)

	bpMu.Lock()
	if bpCurrent == r {
		bpCurrent = nil
	}
	bpMu.Unlock()

	r.stMu.Lock()
	executed := r.callCount
	r.stMu.Unlock()
	r.push(map[string]any{
		"type":   "ltp9/bp_done",
		"reason": reason,
		"stats": map[string]any{
			"executed":   executed,
			"nodes":      len(r.order),
			"elapsed_ms": time.Since(r.startAt).Milliseconds(),
		},
	})
	LoggerGeneral.Info(ServiceName, "蓝图运行结束 run=%s reason=%s executed=%d", r.runID, reason, executed)
}

// ==== 节点执行 ====

// execNode 执行一次节点激活：同节点经互斥锁串行；先注入上游数据，推送 running 与最终状态。
func (r *bpRunner) execNode(n *bpNodeRun) (out bpOutcome) {
	n.mu.Lock()
	defer n.mu.Unlock()
	r.pushNode(n.spec.ID, "running", nil)
	func() {
		defer func() {
			if e := recover(); e != nil {
				out = bpOutcome{status: "err", errText: fmt.Sprintf("节点执行异常: %v", e), out: map[string]any{"error": fmt.Sprintf("%v", e)}, hadOut: true}
			}
		}()
		r.injectUpstream(n)
		out = r.runNodeInner(n)
	}()
	r.storeOut(n, out)
	r.pushNode(n.spec.ID, out.status, &out)
	return out
}

// storeOut 记录节点输出（下游 nodeOut 取值与复合收尾读取）。
func (r *bpRunner) storeOut(n *bpNodeRun, out bpOutcome) {
	n.outMu.Lock()
	n.primary = out.out
	n.hadOut = out.hadOut
	n.multi = out.multi
	n.pop = out.pop
	n.status = out.status
	n.outMu.Unlock()
}

// linksSnapshot 连线快照（复合执行会并发追加派生连线，读侧经快照避免竞争）。
func (r *bpRunner) linksSnapshot() []bpLinkSpec {
	r.stMu.RLock()
	defer r.stMu.RUnlock()
	return r.links
}

// injectUpstream 上游数据注入：对每条入线按连线源端口取上游输出写入本节点参数
// （gate 顺序端口仅传放行，不注入数据，对齐前端 execute.js 通用注入）。
func (r *bpRunner) injectUpstream(n *bpNodeRun) {
	for _, l := range r.linksSnapshot() {
		if l.To.Node != n.spec.ID {
			continue
		}
		if l.To.Port == "gate" || l.From.Port == "gate" {
			continue
		}
		sv := r.nodeOut(r.nodes[l.From.Node], l.From.Port)
		if sv != nil {
			n.spec.Params[l.To.Port] = sv
		}
	}
}

// nodeOut 取节点输出：多输出含该端口则取之，否则取主输出（对齐前端 srcOut）。
func (r *bpRunner) nodeOut(n *bpNodeRun, port string) any {
	if n == nil {
		return nil
	}
	n.outMu.Lock()
	defer n.outMu.Unlock()
	if n.multi != nil {
		if v, ok := n.multi[port]; ok {
			return v
		}
	}
	if !n.hadOut {
		return nil
	}
	return n.primary
}

// ==== 浏览器协作节点 ====

// bpExecBrowser 推送 ltp9/bp_exec 请求前端执行（params 为已注入上游数据的最终参数），
// 阻塞等待回报：超时 → err；运行停止 → skip；回报含 params_patch 与气泡随 bp_node 推回。
func (r *bpRunner) bpExecBrowser(n *bpNodeRun) bpOutcome {
	r.execMu.Lock()
	r.execSeq++
	execID := fmt.Sprintf("e%d", r.execSeq)
	ch := make(chan *bpExecResult, 1)
	r.execWait[execID] = ch
	r.execMu.Unlock()
	defer func() {
		r.execMu.Lock()
		delete(r.execWait, execID)
		r.execMu.Unlock()
	}()

	r.push(map[string]any{
		"type": "ltp9/bp_exec", "exec_id": execID, "node_id": n.spec.ID,
		"node_type": n.spec.Type, "params": n.spec.Params,
	})

	var res *bpExecResult
	select {
	case res = <-ch:
	case <-time.After(bpExecTimeout):
		res = &bpExecResult{OK: false, Error: "浏览器节点执行回报超时"}
	case <-r.haltCh:
		return bpOutcome{status: "skip"}
	}
	if res == nil || !res.OK {
		msg := "浏览器节点执行失败"
		if res != nil && res.Error != "" {
			msg = res.Error
		}
		return bpOutcome{status: "err", errText: msg, out: map[string]any{"error": msg}, hadOut: true}
	}
	return bpOutcome{status: "ok", out: res.Value, hadOut: true, patch: res.ParamsPatch, pop: res.Pop}
}

// ==== 事件等待（event_start） ====

// notifyEventWaiters 事件观察者回调（SetEmitObserver 注入，须非阻塞）：
// 把事件载荷投递给匹配 topic 的等待者（topic 空 = 任意事件），不匹配的等待者保留。
func (r *bpRunner) notifyEventWaiters(topic string, payload any) {
	r.evWaitMu.Lock()
	waiters := r.evWaiters
	r.evWaiters = nil
	r.evWaitMu.Unlock()
	for _, w := range waiters {
		if w.topic != "" && w.topic != topic {
			r.evWaitMu.Lock()
			r.evWaiters = append(r.evWaiters, w)
			r.evWaitMu.Unlock()
			continue
		}
		select {
		case w.resultCh <- map[string]any{"topic": topic, "payload": payload}:
		default:
		}
	}
}

// registerEventWait 登记事件等待者。
func (r *bpRunner) registerEventWait(topic string) *bpEventWait {
	w := &bpEventWait{topic: topic, resultCh: make(chan any, 1)}
	r.evWaitMu.Lock()
	r.evWaiters = append(r.evWaiters, w)
	r.evWaitMu.Unlock()
	return w
}

// removeEventWait 撤销事件等待者（超时路径）。
func (r *bpRunner) removeEventWait(w *bpEventWait) {
	r.evWaitMu.Lock()
	for i, x := range r.evWaiters {
		if x == w {
			r.evWaiters = append(r.evWaiters[:i], r.evWaiters[i+1:]...)
			break
		}
	}
	r.evWaitMu.Unlock()
}

// ==== 时钟（clock_start） ====

// addClock 登记时钟定时器（finish 时统一停止）；已收尾时立即停止并拒绝。
func (r *bpRunner) addClock(t *time.Ticker) bool {
	r.clockMu.Lock()
	defer r.clockMu.Unlock()
	if r.doneReason != "" {
		t.Stop()
		return false
	}
	r.clocks = append(r.clocks, t)
	return true
}

// ==== 推送 ====

// push 广播一条蓝图信封到 /ws 集线器（自动携带 run_id，前端按 run_id 过滤多标签页串扰）。
func (r *bpRunner) push(m map[string]any) {
	m["run_id"] = r.runID
	b, err := json.Marshal(m)
	if err != nil {
		LoggerGeneral.Warn(ServiceName, "蓝图推送序列化失败: %v", err)
		return
	}
	debugEmit(b)
}

// pushNode 推送单节点状态：running（开始执行）或 ok/err/skip（完成，携带输出/回写/气泡/错误）。
func (r *bpRunner) pushNode(nodeID, status string, out *bpOutcome) {
	m := map[string]any{"type": "ltp9/bp_node", "node_id": nodeID, "status": status}
	if out != nil {
		if out.hadOut {
			m["out_value"] = out.out
		}
		if out.multi != nil {
			m["out_values"] = out.multi
		}
		if out.patch != nil {
			m["params_patch"] = out.patch
		}
		if out.pop != nil {
			m["pop"] = out.pop
		}
		if out.status == "err" {
			m["error"] = out.errText
		}
	}
	r.push(m)
}

// ==== 小工具 ====

// bpEnabled 节点是否启用（对齐前端 enabled !== false）。
func bpEnabled(s bpNodeSpec) bool { return s.Enabled == nil || *s.Enabled }

// halted 运行是否已终止/收尾。
func (r *bpRunner) halted() bool {
	select {
	case <-r.haltCh:
		return true
	default:
		return false
	}
}

// bpNum / bpStr 参数读取辅助（JSON 解码产物：数字为 float64）。
func bpNum(p map[string]any, k string, def float64) float64 {
	if v, ok := p[k].(float64); ok {
		return v
	}
	return def
}

func bpStr(p map[string]any, k string) string {
	s, _ := p[k].(string)
	return s
}
