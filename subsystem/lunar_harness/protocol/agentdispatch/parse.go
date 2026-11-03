package agentdispatch

// parse.go 统一输入的解析与合法性校验。
//
// 解析严格限定四个字段：出现未知字段、字段类型不符、type 为空时一律判为格式错误。
// handle / details 缺省时按规范语义取空数组与 false，其余字段不做补全。

import (
	"fmt"
	"sort"
	"strings"
)

// allowedFields 统一输入允许出现的字段集合。
var allowedFields = map[string]bool{
	FieldType:        true,
	FieldInstruction: true,
	FieldHandle:      true,
	FieldDetails:     true,
}

// Parse 解析并校验统一输入原语。
func Parse(args map[string]any) (Call, error) {
	call := Call{Handle: []string{}}
	if err := rejectUnknownFields(args); err != nil {
		return Call{}, err
	}
	if raw, exists := args[FieldType]; exists {
		text, ok := raw.(string)
		if !ok {
			return Call{}, ErrTypeType
		}
		call.Type = strings.TrimSpace(text)
	}
	if call.Type == "" {
		return Call{}, ErrTypeRequired
	}
	if raw, exists := args[FieldInstruction]; exists && raw != nil {
		text, ok := raw.(string)
		if !ok {
			return Call{}, ErrInstructionType
		}
		call.Instruction = strings.TrimSpace(text)
		if hasControlRunes(call.Instruction) {
			return Call{}, ErrInstructionControl
		}
	}
	if raw, exists := args[FieldHandle]; exists && raw != nil {
		handles, err := parseHandles(raw)
		if err != nil {
			return Call{}, err
		}
		call.Handle = handles
	}
	if raw, exists := args[FieldDetails]; exists && raw != nil {
		details, ok := raw.(bool)
		if !ok {
			return Call{}, ErrDetailsType
		}
		call.Details = details
	}
	return call, nil
}

// Schema 构造统一输入的函数参数 schema。
//
// branchDesc 描述 type 的取值含义；branches 非空时写入枚举；
// handleDesc 用于补充该工具对目标句柄的额外约定。
func Schema(branchDesc string, branches []string, instructionDesc, handleDesc string) map[string]any {
	typeProp := map[string]any{"type": "string", "description": branchDesc}
	if len(branches) > 0 {
		typeProp["enum"] = append([]string(nil), branches...)
	}
	handleText := defaultHandleDesc
	if strings.TrimSpace(handleDesc) != "" {
		handleText += " " + strings.TrimSpace(handleDesc)
	}
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			FieldType:        typeProp,
			FieldInstruction: map[string]any{"type": "string", "description": instructionDesc},
			FieldHandle: map[string]any{
				"type":        "array",
				"items":       map[string]any{"type": "string"},
				"description": handleText,
			},
			FieldDetails: map[string]any{
				"type":        "boolean",
				"description": defaultDetailsDesc,
				"default":     false,
			},
		},
		"required": []string{FieldType, FieldInstruction, FieldHandle, FieldDetails},
	}
}

// rejectUnknownFields 拒绝规范以外的输入字段。
func rejectUnknownFields(args map[string]any) error {
	unknown := make([]string, 0)
	for name := range args {
		if !allowedFields[name] {
			unknown = append(unknown, name)
		}
	}
	if len(unknown) == 0 {
		return nil
	}
	sort.Strings(unknown)
	return fmt.Errorf("%s；收到不支持的字段：%s", unknownFieldHint, strings.Join(unknown, "、"))
}

// parseHandles 解析 handle 字段：只接受字符串数组，空元素会被丢弃。
func parseHandles(raw any) ([]string, error) {
	switch value := raw.(type) {
	case []string:
		return trimHandles(value), nil
	case []any:
		items := make([]string, 0, len(value))
		for _, item := range value {
			text, ok := item.(string)
			if !ok {
				return nil, ErrHandleItem
			}
			items = append(items, text)
		}
		return trimHandles(items), nil
	}
	return nil, ErrHandleType
}

// trimHandles 去掉首尾空白并丢弃空元素。
func trimHandles(items []string) []string {
	handles := make([]string, 0, len(items))
	for _, item := range items {
		if text := strings.TrimSpace(item); text != "" {
			handles = append(handles, text)
		}
	}
	return handles
}

// hasControlRunes 判断文本是否含控制字符（换行、制表与回车除外）。
func hasControlRunes(text string) bool {
	for _, char := range text {
		if char == '\n' || char == '\t' || char == '\r' {
			continue
		}
		if char < 0x20 || char == 0x7f {
			return true
		}
	}
	return false
}
