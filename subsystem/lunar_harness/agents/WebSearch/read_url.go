package WebSearch

// read_url.go 指定页面读取（用户直接发来、还没有缓存摘要的网页地址）
//
// 与网络搜索共用同一套模型生命周期：先卸载主多模态模型，改用 light_reasoning
// 打开页面并提炼摘要，完成后卸载 light_reasoning（主模型由 router 在月华下一次
// 请求时按需重载）。
//
// 提炼结果写入「近期已访问」缓存（web.db 的 web_ltp_page_cache），
// 资源目录据此给 @U 句柄回填「地址 + 标题 + 摘要」，月华下次看到同一地址不再开页面。

import (
	"fmt"
	"strings"
	"time"

	"LunarSubsystem/LunarHarness/engine/browser"
	"LunarSubsystem/LunarHarness/engine/console/logger"
)

// ReadURL 打开指定页面并提炼摘要：返回页面标题与摘要
//
// instruction 是月华对这次引用的具体诉求（可空，交给默认提炼指令）。
func ReadURL(target, instruction string) (title, summary string, err error) {
	target = strings.TrimSpace(target)
	if target == "" {
		return "", "", fmt.Errorf("页面地址为空")
	}
	instruction = strings.TrimSpace(instruction)
	if instruction == "" {
		instruction = readURLInstruction
	}

	runMu.Lock()
	defer runMu.Unlock()

	cfg := LoadConfig()
	deadline := time.Now().Add(time.Duration(cfg.MaxRunSeconds) * time.Second)
	logger.Info("CrystalAstral", "Web-Search 打开指定页面: %s", truncateRunes(target, 80))

	// 模型生命周期与网络搜索一致：卸主模型 → light_reasoning 提炼 → 收尾卸轻量模型
	profile, unloadSearchModel := prepareSearchModel(cfg)
	defer unloadSearchModel()

	sess, err := browser.OpenWebViewSession("", "网页搜索:指定页面", 1280, 860)
	if err != nil {
		return "", "", fmt.Errorf("打开浏览器会话失败: %w", err)
	}
	defer sess.Close()

	if err := navTo(sess, target, 20*time.Second, 1200*time.Millisecond); err != nil {
		return "", "", fmt.Errorf("打开页面失败: %w", err)
	}
	walk := walkPage(sess, cfg.perPageChars(1), cfg, deadline, nil)
	if strings.TrimSpace(walk.Text) == "" {
		return "", "", fmt.Errorf("页面未提取到有效正文")
	}

	title = firstNonEmpty(walk.Title, domainOf(target), target)
	summary, err = composeDigest(instruction, title, []digestSource{{
		Label: "指定页面",
		Title: title,
		URL:   firstNonEmpty(walk.URL, target),
		Text:  walk.Text,
	}}, profile)
	summary = strings.TrimSpace(summary)
	if err != nil || summary == "" {
		if err == nil {
			err = fmt.Errorf("页面摘要为空")
		}
		return title, "", err
	}

	// 记入近期访问缓存：同一地址下次直接命中，不再重复开页面
	if cfg.cacheEnabled() {
		if cache, cacheErr := openPageCache(time.Duration(cfg.CacheTTLDays) * 24 * time.Hour); cacheErr == nil {
			cache.put(normalizeURL(target), title, domainOf(target), summary)
			cache.close()
		} else {
			loggerWarn("近期访问缓存不可用，本次结果不回填: %v", cacheErr)
		}
	}
	logger.Info("CrystalAstral", "Web-Search 指定页面提炼完成: %s", truncateRunes(target, 80))
	return title, summary, nil
}
