package agentdispatch

// variable.go 统一原语的字段名、默认描述与校验错误文案。

import "errors"

const (
	// FieldType 分支类型字段
	FieldType = "type"
	// FieldInstruction 自然语言任务字段
	FieldInstruction = "instruction"
	// FieldHandle 目标句柄列表字段
	FieldHandle = "handle"
	// FieldDetails 职能描述查询字段
	FieldDetails = "details"
)

const (
	// defaultHandleDesc handle 字段的默认说明
	defaultHandleDesc = "本次操作涉及的目标句柄列表；没有指定目标时必须传空数组 []。"
	// defaultDetailsDesc details 字段的默认说明
	defaultDetailsDesc = "为 true 时只返回该分支智能体的职能范围描述，不执行任何实际操作；默认 false。"
	// unknownFieldHint 未知字段错误提示
	unknownFieldHint = "输入格式错误：只允许 type / instruction / handle / details 四个字段"
)

var (
	// ErrTypeType type 字段类型错误
	ErrTypeType = errors.New("type 必须是字符串")
	// ErrTypeRequired type 字段缺失
	ErrTypeRequired = errors.New("type 不能为空，必须与工具注册的类型标识完全匹配")
	// ErrInstructionType instruction 字段类型错误
	ErrInstructionType = errors.New("instruction 必须是字符串")
	// ErrInstructionControl instruction 含控制字符
	ErrInstructionControl = errors.New("instruction 不能包含特殊控制字符")
	// ErrHandleType handle 字段类型错误
	ErrHandleType = errors.New("handle 必须是字符串数组")
	// ErrHandleItem handle 元素类型错误
	ErrHandleItem = errors.New("handle 的每一项都必须是字符串")
	// ErrDetailsType details 字段类型错误
	ErrDetailsType = errors.New("details 必须是布尔值")
)
