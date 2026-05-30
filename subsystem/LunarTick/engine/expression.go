package engine

import (
	"regexp"
	"strings"
)

var (
	varPattern = regexp.MustCompile(`#([a-zA-Z_][a-zA-Z0-9_]*)`)
	ptrPattern = regexp.MustCompile(`\*([a-zA-Z_][a-zA-Z0-9_]*)`)
)

func EvalExpression(expr string, vs *VarStore, pr *PointerRegistry) string {
	var result strings.Builder
	i := 0
	runes := []rune(expr)

	for i < len(runes) {
		if runes[i] == '\'' {
			i++
			for i < len(runes) {
				if runes[i] == '\\' && i+1 < len(runes) {
					i += 2
					continue
				}
				if runes[i] == '\'' {
					if i+1 < len(runes) && runes[i+1] == '\'' {
						result.WriteRune('\'')
						i += 2
						continue
					}
					i++
					break
				}
				result.WriteRune(runes[i])
				i++
			}
			continue
		}
		if runes[i] == '"' {
			i++
			for i < len(runes) {
				if runes[i] == '\\' && i+1 < len(runes) {
					i += 2
					continue
				}
				if runes[i] == '"' {
					i++
					break
				}
				result.WriteRune(runes[i])
				i++
			}
			continue
		}
		if runes[i] == '#' {
			end := i + 1
			for end < len(runes) && (isAlphanumeric(runes[end]) || runes[end] == '_' || runes[end] == '?') {
				end++
			}
			if end > i+1 {
				varName := string(runes[i+1 : end])
				val := vs.Get(varName)
				result.WriteString(val)
				i = end
				continue
			}
		}
		if runes[i] == '*' {
			end := i + 1
			for end < len(runes) && (isAlphanumeric(runes[end]) || runes[end] == '_') {
				end++
			}
			if end > i+1 {
				ptrName := string(runes[i+1 : end])
				if pr != nil && pr.Exists(ptrName) {
					result.WriteString("true")
				} else {
					result.WriteString("false")
				}
				i = end
				continue
			}
		}
		if runes[i] == '+' {
			if result.Len() > 0 {
				last := result.String()[result.Len()-1]
				if last == ' ' {
					tmp := result.String()[:result.Len()-1]
					result.Reset()
					result.WriteString(tmp)
				}
			}
			i++
			for i < len(runes) && isSpaceRune(runes[i]) {
				i++
			}
			continue
		}
		result.WriteRune(runes[i])
		i++
	}

	return result.String()
}

func EvalCondition(cond string, vs *VarStore, pr *PointerRegistry) bool {
	cond = strings.TrimSpace(cond)
	if cond == "" || cond == "false" {
		return false
	}
	if cond == "true" {
		return true
	}

	if strings.Contains(cond, "&") {
		parts := splitConditional(cond, "&")
		return EvalCondition(strings.TrimSpace(parts[0]), vs, pr) &&
			EvalCondition(strings.TrimSpace(parts[1]), vs, pr)
	}

	if strings.Contains(cond, "|") {
		parts := splitConditional(cond, "|")
		return EvalCondition(strings.TrimSpace(parts[0]), vs, pr) ||
			EvalCondition(strings.TrimSpace(parts[1]), vs, pr)
	}

	if strings.HasPrefix(cond, "!") {
		return !EvalCondition(strings.TrimSpace(cond[1:]), vs, pr)
	}

	resolved := EvalExpression(cond, vs, pr)

	ops := []string{">=", "<=", "!=", "=", ">", "<"}
	for _, op := range ops {
		if idx := strings.Index(resolved, op); idx >= 0 {
			left := strings.TrimSpace(resolved[:idx])
			right := strings.TrimSpace(resolved[idx+len(op):])
			switch op {
			case "=":
				return left == right
			case "!=":
				return left != right
			case ">":
				return left > right
			case "<":
				return left < right
			case ">=":
				return left >= right
			case "<=":
				return left <= right
			}
		}
	}

	return resolved != "" && resolved != "false"
}

func splitConditional(s, sep string) []string {
	depth := 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '\'', '"':
			i = skipQuoted(s, i)
		case '(':
			depth++
		case ')':
			depth--
		default:
			if depth == 0 && string(s[i]) == sep {
				return []string{s[:i], s[i+1:]}
			}
		}
	}
	return []string{s, ""}
}

func skipQuoted(s string, start int) int {
	quote := s[start]
	for i := start + 1; i < len(s); i++ {
		if s[i] == '\\' {
			i++
			continue
		}
		if s[i] == quote {
			return i
		}
	}
	return len(s) - 1
}

func isAlphanumeric(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9')
}

func isSpaceRune(r rune) bool {
	return r == ' ' || r == '\t'
}
