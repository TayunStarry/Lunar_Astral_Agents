package handlers

import (
	"Lunar-Astral-Agents/library" // 导入执行模块，用于处理核心逻辑
	"encoding/json"               // JSON编码/解码包，用于处理JSON数据
	"io"                          // 输入/输出包，用于处理IO操作
	"net/http"                    // HTTP协议包，用于处理HTTP请求/响应
)

// 定义请求和响应的数据结构
type KnowledgeQueryRequest struct {
	FilePath    string    `json:"filePath"`
	QueryVector []float64 `json:"queryVector"`
	TopK        int       `json:"topK"`
}

type KnowledgeWriteRequest struct {
	FilePath string                 `json:"filePath"`
	Message  library.HistoryMessage `json:"message"`
}

type KnowledgeDeleteRequest struct {
	FilePath string `json:"filePath"`
	UUID     string `json:"uuid"`
}

// KnowledgeQueryHandler 处理知识库查询请求
func KnowledgeQueryHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "KnowledgeQuery请求[ERROR] -> 不允许的请求方法", http.StatusMethodNotAllowed)
		return
	}

	// 解析请求体
	var req KnowledgeQueryRequest
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "KnowledgeQuery请求[ERROR] -> 读取请求体失败", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	err = json.Unmarshal(body, &req)
	if err != nil {
		http.Error(w, "KnowledgeQuery请求[ERROR] -> 解析请求体失败", http.StatusBadRequest)
		return
	}

	// 调用 execute 模块查询知识库
	results, err := library.QueryKnowledge(req.FilePath, req.QueryVector, req.TopK)
	if err != nil {
		http.Error(w, "KnowledgeQuery请求[ERROR] -> "+err.Error(), http.StatusInternalServerError)
		return
	}

	// 返回结果
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(results)
}

// KnowledgeWriteHandler 处理知识库写入请求 - 只将消息缓存到内存中
func KnowledgeWriteHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "KnowledgeWrite请求[ERROR] -> 不允许的请求方法", http.StatusMethodNotAllowed)
		return
	}

	// 解析请求体
	var req KnowledgeWriteRequest
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "KnowledgeWrite请求[ERROR] -> 读取请求体失败", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	if err = json.Unmarshal(body, &req); err != nil {
		http.Error(w, "KnowledgeWrite请求[ERROR] -> 解析请求体失败", http.StatusBadRequest)
		return
	}

	// 调用 execute 模块写入知识库
	hasDuplicate, err := library.WriteKnowledge(req.FilePath, req.Message)
	if err != nil {
		http.Error(w, "KnowledgeWrite请求[ERROR] -> "+err.Error(), http.StatusInternalServerError)
		return
	}

	if hasDuplicate {
		// 内容已在缓存中，不重复添加
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"message": "内容已存在于缓存中，不重复添加"})
	} else {
		// 返回成功响应
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"message": "消息已缓存，等待写入文件"})
	}
}

// KnowledgeFlushHandler 处理知识库刷新请求 - 接收哨兵信号，将缓存的消息写入文件并执行删除操作
func KnowledgeFlushHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "KnowledgeFlush请求[ERROR] -> 不允许的请求方法", http.StatusMethodNotAllowed)
		return
	}

	// 解析请求体
	type KnowledgeFlushRequest struct {
		FilePath string `json:"filePath"`
	}

	var req KnowledgeFlushRequest
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "KnowledgeFlush请求[ERROR] -> 读取请求体失败", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	if err = json.Unmarshal(body, &req); err != nil {
		http.Error(w, "KnowledgeFlush请求[ERROR] -> 解析请求体失败", http.StatusBadRequest)
		return
	}

	// 调用 execute 模块刷新知识库
	stats, err := library.FlushKnowledge(req.FilePath)
	if err != nil {
		http.Error(w, "KnowledgeFlush请求[ERROR] -> "+err.Error(), http.StatusInternalServerError)
		return
	}

	// 返回成功响应
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(stats)
}

// KnowledgeListHandler 处理知识库列表请求 - 返回知识库下所有条目的SmallHistoryMessage数组
func KnowledgeListHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "KnowledgeList请求[ERROR] -> 不允许的请求方法", http.StatusMethodNotAllowed)
		return
	}

	// 解析请求体
	type KnowledgeListRequest struct {
		FilePath string `json:"filePath"`
	}

	var req KnowledgeListRequest
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "KnowledgeList请求[ERROR] -> 读取请求体失败", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	if err = json.Unmarshal(body, &req); err != nil {
		http.Error(w, "KnowledgeList请求[ERROR] -> 解析请求体失败", http.StatusBadRequest)
		return
	}

	// 调用 execute 模块列出知识库内容
	results, err := library.ListKnowledge(req.FilePath)
	if err != nil {
		http.Error(w, "KnowledgeList请求[ERROR] -> "+err.Error(), http.StatusInternalServerError)
		return
	}

	// 返回结果
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(results)
}

// KnowledgeDeleteHandler 处理知识库删除请求 - 只将删除请求缓存到内存中
func KnowledgeDeleteHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "KnowledgeDelete请求[ERROR] -> 不允许的请求方法", http.StatusMethodNotAllowed)
		return
	}

	// 解析请求体
	var req KnowledgeDeleteRequest
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "KnowledgeDelete请求[ERROR] -> 读取请求体失败", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	if err = json.Unmarshal(body, &req); err != nil {
		http.Error(w, "KnowledgeDelete请求[ERROR] -> 解析请求体失败", http.StatusBadRequest)
		return
	}

	// 调用 execute 模块删除知识库条目
	hasDuplicate, err := library.DeleteKnowledge(req.FilePath, req.UUID)
	if err != nil {
		http.Error(w, "KnowledgeDelete请求[ERROR] -> "+err.Error(), http.StatusInternalServerError)
		return
	}

	if hasDuplicate {
		// 已存在相同的删除请求，不重复添加
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"message": "删除请求已存在于缓存中，不重复添加"})
	} else {
		// 返回成功响应
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"message": "删除请求已缓存，等待执行"})
	}
}
