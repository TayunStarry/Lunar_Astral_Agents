package engine

import (
	"context"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"logger"
)

type Ticker struct {
	Interval   time.Duration
	VS         *VarStore
	PR         *PointerRegistry
	BM         *BlockManager
	PE         *ProcessExecutor
	tickNum    int64
	running    atomic.Bool
	suspended  atomic.Bool
	mu         sync.Mutex
	ctx        context.Context
	cancel     context.CancelFunc
	onTick     func(TickStats)
	onError    func(ErrorInfo)
	onSuspend  func()
	onResume   func()
	logFn      func(msg string)
	readyCheck chan struct{}
}

func NewTicker(interval time.Duration) *Ticker {
	ctx, cancel := context.WithCancel(context.Background())
	return &Ticker{
		Interval:   interval,
		VS:         NewVarStore(),
		PR:         NewPointerRegistry(),
		BM:         NewBlockManager(),
		PE:         NewProcessExecutor(),
		ctx:        ctx,
		cancel:     cancel,
		readyCheck: make(chan struct{}, 1),
	}
}

func (t *Ticker) SetTickCallbacks(onTick func(TickStats), onError func(ErrorInfo), onSuspend func(), onResume func()) {
	t.onTick = onTick
	t.onError = onError
	t.onSuspend = onSuspend
	t.onResume = onResume
}

func (t *Ticker) SetLogFn(fn func(msg string)) {
	t.logFn = fn
}

func (t *Ticker) Start() {
	t.running.Store(true)
	t.VS.Set("TICK_MS", strconv.Itoa(int(t.Interval.Milliseconds())))
	go t.loop()
}

func (t *Ticker) Stop() {
	t.cancel()
	t.running.Store(false)
}

func (t *Ticker) IsRunning() bool {
	return t.running.Load()
}

func (t *Ticker) IsSuspended() bool {
	return t.suspended.Load()
}

func (t *Ticker) Resume() {
	if t.suspended.CompareAndSwap(true, false) {
		if t.onResume != nil {
			t.onResume()
		}
		t.readyCheck <- struct{}{}
	}
}

func (t *Ticker) Inject(lines []string) {
	block := t.BM.CreateBlock(lines, "")
	t.BM.AddReady(block.ID)
	if t.suspended.Load() {
		t.Resume()
	}
}

func (t *Ticker) Invoke(pointerName string) {
	entry, exists := t.PR.Get(pointerName)
	if !exists || entry.Type != PointerClass || len(entry.Lines) == 0 {
		return
	}

	block := t.BM.CreateBlock(entry.Lines, pointerName)
	t.BM.AddPending(block)

	if t.suspended.Load() {
		t.Resume()
	}
}

func (t *Ticker) LoadMarkdown(mdText string) {
	blocks := extractMarkdownBlocks(mdText)
	for _, lines := range blocks {
		t.loadBlock(lines)
	}
}

func (t *Ticker) LoadJSON(jsonArray [][]string) {
	for _, lines := range jsonArray {
		t.loadBlock(lines)
	}
}

func (t *Ticker) loadBlock(lines []string) {
	if len(lines) == 0 {
		return
	}

	firstLine := ""
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" && !strings.HasPrefix(trimmed, "//") {
			firstLine = trimmed
			break
		}
	}

	if strings.HasPrefix(strings.ToUpper(firstLine), "@LAZY") {
		parts := strings.Fields(firstLine)
		if len(parts) >= 2 && strings.HasPrefix(parts[1], "*") {
			ptrName := strings.TrimPrefix(parts[1], "*")
			t.PR.RegisterLazy(ptrName, lines)
		}
		return
	}

	if strings.HasPrefix(strings.ToUpper(firstLine), "@DEF") {
		instr := ParseInstruction(firstLine)
		for _, arg := range instr.Args {
			if arg.Type == ArgPointer {
				t.PR.Define(arg.Value)
			}
		}
	}

	block := t.BM.CreateBlock(lines, "")
	t.BM.AddReady(block.ID)
}

func (t *Ticker) loop() {
	logger.Info("LunarTick", "Tick 循环已启动，间隔: %v", t.Interval)

	for t.running.Load() {
		tickStart := time.Now()
		tickNum := int(atomic.AddInt64(&t.tickNum, 1))
		t.VS.Set("TICK", strconv.Itoa(tickNum))

		t.phaseInject()
		t.phaseExecute(tickNum)
		t.phaseCheckReady()
		t.phaseCleanup()

		if t.BM.ActiveCount() == 0 {
			t.suspended.Store(true)
			logger.Info("LunarTick", "系统进入停摆状态 (tick %d)", tickNum)
			if t.onSuspend != nil {
				t.onSuspend()
			}

			select {
			case <-t.readyCheck:
				logger.Info("LunarTick", "系统从停摆中恢复")
			case <-t.ctx.Done():
				t.running.Store(false)
				return
			}
			continue
		}

		if t.onTick != nil {
			stats := t.BM.Stats()
			stats.TickNumber = tickNum
			stats.ProcessLatency = time.Since(tickStart)
			t.onTick(stats)
		}

		elapsed := time.Since(tickStart)
		remaining := t.Interval - elapsed
		if remaining > 0 {
			select {
			case <-time.After(remaining):
			case <-t.ctx.Done():
				t.running.Store(false)
				return
			}
		} else if elapsed > t.Interval*2 {
			logger.Warn("LunarTick", "Tick %d 处理延迟过高: %v", tickNum, elapsed)
		}
	}

	logger.Info("LunarTick", "Tick 循环已停止")
}

func (t *Ticker) phaseInject() {
	t.BM.FlushPending()
}

func (t *Ticker) phaseExecute(tickNum int) {
	readyBlocks := t.BM.GetReadyList()

	for _, blockID := range readyBlocks {
		block := t.BM.GetBlock(blockID)
		if block == nil || block.Status != StatusReady {
			continue
		}

		if block.LimitMS > 0 && time.Since(block.StartTime) > block.LimitMS {
			if block.RetryMax > 0 && block.RetryCount < block.RetryMax {
				block.RetryCount++
				block.PC = 0
				block.StartTime = time.Now()
				if block.RetryCool > 0 {
					cond := WaitCondition{
						Type:  WaitSleep,
						Until: time.Now().Add(block.RetryCool),
					}
					t.BM.MoveToWaiting(blockID, cond)
					continue
				}
			} else {
				t.BM.Terminate(blockID)
				if t.onError != nil {
					t.onError(ErrorInfo{BlockID: blockID, Message: "limit exceeded", TickNumber: tickNum})
				}
				continue
			}
		}

		if block.PC >= len(block.Instructions) {
			t.BM.Terminate(blockID)
			continue
		}

		instr := block.Instructions[block.PC]

		ctx := &ExecutionContext{
			Block:   block,
			Instr:   instr,
			VS:      t.VS,
			PR:      t.PR,
			BM:      t.BM,
			PE:      t.PE,
			TickNum: tickNum,
			LogFn:   t.logFn,
		}

		result := ExecuteInstruction(ctx)

		switch result {
		case ResultContinue:
			t.BM.AdvancePC(blockID)
		case ResultBlocked:
		case ResultTerminated:
		case ResultError:
			if block.RetryMax > 0 && block.RetryCount < block.RetryMax {
				block.RetryCount++
				block.PC = 0
				if block.RetryCool > 0 {
					cond := WaitCondition{
						Type:  WaitSleep,
						Until: time.Now().Add(block.RetryCool),
					}
					t.BM.MoveToWaiting(blockID, cond)
				}
			} else {
				t.BM.Terminate(blockID)
				if t.onError != nil {
					t.onError(ErrorInfo{BlockID: blockID, Message: "instruction error", TickNumber: tickNum})
				}
			}
		}
	}
}

func (t *Ticker) phaseCheckReady() {
	waitBlocks := t.BM.GetWaitList()

	for _, blockID := range waitBlocks {
		block := t.BM.GetBlock(blockID)
		if block == nil || block.Status != StatusWaiting {
			continue
		}

		switch block.WaitCond.Type {
		case WaitVariable:
			val := t.VS.Get(block.WaitCond.Var)
			if val != "" {
				t.BM.MoveToReady(blockID)
			}

		case WaitSleep:
			if time.Now().After(block.WaitCond.Until) {
				t.BM.MoveToReady(blockID)
			}

		case WaitProcess:
			if t.PE.IsDone(blockID) {
				result := t.PE.GetResult(blockID)
				if result != nil {
					t.VS.Set("?", strconv.Itoa(result.ExitCode))
				}
				t.PE.Remove(blockID)
				t.BM.MoveToReady(blockID)
			}

		case WaitCatch:
			line, ok := t.PE.ReadLine(blockID)
			if ok {
				block := t.BM.GetBlock(blockID)
				if block != nil && block.CatchSubstr != "" {
					if strings.Contains(line, block.CatchSubstr) {
						t.VS.Add(block.CatchVar, line)
						t.BM.MoveToReady(blockID)
					}
				}
			}
			if t.PE.IsDone(blockID) {
				t.BM.MoveToReady(blockID)
			}

		case WaitNone:
			t.BM.MoveToReady(blockID)
		}

		if block.CallRunning && block.CallPtr != "" {
			line, ok := t.PE.ReadLine(blockID)
			if ok {
				t.VS.Set(block.CallVar, line)
				ptr, exists := t.PR.Get(block.CallPtr)
				if exists && ptr.Type == PointerClass && len(ptr.Lines) > 0 {
					newBlock := t.BM.CreateBlock(ptr.Lines, block.CallPtr)
					t.BM.AddPending(newBlock)
				}
			}
		}
	}
}

func (t *Ticker) phaseCleanup() {
	t.BM.CleanupTerminated()
}

func (t *Ticker) Shutdown() {
	t.cancel()
	t.running.Store(false)
	t.PE.Shutdown()
}

func extractMarkdownBlocks(mdText string) [][]string {
	var blocks [][]string
	lines := strings.Split(mdText, "\n")
	inBlock := false
	var currentBlock []string

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") {
			if inBlock {
				if len(currentBlock) > 0 {
					blocks = append(blocks, currentBlock)
					currentBlock = nil
				}
				inBlock = false
			} else if strings.Contains(trimmed, "LunarTick") || strings.Contains(trimmed, "lunartick") {
				inBlock = true
				currentBlock = nil
			}
		} else if inBlock {
			currentBlock = append(currentBlock, line)
		}
	}

	return blocks
}
