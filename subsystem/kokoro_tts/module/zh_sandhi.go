package module

import (
	"sort"
	"strings"
)

// wordPos 分词结果（词 + 词性）
type wordPos struct {
	word string
	pos  string
}

// toneSandhi 普通话变调规则（移植自 misaki ToneSandhi）
type toneSandhi struct {
	mustNeural    map[string]struct{}
	mustNotNeural map[string]struct{}
	punc          string
	cutForSearch  func(word string) []string
}

// newToneSandhi 创建变调规则实例
func newToneSandhi(cutForSearch func(string) []string) *toneSandhi {
	return &toneSandhi{
		mustNeural:    mustNeuralWords(),
		mustNotNeural: mustNotNeuralWords(),
		punc:          "、：，；。？！“”‘’':,;.?!",
		cutForSearch:  cutForSearch,
	}
}

// splitWord 使用搜索引擎分词拆词
func (t *toneSandhi) splitWord(word string) []string {
	words := t.cutForSearch(word)
	if len(words) == 0 {
		return []string{word}
	}
	sort.SliceStable(words, func(i, j int) bool {
		return len(words[i]) < len(words[j])
	})
	first := words[0]
	if first == "" {
		return []string{word}
	}
	idx := strings.Index(word, first)
	if idx == 0 {
		return []string{first, word[len(first):]}
	}
	return []string{word[:len(word)-len(first)], first}
}

// neuralSandhi 轻声变调
func (t *toneSandhi) neuralSandhi(word, pos string, finals []string) []string {
	if _, ok := t.mustNotNeural[word]; ok {
		return finals
	}
	// 叠词（名词/动词/形容词），如 奶奶/试试/旺旺
	chars := []rune(word)
	for j := 1; j < len(chars); j++ {
		if chars[j] == chars[j-1] && pos != "" && strings.Contains("nva", pos[:1]) {
			finals[j] = finals[j][:len(finals[j])-1] + "5"
		}
	}
	geIdx := strings.IndexRune(word, '个')
	switch {
	case strings.ContainsRune("吧呢啊呐噻嘛吖嗨呐哦哒滴哩哟喽啰耶喔诶", rune(word[len(word)-1])):
		finals[len(finals)-1] = finals[len(finals)-1][:len(finals[len(finals)-1])-1] + "5"
	case strings.ContainsRune("的地得", rune(word[len(word)-1])):
		finals[len(finals)-1] = finals[len(finals)-1][:len(finals[len(finals)-1])-1] + "5"
	case len(chars) == 1 && strings.ContainsRune("了着过", rune(word[0])) && (pos == "ul" || pos == "uz" || pos == "ug"):
		finals[len(finals)-1] = finals[len(finals)-1][:len(finals[len(finals)-1])-1] + "5"
	case len(chars) > 1 && strings.ContainsRune("们子", rune(word[len(word)-1])) && (strings.HasPrefix(pos, "r") || strings.HasPrefix(pos, "n")):
		finals[len(finals)-1] = finals[len(finals)-1][:len(finals[len(finals)-1])-1] + "5"
	case len(chars) > 1 && strings.ContainsRune("上下", rune(word[len(word)-1])) && (pos == "s" || pos == "l" || pos == "f"):
		finals[len(finals)-1] = finals[len(finals)-1][:len(finals[len(finals)-1])-1] + "5"
	case len(chars) > 1 && strings.ContainsRune("来去", rune(word[len(word)-1])) && strings.ContainsRune("上下进出回过起开", rune(word[len(word)-2])):
		finals[len(finals)-1] = finals[len(finals)-1][:len(finals[len(finals)-1])-1] + "5"
	case geIdx >= 1 && (isNumRune(rune(word[geIdx-1])) || strings.ContainsRune("几有两半多各整每做是", rune(word[geIdx-1]))) || word == "个":
		finals[geIdx] = finals[geIdx][:len(finals[geIdx])-1] + "5"
	default:
		if _, ok := t.mustNeural[word]; ok {
			finals[len(finals)-1] = finals[len(finals)-1][:len(finals[len(finals)-1])-1] + "5"
		} else if len(word) >= 2 {
			if _, ok := t.mustNeural[word[len(word)-2:]]; ok {
				finals[len(finals)-1] = finals[len(finals)-1][:len(finals[len(finals)-1])-1] + "5"
			}
		}
	}
	// 拆词后复查轻声词
	wordList := t.splitWord(word)
	finalsList := [][]string{finals[:min(len(finals), len([]rune(wordList[0])))], finals[min(len(finals), len([]rune(wordList[0]))):]}
	for i, w := range wordList {
		if _, ok := t.mustNeural[w]; ok {
			finalsList[i][len(finalsList[i])-1] = finalsList[i][len(finalsList[i])-1][:len(finalsList[i][len(finalsList[i])-1])-1] + "5"
		} else if len(w) >= 2 {
			if _, ok := t.mustNeural[w[len(w)-2:]]; ok {
				finalsList[i][len(finalsList[i])-1] = finalsList[i][len(finalsList[i])-1][:len(finalsList[i][len(finalsList[i])-1])-1] + "5"
			}
		}
	}
	var out []string
	out = append(out, finalsList[0]...)
	out = append(out, finalsList[1]...)
	return out
}

func isNumRune(r rune) bool {
	return r >= '0' && r <= '9'
}

// buSandhi 不 的变调
func (t *toneSandhi) buSandhi(word string, finals []string) []string {
	chars := []rune(word)
	if len(chars) == 3 && chars[1] == '不' {
		finals[1] = finals[1][:len(finals[1])-1] + "5"
		return finals
	}
	for i, c := range chars {
		if c == '不' && i+1 < len(chars) && finals[i+1][len(finals[i+1])-1:] == "4" {
			finals[i] = finals[i][:len(finals[i])-1] + "2"
		}
	}
	return finals
}

// yiSandhi 一 的变调
func (t *toneSandhi) yiSandhi(word string, finals []string) []string {
	chars := []rune(word)
	hasYi := false
	allNum := true
	for i, c := range chars {
		if c == '一' {
			hasYi = true
			if !isNumRune(c) {
				// 跳过判断，单独处理
				_ = i
			}
		} else if !isNumRune(c) {
			allNum = false
		}
	}
	if hasYi && allNum {
		return finals
	}
	if len(chars) == 3 && chars[1] == '一' && chars[0] == chars[2] {
		finals[1] = finals[1][:len(finals[1])-1] + "5"
		return finals
	}
	if strings.HasPrefix(word, "第一") {
		finals[1] = finals[1][:len(finals[1])-1] + "1"
		return finals
	}
	for i, c := range chars {
		if c == '一' && i+1 < len(chars) {
			last := finals[i+1][len(finals[i+1])-1:]
			if last == "4" || last == "5" {
				finals[i] = finals[i][:len(finals[i])-1] + "2"
			} else if !strings.ContainsRune(t.punc, chars[i+1]) {
				finals[i] = finals[i][:len(finals[i])-1] + "4"
			}
		}
	}
	return finals
}

func allToneThree(finals []string) bool {
	if len(finals) == 0 {
		return false
	}
	for _, f := range finals {
		if f == "" || f[len(f)-1:] != "3" {
			return false
		}
	}
	return true
}

// threeSandhi 三声变调
func (t *toneSandhi) threeSandhi(word string, finals []string) []string {
	chars := []rune(word)
	switch {
	case len(chars) == 2 && allToneThree(finals):
		finals[0] = finals[0][:len(finals[0])-1] + "2"
	case len(chars) == 3:
		wordList := t.splitWord(word)
		if allToneThree(finals) {
			if len([]rune(wordList[0])) == 2 {
				finals[0] = finals[0][:len(finals[0])-1] + "2"
				finals[1] = finals[1][:len(finals[1])-1] + "2"
			} else if len([]rune(wordList[0])) == 1 {
				finals[1] = finals[1][:len(finals[1])-1] + "2"
			}
		} else {
			finalsList := [][]string{finals[:len([]rune(wordList[0]))], finals[len([]rune(wordList[0])):]}
			if len(finalsList) == 2 {
				for i, sub := range finalsList {
					if allToneThree(sub) && len(sub) == 2 {
						finalsList[i][0] = finalsList[i][0][:len(finalsList[i][0])-1] + "2"
					} else if i == 1 && !allToneThree(sub) && finalsList[i][0][len(finalsList[i][0])-1:] == "3" && finalsList[0][len(finalsList[0])-1][len(finalsList[0][len(finalsList[0])-1])-1:] == "3" {
						finalsList[0][len(finalsList[0])-1] = finalsList[0][len(finalsList[0])-1][:len(finalsList[0][len(finalsList[0])-1])-1] + "2"
					}
				}
				finals = append(finalsList[0], finalsList[1]...)
			}
		}
	case len(chars) == 4:
		var out []string
		for _, sub := range [][]string{finals[:2], finals[2:]} {
			if allToneThree(sub) {
				sub[0] = sub[0][:len(sub[0])-1] + "2"
			}
			out = append(out, sub...)
		}
		finals = out
	}
	return finals
}

// mergeBu 合并「不」与其后词语
func (t *toneSandhi) mergeBu(seg []wordPos) []wordPos {
	var newSeg []wordPos
	for i, s := range seg {
		if s.pos != "x" && s.pos != "eng" {
			if i > 0 && seg[i-1].word == "不" {
				s.word = "不" + s.word
			}
		}
		nextPos := ""
		if i+1 < len(seg) {
			nextPos = seg[i+1].pos
		}
		if s.word != "不" || nextPos == "" || nextPos == "x" || nextPos == "eng" {
			newSeg = append(newSeg, s)
		}
	}
	return newSeg
}

// mergeYi 合并「一」与叠词或后词
func (t *toneSandhi) mergeYi(seg []wordPos) []wordPos {
	var newSeg []wordPos
	skip := false
	for i, s := range seg {
		if skip {
			skip = false
			continue
		}
		if i-1 >= 0 && s.word == "一" && i+1 < len(seg) &&
			seg[i-1].word == seg[i+1].word && seg[i-1].pos == "v" && seg[i+1].pos != "x" && seg[i+1].pos != "eng" {
			last := newSeg[len(newSeg)-1]
			newSeg[len(newSeg)-1] = wordPos{word: last.word + "一" + seg[i+1].word, pos: last.pos}
			skip = true
		} else {
			newSeg = append(newSeg, s)
		}
	}
	seg = newSeg
	newSeg = nil
	for _, s := range seg {
		if len(newSeg) > 0 && newSeg[len(newSeg)-1].word == "一" && s.pos != "x" && s.pos != "eng" {
			last := newSeg[len(newSeg)-1]
			newSeg[len(newSeg)-1] = wordPos{word: last.word + s.word, pos: last.pos}
		} else {
			newSeg = append(newSeg, s)
		}
	}
	return newSeg
}

// isReduplication 判断两字叠词
func isReduplication(word string) bool {
	chars := []rune(word)
	return len(chars) == 2 && chars[0] == chars[1]
}

// mergeContinuousThreeTones 合并连续三声词（处理三声连读）
func (t *toneSandhi) mergeContinuousThreeTones(seg []wordPos) []wordPos {
	var newSeg []wordPos
	var subFinalsList [][]string
	for _, s := range seg {
		if s.pos == "x" || s.pos == "eng" {
			subFinalsList = append(subFinalsList, []string{"0"})
			continue
		}
		_, finals := getInitialsFinals(s.word)
		subFinalsList = append(subFinalsList, finals)
	}
	mergeLast := make([]bool, len(seg))
	for i, s := range seg {
		if s.pos != "x" && s.pos != "eng" && i-1 >= 0 &&
			allToneThree(subFinalsList[i-1]) && allToneThree(subFinalsList[i]) && !mergeLast[i-1] {
			if !isReduplication(seg[i-1].word) && len([]rune(seg[i-1].word))+len([]rune(seg[i].word)) <= 3 {
				last := newSeg[len(newSeg)-1]
				newSeg[len(newSeg)-1] = wordPos{word: last.word + seg[i].word, pos: last.pos}
				mergeLast[i] = true
			} else {
				newSeg = append(newSeg, s)
			}
		} else {
			newSeg = append(newSeg, s)
		}
	}
	return newSeg
}

// mergeContinuousThreeTones2 合并相邻词边界三声（首字/末字三声）
func (t *toneSandhi) mergeContinuousThreeTones2(seg []wordPos) []wordPos {
	var newSeg []wordPos
	var subFinalsList [][]string
	for _, s := range seg {
		if s.pos == "x" || s.pos == "eng" {
			subFinalsList = append(subFinalsList, []string{"0"})
			continue
		}
		_, finals := getInitialsFinals(s.word)
		subFinalsList = append(subFinalsList, finals)
	}
	mergeLast := make([]bool, len(seg))
	for i, s := range seg {
		if s.pos != "x" && s.pos != "eng" && i-1 >= 0 &&
			len(subFinalsList[i-1]) > 0 && len(subFinalsList[i]) > 0 &&
			subFinalsList[i-1][len(subFinalsList[i-1])-1][len(subFinalsList[i-1][len(subFinalsList[i-1])-1])-1:] == "3" &&
			subFinalsList[i][0][len(subFinalsList[i][0])-1:] == "3" && !mergeLast[i-1] {
			if !isReduplication(seg[i-1].word) && len([]rune(seg[i-1].word))+len([]rune(seg[i].word)) <= 3 {
				last := newSeg[len(newSeg)-1]
				newSeg[len(newSeg)-1] = wordPos{word: last.word + seg[i].word, pos: last.pos}
				mergeLast[i] = true
			} else {
				newSeg = append(newSeg, s)
			}
		} else {
			newSeg = append(newSeg, s)
		}
	}
	return newSeg
}

// mergeEr 合并「儿」到前词
func (t *toneSandhi) mergeEr(seg []wordPos) []wordPos {
	var newSeg []wordPos
	for i, s := range seg {
		if i-1 >= 0 && s.word == "儿" && newSeg[len(newSeg)-1].pos != "x" && newSeg[len(newSeg)-1].pos != "eng" {
			last := newSeg[len(newSeg)-1]
			newSeg[len(newSeg)-1] = wordPos{word: last.word + s.word, pos: last.pos}
		} else {
			newSeg = append(newSeg, s)
		}
	}
	return newSeg
}

// mergeReduplication 合并相邻叠词
func (t *toneSandhi) mergeReduplication(seg []wordPos) []wordPos {
	var newSeg []wordPos
	for _, s := range seg {
		if len(newSeg) > 0 && s.word == newSeg[len(newSeg)-1].word && s.pos != "x" && s.pos != "eng" {
			last := newSeg[len(newSeg)-1]
			newSeg[len(newSeg)-1] = wordPos{word: last.word + s.word, pos: last.pos}
		} else {
			newSeg = append(newSeg, s)
		}
	}
	return newSeg
}

// preMerge 变调前合并预处理
func (t *toneSandhi) preMerge(seg []wordPos) []wordPos {
	seg = t.mergeBu(seg)
	seg = t.mergeYi(seg)
	seg = t.mergeReduplication(seg)
	seg = t.mergeContinuousThreeTones(seg)
	seg = t.mergeContinuousThreeTones2(seg)
	seg = t.mergeEr(seg)
	return seg
}

// modifiedTone 应用全部变调规则
func (t *toneSandhi) modifiedTone(word, pos string, finals []string) []string {
	finals = t.buSandhi(word, finals)
	finals = t.yiSandhi(word, finals)
	finals = t.neuralSandhi(word, pos, finals)
	finals = t.threeSandhi(word, finals)
	return finals
}
