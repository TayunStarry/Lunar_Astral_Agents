package engine

import (
	"testing"
)

func TestBlockCreateAndLifecycle(t *testing.T) {
	bm := NewBlockManager()

	block := bm.CreateBlock([]string{"@log 'hello'", "@stop"}, "")
	if block == nil {
		t.Fatal("expected block to be created")
	}
	if block.Status != StatusReady {
		t.Fatalf("expected Ready status, got %s", block.Status)
	}
	if len(block.Instructions) != 2 {
		t.Fatalf("expected 2 instructions, got %d", len(block.Instructions))
	}
}

func TestBlockMoveToWaiting(t *testing.T) {
	bm := NewBlockManager()

	block := bm.CreateBlock([]string{"@wait #x", "@stop"}, "")
	bm.MoveToWaiting(block.ID, WaitCondition{Type: WaitVariable, Var: "x"})

	b := bm.GetBlock(block.ID)
	if b.Status != StatusWaiting {
		t.Fatalf("expected Waiting status, got %s", b.Status)
	}
	if bm.ReadyCount() != 0 {
		t.Fatalf("expected 0 ready blocks, got %d", bm.ReadyCount())
	}
	if bm.WaitCount() != 1 {
		t.Fatalf("expected 1 waiting block, got %d", bm.WaitCount())
	}
}

func TestBlockMoveToReady(t *testing.T) {
	bm := NewBlockManager()

	block := bm.CreateBlock([]string{"@wait #x", "@stop"}, "")
	bm.MoveToWaiting(block.ID, WaitCondition{Type: WaitVariable, Var: "x"})
	bm.MoveToReady(block.ID)

	b := bm.GetBlock(block.ID)
	if b.Status != StatusReady {
		t.Fatalf("expected Ready status, got %s", b.Status)
	}
}

func TestBlockTerminate(t *testing.T) {
	bm := NewBlockManager()

	block := bm.CreateBlock([]string{"@log 'hello'", "@stop"}, "")
	bm.Terminate(block.ID)

	b := bm.GetBlock(block.ID)
	if b.Status != StatusTerminated {
		t.Fatalf("expected Terminated status, got %s", b.Status)
	}
}

func TestBlockFlushPending(t *testing.T) {
	bm := NewBlockManager()

	pending := bm.CreateBlock([]string{"@log 'pending'"}, "")
	bm.AddPending(pending)

	if bm.ReadyCount() != 0 {
		t.Fatal("pending blocks should not be in ready list")
	}

	flushed := bm.FlushPending()
	if len(flushed) != 1 {
		t.Fatalf("expected 1 flushed block, got %d", len(flushed))
	}
	if bm.ReadyCount() != 1 {
		t.Fatalf("expected 1 ready block after flush, got %d", bm.ReadyCount())
	}
}

func TestBlockTerminateAll(t *testing.T) {
	bm := NewBlockManager()

	b1 := bm.CreateBlock([]string{"@log '1'"}, "")
	b2 := bm.CreateBlock([]string{"@log '2'"}, "")
	b3 := bm.CreateBlock([]string{"@log '3'"}, "")
	bm.AddReady(b1.ID)
	bm.AddReady(b2.ID)
	bm.AddReady(b3.ID)

	if bm.ReadyCount() != 3 {
		t.Fatalf("expected 3 ready blocks, got %d", bm.ReadyCount())
	}

	bm.TerminateAll()

	if bm.ActiveCount() != 0 {
		t.Fatalf("expected 0 active blocks, got %d", bm.ActiveCount())
	}
}

func TestBlockAdvancePC(t *testing.T) {
	bm := NewBlockManager()

	block := bm.CreateBlock([]string{"@log 'first'", "@log 'second'", "@stop"}, "")
	if block.PC != 0 {
		t.Fatalf("expected PC 0, got %d", block.PC)
	}

	bm.AdvancePC(block.ID)
	if bm.GetBlock(block.ID).PC != 1 {
		t.Fatalf("expected PC 1, got %d", bm.GetBlock(block.ID).PC)
	}
}

func TestBlockHasMoreInstructions(t *testing.T) {
	bm := NewBlockManager()

	block := bm.CreateBlock([]string{"@log 'first'", "@stop"}, "")

	if !bm.HasMoreInstructions(block.ID) {
		t.Fatal("expected more instructions at PC 0")
	}

	bm.AdvancePC(block.ID)
	if !bm.HasMoreInstructions(block.ID) {
		t.Fatal("expected more instructions at PC 1")
	}

	bm.AdvancePC(block.ID)
	if bm.HasMoreInstructions(block.ID) {
		t.Fatal("expected no more instructions at PC 2")
	}
}

func TestParseLinesSkipsComments(t *testing.T) {
	result := ParseLines([]string{
		"// comment line",
		"",
		"  ",
		"@log 'hello'",
		"// another comment",
		"@stop",
	})

	if len(result) != 2 {
		t.Fatalf("expected 2 instructions, got %d", len(result))
	}
}
