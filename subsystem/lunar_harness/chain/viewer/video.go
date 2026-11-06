package viewer

// 生活链 · B 站视频接口：视频信息、标签与播放地址。

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"LunarSubsystem/LunarHarness/engine/viewerstore"
)

const (
	biliUA      = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0 Safari/537.36"
	biliReferer = "https://www.bilibili.com/"
)

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
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, truncate(string(body), 512))
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
				Mid  int64  `json:"mid"`
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
		OwnerMid:    resp.Data.Owner.Mid,
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
	return viewerstore.ExtractBVID(href)
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
