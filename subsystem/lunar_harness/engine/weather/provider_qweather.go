package weather

// provider_qweather.go 和风天气数据源：城市搜索 → 实时天气 → 3 天预报 → 空气质量。
//
// 凭据硬编码在 variable.go；城市搜索与实时天气是必需链路，预报与空气质量为可选增强，
// 单个可选接口失败不影响主结果。

import (
	"context"
	"fmt"
	"net/url"
	"strings"
)

// queryQWeather 用和风天气查询指定城市。
func queryQWeather(ctx context.Context, city string) (Data, error) {
	base := "https://" + strings.TrimSuffix(strings.TrimPrefix(strings.TrimPrefix(QWeatherHost, "https://"), "http://"), "/")
	headers := map[string]string{"X-QW-Api-Key": QWeatherKey}

	var geo qWeatherGeo
	if err := fetchJSON(ctx, base+"/geo/v2/city/lookup?location="+url.QueryEscape(city)+"&lang=zh", headers, &geo); err != nil {
		return Data{}, fmt.Errorf("城市搜索请求失败: %w", err)
	}
	if geo.Code != "200" {
		return Data{}, fmt.Errorf("城市搜索失败: code=%s", geo.Code)
	}
	if len(geo.Location) == 0 {
		return Data{}, fmt.Errorf("未找到城市：%s", city)
	}
	loc := geo.Location[0]

	var now qWeatherNow
	if err := fetchJSON(ctx, base+"/v7/weather/now?location="+url.QueryEscape(loc.ID), headers, &now); err != nil {
		return Data{}, fmt.Errorf("实时天气请求失败: %w", err)
	}
	if now.Code != "200" {
		return Data{}, fmt.Errorf("实时天气返回异常: code=%s", now.Code)
	}

	data := Data{
		City:   joinCity(loc.Name, loc.Adm1, loc.Country, city),
		Source: "和风天气 (API Key)",
		Current: Current{
			Temperature: numberToInt(now.Now.Temp),
			FeelsLike:   numberToInt(now.Now.FeelsLike),
			Condition:   firstOrDefault(now.Now.Text, "未知"),
			Humidity:    numberToInt(now.Now.Humidity),
			Wind:        strings.TrimSpace(now.Now.WindDir + " " + now.Now.WindSpeed + "km/h"),
			Visibility:  formatVisibility(float64(numberToInt(now.Now.Vis))),
		},
	}

	var daily qWeatherDaily
	if err := fetchJSON(ctx, base+"/v7/weather/3d?location="+url.QueryEscape(loc.ID), headers, &daily); err == nil && daily.Code == "200" {
		for index, day := range daily.Daily {
			if index >= forecastDays {
				break
			}
			data.Forecast = append(data.Forecast, ForecastDay{
				Date:       day.FxDate,
				High:       numberToInt(day.TempMax),
				Low:        numberToInt(day.TempMin),
				Condition:  firstOrDefault(day.TextDay, "未知"),
				PrecipProb: precipOrNil(float64(numberToInt(day.Pop)), day.Pop != ""),
			})
		}
	}

	if loc.Lat != 0 && loc.Lon != 0 {
		var air qWeatherAir
		airURL := fmt.Sprintf("%s/airquality/v1/current/%s/%s?lang=zh",
			base, url.PathEscape(fmt.Sprintf("%g", loc.Lat)), url.PathEscape(fmt.Sprintf("%g", loc.Lon)))
		if err := fetchJSON(ctx, airURL, headers, &air); err == nil && len(air.Indexes) > 0 {
			index := air.Indexes[0]
			data.AQI = &AirQuality{
				Value:    roundFloat(index.AQI),
				Level:    index.Level,
				Category: index.Cat,
				Primary:  index.Prim.Name,
			}
		}
	}
	return data, nil
}

// joinCity 城市展示名：城市 + 省份，非中国时补国家
func joinCity(name, adm1, country, fallback string) string {
	parts := make([]string, 0, 3)
	for _, part := range []string{firstOrDefault(name, fallback), adm1, country} {
		if strings.TrimSpace(part) == "" || part == "中国" {
			continue
		}
		parts = append(parts, strings.TrimSpace(part))
	}
	return strings.Join(parts, ", ")
}

// firstOrDefault 取第一个非空值
func firstOrDefault(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}
