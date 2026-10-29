package BilibiliViewer

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

	"LunarSubsystem/LunarHarness/engine/config"
	"LunarSubsystem/LunarHarness/engine/console/logger"
	module "LunarSubsystem/LunarHarness/engine/multimodal"
	"LunarSubsystem/LunarHarness/engine/resource"
	"LunarSubsystem/LunarHarness/prompts"
)

const (
	biliUA      = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0 Safari/537.36"
	biliReferer = "https://www.bilibili.com/"
)

var bvidRe = regexp.MustCompile(`(BV[0-9A-Za-z]{10})`)

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
			Desc     string `json:"desc"`
			Pic      string `json:"pic"`
			Duration int    `json:"duration"`
			Pubdate  int64  `json:"pubdate"`
			Owner    struct {
				Name string `json:"name"`
			} `json:"owner"`
			Stat struct {
				View    int64 `json:"view"`
				Like    int64 `json:"like"`
				Coin    int64 `json:"coin"`
				Favorit int64 `json:"favorite"`
				Reply   int64 `json:"reply"`
				Danmaku int64 `json:"danmaku"`
			} `json:"stat"`
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
		Bvid:        bvid,
		Aid:         resp.Data.Aid,
		Cid:         resp.Data.Cid,
		Title:       resp.Data.Title,
		Owner:       resp.Data.Owner.Name,
		Duration:    resp.Data.Duration,
		URL:         "https://www.bilibili.com/video/" + bvid,
		Desc:        resp.Data.Desc,
		Pic:         resp.Data.Pic,
		PublishTime: resp.Data.Pubdate,
		View:        resp.Data.Stat.View,
		Like:        resp.Data.Stat.Like,
		Coin:        resp.Data.Stat.Coin,
		Favorite:    resp.Data.Stat.Favorit,
		ReplyNum:    resp.Data.Stat.Reply,
		Danmaku:     resp.Data.Stat.Danmaku,
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
	cacheDir := filepath.Join(config.LocalDir, "cache")
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

// prepareVideoSegments 按统一观看管线切分下载好的视频：60 秒一片，丢弃不足 1 秒的尾巴。
//
// 切片只在内存里算出时间区间，不再生成 footage 片段文件；
// 抽帧在逐片观看时由 module.SegmentImages 现场完成。
func prepareVideoSegments(localPath string, maxSlices int) ([]module.MediaSegment, error) {
	duration, err := module.GetVideoDuration(localPath)
	if err != nil {
		return nil, fmt.Errorf("获取视频时长失败: %w", err)
	}
	return module.SplitMediaSegments(duration, maxSlices), nil
}

// understandOneSlice 观看一个 60 秒切片；供可中断的状态机逐片调用。
//
// 抽帧与任务措辞全部来自统一观看管线（module.SegmentImages / module.WatchTask），
// 与描述者的影像理解一字不差，本智能体只补充视频背景。
func understandOneSlice(localPath string, info videoInfo, seg module.MediaSegment, index, total int) (string, error) {
	images, err := module.SegmentImages(localPath, seg, module.WatchFrameMax)
	if err != nil {
		return "", fmt.Errorf("抽帧失败: %w", err)
	}
	system, err := prompts.Read("lifeRole.md")
	if err != nil {
		return "", fmt.Errorf("读取人设提示词失败: %w", err)
	}
	slice := module.WatchSlice{Index: index, Start: seg.Start, End: seg.End, Images: images}
	context := fmt.Sprintf("你正在看《%s》，UP主：%s。", info.Title, info.Owner)
	prompt := module.WatchTask(index, total, slice, context)
	// 单次调用、不重试：某次输出触发防死循环时，直接放弃该切片
	text, err := chatWithImagesOnce(string(system), prompt, images)
	if err != nil {
		return "", err
	}
	return cleanMemoryText(text), nil
}

// summarizeWatch 把各切片理解合成为第一人称观影感受。
//
// 切片理解与时间轴只当输入材料，输出必须是人格化的印象/感受；
// 机械化的过程文本既不进记忆库，也不进 URL 主表。
func summarizeWatch(video *videoRecord, summaries []string) string {
	if video == nil || len(summaries) == 0 {
		return ""
	}
	material := strings.Builder{}
	fmt.Fprintf(&material, "视频：《%s》\n", video.Info.Title)
	fmt.Fprintf(&material, "UP主：%s\n", video.Info.Owner)
	material.WriteString("画面理解（只作素材，不要照抄）：\n")
	for _, text := range summaries {
		text = strings.TrimSpace(text)
		if text == "" {
			continue
		}
		material.WriteString("- ")
		material.WriteString(text)
		material.WriteString("\n")
	}

	// 合成任务的措辞与描述者共用一份（prompts/mediaWatchMergeTask.md）
	task := module.WatchMergeTask()
	system := task
	if persona, err := prompts.Read("lifeRole.md"); err == nil {
		system = string(persona) + "\n\n## 本次任务\n\n" + task
	}
	text, err := chatTextModel(modelPrimary, system, material.String())
	if err != nil {
		logger.Warn("BilibiliViewer", "生成观影感受失败: %v", err)
		return ""
	}
	return cleanMemoryText(text)
}

// storeVideoSummary 把人格化的观影感受写入 URL 主表的唯一解析文本。
//
// 只按完整页面 URL 存一份；不再按 BV 号或分片地址另存（规范一.3）。
func storeVideoSummary(info videoInfo, summary string) {
	key := resource.NormalizeURL(info.URL)
	summary = cleanMemoryText(summary)
	if key == "" || summary == "" {
		return
	}
	// 确保该完整 URL 已绑定视频句柄，再写入唯一解析文本
	if _, err := resource.AdmitURL(resource.URLAdmitInput{
		URL: key, Title: info.Title, Origin: resource.OriginChannel,
	}); err != nil {
		logger.Warn("BilibiliViewer", "观影感受登记句柄失败(%s): %v", key, err)
		return
	}
	if err := resource.SetParseText(key, summary); err != nil {
		logger.Warn("BilibiliViewer", "观影感受写入解析文本失败(%s): %v", key, err)
		return
	}
	logger.Info("BilibiliViewer", "观影感受已写入 URL 主表: %s", key)
}

// cleanupMechanicalVideoSummaries 新体系不再保留历史摘要库，直接返回 0。
func cleanupMechanicalVideoSummaries() int {
	return 0
}

// isBilibiliSummaryKey 判断摘要库的 key 是否来自 B 站（视频 URL 或 BV 号）。
func isBilibiliSummaryKey(key string) bool {
	key = strings.TrimSpace(key)
	if strings.Contains(key, "bilibili.com") {
		return true
	}
	return regexp.MustCompile(`^BV[0-9A-Za-z]+$`).MatchString(key)
}
