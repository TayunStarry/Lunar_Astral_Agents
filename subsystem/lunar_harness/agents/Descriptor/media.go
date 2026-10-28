package Descriptor

// media.go 影像描述入口：图片 / 动图统一走这里
//
// 查询语义（与句柄系统约定一致）：
//   已有描述 → 直接返回，零模型调用；
//   没有描述 → 图片走视觉副本描述，动图走统一观看管线（整段动画视为一个切片），
//              返回后写回资源目录；
//   句柄失效 / 本体丢失 → 返回错误，由句柄层走「找不到」流程。
// 定向问题只回本次调用，不覆盖共用描述。
//
// 视频不再使用 @V 句柄：网络视频统一登记为 @U 链接资源。

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"LunarSubsystem/LunarHarness/engine/config"
	"LunarSubsystem/LunarHarness/engine/console/logger"
	"LunarSubsystem/LunarHarness/engine/multimodal"
	persistence "LunarSubsystem/LunarHarness/engine/persistence"
	"LunarSubsystem/LunarHarness/engine/resource"
)

// DescribeMedia 取回图片 / 动图 / 视频的内容描述
//
// @param handle   资源句柄（@I.007）
// @param question 可选定向问题（非空时不读缓存、不写缓存）
// @return 描述文本；句柄失效或本体丢失时返回错误
func DescribeMedia(handle, question string) (string, error) {
	runMu.Lock()
	defer runMu.Unlock()

	question = strings.TrimSpace(question)
	res, err := resource.Resolve(handle)
	if err != nil {
		return "", err
	}
	if res.State != resource.StateActive {
		return "", fmt.Errorf("%s 的本体已丢失", handle)
	}
	if res.Kind != resource.KindImage {
		return "", fmt.Errorf("%s 不是图片或动图", handle)
	}
	// 已有描述且无定向问题：直接命中
	if cached := strings.TrimSpace(res.Summary); cached != "" && question == "" {
		return cached, nil
	}

	abs := filepath.Join(filepath.Clean(config.LocalDir), filepath.FromSlash(res.BlobPath))

	data, err := os.ReadFile(abs)
	if err != nil {
		return "", fmt.Errorf("读取图片失败: %w", err)
	}
	// 动图与视频同链路：整段动画就是一个切片
	if multimodal.IsAnimatedImage(data) {
		slice, err := multimodal.AnimatedToWatchSlice(data, multimodal.WatchFrameMax)
		if err != nil {
			return "", err
		}
		return watchSlices(res, []multimodal.WatchSlice{slice}, "动图", question)
	}
	return describeStaticImage(res, abs, question)
}

// ErrVideoExpired 远程视频地址失效，且本机没有可复用的副本。
var ErrVideoExpired = errors.New("视频地址已失效")

// DescribeVideoURL 解读本机视频地址（@U 句柄背后的 /file/read/ 地址）。
//
// 视频不再单独建 @V 句柄：QQ 等渠道把视频落到 multimedia/ 后登记成 @U，
// 这里按句柄里的地址解析本地文件，复用同一条观看管线。
func DescribeVideoURL(rawURL, question string) (string, error) {
	runMu.Lock()
	defer runMu.Unlock()

	res, found := resource.FindByURL(rawURL)
	var fallbackNames []string
	if found {
		fallbackNames = append(fallbackNames, res.Name, res.Title)
	}
	abs, err := ensureLocalVideoPath(rawURL, fallbackNames...)
	if err != nil {
		return "", err
	}
	slices, err := multimodal.VideoToWatchSlices(abs, multimodal.WatchMaxSlices, multimodal.WatchFrameMax)
	if err != nil {
		return "", err
	}
	if !found {
		res = resource.Resource{Kind: resource.KindURL, URL: rawURL, Name: filepath.Base(abs)}
	}
	return watchSlices(res, slices, "视频", strings.TrimSpace(question))
}

// ensureLocalVideoPath 把视频地址解析为本地文件；远程地址按内容哈希下载落盘一次。
//
// 兼容旧 @U 句柄：早期渠道视频只登记了远程 QQ CDN 地址，没有本地副本；
// 第一次解读时按需下载并写入 multimedia/，后续同一内容复用同一路径。
func ensureLocalVideoPath(rawURL string, fallbackNames ...string) (string, error) {
	if abs, ok := resource.LocalPathURL(rawURL); ok {
		if info, err := os.Stat(abs); err == nil && !info.IsDir() {
			return abs, nil
		}
	}
	// 旧句柄优先复用本地同名副本：QQ CDN 地址过期后，本地往往还留着当时的视频。
	for _, name := range fallbackNames {
		if abs, ok := localVideoByName(name); ok {
			return abs, nil
		}
	}
	if abs, ok := localVideoByName(filepath.Base(pathPartOfURL(rawURL))); ok {
		return abs, nil
	}
	lower := strings.ToLower(strings.TrimSpace(rawURL))
	if !strings.HasPrefix(lower, "http://") && !strings.HasPrefix(lower, "https://") {
		return "", fmt.Errorf("不是可读取的视频地址: %s", rawURL)
	}
	client := &http.Client{Timeout: 5 * time.Minute}
	resp, err := client.Get(rawURL)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrVideoExpired, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("%w: 下载状态码 %d", ErrVideoExpired, resp.StatusCode)
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil || len(data) == 0 {
		return "", fmt.Errorf("%w: 读取视频字节失败 %v", ErrVideoExpired, err)
	}
	ext := strings.ToLower(filepath.Ext(pathPartOfURL(rawURL)))
	if resource.KindOfExt(ext) != resource.KindVideo {
		ext = ".mp4"
	}
	sum := sha256.Sum256(data)
	rel := fmt.Sprintf("multimedia/video_%s%s", hex.EncodeToString(sum[:8]), ext)
	if _, err := persistence.SaveFileEx(rel, persistence.SaveOptions{Overwrite: true}, bytes.NewReader(data)); err != nil {
		return "", fmt.Errorf("视频落盘失败: %w", err)
	}
	return filepath.Join(filepath.Clean(config.LocalDir), filepath.FromSlash(rel)), nil
}

func pathPartOfURL(raw string) string {
	path := strings.TrimSpace(raw)
	if cut := strings.IndexAny(path, "?#"); cut >= 0 {
		path = path[:cut]
	}
	return path
}

// localVideoByName 在 multimedia/ 下按文件名查找可复用的本地视频副本。
func localVideoByName(name string) (string, bool) {
	base := filepath.Base(strings.TrimSpace(name))
	if decoded, err := url.PathUnescape(base); err == nil {
		base = decoded
	}
	if base == "" || base == "." || base == "/" || base == "\\" {
		return "", false
	}
	abs := filepath.Join(filepath.Clean(config.LocalDir), "multimedia", base)
	info, err := os.Stat(abs)
	if err != nil || info.IsDir() {
		return "", false
	}
	if resource.KindOfExt(filepath.Ext(abs)) != resource.KindVideo {
		return "", false
	}
	return abs, true
}

// storeResourceSummary 把通用描述写回资源目录（定向问题的答复不入库）
func storeResourceSummary(res resource.Resource, text string) {
	text = strings.TrimSpace(text)
	if text == "" {
		return
	}
	if err := resource.SetSummary(res.ID, hardCutRunes(text, summaryStoreChars), nil); err != nil {
		logger.Warn("CrystalAstral", "描述者：写入 %s 描述失败: %v", res.Handle, err)
	}
}
