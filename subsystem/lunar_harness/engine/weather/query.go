package weather

// query.go 天气查询调度与共享请求工具。
//
// 策略与前端包 lunar.search-weather 一致：和风天气（有 Key）→ Open-Meteo → wttr.in。
// 城市为空时前两家无法定位，直接交给 wttr.in 按 IP 返回所在地天气。

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
)

// Query 按 和风 → Open-Meteo → wttr.in 的顺序查询天气。
func Query(ctx context.Context, city string) (Data, error) {
	city = strings.TrimSpace(city)
	var failures []string

	if city != "" && QWeatherHost != "" && QWeatherKey != "" {
		if data, err := queryQWeather(ctx, city); err == nil {
			data.Provider, data.Strategy = "qweather", "auto"
			return data, nil
		} else {
			failures = append(failures, label("qweather")+"："+err.Error())
		}
	}
	if city != "" {
		if data, err := queryOpenMeteo(ctx, city); err == nil {
			data.Provider, data.Strategy = "openmeteo", "auto"
			return data, nil
		} else {
			failures = append(failures, label("openmeteo")+"："+err.Error())
		}
	}
	if data, err := queryWttr(ctx, city); err == nil {
		data.Provider, data.Strategy = "wttr", "auto"
		return data, nil
	} else {
		failures = append(failures, label("wttr")+"："+err.Error())
	}
	return Data{}, fmt.Errorf("所有天气服务暂不可用：%s", strings.Join(failures, "；"))
}

// label 数据源中文名（未登记时回退为内部标识）
func label(provider string) string {
	if name, ok := providerLabels[provider]; ok {
		return name
	}
	return provider
}

// fetchJSON 带超时与 UA 的 JSON 请求；先解析 JSON 再看 HTTP 状态，便于区分「非 JSON 返回」。
func fetchJSON(ctx context.Context, url string, headers map[string]string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", userAgent)
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return err
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("返回内容不是 JSON（HTTP %d）", resp.StatusCode)
	}
	if resp.StatusCode >= 400 {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return nil
}

// numberToInt 把数据源里的字符串或浮点数字段安全取整（空值 / 非法值回退 0）
func numberToInt(value any) int {
	switch v := value.(type) {
	case string:
		n, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
		if err != nil {
			return 0
		}
		return roundFloat(n)
	case float64:
		return roundFloat(v)
	case int:
		return v
	}
	return 0
}

// roundFloat 四舍五入到整数（与前端 Math.round 语义一致）
func roundFloat(value float64) int {
	if value >= 0 {
		return int(value + 0.5)
	}
	return int(value - 0.5)
}

// formatVisibility 能见度取一位小数
func formatVisibility(value float64) float64 {
	return float64(roundFloat(value*10)) / 10
}

// precipOrNil 降水概率：无数据时返回 nil（JSON 省略），有数据取整
func precipOrNil(value float64, present bool) *int {
	if !present {
		return nil
	}
	n := roundFloat(value)
	return &n
}
