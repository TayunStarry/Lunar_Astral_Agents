package engine

import (
	"sync"
	"time"
)

type Engine struct {
	ticker    *Ticker
	interval  time.Duration
	started   bool
	mu        sync.Mutex
	tickStats []TickStats
	errors    []ErrorInfo
	maxStats  int
}

func NewEngine(tickInterval time.Duration) *Engine {
	return &Engine{
		ticker:   NewTicker(tickInterval),
		interval: tickInterval,
		maxStats: 100,
	}
}

func (e *Engine) SetTickInterval(ms int) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.interval = time.Duration(ms) * time.Millisecond
	e.ticker.Interval = e.interval
}

func (e *Engine) LoadMarkdown(mdText string) {
	e.ticker.LoadMarkdown(mdText)
}

func (e *Engine) LoadJSON(jsonArray [][]string) {
	e.ticker.LoadJSON(jsonArray)
}

func (e *Engine) Inject(lines []string) {
	e.ticker.Inject(lines)
}

func (e *Engine) Invoke(pointerName string) {
	e.ticker.Invoke(pointerName)
}

func (e *Engine) Start() {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.started {
		return
	}

	e.ticker.SetTickCallbacks(
		func(stats TickStats) {
			e.mu.Lock()
			e.tickStats = append(e.tickStats, stats)
			if len(e.tickStats) > e.maxStats {
				e.tickStats = e.tickStats[1:]
			}
			e.mu.Unlock()
		},
		func(err ErrorInfo) {
			e.mu.Lock()
			e.errors = append(e.errors, err)
			if len(e.errors) > e.maxStats {
				e.errors = e.errors[1:]
			}
			e.mu.Unlock()
		},
		func() {},
		func() {},
	)

	e.ticker.Start()
	e.started = true
}

func (e *Engine) Stop() {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.ticker.Stop()
	e.started = false
}

func (e *Engine) Shutdown() {
	e.ticker.Shutdown()
	e.started = false
}

func (e *Engine) IsRunning() bool {
	return e.ticker.IsRunning()
}

func (e *Engine) IsSuspended() bool {
	return e.ticker.IsSuspended()
}

func (e *Engine) Resume() {
	e.ticker.Resume()
}

func (e *Engine) GetVariable(name string) string {
	return e.ticker.VS.Get(name)
}

func (e *Engine) SetVariable(name, value string) {
	e.ticker.VS.Set(name, value)
}

func (e *Engine) GetAllVariables() map[string]string {
	return e.ticker.VS.GetAll()
}

func (e *Engine) GetPointerNames() []string {
	return e.ticker.PR.GetAllClassNames()
}

func (e *Engine) PointerExists(name string) bool {
	return e.ticker.PR.Exists(name)
}

func (e *Engine) GetStats() TickStats {
	e.mu.Lock()
	defer e.mu.Unlock()

	if len(e.tickStats) > 0 {
		return e.tickStats[len(e.tickStats)-1]
	}
	return TickStats{}
}

func (e *Engine) GetRecentStats(n int) []TickStats {
	e.mu.Lock()
	defer e.mu.Unlock()

	if n <= 0 || n > len(e.tickStats) {
		n = len(e.tickStats)
	}
	start := len(e.tickStats) - n
	result := make([]TickStats, n)
	copy(result, e.tickStats[start:])
	return result
}

func (e *Engine) GetErrors() []ErrorInfo {
	e.mu.Lock()
	defer e.mu.Unlock()

	result := make([]ErrorInfo, len(e.errors))
	copy(result, e.errors)
	return result
}

func (e *Engine) SetLogFn(fn func(msg string)) {
	e.ticker.SetLogFn(fn)
}

var defaultEngine *Engine
var defaultMu sync.Mutex

func DefaultEngine() *Engine {
	defaultMu.Lock()
	defer defaultMu.Unlock()

	if defaultEngine == nil {
		defaultEngine = NewEngine(100 * time.Millisecond)
	}
	return defaultEngine
}

func LoadMarkdown(mdText string) {
	DefaultEngine().LoadMarkdown(mdText)
}

func LoadJSON(jsonArray [][]string) {
	DefaultEngine().LoadJSON(jsonArray)
}

func SetTickInterval(ms int) {
	DefaultEngine().SetTickInterval(ms)
}

func Start() {
	DefaultEngine().Start()
}

func Inject(lines []string) {
	DefaultEngine().Inject(lines)
}

func Invoke(pointerName string) {
	DefaultEngine().Invoke(pointerName)
}
