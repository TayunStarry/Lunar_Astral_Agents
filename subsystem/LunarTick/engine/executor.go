package engine

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os/exec"
	"sync"
	"time"
)

type ProcessResult struct {
	Stdout   string
	Stderr   string
	ExitCode int
	Error    error
	Duration time.Duration
}

type RunningProcess struct {
	Cmd      *exec.Cmd
	Result   *ProcessResult
	Ctx      context.Context
	Cancel   context.CancelFunc
	Done     chan struct{}
	LineChan chan string
	Stdout   io.ReadCloser
	Stderr   io.ReadCloser
	mu       sync.Mutex
}

type ProcessExecutor struct {
	processes map[string]*RunningProcess
	mu        sync.RWMutex
}

func NewProcessExecutor() *ProcessExecutor {
	return &ProcessExecutor{
		processes: make(map[string]*RunningProcess),
	}
}

func (pe *ProcessExecutor) Start(blockID, path string, args []string) (*RunningProcess, error) {
	pe.mu.Lock()
	defer pe.mu.Unlock()

	ctx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(ctx, path, args...)

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("stdout pipe: %w", err)
	}

	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("stderr pipe: %w", err)
	}

	rp := &RunningProcess{
		Cmd:      cmd,
		Result:   &ProcessResult{ExitCode: -1},
		Ctx:      ctx,
		Cancel:   cancel,
		Done:     make(chan struct{}),
		LineChan: make(chan string, 256),
		Stdout:   stdoutPipe,
		Stderr:   stderrPipe,
	}

	if err := cmd.Start(); err != nil {
		cancel()
		return nil, fmt.Errorf("start process: %w", err)
	}

	pe.processes[blockID] = rp

	go pe.collectOutput(rp)
	go pe.waitProcess(rp)

	return rp, nil
}

func (pe *ProcessExecutor) collectOutput(rp *RunningProcess) {
	combined := io.MultiReader(rp.Stdout, rp.Stderr)
	scanner := bufio.NewScanner(combined)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)

	for scanner.Scan() {
		line := scanner.Text()
		rp.mu.Lock()
		rp.Result.Stdout += line + "\n"
		rp.mu.Unlock()

		select {
		case rp.LineChan <- line:
		default:
		}
	}
}

func (pe *ProcessExecutor) waitProcess(rp *RunningProcess) {
	defer close(rp.Done)

	err := rp.Cmd.Wait()
	rp.mu.Lock()
	defer rp.mu.Unlock()

	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			rp.Result.ExitCode = exitErr.ExitCode()
		} else {
			rp.Result.Error = err
		}
	} else {
		rp.Result.ExitCode = 0
	}
}

func (pe *ProcessExecutor) Kill(blockID string) error {
	pe.mu.Lock()
	defer pe.mu.Unlock()

	rp, exists := pe.processes[blockID]
	if !exists {
		return fmt.Errorf("process not found: %s", blockID)
	}

	rp.Cancel()
	return nil
}

func (pe *ProcessExecutor) GetResult(blockID string) *ProcessResult {
	pe.mu.RLock()
	rp, exists := pe.processes[blockID]
	pe.mu.RUnlock()

	if !exists {
		return nil
	}

	rp.mu.Lock()
	defer rp.mu.Unlock()
	return rp.Result
}

func (pe *ProcessExecutor) IsDone(blockID string) bool {
	pe.mu.RLock()
	rp, exists := pe.processes[blockID]
	pe.mu.RUnlock()

	if !exists {
		return true
	}

	select {
	case <-rp.Done:
		return true
	default:
		return false
	}
}

func (pe *ProcessExecutor) ReadLine(blockID string) (string, bool) {
	pe.mu.RLock()
	rp, exists := pe.processes[blockID]
	pe.mu.RUnlock()

	if !exists {
		return "", false
	}

	select {
	case line, ok := <-rp.LineChan:
		return line, ok
	default:
		return "", false
	}
}

func (pe *ProcessExecutor) WaitDone(blockID string) <-chan struct{} {
	pe.mu.RLock()
	rp, exists := pe.processes[blockID]
	pe.mu.RUnlock()

	if !exists {
		ch := make(chan struct{})
		close(ch)
		return ch
	}

	return rp.Done
}

func (pe *ProcessExecutor) Remove(blockID string) {
	pe.mu.Lock()
	defer pe.mu.Unlock()

	if rp, exists := pe.processes[blockID]; exists {
		rp.Cancel()
	}
	delete(pe.processes, blockID)
}

func (pe *ProcessExecutor) Count() int {
	pe.mu.RLock()
	defer pe.mu.RUnlock()
	return len(pe.processes)
}

func (pe *ProcessExecutor) Shutdown() {
	pe.mu.Lock()
	defer pe.mu.Unlock()

	for id, rp := range pe.processes {
		rp.Cancel()
		delete(pe.processes, id)
	}
}