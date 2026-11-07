package server

// media_platforms.go 共享媒体平台域名表的对外暴露。
//
// 前端把输入框里的 URL 抽成 webpage 信封时需要判断是否为视频平台页面，
// 判定依据必须与后端校验完全一致：这里直接返回 engine/mediaobject.VideoPlatformHosts，
// 不在前端另写一份清单。

import (
	"net/http"

	"LunarSubsystem/LunarHarness/engine/mediaobject"
)

// mediaPlatformHostsHandler 返回共享视频平台域名表
func mediaPlatformHostsHandler(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, MediaPlatformHostsResponse{VideoPlatformHosts: mediaobject.VideoPlatformHosts})
}
