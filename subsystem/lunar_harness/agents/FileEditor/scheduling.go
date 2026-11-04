package FileEditor

// scheduling.go 统一调度：文件编辑把完整任务交给其他智能体，并回收结果。
//
// 调度目标（type）：
//   archive_ops      归档操作：创建 / 删除压缩包、增删条目（自然语言入口）
//   archive_reader   归档解析：压缩包结构 / 条目内容 / GGUF 元数据（确定性解析）
//   document_reader  文献解析：理解文档正文
//   vision           视觉理解：图片 / 动图 / 视频
//
// 输入严格使用统一原语 {type, instruction, handle, details}，
// 输出统一为 {result, handle, error}；handle 给出目标路径或句柄，缺省时从 instruction 提取。

import (
	"context"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"LunarSubsystem/LunarHarness/agents/ArchiveOps"
	"LunarSubsystem/LunarHarness/agents/DocumentReader"
	"LunarSubsystem/LunarHarness/agents/Vision"
	"LunarSubsystem/LunarHarness/engine/resource"
	"LunarSubsystem/LunarHarness/executors/ArchiveReader"
	"LunarSubsystem/LunarHarness/protocol/agentdispatch"
)

// archiveTargetPattern instruction 中的压缩包 / GGUF 路径
var archiveTargetPattern = regexp.MustCompile(`(?i)[\p{L}\p{N}_./\\-]+\.(?:zip|ltpx|gguf)`)

// urlPattern instruction 中的视频 / 网页链接
var urlPattern = regexp.MustCompile(`https?://[^\s"'<>（）()，。；]+`)

// scheduleTask 按 type 单方面调度目标智能体，并序列化为统一输出。
func scheduleTask(ctx context.Context, args map[string]any) OpResult {
	call, err := agentdispatch.Parse(args)
	if err != nil {
		return agentResult(agentdispatch.Failure(err.Error(), nil))
	}
	kind := strings.ToLower(call.Type)
	if call.Details {
		if text, ok := scheduleScope(kind); ok {
			return agentResult(agentdispatch.Success(text, nil))
		}
		return agentResult(agentdispatch.Failure("type 只能取 archive_ops / archive_reader / document_reader / vision", nil))
	}
	if call.Instruction == "" {
		return agentResult(agentdispatch.Failure("instruction 不能为空，请用自然语言说明要做什么", nil))
	}
	switch kind {
	case ScheduleArchiveOps:
		return agentResult(scheduleArchiveOps(ctx, call.Instruction, call.Handle))
	case ScheduleArchiveReader:
		return agentResult(scheduleArchiveReader(call.Instruction, call.Handle))
	case ScheduleDocumentReader:
		return agentResult(scheduleDocumentReader(ctx, call.Instruction, call.Handle))
	case ScheduleVision:
		return agentResult(scheduleVision(call.Instruction, call.Handle))
	default:
		return agentResult(agentdispatch.Failure("type 只能取 archive_ops / archive_reader / document_reader / vision", nil))
	}
}

// scheduleScope 返回各调度目标的职能范围描述（details=true 专用，不产生副作用）。
func scheduleScope(kind string) (string, bool) {
	switch kind {
	case ScheduleArchiveOps:
		return "归档操作子智能体：创建 / 删除压缩包、增删压缩包条目。", true
	case ScheduleArchiveReader:
		return "归档解析子智能体：查看压缩包结构、读取条目内容、解析 GGUF 元数据。", true
	case ScheduleDocumentReader:
		return "文献解析子智能体：理解文档正文与结构，按需求查阅内容。", true
	case ScheduleVision:
		return "视觉理解子智能体：观看图片、动图与视频并给出描述。", true
	}
	return "", false
}

// agentResult 把统一输出原语包装成本智能体的操作结果：
// 序列化时只输出 {result, handle, error}，Summary 仅供控制台展示。
func agentResult(result agentdispatch.Result) OpResult {
	summary := result.Result
	if summary == "" {
		summary = result.Error
	}
	return OpResult{
		OK:       result.Error == "",
		Op:       ToolSchedule,
		Handles:  result.Handle,
		Summary:  summary,
		Dispatch: &result,
	}
}

// scheduleArchiveOps 调度归档操作：自然语言指令交给归档操作智能体自行拆解
func scheduleArchiveOps(ctx context.Context, instruction string, handles []string) agentdispatch.Result {
	instruction = agentdispatch.WithHandles(instruction, handles)
	result, err := ArchiveOps.Execute(ctx, instruction)
	if err != nil {
		return agentdispatch.Failure("归档操作调度失败: "+err.Error(), nil)
	}
	if !result.OK {
		return agentdispatch.PartialFailure(fmt.Sprintf("归档操作未成功：%s", strings.TrimSpace(result.Summary)), result.Summary, result.Handles)
	}
	return agentdispatch.Success(result.Summary, result.Handles)
}

// scheduleArchiveReader 调度归档解析：压缩包结构与 GGUF 由归档解析确定性返回
func scheduleArchiveReader(instruction string, handles []string) agentdispatch.Result {
	path := firstHandle(handles)
	if path == "" {
		path = archiveTargetPattern.FindString(instruction)
	}
	if path == "" {
		return agentdispatch.Failure("请提供压缩包或 GGUF 路径（用 handle 参数，或在 instruction 里写清路径）", nil)
	}
	path = archiveTargetPath(path)
	text, err := ArchiveReader.Inspect(path, instruction)
	if err != nil {
		return agentdispatch.Failure("归档解析失败: "+err.Error(), nil)
	}
	return agentdispatch.Success(text, nil)
}

// scheduleDocumentReader 调度文献解析：理解文档正文与结构
func scheduleDocumentReader(ctx context.Context, instruction string, handles []string) agentdispatch.Result {
	handle, err := resolveDocumentHandle(strings.Join(handles, " "), instruction)
	if err != nil {
		return agentdispatch.Failure(err.Error(), nil)
	}
	text, err := DocumentReader.InspectTextContext(ctx, handle, instruction)
	if err != nil {
		return agentdispatch.Failure("文献解析失败: "+err.Error(), []string{handle})
	}
	return agentdispatch.Success(text, []string{handle})
}

// scheduleVision 调度视觉理解：图片 / 动图走观看管线，链接走视频印象
func scheduleVision(instruction string, handles []string) agentdispatch.Result {
	raw, isVideo := resolveVisualTarget(strings.Join(handles, " "), instruction)
	if raw == "" {
		return agentdispatch.Failure("请提供图片 / 动图句柄或视频链接（用 handle 参数，或在 instruction 里写清句柄）", nil)
	}
	if isVideo {
		text, err := Vision.DescribeVideoURL(raw, instruction)
		if err != nil {
			return agentdispatch.Failure("视觉理解失败: "+err.Error(), nil)
		}
		return agentdispatch.Success(text, nil)
	}
	text, err := Vision.DescribeMedia(raw, instruction)
	if err != nil {
		return agentdispatch.Failure("视觉理解失败: "+err.Error(), []string{raw})
	}
	return agentdispatch.Success(text, []string{raw})
}

// firstHandle 取第一个非空目标（多句柄任务只用首个作为显式入口）。
func firstHandle(handles []string) string {
	for _, handle := range handles {
		if text := strings.TrimSpace(handle); text != "" {
			return text
		}
	}
	return ""
}

// resolveDocumentHandle 取文档句柄：优先句柄，其次 /document 内的路径
func resolveDocumentHandle(target, instruction string) (string, error) {
	for _, handle := range resource.ExtractHandles(target + " " + instruction) {
		parsed, ok := resource.ParseHandle(handle)
		if ok && (parsed.Kind == resource.KindDocument || parsed.Kind == resource.KindTable) {
			return resource.FormatHandle(parsed.Kind, parsed.Seq), nil
		}
	}
	if target != "" && !strings.HasPrefix(target, "@") {
		if _, localRel, err := documentPath(target); err == nil {
			if handle := registerHandle(localRel); handle != "" {
				return handle, nil
			}
		}
	}
	return "", fmt.Errorf("请提供文档句柄（@D.xxx）或 /document 内的文档路径")
}

// resolveVisualTarget 取视觉目标：图片 / 动图句柄，或视频链接
func resolveVisualTarget(target, instruction string) (string, bool) {
	if target != "" {
		if strings.HasPrefix(target, "@") {
			return target, false
		}
		if link := urlPattern.FindString(target); link != "" {
			return link, true
		}
		if !isArchiveTarget(target) {
			if _, localRel, err := documentPath(target); err == nil {
				if handle := registerHandle(localRel); handle != "" {
					return handle, false
				}
			}
		}
		return target, false
	}
	for _, handle := range resource.ExtractHandles(instruction) {
		parsed, ok := resource.ParseHandle(handle)
		if ok && (parsed.Kind == resource.KindImage || parsed.Kind == resource.KindAnimated) {
			return resource.FormatHandle(parsed.Kind, parsed.Seq), false
		}
	}
	if link := urlPattern.FindString(instruction); link != "" {
		return link, true
	}
	return "", false
}

// isFileTool 普通文档工具（压缩包 / GGUF 不允许直接使用）
func isFileTool(name string) bool {
	switch name {
	case ToolList, ToolRead, ToolWrite, ToolCreate, ToolDelete,
		ToolCopy, ToolMove, ToolRename, ToolSearch, ToolReplace, ToolInfo:
		return true
	}
	return false
}

// isArchivePath 路径是否指向压缩包或 GGUF 二进制容器
func isArchivePath(path string) bool {
	if isArchiveTarget(path) {
		return true
	}
	return strings.EqualFold(filepath.Ext(strings.TrimSpace(path)), ".gguf")
}

// isArchiveTarget 路径是否指向压缩包（含条目语法）
func isArchiveTarget(path string) bool {
	lower := strings.ToLower(filepath.ToSlash(strings.TrimSpace(path)))
	if lower == "" {
		return false
	}
	if archive, _, ok := splitArchiveEntry(path); ok {
		lower = strings.ToLower(archive)
	}
	ext := filepath.Ext(lower)
	return ext == ".zip" || ext == ".ltpx"
}

// splitArchiveEntry 拆分「压缩包 + 内部条目」语法（pkg.zip/entry 或 pkg.zip:entry）
func splitArchiveEntry(path string) (archive, entry string, ok bool) {
	clean := filepath.ToSlash(strings.TrimSpace(path))
	lower := strings.ToLower(clean)
	for _, marker := range []string{".zip", ".ltpx"} {
		if index := strings.Index(lower, marker+"/"); index >= 0 {
			end := index + len(marker)
			return clean[:end], clean[end+1:], true
		}
		if index := strings.Index(lower, marker+":"); index >= 0 {
			end := index + len(marker)
			return clean[:end], clean[end+1:], true
		}
	}
	return "", "", false
}

// archiveTargetPath 把 /document 相对路径补全为归档智能体可解析的路径
//
// 归档智能体同时接受 /document/ 前缀与相对 LocalDir 的路径；
// 这里只补全无前缀的普通路径，已带前缀、句柄或绝对路径原样保留。
func archiveTargetPath(path string) string {
	cleaned := strings.TrimSpace(path)
	if cleaned == "" || strings.HasPrefix(cleaned, "@") || strings.HasPrefix(cleaned, "/") {
		return cleaned
	}
	if filepath.IsAbs(cleaned) {
		return cleaned
	}
	for _, prefix := range []string{"documents/", "multimedia/", "audios/", "unknown/"} {
		if strings.HasPrefix(cleaned, prefix) {
			return cleaned
		}
	}
	return "/document/" + strings.TrimPrefix(cleaned, "./")
}
