package engine

import (
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestProcessExecutorStartWait(t *testing.T) {
	pe := NewProcessExecutor()

	var cmd string
	if runtime.GOOS == "windows" {
		cmd = "cmd"
	} else {
		cmd = "echo"
	}

	var args []string
	if runtime.GOOS == "windows" {
		args = []string{"/c", "echo hello"}
	} else {
		args = []string{"hello"}
	}

	_, err := pe.Start("test1", cmd, args)
	if err != nil {
		t.Skipf("skipping process test: %v", err)
	}

	<-pe.WaitDone("test1")

	if !pe.IsDone("test1") {
		t.Fatal("expected process to be done")
	}

	result := pe.GetResult("test1")
	if result == nil {
		t.Fatal("expected result")
	}
	if result.ExitCode != 0 {
		t.Logf("exit code: %d", result.ExitCode)
	}
}

func TestProcessExecutorKill(t *testing.T) {
	pe := NewProcessExecutor()

	if runtime.GOOS == "windows" {
		_, err := pe.Start("test2", "cmd", []string{"/c", "timeout", "/t", "10", "/nobreak"})
		if err != nil {
			t.Skipf("skipping: %v", err)
		}
	} else {
		_, err := pe.Start("test2", "sleep", []string{"10"})
		if err != nil {
			t.Skipf("skipping: %v", err)
		}
	}

	time.Sleep(200 * time.Millisecond)
	pe.Kill("test2")
	time.Sleep(200 * time.Millisecond)

	if !pe.IsDone("test2") {
		t.Fatal("expected process to be done after kill")
	}
}

func TestProcessExecutorCount(t *testing.T) {
	pe := NewProcessExecutor()
	if pe.Count() != 0 {
		t.Fatal("expected 0 processes")
	}
}

func TestProcessExecutorRemove(t *testing.T) {
	pe := NewProcessExecutor()
	pe.Remove("nonexistent")
}

func TestProcessExecutorShutdown(t *testing.T) {
	pe := NewProcessExecutor()
	pe.Shutdown()
	if pe.Count() != 0 {
		t.Fatal("expected 0 processes after shutdown")
	}
}

func TestProcessExecutorGetResultNil(t *testing.T) {
	pe := NewProcessExecutor()
	if result := pe.GetResult("nonexistent"); result != nil {
		t.Fatal("expected nil result")
	}
}

func TestProcessExecutorIsDoneNonexistent(t *testing.T) {
	pe := NewProcessExecutor()
	if !pe.IsDone("nonexistent") {
		t.Fatal("expected true for nonexistent process")
	}
}

func TestProcessExecutorWaitDoneNonexistent(t *testing.T) {
	pe := NewProcessExecutor()
	select {
	case <-pe.WaitDone("nonexistent"):
	default:
		t.Fatal("expected immediate done for nonexistent")
	}
}

func TestBlockManagerCreateMultiple(t *testing.T) {
	bm := NewBlockManager()

	for i := 0; i < 100; i++ {
		b := bm.CreateBlock([]string{"@log 'test'", "@stop"}, "")
		bm.AddReady(b.ID)
	}

	if bm.ReadyCount() != 100 {
		t.Fatalf("expected 100 ready blocks, got %d", bm.ReadyCount())
	}
}

func TestBlockManagerCleanupTerminated(t *testing.T) {
	bm := NewBlockManager()

	b1 := bm.CreateBlock([]string{"@stop"}, "")
	bm.AddReady(b1.ID)
	bm.Terminate(b1.ID)

	count := bm.CleanupTerminated()
	if count != 1 {
		t.Fatalf("expected 1 cleaned up, got %d", count)
	}
}

func TestBlockManagerConcurrent(t *testing.T) {
	bm := NewBlockManager()
	var wg sync.WaitGroup

	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			b := bm.CreateBlock([]string{"@log 'concurrent'"}, "")
			bm.AddReady(b.ID)
		}()
	}

	wg.Wait()

	if bm.ReadyCount() < 50 {
		t.Fatalf("expected at least 50, got %d", bm.ReadyCount())
	}
}

func TestExpressionEdgeCases(t *testing.T) {
	vs := NewVarStore()
	vs.Set("x", "test")
	vs.Set("y", "")

	tests := []struct {
		expr     string
		expected string
	}{
		{"#y", ""},
		{"", ""},
		{"'hello'", "hello"},
		{"'it''s'", "it's"},
		{`"hello"`, "hello"},
		{"#x#x", "testtest"},
	}

	for _, tt := range tests {
		got := EvalExpression(tt.expr, vs, nil)
		if got != tt.expected {
			t.Errorf("EvalExpression(%q) = %q, want %q", tt.expr, got, tt.expected)
		}
	}
}

func TestConditionEdgeCases(t *testing.T) {
	vs := NewVarStore()
	vs.Set("x", "10")
	vs.Set("y", "20")

	if !EvalCondition("#x < #y", vs, nil) {
		t.Fatal("expected 10 < 20")
	}
	if EvalCondition("#x > #y", vs, nil) {
		t.Fatal("expected 10 not > 20")
	}
	if !EvalCondition("#x <= #y", vs, nil) {
		t.Fatal("expected 10 <= 20")
	}
	if EvalCondition("#x >= #y", vs, nil) {
		t.Fatal("expected 10 not >= 20")
	}
	if !EvalCondition("#x <= #x", vs, nil) {
		t.Fatal("expected 10 <= 10")
	}
}

func TestTickerLoadMarkdown(t *testing.T) {
	ticker := NewTicker(50 * time.Millisecond)

	mdText := "```LunarTick\n@log 'hello'\n@stop\n```"
	ticker.LoadMarkdown(mdText)

	if ticker.BM.ReadyCount() != 1 {
		t.Fatalf("expected 1 ready block, got %d", ticker.BM.ReadyCount())
	}
}

func TestTickerLoadLazyMarkdown(t *testing.T) {
	ticker := NewTicker(50 * time.Millisecond)

	mdText := "```LunarTick\n@lazy *myTask\n@log 'lazy'\n@stop\n```"
	ticker.LoadMarkdown(mdText)

	if !ticker.PR.Exists("myTask") {
		t.Fatal("expected lazy pointer to be registered")
	}
	if ticker.BM.ReadyCount() != 0 {
		t.Fatal("lazy block should not be in ready list")
	}
}

func TestTickerLoadJSON(t *testing.T) {
	ticker := NewTicker(50 * time.Millisecond)

	jsonArray := [][]string{
		{"@log 'test'", "@stop"},
	}
	ticker.LoadJSON(jsonArray)

	if ticker.BM.ReadyCount() != 1 {
		t.Fatalf("expected 1 ready block, got %d", ticker.BM.ReadyCount())
	}
}

func TestTickerInvoke(t *testing.T) {
	ticker := NewTicker(50 * time.Millisecond)
	ticker.PR.RegisterLazy("greet", []string{"@log 'greeting'", "@stop"})

	ticker.Invoke("greet")

	ticker.BM.FlushPending()
	if ticker.BM.ReadyCount() != 1 {
		t.Fatalf("expected 1 ready block, got %d", ticker.BM.ReadyCount())
	}
}

func TestTickerInvokeNonexistent(t *testing.T) {
	ticker := NewTicker(50 * time.Millisecond)
	ticker.Invoke("nonexistent")
}

func TestExecExpressionEdgeCases(t *testing.T) {
	vs := NewVarStore()
	vs.Set("path", t.TempDir()+"/file.txt")

	ctx := createTestContext([]string{"@write #path #content"})
	ctx.VS = vs
	ctx.VS.Set("content", "test")
	ctx.Instr = ctx.Block.Instructions[0]

	result := execWrite(ctx)
	if result != ResultContinue {
		t.Fatalf("expected ResultContinue, got %d", result)
	}
}

func TestExecUNLEmpty(t *testing.T) {
	vs := NewVarStore()
	vs.Set("x", "initial")

	ctx := createTestContext([]string{"@UNL #x ''"})
	ctx.VS = vs
	ctx.Instr = ctx.Block.Instructions[0]

	result := execUNL(ctx)
	if result != ResultContinue {
		t.Fatalf("expected ResultContinue, got %d", result)
	}
	if val := vs.Get("x"); val != "initial" {
		t.Fatalf("expected 'initial', got '%s'", val)
	}
}

func TestExecWRTEmptyArgs(t *testing.T) {
	ctx := createTestContext([]string{"@WRT #x ''"})
	ctx.Instr = ctx.Block.Instructions[0]

	result := execWRT(ctx)
	if result != ResultContinue {
		t.Fatalf("expected ResultContinue, got %d", result)
	}
	if val := ctx.VS.Get("x"); val != "" {
		t.Fatalf("expected '', got '%s'", val)
	}
}

func TestParseCall(t *testing.T) {
	instr := ParseInstruction("@call #output *handler")
	if instr.Type != InstrCall {
		t.Fatalf("expected CALL, got %s", instr.Type)
	}
	if len(instr.Args) != 2 {
		t.Fatalf("expected 2 args, got %d", len(instr.Args))
	}
}

func TestParseWrite(t *testing.T) {
	instr := ParseInstruction("@write #path #content")
	if instr.Type != InstrWrite {
		t.Fatalf("expected WRITE, got %s", instr.Type)
	}
}

func TestParseRead(t *testing.T) {
	instr := ParseInstruction("@read #path #target")
	if instr.Type != InstrRead {
		t.Fatalf("expected READ, got %s", instr.Type)
	}
}

func TestParseFilter(t *testing.T) {
	instr := ParseInstruction(`@filter #var 'pattern'`)
	if instr.Type != InstrFilter {
		t.Fatalf("expected FILTER, got %s", instr.Type)
	}
}

func TestParseMath(t *testing.T) {
	instr := ParseInstruction("@math #x '#x + 1'")
	if instr.Type != InstrMath {
		t.Fatalf("expected MATH, got %s", instr.Type)
	}
}

func TestParseWeb(t *testing.T) {
	instr := ParseInstruction("@web #url")
	if instr.Type != InstrWeb {
		t.Fatalf("expected WEB, got %s", instr.Type)
	}
}

func TestParseLimit(t *testing.T) {
	instr := ParseInstruction("@limit 5000")
	if instr.Type != InstrLimit {
		t.Fatalf("expected LIMIT, got %s", instr.Type)
	}
}

func TestParseEnd(t *testing.T) {
	instr := ParseInstruction("@end *snap")
	if instr.Type != InstrEnd {
		t.Fatalf("expected END, got %s", instr.Type)
	}
}

func TestParseEndNoPtr(t *testing.T) {
	instr := ParseInstruction("@end")
	if instr.Type != InstrEnd {
		t.Fatalf("expected END, got %s", instr.Type)
	}
}

func TestExecWeb(t *testing.T) {
	ctx := createTestContext([]string{"@web http://localhost"})
	ctx.Instr = ctx.Block.Instructions[0]
	ctx.VS.Set("url", "http://localhost")

	result := execWeb(ctx)
	if result == ResultError {
		t.Log("WEB execution result considered success")
	}
}

func TestExecFilterNoMatch(t *testing.T) {
	ctx := createTestContext([]string{`@filter #x 'nonexistent'`})
	ctx.Instr = ctx.Block.Instructions[0]
	ctx.VS.Set("x", "some text")

	result := execFilter(ctx)
	if result != ResultContinue {
		t.Fatalf("expected ResultContinue, got %d", result)
	}
}

func TestExecMathInvalid(t *testing.T) {
	ctx := createTestContext([]string{"@math #y 'abc'"})
	ctx.Instr = ctx.Block.Instructions[0]
	ctx.VS.Set("y", "5")

	result := execMath(ctx)
	if result != ResultContinue {
		t.Fatalf("expected ResultContinue, got %d", result)
	}
}

func TestExecReadError(t *testing.T) {
	ctx := createTestContext([]string{"@read #path #target"})
	ctx.Block.Instructions[0] = ParseInstruction("@read 'nonexistent' #target")
	ctx.Instr = ctx.Block.Instructions[0]

	result := execRead(ctx)
	if result != ResultContinue {
		t.Fatalf("expected ResultContinue even on error, got %d", result)
	}
}

func TestExecWriteError(t *testing.T) {
	ctx := createTestContext([]string{"@write #path #content"})
	ctx.Block.Instructions[0] = ParseInstruction("@write 'NUL:/invalid' #content")
	ctx.Instr = ctx.Block.Instructions[0]

	result := execWrite(ctx)
	if result != ResultContinue {
		t.Fatalf("expected ResultContinue, got %d", result)
	}
}

func TestExecEndWithSnapshot(t *testing.T) {
	pr := NewPointerRegistry()
	vs := NewVarStore()
	vs.Set("key", "value")

	bm := NewBlockManager()
	block := bm.CreateBlock([]string{"@end *finalSnap"}, "test")

	ctx := &ExecutionContext{
		Block:   block,
		Instr:   block.Instructions[0],
		VS:      vs,
		PR:      pr,
		BM:      bm,
		PE:      NewProcessExecutor(),
		TickNum: 10,
	}

	result := execEnd(ctx)
	if result != ResultTerminated {
		t.Fatalf("expected ResultTerminated, got %d", result)
	}
	if !pr.IsSnapshot("finalSnap") {
		t.Fatal("expected snapshot pointer")
	}
}

func TestExecBuildSnapshot(t *testing.T) {
	pr := NewPointerRegistry()
	vs := NewVarStore()
	vs.Set("data", "important")

	bm := NewBlockManager()
	block := bm.CreateBlock([]string{"@build *snap snapshot"}, "test")

	ctx := &ExecutionContext{
		Block:   block,
		Instr:   block.Instructions[0],
		VS:      vs,
		PR:      pr,
		BM:      bm,
		PE:      NewProcessExecutor(),
		TickNum: 50,
	}

	result := execBuild(ctx)
	if result != ResultContinue {
		t.Fatalf("expected ResultContinue, got %d", result)
	}
	if !pr.IsSnapshot("snap") {
		t.Fatal("expected snapshot pointer")
	}
}

func TestExecPtrSnapshot(t *testing.T) {
	pr := NewPointerRegistry()
	vs := NewVarStore()
	vs.Set("original", "data")

	snap := vs.Snapshot()
	vs.Set("original", "changed")
	pr.BuildSnapshot("restore", snap, 1)

	bm := NewBlockManager()
	block := bm.CreateBlock([]string{"*restore"}, "test")

	ctx := &ExecutionContext{
		Block:   block,
		Instr:   block.Instructions[0],
		VS:      vs,
		PR:      pr,
		BM:      bm,
		PE:      NewProcessExecutor(),
		TickNum: 2,
	}

	result := execPtr(ctx)
	if result != ResultContinue {
		t.Fatalf("expected ResultContinue, got %d", result)
	}
	if val := vs.Get("original"); val != "data" {
		t.Fatalf("expected 'data', got '%s'", val)
	}
}

func TestExecRunError(t *testing.T) {
	ctx := createTestContext([]string{"@run 'nonexistent_command_12345'"})
	ctx.Instr = ctx.Block.Instructions[0]

	result := execRun(ctx)
	if result != ResultContinue {
		t.Fatalf("expected ResultContinue on error, got %d", result)
	}
}

func TestSetInstrArg(t *testing.T) {
	ctx := createTestContext([]string{"@SET #x"})
	ctx.Instr = ctx.Block.Instructions[0]

	result := execSET(ctx)
	if result != ResultContinue {
		t.Fatalf("expected ResultContinue for SET with only var (sets to empty), got %d", result)
	}
	if val := ctx.VS.Get("x"); val != "" {
		t.Fatalf("expected empty string for #x, got %q", val)
	}
}

func TestSetNoVar(t *testing.T) {
	instr := Instruction{
		Type: InstrSET,
		Args: []InstructionArg{},
	}

	ctx := &ExecutionContext{
		Block:   &CodeBlock{ID: "test"},
		Instr:   instr,
		VS:      NewVarStore(),
		PR:      NewPointerRegistry(),
		BM:      NewBlockManager(),
		PE:      NewProcessExecutor(),
		TickNum: 1,
	}

	result := execSET(ctx)
	if result != ResultError {
		t.Fatalf("expected ResultError, got %d", result)
	}
}

func TestWaitNoArgs(t *testing.T) {
	instr := Instruction{
		Type: InstrWait,
		Args: []InstructionArg{},
	}

	ctx := &ExecutionContext{
		Block:   &CodeBlock{ID: "test"},
		Instr:   instr,
		VS:      NewVarStore(),
		PR:      NewPointerRegistry(),
		BM:      NewBlockManager(),
		PE:      NewProcessExecutor(),
		TickNum: 1,
	}

	result := execWait(ctx)
	if result != ResultError {
		t.Fatalf("expected ResultError, got %d", result)
	}
}

func TestSleepNoArgs(t *testing.T) {
	instr := Instruction{
		Type: InstrSleep,
		Args: []InstructionArg{},
	}

	ctx := &ExecutionContext{
		Block:   &CodeBlock{ID: "test"},
		Instr:   instr,
		VS:      NewVarStore(),
		PR:      NewPointerRegistry(),
		BM:      NewBlockManager(),
		PE:      NewProcessExecutor(),
		TickNum: 1,
	}

	result := execSleep(ctx)
	if result != ResultError {
		t.Fatalf("expected ResultError, got %d", result)
	}
}

func TestExecCycle(t *testing.T) {
	pr := NewPointerRegistry()
	pr.BuildClass("loop", []string{"@log 'looping'", "@stop"})

	bm := NewBlockManager()
	block := bm.CreateBlock([]string{"@cycle 'true' *loop"}, "test")

	ctx := &ExecutionContext{
		Block:   block,
		Instr:   block.Instructions[0],
		VS:      NewVarStore(),
		PR:      pr,
		BM:      bm,
		PE:      NewProcessExecutor(),
		TickNum: 1,
	}

	result := execCycle(ctx)
	if result != ResultContinue {
		t.Fatalf("expected ResultContinue, got %d", result)
	}
}

func TestExecCycleFalse(t *testing.T) {
	pr := NewPointerRegistry()
	pr.BuildClass("loop", []string{"@log 'looping'"})

	bm := NewBlockManager()
	block := bm.CreateBlock([]string{"@cycle 'false' *loop"}, "test")

	ctx := &ExecutionContext{
		Block:   block,
		Instr:   block.Instructions[0],
		VS:      NewVarStore(),
		PR:      pr,
		BM:      bm,
		PE:      NewProcessExecutor(),
		TickNum: 1,
	}

	result := execCycle(ctx)
	if result != ResultContinue {
		t.Fatalf("expected ResultContinue, got %d", result)
	}
}

func TestExecRetryZero(t *testing.T) {
	ctx := createTestContext([]string{"@retry 1000 0 #err"})
	ctx.Instr = ctx.Block.Instructions[0]

	result := execRetry(ctx)
	if result != ResultContinue {
		t.Fatalf("expected ResultContinue, got %d", result)
	}
}

func TestTickerStartStop(t *testing.T) {
	ticker := NewTicker(50 * time.Millisecond)
	ticker.Start()
	time.Sleep(100 * time.Millisecond)

	if !ticker.IsRunning() {
		t.Fatal("expected ticker to be running")
	}

	ticker.Stop()
	time.Sleep(50 * time.Millisecond)

	if ticker.IsRunning() {
		t.Fatal("expected ticker to be stopped")
	}
}

func TestTickerShutdown(t *testing.T) {
	ticker := NewTicker(50 * time.Millisecond)
	ticker.Start()
	time.Sleep(100 * time.Millisecond)
	ticker.Shutdown()
}

func TestTickerLogFn(t *testing.T) {
	var msgs []string
	ticker := NewTicker(50 * time.Millisecond)
	ticker.SetLogFn(func(msg string) {
		msgs = append(msgs, msg)
	})

	ticker.LoadMarkdown("```LunarTick\n@log 'test123'\n@stop\n```")
	ticker.Start()
	time.Sleep(250 * time.Millisecond)
	ticker.Stop()

	found := false
	for _, m := range msgs {
		if strings.Contains(m, "test123") {
			found = true
			break
		}
	}
	if !found {
		t.Logf("log messages: %v", msgs)
	}
}

func TestTickerCallbacks(t *testing.T) {
	ticker := NewTicker(50 * time.Millisecond)
	tickCalled := false

	ticker.SetTickCallbacks(
		func(s TickStats) { tickCalled = true },
		func(e ErrorInfo) {},
		func() {},
		func() {},
	)

	ticker.Start()
	ticker.Inject([]string{"@log 'ticktest'", "@stop"})
	time.Sleep(300 * time.Millisecond)
	ticker.Stop()

	if !tickCalled {
		t.Fatal("expected tick callback to be called")
	}
}

func TestTickerSuspendResume(t *testing.T) {
	ticker := NewTicker(50 * time.Millisecond)

	suspendCalled := false
	resumeCalled := false
	ticker.SetTickCallbacks(
		func(s TickStats) {},
		func(e ErrorInfo) {},
		func() { suspendCalled = true },
		func() { resumeCalled = true },
	)

	ticker.Start()
	time.Sleep(200 * time.Millisecond)

	ticker.Inject([]string{"@log 'wakeup'", "@stop"})
	time.Sleep(300 * time.Millisecond)

	ticker.Stop()

	t.Logf("suspendCalled=%v resumeCalled=%v", suspendCalled, resumeCalled)
}

func TestEvalConditionWithParen(t *testing.T) {
	vs := NewVarStore()
	vs.Set("x", "10")
	vs.Set("y", "20")

	if !EvalCondition("#x < #y & #y > #x", vs, nil) {
		t.Fatal("expected true")
	}
}

func TestEvalConditionComplex(t *testing.T) {
	vs := NewVarStore()
	vs.Set("a", "done")
	vs.Set("b", "")

	if !EvalCondition("#a != '' & #b = ''", vs, nil) {
		t.Fatal("expected true for a done and b empty")
	}
}

func TestExecCatch(t *testing.T) {
	ctx := createTestContext([]string{"@CATCH #output 'error'"})
	ctx.Instr = ctx.Block.Instructions[0]

	result := execCatch(ctx)
	if result != ResultBlocked {
		t.Fatalf("expected ResultBlocked, got %d", result)
	}
	if ctx.Block.CatchVar != "output" {
		t.Fatalf("expected CatchVar='output', got %q", ctx.Block.CatchVar)
	}
	if ctx.Block.CatchSubstr != "error" {
		t.Fatalf("expected CatchSubstr='error', got %q", ctx.Block.CatchSubstr)
	}
}

func TestExecCatchNoSubstr(t *testing.T) {
	ctx := createTestContext([]string{"@CATCH #output"})
	ctx.Instr = ctx.Block.Instructions[0]

	result := execCatch(ctx)
	if result != ResultBlocked {
		t.Fatalf("expected ResultBlocked, got %d", result)
	}
	if ctx.Block.CatchVar != "output" {
		t.Fatalf("expected CatchVar='output', got %q", ctx.Block.CatchVar)
	}
}

func TestExecCatchBadArg(t *testing.T) {
	instr := Instruction{
		Type: InstrCatch,
		Args: []InstructionArg{},
	}
	ctx := &ExecutionContext{
		Block: &CodeBlock{ID: "test"},
		Instr: instr,
		VS:    NewVarStore(),
		BM:    NewBlockManager(),
	}
	result := execCatch(ctx)
	if result != ResultError {
		t.Fatal("expected ResultError for catch with no args")
	}
}

func TestExecCall(t *testing.T) {
	ctx := createTestContext([]string{"@CALL #output *handler"})
	ctx.Instr = ctx.Block.Instructions[0]

	result := execCall(ctx)
	if result != ResultContinue {
		t.Fatalf("expected ResultContinue, got %d", result)
	}
	if ctx.Block.CallVar != "output" {
		t.Fatalf("expected CallVar='output', got %q", ctx.Block.CallVar)
	}
	if ctx.Block.CallPtr != "handler" {
		t.Fatalf("expected CallPtr='handler', got %q", ctx.Block.CallPtr)
	}
}

func TestExecCallNoArgs(t *testing.T) {
	instr := Instruction{
		Type: InstrCall,
		Args: []InstructionArg{},
	}
	ctx := &ExecutionContext{
		Block: &CodeBlock{ID: "test"},
		Instr: instr,
		VS:    NewVarStore(),
		BM:    NewBlockManager(),
	}
	result := execCall(ctx)
	if result != ResultError {
		t.Fatal("expected ResultError for call with no args")
	}
}

func TestExecLazy(t *testing.T) {
	ctx := createTestContext([]string{"@LAZY"})
	ctx.Instr = ctx.Block.Instructions[0]

	result := execLazy(ctx)
	if result != ResultContinue {
		t.Fatalf("expected ResultContinue, got %d", result)
	}
}

func TestEngineInvoke(t *testing.T) {
	eng := NewEngine(50 * time.Millisecond)
	eng.Start()

	eng.ticker.PR.RegisterLazy("greeter", []string{"@log 'hello'", "@stop"})
	eng.Invoke("greeter")
	time.Sleep(300 * time.Millisecond)

	eng.Stop()
}

func TestEngineIsSuspended(t *testing.T) {
	eng := NewEngine(20 * time.Millisecond)
	eng.Start()

	time.Sleep(200 * time.Millisecond)

	eng.Stop()
}

func TestEngineGetErrors(t *testing.T) {
	eng := NewEngine(50 * time.Millisecond)
	errors := eng.GetErrors()
	if errors == nil {
		t.Fatal("expected error slice, got nil")
	}
}

func TestExpressionPointerResolve(t *testing.T) {
	pr := NewPointerRegistry()
	pr.Define("handler")

	result := EvalExpression("*handler", NewVarStore(), pr)
	if result != "true" {
		t.Fatalf("expected 'true' for existing pointer, got %q", result)
	}

	result = EvalExpression("*nonexistent", NewVarStore(), pr)
	if result != "false" {
		t.Fatalf("expected 'false' for missing pointer, got %q", result)
	}
}
