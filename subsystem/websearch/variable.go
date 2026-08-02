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
		Enabled:                  true,
		MaxRounds:                1,
		MaxSubQueries:            6,
		MaxSupplementarySearches: 3,
	},
	HTTP: HTTPConfig{
		Timeout:      10 * time.Second,
		UserAgent:    "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36",
		MaxRetries:   2,
		RetryBackoff: 500 * time.Millisecond,
	},
}
