package weather

// provider_openmeteo.go Open-Meteo 数据源：地理编码 + 当前天气 + 3 天预报（免 Key）。
// 当前天气里的能见度单位是米，输出前折算为公里。

import (
	"context"
	"fmt"
	"net/url"
)

// queryOpenMeteo 用 Open-Meteo 查询指定城市。
func queryOpenMeteo(ctx context.Context, city string) (Data, error) {
	var geo openMeteoGeo
	geoURL := "https://geocoding-api.open-meteo.com/v1/search?name=" + url.QueryEscape(city) + "&count=1&language=zh&format=json"
	if err := fetchJSON(ctx, geoURL, nil, &geo); err != nil {
		return Data{}, fmt.Errorf("城市定位请求失败: %w", err)
	}
	if len(geo.Results) == 0 {
		return Data{}, fmt.Errorf("未找到城市：%s", city)
	}
	loc := geo.Results[0]

	var forecast openMeteoForecast
	forecastURL := fmt.Sprintf("https://api.open-meteo.com/v1/forecast?latitude=%s&longitude=%s"+
		"&current=temperature_2m,apparent_temperature,weather_code,relative_humidity_2m,wind_speed_10m,wind_direction_10m,visibility"+
		"&daily=temperature_2m_max,temperature_2m_min,weather_code,precipitation_probability_max"+
		"&timezone=auto&forecast_days=%d",
		url.QueryEscape(fmt.Sprintf("%g", loc.Latitude)), url.QueryEscape(fmt.Sprintf("%g", loc.Longitude)), forecastDays)
	if err := fetchJSON(ctx, forecastURL, nil, &forecast); err != nil {
		return Data{}, fmt.Errorf("天气请求失败: %w", err)
	}

	cur := forecast.Current
	condition := wmoCodes[cur.WeatherCode]
	if condition == "" {
		condition = fmt.Sprintf("天气代码 %d", cur.WeatherCode)
	}
	dirIndex := roundFloat(cur.WindDirection/45) % 8
	if dirIndex < 0 {
		dirIndex += 8
	}
	data := Data{
		City:   joinCity(loc.Name, loc.Admin1, loc.Country, city),
		Source: "Open-Meteo",
		Current: Current{
			Temperature: roundFloat(cur.Temperature),
			FeelsLike:   roundFloat(cur.ApparentTemp),
			Condition:   condition,
			Humidity:    roundFloat(cur.Humidity),
			Wind:        fmt.Sprintf("%s %dkm/h", windDirections[dirIndex], roundFloat(cur.WindSpeed)),
			Visibility:  formatVisibility(cur.Visibility / 1000),
		},
	}

	for index, date := range forecast.Daily.Time {
		if index >= forecastDays {
			break
		}
		day := ForecastDay{Date: date}
		if index < len(forecast.Daily.TempMax) {
			day.High = roundFloat(forecast.Daily.TempMax[index])
		}
		if index < len(forecast.Daily.TempMin) {
			day.Low = roundFloat(forecast.Daily.TempMin[index])
		}
		if index < len(forecast.Daily.WeatherCode) {
			day.Condition = firstOrDefault(wmoCodes[forecast.Daily.WeatherCode[index]], "未知")
		}
		if index < len(forecast.Daily.PrecipProb) {
			day.PrecipProb = precipOrNil(forecast.Daily.PrecipProb[index], true)
		}
		data.Forecast = append(data.Forecast, day)
	}
	return data, nil
}
