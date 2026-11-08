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
	// lastSeq 该文件最后一次写入对应的全局序号（增量同步游标）。
	lastSeq uint64
}

// EditSnapshot 一次取回的状态集合。
// Total / Offset / Limit / HasMore 支持分页；Reset 表示增量游标已失效，需要全量重读。
type EditSnapshot struct {
	Version uint64      `json:"version"`
	Files   []FileState `json:"files"`
	Total   int         `json:"total,omitempty"`
	Offset  int         `json:"offset,omitempty"`
	Limit   int         `json:"limit,omitempty"`
	HasMore bool        `json:"has_more,omitempty"`
	Reset   bool        `json:"reset,omitempty"`
}

// EditQuery 状态查询条件：Since 为增量游标（0 表示全量），Offset/Limit 为分页。
type EditQuery struct {
	Since  uint64
	Offset int
	Limit  int
}

// HistoryQuery 单文件历史记录分页条件（Before 为上一页返回的游标，0 表示最新）。
type HistoryQuery struct {
	Path   string
	Before uint64
	Limit  int
}

// HistoryPage 单文件历史记录的一页（按时间倒序，最新在前）。
type HistoryPage struct {
	Path       string       `json:"path"`
	Records    []EditRecord `json:"records"`
	Total      int          `json:"total"`
	NextBefore uint64       `json:"next_before,omitempty"`
	HasMore    bool         `json:"has_more"`
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

// DBTable 数据库中的一张表或视图。
type DBTable struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

// DBColumn 表结构中的一列。
type DBColumn struct {
	Name    string `json:"name"`
	Type    string `json:"type"`
	NotNull bool   `json:"not_null"`
	PK      bool   `json:"pk"`
	Default string `json:"default"`
}

// DBTablesResponse 数据库表清单回执。
type DBTablesResponse struct {
	OK     bool      `json:"ok"`
	Path   string    `json:"path"`
	Tables []DBTable `json:"tables"`
	Error  string    `json:"error,omitempty"`
}

// DBSchemaResponse 表结构回执。
type DBSchemaResponse struct {
	OK      bool       `json:"ok"`
	Path    string     `json:"path"`
	Table   string     `json:"table"`
	Columns []DBColumn `json:"columns"`
	Error   string     `json:"error,omitempty"`
}

// DBRowsResponse 分页行数据回执。Rows 按 Columns 顺序排列，值已转为可 JSON 化的标量。
type DBRowsResponse struct {
	OK      bool     `json:"ok"`
	Path    string   `json:"path"`
	Table   string   `json:"table"`
	Columns []string `json:"columns"`
	Rows    [][]any  `json:"rows"`
	Total   int      `json:"total"`
	Offset  int      `json:"offset"`
	Limit   int      `json:"limit"`
	Sort    string   `json:"sort"`
	Order   string   `json:"order"`
	Error   string   `json:"error,omitempty"`
}

// DBQueryRequest 只读 SQL 执行请求。
type DBQueryRequest struct {
	Path string `json:"path"`
	SQL  string `json:"sql"`
}

// DBQueryResponse 只读 SQL 执行回执。Rows 按 Columns 顺序排列，值已转为可 JSON 化的标量。
type DBQueryResponse struct {
	OK        bool     `json:"ok"`
	Path      string   `json:"path"`
	SQL       string   `json:"sql"`
	Columns   []string `json:"columns"`
	Rows      [][]any  `json:"rows"`
	RowCount  int      `json:"row_count"`
	Truncated bool     `json:"truncated"`
	ElapsedMS int64    `json:"elapsed_ms"`
	Error     string   `json:"error,omitempty"`
}

// DBSQLRequest 自然语言转 SQL 请求。
type DBSQLRequest struct {
	Path     string `json:"path"`
	Question string `json:"question"`
}

// DBSQLResponse 自然语言转 SQL 回执：只返回一条只读 SQL，不执行。
type DBSQLResponse struct {
	OK    bool   `json:"ok"`
	Path  string `json:"path"`
	SQL   string `json:"sql"`
	Error string `json:"error,omitempty"`
}

// ArchiveEntry 压缩包内一个条目（只读元数据，不解压内容）。
type ArchiveEntry struct {
	Name     string `json:"name"`
	Path     string `json:"path"`
	Type     string `json:"type"`
	Size     int64  `json:"size"`
	Modified int64  `json:"modified"`
}

// ArchiveResponse 压缩包条目分页回执。
type ArchiveResponse struct {
	OK        bool           `json:"ok"`
	Path      string         `json:"path"`
	Format    string         `json:"format"`
	Dir       string         `json:"dir"`
	Entries   []ArchiveEntry `json:"entries"`
	Total     int            `json:"total"`
	Offset    int            `json:"offset"`
	Limit     int            `json:"limit"`
	HasMore   bool           `json:"has_more"`
	Truncated bool           `json:"truncated"`
	Error     string         `json:"error,omitempty"`
}
