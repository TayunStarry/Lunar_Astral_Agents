package creation

// flow_record.go 流程上下文汇入：把读取 / 写入 / 失败结果统一记录到当前流程。
// 记录包含操作种类、目标、路由、摘要、句柄、审查结果与错误；同一记录也用于日志与追溯。

import (
	"fmt"

	"LunarSubsystem/LunarHarness/agents/FileEditor"
	"LunarSubsystem/LunarHarness/engine/fileflow"
)

// ensureFlow 初始化/返回当前流程上下文。
func (s *session) ensureFlow() *fileflow.Context {
	if s.flow == nil {
		if s.flowID == "" {
			s.flowID = fileflow.NewFlowID()
		}
		s.flow = fileflow.NewContext(s.mode, s.flowID)
	}
	return s.flow
}

// recordRead 记录一次读取操作。
func (s *session) recordRead(op, target string, decision routeDecision, summary string, handles []string) {
	s.recordMutation(fileflow.Record{
		Op:      op,
		Target:  target,
		Route:   string(decision.Kind),
		Summary: summary,
		Handles: handles,
	})
}

// recordFileResult 记录文件编辑的读取结果：失败时按错误汇入流程上下文。
func (s *session) recordFileResult(op, target string, decision routeDecision, result FileEditor.OpResult) {
	if result.OK {
		s.recordRead(op, target, decision, result.Summary, result.Handles)
		return
	}
	s.recordFailure(op, target, decision, fmt.Errorf("%s", result.Summary))
}

// recordFailure 记录一次读取失败。
func (s *session) recordFailure(op, target string, decision routeDecision, err error) {
	s.recordMutation(fileflow.Record{
		Op:     op,
		Target: target,
		Route:  string(decision.Kind),
		Error:  err.Error(),
	})
}

// recordMutation 把一条记录汇入流程上下文。
func (s *session) recordMutation(record fileflow.Record) {
	flow := s.ensureFlow()
	record.Seq = len(flow.Records) + 1
	record.Step = s.stepIndex + 1
	record.At = nowUnix()
	flow.Add(record)
	if record.Error != "" {
		flow.AddError(record.Error)
		s.addError(fmt.Errorf("%s", record.Error))
	}
}

// readOutcome 把路由读取结果包装成统一工具回填。
func (s *session) readOutcome(op, target string, decision routeDecision, text string) toolOutcome {
	summary := fmt.Sprintf("%s 读取 %s（%s）", routeLabel(decision.Kind), target, decision.Reason)
	payload := map[string]any{
		"ok":      true,
		"op":      op,
		"path":    target,
		"route":   string(decision.Kind),
		"summary": summary,
		"handles": []string{},
		"routing": decision.Reason,
		"result":  text,
	}
	s.recordRead(op, target, decision, summary, nil)
	return toolOutcome{JSON: jsonText(payload), Summary: previewRunes(summary, 512)}
}

// routeLabel 路由中文名。
func routeLabel(kind routeKind) string {
	switch kind {
	case routeArchiveReader:
		return "归档解析"
	case routeArchiveOps:
		return "归档操作"
	case routeDocumentReader:
		return "文献解析"
	default:
		return "文件编辑"
	}
}
