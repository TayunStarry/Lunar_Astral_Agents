package engine

import (
	"testing"
	"time"
)

func TestEngineCreate(t *testing.T) {
	eng := NewEngine(100 * time.Millisecond)
	if eng == nil {
		t.Fatal("expected engine to be created")
	}
	if eng.interval != 100*time.Millisecond {
		t.Fatalf("expected 100ms interval, got %v", eng.interval)
	}
}

func TestEngineSetTickInterval(t *testing.T) {
	eng := NewEngine(100 * time.Millisecond)
	eng.SetTickInterval(50)
	if eng.interval != 50*time.Millisecond {
		t.Fatalf("expected 50ms interval, got %v", eng.interval)
	}
}

func TestEngineLoadMarkdown(t *testing.T) {
	eng := NewEngine(50 * time.Millisecond)

	mdText := "```LunarTick\n@log 'hello'\n@stop\n```"
	eng.LoadMarkdown(mdText)
}

func TestEngineLoadJSON(t *testing.T) {
	eng := NewEngine(50 * time.Millisecond)

	jsonArray := [][]string{
		{"@log 'test'", "@stop"},
	}
	eng.LoadJSON(jsonArray)
}

func TestEngineInject(t *testing.T) {
	eng := NewEngine(50 * time.Millisecond)
	eng.Inject([]string{"@SET #test 'injected'", "@stop"})
}

func TestEngineGetVariable(t *testing.T) {
	eng := NewEngine(50 * time.Millisecond)
	eng.SetVariable("test", "value")
	if val := eng.GetVariable("test"); val != "value" {
		t.Fatalf("expected 'value', got '%s'", val)
	}
}

func TestEngineSetVariable(t *testing.T) {
	eng := NewEngine(50 * time.Millisecond)
	eng.SetVariable("key", "data")
	eng.SetVariable("key2", "data2")
	all := eng.GetAllVariables()
	if all["key"] != "data" {
		t.Fatalf("expected 'data', got '%s'", all["key"])
	}
}

func TestEnginePointerExists(t *testing.T) {
	eng := NewEngine(50 * time.Millisecond)

	eng.LoadMarkdown("```LunarTick\n@def *testPtr\n```")
	if !eng.PointerExists("testPtr") {
		t.Fatal("expected pointer to exist")
	}
	if eng.PointerExists("nonexistent") {
		t.Fatal("expected pointer to not exist")
	}
}

func TestEngineLoadLazy(t *testing.T) {
	eng := NewEngine(50 * time.Millisecond)

	mdText := "```LunarTick\n@lazy *myTask\n@log 'task running'\n@stop\n```"
	eng.LoadMarkdown(mdText)

	if !eng.PointerExists("myTask") {
		t.Fatal("expected lazy pointer to be registered")
	}
}

func TestEngineStartStop(t *testing.T) {
	eng := NewEngine(50 * time.Millisecond)

	if eng.IsRunning() {
		t.Fatal("engine should not be running before Start")
	}

	eng.Start()
	time.Sleep(50 * time.Millisecond)

	if !eng.IsRunning() {
		t.Fatal("engine should be running after Start")
	}

	eng.Stop()
	time.Sleep(50 * time.Millisecond)

	if eng.IsRunning() {
		t.Fatal("engine should not be running after Stop")
	}
}

func TestEngineShutdown(t *testing.T) {
	eng := NewEngine(50 * time.Millisecond)
	eng.Start()
	time.Sleep(50 * time.Millisecond)
	eng.Shutdown()

	if eng.IsRunning() {
		t.Fatal("engine should not be running after Shutdown")
	}
}

func TestDefaultEngine(t *testing.T) {
	eng := DefaultEngine()
	if eng == nil {
		t.Fatal("expected default engine")
	}

	eng2 := DefaultEngine()
	if eng != eng2 {
		t.Fatal("expected same engine instance")
	}
}

func TestEngineStats(t *testing.T) {
	eng := NewEngine(50 * time.Millisecond)

	eng.LoadMarkdown("```LunarTick\n@log 'hello'\n@stop\n```")
	eng.Start()
	time.Sleep(200 * time.Millisecond)
	eng.Stop()

	stats := eng.GetStats()
	if stats.TickNumber == 0 {
		t.Fatal("expected some tick numbers")
	}

	recent := eng.GetRecentStats(5)
	if len(recent) == 0 {
		t.Fatal("expected some recent stats")
	}
}

func TestEngineGetPointerNames(t *testing.T) {
	eng := NewEngine(50 * time.Millisecond)

	eng.LoadMarkdown("```LunarTick\n@def *ptrA\n```\n```LunarTick\n@def *ptrB\n```")

	names := eng.GetPointerNames()
	if len(names) != 2 {
		t.Fatalf("expected 2 pointers, got %d", len(names))
	}
}

func TestEngineResume(t *testing.T) {
	eng := NewEngine(50 * time.Millisecond)
	eng.Start()

	time.Sleep(300 * time.Millisecond)

	eng.Inject([]string{"@log 'resumed'", "@stop"})
	time.Sleep(200 * time.Millisecond)

	eng.Stop()
}
