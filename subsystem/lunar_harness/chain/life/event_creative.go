package life

// event_creative.go 生活创作事件：由生活链按主题把任务直接交给对应的专业分支。
//
// 故事 / 小说主题走「故事创作」，其余走「文件编辑」，两个领域并行、互不调用；
// 生活链只做路由与活动记录。心情决定延续上一轮主题还是切换随机主题；
// 延续任务会带上上一轮的项目目录与资源句柄，新主题只下发一个主题。

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"path"
	"strings"

	"LunarSubsystem/LunarHarness/agents/file/FileOperations"
	"LunarSubsystem/LunarHarness/agents/file/StoryWriter"
	"LunarSubsystem/LunarHarness/chain/storyboard"
	"LunarSubsystem/LunarHarness/engine/activity"
	"LunarSubsystem/LunarHarness/engine/wishchain"
)

type creativeEvent struct{ params eventParams }

func newCreativeEvent() Event { return &creativeEvent{params: paramsOf("creative")} }

func (e *creativeEvent) ID() string       { return "creative" }
func (e *creativeEvent) Weight() int      { return e.params.Weight }
func (e *creativeEvent) DailyQuota() int  { return e.params.Quota }
func (e *creativeEvent) NewRun() EventRun { return &creativeRun{} }

type creativeRunData struct {
	Instruction  string   `json:"instruction"`
	Topic        string   `json:"topic"`
	Folder       string   `json:"folder"`
	Handles      []string `json:"handles,omitempty"`
	Continuation bool     `json:"continuation"`
	Rounds       int      `json:"rounds"`
	Illustrate   bool     `json:"illustrate"`
	Activity     string   `json:"activity,omitempty"`
}

type creativeRun struct{ data creativeRunData }

// creationTopics 自由创作主题池。
var creationTopics = []string{
	"写个小说",
	"写个随机主题的HTML小游戏",
	"写个随机要求的JavaScript代码",
	"写个随机要求的HTML小程序",
	"写个歌曲的歌词",
	"写个带插画的故事",
	"写个长篇故事（按大纲分章推进）",
}

func (r *creativeRun) Start(_ context.Context, env Env) error {
	topic, folder, handles, continuation, rounds := chooseCreation(context.Background(), env)
	r.data.Topic = topic
	r.data.Folder = folder
	r.data.Handles = append([]string(nil), handles...)
	r.data.Continuation = continuation
	r.data.Rounds = rounds
	r.data.Illustrate = strings.Contains(topic, "插画") || strings.Contains(topic, "配图")
	r.data.Instruction = buildCreationInstruction(env, topic, folder, handles, continuation)
	return nil
}

func (r *creativeRun) Next(ctx context.Context, _ Env) (EventStep, error) {
	story := isStoryTopic(r.data.Topic)
	var (
		summary string
		handles []string
		folder  string
		err     error
	)
	if story {
		req := StoryWriter.Request{
			Instruction:   r.data.Instruction,
			Handles:       r.data.Handles,
			WorkspaceRoot: "documents",
			Mode:          StoryWriter.ModeNew,
		}
		// 长篇故事由心情决定新建或延续，直接把模式下发给故事创作，避免模型二次猜测。
		if r.data.Continuation {
			req.Mode = StoryWriter.ModeContinue
			req.Story = path.Base(strings.Trim(r.data.Folder, "/"))
			if req.Story == "." {
				req.Story = ""
			}
		}
		result, runErr := StoryWriter.Run(ctx, req)
		summary, handles, folder, err = result.Summary, result.Handles, result.Folder, runErr
	} else {
		result, runErr := FileOperations.Run(ctx, FileOperations.Request{
			Instruction:   r.data.Instruction,
			Handles:       r.data.Handles,
			WorkspaceRoot: "documents",
		})
		summary, handles, folder, err = result.Summary, result.Handles, result.Folder, runErr
	}
	if err != nil {
		wishchain.NoteLink(wishchain.LinkFailure, truncateAtSentence(err.Error(), 120))
		return EventStep{}, fmt.Errorf("创作失败: %w", err)
	}
	summary = strings.TrimSpace(summary)
	if summary == "" {
		summary = "完成了一次创作"
	}
	folder = strings.TrimSpace(folder)
	if folder == "" {
		folder = strings.TrimSpace(r.data.Folder)
	}
	r.data.Folder = folder
	title := creationTitle(folder, r.data.Topic)
	if r.data.Illustrate {
		r.submitIllustration(handles)
	}
	activityText := fmt.Sprintf("刚写完《%s》", title)
	if r.data.Continuation {
		activityText = fmt.Sprintf("刚把《%s》继续推进了一轮", title)
		if story {
			activityText = fmt.Sprintf("刚把《%s》续写了一章", title)
		}
	}
	r.data.Activity = activityText
	activity.Set(activityText)
	saveCreationState(r.data.Topic, folder, handles, r.data.Rounds)
	wishchain.CompleteMatching(title + "\n" + summary)
	wishchain.NoteLink(wishchain.LinkCreationDone, truncateAtSentence(activityText, 120))
	return EventStep{
		Done:   true,
		Record: activityText,
		Detail: fmt.Sprintf("创作返回 %d 字", len([]rune(summary))),
	}, nil
}

// submitIllustration 把写好的故事交给演绎，输出目录与故事项目目录对齐。
func (r *creativeRun) submitIllustration(handles []string) {
	storyHandle := ""
	for _, handle := range handles {
		if strings.HasPrefix(handle, "@D") {
			storyHandle = handle
			break
		}
	}
	folder := strings.Trim(strings.ReplaceAll(r.data.Folder, "\\", "/"), "/")
	if storyHandle == "" || folder == "" {
		return
	}
	_, _ = storyboard.SubmitTask(storyboard.SubmitRequest{
		Handle:         storyHandle,
		OutputFolder:   path.Base(folder),
		ReviewRequired: false,
	})
}

func (r *creativeRun) ActivityText() string { return strings.TrimSpace(r.data.Activity) }

func (r *creativeRun) Snapshot() json.RawMessage {
	raw, _ := json.Marshal(r.data)
	return raw
}

func (r *creativeRun) Restore(raw json.RawMessage) error {
	if len(raw) == 0 {
		return nil
	}
	return json.Unmarshal(raw, &r.data)
}

// chooseCreation 依据心情与上次创作状态选择延续或切换主题。
func chooseCreation(_ context.Context, env Env) (topic, folder string, handles []string, continuation bool, rounds int) {
	if instruction := strings.TrimSpace(env.Instruction); instruction != "" {
		return instruction, "", nil, false, 1
	}
	stateTopic, stateFolder, stateHandles, stateRounds := creationState()
	if moodPositive(env.MoodLabel) && stateTopic != "" && stateFolder != "" && stateRounds < 6 {
		handles = append([]string(nil), stateHandles...)
		return stateTopic, stateFolder, handles, true, stateRounds + 1
	}
	topic = creationTopics[rand.Intn(len(creationTopics))]
	return topic, "", nil, false, 1
}

// buildCreationInstruction 拼接主题与延续上下文；新作品不预设路径，由智能体按内容起标题。
func buildCreationInstruction(env Env, topic, folder string, handles []string, continuation bool) string {
	var b strings.Builder
	b.WriteString("这是月华本人的状态与创作任务，不是用户的状态或任务。\n")
	b.WriteString("当前心情：")
	b.WriteString(emptyAs(env.MoodLabel, "平静"))
	b.WriteString("；刚才月华在：")
	b.WriteString(emptyAs(env.LastRecord, "没做什么"))
	b.WriteString("。\n")
	// 故事主题把新建 / 延续写成显式标记，故事创作子智能体都以它为准。
	if isStoryTopic(topic) {
		if continuation {
			b.WriteString("【故事模式】延续\n")
		} else {
			b.WriteString("【故事模式】新建\n")
		}
	}
	if continuation {
		b.WriteString("这是延续上一次的创作，不要另起炉灶。\n")
		b.WriteString("上一次的作品标题：")
		b.WriteString(creationTitle(folder, topic))
		b.WriteString("；项目目录：")
		b.WriteString(folder)
		b.WriteString("。\n")
		b.WriteString("本轮修改都放在这个项目目录下，路径写相对该目录的路径即可。\n")
		if len(handles) > 0 {
			b.WriteString("上一次的文件句柄：")
			b.WriteString(strings.Join(handles, "、"))
			b.WriteString("\n")
		}
	} else {
		b.WriteString("这次开始一个新作品，只做下面这一个主题：")
		b.WriteString(topic)
		b.WriteString("。\n")
		b.WriteString("先为作品起一个具体、贴合内容的标题，再把所有文件放进 documents/<标题>/ 目录；" +
			"不要用主题原句或时间戳当目录名。\n")
	}
	if wishes := strings.TrimSpace(wishchain.FormatList()); wishes != "" {
		b.WriteString("\n")
		b.WriteString(wishes)
		b.WriteString("\n")
	}
	b.WriteString("\n请按本主题的要求完成本轮创作。")
	if strings.Contains(topic, "插画") || strings.Contains(topic, "配图") {
		b.WriteString("故事写完后需要交给演绎生成分镜，输出目录与故事目录对齐。")
	}
	return b.String()
}

// creationState 读取创作延续状态。
func creationState() (topic, folder string, handles []string, rounds int) {
	mu.Lock()
	defer mu.Unlock()
	return state.CreationTopic, state.CreationFolder, append([]string(nil), state.CreationHandles...), state.CreationRounds
}

// saveCreationState 写入创作延续状态。
func saveCreationState(topic, folder string, handles []string, rounds int) {
	mu.Lock()
	defer mu.Unlock()
	state.CreationTopic = strings.TrimSpace(topic)
	state.CreationFolder = strings.TrimSpace(folder)
	state.CreationHandles = append([]string(nil), handles...)
	state.CreationRounds = rounds
	saveStateLocked()
}

// isStoryTopic 是否为按大纲分章推进的故事 / 小说主题。
func isStoryTopic(topic string) bool {
	for _, keyword := range []string{"故事", "小说", "长篇"} {
		if strings.Contains(topic, keyword) {
			return true
		}
	}
	return false
}

// creationTitle 取项目目录名作为作品标题；目录未知时回退到主题。
func creationTitle(folder, topic string) string {
	folder = strings.Trim(strings.ReplaceAll(folder, "\\", "/"), "/")
	if folder != "" {
		base := path.Base(folder)
		if base != "." && base != "/" && base != "documents" {
			return base
		}
	}
	return strings.TrimSpace(topic)
}

// moodPositive 判断心情是否偏正向；未知或空标签按正向处理。
func moodPositive(label string) bool {
	switch strings.TrimSpace(label) {
	case "愤怒", "伤心", "无聊", "失望", "厌倦", "担忧", "恐慌", "遗憾":
		return false
	}
	return true
}
