package FileEditor

// scheduling.go 统一调度：文件编辑把完整任务交给其他智能体，并回收结果。
//
// 调度目标（type）：
//   archive_ops      归档操作：创建 / 删除压缩包、增删条目（自然语言入口）
//   archive_reader   归档解析：压缩包结构 / 条目内容 / GGUF 元数据（确定性解析）
//   document_reader  文献解析：理解文档正文
//   vision           视觉理解：图片 / 动图 / 视频
//
// target 可选：给了就直接用；缺省时从 instruction 中提取句柄、压缩包路径或链接。

import (
	"context"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"LunarSubsystem/LunarHarness/agents/ArchiveOps"
	"LunarSubsystem/LunarHarness/agents/ArchiveReader"
	"LunarSubsystem/LunarHarness/agents/DocumentReader"
	"LunarSubsystem/LunarHarness/agents/Vision"
	"LunarSubsystem/LunarHarness/engine/resource"
)

// archiveTargetPattern instruction 中的压缩包 / GGUF 路径
var archiveTargetPattern = regexp.MustCompile(`(?i)[\p{L}\p{N}_./\\-]+\.(?:zip|ltpx|gguf)`)

// urlPattern instruction 中的视频 / 网页链接
var urlPattern = regexp.MustCompile(`https?://[^\s"'<>（）()，。；]+`)

// scheduleTask 按 type 单方面调度目标智能体。
func scheduleTask(ctx context.Context, args map[string]any) OpResult {
	kind := strings.ToLower(strings.TrimSpace(argString(args, "type")))
	instruction := strings.TrimSpace(argString(args, "instruction"))
	target := strings.TrimSpace(argString(args, "target"))
	if instruction == "" {
		return failResult(ToolSchedule, target, fmt.Errorf("instruction 不能为空，请用自然语言说明要做什么"))
	}
	switch kind {
	case ScheduleArchiveOps:
		return scheduleArchiveOps(ctx, instruction, target)
	case ScheduleArchiveReader:
		return scheduleArchiveReader(instruction, target)
	case ScheduleDocumentReader:
		return scheduleDocumentReader(instruction, target)
	case ScheduleVision:
		return scheduleVision(instruction, target)
	default:
		return failResult(ToolSchedule, target,
			fmt.Errorf("type 只能取 archive_ops / archive_reader / document_reader / vision"))
	}
}

// scheduleArchiveOps 调度归档操作：自然语言指令交给归档操作智能体自行拆解
func scheduleArchiveOps(ctx context.Context, instruction, target string) OpResult {
	if target != "" {
		instruction += "\n（目标：" + target + "）"
	}
	result, err := ArchiveOps.Execute(ctx, instruction)
	if err != nil {
		return failResult(ToolSchedule, target, fmt.Errorf("归档操作调度失败: %w", err))
	}
	if !result.OK {
		return OpResult{
			OK:      false,
			Op:      ToolSchedule,
			Path:    result.Path,
			Handles: []string{},
			Summary: result.Summary,
			Detail:  result.Detail,
		}
	}
	return newResult(ToolSchedule, result.Path, result.Summary, result.Handles, result.Detail)
}

// scheduleArchiveReader 调度归档解析：压缩包结构与 GGUF 由归档解析确定性返回
func scheduleArchiveReader(instruction, target string) OpResult {
	path := target
	if path == "" {
		path = archiveTargetPattern.FindString(instruction)
	}
	if path == "" {
		return failResult(ToolSchedule, "", fmt.Errorf(
			"请提供压缩包或 GGUF 路径（用 target 参数，或在 instruction 里写清路径）"))
	}
	path = archiveTargetPath(path)
	text, err := ArchiveReader.Inspect(path, instruction)
	if err != nil {
		return failResult(ToolSchedule, path, fmt.Errorf("归档解析失败: %w", err))
	}
	return newResult(ToolSchedule, path, text, nil, nil)
}

// scheduleDocumentReader 调度文献解析：理解文档正文与结构
func scheduleDocumentReader(instruction, target string) OpResult {
	handle, err := resolveDocumentHandle(target, instruction)
	if err != nil {
		return failResult(ToolSchedule, target, err)
	}
	text, err := DocumentReader.InspectText(handle, instruction)
	if err != nil {
		return failResult(ToolSchedule, handle, fmt.Errorf("文献解析失败: %w", err))
	}
	return newResult(ToolSchedule, handle, text, []string{handle}, nil)
}

// scheduleVision 调度视觉理解：图片 / 动图走观看管线，链接走视频印象
func scheduleVision(instruction, target string) OpResult {
	raw, isVideo := resolveVisualTarget(target, instruction)
	if raw == "" {
		return failResult(ToolSchedule, "", fmt.Errorf(
			"请提供图片 / 动图句柄或视频链接（用 target 参数，或在 instruction 里写清句柄）"))
	}
	if isVideo {
		text, err := Vision.DescribeVideoURL(raw, instruction)
		if err != nil {
			return failResult(ToolSchedule, raw, fmt.Errorf("视觉理解失败: %w", err))
		}
		return newResult(ToolSchedule, raw, text, nil, nil)
	}
	text, err := Vision.DescribeMedia(raw, instruction)
	if err != nil {
		return failResult(ToolSchedule, raw, fmt.Errorf("视觉理解失败: %w", err))
	}
	return newResult(ToolSchedule, raw, text, []string{raw}, nil)
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
