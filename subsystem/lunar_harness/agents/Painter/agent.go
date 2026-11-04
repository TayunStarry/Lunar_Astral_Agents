package Painter

// agent.go 绘制者对外入口与参考图 / 运行时画作编排。

import (
	"context"
	"fmt"

	"LunarSubsystem/LunarHarness/engine/console/logger"
	"LunarSubsystem/LunarHarness/engine/resource"
	"LunarSubsystem/LunarHarness/protocol/agentbase"
)

// Perform 执行一次绘制任务。
//
// 跨任务保留运行时画作、最近作品路径与对话上下文，与原绘制者角色的行为一致；
// 串行执行避免并发任务互相覆盖参考图基座。
func Perform(ctx context.Context, instruction string, ref *Reference, env Env) (Result, error) {
	library := newPromptLibrary()
	system := systemPrompt(library)

	stateMu.Lock()
	defer stateMu.Unlock()

	s := &session{
		ctx:        ctx,
		env:        env,
		prompts:    library,
		custom:     CustomCharacters(),
		resultPath: state.path,
		artwork:    state.artwork,
	}
	s.bindReference(ref)
	s.task = s.buildTaskContent(instruction)
	state.history = append(state.history, chatMessage{Role: "user", Content: s.task})
	if len(state.history) > contextLimit {
		state.history = state.history[len(state.history)-contextLimit:]
	}
	summary := s.run(system, s.tools(), &state.history)
	state.artwork = s.artwork
	state.path = s.resultPath

	handles := make([]string, 0, 1)
	if s.artwork.Handle != "" {
		handles = append(handles, s.artwork.Handle)
	}
	recordRun(summary, handles)
	return Result{Summary: summary, Handles: handles}, nil
}

// bindReference 绑定本次任务的参考图。
//
// 没有显式 reference_tag 时自动沿用最近一次产出的画作：这样「把刚才那张图改成…」
// 这类跨流程请求能直接以刚生成的图为图生图基座，而不是误判成「没有参考图」。
func (s *session) bindReference(reference *Reference) {
	s.reference = ReferenceState{}
	s.fromRuntime = false
	if reference == nil || reference.Source == "" {
		if s.artwork.Image != "" {
			s.reference = s.artwork
			s.fromRuntime = true
			logger.Info(module, "[绘制者] 本次任务未指定参考图，自动沿用最近产出画作: %s", s.artwork.Image)
		}
		return
	}
	s.reference.Handle = reference.Handle
	s.reference.Image = reference.Source
	width, height, err := s.env.VisionCopy(reference.Source, reference.Handle)
	vision := s.env.VisionReference(reference.Handle, reference.Source)
	if err != nil || vision == "" {
		logger.Warn(module, "[绘制者] 参考图视觉副本生成失败: %v", err)
		return
	}
	s.reference.Vision = vision
	s.reference.Width = width
	s.reference.Height = height
	// 本次任务指定了参考图：运行时画作先刷新为该图，直到产出新作品。
	s.setRuntimeImage(reference.Handle, vision, "本次参考图")
}

// rememberRuntimeArtwork 把最新产出登记为运行时画作（句柄缩影版），
// 同时更新图生图基座：同一创作流程后续的局部修改会直接以这张新图为起点。
func (s *session) rememberRuntimeArtwork(source, note string) {
	rel := localPathToRel(source)
	if rel == "" {
		return
	}
	width, height, err := s.env.VisionCopy(rel, "")
	if err != nil {
		logger.Warn(module, "[绘制者] 运行时画作缩影生成失败: %v", err)
		return
	}
	handle := ""
	if entry, ok := resource.ForPath(rel); ok {
		handle = entry.Handle
	}
	vision := s.env.VisionReference(handle, rel)
	s.artwork = ReferenceState{
		Image:  rel,
		Handle: handle,
		Vision: vision,
		Width:  width,
		Height: height,
	}
	s.reference = s.artwork
	s.fromRuntime = false
	s.setRuntimeImage(handle, vision, note)
}

// setRuntimeImage 刷新绘制者的运行时画作消息。
func (s *session) setRuntimeImage(handle, vision, note string) {
	if vision == "" {
		return
	}
	label := "【运行时画作】" + note
	if handle != "" {
		label += "（" + handle + "）"
	}
	label += "。局部修改、延续风格或调整细节时，以这张图作为当前画面基准，不要凭记忆重画。"
	s.runtime = []chatMessage{{
		Role: "user",
		Content: []agentbase.ContentPart{
			{Type: "text", Text: label},
			{Type: "image_url", ImageURL: &agentbase.ImageURL{URL: vision}},
		},
	}}
}

func (s *session) buildTaskContent(taskDescription string) any {
	// 参考图来自运行时画作时，图片已由运行时消息挂载，正文不再重复附图
	if s.reference.Vision == "" || s.fromRuntime {
		return taskDescription
	}
	return []agentbase.ContentPart{
		{Type: "text", Text: taskDescription},
		{Type: "image_url", ImageURL: &agentbase.ImageURL{URL: s.reference.Vision}},
	}
}

func (s *session) planFor(ratio string) DimensionPlan {
	var reference *DimensionPlan
	if s.reference.Image != "" && s.reference.Width > 0 && s.reference.Height > 0 {
		reference = &DimensionPlan{Width: s.reference.Width, Height: s.reference.Height}
	}
	return resolveDimensionPlan(ratio, reference)
}

func (s *session) referenceStatus() string {
	if s.reference.Image == "" {
		return "本次任务尚未附带参考图"
	}
	plan := s.planFor("")
	size := ""
	if s.reference.Width > 0 && s.reference.Height > 0 {
		size = fmt.Sprintf("（%dx%d）", s.reference.Width, s.reference.Height)
	}
	label := s.reference.Handle
	if label == "" {
		label = s.reference.Image
	}
	source := ""
	if s.fromRuntime {
		source = "（沿用上一次生成的画作）"
	}
	return fmt.Sprintf("本次任务参考图：%s%s%s，输出尺寸将按参考图比例取 %dx%d", label, size, source, plan.Width, plan.Height)
}
