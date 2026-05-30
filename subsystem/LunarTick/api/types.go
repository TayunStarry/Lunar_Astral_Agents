package api

type RunRequest struct {
	Path string   `json:"path"`
	Args []string `json:"args"`
}

type RunResponse struct {
	BlockID  string `json:"block_id"`
	Status   string `json:"status"`
}

type InjectRequest struct {
	Lines []string `json:"lines"`
}

type InjectResponse struct {
	BlockCount int    `json:"block_count"`
	Status     string `json:"status"`
}

type InvokeRequest struct {
	Pointer string `json:"pointer"`
}

type InvokeResponse struct {
	Status string `json:"status"`
}

type StatusResponse struct {
	Running      bool              `json:"running"`
	Suspended    bool              `json:"suspended"`
	TickNumber   int               `json:"tick_number"`
	ReadyBlocks  int               `json:"ready_blocks"`
	WaitingBlocks int              `json:"waiting_blocks"`
	Variables    map[string]string `json:"variables"`
	Pointers     []string          `json:"pointers"`
	Errors       []ErrorEntry      `json:"errors,omitempty"`
}

type ErrorEntry struct {
	BlockID    string `json:"block_id"`
	Message    string `json:"message"`
	TickNumber int    `json:"tick_number"`
}

type VariableRequest struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type VariableResponse struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type LoadRequest struct {
	Source string `json:"source"`
	Format string `json:"format"`
}

type LoadResponse struct {
	Status     string `json:"status"`
	BlockCount int    `json:"block_count"`
}

type ErrorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message,omitempty"`
}