package lunar

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"LunarSubsystem/LunarHarness/engine/config"
	"LunarSubsystem/LunarHarness/engine/console/logger"
)

// defaultAddress 查询失败时的兜底地址（省份、城市）
var defaultAddress = []string{"江苏省", "南京市"}

// addressMu 保护地址缓存
var addressMu sync.Mutex

// addressAttemptAt 最近一次外网查询的时刻（失败也记录，避免每个角色构造都重试）
var addressAttemptAt time.Time

// addressRetryInterval 查询失败后的重试间隔
const addressRetryInterval = 10 * time.Minute

// resolveAddress 取当前位置（省份、城市）。
//
// 永远返回非空结果：查询失败时用兜底地址而不是空串。空串会让系统提示词里的
// 「当前位置：」整行为空，模型就只能反问或编造——这正是「月华不知道自己在哪个城市」的成因。
func resolveAddress() []string {
	addressMu.Lock()
	defer addressMu.Unlock()
	if len(config.ServerAddress) >= 2 {
		return config.ServerAddress
	}
	// 失败结果保留重试能力：间隔内直接复用兜底，之后重新查询
	if !addressAttemptAt.IsZero() && time.Since(addressAttemptAt) < addressRetryInterval {
		return defaultAddress
	}
	addressAttemptAt = time.Now()
	if location, err := queryAddress(); err == nil && len(location) >= 2 {
		config.ServerAddress = location
		return location
	}
	logger.Warn(lunarCoreModule, "位置查询不可用，暂用兜底地址：%s", strings.Join(defaultAddress, " "))
	return defaultAddress
}

// queryAddress 查询公网出口位置（省份、城市）
func queryAddress() ([]string, error) {
	type endpoint struct {
		url   string
		parse func([]byte) ([]string, error)
	}
	endpoints := []endpoint{
		{"http://ip-api.com/json/?lang=zh-CN&fields=status,message,regionName,city", parseIPApi},
		{"https://ipapi.co/json/", parseIPApiCo},
	}
	client := &http.Client{Timeout: 6 * time.Second}
	var lastErr error
	for _, item := range endpoints {
		response, err := client.Get(item.url)
		if err != nil {
			lastErr = err
			continue
		}
		body, readErr := readAllAndClose(response)
		if readErr != nil {
			lastErr = readErr
			continue
		}
		if response.StatusCode != http.StatusOK {
			lastErr = fmt.Errorf("HTTP状态异常: %s", response.Status)
			continue
		}
		location, parseErr := item.parse(body)
		if parseErr != nil {
			lastErr = parseErr
			continue
		}
		return location, nil
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("没有可用的位置查询端点")
	}
	return nil, lastErr
}

// parseIPApi 解析 ip-api.com 响应
func parseIPApi(body []byte) ([]string, error) {
	var data struct {
		Status     string `json:"status"`
		Message    string `json:"message"`
		RegionName string `json:"regionName"`
		City       string `json:"city"`
	}
	if err := json.Unmarshal(body, &data); err != nil {
		return nil, err
	}
	if data.Status != "success" {
		return nil, fmt.Errorf("IP位置接口返回失败: %s", data.Message)
	}
	return splitRegionCity(data.RegionName, data.City)
}

// parseIPApiCo 解析 ipapi.co 响应
func parseIPApiCo(body []byte) ([]string, error) {
	var data struct {
		Region string `json:"region"`
		City   string `json:"city"`
		Error  bool   `json:"error"`
	}
	if err := json.Unmarshal(body, &data); err != nil {
		return nil, err
	}
	if data.Error {
		return nil, fmt.Errorf("IP位置接口返回失败")
	}
	return splitRegionCity(data.Region, data.City)
}

// splitRegionCity 规整省份与城市（两者至少有一个非空）
func splitRegionCity(region, city string) ([]string, error) {
	region = strings.TrimSpace(region)
	city = strings.TrimSpace(city)
	if region == "" || city == "" {
		return nil, fmt.Errorf("省份或城市信息缺失")
	}
	return []string{region, city}, nil
}

// readAllAndClose 读取并关闭响应体
func readAllAndClose(response *http.Response) ([]byte, error) {
	defer response.Body.Close()
	return io.ReadAll(response.Body)
}
