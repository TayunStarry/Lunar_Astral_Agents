package BilibiliViewer

// dynamic_target.go 解析动态真正的评论区。
//
// dynamic_svr 返回的 dynamic_id 只是动态 ID，不等于评论 oid：
// 纯文字动态常被归为图文动态（comment_type=11），评论接口必须用详情里的
// basic.comment_type + basic.comment_id_str，否则一律 code=-404。

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"
)

// resolveDynamicCommentTarget 读取动态详情，取回真正的评论类型与 oid。
func (w *webSession) resolveDynamicCommentTarget(dynID string) (dynamicCommentTarget, error) {
	dynID = strings.TrimSpace(dynID)
	if dynID == "" {
		return dynamicCommentTarget{}, fmt.Errorf("动态 ID 为空")
	}
	target := fmt.Sprintf("https://api.bilibili.com/x/polymer/web-dynamic/v1/detail?id=%s", url.QueryEscape(dynID))
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			w.wait(2 * time.Second)
		}
		raw, err := w.pageFetchText("GET", target, "", "", 20*time.Second)
		if err != nil {
			lastErr = err
			continue
		}
		resolved, err := parseDynamicCommentTarget(raw, dynID)
		if err == nil {
			return resolved, nil
		}
		lastErr = err
	}
	return dynamicCommentTarget{}, lastErr
}

// parseDynamicCommentTarget 从详情响应里取 comment_type / comment_id_str。
//
// 详情缺字段时回退成「纯文字动态」：type=17 + 动态 ID。
func parseDynamicCommentTarget(raw, fallbackID string) (dynamicCommentTarget, error) {
	var resp struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Data    struct {
			Item struct {
				Basic struct {
					CommentIDStr string `json:"comment_id_str"`
					CommentType  int    `json:"comment_type"`
					RidStr       string `json:"rid_str"`
				} `json:"basic"`
			} `json:"item"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(raw), &resp); err != nil {
		return dynamicCommentTarget{}, err
	}
	if resp.Code != 0 {
		return dynamicCommentTarget{}, fmt.Errorf("动态详情 code=%d %s", resp.Code, resp.Message)
	}

	basic := resp.Data.Item.Basic
	oid := strings.TrimSpace(basic.CommentIDStr)
	if oid == "" {
		oid = strings.TrimSpace(basic.RidStr)
	}
	if oid != "" && basic.CommentType > 0 {
		return dynamicCommentTarget{Type: basic.CommentType, Oid: oid}, nil
	}
	fallbackID = strings.TrimSpace(fallbackID)
	if fallbackID == "" {
		return dynamicCommentTarget{}, fmt.Errorf("动态详情缺少评论区字段")
	}
	return dynamicCommentTarget{Type: 17, Oid: fallbackID}, nil
}
