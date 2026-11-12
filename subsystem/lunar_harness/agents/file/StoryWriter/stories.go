package StoryWriter

// stories.go 已有故事枚举：只有包含 大纲.md 的目录才算故事。

import (
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"LunarSubsystem/LunarHarness/agents/file/FileSupport"
	"LunarSubsystem/LunarHarness/engine/resource"
)

var (
	chapterNamePattern = regexp.MustCompile(`^\d{3}-.+\.md$`)
	// progressSeqPattern 进度文件里的章节小标题，如「## 001 实际发生」。
	progressSeqPattern = regexp.MustCompile(`(?m)^##\s*(\d{3})`)
)

// ListStories 列出 documents 下包含 大纲.md 的故事目录，按最近更新倒序。
func ListStories() ([]StoryInfo, error) {
	root := FileSupport.Abs(storyRoot)
	entries, err := os.ReadDir(root)
	if errors.Is(err, os.ErrNotExist) {
		return []StoryInfo{}, nil
	}
	if err != nil {
		return nil, err
	}
	stories := make([]StoryInfo, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		dir := filepath.Join(root, name)
		outline, statErr := os.Stat(filepath.Join(dir, outlineFile))
		if statErr != nil || outline.IsDir() {
			continue
		}
		latest := outline.ModTime()
		chapters := 0
		lastChapter := 0
		files, readErr := os.ReadDir(dir)
		if readErr == nil {
			for _, item := range files {
				if item.IsDir() || !strings.HasSuffix(strings.ToLower(item.Name()), ".md") {
					continue
				}
				switch item.Name() {
				case outlineFile, progressFile:
					continue
				}
				chapters++
				if match := chapterSeqPattern.FindStringSubmatch(item.Name()); match != nil {
					if seq, err := strconv.Atoi(match[1]); err == nil && seq > lastChapter {
						lastChapter = seq
					}
				}
				if info, infoErr := item.Info(); infoErr == nil && info.ModTime().After(latest) {
					latest = info.ModTime()
				}
			}
		}
		stories = append(stories, StoryInfo{
			Name:        name,
			Dir:         storyRoot + "/" + name,
			Chapters:    chapters,
			LastChapter: lastChapter,
			Updated:     latest.Format("2006-01-02 15:04"),
			UpdatedUnix: latest.Unix(),
		})
	}
	sort.Slice(stories, func(i, j int) bool { return stories[i].UpdatedUnix > stories[j].UpdatedUnix })
	return stories, nil
}

// preferredHandles 把章节正文句柄排在前面，资源库创作完成后优先预览本章。
func preferredHandles(handles []string) []string {
	chapters := make([]string, 0, len(handles))
	others := make([]string, 0, len(handles))
	for _, handle := range handles {
		entry, err := resource.Resolve(handle)
		if err == nil && chapterNamePattern.MatchString(filepath.Base(entry.BlobPath)) {
			chapters = append(chapters, handle)
		} else {
			others = append(others, handle)
		}
	}
	return append(chapters, others...)
}

// storyFolderFrom 从变更路径推断故事目录（documents/<故事名>）。
func storyFolderFrom(paths []string) string {
	for _, rel := range paths {
		rel = strings.Trim(strings.ReplaceAll(rel, "\\", "/"), "/")
		rest, ok := strings.CutPrefix(rel, storyRoot+"/")
		if !ok {
			continue
		}
		name, _, ok := strings.Cut(rest, "/")
		if ok && name != "" {
			return storyRoot + "/" + name
		}
	}
	return ""
}

// chapterChange 在本次变更里找属于指定故事的章节文件。
func chapterChange(paths []string, story string) string {
	for _, rel := range paths {
		rel = strings.Trim(strings.ReplaceAll(rel, "\\", "/"), "/")
		if story != "" && !strings.HasPrefix(rel, storyRoot+"/"+story+"/") {
			continue
		}
		if strings.HasPrefix(rel, storyRoot+"/") && chapterNamePattern.MatchString(baseName(rel)) {
			return rel
		}
	}
	return ""
}

// nextChapterNumber 取下一章编号：章节文件与进度记录里的最大编号 + 1。
func nextChapterNumber(story string) int {
	last := 0
	dir := FileSupport.Abs(storyRoot + "/" + story)
	if files, err := os.ReadDir(dir); err == nil {
		for _, item := range files {
			if item.IsDir() {
				continue
			}
			if seq := chapterSeq(item.Name()); seq > last {
				last = seq
			}
		}
	}
	if data, err := os.ReadFile(filepath.Join(dir, progressFile)); err == nil {
		for _, match := range progressSeqPattern.FindAllStringSubmatch(string(data), -1) {
			if seq, convErr := strconv.Atoi(match[1]); convErr == nil && seq > last {
				last = seq
			}
		}
	}
	return last + 1
}

// chapterSeq 从文件名解析章节编号。
func chapterSeq(name string) int {
	match := chapterSeqPattern.FindStringSubmatch(name)
	if match == nil {
		return 0
	}
	seq, err := strconv.Atoi(match[1])
	if err != nil {
		return 0
	}
	return seq
}

// baseName 取路径最后一段。
func baseName(rel string) string {
	if index := strings.LastIndex(rel, "/"); index >= 0 {
		return rel[index+1:]
	}
	return rel
}
