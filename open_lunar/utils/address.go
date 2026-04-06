package utils

import (
	"encoding/json"
	"fmt"
	"net/http"
	"open-lunar/parameter"
)

// IPInfo 存储IP地址信息
type IPInfo struct {
	Region string `json:"region"`
	City   string `json:"city"`
}

// QueryCurrentAddress 查询当前地址信息
func QueryCurrentAddress() []string {
	// 如果当前地址已缓存，直接返回
	if len(parameter.ServerAddress) > 0 {
		return parameter.ServerAddress
	}

	// 从IP地址查询位置信息
	resp, err := http.Get("https://ipapi.co/json/")
	if err != nil {
		fmt.Printf("获取位置失败: %v\n", err)
		return []string{"江苏省", "南京市"}
	}
	defer resp.Body.Close()

	// 检查响应状态
	if resp.StatusCode != http.StatusOK {
		fmt.Printf("获取位置失败: %s\n", resp.Status)
		return []string{"江苏省", "南京市"}
	}

	// 解析JSON响应
	var data IPInfo
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		fmt.Printf("解析位置信息失败: %v\n", err)
		return []string{"江苏省", "南京市"}
	}

	// 确保省份和城市信息存在
	if data.Region == "" || data.City == "" {
		fmt.Println("获取位置失败: 省份或城市信息缺失")
		return []string{"江苏省", "南京市"}
	}

	// 缓存当前地址
	parameter.ServerAddress = []string{data.Region, data.City}
	return parameter.ServerAddress
}
