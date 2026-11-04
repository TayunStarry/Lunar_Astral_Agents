package thinking

import (
	"context"
	"fmt"
	"strings"
	"time"

	"LunarSubsystem/LunarHarness/agents/Musician"
	"LunarSubsystem/LunarHarness/agents/Painter"
	"LunarSubsystem/LunarHarness/agents/Screenshoter"
	"LunarSubsystem/LunarHarness/chain/creation"
	"LunarSubsystem/LunarHarness/engine/resourcehub"
	"LunarSubsystem/LunarHarness/protocol/agentdispatch"
)

// ==== 工具定义 ====

const (
	// techniqueToolName 技法创作聚合工具名
	techniqueToolName = "dispatch_technique"
	// techniquePainter 绘图子工具
	techniquePainter = "painter"
	// techniqueMusician 音乐子工具
	techniqueMusician = "musician"
	// techniqueScreenshoter 截图子工具
	techniqueScreenshoter = "screenshoter"
	// techniqueFileOperations 文件操作编排分支：统一承接文件读写、归档处理、文献读取与脚本运行
	techniqueFileOperations = "file_operations"
	// techniqueCreatorLegacy 旧创作分支（兼容保留，内部同样转文件操作编排）
	techniqueCreatorLegacy = "creator"
	// techniqueFileEditorLegacy 旧文件编辑分支（兼容保留，内部同样转文件操作编排）
	techniqueFileEditorLegacy = "file_editor"
	// fileOperationsTimeout 单次文件操作编排任务的超时上限
	fileOperationsTimeout = 20 * time.Minute
)

// ==== 智能体控制 ====

// handleDispatchTechnique 技法创作聚合工具：按 type 分派给对应子智能体。
// 输入严格使用统一原语 {type, instruction, handle, details}，输出统一为 {result, handle, error}。
func handleDispatchTechnique(args map[string]any) ToolResult {
	call, err := agentdispatch.Parse(args)
	if err != nil {
		return toolJSON(agentdispatch.Failure(err.Error(), nil))
	}
	kind := strings.ToLower(call.Type)
	if call.Details {
		if text, ok := techniqueScope(kind); ok {
			return toolJSON(agentdispatch.Success(text, nil))
		}
		return toolJSON(agentdispatch.Failure("type 只能取 painter / musician / screenshoter / file_operations", nil))
	}
	if call.Instruction == "" {
		return toolJSON(agentdispatch.Failure("instruction 不能为空，请用自然语言说明要做什么", nil))
	}
	switch kind {
	case techniquePainter:
		return toolJSON(dispatchPainter(call.Instruction, call.Handle))
	case techniqueMusician:
		return toolJSON(dispatchMusician(call.Instruction))
	case techniqueScreenshoter:
		result, image := dispatchScreenshoter(call.Instruction)
		outcome := toolJSON(result)
		outcome.Image = image
		return outcome
	case techniqueFileOperations, techniqueCreatorLegacy, techniqueFileEditorLegacy:
		return toolJSON(dispatchFileOperations(call.Instruction, call.Handle))
	default:
		return toolJSON(agentdispatch.Failure("type 只能取 painter / musician / screenshoter / file_operations", nil))
	}
}

// dispatchFileOperations 把任务交给文件操作编排：由它统一编排、路由与执行全部文件操作。
// 旧 creator / file_editor 分支同样转这里，不再直接触发文件编辑智能体。
func dispatchFileOperations(instruction string, handles []string) agentdispatch.Result {
	if strings.TrimSpace(instruction) == "" {
		return agentdispatch.Failure("文件操作编排失败：操作要求不能为空", nil)
	}
	ctx, cancel := context.WithTimeout(context.Background(), fileOperationsTimeout)
	defer cancel()
	result, err := creation.RunAssigned(ctx, instruction, handles)
	if err != nil {
		return agentdispatch.PartialFailure("文件操作任务失败："+err.Error(), strings.TrimSpace(result.Summary), result.Handles)
	}
	summary := strings.TrimSpace(result.Summary)
	if summary == "" {
		summary = "文件操作已完成"
	}
	if len(result.ReviewFailures) > 0 {
		summary += fmt.Sprintf("（%d 次校验失败，已按快照回滚或上报回滚错误）", len(result.ReviewFailures))
	}
	return agentdispatch.Success(summary, result.Handles)
}

func dispatchPainter(instruction string, handles []string) agentdispatch.Result {
	if instruction == "" {
		return agentdispatch.Failure("绘画任务调度失败：创作描述不能为空", nil)
	}
	var reference *Painter.Reference
	usedHandles := make([]string, 0, 2)
	if len(handles) > 0 {
		tag := strings.TrimSpace(handles[0])
		handle, source, desc, ok := resolveImageRef(tag)
		if !ok {
			return agentdispatch.Failure("绘画任务调度失败：图片句柄 "+tag+" 不存在，请用 fetch_resources（type=\"list\", query=[\"I\"]）查看可引用的图片句柄后重试", nil)
		}
		if source == "" {
			note := ""
			if desc != "" {
				note = "：" + desc
			}
			return agentdispatch.Failure("绘画任务调度失败："+handle+" 的图片本体已从内容仓库回收（仅存描述"+note+"），请让用户重新发送该图片", nil)
		}
		reference = &Painter.Reference{Handle: handle, Source: source, Desc: desc}
		usedHandles = append(usedHandles, handle)
	}
	result, err := Painter.Perform(context.Background(), instruction, reference, painterEnv{})
	usedHandles = append(usedHandles, result.Handles...)
	summary := strings.TrimSpace(result.Summary)
	if err != nil {
		return agentdispatch.PartialFailure("绘画任务失败："+err.Error(), summary, usedHandles)
	}
	if summary == "" {
		summary = "绘画已完成"
	}
	return agentdispatch.Success(summary, usedHandles)
}

// painterEnv 把绘制者需要的宿主能力接到月华的推送、视觉副本与图片解析管线。
// 绘制者只声明能力，具体传输细节留在月华这一侧。
type painterEnv struct{}

func (painterEnv) PushImages(images []string) []string {
	return pushAssistantImage(images, false)
}

func (painterEnv) VisionCopy(relPath, tag string) (int, int, error) {
	_, _, width, height, err := visionCopy(relPath, tag, 0)
	return width, height, err
}

func (painterEnv) VisionReference(handle, blobPath string) string {
	return visionReference(handle, blobPath)
}

func (painterEnv) ReadImageBytes(source string) ([]byte, error) {
	return resolveImageBytes(source)
}

func dispatchMusician(instruction string) agentdispatch.Result {
	if instruction == "" {
		return agentdispatch.Failure("音乐任务调度失败：创作描述不能为空", nil)
	}
	result, err := Musician.Perform(context.Background(), instruction)
	if err != nil {
		return agentdispatch.Failure("演奏没能完成："+err.Error(), nil)
	}
	// 两个前端共用同一份音乐消息：可播放音频地址 + 标题 + 描述 + 时长。
	pushAssistantMusic(result.Title, result.Description, result.Duration, result.AudioURL)
	return agentdispatch.Success(result.Summary, nil)
}

// dispatchScreenshoter 把截图需求交给独立截图智能体，只消费其汇报与主帧。
func dispatchScreenshoter(requirement string) (agentdispatch.Result, string) {
	if requirement == "" {
		return agentdispatch.Failure("截图任务调度失败：截图要求不能为空", nil), ""
	}
	result, err := Screenshoter.Perform(context.Background(), requirement, screenshotEnv{})
	if err != nil {
		return agentdispatch.Failure("截图任务调度失败："+err.Error(), nil), ""
	}
	return agentdispatch.Success(result.Summary, nil), result.Image
}

// screenshotEnv 把截图者需要的宿主能力接到月华的推送与落盘管线。
// 截图者只声明能力，具体传输细节留在月华这一侧。
type screenshotEnv struct{}

func (screenshotEnv) PushImages(images []string) []string {
	return pushAssistantImage(images, false)
}

func (screenshotEnv) SaveImage(name string, data []byte) (string, error) {
	if _, err := persistenceSaveBytes(name, data); err != nil {
		return "", err
	}
	return localPathToReadURL(name), nil
}

// ==== 统一资源工具 ====

// handleFetchResources 统一资源入口：memory / handle / path / list。
// 句柄解析、路径登记与内容渲染全部由 engine/resourcehub 完成。
func handleFetchResources(args map[string]any) ToolResult {
	return ToolResult{Text: resourcehub.RunArgs(context.Background(), args).Text}
}

// registerDialogueTools 已迁移到 tool_registry.go：执行器由装配层注册进 toolreg。
