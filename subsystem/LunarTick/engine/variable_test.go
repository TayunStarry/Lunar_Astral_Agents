package engine

import (
	"testing"
)

func TestVarStoreSetGet(t *testing.T) {
	vs := NewVarStore()

	vs.Set("name", "hello")
	if val := vs.Get("name"); val != "hello" {
		t.Fatalf("expected 'hello', got '%s'", val)
	}

	if val := vs.Get("nonexistent"); val != "" {
		t.Fatalf("expected empty, got '%s'", val)
	}
}

func TestVarStoreAdd(t *testing.T) {
	vs := NewVarStore()

	vs.Set("msg", "hello")
	vs.Add("msg", " world")
	if val := vs.Get("msg"); val != "hello world" {
		t.Fatalf("expected 'hello world', got '%s'", val)
	}
}

func TestVarStoreModeReadOnly(t *testing.T) {
	vs := NewVarStore()

	vs.Set("x", "100")
	vs.SetMode("x", ModeReadOnly)

	vs.Set("x", "200")
	if val := vs.Get("x"); val != "100" {
		t.Fatalf("readonly var should not change, got '%s'", val)
	}
}

func TestVarStoreModeWriteOnly(t *testing.T) {
	vs := NewVarStore()

	vs.Set("secret", "pass123")
	vs.SetMode("secret", ModeWriteOnly)

	if val := vs.Get("secret"); val != "" {
		t.Fatalf("writeonly var should return empty, got '%s'", val)
	}

	vs.Set("secret", "newpass")
	if val := vs.Get("secret"); val != "" {
		t.Fatalf("writeonly var should still return empty, got '%s'", val)
	}
}

func TestVarStoreSnapshot(t *testing.T) {
	vs := NewVarStore()

	vs.Set("a", "1")
	vs.Set("b", "2")
	vs.SetMode("a", ModeReadOnly)

	snap := vs.Snapshot()

	vs.Set("a", "changed")
	vs.Set("b", "changed")
	vs.Set("c", "3")

	vs.Restore(snap)

	if val := vs.Get("a"); val != "1" {
		t.Fatalf("expected '1', got '%s'", val)
	}
	if val := vs.Get("b"); val != "2" {
		t.Fatalf("expected '2', got '%s'", val)
	}
	if val := vs.Get("c"); val != "" {
		t.Fatalf("expected empty, got '%s'", val)
	}
}

func TestVarStoreGetAll(t *testing.T) {
	vs := NewVarStore()

	vs.Set("x", "10")
	vs.Set("y", "20")
	vs.SetMode("y", ModeWriteOnly)

	all := vs.GetAll()

	if all["x"] != "10" {
		t.Fatalf("expected '10', got '%s'", all["x"])
	}
	if all["y"] != "" {
		t.Fatalf("writeonly should return empty in GetAll, got '%s'", all["y"])
	}
}

func TestVarStoreConcurrent(t *testing.T) {
	vs := NewVarStore()
	done := make(chan bool)

	for i := 0; i < 100; i++ {
		go func(n int) {
			vs.Set("counter", "hit")
			done <- true
		}(i)
	}

	for i := 0; i < 100; i++ {
		<-done
	}

	if val := vs.Get("counter"); val != "hit" {
		t.Fatalf("expected 'hit', got '%s'", val)
	}
}