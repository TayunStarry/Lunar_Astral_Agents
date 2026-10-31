package ArchiveReader

// gguf.go GGUF 模型元数据解析（复用 engine/media 的原生解析器）。
//
// 只读元数据，不加载权重；输出结构化摘要，供句柄系统直接回答。

import (
	"fmt"
	"path/filepath"
	"strings"

	"LunarSubsystem/LunarHarness/engine/media"
)

// 摘要输出顺序（固定键优先，避免 map 迭代顺序随机）
var ggufSummaryKeys = []string{
	"Model Name", "Architecture", "Quantization", "Quant Version",
	"Context Length", "Embedding Dim", "Block Count", "Attention Heads",
	"KV Heads", "FFN Dim", "Vocab Size",
}

// GGUFSummary 解析 GGUF 文件并返回元数据摘要
func GGUFSummary(target string) (string, error) {
	abs, rel, err := resolveGGUFPath(target)
	if err != nil {
		return "", err
	}
	metadata, err := media.ParseGGUFFile(abs)
	if err != nil {
		return "", fmt.Errorf("GGUF 解析失败: %w", err)
	}
	summary := media.ExtractGGUFSummary(metadata, filepath.Base(abs))
	var builder strings.Builder
	fmt.Fprintf(&builder, "【GGUF 模型元数据】%s\n", rel)
	for _, key := range ggufSummaryKeys {
		if value, ok := summary[key]; ok && strings.TrimSpace(value) != "" {
			fmt.Fprintf(&builder, "- %s：%s\n", key, value)
		}
	}
	fmt.Fprintf(&builder, "- 元数据项：%d", len(metadata))
	return strings.TrimRight(builder.String(), "\n"), nil
}

// resolveGGUFPath 解析 GGUF 目标（支持句柄与路径），并校验扩展名与实体
func resolveGGUFPath(target string) (abs, rel string, err error) {
	abs, rel, err = ResolveExisting(target)
	if err != nil {
		return "", "", err
	}
	if !IsGGUFExt(filepath.Ext(abs)) {
		return "", "", fmt.Errorf("不是 .gguf 模型文件: %s", filepath.Ext(abs))
	}
	return abs, rel, nil
}
