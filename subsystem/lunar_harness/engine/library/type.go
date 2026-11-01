package library

// type.go 资源库（内嵌前端）的数据类型定义。
//
// 资源库只读展示内容、按 4 个存储根管理文件名与位置，不接受任何正文编辑。

import "time"

// Entry 资源库中的一个文件或目录（路径相对 LocalDir）。
type Entry struct {
	Root  string `json:"root"`
	Path  string `json:"path"`
	Name  string `json:"name"`
	Dir   bool   `json:"dir"`
	Size  int64  `json:"size"`
	MTime int64  `json:"mtime"`
	Ext   string `json:"ext,omitempty"`
}

// Tree 一次递归清单。
type Tree struct {
	Roots     []string `json:"roots"`
	Entries   []Entry  `json:"entries"`
	Truncated bool     `json:"truncated"`
}

// Metadata 非文本文件在详情区固定展示的字段。
type Metadata struct {
	Path           string `json:"path"`
	Name           string `json:"name"`
	Ext            string `json:"ext"`
	Size           int64  `json:"size"`
	MTime          int64  `json:"mtime"`
	Handle         string `json:"handle"`
	Kind           string `json:"kind"`
	CreatedAt      int64  `json:"created_at"`
	Points         int    `json:"points"`
	HasDescription bool   `json:"has_description"`
	Summary        string `json:"summary"`
	State          string `json:"state"`
	MIME           string `json:"mime"`

	// GCTracked 是否处于积分回收范围（文档、子目录与普通音频不参与）
	GCTracked bool `json:"gc_tracked"`
	// GCCyclesLeft 持续未使用时，距回收还剩几次结算
	GCCyclesLeft int `json:"gc_cycles_left"`
	// GCCycleHours 结算周期（小时）
	GCCycleHours int `json:"gc_cycle_hours"`
	// GCProtected 有描述：触发回收时只删除文件本体，句柄与描述保留
	GCProtected bool `json:"gc_protected"`
}

// EditRecord 一次由 AI 触发的文件操作，携带新增与删除的文本片段。
type EditRecord struct {
	Op        string `json:"op"`
	Added     string `json:"added,omitempty"`
	Removed   string `json:"removed,omitempty"`
	StartLine int    `json:"start_line,omitempty"`
	Lines     int    `json:"lines,omitempty"`
	At        int64  `json:"at"`
}

// FileState 一个文件的视觉状态与最近操作记录。
type FileState struct {
	Path    string       `json:"path"`
	Status  string       `json:"status"`
	Op      string       `json:"op"`
	Failed  bool         `json:"failed"`
	Handle  string       `json:"handle,omitempty"`
	At      int64        `json:"at"`
	Records []EditRecord `json:"records"`

	// hasRun / runOK 运行状态内部标记：runOK 为 true 时文件名保持绿色，
	// 直到下一次运行失败转红。
	hasRun bool
	runOK  bool
}

// EditSnapshot 一次取回的状态集合。
type EditSnapshot struct {
	Version uint64      `json:"version"`
	Files   []FileState `json:"files"`
}

// EditNotice 文件编辑智能体的一次上报。
type EditNotice struct {
	Path      string
	Op        string
	Added     string
	Removed   string
	StartLine int
	Lines     int
	Handle    string
	Failed    bool
	At        time.Time
}

// OpRequest 重命名 / 移动 / 删除请求。
type OpRequest struct {
	Path   string `json:"path"`
	Name   string `json:"name,omitempty"`
	Target string `json:"target,omitempty"`
}

// OpResponse 管理操作回执。
type OpResponse struct {
	OK    bool   `json:"ok"`
	Path  string `json:"path,omitempty"`
	Error string `json:"error,omitempty"`
}

// MetaResponse 详情区元数据回执。
type MetaResponse struct {
	OK    bool     `json:"ok"`
	Meta  Metadata `json:"meta"`
	Error string   `json:"error,omitempty"`
}
