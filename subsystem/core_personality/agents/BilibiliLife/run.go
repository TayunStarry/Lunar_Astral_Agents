package BilibiliLife

// 生活链 · B 站真实浏览器版：一次完整生活。
//
// 流程：
//   打开 B 站首页 → 未登录则点登录并等扫码（10s 一次，60s 放弃）
//   → 首页选一个视频，停留详情页
//   → 下载 → 60 秒切片 → 逐片抽帧理解（合成“一个视频”的整体感受）
//   → 滚到评论区读 ≤15 条 → 生成并发布 1–3 条评论
//   → 用看过的视频发动态并 @钛宇星光阁
//   → 回复这条动态下的评论（≤5 条）
//
// 本模块不经过 goja；TUI 只负责启动，进度全部写引擎日志。

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"

	"LunarSubsystem/LoggerGeneral"
)

const (
	// 目标账号在 B 站的实际昵称是「钛宇-星光阁」（带连字符），
	// @ 文本、at_uids、ctrl 必须与它一致，否则动态接口会返回 4126126。
	targetName = "钛宇-星光阁"
	targetUID  = int64(356123824)
	maxVideos  = 1
	maxSlices  = 3
	maxRead    = 15
	maxWrite   = 2
	maxReplies = 5
)

var runMu sync.Mutex

// Start 由 TUI 调用：立刻返回一句状态，完整流程在后台跑；已有任务时拒绝重复启动。
func Start() (string, error) {
	if !runMu.TryLock() {
		return "已有一次刷视频正在进行，等它结束再试", nil
	}
	go func() {
		defer runMu.Unlock()
		defer func() {
			if r := recover(); r != nil {
				LoggerGeneral.Error("BilibiliLife", "本次刷视频 panic: %v", r)
			}
		}()
		if err := runLife(); err != nil {
			LoggerGeneral.Error("BilibiliLife", "本次刷视频失败: %v", err)
			return
		}
		LoggerGeneral.Info("BilibiliLife", "本次刷视频完成")
	}()
	return "已启动：正在打开 B 站浏览器", nil
}

// Run 同步执行一次，供测试/脚本使用；TUI 用 Start。
func Run() error {
	runMu.Lock()
	defer runMu.Unlock()
	return runLife()
}

func runLife() error {
	started := time.Now()
	cfg := LoadConfig()
	LoggerGeneral.Info("BilibiliLife", "B站生活开始（真实浏览器）")

	browser, err := openBilibiliBrowser()
	if err != nil {
		return err
	}
	defer browser.close()

	if err := browser.ensureLogin(); err != nil {
		browser.close()
		return err
	}

	link, err := browser.selectVideo(cfg)
	if err != nil {
		return err
	}
	bvid := ""
	if m := bvidRe.FindStringSubmatch(link.Href); len(m) > 0 {
		bvid = m[1]
	}
	if bvid == "" {
		return fmt.Errorf("详情页 URL 里没有 BV 号: %s", link.Href)
	}
	info, err := fetchVideoInfo(bvid)
	if err != nil {
		return fmt.Errorf("读取视频信息失败: %w", err)
	}
	if info.Title == "" {
		info.Title = link.Title
	}
	LoggerGeneral.Info("BilibiliLife", "视频信息: 《%s》 aid=%d cid=%d 时长=%ds", info.Title, info.Aid, info.Cid, info.Duration)

	playURL, err := fetchPlayURL(info.Bvid, info.Cid, 32)
	if err != nil {
		return fmt.Errorf("获取播放直链失败: %w", err)
	}
	seconds := maxSlices * sliceSec
	if info.Duration > 0 && info.Duration < seconds {
		seconds = info.Duration
	}
	local, err := downloadClip(playURL, info.Bvid, seconds)
	if err != nil {
		return fmt.Errorf("下载视频失败: %w", err)
	}
	summary, slices, err := understandVideo(local, info, maxSlices)
	if err != nil {
		return fmt.Errorf("理解视频失败: %w", err)
	}
	okSlices := 0
	for _, one := range slices {
		if one.Text != "" {
			okSlices++
		}
	}
	LoggerGeneral.Info("BilibiliLife", "切片理解完成：%d/%d 片；摘要 %d 字", okSlices, len(slices), len([]rune(summary)))

	// 读评论：先滚动到评论区，再优先用 API（DOM 只做兜底）
	browser.scrollToComments()
	comments, cErr := browser.readCommentsAPI(info.Aid, maxRead)
	if cErr != nil || len(comments) == 0 {
		if dom, domErr := browser.readComments(maxRead); domErr == nil && len(dom) > 0 {
			comments = make([]commentItem, 0, len(dom))
			for _, c := range dom {
				comments = append(comments, commentItem{User: c.User, Content: c.Content, Like: c.Like})
			}
		}
	}
	if len(comments) > maxRead {
		comments = comments[:maxRead]
	}
	LoggerGeneral.Info("BilibiliLife", "读取评论 %d 条", len(comments))

	// 写评论 1–3 条（默认 2）
	writeCount := maxWrite
	if writeCount < 1 {
		writeCount = 1
	}
	if writeCount > 3 {
		writeCount = 3
	}
	for i := 0; i < writeCount; i++ {
		text, gErr := generateComment(info, summary, comments)
		if gErr != nil {
			LoggerGeneral.Warn("BilibiliLife", "第 %d 条评论生成失败: %v", i+1, gErr)
			continue
		}
		if err := browser.postComment(info.Aid, text); err != nil {
			LoggerGeneral.Warn("BilibiliLife", "第 %d 条评论发布失败: %v", i+1, err)
			continue
		}
		LoggerGeneral.Info("BilibiliLife", "评论已发布（%d/%d）：%s", i+1, writeCount, text)
		time.Sleep(2 * time.Second)
	}

	// 单独的 @分享额度：把原本要发在动态里的分享文案写进这条视频的评论，
	// 不占用上面的普通评论额度。评论接口不支持 ctrl，@ 以正文形式出现。
	shareText, gErr := generateShareComment(info, summary, comments)
	if gErr != nil {
		LoggerGeneral.Warn("BilibiliLife", "分享评论生成失败: %v", gErr)
	} else {
		shareText = strings.TrimSpace(strings.ReplaceAll(shareText, "@", ""))
		shareText = fmt.Sprintf("%s @%s", truncate(shareText, 180), targetName)
		if err := browser.postComment(info.Aid, shareText); err != nil {
			LoggerGeneral.Warn("BilibiliLife", "分享评论发布失败: %v", err)
		} else {
			LoggerGeneral.Info("BilibiliLife", "分享评论已发布（独立额度）：%s", shareText)
			time.Sleep(2 * time.Second)
		}
	}

	// 动态 = 这一次生活的日记，不再发视频卡、不再在动态里 @。
	dynID := ""
	diary, gErr := generateDiary(info, summary, comments)
	if gErr != nil {
		LoggerGeneral.Warn("BilibiliLife", "日记生成失败: %v", gErr)
	} else if dynID, err = browser.publishDiary(diary); err != nil {
		LoggerGeneral.Warn("BilibiliLife", "日记动态发布失败: %v", err)
	} else {
		LoggerGeneral.Info("BilibiliLife", "日记动态已发布 dynId=%s：%s", dynID, diary)
		// 给动态评论索引一点时间，否则刚发完读评论常是 -404
		time.Sleep(15 * time.Second)
	}

	// 回复动态评论（≤5）
	if dynID != "" {
		if err := browser.replyDynamic(dynID, maxReplies); err != nil {
			LoggerGeneral.Warn("BilibiliLife", "回复动态评论失败: %v", err)
		}
	}

	LoggerGeneral.Info("BilibiliLife", "B站生活结束：视频《%s》，评论 %d 条，动态 %s", info.Title, len(comments), dynID)
	LoggerGeneral.Info("BilibiliLife", "总耗时 %s", time.Since(started).Truncate(time.Second))
	return nil
}

func generateComment(info videoInfo, summary string, comments []commentItem) (string, error) {
	var hot []string
	for i, c := range comments {
		if i >= 8 {
			break
		}
		hot = append(hot, fmt.Sprintf("%s：%s", c.User, c.Content))
	}
	material := fmt.Sprintf("视频：《%s》\nUP主：%s\n我的观看感受：\n%s\n\n评论区节选：\n%s",
		info.Title, info.Owner, truncate(summary, 1600), strings.Join(hot, "\n"))
	system := "你是月华，一个普通的 B 站用户。根据视频内容和评论区写一条自然、具体、不套路的评论。" +
		"只输出评论正文，1–120 字；不要链接、不要 @、不要暴露 AI/模型/程序。"
	text, err := chatText(system, material)
	if err != nil {
		return "", err
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return "", fmt.Errorf("评论为空")
	}
	text = truncate(text, 120)
	return text, nil
}

// generateShareComment 生成原本要发在动态里的分享文案；发送时会追加 @目标，
// 作为这条视频下的一条独立“@分享评论”，不占普通评论额度。
func generateShareComment(info videoInfo, summary string, comments []commentItem) (string, error) {
	var hot []string
	for i, c := range comments {
		if i >= 5 {
			break
		}
		hot = append(hot, fmt.Sprintf("%s：%s", c.User, c.Content))
	}
	material := fmt.Sprintf("视频：《%s》\nUP主：%s\n我的观看感受：\n%s\n\n评论区节选：\n%s",
		info.Title, info.Owner, truncate(summary, 1200), strings.Join(hot, "\n"))
	system := "你是月华，刚看完这个视频，想写一句分享/推荐评论。只输出正文，80 字以内；" +
		"不要 @、不要链接、不要暴露 AI/模型/程序。"
	text, err := chatText(system, material)
	if err != nil {
		return "", err
	}
	text = strings.TrimSpace(strings.ReplaceAll(text, "@", ""))
	if text == "" {
		return "", fmt.Errorf("分享评论为空")
	}
	return truncate(text, 80), nil
}

// generateDiary 生成这一轮生活的日记，动态正文就是日记。
func generateDiary(info videoInfo, summary string, comments []commentItem) (string, error) {
	var hot []string
	for i, c := range comments {
		if i >= 6 {
			break
		}
		hot = append(hot, fmt.Sprintf("%s：%s", c.User, c.Content))
	}
	material := fmt.Sprintf("视频：《%s》\nUP主：%s\n我的观看感受：\n%s\n\n评论区节选：\n%s",
		info.Title, info.Owner, truncate(summary, 1800), strings.Join(hot, "\n"))
	system := "你是月华。把这一轮看视频、看评论的心情写成一条约 200 字的第一人称日记，只输出正文；" +
		"不要 @、不要链接、不要列表/标题/markdown、不要暴露 AI/模型/程序；要具体，不要套话。"
	text, err := chatText(system, material)
	if err != nil {
		return "", err
	}
	text = strings.TrimSpace(strings.ReplaceAll(text, "@", ""))
	if text == "" {
		return "", fmt.Errorf("日记为空")
	}
	return truncate(text, 300), nil
}

// readCommentsAPI 在页面上下文读视频评论（type=1）。
func (w *webSession) readCommentsAPI(aid int64, max int) ([]commentItem, error) {
	target := fmt.Sprintf("https://api.bilibili.com/x/v2/reply?type=1&oid=%d&sort=2&ps=20&pn=1", aid)
	text, err := w.pageFetchText("GET", target, "", "", 25*time.Second)
	if err != nil {
		return nil, err
	}
	var resp struct {
		Code int `json:"code"`
		Data struct {
			Replies []struct {
				Member struct {
					Uname string `json:"uname"`
				} `json:"member"`
				Content struct {
					Message string `json:"message"`
				} `json:"content"`
				Like int `json:"like"`
			} `json:"replies"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(text), &resp); err != nil {
		return nil, err
	}
	if resp.Code != 0 {
		return nil, fmt.Errorf("评论接口 code=%d", resp.Code)
	}
	out := make([]commentItem, 0, len(resp.Data.Replies))
	for _, r := range resp.Data.Replies {
		if len(out) >= max {
			break
		}
		if strings.TrimSpace(r.Content.Message) == "" {
			continue
		}
		out = append(out, commentItem{User: r.Member.Uname, Content: r.Content.Message, Like: r.Like})
	}
	return out, nil
}

// postComment 在页面上下文发视频评论（type=1）。
func (w *webSession) postComment(aid int64, message string) error {
	csrf := w.csrf()
	if csrf == "" {
		return fmt.Errorf("页面里读不到 bili_jct，无法发评论")
	}
	form := url.Values{}
	form.Set("oid", fmt.Sprintf("%d", aid))
	form.Set("type", "1")
	form.Set("message", message)
	form.Set("plat", "1")
	form.Set("csrf", csrf)
	text, err := w.pageFetchText("POST", "https://api.bilibili.com/x/v2/reply/add",
		"application/x-www-form-urlencoded", form.Encode(), 25*time.Second)
	if err != nil {
		return err
	}
	var resp struct {
		Code int    `json:"code"`
		Msg  string `json:"message"`
	}
	if err := json.Unmarshal([]byte(text), &resp); err != nil {
		return err
	}
	if resp.Code != 0 {
		return fmt.Errorf("B站拒绝: code=%d %s", resp.Code, resp.Msg)
	}
	return nil
}

// publishDiary 把这一轮生活的日记作为纯文本动态发布（type=4，不带视频卡、不带 @）。
func (w *webSession) publishDiary(text string) (string, error) {
	csrf := w.csrf()
	if csrf == "" {
		return "", fmt.Errorf("页面里读不到 bili_jct，无法发日记动态")
	}
	clean := strings.TrimSpace(text)
	if clean == "" {
		return "", fmt.Errorf("日记正文为空")
	}
	fields := map[string]string{
		"dynamic_id":        "0",
		"type":              "4",
		"rid":               "0",
		"content":           clean,
		"up_choose_comment": "0",
		"up_close_comment":  "0",
		"extension":         `{"emoji_type":1,"from":{"emoji_type":1},"flag_cfg":{}}`,
		"csrf":              csrf,
		"csrf_token":        csrf,
	}
	raw, err := w.pageFetchForm("https://api.vc.bilibili.com/dynamic_svr/v1/dynamic_svr/create", fields, 30*time.Second)
	if err != nil {
		return "", err
	}
	var resp struct {
		Code int    `json:"code"`
		Msg  string `json:"message"`
		Data struct {
			DynamicID    int64  `json:"dynamic_id"`
			DynamicIDStr string `json:"dynamic_id_str"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(raw), &resp); err != nil {
		return "", err
	}
	if resp.Code != 0 {
		return "", fmt.Errorf("B站拒绝日记动态: code=%d %s", resp.Code, resp.Msg)
	}
	id := resp.Data.DynamicIDStr
	if id == "" && resp.Data.DynamicID != 0 {
		id = fmt.Sprintf("%d", resp.Data.DynamicID)
	}
	if id == "" {
		return "", fmt.Errorf("日记动态已提交但没有返回 dynamic_id")
	}
	return id, nil
}

// replyDynamic 读取动态评论并逐条回复，最多 max 条。
func (w *webSession) replyDynamic(dynID string, max int) error {
	listURL := fmt.Sprintf("https://api.bilibili.com/x/v2/reply?type=17&oid=%s&sort=2&ps=20", url.QueryEscape(dynID))
	raw, err := w.pageFetchText("GET", listURL, "", "", 25*time.Second)
	if err != nil {
		return err
	}
	var list struct {
		Code int `json:"code"`
		Data struct {
			Replies []struct {
				Rpid      int64  `json:"rpid"`
				RpidStr   string `json:"rpid_str"`
				Root      int64  `json:"root"`
				RootStr   string `json:"root_str"`
				Parent    int64  `json:"parent"`
				ParentStr string `json:"parent_str"`
				Member    struct {
					Uname string `json:"uname"`
				} `json:"member"`
				Content struct {
					Message string `json:"message"`
				} `json:"content"`
			} `json:"replies"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(raw), &list); err != nil {
		return err
	}
	if list.Code == -404 {
		LoggerGeneral.Info("BilibiliLife", "动态刚发布，评论索引尚未就绪或暂时没有评论")
		return nil
	}
	if list.Code != 0 {
		return fmt.Errorf("动态评论接口 code=%d", list.Code)
	}
	csrf := w.csrf()
	if csrf == "" {
		return fmt.Errorf("页面里读不到 bili_jct，无法回复动态")
	}
	sent := 0
	for _, r := range list.Data.Replies {
		if sent >= max {
			break
		}
		if strings.TrimSpace(r.Content.Message) == "" {
			continue
		}
		reply, err := generateReply(r.Member.Uname, r.Content.Message)
		if err != nil {
			LoggerGeneral.Warn("BilibiliLife", "动态回复文案生成失败: %v", err)
			continue
		}
		rpid := r.RpidStr
		if rpid == "" && r.Rpid != 0 {
			rpid = fmt.Sprintf("%d", r.Rpid)
		}
		root := r.RootStr
		if root == "" && r.Root != 0 {
			root = fmt.Sprintf("%d", r.Root)
		}
		parent := r.ParentStr
		if parent == "" && r.Parent != 0 {
			parent = fmt.Sprintf("%d", r.Parent)
		}
		if root == "" {
			root = rpid
		}
		if parent == "" {
			parent = rpid
		}
		form := url.Values{}
		form.Set("type", "17")
		form.Set("oid", dynID)
		form.Set("message", reply)
		form.Set("plat", "1")
		form.Set("csrf", csrf)
		form.Set("root", root)
		form.Set("parent", parent)
		text, err := w.pageFetchText("POST", "https://api.bilibili.com/x/v2/reply/add",
			"application/x-www-form-urlencoded", form.Encode(), 25*time.Second)
		if err != nil {
			LoggerGeneral.Warn("BilibiliLife", "回复 %s 失败: %v", r.Member.Uname, err)
			continue
		}
		var resp struct {
			Code int    `json:"code"`
			Msg  string `json:"message"`
		}
		if err := json.Unmarshal([]byte(text), &resp); err != nil || resp.Code != 0 {
			LoggerGeneral.Warn("BilibiliLife", "回复 %s 被拒: %s", r.Member.Uname, truncate(text, 160))
			continue
		}
		sent++
		LoggerGeneral.Info("BilibiliLife", "回复动态评论（%d/%d）→ %s：%s", sent, max, r.Member.Uname, reply)
		time.Sleep(2 * time.Second)
	}
	return nil
}

func generateReply(user, message string) (string, error) {
	system := "你是月华，正在回复自己 B 站动态下的评论。只输出一句自然、友好的回复，1–60 字；" +
		"不争论、不引战、不放链接/@、不暴露 AI/模型/程序。"
	text, err := chatText(system, fmt.Sprintf("对方：%s\n评论：%s", user, message))
	if err != nil {
		return "", err
	}
	text = strings.TrimSpace(strings.ReplaceAll(text, "@", ""))
	if text == "" {
		return "", fmt.Errorf("回复为空")
	}
	return truncate(text, 60), nil
}
