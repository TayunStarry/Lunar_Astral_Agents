package Painter

// agent.go 绘制者对外入口与参考图 / 运行时画作编排。

import (
	"context"
	"fmt"
	"strings"

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
	if err := s.bindReference(ref); err != nil {
		return Result{}, err
	}
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

// UpdateArtworkPath 在外部迁移画作文件后同步运行时画作与最近作品路径。
// 只改内部记录的本地相对路径，视觉副本与资源句柄保持不变；
// 避免图片被移动后，「把刚才那张图改成…」仍指向旧路径。
func UpdateArtworkPath(oldRel, newRel string) {
	oldRel = strings.TrimSpace(oldRel)
	newRel = strings.TrimSpace(newRel)
	if oldRel == "" || newRel == "" || oldRel == newRel {
		return
	}
	stateMu.Lock()
	defer stateMu.Unlock()
	if state.artwork.Image == oldRel {
		state.artwork.Image = newRel
	}
	if state.path == oldRel {
		state.path = newRel
	}
}

// bindReference 绑定本次任务的参考图。
//
// 显式请求（创作链）没有参考图时保持纯文生图，不再沿用最近画作；
// 未显式声明的调用（人格对话）仍沿用最近产出画作，支持「把刚才那张图改成…」。
func (s *session) bindReference(reference *Reference) error {
	s.reference = ReferenceState{}
	s.extras = nil
	s.textOnly = false
	s.forceImg2Img = false
	s.fromRuntime = false
	explicit := reference != nil && reference.Explicit
	if reference == nil || (!explicit && reference.Source == "") {
		if s.artwork.Image != "" {
			s.reference = s.artwork
			s.fromRuntime = true
			logger.Info(module, "["+roleLabel+"] 本次任务未指定参考图，自动沿用最近产出画作: %s", s.artwork.Image)
		}
		return nil
	}
	if reference.Source == "" {
		s.textOnly = true
		logger.Info(module, "["+roleLabel+"] 本次任务明确要求纯文生图，不沿用最近画作")
		return nil
	}
	primary, err := s.prepareReference(*reference, "本次参考图")
	if err != nil {
		return err
	}
	extras := make([]ReferenceState, 0, len(reference.Extras))
	for _, extra := range reference.Extras {
		state, err := s.prepareReference(extra, "附加参考图")
		if err != nil {
			return err
		}
		extras = append(extras, state)
	}
	s.reference = primary
	s.extras = extras
	// 创作链显式参考图必须作为图生图基座使用，禁止退化成纯文生图重画；
	// 人格对话保留「参考图只作视觉对照」的弹性。
	s.forceImg2Img = explicit
	// 本次任务指定了参考图：运行时画作先刷新为这些图，直到产出新作品。
	s.setRuntimeImages(primary, extras, "本次参考图")
	return nil
}

// prepareReference 生成参考图的视觉副本与引用；失败直接报错，禁止静默降级。
func (s *session) prepareReference(reference Reference, note string) (ReferenceState, error) {
	label := referenceLabel(reference)
	width, height, err := s.env.VisionCopy(reference.Source, reference.Handle)
	if err != nil {
		return ReferenceState{}, fmt.Errorf("%s %s 视觉副本生成失败：%w", note, label, err)
	}
	vision := s.env.VisionReference(reference.Handle, reference.Source)
	if vision == "" {
		return ReferenceState{}, fmt.Errorf("%s %s 没有生成可用的视觉引用", note, label)
	}
	return ReferenceState{
		Image:  reference.Source,
		Handle: reference.Handle,
		Vision: vision,
		Width:  width,
		Height: height,
	}, nil
}

// referenceLabel 参考图的展示名。
func referenceLabel(reference Reference) string {
	if label := strings.TrimSpace(reference.Handle); label != "" {
		return label
	}
	if label := strings.TrimSpace(reference.Source); label != "" {
		return label
	}
	return "（未命名参考图）"
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
		logger.Warn(module, "["+roleLabel+"] 运行时画作缩影生成失败: %v", err)
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
	s.extras = nil
	s.fromRuntime = false
	s.setRuntimeImages(s.artwork, nil, note)
}

// setRuntimeImages 刷新绘制者的运行时画作消息；第一张是图生图基座，其余为视觉对照。
func (s *session) setRuntimeImages(primary ReferenceState, extras []ReferenceState, note string) {
	refs := make([]ReferenceState, 0, 1+len(extras))
	refs = append(refs, primary)
	refs = append(refs, extras...)
	labels := make([]string, 0, len(refs))
	parts := make([]agentbase.ContentPart, 0, len(refs)+1)
	for index, ref := range refs {
		if ref.Vision == "" {
			continue
		}
		label := "主参考图"
		if index > 0 {
			label = "附加参考图"
		}
		if ref.Handle != "" {
			label += "（" + ref.Handle + "）"
		}
		labels = append(labels, label)
		parts = append(parts, agentbase.ContentPart{
			Type:     "image_url",
			ImageURL: &agentbase.ImageURL{URL: ref.Vision},
		})
	}
	if len(parts) == 0 {
		return
	}
	text := "【运行时画作】" + note + "：" + strings.Join(labels, "；") +
		"。第一张是图生图基座；局部修改、延续风格或调整细节时以这些图作为画面基准，不要凭记忆重画。"
	content := make([]agentbase.ContentPart, 0, len(parts)+1)
	content = append(content, agentbase.ContentPart{Type: "text", Text: text})
	content = append(content, parts...)
	s.runtime = []chatMessage{{Role: "user", Content: content}}
}

func (s *session) buildTaskContent(taskDescription string) any {
	// 参考图已由运行时消息挂载时，正文不再重复附图
	if len(s.runtime) > 0 || s.fromRuntime || s.reference.Vision == "" {
		return taskDescription
	}
	parts := make([]agentbase.ContentPart, 0, 2+len(s.extras))
	parts = append(parts, agentbase.ContentPart{Type: "text", Text: taskDescription})
	parts = append(parts, agentbase.ContentPart{Type: "image_url", ImageURL: &agentbase.ImageURL{URL: s.reference.Vision}})
	for _, extra := range s.extras {
		if extra.Vision == "" {
			continue
		}
		parts = append(parts, agentbase.ContentPart{Type: "image_url", ImageURL: &agentbase.ImageURL{URL: extra.Vision}})
	}
	return parts
}

func (s *session) planFor(ratio string) DimensionPlan {
	var reference *DimensionPlan
	if s.reference.Image != "" && s.reference.Width > 0 && s.reference.Height > 0 {
		reference = &DimensionPlan{Width: s.reference.Width, Height: s.reference.Height}
	}
	return resolveDimensionPlan(ratio, reference)
}

func (s *session) referenceStatus() string {
	if s.textOnly {
		return "本次任务明确要求纯文生图，没有参考图"
	}
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
	extra := ""
	if len(s.extras) > 0 {
		extra = fmt.Sprintf("，另有 %d 张附加参考图", len(s.extras))
	}
	return fmt.Sprintf("本次任务参考图：%s%s%s%s，输出尺寸将按参考图比例取 %dx%d", label, size, source, extra, plan.Width, plan.Height)
}
