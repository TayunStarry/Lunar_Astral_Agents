package FaceLTP

import (
	"LunarSubsystem/GeneralConfig"
	"strings"
)

// loadModelConfig 从配置模块读取多模态模型配置（lunar_config.json 的 agent 字段），
// 存入全局 modelConfig 缓存。模型名不允许硬编码，读取失败回退占位值，仍走同源 /v1 代理。
func loadModelConfig() {
	modelConfig.Model = stringOr(*GeneralConfig.AgentMultimodalModel, "system-multimodal")
	modelConfig.URL = normalizeV1URL(stringOr(*GeneralConfig.AgentMultimodalURL, "http://127.0.0.1:36789/v1"))
	modelConfig.Key = stringOr(*GeneralConfig.AgentMultimodalKey, "")
}

// stringOr 空串回退默认值
func stringOr(v, def string) string {
	if v == "" {
		return def
	}
	return v
}

// normalizeV1URL 规范化 API 基础地址，确保其以 /v1 结尾
func normalizeV1URL(raw string) string {
	u := strings.TrimRight(strings.TrimSpace(raw), "/")
	if strings.HasSuffix(u, "/v1") {
		return u
	}
	return u + "/v1"
}