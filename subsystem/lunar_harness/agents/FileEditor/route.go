package FileEditor

// route.go 压缩包任务的路由分发。
//
// 文件编辑只维护普通文档；压缩包属于二进制容器，读写都必须交给专门的智能体：
//   创建 / 删除 / 条目新增 / 条目移除 → 归档操作智能体（ArchiveOps）
//   结构查询 / 条目读取 / GGUF 元数据 → 归档解析智能体（ArchiveReader）
//
// 两层触发：
//   1. 模型直接调用 archive_* 工具 → 原样转发；
//   2. 模型仍用 file_read / file_delete 等普通工具指向压缩包 → 自动改道，
//      避免文件编辑用文本方式去写二进制容器。

import (
	"fmt"
	"path/filepath"
	"strings"

	"LunarSubsystem/LunarHarness/agents/ArchiveOps"
	"LunarSubsystem/LunarHarness/agents/ArchiveReader"
)

// routeArchive 尝试把一次工具步骤转发给归档智能体；返回 routed=false 表示与压缩包无关
func routeArchive(name string, args map[string]any) (OpResult, bool) {
	path := archivePath(argString(args, "path"))
	switch name {
	case ToolArchiveList:
		return routeArchiveList(path), true
	case ToolArchiveRead:
		return routeArchiveRead(path, argString(args, "entry")), true
	case ToolArchiveCreate:
		return routeArchiveCreate(path, archivePaths(argStringList(args, "paths"))), true
	case ToolArchiveDelete:
		return fromArchiveResult(ArchiveOps.Delete(path)), true
	case ToolArchiveAdd:
		return fromArchiveResult(ArchiveOps.AddEntry(path, argString(args, "entry"), archivePath(argString(args, "source")), argString(args, "content"))), true
	case ToolArchiveRemove:
		return fromArchiveResult(ArchiveOps.RemoveEntry(path, argString(args, "entry"))), true
	case ToolGGUF:
		return routeGGUF(archivePath(path)), true
	}

	if !isArchiveTarget(path) {
		return OpResult{}, false
	}
	// 压缩包内条目语法：pkg.zip/src/main.go 或 pkg.zip:src/main.go
	if archive, entry, ok := splitArchiveEntry(path); ok {
		switch name {
		case ToolRead:
			return routeArchiveRead(archive, entry), true
		case ToolDelete:
			return fromArchiveResult(ArchiveOps.RemoveEntry(archive, entry)), true
		case ToolWrite, ToolCreate, ToolReplace:
			return fromArchiveResult(ArchiveOps.AddEntry(archive, entry, "", argString(args, "content"))), true
		case ToolInfo:
			return routeArchiveRead(archive, entry), true
		}
	}

	// 整包级操作：自动改道到归档智能体
	switch name {
	case ToolDelete:
		return fromArchiveResult(ArchiveOps.Delete(path)), true
	case ToolList, ToolInfo:
		return routeArchiveList(path), true
	case ToolRead, ToolSearch:
		return routeArchiveStructureOnly(path), true
	case ToolWrite, ToolCreate, ToolReplace:
		return failResult(name, path, fmt.Errorf("压缩包是二进制容器，请使用 %s 新增文件，或使用 %s 移除条目", ToolArchiveAdd, ToolArchiveRemove)), true
	}
	return OpResult{}, false
}

// routeArchiveList 查询压缩包结构
func routeArchiveList(path string) OpResult {
	if path == "" {
		return failResult(ToolArchiveList, path, fmt.Errorf("请提供压缩包路径"))
	}
	summary, err := ArchiveReader.Summary(path)
	if err != nil {
		return failResult(ToolArchiveList, path, err)
	}
	return newResult(ToolArchiveList, path, summary, nil, nil)
}

// routeArchiveRead 读取压缩包内指定条目
func routeArchiveRead(path, entry string) OpResult {
	if path == "" || strings.TrimSpace(entry) == "" {
		return failResult(ToolArchiveRead, path, fmt.Errorf("请提供压缩包路径与条目名"))
	}
	text, err := ArchiveOps.Read(path, entry, 0)
	if err != nil {
		return failResult(ToolArchiveRead, path, err)
	}
	return newResult(ToolArchiveRead, path, text, nil, map[string]any{"entry": entry})
}

// routeArchiveCreate 创建压缩包
func routeArchiveCreate(path string, sources []string) OpResult {
	if path == "" {
		return failResult(ToolArchiveCreate, path, fmt.Errorf("请提供要创建的压缩包路径"))
	}
	return fromArchiveResult(ArchiveOps.Create(path, sources))
}

// routeArchiveStructureOnly 普通读取 / 检索指向压缩包时，只给结构并提示专用工具
func routeArchiveStructureOnly(path string) OpResult {
	summary, err := ArchiveReader.Summary(path)
	if err != nil {
		return failResult(ToolRead, path, err)
	}
	summary += fmt.Sprintf("\n（压缩包请使用 %s 读取指定条目，或使用 %s 新增 / %s 移除条目）",
		ToolArchiveRead, ToolArchiveAdd, ToolArchiveRemove)
	return newResult(ToolRead, path, summary, nil, nil)
}

// routeGGUF 解析 GGUF 模型元数据
func routeGGUF(path string) OpResult {
	if path == "" {
		return failResult(ToolGGUF, path, fmt.Errorf("请提供 GGUF 文件路径"))
	}
	summary, err := ArchiveReader.GGUFSummary(path)
	if err != nil {
		return failResult(ToolGGUF, path, err)
	}
	return newResult(ToolGGUF, path, summary, nil, nil)
}

// fromArchiveResult 把归档操作结果转换为文件编辑的结构化结果
func fromArchiveResult(result ArchiveOps.Result) OpResult {
	if result.OK {
		return newResult(result.Op, result.Path, result.Summary, result.Handles, result.Detail)
	}
	return OpResult{
		OK:      false,
		Op:      result.Op,
		Path:    result.Path,
		Handles: []string{},
		Summary: result.Summary,
		Detail:  result.Detail,
	}
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

// archivePath 把文件编辑的 /document 相对路径补全为归档智能体可解析的路径。
//
// 文件编辑的路径契约是「相对 /document」；归档智能体同时接受 /document/ 前缀与
// 相对 LocalDir 的路径。这里只补全无前缀的普通路径，已带前缀或绝对路径原样保留。
func archivePath(path string) string {
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

// archivePaths 批量补全打包来源路径
func archivePaths(paths []string) []string {
	out := make([]string, 0, len(paths))
	for _, path := range paths {
		out = append(out, archivePath(path))
	}
	return out
}

// argStringList 读取字符串数组参数（兼容数组与单个字符串）
func argStringList(args map[string]any, key string) []string {
	switch value := args[key].(type) {
	case []any:
		out := make([]string, 0, len(value))
		for _, item := range value {
			if text, ok := item.(string); ok && strings.TrimSpace(text) != "" {
				out = append(out, strings.TrimSpace(text))
			}
		}
		return out
	case []string:
		return value
	case string:
		if strings.TrimSpace(value) == "" {
			return nil
		}
		return []string{strings.TrimSpace(value)}
	}
	return nil
}
