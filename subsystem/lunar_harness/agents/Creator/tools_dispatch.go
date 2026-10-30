package Creator

// tools_dispatch.go 内容调度：把文本或 /document 下的文件发送到指定群聊。

import (
	"fmt"
	"strconv"
	"strings"

	"LunarSubsystem/LunarHarness/agents/DialogueBridging"
	"LunarSubsystem/LunarHarness/agents/FileEditor"
)

func sendGroupText(args map[string]any) toolOutcome {
	groupID, groupName, err := resolveGroup(argString(args, "group"))
	if err != nil {
		return failure(toolSendGroupText, err)
	}
	text := argString(args, "text")
	if strings.TrimSpace(text) == "" {
		return failure(toolSendGroupText, fmt.Errorf("text 不能为空"))
	}
	if err := DialogueBridging.SendGroupPlainTextMessage(groupID, text); err != nil {
		return failure(toolSendGroupText, err)
	}
	return toolOutcome{
		JSON:    jsonText(map[string]any{"group": groupName, "group_id": groupID, "sent": true}),
		Summary: "已发送到群「" + groupName + "」",
	}
}

func sendGroupFile(args map[string]any) toolOutcome {
	groupID, groupName, err := resolveGroup(argString(args, "group"))
	if err != nil {
		return failure(toolSendGroupFile, err)
	}
	full, err := FileEditor.AbsPath(argString(args, "path"))
	if err != nil {
		return failure(toolSendGroupFile, err)
	}
	name := strings.TrimSpace(argString(args, "name"))
	if name == "" {
		name = filepathBase(full)
	}
	if err := DialogueBridging.SendGroupFile(groupID, full, name); err != nil {
		return failure(toolSendGroupFile, err)
	}
	return toolOutcome{
		JSON:    jsonText(map[string]any{"group": groupName, "group_id": groupID, "file": name, "sent": true}),
		Summary: "已发送文件到群「" + groupName + "」",
	}
}

// resolveGroup 把群号或群名解析为群 ID。
func resolveGroup(target string) (int64, string, error) {
	target = strings.TrimSpace(target)
	if target == "" {
		return 0, "", fmt.Errorf("group 不能为空")
	}
	if id, err := strconv.ParseInt(target, 10, 64); err == nil {
		return id, target, nil
	}
	groups, _, err := DialogueBridging.QQTargets()
	if err != nil {
		return 0, "", fmt.Errorf("获取群列表失败: %w", err)
	}
	for _, group := range groups {
		if group.GroupName == target {
			return group.GroupID, group.GroupName, nil
		}
	}
	for _, group := range groups {
		if strings.Contains(group.GroupName, target) {
			return group.GroupID, group.GroupName, nil
		}
	}
	names := make([]string, 0, len(groups))
	for _, group := range groups {
		names = append(names, group.GroupName)
	}
	return 0, "", fmt.Errorf("未找到群「%s」，可用群：%s", target, strings.Join(names, "、"))
}

// filepathBase 取路径最后一段。
func filepathBase(path string) string {
	path = strings.ReplaceAll(path, "\\", "/")
	if idx := strings.LastIndex(path, "/"); idx >= 0 {
		return path[idx+1:]
	}
	return path
}
