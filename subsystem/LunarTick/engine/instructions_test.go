package engine

import (
	"strings"
	"testing"
)

func createTestContext(lines []string) *ExecutionContext {
	bm := NewBlockManager()
	block := bm.CreateBlock(lines, "test")

	return &ExecutionContext{
		Block:   block,
		Instr:   block.Instructions[0],
		VS:      NewVarStore(),
		PR:      NewPointerRegistry(),
		BM:      bm,
		PE:      NewProcessExecutor(),
		TickNum: 1,
	}
}

func TestExecSET(t *testing.T) {
	ctx := createTestContext([]string{"@SET #x 'hello'"})
	ctx.Instr = ctx.Block.Instructions[0]

	result := execSET(ctx)
	if result != ResultContinue {
		t.Fatalf("expected ResultContinue, got %d", result)
	}
	if val := ctx.VS.Get("x"); val != "hello" {
		t.Fatalf("expected 'hello', got '%s'", val)
	}
}

func TestExecADD(t *testing.T) {
	ctx := createTestContext([]string{"@ADD #msg ' world'"})
	ctx.Instr = ctx.Block.Instructions[0]
	ctx.VS.Set("msg", "hello")

	result := execADD(ctx)
	if result != ResultContinue {
		t.Fatalf("expected ResultContinue, got %d", result)
	}
	if val := ctx.VS.Get("msg"); val != "hello world" {
		t.Fatalf("expected 'hello world', got '%s'", val)
	}
}

func TestExecWRT(t *testing.T) {
	ctx := createTestContext([]string{"@WRT #secret 'value'"})
	ctx.Instr = ctx.Block.Instructions[0]

	result := execWRT(ctx)
	if result != ResultContinue {
		t.Fatalf("expected ResultContinue, got %d", result)
	}
	if val := ctx.VS.Get("secret"); val != "" {
		t.Fatalf("writeonly var should return empty, got '%s'", val)
	}
}

func TestExecRON(t *testing.T) {
	ctx := createTestContext([]string{"@RON #const '100'"})
	ctx.Instr = ctx.Block.Instructions[0]

	result := execRON(ctx)
	if result != ResultContinue {
		t.Fatalf("expected ResultContinue, got %d", result)
	}

	ctx.VS.Set("const", "200")
	if val := ctx.VS.Get("const"); val != "100" {
		t.Fatalf("readonly var should not change, got '%s'", val)
	}
}

func TestExecUNL(t *testing.T) {
	ctx := createTestContext([]string{"@UNL #var 'extra'"})
	ctx.Instr = ctx.Block.Instructions[0]
	ctx.VS.Set("var", "base")
	ctx.VS.SetMode("var", ModeReadOnly)

	result := execUNL(ctx)
	if result != ResultContinue {
		t.Fatalf("expected ResultContinue, got %d", result)
	}
	if val := ctx.VS.Get("var"); val != "baseextra" {
		t.Fatalf("expected 'baseextra', got '%s'", val)
	}

	ctx.VS.Set("var", "modified")
	if val := ctx.VS.Get("var"); val != "modified" {
		t.Fatalf("expected 'modified', got '%s'", val)
	}
}

func TestExecWaitSatisfied(t *testing.T) {
	ctx := createTestContext([]string{"@wait #x"})
	ctx.Instr = ctx.Block.Instructions[0]
	ctx.VS.Set("x", "ready")

	result := execWait(ctx)
	if result != ResultContinue {
		t.Fatalf("expected ResultContinue when var is set, got %d", result)
	}
}

func TestExecWaitBlocked(t *testing.T) {
	ctx := createTestContext([]string{"@wait #x"})
	ctx.Instr = ctx.Block.Instructions[0]

	result := execWait(ctx)
	if result != ResultBlocked {
		t.Fatalf("expected ResultBlocked when var is empty, got %d", result)
	}
}

func TestExecSleep(t *testing.T) {
	ctx := createTestContext([]string{"@sleep 10"})
	ctx.Instr = ctx.Block.Instructions[0]

	result := execSleep(ctx)
	if result != ResultBlocked {
		t.Fatalf("expected ResultBlocked, got %d", result)
	}
}

func TestExecFilter(t *testing.T) {
	ctx := createTestContext([]string{`@filter #html 'href="([^"]+)"'`})
	ctx.Instr = ctx.Block.Instructions[0]
	ctx.VS.Set("html", `<a href="http://example.com">link</a>`)

	result := execFilter(ctx)
	if result != ResultContinue {
		t.Fatalf("expected ResultContinue, got %d", result)
	}
	if val := ctx.VS.Get("html"); val != "http://example.com" {
		t.Fatalf("expected 'http://example.com', got '%s'", val)
	}
}

func TestExecMath(t *testing.T) {
	ctx := createTestContext([]string{"@math #result '#x + 1'"})
	ctx.Instr = ctx.Block.Instructions[0]
	ctx.VS.Set("x", "5")

	result := execMath(ctx)
	if result != ResultContinue {
		t.Fatalf("expected ResultContinue, got %d", result)
	}
	got := ctx.VS.Get("result")
	if got != "6" {
		t.Fatalf("expected '6', got '%s'", got)
	}
}

func TestExecIf(t *testing.T) {
	pr := NewPointerRegistry()
	pr.BuildClass("yes", []string{"@log 'yes'"})
	pr.BuildClass("no", []string{"@log 'no'"})

	bm := NewBlockManager()
	block := bm.CreateBlock([]string{"@if '1 = 1' ? *yes : *no"}, "test")

	ctx := &ExecutionContext{
		Block:   block,
		Instr:   block.Instructions[0],
		VS:      NewVarStore(),
		PR:      pr,
		BM:      bm,
		PE:      NewProcessExecutor(),
		TickNum: 1,
	}

	result := execIf(ctx)
	if result != ResultContinue {
		t.Fatalf("expected ResultContinue, got %d", result)
	}
}

func TestExecLog(t *testing.T) {
	var logged []string
	ctx := createTestContext([]string{"@log 'test message'"})
	ctx.Instr = ctx.Block.Instructions[0]
	ctx.LogFn = func(msg string) {
		logged = append(logged, msg)
	}

	result := execLog(ctx)
	if result != ResultContinue {
		t.Fatalf("expected ResultContinue, got %d", result)
	}
	if len(logged) != 1 || !strings.Contains(logged[0], "test message") {
		t.Fatalf("expected log message, got %v", logged)
	}
}

func TestExecStop(t *testing.T) {
	ctx := createTestContext([]string{"@stop"})
	ctx.Instr = ctx.Block.Instructions[0]

	result := execStop(ctx)
	if result != ResultTerminated {
		t.Fatalf("expected ResultTerminated, got %d", result)
	}
}

func TestExecDef(t *testing.T) {
	pr := NewPointerRegistry()
	bm := NewBlockManager()
	block := bm.CreateBlock([]string{"@def *myPtr"}, "test")

	ctx := &ExecutionContext{
		Block:   block,
		Instr:   block.Instructions[0],
		VS:      NewVarStore(),
		PR:      pr,
		BM:      bm,
		PE:      NewProcessExecutor(),
		TickNum: 1,
	}

	result := execDef(ctx)
	if result != ResultContinue {
		t.Fatalf("expected ResultContinue, got %d", result)
	}
	if !pr.Exists("myPtr") {
		t.Fatal("expected pointer to exist after @def")
	}
}

func TestExecBuildClass(t *testing.T) {
	pr := NewPointerRegistry()
	bm := NewBlockManager()
	block := bm.CreateBlock([]string{"@build *myClass class", "@log 'in class'"}, "test")

	ctx := &ExecutionContext{
		Block:   block,
		Instr:   block.Instructions[0],
		VS:      NewVarStore(),
		PR:      pr,
		BM:      bm,
		PE:      NewProcessExecutor(),
		TickNum: 42,
	}

	result := execBuild(ctx)
	if result != ResultContinue {
		t.Fatalf("expected ResultContinue, got %d", result)
	}
	if !pr.IsClass("myClass") {
		t.Fatal("expected class pointer")
	}
}

func TestExecPtrClassCall(t *testing.T) {
	pr := NewPointerRegistry()
	pr.BuildClass("greet", []string{"@log 'hello from pointer'", "@stop"})

	bm := NewBlockManager()
	block := bm.CreateBlock([]string{"*greet"}, "test")

	ctx := &ExecutionContext{
		Block:   block,
		Instr:   block.Instructions[0],
		VS:      NewVarStore(),
		PR:      pr,
		BM:      bm,
		PE:      NewProcessExecutor(),
		TickNum: 1,
	}

	result := execPtr(ctx)
	if result != ResultContinue {
		t.Fatalf("expected ResultContinue, got %d", result)
	}
}

func TestExecWrite(t *testing.T) {
	ctx := createTestContext([]string{"@write #path #content"})
	ctx.Instr = ctx.Block.Instructions[0]
	ctx.VS.Set("path", t.TempDir()+"/test_write.txt")
	ctx.VS.Set("content", "test content")

	result := execWrite(ctx)
	if result != ResultContinue {
		t.Fatalf("expected ResultContinue, got %d", result)
	}
}

func TestExecRead(t *testing.T) {
	tmpDir := t.TempDir()
	path := tmpDir + "/test_read.txt"

	ctx := createTestContext([]string{"@read #path #target"})
	ctx.Block.Instructions[0].Args[0] = InstructionArg{Type: ArgLiteral, Value: path}
	ctx.Block.Instructions[0].Args[1] = InstructionArg{Type: ArgVariable, Value: "target"}
	ctx.Instr = ctx.Block.Instructions[0]

	result := execRead(ctx)
	if result != ResultContinue {
		t.Fatalf("expected ResultContinue, got %d", result)
	}
}

func TestExecLimit(t *testing.T) {
	ctx := createTestContext([]string{"@limit 5000"})
	ctx.Instr = ctx.Block.Instructions[0]

	result := execLimit(ctx)
	if result != ResultContinue {
		t.Fatalf("expected ResultContinue, got %d", result)
	}
	if ctx.Block.LimitMS == 0 {
		t.Fatal("expected limit to be set")
	}
}

func TestExecuteInstruction(t *testing.T) {
	ctx := createTestContext([]string{"@SET #x '42'"})
	ctx.Instr = ctx.Block.Instructions[0]

	result := ExecuteInstruction(ctx)
	if result != ResultContinue {
		t.Fatalf("expected ResultContinue, got %d", result)
	}
	if val := ctx.VS.Get("x"); val != "42" {
		t.Fatalf("expected '42', got '%s'", val)
	}
}

func TestExecRetry(t *testing.T) {
	ctx := createTestContext([]string{"@retry 1000 3 #errVar"})
	ctx.Instr = ctx.Block.Instructions[0]

	result := execRetry(ctx)
	if result != ResultContinue {
		t.Fatalf("expected ResultContinue, got %d", result)
	}
	if ctx.Block.RetryMax != 3 {
		t.Fatalf("expected RetryMax=3, got %d", ctx.Block.RetryMax)
	}
	if ctx.Block.RetryErrVar != "errVar" {
		t.Fatalf("expected RetryErrVar='errVar', got '%s'", ctx.Block.RetryErrVar)
	}
}