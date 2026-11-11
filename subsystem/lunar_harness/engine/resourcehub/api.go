package resourcehub

// api.go 统一资源基座的对外门面：句柄 ↔ 路径转换、统一工具定义。
//
// 两个方向都只走句柄体系的权威实现：
//   - HandleToPath 不触发解析、不做 GC 记账，只把句柄翻译成本地实体路径；
//   - PathToHandle 复用定位器，必要时登记句柄，保证「文件信息一律以句柄传递」。

import (
	"fmt"
	"os"
	"path/filepath"

	"LunarSubsystem/LunarHarness/engine/config"
	"LunarSubsystem/LunarHarness/engine/resource"
)

// HandleToPath 句柄 → 本地实体路径（返回绝对路径与相对 LocalDir 的路径）。
//
// 只有带本地实体的句柄（本地文件 / 已落盘的媒体）才有路径；
// 纯链接句柄（@U）与悬空句柄（如音频转写）没有本地实体，返回错误。
func HandleToPath(rawHandle string) (absPath, relPath string, err error) {
	handle, ok := resource.ParseHandle(rawHandle)
	if !ok || handle.Seq <= 0 {
		return "", "", fmt.Errorf("%s", resource.ErrHandleInvalid)
	}
	normalized := handle.FullText()
	entry, found := resource.Lookup(normalized)
	if !found {
		return "", "", fmt.Errorf("%s", resource.ErrHandleInvalid)
	}
	if entry.BlobPath == "" {
		return "", "", fmt.Errorf("%s 没有本地实体（%s）", normalized, entry.URL)
	}
	absPath = filepath.Join(filepath.Clean(config.LocalDir), filepath.FromSlash(entry.BlobPath))
	if info, statErr := os.Stat(absPath); statErr != nil || info.IsDir() {
		return absPath, entry.BlobPath, fmt.Errorf("%s 的本体已丢失：%s", normalized, entry.BlobPath)
	}
	return absPath, entry.BlobPath, nil
}

// PathToHandle 文件名 / 相对路径 → 已登记句柄（必要时现场登记）。
//
// 返回规范句柄与相对 LocalDir 的路径；同名歧义时返回 *AmbiguousError。
func PathToHandle(input string) (handle, relPath string, err error) {
	match, err := ResolveToHandle(input, nil)
	if err != nil {
		return "", "", err
	}
	return match.Handle, match.Rel, nil
}

// ToolDescription 返回统一资源聚合工具的描述文本。
func ToolDescription() string {
	return "对文件内容与长期记忆进行统一检索：type 是模式数组，可传 memory（查记忆）、handle（按句柄读内容）、" +
		"path（按文件名或路径读内容）、list（列出符合标准的句柄）；一次可传多个模式，例如 [\"handle\"] 或 [\"handle\",\"memory\"]。" +
		"多个句柄同时查询时放进 query 数组，例如 query=[\"@D.077\",\"@D.078\"]。系统默认以句柄传递文件信息；" +
		"只有 path 模式接受文件名或路径，并会自动解析并登记为句柄后返回结果。"
}

// ToolSchema 返回统一资源聚合工具的函数参数 schema。
func ToolSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"type": map[string]any{
				"type":     "array",
				"minItems": 1,
				"items": map[string]any{
					"type": "string",
					"enum": []string{TypeMemory, TypeHandle, TypePath, TypeList},
				},
				"description": "查询模式数组：memory=检索长期记忆；handle=按句柄读取内容；path=按文件名或路径读取内容；list=列出句柄。可同时传多个模式。",
			},
			"query": map[string]any{
				"type":  "array",
				"items": map[string]any{"type": "string"},
				"description": "memory 填 1-4 个搜索关键词；handle 只填句柄（如 @D.003），可一次传多个句柄同时查询；" +
					"path 填文件名或相对路径；list 留空列全部，填类型码 D/I/G/A/V/U 只列该类型，填文件名 / 路径则按名称与路径过滤",
			},
			"limit": map[string]any{
				"type":        "number",
				"description": "返回结果数量，默认 5（list 模式默认 50，上限 200）",
			},
		},
		"required": []string{"type"},
	}
}
