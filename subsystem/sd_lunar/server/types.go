package server

type Txt2ImgRequest struct {
	Prompt         string  `json:"prompt"`
	NegativePrompt string  `json:"negative_prompt"`
	Width          int     `json:"width"`
	Height         int     `json:"height"`
	Steps          int     `json:"steps"`
	Sampler        string  `json:"sampler"`
	Scheduler      string  `json:"scheduler"`
	CFGScale       float64 `json:"cfg_scale"`
	Seed           int64   `json:"seed"`
	ClipSkip       int     `json:"clip_skip"`
}

type Img2ImgRequest struct {
	Prompt         string  `json:"prompt"`
	NegativePrompt string  `json:"negative_prompt"`
	Width          int     `json:"width"`
	Height         int     `json:"height"`
	Steps          int     `json:"steps"`
	Sampler        string  `json:"sampler"`
	Scheduler      string  `json:"scheduler"`
	CFGScale       float64 `json:"cfg_scale"`
	Seed           int64   `json:"seed"`
	ClipSkip       int     `json:"clip_skip"`
	Strength       float64 `json:"strength"`
	ImageBase64    string  `json:"image_base64"`
}

type GenerateResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message,omitempty"`
	Data    string `json:"data,omitempty"`
	Width   int    `json:"width,omitempty"`
	Height  int    `json:"height,omitempty"`
	Seed    int64  `json:"seed,omitempty"`
}

type StatusResponse struct {
	Success bool   `json:"success"`
	Ready   bool   `json:"ready"`
	Info    string `json:"info,omitempty"`
	Message string `json:"message,omitempty"`
}
