package engine

import (
	"testing"
)

func TestParseSET(t *testing.T) {
	instr := ParseInstruction("@SET #x 'hello'")
	if instr.Type != InstrSET {
		t.Fatalf("expected SET, got %s", instr.Type)
	}
	if len(instr.Args) != 2 {
		t.Fatalf("expected 2 args, got %d", len(instr.Args))
	}
	if instr.Args[0].Type != ArgVariable || instr.Args[0].Value != "x" {
		t.Fatal("first arg should be variable x")
	}
	if instr.Args[1].Type != ArgLiteral || instr.Args[1].Value != "hello" {
		t.Fatal("second arg should be literal 'hello'")
	}
}

func TestParseADD(t *testing.T) {
	instr := ParseInstruction("@ADD #y ' world'")
	if instr.Type != InstrADD {
		t.Fatalf("expected ADD, got %s", instr.Type)
	}
}

func TestParseWRT(t *testing.T) {
	instr := ParseInstruction("@WRT #secret 'val'")
	if instr.Type != InstrWRT {
		t.Fatalf("expected WRT, got %s", instr.Type)
	}
}

func TestParseRON(t *testing.T) {
	instr := ParseInstruction("@RON #const 'val'")
	if instr.Type != InstrRON {
		t.Fatalf("expected RON, got %s", instr.Type)
	}
}

func TestParseUNL(t *testing.T) {
	instr := ParseInstruction("@UNL #var 'extra'")
	if instr.Type != InstrUNL {
		t.Fatalf("expected UNL, got %s", instr.Type)
	}
}

func TestParseRun(t *testing.T) {
	instr := ParseInstruction("@run './app.exe' '--arg' 'value'")
	if instr.Type != InstrRun {
		t.Fatalf("expected RUN, got %s", instr.Type)
	}
	if len(instr.Args) != 3 {
		t.Fatalf("expected 3 args, got %d", len(instr.Args))
	}
	if instr.Args[0].Value != "./app.exe" {
		t.Fatalf("expected './app.exe', got '%s'", instr.Args[0].Value)
	}
}

func TestParseCatch(t *testing.T) {
	instr := ParseInstruction("@catch #found 'READY'")
	if instr.Type != InstrCatch {
		t.Fatalf("expected CATCH, got %s", instr.Type)
	}
	if instr.Args[0].Type != ArgVariable || instr.Args[0].Value != "found" {
		t.Fatal("first arg should be variable found")
	}
}

func TestParseWait(t *testing.T) {
	instr := ParseInstruction("@wait #done")
	if instr.Type != InstrWait {
		t.Fatalf("expected WAIT, got %s", instr.Type)
	}
	if instr.Args[0].Value != "done" {
		t.Fatalf("expected 'done', got '%s'", instr.Args[0].Value)
	}
}

func TestParseSleep(t *testing.T) {
	instr := ParseInstruction("@sleep 5000")
	if instr.Type != InstrSleep {
		t.Fatalf("expected SLEEP, got %s", instr.Type)
	}
	if instr.Args[0].Value != "5000" {
		t.Fatalf("expected '5000', got '%s'", instr.Args[0].Value)
	}
}

func TestParseIf(t *testing.T) {
	instr := ParseInstruction("@if 'true' ? *A : *B")
	if instr.Type != InstrIf {
		t.Fatalf("expected IF, got %s", instr.Type)
	}
	if len(instr.Args) != 3 {
		t.Fatalf("expected 3 args, got %d", len(instr.Args))
	}
}

func TestParseCycle(t *testing.T) {
	instr := ParseInstruction("@cycle '#x = \"\"' *poll")
	if instr.Type != InstrCycle {
		t.Fatalf("expected CYCLE, got %s", instr.Type)
	}
}

func TestParseLog(t *testing.T) {
	instr := ParseInstruction("@log 'Hello World'")
	if instr.Type != InstrLog {
		t.Fatalf("expected LOG, got %s", instr.Type)
	}
	if instr.Args[0].Value != "Hello World" {
		t.Fatalf("expected 'Hello World', got '%s'", instr.Args[0].Value)
	}
}

func TestParseStop(t *testing.T) {
	instr := ParseInstruction("@stop")
	if instr.Type != InstrStop {
		t.Fatalf("expected STOP, got %s", instr.Type)
	}
}

func TestParseDef(t *testing.T) {
	instr := ParseInstruction("@def *myPtr")
	if instr.Type != InstrDef {
		t.Fatalf("expected DEF, got %s", instr.Type)
	}
	if instr.Args[0].Value != "myPtr" {
		t.Fatalf("expected 'myPtr', got '%s'", instr.Args[0].Value)
	}
}

func TestParseLazy(t *testing.T) {
	instr := ParseInstruction("@lazy *myFunc")
	if instr.Type != InstrLazy {
		t.Fatalf("expected LAZY, got %s", instr.Type)
	}
}

func TestParseBuild(t *testing.T) {
	instr := ParseInstruction("@build *ptr class")
	if instr.Type != InstrBuild {
		t.Fatalf("expected BUILD, got %s", instr.Type)
	}
	if instr.Args[0].Value != "ptr" {
		t.Fatalf("expected 'ptr', got '%s'", instr.Args[0].Value)
	}
	if instr.Args[1].Value != "class" {
		t.Fatalf("expected 'class', got '%s'", instr.Args[1].Value)
	}
}

func TestParsePtrCall(t *testing.T) {
	instr := ParseInstruction("*backupTask")
	if instr.Type != InstrPtr {
		t.Fatalf("expected PTR, got %s", instr.Type)
	}
	if instr.Args[0].Value != "backupTask" {
		t.Fatalf("expected 'backupTask', got '%s'", instr.Args[0].Value)
	}
}

func TestParseComment(t *testing.T) {
	result := ParseLines([]string{"// this is a comment", "@log 'hello'"})
	if len(result) != 1 {
		t.Fatalf("expected 1 instruction, got %d", len(result))
	}
}

func TestParseEmptyLine(t *testing.T) {
	result := ParseLines([]string{"", "  ", "@log 'hello'"})
	if len(result) != 1 {
		t.Fatalf("expected 1 instruction, got %d", len(result))
	}
}

func TestParseLowercase(t *testing.T) {
	instr := ParseInstruction("@set #x 'hello'")
	if instr.Type != InstrSET {
		t.Fatalf("expected SET for lowercase, got %s", instr.Type)
	}
}

func TestParseRetry(t *testing.T) {
	instr := ParseInstruction("@retry 1000 3 #errVar")
	if instr.Type != InstrRetry {
		t.Fatalf("expected RETRY, got %s", instr.Type)
	}
	if len(instr.Args) != 3 {
		t.Fatalf("expected 3 args, got %d", len(instr.Args))
	}
}
