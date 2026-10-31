package weather

// variable.go 天气查询的常量、正则与共享 HTTP 客户端。
//
// 本能力移植自本地包 lunar.search-weather：和风天气为主源，Open-Meteo 与 wttr.in
// 作为免费回退源。和风天气凭据在包内硬编码，调用方无需再读前端配置。

import (
	"net/http"
	"regexp"
	"time"
)

const (
	// QWeatherHost 和风天气 API Host（专属域名）
	QWeatherHost = "nn3qqtp7v4.re.qweatherapi.com"
	// QWeatherKey 和风天气 API Key（从 lunar.search-weather 前端包内置过来）
	QWeatherKey = "73cf082a6a7148da9dcf1549fb58694f"
	// QueryTimeout 单次天气查询的总超时（含多源回退）
	QueryTimeout = 45 * time.Second
	// forecastDays 预报天数（和风 3d / Open-Meteo 3 天 / wttr 2 天）
	forecastDays = 3
	// wttrDayCount wttr 预报取前 2 天（与前端包一致）
	wttrDayCount = 2
	// requestTimeout 单个 HTTP 请求超时
	requestTimeout = 20 * time.Second
	// userAgent 请求标识，部分免费源会拒绝空 UA
	userAgent = "LunarAstral/1.0 (weather-agent)"
)

// httpClient 天气请求专用客户端（不复用其余模块的全局客户端设置）
var httpClient = &http.Client{Timeout: requestTimeout}

// wmoCodes WMO 天气代码 → 中文描述（Open-Meteo 使用）
var wmoCodes = map[int]string{
	0: "晴天", 1: "大部晴朗", 2: "多云", 3: "阴天",
	45: "雾", 48: "冻雾",
	51: "小毛毛雨", 53: "毛毛雨", 55: "大毛毛雨",
	56: "小冻毛毛雨", 57: "大冻毛毛雨",
	61: "小雨", 63: "中雨", 65: "大雨",
	66: "小冻雨", 67: "大冻雨",
	71: "小雪", 73: "中雪", 75: "大雪", 77: "雪粒",
	80: "小阵雨", 81: "阵雨", 82: "大阵雨",
	85: "小阵雪", 86: "大阵雪",
	95: "雷暴", 96: "小冰雹雷暴", 99: "大冰雹雷暴",
}

// windDirections 八方位中文名（Open-Meteo 风向角 → 名称）
var windDirections = []string{"北", "东北", "东", "东南", "南", "西南", "西", "西北"}

// providerLabels 数据源中文名（报告与日志使用）
var providerLabels = map[string]string{
	"qweather":  "和风天气",
	"openmeteo": "Open-Meteo",
	"wttr":      "wttr.in",
}

// cjkPattern 判断输入是否已是中文（中文城市名优先保留原样展示）
var cjkPattern = regexp.MustCompile(`[\x{4e00}-\x{9fff}]`)
