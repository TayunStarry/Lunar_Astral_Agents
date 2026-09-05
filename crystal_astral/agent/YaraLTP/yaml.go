package YaraLTP

// ==== 极简 YAML 子集解析器 ====
// LTP3 仅支持 config.yaml（要求5），且无需重活第三方 YAML 库：
// 支持注释、键值映射、缩进嵌套、序列（- item）、内联数组/对象、引号字符串，
// 纯 stdlib 实现，仅做自动类型推断（null/bool/int/float/string）。

import (
	"fmt"
	"strconv"
	"strings"
)

// yamlLine 单行 YAML（已去除注释与空白信息保留缩进）。
type yamlLine struct {
	indent int
	text   string
}

// parseYAML 将 config.yaml 文本解析为 map[string]any，失败返回错误。
func parseYAML(data string) (map[string]any, error) {
	lines := splitYAMLLines(data)
	if len(lines) == 0 {
		return map[string]any{}, nil
	}
	root, _, err := yamlParseBlock(lines, 0, 0)
	if err != nil {
		return nil, err
	}
	m, ok := root.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("config.yaml 根节点必须是映射")
	}
	return m, nil
}

// splitYAMLLines 切分 YAML 文本为带缩进的行，剔除空行与纯注释行。
func splitYAMLLines(data string) []yamlLine {
	raw := strings.Split(data, "\n")
	out := make([]yamlLine, 0, len(raw))
	for _, line := range raw {
		if strings.TrimRight(line, "\r") != "" {
			stripped := strings.TrimRight(line, "\r\t ")
			content := strings.TrimSpace(stripped)
			if content == "" || strings.HasPrefix(content, "#") {
				continue
			}
			indent := len(stripped) - len(strings.TrimLeft(stripped, " "))
			out = append(out, yamlLine{indent: indent, text: content})
		}
	}
	return out
}

// yamlParseBlock 从 lines[pos] 开始解析同缩进节点，返回 (值, 已消费行数)。
func yamlParseBlock(lines []yamlLine, pos, indent int) (any, int, error) {
	if pos >= len(lines) {
		return nil, 0, nil
	}
	// 序列块：当前行以 "- " 开头且属于本块缩进
	if isSeqItem(lines[pos].text) && (pos == 0 || lines[pos].indent == indent) {
		return yamlParseSeq(lines, pos, indent)
	}
	// 映射块
	m := map[string]any{}
	i := pos
	for i < len(lines) {
		ln := lines[i]
		if ln.indent < indent {
			break
		}
		if ln.indent > indent {
			// 缩进更深：属于某键的嵌套值，由上层解析处理；这里不应进入
			break
		}
		key, rest, ok := splitYAMLKey(ln.text)
		if !ok {
			return nil, 0, fmt.Errorf("YAML 语法错误（第 %d 行）: %s", i+1, ln.text)
		}
		key = unquoteYAML(key)
		// 有内联值
		if strings.TrimSpace(rest) != "" {
			val, err := yamlScalar(strings.TrimSpace(rest))
			if err != nil {
				return nil, 0, err
			}
			m[key] = val
			i++
			continue
		}
		// 无内联值 → 看下一行是否缩进子块
		if i+1 < len(lines) && lines[i+1].indent > indent {
			child, consumed, err := yamlParseBlock(lines, i+1, lines[i+1].indent)
			if err != nil {
				return nil, 0, err
			}
			m[key] = child
			i += 1 + consumed
			continue
		}
		// 空值
		m[key] = nil
		i++
	}
	return m, i - pos, nil
}

// yamlParseSeq 解析以 "- item" 起始的序列。
func yamlParseSeq(lines []yamlLine, pos, indent int) (any, int, error) {
	s := []any{}
	i := pos
	for i < len(lines) {
		ln := lines[i]
		if ln.indent != indent || !isSeqItem(ln.text) {
			break
		}
		rest := strings.TrimSpace(strings.TrimLeft(ln.text, "- "))
		if rest != "" {
			// 内联序列项
			if strings.HasPrefix(rest, "{") || strings.HasPrefix(rest, "[") {
				val, err := yamlScalar(rest)
				if err != nil {
					return nil, 0, err
				}
				s = append(s, val)
				i++
				continue
			}
			val, err := yamlScalar(rest)
			if err != nil {
				return nil, 0, err
			}
			s = append(s, val)
			i++
			continue
		}
		// "- " 后为空 → 子块（- key: value）
		if i+1 < len(lines) && lines[i+1].indent > indent {
			child, consumed, err := yamlParseBlock(lines, i+1, lines[i+1].indent)
			if err != nil {
				return nil, 0, err
			}
			s = append(s, child)
			i += 1 + consumed
			continue
		}
		s = append(s, nil)
		i++
	}
	return s, i - pos, nil
}

// splitYAMLKey 在冒号处拆分 "key: rest"；跳过引号内的冒号。
func splitYAMLKey(text string) (key, rest string, ok bool) {
	inS, inD := false, false
	for i, r := range text {
		switch r {
		case '\'':
			if !inD {
				inS = !inS
			}
		case '"':
			if !inS {
				inD = !inD
			}
		case ':':
			if !inS && !inD {
				return text[:i], text[i+1:], true
			}
		}
	}
	return "", "", false
}

func isSeqItem(text string) bool {
	t := strings.TrimSpace(text)
	return strings.HasPrefix(t, "- ") || t == "-"
}

// yamlScalar 解析一个标量 / 内联数组 / 内联对象。
func yamlScalar(s string) (any, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, nil
	}
	if strings.HasPrefix(s, "[") && strings.HasSuffix(s, "]") {
		return yamlParseInlineArray(s[1 : len(s)-1])
	}
	if strings.HasPrefix(s, "{") && strings.HasSuffix(s, "}") {
		return yamlParseInlineObject(s[1 : len(s)-1])
	}
	if strings.HasPrefix(s, "'") && strings.HasSuffix(s, "'") && len(s) >= 2 {
		return strings.ReplaceAll(s[1:len(s)-1], "''", "'"), nil
	}
	if strings.HasPrefix(s, `"`) && strings.HasSuffix(s, `"`) && len(s) >= 2 {
		u, err := strconv.Unquote(s)
		if err != nil {
			return s[1 : len(s)-1], nil
		}
		return u, nil
	}
	low := strings.ToLower(s)
	switch low {
	case "null", "~":
		return nil, nil
	case "true":
		return true, nil
	case "false":
		return false, nil
	}
	if i, err := strconv.ParseInt(s, 10, 64); err == nil {
		return i, nil
	}
	if f, err := strconv.ParseFloat(s, 64); err == nil {
		return f, nil
	}
	return s, nil
}

func yamlParseInlineArray(body string) ([]any, error) {
	items := strings.Split(body, ",")
	out := make([]any, 0, len(items))
	for _, it := range items {
		if strings.TrimSpace(it) == "" {
			continue
		}
		v, err := yamlScalar(strings.TrimSpace(it))
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, nil
}

func yamlParseInlineObject(body string) (map[string]any, error) {
	m := map[string]any{}
	// 按逗号分割键值对（不做深度嵌套，纯 Go 内联对象为宽松子集）
	for _, pair := range strings.Split(body, ",") {
		if strings.TrimSpace(pair) == "" {
			continue
		}
		k, v, ok := splitYAMLKey(pair)
		if !ok {
			continue
		}
		val, err := yamlScalar(strings.TrimSpace(v))
		if err != nil {
			return nil, err
		}
		m[unquoteYAML(strings.TrimSpace(k))] = val
	}
	return m, nil
}

func unquoteYAML(s string) string {
	s = strings.TrimSpace(s)
	if len(s) >= 2 {
		if (strings.HasPrefix(s, "'") && strings.HasSuffix(s, "'")) ||
			(strings.HasPrefix(s, `"`) && strings.HasSuffix(s, `"`)) {
			return s[1 : len(s)-1]
		}
	}
	return s
}

// marshalYAML 把 map 序列化回 YAML 文本（config.setFile 用）。
func marshalYAML(root map[string]any) string {
	var b strings.Builder
	writeYAMLMap(&b, root, 0)
	return b.String()
}

func writeYAMLMap(b *strings.Builder, m map[string]any, indent int) {
	pad := strings.Repeat("  ", indent)
	for k, v := range m {
		switch tv := v.(type) {
		case map[string]any:
			b.WriteString(pad)
			b.WriteString(k)
			b.WriteString(":\n")
			writeYAMLMap(b, tv, indent+1)
		case []any:
			b.WriteString(pad)
			b.WriteString(k)
			b.WriteString(":\n")
			writeYAMLSeq(b, tv, indent+1)
		case nil:
			b.WriteString(pad)
			b.WriteString(k)
			b.WriteString(": null\n")
		default:
			b.WriteString(pad)
			b.WriteString(k)
			b.WriteString(": ")
			b.WriteString(yamlJSValue(v))
			b.WriteString("\n")
		}
	}
}

func writeYAMLSeq(b *strings.Builder, s []any, indent int) {
	pad := strings.Repeat("  ", indent)
	if len(s) == 0 {
		b.WriteString(pad)
		b.WriteString("- null\n")
		return
	}
	for _, v := range s {
		switch tv := v.(type) {
		case map[string]any:
			if len(tv) == 0 {
				b.WriteString(pad)
				b.WriteString("- {}\n")
				continue
			}
			// 序列内映射：首层键与 "- " 同行
			first := true
			for k, sv := range tv {
				if svMap, ok := sv.(map[string]any); ok {
					if first {
						b.WriteString(pad)
						b.WriteString("- ")
						b.WriteString(k)
						b.WriteString(":\n")
						writeYAMLMap(b, svMap, indent+1)
						first = false
					} else {
						b.WriteString(strings.Repeat("  ", indent))
						b.WriteString("  ")
						b.WriteString(k)
						b.WriteString(":\n")
						writeYAMLMap(b, svMap, indent+1)
					}
					continue
				}
				if first {
					b.WriteString(pad)
					b.WriteString("- ")
					b.WriteString(k)
					b.WriteString(": ")
					b.WriteString(yamlJSValue(sv))
					b.WriteString("\n")
					first = false
				} else {
					b.WriteString(strings.Repeat("  ", indent))
					b.WriteString("  ")
					b.WriteString(k)
					b.WriteString(": ")
					b.WriteString(yamlJSValue(sv))
					b.WriteString("\n")
				}
			}
		case []any:
			b.WriteString(pad)
			b.WriteString("-\n")
			writeYAMLSeq(b, tv, indent+1)
		default:
			b.WriteString(pad)
			b.WriteString("- ")
			b.WriteString(yamlJSValue(v))
			b.WriteString("\n")
		}
	}
}

// yamlJSValue 把 goja 导出值序列化为 YAML 标量字符串。
func yamlJSValue(v any) string {
	switch tv := v.(type) {
	case nil:
		return "null"
	case bool:
		if tv {
			return "true"
		}
		return "false"
	case string:
		// 需要引号：含特殊字符或空
		if tv == "" || strings.ContainsAny(tv, ":#[]{}'\",") || strings.HasPrefix(tv, " ") {
			return `"` + strings.ReplaceAll(tv, `"`, `\"`) + `"`
		}
		return tv
	case float64:
		return strconv.FormatFloat(tv, 'g', -1, 64)
	case int64:
		return strconv.FormatInt(tv, 10)
	case int:
		return strconv.Itoa(tv)
	default:
		return fmt.Sprintf("%v", v)
	}
}
