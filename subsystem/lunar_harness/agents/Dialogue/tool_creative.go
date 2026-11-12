package Dialogue

// tool_creative.go 创作聚合工具：用统一输入输出协议驱动六个并列的专业分支。
//
// 分支 story / files / storyboard 对应故事创作、文件编辑、演绎；
// painter / musician / screenshoter 对应绘画、音乐与截图。
// 各领域并行、互不调用，聚合只发生在人格对话这一层，便于模型用同一套参数表达需求。
// 故事与文件编辑同步返回结果；演绎登记强制生活事件，下一 Tick 由生活链执行；
// 技法类分支直接调用对应子智能体，截图的图片帧随工具结果一并返回。

import (
	"context"
	"strings"
	"time"

	"LunarSubsystem/LunarHarness/agents/file/FileOperations"
	"LunarSubsystem/LunarHarness/agents/file/StoryWriter"
	"LunarSubsystem/LunarHarness/protocol/agentdispatch"
)

const (
	// ToolCreativeWork 创作聚合工具名
	ToolCreativeWork = "creative_work"

	// creativeTypeStory 故事创作：新建故事或续写下一章
	creativeTypeStory = "story"
	// creativeTypeFiles 文件编辑：读写、整理、压缩包、脚本与回滚
	creativeTypeFiles = "files"
	// creativeTypeStoryboard 演绎：把故事拆成分镜并逐镜绘制
	creativeTypeStoryboard = "storyboard"

	// creativeStoryTimeout 故事创作单次运行的总超时
	creativeStoryTimeout = 15 * time.Minute
	// creativeFilesTimeout 文件编辑单次运行的总超时
	creativeFilesTimeout = 10 * time.Minute
)

// creativeTypeOrder 创作聚合工具 type 的固定顺序。
var creativeTypeOrder = []string{
	creativeTypeStory, creativeTypeFiles, creativeTypeStoryboard,
	TechniquePainter, TechniqueMusician, TechniqueScreenshoter,
}

// creativeTypeScopes 各领域的职能描述（details=true 专用）。
var creativeTypeScopes = map[string]string{
	creativeTypeStory: "故事创作子智能体：以文件夹为单位管理故事，包含大纲与若干章节。" +
		"新建故事会先起标题、建目录、写大纲；延续故事会先读大纲，再按章节纲要续写下一章。" +
		"续写时在 instruction 里写明「续写《故事名》」或「继续写下一章」。",
	creativeTypeFiles: "文件编辑子智能体：负责文件读写、文档整理、压缩包处理、脚本运行与内容回滚。" +
		"写故事或续写故事请使用 story 分支。",
	creativeTypeStoryboard: "演绎子智能体：把故事拆成分镜并逐镜绘制，图片输出到多媒体标题目录，分镜文档写入 documents/lens/标题。" +
		"故事正文可写进 instruction，也可把故事正文文件的句柄放进 handle。",
}

// ExecuteCreativeWork 执行一次创作聚合工具调用。
func ExecuteCreativeWork(args map[string]any) (result ToolResult) {
	defer func() {
		if recovered := recover(); recovered != nil {
			result = toolJSON(agentdispatch.Failure("创作任务执行异常: "+toText(recovered), nil))
		}
	}()
	call, err := agentdispatch.Parse(args)
	if err != nil {
		return toolJSON(agentdispatch.Failure(err.Error(), nil))
	}
	kind := strings.ToLower(call.Type)
	scope, ok := creativeTypeScopes[kind]
	if !ok {
		scope, ok = techniqueScope(kind)
	}
	if !ok {
		return toolJSON(agentdispatch.Failure("type 只能取 "+strings.Join(creativeTypeOrder, " / "), nil))
	}
	if call.Details {
		return toolJSON(agentdispatch.Success(scope, nil))
	}
	if call.Instruction == "" {
		return toolJSON(agentdispatch.Failure("instruction 不能为空，请用自然语言说明要做什么", nil))
	}

	switch kind {
	case creativeTypeStory:
		return executeStory(call.Instruction, call.Handle)
	case creativeTypeFiles:
		return executeFiles(call.Instruction, call.Handle)
	case creativeTypeStoryboard:
		return scheduleLifePlanning("storyboard", call.Instruction, call.Handle, StoryboardFixedReply)
	case TechniquePainter:
		return toolJSON(dispatchPainter(call.Instruction, call.Handle))
	case TechniqueMusician:
		return toolJSON(dispatchMusician(call.Instruction))
	case TechniqueScreenshoter:
		result, image := dispatchScreenshoter(call.Instruction)
		outcome := toolJSON(result)
		outcome.Image = image
		return outcome
	default:
		return toolJSON(agentdispatch.Failure("type 只能取 "+strings.Join(creativeTypeOrder, " / "), nil))
	}
}

// executeStory 同步驱动故事创作智能体。
func executeStory(instruction string, handles []string) ToolResult {
	ctx, cancel := context.WithTimeout(context.Background(), creativeStoryTimeout)
	defer cancel()
	result, err := StoryWriter.Run(ctx, StoryWriter.Request{
		Instruction:   agentdispatch.WithHandles(instruction, handles),
		Handles:       append([]string(nil), handles...),
		WorkspaceRoot: "documents",
	})
	summary := strings.TrimSpace(result.Summary)
	if err != nil {
		if summary == "" {
			summary = err.Error()
		}
		return toolJSON(agentdispatch.PartialFailure("故事创作未完成："+err.Error(), summary, result.Handles))
	}
	if summary == "" {
		summary = "故事创作已完成"
	}
	return toolJSON(agentdispatch.Success(summary, result.Handles))
}

// executeFiles 同步驱动文件编辑智能体。
func executeFiles(instruction string, handles []string) ToolResult {
	ctx, cancel := context.WithTimeout(context.Background(), creativeFilesTimeout)
	defer cancel()
	result, err := FileOperations.Run(ctx, FileOperations.Request{
		Instruction:   agentdispatch.WithHandles(instruction, handles),
		Handles:       append([]string(nil), handles...),
		WorkspaceRoot: "documents",
	})
	summary := strings.TrimSpace(result.Summary)
	if err != nil {
		if summary == "" {
			summary = err.Error()
		}
		return toolJSON(agentdispatch.PartialFailure("文件编辑未完成："+err.Error(), summary, result.Handles))
	}
	if summary == "" {
		summary = "文件编辑已完成"
	}
	return toolJSON(agentdispatch.Success(summary, result.Handles))
}

// toText 把 recover 到的任意值转成可读文本。
func toText(value any) string {
	if text, ok := value.(string); ok {
		return text
	}
	if err, ok := value.(error); ok {
		return err.Error()
	}
	return "未知错误"
}
