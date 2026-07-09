package websearch

import "time"

// defaultConfig 默认配置
var defaultConfig = Config{
	Simple: SimpleConfig{
		MaxResults: 10,
	},
	Webpage: WebpageConfig{
		MaxResults:       30,
		FetchContent:     true,
		FetchTimeout:     10,
		MaxContentLength: 2000,
	},
	Depth: DepthConfig{
		Enabled:       true,
		MaxRounds:     1,
		MaxResults:    10,
		MaxSubQueries: 6,
	},
	LLM: llmConfig{
		BaseURL:     "https://api.openai.com/v1",
		Model:       "gpt-4o-mini",
		MaxTokens:   4096,
		Temperature: 0.7,
	},
	HTTP: HTTPConfig{
		Timeout:   10 * time.Second,
		UserAgent: "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
	},
}
