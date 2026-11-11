package Vision

// media.go 视觉理解入口：图片 / 动图 / 视频统一走这里
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
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"LunarSubsystem/LunarHarness/engine/config"
	"LunarSubsystem/LunarHarness/engine/console/logger"
	"LunarSubsystem/LunarHarness/engine/multimodal"
	"LunarSubsystem/LunarHarness/engine/netsource"
	persistence "LunarSubsystem/LunarHarness/engine/persistence"
	"LunarSubsystem/LunarHarness/engine/resource"
	"LunarSubsystem/LunarHarness/engine/resourcehub"
	"LunarSubsystem/LunarHarness/protocol/agentbase"
)

// ResolveHandle 把句柄或路径统一解析为资源句柄（路径支持完整 / 相对 / 纯文件名）
func ResolveHandle(input string) (string, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return "", fmt.Errorf("文件或句柄不能为空")
	}
	match, err := resourcehub.ResolveToHandle(input, []string{resource.StorageRootDocuments, resource.StorageRootMultimedia})
	if err != nil {
		return "", err
	}
	return match.Handle, nil
}

// runMu 串行化视觉理解运行（共享模型通道与实况流）
var runMu sync.Mutex

// DescribeMedia 取回图片 / 动图的内容描述
//
// @param input    资源句柄（@I.007），或完整 / 相对 / 纯文件名路径
// @param question 可选定向问题（非空时不读缓存、不写缓存）
// @return 描述文本；句柄失效或本体丢失时返回错误
func DescribeMedia(input, question string) (string, error) {
	handle, err := ResolveHandle(input)
	if err != nil {
		return "", err
	}

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
	if res.Kind != resource.KindImage && res.Kind != resource.KindAnimated {
		return "", fmt.Errorf("%s 不是图片或动图", handle)
	}
	// 已有未过期的解析文本且无定向问题：直接命中（本地图片按本地档，不设 TTL）
	if cached := strings.TrimSpace(res.ParseText); cached != "" && question == "" &&
		resource.ParseTextFresh(res, resource.ParseCategoryOf(res)) {
		return cached, nil
	}

	abs := filepath.Join(filepath.Clean(config.LocalDir), filepath.FromSlash(res.BlobPath))

	data, err := os.ReadFile(abs)
	if err != nil {
		return "", fmt.Errorf("读取图片失败: %w", err)
	}
	// 动图与视频同链路：整段动画就是一个切片
	if multimodal.IsAnimatedImage(data) {
		slice, err := multimodal.AnimatedToWatchSlice(data, multimodal.WatchFrameAuto)
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
	slices, err := multimodal.VideoToWatchSlices(abs, multimodal.WatchMaxSlices, multimodal.WatchFrameAuto)
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
	data, err := downloadRemoteVideoBytes(rawURL, maxRemoteVideoBytes)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrVideoExpired, err)
	}
	if len(data) == 0 {
		return "", fmt.Errorf("%w: 读取视频字节为空", ErrVideoExpired)
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

// downloadRemoteVideoBytes 回源读取远程视频；超过上限时明确报错，不返回半段视频。
func downloadRemoteVideoBytes(rawURL string, maxBytes int64) ([]byte, error) {
	resp, err := netsource.Open(rawURL, remoteVideoDownloadTimeout)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	buf := &bytes.Buffer{}
	if err := netsource.CopyLimited(buf, resp.Body, maxBytes); err != nil {
		return nil, fmt.Errorf("视频下载失败: %w", err)
	}
	return buf.Bytes(), nil
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

// storeResourceSummary 把视觉理解写回资源目录（定向问题的答复不入库）
func storeResourceSummary(res resource.Resource, text string) {
	text = strings.TrimSpace(text)
	if text == "" {
		return
	}
	if err := resource.SetSummary(res.ID, agentbase.CutRunes(text, summaryStoreChars), nil); err != nil {
		logger.Warn("CrystalAstral", "视觉理解：写入 %s 描述失败: %v", res.Handle, err)
	}
}
