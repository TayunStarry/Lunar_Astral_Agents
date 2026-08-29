package BrowserClient

import (
	"LunarSubsystem/LoggerGeneral"
	"encoding/json"
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
	LoggerGeneral.SubInfo("BrowserClient", "OpenSystemBrowser", "使用系统浏览器打开: %s", url)
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
		LoggerGeneral.SubError("BrowserClient", "OpenSystemBrowser", "%v 建议手动访问: %s", err, url)
	}
}

// OpenBrowser 使用浏览器打开指定 URL
func OpenBrowser(url string) {
	LoggerGeneral.SubInfo("BrowserClient", "OpenBrowser", "开始选择浏览器")
	if !IsWebViewSupported() {
		LoggerGeneral.SubInfo("BrowserClient", "OpenBrowser", "webview 不支持，回退到系统浏览器")
		OpenSystemBrowser(url)
		return
	}
	LoggerGeneral.SubInfo("BrowserClient", "OpenBrowser", "启动 webview 专用线程")
	go StartWebViewBrowser(url, "")
}

// OpenBrowserWithReturnButton 与 OpenBrowser 相同，但额外向每个非主页面的顶层页面注入
// 「返回主页面」悬浮按钮，用于外部页面跳转后回到主页面（如琉璃应用被 _self 跳转走的情况）。
// mainURL 为点击悬浮按钮时导航回的目标地址（主页面 URL）。
func OpenBrowserWithReturnButton(url string, mainURL string) {
	LoggerGeneral.SubInfo("BrowserClient", "OpenBrowserWithReturnButton", "开始选择浏览器")
	if !IsWebViewSupported() {
		LoggerGeneral.SubInfo("BrowserClient", "OpenBrowserWithReturnButton", "webview 不支持，回退到系统浏览器")
		OpenSystemBrowser(url)
		return
	}
	LoggerGeneral.SubInfo("BrowserClient", "OpenBrowserWithReturnButton", "启动 webview 专用线程")
	go StartWebViewBrowser(url, buildReturnButtonJS(mainURL))
}

// buildReturnButtonJS 生成「返回主页面」悬浮按钮注入脚本。
// 通过 webview.Init 注入，在 WebView2 每个新文档创建时执行：
//   - 仅顶层页面显示（iframe 内不注入，避免干扰 LTPX 覆盖层中的包页面）
//   - 主页面自身不显示（按根路径 pathname == '/' 判断）
//   - 点击后导航回 mainURL
func buildReturnButtonJS(mainURL string) string {
	homeJSON, _ := json.Marshal(mainURL)
	return `(function () {
    if (window.top !== window.self) return; // iframe 内不注入
    var p = window.location.pathname;
    if (p === '/' || p === '') return;      // 主页面自身不注入
    if (document.getElementById('crystal-return-home')) return; // 避免重复注入

    var home = ` + string(homeJSON) + `;
    var btn = document.createElement('div');
    btn.id = 'crystal-return-home';
    btn.title = '返回琉璃主页面';
    btn.setAttribute('role', 'button');
    btn.innerHTML = '<span style="font-size:14px;line-height:1">&#9670;</span><span>返回琉璃</span>';
    btn.style.cssText = [
        'position:fixed', 'right:18px', 'bottom:18px', 'z-index:2147483647',
        'display:flex', 'align-items:center', 'gap:7px',
        'padding:10px 18px', 'border-radius:999px', 'cursor:pointer',
        'font:600 13px/1 "Microsoft YaHei","PingFang SC",sans-serif', 'color:#fff',
        'background:rgba(120,140,255,0.92)',
        'box-shadow:0 4px 18px rgba(0,0,0,0.28),inset 0 0 0 1px rgba(255,255,255,0.25)',
        'backdrop-filter:blur(8px)', '-webkit-backdrop-filter:blur(8px)',
        'user-select:none', '-webkit-user-select:none',
        'opacity:0.92', 'transition:transform .15s ease,box-shadow .15s ease,opacity .2s ease'
    ].join(';') + ';';
    btn.onmouseenter = function () { btn.style.transform = 'scale(1.05)'; btn.style.opacity = '1'; };
    btn.onmouseleave = function () { btn.style.transform = 'scale(1)'; btn.style.opacity = '0.92'; };
    btn.onclick = function () {
        btn.style.pointerEvents = 'none';
        btn.style.opacity = '0.5';
        window.location.href = home;
    };
    function mount() {
        if (document.body) { document.body.appendChild(btn); }
        else { setTimeout(mount, 50); }
    }
    mount();
})();`
}
