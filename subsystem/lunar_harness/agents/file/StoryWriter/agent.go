package StoryWriter

// agent.go 故事编写智能体：建纲、按章续写、维护进度，写入 documents/<故事名>/。

import (
	"context"
	"fmt"
	"path"
	"strings"
	"time"

	"LunarSubsystem/LunarHarness/agents/file"
	"LunarSubsystem/LunarHarness/prompts"
)

// Run 执行一次故事编写任务（一次只推进一章）。
//
// 新建还是延续由 ResolveIntent 先判定，再交给模型执行；模型不再自行选择模式。
// 模型建完大纲就停下询问用户时，这里会补一次「立即写正文」的纠正指令，
// 避免把「没有调用 write_chapter」直接抛给 Tick 调度层。
func Run(ctx context.Context, req Request) (Result, error) {
	started := req.StartedUnix
	if started <= 0 {
		started = time.Now().Unix()
	}
	collector := &collector{root: req.WorkspaceRoot}
	stories, _ := ListStories()
	intent := ResolveIntent(req.Mode, req.Story, req.Instruction, req.Handles, stories, started)
	instruction := withHandlePaths(taskBlock(intent, stories)+"\n"+strings.TrimSpace(req.Instruction), req.Handles)

	run := func(instruction string) (string, error) {
		return file.Run(ctx, file.Request{
			AgentID:        AgentID,
			Role:           Role,
			SystemPrompt:   prompts.ReadOr(promptFile, "你是故事创作智能体，只通过工具读写故事目录。"),
			Instruction:    instruction,
			Tools:          definitions(),
			Handler:        toolHandler(collector, intent),
			MaxRounds:      maxRounds,
			Timeout:        writeTimeout,
			MaxTokens:      maxTokens,
			KeepFullBudget: true,
		})
	}
	buildResult := func(summary string) Result {
		result := Result{
			Summary: strings.TrimSpace(summary),
			Folder:  storyFolderFrom(collector.changed()),
			Handles: preferredHandles(collector.handles()),
			Changed: collector.changed(),
			Mode:    intent.Mode,
			Story:   intent.Story,
		}
		if collector.folder != "" {
			result.Folder = collector.folder
		}
		if result.Mode == ModeNew && result.Folder != "" {
			result.Story = path.Base(strings.Trim(result.Folder, "/"))
		}
		return result
	}

	summary, err := run(instruction)
	result := buildResult(summary)
	if err == nil && requiresChapter(req.Instruction) && chapterChange(result.Changed, result.Story) == "" {
		summary, err = run(retryInstruction(instruction, result.Story))
		result = buildResult(summary)
	}
	if err == nil && requiresChapter(req.Instruction) {
		// 模型可能只写了大纲与进度却声称写了正文；没有章节文件就交给调度层重试。
		if chapter := chapterChange(result.Changed, result.Story); chapter == "" {
			err = fmt.Errorf("没有写入任何章节文件；请用 write_chapter 写完本章正文")
		}
	}
	if result.Summary == "" {
		if err != nil {
			result.Summary = "故事创作未完成"
		} else {
			result.Summary = "故事创作已完成"
		}
	}
	return result, err
}

// retryInstruction 追加纠正说明：大纲已建好，本轮必须直接写正文，不要询问用户。
func retryInstruction(base, story string) string {
	var b strings.Builder
	b.WriteString(base)
	b.WriteString("\n\n【上一轮未完成】\n")
	if story != "" {
		b.WriteString("故事《")
		b.WriteString(story)
		b.WriteString("》的大纲已经建好，不要重复建纲。")
	}
	b.WriteString("上一轮没有调用 write_chapter，本轮必须立即用 write_chapter 写完本章正文：" +
		"不要询问用户、不要等待确认、不要只输出创作说明。")
	return b.String()
}

// requiresChapter 判断本次任务是否必须产出章节；明确说“只建大纲”时不强制。
func requiresChapter(instruction string) bool {
	for _, cue := range []string{"只建大纲", "只写大纲", "不写章节", "不要写章节", "先不写章节", "暂时不写"} {
		if strings.Contains(instruction, cue) {
			return false
		}
	}
	return true
}
