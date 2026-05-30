package engine

import (
	"testing"
)

func TestPointerDefine(t *testing.T) {
	pr := NewPointerRegistry()

	pr.Define("test")
	if !pr.Exists("test") {
		t.Fatal("expected pointer to exist after Define")
	}
	if !pr.IsClass("test") {
		t.Fatal("expected Define to create class pointer")
	}
}

func TestPointerBuildClass(t *testing.T) {
	pr := NewPointerRegistry()

	lines := []string{"@log 'hello'", "@stop"}
	pr.BuildClass("myFunc", lines)

	entry, exists := pr.Get("myFunc")
	if !exists {
		t.Fatal("expected pointer to exist")
	}
	if entry.Type != PointerClass {
		t.Fatal("expected class pointer")
	}
	if len(entry.Lines) != 2 {
		t.Fatalf("expected 2 lines, got %d", len(entry.Lines))
	}
}

func TestPointerBuildSnapshot(t *testing.T) {
	pr := NewPointerRegistry()

	vs := NewVarStore()
	vs.Set("a", "1")
	snap := vs.Snapshot()

	pr.BuildSnapshot("snap1", snap, 42)

	entry, exists := pr.Get("snap1")
	if !exists {
		t.Fatal("expected snapshot pointer to exist")
	}
	if entry.Type != PointerSnapshot {
		t.Fatal("expected snapshot pointer")
	}
	if entry.TickStamp != 42 {
		t.Fatalf("expected tick 42, got %d", entry.TickStamp)
	}
	if entry.SnapData["a"].Value != "1" {
		t.Fatalf("expected '1', got '%s'", entry.SnapData["a"].Value)
	}
}

func TestPointerRegisterLazy(t *testing.T) {
	pr := NewPointerRegistry()

	lines := []string{"@lazy *myTask", "@log 'task'", "@stop"}
	pr.RegisterLazy("myTask", lines)

	entry, exists := pr.Get("myTask")
	if !exists {
		t.Fatal("expected lazy pointer to exist")
	}
	if entry.Type != PointerClass {
		t.Fatal("expected class pointer")
	}
	if len(entry.Lines) != 3 {
		t.Fatalf("expected 3 lines, got %d", len(entry.Lines))
	}
}

func TestPointerRemove(t *testing.T) {
	pr := NewPointerRegistry()

	pr.Define("removable")
	pr.Remove("removable")

	if pr.Exists("removable") {
		t.Fatal("expected pointer to be removed")
	}
}

func TestPointerGetAllClassNames(t *testing.T) {
	pr := NewPointerRegistry()

	pr.Define("a")
	pr.BuildClass("b", []string{"@log 'b'"})
	pr.BuildSnapshot("c", nil, 0)

	names := pr.GetAllClassNames()
	if len(names) != 2 {
		t.Fatalf("expected 2 class pointers, got %d", len(names))
	}
}

func TestPointerOverwrite(t *testing.T) {
	pr := NewPointerRegistry()

	pr.Define("ptr")
	pr.BuildClass("ptr", []string{"@log 'new'"})

	entry, _ := pr.Get("ptr")
	if len(entry.Lines) != 1 {
		t.Fatalf("expected 1 line, got %d", len(entry.Lines))
	}
	if entry.Lines[0] != "@log 'new'" {
		t.Fatalf("expected '@log new', got '%s'", entry.Lines[0])
	}
}