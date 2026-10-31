package weather

// text.go 把归一化天气数据渲染成回填给模型的紧凑中文文本。

import (
	"fmt"
	"strings"
)

// Render 把天气数据渲染为多行文本（当前天气 + 空气质量 + 预报）。
func Render(data Data) string {
	current := data.Current
	lines := make([]string, 0, 8+len(data.Forecast))
	lines = append(lines, fmt.Sprintf("【%s】当前天气（%s · 策略 %s）：", data.City, providerName(data), data.Strategy))
	lines = append(lines, fmt.Sprintf("- 气温：%d°C（体感 %d°C）", current.Temperature, current.FeelsLike))
	lines = append(lines, "- 天气状况："+firstOrDefault(current.Condition, "未知"))
	lines = append(lines, fmt.Sprintf("- 湿度：%d%%", current.Humidity))
	lines = append(lines, "- 风力："+firstOrDefault(current.Wind, "-"))
	if current.Visibility > 0 {
		lines = append(lines, fmt.Sprintf("- 能见度：%gkm", current.Visibility))
	}
	if data.AQI != nil {
		line := fmt.Sprintf("- 空气质量：%s（AQI %d", firstOrDefault(data.AQI.Category, firstOrDefault(data.AQI.Level, "未知")), data.AQI.Value)
		if data.AQI.Primary != "" {
			line += "，主要污染物 " + data.AQI.Primary
		}
		lines = append(lines, line+"）")
	}
	for _, day := range data.Forecast {
		line := fmt.Sprintf("- 预报 %s：%s，%d°C ~ %d°C", day.Date, firstOrDefault(day.Condition, "未知"), day.Low, day.High)
		if day.PrecipProb != nil {
			line += fmt.Sprintf("，降水概率 %d%%", *day.PrecipProb)
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

// providerName 数据源展示名（未登记时用 Source）
func providerName(data Data) string {
	if name, ok := providerLabels[data.Provider]; ok {
		return name
	}
	if data.Source != "" {
		return data.Source
	}
	return data.Provider
}
