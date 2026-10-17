package lunar

import (
	"encoding/json"
	"io"
	"net/http"

	"CorePersonality/engine/lunar_loop"
)

// MessageBatchHandler 消息批量写入（POST /write/message）。
// 统一时序队列的唯一写入口：content 为文本/多模态内容数组，
// 视频/音频 URL 以 MediaUrlContent 内容项直接放入 messages。
func MessageBatchHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "MessageBatch请求[ERROR] -> 不允许的请求方法", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "MessageBatch请求[ERROR] -> 读取请求体失败", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	var req MessageBatchRequest
	if err = json.Unmarshal(body, &req); err != nil {
		http.Error(w, "MessageBatch请求[ERROR] -> 解析请求体失败", http.StatusBadRequest)
		return
	}

	lunarloop.EnqueueMessage(req.Messages...)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(BatchResponse{
		Success: true,
		Length:  lunarloop.UnreadCount(),
	})
}
