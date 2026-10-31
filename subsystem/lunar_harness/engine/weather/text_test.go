package weather

// text_test.go 天气文本渲染的回归测试。

import (
	"strings"
	"testing"
)

// TestRenderWeatherText 渲染结果必须包含当前天气与预报关键信息
func TestRenderWeatherText(t *testing.T) {
	precip := 40
	data := Data{
		City:     "北京",
		Provider: "qweather",
		Strategy: "auto",
		Source:   "和风天气 (API Key)",
		Current:  Current{Temperature: 12, FeelsLike: 10, Condition: "小雨", Humidity: 66, Wind: "东南风 12km/h", Visibility: 8.5},
		AQI:      &AirQuality{Value: 42, Category: "优", Primary: "PM2.5"},
		Forecast: []ForecastDay{{Date: "2026-10-31", High: 15, Low: 6, Condition: "多云", PrecipProb: &precip}},
	}
	text := Render(data)
	for _, want := range []string{"北京", "和风天气", "12°C", "体感 10°C", "小雨", "66%", "能见度：8.5km", "AQI 42", "2026-10-31", "降水概率 40%"} {
		if !strings.Contains(text, want) {
			t.Fatalf("渲染结果缺少 %q:\n%s", want, text)
		}
	}
}
