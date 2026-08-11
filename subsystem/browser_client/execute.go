package browser

import (
	"LunarSubsystem/general_logger"
	"fmt"
	"net"
	"os/exec"
	"runtime"
	"sort"
	"strings"
)

// shouldSkipInterface 判断是否应该跳过该网络接口
func shouldSkipInterface(iface net.Interface) bool {
	// 转换接口名称为小写，统一比较
	name := strings.ToLower(iface.Name)
	// 跳过虚拟接口
	virtualKeywords := []string{
		"vEthernet", "hyper-v", "default switch",
		"docker", "br-", "veth",
		"vmnet", "virtualbox", "vboxnet",
		"tap-", "tun-", "ppp",
		"npcap", "npcapi", "ndis",
	}
	for _, keyword := range virtualKeywords {
		if strings.Contains(name, strings.ToLower(keyword)) {
			return true
		}
	}
	// 跳过未启用的接口
	if iface.Flags&net.FlagUp == 0 {
		return true
	}
	// 跳过环回接口
	if iface.Flags&net.FlagLoopback != 0 {
		return true
	}
	return false
}

// calculatePriority 计算IP地址的优先级
func calculatePriority(ip string, preferredNetworks []string) int {
	// 最高优先级：用户指定的优先网段
	for i, network := range preferredNetworks {
		if strings.HasPrefix(ip, network) {
			return i // 返回索引值，索引越小优先级越高
		}
	}

	// 标准局域网网段优先级
	if strings.HasPrefix(ip, "192.168.") {
		return 100 // 192.168.x.x
	} else if strings.HasPrefix(ip, "10.") {
		return 200 // 10.x.x.x
	} else if len(ip) >= 7 && strings.HasPrefix(ip, "172.") {
		// 检查是否为172.16.x.x - 172.31.x.x
		parts := strings.Split(ip, ".")
		if len(parts) >= 2 {
			second := parts[1]
			if second >= "16" && second <= "31" {
				return 300 // 172.16.x.x - 172.31.x.x
			}
		}
	}

	// 其他私有地址
	if strings.HasPrefix(ip, "169.254.") {
		return 400 // 链路本地地址
	}

	// 其他公网地址
	return 500
}

// GetLocalIP 获取本地 IP 地址
func GetLocalIP(preferredNetworks []string) (string, error) {
	interfaces, err := net.Interfaces()
	if err != nil {
		return "", fmt.Errorf("获取网络接口失败: %v", err)
	}

	var candidates []ipCandidate

	for _, iface := range interfaces {
		// 跳过虚拟接口和停用的接口
		if shouldSkipInterface(iface) {
			continue
		}

		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}

		for _, addr := range addrs {
			ipNet, ok := addr.(*net.IPNet)
			if !ok || ipNet.IP.IsLoopback() || ipNet.IP.To4() == nil {
				continue
			}

			ip := ipNet.IP.String()
			priority := calculatePriority(ip, preferredNetworks)

			// 如果找到最高优先级的IP（192.168.1.x），立即返回
			if priority == 0 {
				return ip, nil
			}

			candidates = append(candidates, ipCandidate{
				IP:        ip,
				Priority:  priority,
				Interface: iface.Name,
			})
		}
	}

	// 按优先级排序
	if len(candidates) > 0 {
		sort.Slice(candidates, func(i, j int) bool {
			return candidates[i].Priority < candidates[j].Priority
		})
		return candidates[0].IP, nil
	}

	return "", fmt.Errorf("未找到可用的IP地址")
}

// OpenSystemBrowser 在系统默认浏览器中打开指定 URL
func OpenSystemBrowser(url string) {
	logger.SubInfo("Browser", "OpenSystemBrowser", "使用系统浏览器打开: %s", url)
	var cmd string
	var args []string

	switch runtime.GOOS {
	case "windows":
		cmd = "cmd"
		args = []string{"/c", "start", url}
	case "darwin":
		cmd = "open"
		args = []string{url}
	default:
		cmd = "xdg-open"
		args = []string{url}
	}

	if err := exec.Command(cmd, args...).Start(); err != nil {
		logger.SubError("Browser", "OpenSystemBrowser", "%v 建议手动访问: %s", err, url)
	}
}

// OpenBrowser 使用浏览器打开指定 URL
func OpenBrowser(url string) {
	logger.SubInfo("Browser", "OpenBrowser", "开始选择浏览器")
	if !IsWebViewSupported() {
		logger.SubInfo("Browser", "OpenBrowser", "webview 不支持，回退到系统浏览器")
		OpenSystemBrowser(url)
		return
	}
	logger.SubInfo("Browser", "OpenBrowser", "启动 webview 专用线程")
	go StartWebViewBrowser(url)
}
