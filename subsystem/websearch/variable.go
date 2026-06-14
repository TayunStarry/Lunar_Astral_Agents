package websearch

import "time"

// defaultConfig 默认配置
var defaultConfig = Config{
	Shallow: ShallowConfig{
		MaxResults: 10,
	},
	Deep: DeepConfig{
		MaxResults:       30,
		FetchContent:     true,
		FetchTimeout:     10,
		MaxContentLength: 2000,
	},
	Research: ResearchConfig{
		MaxResults:    10,
		MaxSubQueries: 6,
	},
	LLM: LLMConfig{
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

// DefaultConfig 返回默认配置
func DefaultConfig() Config {
	return defaultConfig
}
