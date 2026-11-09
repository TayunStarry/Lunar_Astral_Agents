package server

// library_ws.go 资源库前端的 WebSocket 命令通道。
//
// 资源库前端以 /ws?channel=library 接入，上行只有一种结构：
//   {"type":"command","id":"<客户端生成>","action":"storyboard.submit","data":{...}}
// 服务端在发起命令的同一条连接上回执：
//   {"type":"library_reply","data":{"id":"...","ok":true,"result":{...}}}
// 演绎任务的增量推送仍走原有 storyboard_task / creation_task 频道广播。
//
// 这样「命令 -> 回执」与「状态推送」共用一条连接：前端拿得到失败原因，
// 不再需要 HTTP 轮询兜底，也就没有推送与轮询两套状态互相覆盖的问题。

import (
	"encoding/json"
	"fmt"
	"time"

	"LunarSubsystem/LunarHarness/engine/websocket"
)

// HandleLibraryWSMessage 处理资源库频道的上行命令，并用同一条连接回执。
func HandleLibraryWSMessage(msg websocket.ClientMessage) {
	if msg.Channel != LibraryChannel {
		return
	}
	var env LibraryWSEnvelope
	if err := json.Unmarshal(msg.Data, &env); err != nil {
		return
	}
	if env.Type != "command" || env.Action == "" {
		return
	}
	reply := LibraryWSReply{ID: env.ID}
	result, err := dispatchLibraryCommand(env.Action, env.Data)
	if err != nil {
		reply.Error = err.Error()
	} else {
		reply.OK = true
		reply.Result = result
	}
	raw, err := json.Marshal(websocket.WSResponse{Type: "library_reply", Data: reply})
	if err != nil {
		return
	}
	msg.Send(raw)
}

// dispatchLibraryCommand 把命令分派给装配层注入的任务函数（提交 / 查询 / 控制）。
func dispatchLibraryCommand(action string, raw json.RawMessage) (any, error) {
	switch action {
	case "ping":
		return map[string]int64{"time": time.Now().UnixMilli()}, nil
	case "storyboard.submit":
		return wsStoryboardSubmit(raw)
	case "storyboard.status":
		return wsStoryboardStatus(raw)
	case "storyboard.list":
		return wsStoryboardList(raw)
	case "storyboard.confirm", "storyboard.pause", "storyboard.resume", "storyboard.cancel":
		return wsStoryboardAction(action, raw)
	case "creation.submit":
		return wsCreationSubmit(raw)
	case "creation.status":
		return wsCreationStatus(raw)
	case "creation.list":
		return wsCreationList(raw)
	default:
		return nil, fmt.Errorf("未知命令：%s", action)
	}
}

func wsStoryboardSubmit(raw json.RawMessage) (any, error) {
	if LibraryStoryboardSubmitFunc == nil {
		return nil, fmt.Errorf("演绎链尚未就绪")
	}
	var req LibraryStoryboardRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		return nil, fmt.Errorf("请求体解析失败: %v", err)
	}
	task, err := LibraryStoryboardSubmitFunc(req)
	if err != nil {
		return nil, err
	}
	return LibraryStoryboardResponse{OK: true, Task: &task}, nil
}

func wsStoryboardStatus(raw json.RawMessage) (any, error) {
	if LibraryStoryboardStatusFunc == nil {
		return nil, fmt.Errorf("演绎链尚未就绪")
	}
	query := decodeTaskQuery(raw)
	if query.TaskID == "" {
		return nil, fmt.Errorf("缺少 task_id")
	}
	task, ok := LibraryStoryboardStatusFunc(query.TaskID)
	if !ok {
		return nil, fmt.Errorf("任务不存在或已过期: %s", query.TaskID)
	}
	return LibraryStoryboardResponse{OK: true, Task: &task}, nil
}

func wsStoryboardList(raw json.RawMessage) (any, error) {
	if LibraryStoryboardListFunc == nil {
		return nil, fmt.Errorf("演绎链尚未就绪")
	}
	query := decodeTaskQuery(raw)
	return LibraryStoryboardListResponse{OK: true, Tasks: LibraryStoryboardListFunc(query.Limit)}, nil
}

func wsStoryboardAction(action string, raw json.RawMessage) (any, error) {
	var req LibraryStoryboardActionRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		return nil, fmt.Errorf("请求体解析失败: %v", err)
	}
	if req.TaskID == "" {
		return nil, fmt.Errorf("缺少 task_id")
	}
	var (
		task LibraryStoryboardTask
		err  error
	)
	switch action {
	case "storyboard.confirm":
		if LibraryStoryboardConfirmFunc == nil {
			return nil, fmt.Errorf("演绎链尚未就绪")
		}
		task, err = LibraryStoryboardConfirmFunc(req.TaskID, req.Title, req.Shots)
	case "storyboard.pause":
		if LibraryStoryboardPauseFunc == nil {
			return nil, fmt.Errorf("演绎链尚未就绪")
		}
		task, err = LibraryStoryboardPauseFunc(req.TaskID)
	case "storyboard.resume":
		if LibraryStoryboardResumeFunc == nil {
			return nil, fmt.Errorf("演绎链尚未就绪")
		}
		task, err = LibraryStoryboardResumeFunc(req.TaskID)
	case "storyboard.cancel":
		if LibraryStoryboardCancelFunc == nil {
			return nil, fmt.Errorf("演绎链尚未就绪")
		}
		task, err = LibraryStoryboardCancelFunc(req.TaskID, req.Reason)
	}
	if err != nil {
		return nil, err
	}
	return LibraryStoryboardResponse{OK: true, Task: &task}, nil
}

func wsCreationSubmit(raw json.RawMessage) (any, error) {
	if LibraryCreationSubmitFunc == nil {
		return nil, fmt.Errorf("创作链尚未就绪")
	}
	var req LibraryCreationRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		return nil, fmt.Errorf("请求体解析失败: %v", err)
	}
	task, err := LibraryCreationSubmitFunc(req)
	if err != nil {
		return nil, err
	}
	return LibraryCreationResponse{OK: true, Task: &task}, nil
}

func wsCreationStatus(raw json.RawMessage) (any, error) {
	if LibraryCreationStatusFunc == nil {
		return nil, fmt.Errorf("创作链尚未就绪")
	}
	query := decodeTaskQuery(raw)
	if query.TaskID == "" {
		return nil, fmt.Errorf("缺少 task_id")
	}
	task, ok := LibraryCreationStatusFunc(query.TaskID)
	if !ok {
		return nil, fmt.Errorf("任务不存在或已过期: %s", query.TaskID)
	}
	return LibraryCreationResponse{OK: true, Task: &task}, nil
}

func wsCreationList(raw json.RawMessage) (any, error) {
	if LibraryCreationListFunc == nil {
		return nil, fmt.Errorf("创作链尚未就绪")
	}
	query := decodeTaskQuery(raw)
	return LibraryCreationListResponse{OK: true, Tasks: LibraryCreationListFunc(query.Limit)}, nil
}

// decodeTaskQuery 解析通用查询入参；空 body 视为全部缺省。
func decodeTaskQuery(raw json.RawMessage) LibraryTaskQuery {
	var query LibraryTaskQuery
	if len(raw) == 0 {
		return query
	}
	_ = json.Unmarshal(raw, &query)
	return query
}
