package LunarGoja

import (
	"LunarSubsystem/LoggerGeneral"
	"net"

	"github.com/dop251/goja"
)

// ==== 网络接口枚举 ====

// getNetworkInterfaces 枚举本机所有网络接口及其地址
// 返回值: [Array<{name, addresses:[{address, family, internal}]}>, error]
func (env *standardEnv) getNetworkInterfaces(call goja.FunctionCall) goja.Value {
	ifaces, err := net.Interfaces()
	if err != nil {
		LoggerGeneral.Error("LunarGoja", "枚举网络接口失败: %v", err)
		return env.runtime.ToValue([]any{[]any{}, err})
	}

	result := make([]any, 0, len(ifaces))
	for _, iface := range ifaces {
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		addressList := make([]any, 0, len(addrs))
		for _, addr := range addrs {
			ip, _, err := net.ParseCIDR(addr.String())
			if err != nil {
				continue
			}
			family := "IPv4"
			if ip.To4() == nil {
				family = "IPv6"
			}
			addressList = append(addressList, map[string]any{
				"address":  ip.String(),
				"family":   family,
				"internal": ip.IsLoopback(),
			})
		}
		result = append(result, map[string]any{
			"name":      iface.Name,
			"addresses": addressList,
		})
	}
	return env.runtime.ToValue([]any{result, nil})
}
