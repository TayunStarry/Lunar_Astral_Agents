package server

// vision.go 视觉读取端点：思维链中静态图的统一消费入口
//
// GET /file/miniature/<相对 LocalDir 路径>?tag=<句柄>&size=<边长>
//
// 行为：
//   - 仅允许读取 multimedia/ 下的图片类文件（其他一律拒绝）
//   - 返回的是**派生副本**而非原数据：长边不超过 size（默认取 agent.vision_max_side，1080），
//     右上角叠印句柄标签（如 @I.003），按「内容哈希 + 边长」盘上缓存，重复读取秒回
//   - 动图（GIF / APNG / 动态 WebP）原样直通：缩放会坍缩成单帧，破坏下游动图理解链路
//   - 原图 blob 永远不被修改：绘画者参考图等直读原图的场景不受叠印影响
//
// 变换与缓存逻辑与 base64 回退绑定（visionRead）共用同一实现（lunar.VisionReadBlob），
// 本端点只做 HTTP 形态的包装。

import (
	"net/http"
	"strconv"
	"strings"

	"LunarSubsystem/LunarHarness/engine/console/logger"
	"LunarSubsystem/LunarHarness/lunar"
)

// VisionReadHandler 视觉读取端点处理函数
func VisionReadHandler(w http.ResponseWriter, r *http.Request) {
	rel := strings.TrimPrefix(r.URL.Path, "/file/miniature/")
	if rel == "" {
		http.Error(w, "未指定文件路径", http.StatusBadRequest)
		return
	}
	// 叠印句柄：调用方经 tag 参数传入（入队赋签时已知），缺省时由管线按 blob 路径反查资源目录
	tag := r.URL.Query().Get("tag")
	// 副本边长：缺省取配置 agent.vision_max_side
	side := 0
	if raw := r.URL.Query().Get("size"); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil {
			side = parsed
		}
	}
	data, mime, err := lunar.VisionReadBlob(rel, tag, side)
	if err != nil {
		logger.SubInfo("CoreServer", "VisionRead", "读取失败 %s: %v", rel, err)
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	// 副本按内容寻址、不可变：允许客户端与推理服务长期缓存
	w.Header().Set("Content-Type", mime)
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}
