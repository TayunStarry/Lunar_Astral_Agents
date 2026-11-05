package proxy

// proxy_network.go 本机地址探测

import (
	"net"
)

func LocalIPs() []string {
	candidates := listLocalIPs()

	// 按可用性打分排序：私网 → 公网 → 链路本地 → 回环
	for i := 0; i < len(candidates); i++ {
		for j := i + 1; j < len(candidates); j++ {
			if ipScore(candidates[j].IP) < ipScore(candidates[i].IP) {
				candidates[i], candidates[j] = candidates[j], candidates[i]
			}
		}
	}

	ips := make([]string, 0, len(candidates))
	for _, c := range candidates {
		ips = append(ips, c.IP)
	}
	return ips
}

// listLocalIPs 枚举本机所有可用 IPv4 地址（附带网卡名，仅列出已启用的网卡）
func listLocalIPs() []ipCandidate {
	var candidates []ipCandidate
	seen := make(map[string]bool)

	ifaces, err := net.Interfaces()
	if err != nil {
		return candidates
	}

	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			if ipNet, ok := addr.(*net.IPNet); ok {
				if ip := ipNet.IP.To4(); ip != nil && !seen[ip.String()] {
					seen[ip.String()] = true
					candidates = append(candidates, ipCandidate{
						IP:        ip.String(),
						Interface: iface.Name,
					})
				}
			}
		}
	}
	return candidates
}

// ipScore 为 IP 地址打分用于自动选择：私网地址优先，链路本地 (169.254.*) 与回环最后
// 返回值越小越优先
func ipScore(ipStr string) int {
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return 99
	}
	switch {
	case ip.IsLoopback():
		return 90
	case ip.IsLinkLocalUnicast(): // 169.254.0.0/16，通常是网卡未获得 DHCP 分配时的自动地址
		return 80
	case ip.IsPrivate(): // 192.168.0.0/16、10.0.0.0/8、172.16.0.0/12
		return 0
	default:
		return 50
	}
}
