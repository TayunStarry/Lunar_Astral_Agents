package app

// library_creation.go 资源库创作入口的内存任务表。
//
// 资源库提交的创作需求不再经过统一调度，而是按领域直接交给专业分支：
// story → 故事创作（agents/file/StoryWriter）；edit → 文件编辑（agents/file/FileOperations）。
// HTTP 层只负责受理、查询与推送任务状态，真正的文件操纵都在智能体内完成。

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"LunarSubsystem/LunarHarness/agents/file/FileOperations"
	"LunarSubsystem/LunarHarness/agents/file/StoryWriter"
	"LunarSubsystem/LunarHarness/engine/websocket"
	"LunarSubsystem/LunarHarness/server"
)

// creationTaskStore 资源库创作任务的进程内任务表。
// 任务本身是短生命周期的后台操作，不做持久化：进程重启后列表为空，
// 前端会重新提交，避免把过期任务当成仍在运行。
type creationTaskStore struct {
	mu    sync.Mutex
	seq   int
	tasks map[string]server.LibraryCreationTask
	order []string
}

var creationTasks = &creationTaskStore{tasks: map[string]server.LibraryCreationTask{}}

const (
	// libraryCreationTimeout 单次创作任务的最长运行时间。
	libraryCreationTimeout = 30 * time.Minute
	// creationTaskKeep 进程内保留的最近任务数，避免长期运行后无限增长。
	creationTaskKeep = 100

	// creationDomainStory 故事创作领域
	creationDomainStory = "story"
	// creationDomainEdit 文件编辑领域
	creationDomainEdit = "edit"
)

// submitLibraryCreation 受理资源库创作请求并在后台执行对应领域的智能体。
func submitLibraryCreation(req server.LibraryCreationRequest) (server.LibraryCreationTask, error) {
	handles := append([]string(nil), req.Handles...)
	if strings.TrimSpace(req.Handle) != "" {
		handles = append(handles, strings.TrimSpace(req.Handle))
	}
	instruction := strings.TrimSpace(req.Description)
	mode := strings.TrimSpace(req.Mode)
	if mode == "" {
		mode = "assigned"
	}
	if mode == "random" || (instruction == "" && len(handles) == 0) {
		mode = "random"
		// 没有句柄但用户写了需求时，按用户需求执行；只有完全空白才回落到随机选题。
		if instruction == "" {
			instruction = defaultCreationInstruction()
		}
	}
	if instruction == "" {
		return server.LibraryCreationTask{}, fmt.Errorf("请填写创作需求描述")
	}
	domain := resolveCreationDomain(req.Domain, instruction)

	now := time.Now()
	task := server.LibraryCreationTask{
		TaskID:    creationTasks.nextID(now),
		Mode:      mode,
		Domain:    domain,
		Status:    "running",
		Stage:     creationDomainStage(domain),
		Progress:  10,
		CreatedAt: now.Format(time.RFC3339),
		UpdatedAt: now.Format(time.RFC3339),
	}
	creationTasks.put(task)
	go runLibraryCreation(task.TaskID, domain, instruction, handles)
	return task, nil
}

// resolveCreationDomain 规整领域；旧客户端不带 domain 时按描述推断。
func resolveCreationDomain(domain, instruction string) string {
	switch strings.ToLower(strings.TrimSpace(domain)) {
	case creationDomainStory:
		return creationDomainStory
	case creationDomainEdit:
		return creationDomainEdit
	}
	for _, keyword := range []string{"故事", "小说", "续写", "长篇", "章节", "大纲"} {
		if strings.Contains(instruction, keyword) {
			return creationDomainStory
		}
	}
	return creationDomainEdit
}

// creationDomainStage 领域对应的任务阶段名。
func creationDomainStage(domain string) string {
	if domain == creationDomainStory {
		return "story_writer"
	}
	return "file_operations"
}

// libraryCreationStatus 查询单个创作任务。
func libraryCreationStatus(taskID string) (server.LibraryCreationTask, bool) {
	task, ok := creationTasks.get(taskID)
	return task, ok
}

// libraryCreationList 查询最近创作任务。
func libraryCreationList(limit int) []server.LibraryCreationTask {
	return creationTasks.list(limit)
}

// runLibraryCreation 在后台执行一次创作，并把最终状态写回任务表。
func runLibraryCreation(taskID, domain, instruction string, handles []string) {
	ctx, cancel := context.WithTimeout(context.Background(), libraryCreationTimeout)
	defer cancel()

	var (
		summary string
		out     []string
		err     error
	)
	if domain == creationDomainStory {
		result, runErr := StoryWriter.Run(ctx, StoryWriter.Request{
			Instruction:   instruction,
			Handles:       handles,
			WorkspaceRoot: "documents",
		})
		summary, out, err = result.Summary, result.Handles, runErr
	} else {
		result, runErr := FileOperations.Run(ctx, FileOperations.Request{
			Instruction:   instruction,
			Handles:       handles,
			WorkspaceRoot: "documents",
		})
		summary, out, err = result.Summary, result.Handles, runErr
	}

	update := func(task server.LibraryCreationTask) server.LibraryCreationTask {
		task.Progress = 100
		task.UpdatedAt = time.Now().Format(time.RFC3339)
		if err != nil {
			task.Status = "failed"
			task.Error = err.Error()
		} else {
			task.Status = "completed"
			task.Summary = summary
			task.Handles = append([]string(nil), out...)
		}
		creationTasks.put(task)
		broadcastCreationTask(task)
		return task
	}

	task, ok := creationTasks.get(taskID)
	if !ok {
		return
	}
	update(task)
}

// defaultCreationInstruction 随机创作模式下交给文件编辑智能体的默认要求。
func defaultCreationInstruction() string {
	return "请随机选择一个短小的创作方向（HTML 小游戏、JavaScript 代码、HTML 小程序或歌词），" +
		"在 documents 下新建一个专属目录并完成作品；需要插画时只负责文字与目录，插画交由演绎系统处理。"
}

// broadcastCreationTask 把任务快照推送给资源库前端。
func broadcastCreationTask(task server.LibraryCreationTask) {
	if websocket.Instance == nil {
		return
	}
	websocket.Instance.BroadcastMessageToChannel(libraryTerminalChannel, "creation_task", task)
}

// nextID 生成任务标识。
func (s *creationTaskStore) nextID(now time.Time) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.seq++
	return fmt.Sprintf("creation-%s-%03d", now.Format("20060102-150405"), s.seq)
}

func (s *creationTaskStore) put(task server.LibraryCreationTask) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.tasks[task.TaskID]; !exists {
		s.order = append(s.order, task.TaskID)
	}
	s.tasks[task.TaskID] = task
	for len(s.order) > creationTaskKeep {
		delete(s.tasks, s.order[0])
		s.order = s.order[1:]
	}
}

func (s *creationTaskStore) get(taskID string) (server.LibraryCreationTask, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	task, ok := s.tasks[taskID]
	return task, ok
}

func (s *creationTaskStore) list(limit int) []server.LibraryCreationTask {
	s.mu.Lock()
	defer s.mu.Unlock()
	if limit <= 0 || limit > len(s.order) {
		limit = len(s.order)
	}
	out := make([]server.LibraryCreationTask, 0, limit)
	for i := len(s.order) - 1; i >= 0 && len(out) < limit; i-- {
		if task, ok := s.tasks[s.order[i]]; ok {
			out = append(out, task)
		}
	}
	return out
}
