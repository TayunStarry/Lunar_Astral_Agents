package weather

// provider_wttr.go wttr.in 数据源：无需 Key，城市为空时按请求方 IP 自动定位。
// 返回当前天气与未来 2 天预报（wttr 的日级预报最多 3 天，这里取前 2 天）。

import (
	"context"
	"fmt"
	"net/url"
	"strings"
)

// queryWttr 用 wttr.in 查询天气；city 为空时由服务端按 IP 定位。
func queryWttr(ctx context.Context, city string) (Data, error) {
	target := url.PathEscape(strings.TrimSpace(city))
	endpoint := "https://wttr.in/" + target + "?format=j1&lang=zh"
	var raw wttrResponse
	if err := fetchJSON(ctx, endpoint, nil, &raw); err != nil {
		return Data{}, fmt.Errorf("天气请求失败: %w", err)
	}
	if len(raw.Current) == 0 {
		return Data{}, fmt.Errorf("天气数据解析失败")
	}
	current := raw.Current[0]

	displayCity := strings.TrimSpace(city)
	if displayCity == "" || !cjkPattern.MatchString(displayCity) {
		if len(raw.NearestArea) > 0 && len(raw.NearestArea[0].AreaName) > 0 {
			if name := strings.TrimSpace(raw.NearestArea[0].AreaName[0].Value); name != "" {
				displayCity = name
			}
		}
	}
	if displayCity == "" {
		displayCity = "当前位置"
	}

	data := Data{
		City:   displayCity,
		Source: "wttr.in",
		Current: Current{
			Temperature: numberToInt(current.TempC),
			FeelsLike:   numberToInt(current.FeelsLikeC),
			Condition:   firstOrDefault(wttrDescValue(current.WeatherDesc), "未知"),
			Humidity:    numberToInt(current.Humidity),
			Wind:        strings.TrimSpace(current.WindDir + " " + current.WindSpeed + "km/h"),
			Visibility:  formatVisibility(float64(numberToInt(current.Visibility))),
		},
	}

	for index, day := range raw.Weather {
		if index >= wttrDayCount {
			break
		}
		dayData := ForecastDay{
			Date:      day.Date,
			High:      numberToInt(day.MaxTemp),
			Low:       numberToInt(day.MinTemp),
			Condition: "未知",
		}
		// 取当日第 5 个三小时时段（约正午）作为日间天气描述，与前端包取 hourly[4] 一致
		if len(day.Hourly) > 4 {
			dayData.Condition = firstOrDefault(wttrDescValue(day.Hourly[4].WeatherDesc), "未知")
		}
		data.Forecast = append(data.Forecast, dayData)
	}
	return data, nil
}

// wttrDescValue 取第一段天气描述文本
func wttrDescValue(list []wttrDesc) string {
	if len(list) == 0 {
		return ""
	}
	return list[0].Value
}
