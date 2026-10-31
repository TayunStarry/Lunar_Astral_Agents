package CrystalPersonality

import (
	"bytes"
	"fmt"
	"io"
	"net/http"

	"LunarSubsystem/LunarHarness/engine/console/logger"
)

// ==== 进程内端点调用适配器 ====
//
// LTPX 协调合并为单进程后，工具链状态、工具调用与事件推送不再经 HTTP 通道往返，
// 但注册表中已有的一批处理器已经承载了完整的业务逻辑。为使「协议实现」只有一份，
// 这里提供一个纯内存的请求适配器：构造请求对象 + 捕获响应，直接调用处理器函数，
// **无 socket、无端口、无网络、无序列化落地**，处理器保持唯一真相源。

// inProcessResponse 进程内响应记录器（实现 http.ResponseWriter）
type inProcessResponse struct {
	header http.Header
	code   int
	body   bytes.Buffer
}

// Header 返回响应头集合
func (r *inProcessResponse) Header() http.Header {
	return r.header
}

// Write 写入响应体（隐式 200）
func (r *inProcessResponse) Write(b []byte) (int, error) {
	if r.code == 0 {
		r.code = http.StatusOK
	}
	return r.body.Write(b)
}

// WriteHeader 记录状态码
func (r *inProcessResponse) WriteHeader(code int) {
	r.code = code
}

// invokeEndpoint 进程内直调端点处理器，返回状态码与响应体。
// body 为 nil 表示无请求体；query 以 "?k=v" 形式拼接在 path 之后。
//
// 进程内直调位于调用方 goroutine（不再是 net/http 的独立请求协程），因此这里自行
// 兜住处理器 panic：转成 500 响应体返回，避免单个处理器异常带崩整个宿主进程。
func invokeEndpoint(handler http.HandlerFunc, method, path string, body []byte) (code int, respBody []byte) {
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequest(method, "http://localhost"+path, reader)
	if err != nil {
		return 0, nil
	}
	req.Header.Set("Content-Type", "application/json")

	rec := &inProcessResponse{header: http.Header{}}
	defer func() {
		if r := recover(); r != nil {
			logger.Error("CrystalAstral", "进程内端点 %s %s 处理 panic: %v", method, path, r)
			code = http.StatusInternalServerError
			respBody = []byte(fmt.Sprintf(`{"error":"endpoint panic: %v"}`, r))
		}
	}()
	handler(rec, req)

	code = rec.code
	if code == 0 {
		code = http.StatusOK
	}
	return code, rec.body.Bytes()
}
