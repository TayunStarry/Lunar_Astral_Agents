package creation

// flow.go 文件操作流程的执行与记录核心：
//   - 读取节点见 flow_read.go；
//   - 写入节点见 flow_write.go；
//   - 本文件负责「快照 → 执行 → 审查 → 回滚 → 记录」的统一包装与流程上下文汇入。

import (
	"fmt"
	"strings"

	"LunarSubsystem/LunarHarness/agents/ArchiveOps"
	"LunarSubsystem/LunarHarness/agents/FileEditor"
	"LunarSubsystem/LunarHarness/engine/fileflow"
)

// mutate 执行一次写入类操作：路由 → 快照 → 执行 → 采集内容增量 → 审查 → 回滚 → 落库。
func (s *session) mutate(op, tool, target string, snapshotPaths []string, defaults []fileflow.Expect, exec func() mutationResult) toolOutcome {
	decision, err := routeTarget(target, op)
	if err != nil {
		return failure(op, err)
	}
	if decision.Kind == routeArchiveReader {
		return failure(op, fmt.Errorf("目标是压缩包 / GGUF，只能读取，请用 archive_read"))
	}
	if op != fileflow.OpArchiveWrite && decision.Kind == routeArchiveOps {
		return failure(op, fmt.Errorf("压缩包写入请用 archive_write（create / delete / add / remove）"))
	}
	if op == fileflow.OpArchiveWrite && decision.Kind != routeArchiveOps {
		return failure(op, fmt.Errorf("archive_write 只能用于 .zip / .ltpx 压缩包: %s", target))
	}

	flow := s.ensureFlow()
	seq := len(flow.Records) + 1
	actualPaths, remaps, pathErr := actualSnapshotPaths(target, decision, snapshotPaths)
	if pathErr != nil {
		return failure(op, fmt.Errorf("解析操作目标失败: %w", pathErr))
	}
	snapshot, snapErr := fileflow.Capture(flow.FlowID, seq, op, target, actualPaths)
	if snapErr != nil {
		s.recordMutation(fileflow.Record{
			Op:     op,
			Target: target,
			Route:  string(decision.Kind),
			Error:  "创建操作前快照失败：" + snapErr.Error(),
		})
		return failure(op, fmt.Errorf("创建操作前快照失败，已取消本次写入: %w", snapErr))
	}

	result := exec()
	if !result.OK {
		message := result.Summary
		if message == "" {
			message = "操作未成功"
		}
		s.recordMutation(fileflow.Record{
			Op:      op,
			Target:  target,
			Route:   string(decision.Kind),
			Summary: message,
			Error:   message,
			Handles: result.Handles,
		})
		return mutationFailure(op, result)
	}
	// 写入成功会让路径树变化：立即失效快照，下一轮规划不会拿到旧树。
	invalidatePathSnapshot()

	step := 0
	if s.stepIndex >= 0 {
		step = s.stepIndex + 1
	}
	built, buildErr := fileflow.BuildOp(fileflow.OpInput{
		FlowID:   flow.FlowID,
		Seq:      seq,
		Step:     step,
		Mode:     s.mode,
		CallID:   s.callID,
		Op:       op,
		Target:   target,
		PathRel:  decision.Rel,
		PathAbs:  decision.Abs,
		Agent:    string(decision.Kind),
		Tool:     tool,
		Detail:   result.Detail,
		Snapshot: snapshot,
	})

	affected := affectedPaths(snapshot)
	review := fileflow.EvaluateAndRollback(snapshot,
		remapExpectsBy(s.stepExpects(), remaps),
		remapExpectsBy(defaults, remaps),
		affected)
	if buildErr != nil {
		s.addError(fmt.Errorf("内容增量采集失败：%w", buildErr))
		flow.AddError("内容增量采集失败：" + buildErr.Error())
	} else if commitErr := built.Commit(review); commitErr != nil {
		s.addError(fmt.Errorf("内容增量落库失败：%w", commitErr))
		flow.AddError("内容增量落库失败：" + commitErr.Error())
	}
	record := fileflow.Record{
		Op:      op,
		Target:  target,
		Route:   string(decision.Kind),
		Summary: result.Summary,
		Handles: result.Handles,
		Review:  &review,
	}
	s.recordMutation(record)
	if !review.OK {
		contextText := review.Context
		if contextText == "" {
			contextText = review.Summary()
		}
		s.addError(fmt.Errorf("%s", contextText))
		s.flow.AddError(contextText)
		if review.RollbackError != "" {
			rollbackText := fmt.Sprintf("回滚失败（%s）：%s", target, review.RollbackError)
			s.addError(fmt.Errorf("%s", rollbackText))
			s.flow.AddError(rollbackText)
		}
	}
	return mutationOutcome(op, target, decision, result, &review)
}

// mutationOutcome 汇总写操作结果与审查结论。
func mutationOutcome(op, target string, decision routeDecision, result mutationResult, review *fileflow.Review) toolOutcome {
	payload := map[string]any{
		"ok":      result.OK && review.OK,
		"op":      op,
		"path":    result.Path,
		"route":   string(decision.Kind),
		"summary": result.Summary,
		"handles": ensureStrings(result.Handles),
		"routing": decision.Reason,
		"review":  review,
	}
	if result.Detail != nil {
		payload["detail"] = result.Detail
	}
	summary := result.Summary
	if !review.OK {
		summary = review.Summary() + "｜" + result.Summary
	}
	return toolOutcome{JSON: jsonText(payload), Summary: previewRunes(summary, 512)}
}

// mutationFailure 执行阶段失败（未进入审查）。
func mutationFailure(op string, result mutationResult) toolOutcome {
	message := result.Summary
	if message == "" {
		message = "操作未成功"
	}
	payload := map[string]any{
		"ok":      false,
		"op":      op,
		"path":    result.Path,
		"summary": message,
		"handles": ensureStrings(result.Handles),
	}
	return toolOutcome{JSON: jsonText(payload), Summary: previewRunes(message, 512)}
}

// ==== 预期与快照辅助 ====

// stepExpects 当前步骤声明的规划预期。
func (s *session) stepExpects() []fileflow.Expect {
	if s.current == nil {
		return nil
	}
	return s.current.Expect
}

// expectExists 构造存在性预期。
func expectExists(path string, exists bool) fileflow.Expect {
	value := exists
	return fileflow.Expect{Path: path, Exists: &value}
}

// remapExpects 把指向调用方目标路径的预期改写到真实产物路径（压缩包创建专用）。
func remapExpects(expects []fileflow.Expect, fromPath, toRel string) []fileflow.Expect {
	if fromPath == "" || toRel == "" {
		return expects
	}
	_, fromRel, err := fileflow.Resolve(fromPath)
	if err != nil {
		return expects
	}
	out := make([]fileflow.Expect, 0, len(expects))
	for _, expect := range expects {
		_, rel, resolveErr := fileflow.Resolve(expect.Path)
		if resolveErr == nil && rel == fromRel {
			expect.Path = toRel
		}
		out = append(out, expect)
	}
	return out
}

// actualSnapshotPaths 把调用方给出的快照路径解析为相对 LocalDir 的规范路径，并建立「严格路径 → 实际路径」映射。
//
// 文件编辑在严格路径不存在时会做同名 / 后缀模糊定位；快照与审查必须对准同一个实际实体，
// 否则会出现「改了 A、快照了 B、回滚落空」的隐蔽错误。
//
// 这里只回传 rel：绝对路径再次交给 Capture 解析时，若 LocalDir 是相对路径会被补上一层
// documents/（历史上快照因此记成 documents/local_data/documents/…，回滚必然落空）。
func actualSnapshotPaths(target string, decision routeDecision, paths []string) ([]string, map[string]string, error) {
	actual := make([]string, 0, len(paths))
	remaps := make(map[string]string, len(paths))
	for _, path := range paths {
		if strings.TrimSpace(path) == "" {
			continue
		}
		if strings.TrimSpace(path) == strings.TrimSpace(target) {
			if _, rel, err := fileflow.Resolve(path); err == nil {
				remaps[rel] = decision.Rel
			}
			actual = append(actual, decision.Rel)
			continue
		}
		var rel string
		var err error
		if hasStorageRootPrefix(path) {
			// 已写明存储根（压缩包产物 unknown/…）：按严格路径快照，不做模糊匹配
			_, rel, err = fileflow.Resolve(path)
		} else {
			_, rel, err = resolveActualPath(path)
		}
		if err != nil {
			return nil, nil, err
		}
		if _, strictRel, err := fileflow.Resolve(path); err == nil {
			remaps[strictRel] = rel
		}
		actual = append(actual, rel)
	}
	return actual, remaps, nil
}

// hasStorageRootPrefix 判断路径是否已显式写明存储根。
func hasStorageRootPrefix(path string) bool {
	normalized := strings.ReplaceAll(strings.TrimSpace(path), "\\", "/")
	for _, root := range []string{"documents/", "unknown/", "audios/", "multimedia/"} {
		if strings.HasPrefix(normalized, root) {
			return true
		}
	}
	return false
}

// remapExpectsBy 按实际路径映射改写预期路径。
func remapExpectsBy(expects []fileflow.Expect, remaps map[string]string) []fileflow.Expect {
	if len(remaps) == 0 || len(expects) == 0 {
		return expects
	}
	out := make([]fileflow.Expect, 0, len(expects))
	for _, expect := range expects {
		if _, rel, err := fileflow.Resolve(expect.Path); err == nil {
			if to, ok := remaps[rel]; ok {
				expect.Path = to
			}
		}
		out = append(out, expect)
	}
	return out
}

// affectedPaths 快照路径集合（相对本地目录）。
func affectedPaths(snapshot fileflow.Snapshot) map[string]bool {
	affected := make(map[string]bool, len(snapshot.Entries))
	for _, entry := range snapshot.Entries {
		affected[entry.Rel] = true
	}
	return affected
}

// fromFileEditor 转换文件编辑结果。
func fromFileEditor(result FileEditor.OpResult) mutationResult {
	return mutationResult{
		OK:      result.OK,
		Path:    result.Path,
		Summary: result.Summary,
		Handles: result.Handles,
		Detail:  result.Detail,
	}
}

// fromArchive 转换归档操作结果。
func fromArchive(result ArchiveOps.Result) mutationResult {
	return mutationResult{
		OK:      result.OK,
		Path:    result.Path,
		Summary: result.Summary,
		Handles: result.Handles,
		Detail:  result.Detail,
	}
}

// ensureStrings 保证字符串切片非 nil。
func ensureStrings(items []string) []string {
	if items == nil {
		return []string{}
	}
	return items
}
