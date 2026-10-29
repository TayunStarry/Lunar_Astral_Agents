package Creator

// codeindex.go 代码索引：抽取目标代码文件的三类结构化数据。
// 采用顶层行扫描：只有从第 0 列开始（export 修饰允许）的声明才视为全局，
// 注释按行累积并附着到紧随其后的声明上；不做完整语法分析，避免引入解析器依赖。

import (
	"regexp"
	"strings"
)

var (
	reGlobalVar = regexp.MustCompile(`^(?:var|let|const)\s+([A-Za-z_$][A-Za-z0-9_$]*)\s*=`)
	reFunction  = regexp.MustCompile(`^(?:async\s+)?function\s+([A-Za-z_$][A-Za-z0-9_$]*)\s*\(([^)]*)\)`)
	reArrowFunc = regexp.MustCompile(`^(?:const|let|var)\s+([A-Za-z_$][A-Za-z0-9_$]*)\s*=\s*(?:async\s*)?(?:function\s*)?\(([^)]*)\)\s*=>`)
	reFuncExpr  = regexp.MustCompile(`^(?:const|let|var)\s+([A-Za-z_$][A-Za-z0-9_$]*)\s*=\s*(?:async\s+)?function\s*\(([^)]*)\)`)
	reClassDecl = regexp.MustCompile(`^(?:export\s+)?(?:default\s+)?class\s+([A-Za-z_$][A-Za-z0-9_$]*)`)
)

// buildCodeIndex 生成一个文件的代码索引。
func buildCodeIndex(source string) codeIndex {
	index := codeIndex{
		Variables: []codeSymbol{},
		Functions: []codeSymbol{},
		Classes:   []codeSymbol{},
	}
	var pending []string
	inBlock := false
	for _, raw := range strings.Split(source, "\n") {
		line := strings.TrimRight(raw, "\r")
		trimmed := strings.TrimSpace(line)
		if inBlock {
			if end := strings.Index(trimmed, "*/"); end >= 0 {
				if piece := cleanComment(trimmed[:end]); piece != "" {
					pending = append(pending, piece)
				}
				inBlock = false
			} else if piece := cleanComment(trimmed); piece != "" {
				pending = append(pending, piece)
			}
			continue
		}
		if strings.HasPrefix(trimmed, "/*") {
			if !strings.Contains(trimmed, "*/") {
				inBlock = true
			}
			if piece := cleanComment(strings.TrimSuffix(strings.TrimPrefix(trimmed, "/*"), "*/")); piece != "" {
				pending = append(pending, piece)
			}
			continue
		}
		if strings.HasPrefix(trimmed, "//") {
			if piece := cleanComment(trimmed); piece != "" {
				pending = append(pending, piece)
			}
			continue
		}
		if trimmed == "" {
			continue
		}
		// 顶层只认第 0 列的声明；缩进行属于函数/类内部。
		if strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t") {
			pending = nil
			continue
		}
		body := strings.TrimPrefix(line, "export ")
		body = strings.TrimPrefix(body, "default ")
		comment := strings.Join(pending, " ")
		pending = nil

		if m := reFunction.FindStringSubmatch(body); m != nil {
			index.Functions = append(index.Functions, codeSymbol{Name: m[1], Signature: m[1] + "(" + compactParams(m[2]) + ")", Comment: comment})
			continue
		}
		if m := reArrowFunc.FindStringSubmatch(body); m != nil {
			index.Functions = append(index.Functions, codeSymbol{Name: m[1], Signature: m[1] + "(" + compactParams(m[2]) + ") =>", Comment: comment})
			continue
		}
		if m := reFuncExpr.FindStringSubmatch(body); m != nil {
			index.Functions = append(index.Functions, codeSymbol{Name: m[1], Signature: m[1] + "(" + compactParams(m[2]) + ")", Comment: comment})
			continue
		}
		if m := reClassDecl.FindStringSubmatch(body); m != nil {
			index.Classes = append(index.Classes, codeSymbol{Name: m[1], Comment: comment})
			continue
		}
		if m := reGlobalVar.FindStringSubmatch(body); m != nil {
			index.Variables = append(index.Variables, codeSymbol{Name: m[1], Comment: comment})
		}
	}
	return index
}

// compactParams 压缩参数列表空白。
func compactParams(params string) string {
	return strings.Join(strings.Fields(params), " ")
}

// cleanComment 去掉注释符号与前后空白。
func cleanComment(line string) string {
	line = strings.TrimSpace(line)
	line = strings.TrimPrefix(line, "//")
	line = strings.TrimPrefix(line, "/*")
	line = strings.TrimSuffix(line, "*/")
	line = strings.TrimPrefix(line, "*")
	line = strings.TrimSuffix(line, "*")
	return strings.TrimSpace(line)
}
