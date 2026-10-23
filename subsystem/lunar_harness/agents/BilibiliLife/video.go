package BilibiliLife

// 生活链 · B 站真实浏览器版：视频 API、下载、60 秒切片与逐片理解。

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"LunarSubsystem/GeneralConfig"
	"LunarSubsystem/LunarHarness/engine/console/logger"
	module "LunarSubsystem/LunarHarness/engine/multimodal"
	"LunarSubsystem/LunarHarness/engine/persistence"
	"LunarSubsystem/LunarHarness/prompts"
)

const (
	biliUA      = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0 Safari/537.36"
	biliReferer = "https://www.bilibili.com/"
	sliceSec    = 60
)

var bvidRe = regexp.MustCompile(`(BV[0-9A-Za-z]{10})`)

type videoInfo struct {
	Bvid     string
	Aid      int64
	Cid      int64
	Title    string
	Owner    string
	Duration int
	URL      string
}

func biliHTTPGetJSON(target string, out any) error {
	req, err := http.NewRequest(http.MethodGet, target, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", biliUA)
	req.Header.Set("Referer", biliReferer)
	client := &http.Client{Timeout: 25 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, truncate(string(body), 160))
	}
	return json.Unmarshal(body, out)
}

func fetchVideoInfo(bvid string) (videoInfo, error) {
	var resp struct {
		Code int    `json:"code"`
		Msg  string `json:"message"`
		Data struct {
			Aid      int64  `json:"aid"`
			Cid      int64  `json:"cid"`
			Title    string `json:"title"`
			Duration int    `json:"duration"`
			Owner    struct {
				Name string `json:"name"`
			} `json:"owner"`
		} `json:"data"`
	}
	target := "https://api.bilibili.com/x/web-interface/view?bvid=" + url.QueryEscape(bvid)
	if err := biliHTTPGetJSON(target, &resp); err != nil {
		return videoInfo{}, err
	}
	if resp.Code != 0 {
		return videoInfo{}, fmt.Errorf("view code=%d %s", resp.Code, resp.Msg)
	}
	return videoInfo{
		Bvid:     bvid,
		Aid:      resp.Data.Aid,
		Cid:      resp.Data.Cid,
		Title:    resp.Data.Title,
		Owner:    resp.Data.Owner.Name,
		Duration: resp.Data.Duration,
		URL:      "https://www.bilibili.com/video/" + bvid,
	}, nil
}

// bvidOf 从视频链接里提取 BV 号（无法提取时返回空串）
func bvidOf(href string) string {
	if match := bvidRe.FindStringSubmatch(href); len(match) > 0 {
		return match[1]
	}
	return ""
}

// fetchVideoTags 读取视频的内容标签，参与参考词打分。
// 失败只返回错误、不阻断选片：标签是加分项，不是必需信息。
func fetchVideoTags(bvid string) ([]string, error) {
	var resp struct {
		Code int    `json:"code"`
		Msg  string `json:"message"`
		Data []struct {
			TagName string `json:"tag_name"`
		} `json:"data"`
	}
	target := "https://api.bilibili.com/x/tag/archive/tags?bvid=" + url.QueryEscape(bvid)
	if err := biliHTTPGetJSON(target, &resp); err != nil {
		return nil, err
	}
	if resp.Code != 0 {
		return nil, fmt.Errorf("tags code=%d %s", resp.Code, resp.Msg)
	}
	tags := make([]string, 0, len(resp.Data))
	for _, item := range resp.Data {
		if name := strings.TrimSpace(item.TagName); name != "" {
			tags = append(tags, name)
		}
	}
	return tags, nil
}

func fetchPlayURL(bvid string, cid int64, qn int) (string, error) {
	var resp struct {
		Code int    `json:"code"`
		Msg  string `json:"message"`
		Data struct {
			Durl []struct {
				URL string `json:"url"`
			} `json:"durl"`
		} `json:"data"`
	}
	target := fmt.Sprintf("https://api.bilibili.com/x/player/playurl?bvid=%s&cid=%d&qn=%d&fnval=1&platform=html5&high_quality=1",
		url.QueryEscape(bvid), cid, qn)
	if err := biliHTTPGetJSON(target, &resp); err != nil {
		return "", err
	}
	if resp.Code != 0 {
		return "", fmt.Errorf("playurl code=%d %s", resp.Code, resp.Msg)
	}
	if len(resp.Data.Durl) == 0 || resp.Data.Durl[0].URL == "" {
		return "", fmt.Errorf("playurl 没有 durl")
	}
	return resp.Data.Durl[0].URL, nil
}

func downloadClip(playURL, bvid string, seconds int) (string, error) {
	cacheDir := filepath.Join(*GeneralConfig.LocalDir, "cache")
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return "", err
	}
	out := filepath.Join(cacheDir, fmt.Sprintf("bili_life_%s_%d.mp4", bvid, time.Now().UnixNano()))
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		return "", fmt.Errorf("找不到 ffmpeg: %w", err)
	}
	args := []string{
		"-y", "-hide_banner", "-loglevel", "error",
		"-headers", "Referer: https://www.bilibili.com/\r\nUser-Agent: " + biliUA + "\r\n",
		"-i", playURL,
		"-t", fmt.Sprintf("%d", seconds),
		"-c", "copy",
		out,
	}
	cmd := exec.Command(ffmpeg, args...)
	if data, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("ffmpeg 下载失败: %v %s", err, truncate(string(data), 300))
	}
	if st, err := os.Stat(out); err != nil || st.Size() == 0 {
		return "", fmt.Errorf("下载文件为空")
	}
	return out, nil
}

type sliceUnderstanding struct {
	Index int
	Text  string
	Error string
}

func understandVideo(localPath string, info videoInfo, maxSlices int) (string, []sliceUnderstanding, error) {
	defer os.Remove(localPath)
	rawSegments, err := module.VideoToMedia(localPath)
	if err != nil {
		return "", nil, fmt.Errorf("视频切片失败: %w", err)
	}
	// 截断下载的尾巴经常只有 0.1 秒，却也会生成一个切片；不能把它当成一次模型调用。
	segments := make([]module.MediaSegment, 0, len(rawSegments))
	for _, seg := range rawSegments {
		if seg.End-seg.Start < 1.0 {
			_ = os.Remove(filepath.Join(*GeneralConfig.LocalDir, "multimedia", filepath.FromSlash(seg.File)))
			continue
		}
		segments = append(segments, seg)
	}
	if len(segments) == 0 && len(rawSegments) > 0 {
		segments = rawSegments[:1]
	}
	if maxSlices <= 0 {
		maxSlices = 3
	}
	if len(segments) > maxSlices {
		segments = segments[:maxSlices]
	}
	system, err := prompts.Read("descriptorMediaRole.md")
	if err != nil {
		return "", nil, fmt.Errorf("读取描述者提示词失败: %w", err)
	}
	var parts []string
	var results []sliceUnderstanding
	for i, seg := range segments {
		abs := filepath.Join(*GeneralConfig.LocalDir, "multimedia", filepath.FromSlash(seg.File))
		groups, frameErr := module.VideoToFrames(abs)
		if frameErr != nil {
			results = append(results, sliceUnderstanding{Index: i + 1, Error: frameErr.Error()})
			logger.Warn("BilibiliLife", "第 %d 片抽帧失败: %v", i+1, frameErr)
			continue
		}
		var images []string
		for _, group := range groups {
			for _, frame := range group.Frames {
				if strings.TrimSpace(frame.Image) != "" {
					images = append(images, frame.Image)
				}
				if len(images) >= 60 {
					break
				}
			}
			if len(images) >= 60 {
				break
			}
		}
		prompt := fmt.Sprintf("这是同一个视频《%s》的第 %d/%d 个 60 秒切片（UP主：%s）。请按描述者格式客观描述这一段画面内容，不要当成另一个视频。",
			info.Title, i+1, len(segments), info.Owner)
		text, chatErr := chatWithImages(string(system), prompt, images)
		if chatErr != nil {
			results = append(results, sliceUnderstanding{Index: i + 1, Error: chatErr.Error()})
			logger.Warn("BilibiliLife", "第 %d 片理解失败: %v", i+1, chatErr)
			continue
		}
		text = strings.TrimSpace(text)
		results = append(results, sliceUnderstanding{Index: i + 1, Text: text})
		parts = append(parts, fmt.Sprintf("【第 %d/%d 个 60 秒切片】%s", i+1, len(segments), text))
	}
	// 清理切片与临时 mp4：切片已理解完，不留在 footage 里
	for _, seg := range segments {
		_ = os.Remove(filepath.Join(*GeneralConfig.LocalDir, "multimedia", filepath.FromSlash(seg.File)))
	}
	_ = os.Remove(localPath)
	if len(parts) == 0 {
		return "", results, fmt.Errorf("所有切片理解都失败")
	}
	summary := fmt.Sprintf("《%s》是同一个视频，下面按时间顺序给出 %d 个 60 秒切片的理解，请合成为对这一个视频的整体观看感受：\n\n%s",
		info.Title, len(parts), strings.Join(parts, "\n\n"))
	return summary, results, nil
}

// prepareVideoSegments 把下载好的视频切成 60 秒片段，过滤 1 秒以下的尾巴。
func prepareVideoSegments(localPath string, maxSlices int) ([]module.MediaSegment, error) {
	rawSegments, err := module.VideoToMedia(localPath)
	if err != nil {
		return nil, fmt.Errorf("视频切片失败: %w", err)
	}
	segments := make([]module.MediaSegment, 0, len(rawSegments))
	for _, seg := range rawSegments {
		if seg.End-seg.Start < 1.0 {
			_ = os.Remove(filepath.Join(*GeneralConfig.LocalDir, "multimedia", filepath.FromSlash(seg.File)))
			continue
		}
		segments = append(segments, seg)
	}
	if len(segments) == 0 && len(rawSegments) > 0 {
		segments = rawSegments[:1]
	}
	if maxSlices <= 0 {
		maxSlices = 3
	}
	if len(segments) > maxSlices {
		for _, extra := range segments[maxSlices:] {
			_ = os.Remove(filepath.Join(*GeneralConfig.LocalDir, "multimedia", filepath.FromSlash(extra.File)))
		}
		segments = segments[:maxSlices]
	}
	return segments, nil
}

// understandOneSlice 只理解一个 60 秒切片；供可中断的状态机逐片调用。
func understandOneSlice(info videoInfo, seg module.MediaSegment, index, total int) (string, error) {
	abs := filepath.Join(*GeneralConfig.LocalDir, "multimedia", filepath.FromSlash(seg.File))
	groups, err := module.VideoToFrames(abs)
	if err != nil {
		return "", fmt.Errorf("抽帧失败: %w", err)
	}
	var images []string
	for _, group := range groups {
		for _, frame := range group.Frames {
			if strings.TrimSpace(frame.Image) != "" {
				images = append(images, frame.Image)
			}
			if len(images) >= 60 {
				break
			}
		}
		if len(images) >= 60 {
			break
		}
	}
	system, err := prompts.Read("descriptorMediaRole.md")
	if err != nil {
		return "", fmt.Errorf("读取描述者提示词失败: %w", err)
	}
	prompt := fmt.Sprintf("这是同一个视频《%s》的第 %d/%d 个 60 秒切片（UP主：%s）。请按描述者格式客观描述这一段画面内容，不要当成另一个视频。",
		info.Title, index, total, info.Owner)
	text, err := chatWithImages(string(system), prompt, images)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(text), nil
}

func combineSliceSummaries(info videoInfo, summaries []string) string {
	parts := make([]string, 0, len(summaries))
	for i, text := range summaries {
		parts = append(parts, fmt.Sprintf("【第 %d/%d 个 60 秒切片】%s", i+1, len(summaries), text))
	}
	return fmt.Sprintf("《%s》是同一个视频，下面按时间顺序给出 %d 个 60 秒切片的理解，请合成为对这一个视频的整体观看感受：\n\n%s",
		info.Title, len(parts), strings.Join(parts, "\n\n"))
}

// storeVideoSummary 把一条视频的整体理解写入视频摘要库（media.db 的 video_summary 表，
// 键 = 视频主页 URL）。月华之后用该 URL 查记忆时可直接召回这段理解文本，
// 不需要重新看一遍视频；重复刷到同一视频时按键覆盖更新。
// 写入失败仅告警，不阻断生活链。
func storeVideoSummary(info videoInfo, summaries []string) {
	if strings.TrimSpace(info.URL) == "" {
		return
	}
	var builder strings.Builder
	fmt.Fprintf(&builder, "《%s》｜UP主：%s\n", info.Title, info.Owner)
	for i, text := range summaries {
		if strings.TrimSpace(text) == "" {
			continue
		}
		fmt.Fprintf(&builder, "【第 %d/%d 个 60 秒切片】%s\n", i+1, len(summaries), text)
	}
	summary := strings.TrimSpace(builder.String())
	if summary == "" {
		return
	}
	result := persistence.ExecuteSQLOn("media",
		"INSERT OR REPLACE INTO video_summary (key, value) VALUES (?, ?)", []any{info.URL, summary})
	if !result.Success || len(result.Results) == 0 || !result.Results[0].Success {
		logger.Warn("BilibiliLife", "视频理解写入摘要库失败(%s): %s", info.URL, result.Error)
		return
	}
	logger.Info("BilibiliLife", "视频理解已写入摘要库: %s", info.URL)
}

func removeSegments(segments []module.MediaSegment) {
	for _, seg := range segments {
		_ = os.Remove(filepath.Join(*GeneralConfig.LocalDir, "multimedia", filepath.FromSlash(seg.File)))
	}
}
