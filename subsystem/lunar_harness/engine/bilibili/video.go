package bilibili

// video.go 视频信息、标签与播放地址接口。

import (
	"fmt"
	"net/url"
	"strings"
)

// FetchVideo 读取视频基础信息与热度指标。
func FetchVideo(bvid string) (Video, error) {
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
	target := viewURL + url.QueryEscape(bvid)
	if err := getJSON(target, &resp); err != nil {
		return Video{}, err
	}
	if resp.Code != 0 {
		return Video{}, fmt.Errorf("view code=%d %s", resp.Code, resp.Msg)
	}
	return Video{
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

// FetchTags 读取视频的内容标签。
// 失败只返回错误、不阻断调用方选片：标签是加分项，不是必需信息。
func FetchTags(bvid string) ([]string, error) {
	var resp struct {
		Code int    `json:"code"`
		Msg  string `json:"message"`
		Data []struct {
			TagName string `json:"tag_name"`
		} `json:"data"`
	}
	target := tagsURL + url.QueryEscape(bvid)
	if err := getJSON(target, &resp); err != nil {
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

// PlayURL 读取可直接下载的播放地址。
func PlayURL(bvid string, cid int64, qn int) (string, error) {
	var resp struct {
		Code int    `json:"code"`
		Msg  string `json:"message"`
		Data struct {
			Durl []struct {
				URL string `json:"url"`
			} `json:"durl"`
		} `json:"data"`
	}
	target := fmt.Sprintf("%s?bvid=%s&cid=%d&qn=%d&fnval=1&platform=html5&high_quality=1",
		playURL, url.QueryEscape(bvid), cid, qn)
	if err := getJSON(target, &resp); err != nil {
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
