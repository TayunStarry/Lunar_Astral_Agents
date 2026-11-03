package Creator

// flow_read.go 文件 / 目录 / 压缩包读取节点：按路由把读取交给对应智能体。
// 所有读取结果都会记录到流程上下文（路由、摘要、句柄），供后续节点与追溯使用。

import (
	"context"
	"fmt"
	"strings"

	"LunarSubsystem/LunarHarness/agents/ArchiveReader"
	"LunarSubsystem/LunarHarness/agents/DocumentReader"
	"LunarSubsystem/LunarHarness/agents/FileEditor"
	"LunarSubsystem/LunarHarness/engine/fileflow"
)

// fileList 列出目录；大型目录交给文献解析，压缩包交给归档解析。
func (s *session) fileList(ctx context.Context, args map[string]any) toolOutcome {
	target := argString(args, "path")
	decision, err := routeTarget(target, fileflow.OpList)
	if err != nil {
		return failure(toolFileList, err)
	}
	if decision.Kind == routeFileEditor {
		result := FileEditor.FileList(target)
		s.recordFileResult(fileflow.OpList, target, decision, result)
		return fileOutcome(result)
	}
	text, err := s.readByRoute(ctx, decision, "", "")
	if err != nil {
		s.recordFailure(fileflow.OpList, target, decision, err)
		return failure(toolFileList, err)
	}
	return s.readOutcome(fileflow.OpList, target, decision, text)
}

// fileRead 读取文件；大文件与压缩包按路由交给对应智能体。
func (s *session) fileRead(ctx context.Context, args map[string]any) toolOutcome {
	target := argString(args, "path")
	decision, err := routeTarget(target, fileflow.OpRead)
	if err != nil {
		return failure(toolFileRead, err)
	}
	if decision.Kind == routeFileEditor {
		result := FileEditor.ReadFile(target, argInt(args, "offset", 0), argInt(args, "max_chars", 0))
		s.recordFileResult(fileflow.OpRead, target, decision, result)
		return fileOutcome(result)
	}
	text, err := s.readByRoute(ctx, decision, decision.Entry, readRequest(args))
	if err != nil {
		s.recordFailure(fileflow.OpRead, target, decision, err)
		return failure(toolFileRead, err)
	}
	return s.readOutcome(fileflow.OpRead, target, decision, text)
}

// archiveRead 压缩包 / GGUF 读取（显式入口，便于模型按条目读取）。
func (s *session) archiveRead(ctx context.Context, args map[string]any) toolOutcome {
	target := argString(args, "target")
	entry := strings.TrimSpace(argString(args, "entry"))
	decision, err := routeTarget(target, fileflow.OpArchiveRead)
	if err != nil {
		return failure(toolArchiveRead, err)
	}
	if decision.Kind != routeArchiveReader {
		return failure(toolArchiveRead, fmt.Errorf("目标不是压缩包或 GGUF: %s", target))
	}
	request := entry
	if request == "" {
		request = strings.TrimSpace(argString(args, "query"))
	}
	text, err := s.readByRoute(ctx, decision, request, request)
	if err != nil {
		s.recordFailure(fileflow.OpArchiveRead, target, decision, err)
		return failure(toolArchiveRead, err)
	}
	return s.readOutcome(fileflow.OpArchiveRead, target, decision, text)
}

// readByRoute 按路由执行一次读取，返回文本结果。
func (s *session) readByRoute(ctx context.Context, decision routeDecision, entry, request string) (string, error) {
	switch decision.Kind {
	case routeArchiveReader:
		return ArchiveReader.Inspect(decision.Abs, strings.TrimSpace(entry))
	case routeDocumentReader:
		if decision.IsDir {
			return DocumentReader.InspectFolderContext(ctx, decision.Abs, request)
		}
		handle, err := DocumentReader.ResolveHandle(decision.Abs)
		if err != nil {
			return "", err
		}
		return DocumentReader.InspectTextContext(ctx, handle, request)
	default:
		return "", fmt.Errorf("路由 %s 不支持文本读取", decision.Kind)
	}
}

// readRequest 从工具参数里取阅读诉求。
func readRequest(args map[string]any) string {
	for _, key := range []string{"query", "request", "question"} {
		if text := strings.TrimSpace(argString(args, key)); text != "" {
			return text
		}
	}
	return "阅读这个文件并概述要点。"
}
