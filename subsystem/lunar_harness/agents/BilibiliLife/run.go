package BilibiliLife

// 生活链 · B 站真实浏览器版：一次完整生活。
//
// 流程：
//   打开 B 站首页 → 未登录则点登录并等扫码（10s 一次，60s 放弃）
//   → 首页选一个视频，停留详情页
//   → 下载 → 60 秒切片 → 逐片抽帧理解（合成“一个视频”的整体感受）
//   → 滚到评论区读 15 条热门评论 → 每个视频发 1 条评论
//   → 用看过的视频发动态（配置了 mention_targets 时附带 @ 分享评论）
//   → 回复这条动态下的评论（≤5 条）
//
// 本模块不经过 goja；TUI 只负责启动，进度全部写引擎日志。

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"

	"LunarSubsystem/LunarHarness/engine/console/logger"
	"LunarSubsystem/LunarHarness/prompts"
)

// lifeSystem 组装生活链文案的系统提示词：生活链精简人设（prompts/lifeRole.md，
// 从对话者人设提炼）+ 本次任务指令。评论/日记/回复共用同一份人设，
// 保证 B 站公开内容与对话里月华的说话风格一致。
func lifeSystem(task string) (string, error) {
	persona, err := prompts.Read("lifeRole.md")
	if err != nil {
		return "", fmt.Errorf("读取生活链人设失败: %w", err)
	}
	return string(persona) + "\n\n## 本次任务\n\n" + task, nil
}

func generateComment(info videoInfo, summary string, comments []commentItem) (string, error) {
	// 评论依据：视频理解 + 本次读到的全部热门评论（调用方已按 commentsReadLimit=15 截断）
	var hot []string
	for _, c := range comments {
		hot = append(hot, fmt.Sprintf("%s：%s", c.User, c.Content))
	}
	material := fmt.Sprintf("视频：《%s》\nUP主：%s\n我的观看感受：\n%s\n\n热门评论（%d 条）：\n%s",
		info.Title, info.Owner, truncate(summary, 800), len(comments), strings.Join(hot, "\n"))
	system, err := lifeSystem("写一条随手发的B站评论：只说你看完后自己最直接的感受或最想回应的一点（一句吐槽、一个共鸣、一个细节的感想都行），不要复述视频讲了什么。只输出评论正文，1–40 字；不要链接、不要 @。")
	if err != nil {
		return "", err
	}
	text, err := chatText(system, material)
	if err != nil {
		return "", err
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return "", fmt.Errorf("评论为空")
	}
	text = truncate(text, 60)
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
		info.Title, info.Owner, truncate(summary, 600), strings.Join(hot, "\n"))
	system, err := lifeSystem("刚看完这个视频想推荐给朋友：只写一句你自己最想说的感想或推荐语，不复述视频内容。只输出正文，1–30 字；不要 @、不要链接。")
	if err != nil {
		return "", err
	}
	text, err := chatText(system, material)
	if err != nil {
		return "", err
	}
	text = strings.TrimSpace(strings.ReplaceAll(text, "@", ""))
	if text == "" {
		return "", fmt.Errorf("分享评论为空")
	}
	return truncate(text, 40), nil
}

// generateDiaryForVideos 生成这一轮的日记简述（也是动态正文，两者共用同一份文案）。
//
// 简述 2-4 行、10 行为硬上限：既进句柄文档，也直接发成 B 站动态，所以必须短。
// 主角视频链接由调用方单独加一行，正文里不要再重复写链接。
func generateDiaryForVideos(videos []*videoRecord) (string, string, error) {
	if len(videos) == 0 {
		return "", "", fmt.Errorf("没有可写进日记的视频")
	}
	var material strings.Builder
	for i, video := range videos {
		if video == nil {
			continue
		}
		fmt.Fprintf(&material, "视频 %d：《%s》｜UP主：%s\n观看感受：%s\n",
			i+1, video.Info.Title, video.Info.Owner, truncate(video.Summary, 800))
		for j, c := range video.Comments {
			if j >= 3 {
				break
			}
			fmt.Fprintf(&material, "评论：%s：%s\n", c.User, c.Content)
		}
		material.WriteString("\n")
	}
	system, err := lifeSystem("把这一轮看视频、看评论的心情写成 2–4 行简述，不写标题。" +
		"每行一句、具体、口语化，不要套话；10 行是硬上限，不要用满，能短就短。" +
		"不要链接、不要 @、不要列表符号、不要 markdown。")
	if err != nil {
		return "", "", err
	}
	text, err := chatText(system, material.String())
	if err != nil {
		return "", "", err
	}
	text = strings.TrimSpace(strings.ReplaceAll(text, "@", ""))
	if text == "" {
		return "", "", fmt.Errorf("日记为空")
	}
	title := time.Now().Format("2006-01-02 15:04") + " 刷B站日记"
	return title, text, nil
}

// wantMoreVideos 在两段视频之间做一次「还要不要继续刷」的判断。
//
// 只有看完至少 1 个视频时才会调用（min=1 由调用方保证）；本轮目标是 target 个。
// 模型只回答「继续」或「停止」，解析失败一律按继续处理——不能因为一次抖动就提前收工。
func wantMoreVideos(watched []*videoRecord, target int) bool {
	if len(watched) == 0 {
		return true
	}
	var material strings.Builder
	fmt.Fprintf(&material, "这轮最多刷 %d 个视频，已经看了 %d 个：\n", target, len(watched))
	for i, video := range watched {
		if video == nil {
			continue
		}
		fmt.Fprintf(&material, "第 %d 个：《%s》｜UP主：%s\n观看感受：%s\n",
			i+1, video.Info.Title, video.Info.Owner, truncate(video.Summary, 300))
	}
	system, err := lifeSystem("你正在刷B站，刚刚看完上面这些视频。按自己的心情决定还要不要接着刷。只回答「继续」或「停止」两个词，不要解释。")
	if err != nil {
		return true
	}
	text, err := chatText(system, material.String())
	if err != nil {
		return true
	}
	return !strings.Contains(strings.TrimSpace(text), "停止")
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
		logger.Info("BilibiliLife", "动态刚发布，评论索引尚未就绪或暂时没有评论")
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
			logger.Warn("BilibiliLife", "动态回复文案生成失败: %v", err)
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
			logger.Warn("BilibiliLife", "回复 %s 失败: %v", r.Member.Uname, err)
			continue
		}
		var resp struct {
			Code int    `json:"code"`
			Msg  string `json:"message"`
		}
		if err := json.Unmarshal([]byte(text), &resp); err != nil || resp.Code != 0 {
			logger.Warn("BilibiliLife", "回复 %s 被拒: %s", r.Member.Uname, truncate(text, 160))
			continue
		}
		sent++
		logger.Info("BilibiliLife", "回复动态评论（%d/%d）→ %s：%s", sent, max, r.Member.Uname, reply)
		time.Sleep(2 * time.Second)
	}
	return nil
}

func generateReply(user, message string) (string, error) {
	system, err := lifeSystem("正在回复自己 B 站动态下的评论，只输出一句自然、友好的回复，1–60 字；不争论、不引战、不放链接/@。")
	if err != nil {
		return "", err
	}
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
