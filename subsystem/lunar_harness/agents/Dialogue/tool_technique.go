package Dialogue

// tool_technique.go 技法创作聚合工具执行器：绘制 / 音乐 / 截图 / 文件操作。
//
// 输入严格使用统一原语 {type, instruction, handle, details}，输出统一为 {result, handle, error}。
// 子智能体（Painter / Musician / Screenshoter）由本包直接调用；
// 文件操作编排与宿主推送能力经注入面回调，避免反向依赖 chain。

import (
	"context"
	"fmt"
	"strings"
	"time"

	"LunarSubsystem/LunarHarness/agents/Musician"
	"LunarSubsystem/LunarHarness/agents/Painter"
	"LunarSubsystem/LunarHarness/agents/Screenshoter"
	"LunarSubsystem/LunarHarness/engine/resourcehub"
	"LunarSubsystem/LunarHarness/protocol/agentdispatch"
)

const (
	// TechniquePainter 绘图子工具
	TechniquePainter = "painter"
	// TechniqueMusician 音乐子工具
	TechniqueMusician = "musician"
	// TechniqueScreenshoter 截图子工具
	TechniqueScreenshoter = "screenshoter"
	// TechniqueFileOperations 文件操作编排分支：统一承接文件读写、归档处理、文献读取与脚本运行
	TechniqueFileOperations = "file_operations"
	// TechniqueCreatorLegacy 旧创作分支（兼容保留，内部同样转文件操作编排）
	TechniqueCreatorLegacy = "creator"
	// TechniqueFileEditorLegacy 旧文件编辑分支（兼容保留，内部同样转文件操作编排）
	TechniqueFileEditorLegacy = "file_editor"
	// fileOperationsTimeout 单次文件操作编排任务的超时上限
	fileOperationsTimeout = 20 * time.Minute
)

// ExecuteDispatchTechnique 技法创作聚合工具：按 type 分派给对应子智能体。
func ExecuteDispatchTechnique(args map[string]any) ToolResult {
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
	case TechniquePainter:
		return toolJSON(dispatchPainter(call.Instruction, call.Handle))
	case TechniqueMusician:
		return toolJSON(dispatchMusician(call.Instruction))
	case TechniqueScreenshoter:
		result, image := dispatchScreenshoter(call.Instruction)
		outcome := toolJSON(result)
		outcome.Image = image
		return outcome
	case TechniqueFileOperations, TechniqueCreatorLegacy, TechniqueFileEditorLegacy:
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
	if RunFileOperations == nil {
		return agentdispatch.Failure("文件操作编排失败：创作链未接入", nil)
	}
	ctx, cancel := context.WithTimeout(context.Background(), fileOperationsTimeout)
	defer cancel()
	summary, usedHandles, reviewFailures, err := RunFileOperations(ctx, instruction, handles)
	if err != nil {
		return agentdispatch.PartialFailure("文件操作任务失败："+err.Error(), strings.TrimSpace(summary), usedHandles)
	}
	summary = strings.TrimSpace(summary)
	if summary == "" {
		summary = "文件操作已完成"
	}
	if reviewFailures > 0 {
		summary += fmt.Sprintf("（%d 次校验失败，已按快照回滚或上报回滚错误）", reviewFailures)
	}
	return agentdispatch.Success(summary, usedHandles)
}

// dispatchPainter 把绘画需求交给绘制者子智能体。
func dispatchPainter(instruction string, handles []string) agentdispatch.Result {
	if instruction == "" {
		return agentdispatch.Failure("绘画任务调度失败：创作描述不能为空", nil)
	}
	var reference *Painter.Reference
	usedHandles := make([]string, 0, 2)
	if len(handles) > 0 {
		tag := strings.TrimSpace(handles[0])
		if ResolveImageRef == nil {
			return agentdispatch.Failure("绘画任务调度失败：图片句柄解析未接入", nil)
		}
		handle, source, desc, ok := ResolveImageRef(tag)
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

// painterEnv 把绘制者需要的宿主能力接到注入面。
// 绘制者只声明能力，具体传输细节留在装配层。
type painterEnv struct{}

func (painterEnv) PushImages(images []string) []string {
	if PushImages == nil {
		return nil
	}
	return PushImages(images)
}

func (painterEnv) VisionCopy(relPath, tag string) (int, int, error) {
	if VisionCopy == nil {
		return 0, 0, fmt.Errorf("视觉副本未接入")
	}
	return VisionCopy(relPath, tag)
}

func (painterEnv) VisionReference(handle, blobPath string) string {
	if VisionReference == nil {
		return ""
	}
	return VisionReference(handle, blobPath)
}

func (painterEnv) ReadImageBytes(source string) ([]byte, error) {
	if ReadImageBytes == nil {
		return nil, fmt.Errorf("图片读取未接入")
	}
	return ReadImageBytes(source)
}

// dispatchMusician 把音乐需求交给演奏者子智能体。
func dispatchMusician(instruction string) agentdispatch.Result {
	if instruction == "" {
		return agentdispatch.Failure("音乐任务调度失败：创作描述不能为空", nil)
	}
	result, err := Musician.Perform(context.Background(), instruction)
	if err != nil {
		return agentdispatch.Failure("演奏没能完成："+err.Error(), nil)
	}
	// 两个前端共用同一份音乐消息：可播放音频地址 + 标题 + 描述 + 时长。
	if PushMusic != nil {
		PushMusic(result.Title, result.Description, result.Duration, result.AudioURL)
	}
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

// screenshotEnv 把截图者需要的宿主能力接到注入面。
type screenshotEnv struct{}

func (screenshotEnv) PushImages(images []string) []string {
	if PushImages == nil {
		return nil
	}
	return PushImages(images)
}

func (screenshotEnv) SaveImage(name string, data []byte) (string, error) {
	if SaveImage == nil {
		return "", fmt.Errorf("图片落盘未接入")
	}
	return SaveImage(name, data)
}

// techniqueScope 返回技法创作各分支的职能范围描述。
func techniqueScope(kind string) (string, bool) {
	switch kind {
	case TechniquePainter:
		return "绘制者子智能体：按自然语言创作图像，支持文生图、图生图、立绘、合照与超分；handle 可传参考图句柄（如 @I.003）。", true
	case TechniqueMusician:
		return "演奏者子智能体：创作音乐作品，输出旋律、和弦与乐器编排说明。", true
	case TechniqueScreenshoter:
		return "截图者子智能体：截取屏幕、窗口或指定区域，并把截图展示给用户。", true
	case TechniqueFileOperations, TechniqueCreatorLegacy, TechniqueFileEditorLegacy:
		return "文件操作子智能体：统一编排文件读写、文档整理、压缩包处理、文献读取与脚本运行；按目标自动路由到文件编辑、归档解析 / 操作与文献解析，写入自动快照、审查与回滚。", true
	}
	return "", false
}

// ExecuteFetchResources 统一资源入口：memory / handle / path / list。
// 句柄解析、路径登记与内容渲染全部由 engine/resourcehub 完成。
func ExecuteFetchResources(args map[string]any) ToolResult {
	return ToolResult{Text: resourcehub.RunArgs(context.Background(), args).Text}
}
