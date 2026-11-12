package StoryWriter

// tools.go 故事编写工具：资源读取、故事枚举、整文件创建与追加，
// 以及由 Go 侧拼路径的 create_story / write_chapter。

import (
	"context"
	"fmt"
	"os"
	"path"
	"strings"

	"LunarSubsystem/LunarHarness/agents/file"
	"LunarSubsystem/LunarHarness/agents/file/FileEditor"
	"LunarSubsystem/LunarHarness/agents/file/FileSupport"
	"LunarSubsystem/LunarHarness/engine/resource"
	"LunarSubsystem/LunarHarness/engine/resourcehub"
)

const (
	toolFetch   = "fetch_resources"
	toolStories = "list_stories"
	toolList    = "file_list"
	toolCreate  = "file_create"
	toolAppend  = "file_append"
	toolNew     = "create_story"
	toolChapter = "write_chapter"
)

// collector 收集句柄、变更路径与本次任务落到的故事目录。
type collector struct {
	root         string
	folder       string
	handle       []string
	changedPaths []string
}

func (c *collector) addHandle(handles ...string) {
	for _, handle := range handles {
		handle = strings.TrimSpace(handle)
		if handle == "" || contains(c.handle, handle) {
			continue
		}
		c.handle = append(c.handle, handle)
	}
}

func (c *collector) addChanged(rel string) {
	rel = strings.TrimSpace(rel)
	if rel == "" || contains(c.changedPaths, rel) {
		return
	}
	c.changedPaths = append(c.changedPaths, rel)
}

func (c *collector) handles() []string { return append([]string(nil), c.handle...) }
func (c *collector) changed() []string { return append([]string(nil), c.changedPaths...) }

func toolHandler(c *collector, intent Intent) file.Handler {
	return func(name string, args map[string]any) (file.Outcome, bool, error) {
		switch name {
		case toolFetch:
			return fetch(args), false, nil
		case toolStories:
			return listStories(), false, nil
		case toolList:
			return listFiles(args), false, nil
		case toolCreate:
			return createFile(args, c), false, nil
		case toolAppend:
			return appendContent(args, c), false, nil
		case toolNew:
			return createStory(args, c, intent), false, nil
		case toolChapter:
			return writeChapter(args, c, intent), false, nil
		default:
			return file.Failure(name, fmt.Errorf("未知工具")), false, nil
		}
	}
}

func fetch(args map[string]any) file.Outcome {
	result := resourcehub.RunArgs(context.Background(), args)
	return file.Outcome{JSON: result.Text, Summary: "已读取资源"}
}

func listStories() file.Outcome {
	stories, err := ListStories()
	if err != nil {
		return file.Failure(toolStories, err)
	}
	return file.Outcome{
		JSON:    file.JSON(map[string]any{"stories": stories, "count": len(stories)}),
		Summary: fmt.Sprintf("找到 %d 个故事", len(stories)),
	}
}

func listFiles(args map[string]any) file.Outcome {
	path := file.StringArg(args, "path")
	result := FileEditor.FileList(path)
	return file.Outcome{JSON: result.JSON(), Summary: result.Summary}
}

func createFile(args map[string]any, c *collector) file.Outcome {
	rel, _, err := FileSupport.Resolve(file.StringArg(args, "path"), c.root)
	if err != nil {
		return file.Failure(toolCreate, err)
	}
	if strings.HasPrefix(rel, storyRoot+"/") && chapterNamePattern.MatchString(path.Base(rel)) {
		return file.Failure(toolCreate, fmt.Errorf("章节正文必须用 write_chapter 写入，不要用 file_create 拼路径"))
	}
	content := file.StringArg(args, "content")
	if _, err := FileSupport.BackupWithOp(rel, "create"); err != nil {
		return file.Failure(toolCreate, fmt.Errorf("保存版本失败：%w", err))
	}
	result := FileEditor.CreateFile(rel, content)
	if !result.OK && strings.Contains(result.Summary, "已存在") {
		return file.Failure(toolCreate, fmt.Errorf("目标文件已存在；请用 file_list 确认后换一个编号或路径，不要覆盖已有章节"))
	}
	return mutated(c, rel, result)
}

func appendContent(args map[string]any, c *collector) file.Outcome {
	rel, _, err := FileSupport.Resolve(file.StringArg(args, "path"), c.root)
	if err != nil {
		return file.Failure(toolAppend, err)
	}
	if strings.HasPrefix(rel, storyRoot+"/") && path.Base(rel) == progressFile {
		return file.Failure(toolAppend, fmt.Errorf("进度由 write_chapter 自动追加，不要手动追加"))
	}
	addition := file.StringArg(args, "content")
	if addition == "" {
		return file.Failure(toolAppend, fmt.Errorf("追加内容不能为空"))
	}
	if _, err := FileSupport.BackupWithOp(rel, "append"); err != nil {
		return file.Failure(toolAppend, fmt.Errorf("保存版本失败：%w", err))
	}
	result := FileEditor.AppendText(rel, addition)
	return mutated(c, rel, result)
}

// createStory 新建故事目录：由 Go 侧拼路径并同时创建 大纲.md 与 进度.md。
func createStory(args map[string]any, c *collector, intent Intent) file.Outcome {
	if intent.Mode == ModeContinue {
		return file.Failure(toolNew, fmt.Errorf("本次任务是延续《%s》，不能新建故事目录；请直接用 write_chapter 续写", intent.Story))
	}
	if c.folder != "" {
		// 补救轮次里禁止再建一个新故事：本次任务已有目录，只差正文。
		return file.Failure(toolNew, fmt.Errorf("本次任务已创建《%s》，请直接调用 write_chapter 续写", path.Base(c.folder)))
	}
	title := sanitizeTitle(file.StringArg(args, "title"))
	if intent.Story != "" {
		// 标题由系统判定时强制使用，避免模型改名。
		title = intent.Story
	}
	if title == "" {
		return file.Failure(toolNew, fmt.Errorf("title 不能为空"))
	}
	outline := strings.TrimSpace(file.StringArg(args, "outline"))
	if outline == "" {
		return file.Failure(toolNew, fmt.Errorf("outline 不能为空；请先写完整大纲再建故事"))
	}
	rel := storyRoot + "/" + title + "/" + outlineFile
	if _, err := os.Stat(FileSupport.Abs(rel)); err == nil {
		return file.Failure(toolNew, fmt.Errorf("故事《%s》已存在；请直接调用 write_chapter 续写，不要新建重复目录", title))
	}
	if !strings.HasPrefix(outline, "#") {
		outline = "# 《" + title + "》\n\n" + outline
	}
	if _, err := FileSupport.BackupWithOp(rel, "create"); err != nil {
		return file.Failure(toolNew, fmt.Errorf("保存版本失败：%w", err))
	}
	result := FileEditor.CreateFile(rel, outline)
	if !result.OK {
		return file.Failure(toolNew, fmt.Errorf("创建大纲失败：%s", result.Summary))
	}
	outcome := mutated(c, rel, result)
	c.folder = storyRoot + "/" + title
	c.ensureProgress()
	outcome.Summary = fmt.Sprintf("已创建故事《%s》与大纲：%s", title, rel)
	outcome.JSON = file.JSON(map[string]any{
		"ok": true, "story": title, "dir": storyRoot + "/" + title, "outline": rel,
		"next": "立即调用 write_chapter 写第 001 章正文，不要询问用户或等待确认",
	})
	return outcome
}

// ensureProgress 保证故事目录下有 进度.md。
func (c *collector) ensureProgress() {
	if c.folder == "" {
		return
	}
	rel := c.folder + "/" + progressFile
	if _, err := os.Stat(FileSupport.Abs(rel)); err == nil {
		return
	}
	if _, err := FileSupport.BackupWithOp(rel, "create"); err != nil {
		return
	}
	if result := FileEditor.CreateFile(rel, "# 进度\n"); result.OK {
		c.addChanged(rel)
		c.addHandle(result.Handles...)
	}
}

// writeChapter 写一章正文：路径、编号与进度记录都由 Go 侧生成。
func writeChapter(args map[string]any, c *collector, intent Intent) file.Outcome {
	story := chapterStory(args, c, intent)
	if story == "" {
		return file.Failure(toolChapter, fmt.Errorf("story 不能为空：请先 create_story 或指定已有故事名"))
	}
	dir := storyRoot + "/" + story
	if _, err := os.Stat(FileSupport.Abs(dir + "/" + outlineFile)); err != nil {
		return file.Failure(toolChapter, fmt.Errorf("故事《%s》还没有大纲；新建故事请先调用 create_story", story))
	}
	title := sanitizeTitle(file.StringArg(args, "title"))
	if title == "" {
		return file.Failure(toolChapter, fmt.Errorf("title 不能为空；章节名不要带编号与扩展名"))
	}
	content := file.StringArg(args, "content")
	if content == "" {
		return file.Failure(toolChapter, fmt.Errorf("content 不能为空；本章正文必须完整写入"))
	}
	number := file.IntArg(args, "number")
	if number <= 0 {
		number = nextChapterNumber(story)
	}
	rel := fmt.Sprintf("%s/%03d-%s.md", dir, number, title)
	if _, err := os.Stat(FileSupport.Abs(rel)); err == nil {
		return file.Failure(toolChapter, fmt.Errorf("%s 已存在；请换下一个编号，不要覆盖已有章节", rel))
	}
	if _, err := FileSupport.BackupWithOp(rel, "create"); err != nil {
		return file.Failure(toolChapter, fmt.Errorf("保存版本失败：%w", err))
	}
	result := FileEditor.CreateFile(rel, content)
	if !result.OK {
		return file.Failure(toolChapter, fmt.Errorf("写入章节失败：%s", result.Summary))
	}
	c.folder = dir
	c.ensureProgress()
	outcome := mutated(c, rel, result)
	appendProgress(c, story, number, title, file.StringArg(args, "summary"), file.StringArg(args, "changes"))
	outcome.Summary = fmt.Sprintf("已写入第 %03d 章《%s》：%s", number, title, rel)
	outcome.JSON = file.JSON(map[string]any{
		"ok": true, "story": story, "chapter": rel, "number": number,
	})
	return outcome
}

// chapterStory 确定本章所属故事：系统判定优先，其次工具参数与本次已建目录。
func chapterStory(args map[string]any, c *collector, intent Intent) string {
	if intent.Story != "" {
		return intent.Story
	}
	if story := sanitizeTitle(file.StringArg(args, "story")); story != "" {
		return story
	}
	if c.folder != "" {
		return path.Base(c.folder)
	}
	return ""
}

// appendProgress 把本章记录追加到 进度.md；失败不影响章节已写入的事实。
func appendProgress(c *collector, story string, number int, title, summary, changes string) {
	if summary == "" {
		summary = "—"
	}
	if changes == "" {
		changes = "无"
	}
	rel := storyRoot + "/" + story + "/" + progressFile
	record := fmt.Sprintf("\n## %03d 实际发生\n- 本章：%s\n- 关键事件：%s\n- 设定变更：%s\n", number, title, summary, changes)
	if _, err := FileSupport.BackupWithOp(rel, "append"); err != nil {
		return
	}
	if result := FileEditor.AppendText(rel, record); result.OK {
		c.addChanged(rel)
		c.addHandle(result.Handles...)
	}
}

func mutated(c *collector, rel string, result FileEditor.OpResult) file.Outcome {
	outcome := file.Outcome{JSON: result.JSON(), Summary: result.Summary, Handles: result.Handles}
	if result.OK {
		outcome.Mutated = true
		c.addChanged(rel)
		c.addHandle(result.Handles...)
	}
	return outcome
}

func withHandlePaths(instruction string, handles []string) string {
	lines := make([]string, 0, len(handles))
	for _, handle := range handles {
		entry, err := resource.Resolve(handle)
		if err == nil && strings.TrimSpace(entry.BlobPath) != "" {
			lines = append(lines, "- "+handle+" → "+entry.BlobPath)
		} else {
			lines = append(lines, "- "+handle)
		}
	}
	if len(lines) == 0 {
		return instruction
	}
	return instruction + "\n\n【目标句柄】\n" + strings.Join(lines, "\n")
}

func contains(list []string, item string) bool {
	for _, value := range list {
		if value == item {
			return true
		}
	}
	return false
}
