package module

import (
	"strings"

	"github.com/yanyiwu/gojieba"
)

// zhFrontend 中文文本到注音音素前端（移植自 misaki ZHFrontend）
type zhFrontend struct {
	jieba  *gojieba.Jieba
	sandhi *toneSandhi
	punc   map[string]struct{}
}

// zhToken 前端中间 token
type zhToken struct {
	phonemes   string
	whitespace string
	tag        string
}

// mustErhua 必须儿化的词
var mustErhua = map[string]struct{}{
	"小院儿": {}, "胡同儿": {}, "范儿": {}, "老汉儿": {}, "撒欢儿": {}, "寻老礼儿": {}, "妥妥儿": {}, "媳妇儿": {},
}

// notErhua 不可儿化的词
var notErhua = map[string]struct{}{
	"虐儿": {}, "为儿": {}, "护儿": {}, "瞒儿": {}, "救儿": {}, "替儿": {}, "有儿": {}, "一儿": {}, "我儿": {},
	"俺儿": {}, "妻儿": {}, "拐儿": {}, "聋儿": {}, "乞儿": {}, "患儿": {}, "幼儿": {}, "孤儿": {}, "婴儿": {},
	"婴幼儿": {}, "连体儿": {}, "脑瘫儿": {}, "流浪儿": {}, "体弱儿": {}, "混血儿": {}, "蜜雪儿": {}, "舫儿": {},
	"祖儿": {}, "美儿": {}, "应采儿": {}, "可儿": {}, "侄儿": {}, "孙儿": {}, "侄孙儿": {}, "女儿": {}, "男儿": {},
	"红孩儿": {}, "花儿": {}, "虫儿": {}, "马儿": {}, "鸟儿": {}, "猪儿": {}, "猫儿": {}, "狗儿": {}, "少儿": {},
}

// newZhFrontend 初始化中文前端（加载 jieba 分词）
func newZhFrontend() (*zhFrontend, error) {
	j := gojieba.NewJieba()
	f := &zhFrontend{
		jieba: j,
		punc:  make(map[string]struct{}),
	}
	for _, r := range "；:,.!?—…\"()“”" {
		f.punc[string(r)] = struct{}{}
	}
	f.sandhi = newToneSandhi(func(word string) []string {
		return j.CutForSearch(word, true)
	})
	return f, nil
}

// mapPunctuation 将中文标点映射为英文标点（移植自 misaki）
func mapPunctuation(text string) string {
	replacer := strings.NewReplacer(
		"、", ", ", "，", ", ", "。", ". ", "．", ". ", "！", "! ",
		"：", ": ", "；", "; ", "？", "? ", "«", " “", "»", "” ",
		"《", " “", "》", "” ", "「", " “", "」", "” ", "【", " “", "】", "” ",
		"（", " (", "）", ") ",
	)
	return strings.TrimSpace(replacer.Replace(text))
}

// segment 对文本进行分词（保持空白为 eng token）
func (f *zhFrontend) segment(text string) []wordPos {
	var out []wordPos
	for _, part := range splitKeepSpaces(text) {
		if strings.TrimSpace(part) == "" {
			out = append(out, wordPos{word: part, pos: "eng"})
			continue
		}
		for _, tag := range f.jieba.Tag(part) {
			word, pos := splitTag(tag)
			out = append(out, wordPos{word: word, pos: pos})
		}
	}
	return out
}

// splitKeepSpaces 按空白切分文本，保留空白片段
func splitKeepSpaces(text string) []string {
	var parts []string
	var cur strings.Builder
	curSpace := false
	flush := func() {
		if cur.Len() > 0 {
			parts = append(parts, cur.String())
			cur.Reset()
		}
	}
	for _, r := range text {
		isSpace := r == ' ' || r == '\t' || r == '\n' || r == '\r'
		if isSpace != curSpace && cur.Len() > 0 {
			flush()
		}
		curSpace = isSpace
		cur.WriteRune(r)
	}
	flush()
	return parts
}

// splitTag 解析 gojieba 的 "词/词性"
func splitTag(tag string) (string, string) {
	idx := strings.LastIndex(tag, "/")
	if idx < 0 {
		return tag, "eng"
	}
	return tag[:idx], tag[idx+1:]
}

// Call 中文文本 -> 注音音素串
func (f *zhFrontend) Call(text string) string {
	text = mapPunctuation(text)
	seg := f.segment(text)
	seg = f.sandhi.preMerge(seg)

	var tokens []zhToken
	for _, s := range seg {
		word, pos := s.word, s.pos
		// 全中文但被标为 x 的词按普通词处理（如人名/未登录词）
		if pos == "x" && isAllCJK(word) {
			pos = "X"
		}
		if pos == "x" || pos == "eng" {
			if strings.TrimSpace(word) != "" {
				if pos == "x" && isPuncWord(word, f.punc) {
					tokens = append(tokens, zhToken{phonemes: word, tag: pos})
				}
			} else if len(tokens) > 0 {
				tokens[len(tokens)-1].whitespace += word
			}
			continue
		}
		if len(tokens) > 0 && tokens[len(tokens)-1].tag != "x" && tokens[len(tokens)-1].tag != "eng" &&
			tokens[len(tokens)-1].whitespace == "" {
			tokens[len(tokens)-1].whitespace = "/"
		}
		initials, finals := getInitialsFinals(word)
		// 用户词典显式指定的读音保持精确，不再应用变调/儿化规则
		// （否则「目的地」的 地 会被词尾轻声化规则改成 de5）
		if _, inUserDict := pronunciationDict.Get(word); !inUserDict {
			finals = f.sandhi.modifiedTone(word, pos, finals)
			initials, finals = f.mergeErhua(initials, finals, word, pos)
		}
		phonemes := assemblePhonemes(initials, finals)
		tokens = append(tokens, zhToken{phonemes: phonemes, tag: pos})
	}

	var b strings.Builder
	for _, t := range tokens {
		b.WriteString(t.phonemes)
		b.WriteString(t.whitespace)
	}
	return b.String()
}

func isPuncWord(word string, punc map[string]struct{}) bool {
	if len(word) == 0 {
		return false
	}
	for _, r := range word {
		if _, ok := punc[string(r)]; !ok {
			return false
		}
	}
	return true
}

// isAllCJK 判断字符串是否全为汉字
func isAllCJK(s string) bool {
	if len(s) == 0 {
		return false
	}
	for _, r := range s {
		if r < 0x4E00 || r > 0x9FFF {
			return false
		}
	}
	return true
}

// mergeErhua 儿化处理（移植自 misaki ZHFrontend._merge_erhua）
func (f *zhFrontend) mergeErhua(initials, finals []string, word, pos string) ([]string, []string) {
	chars := []rune(word)
	if len(finals) == 0 {
		return initials, finals
	}
	// 修正 er1 -> er2
	if chars[len(chars)-1] == '儿' && finals[len(finals)-1] == "er1" {
		finals[len(finals)-1] = "er2"
	}
	if _, inMust := mustErhua[word]; !inMust {
		if _, inNot := notErhua[word]; inNot {
			return initials, finals
		}
		if pos == "a" || pos == "j" || pos == "nr" {
			return initials, finals
		}
	}
	if len(finals) != len(chars) {
		return initials, finals
	}
	var newI, newF []string
	for i, phn := range finals {
		if i == len(finals)-1 && chars[i] == '儿' && (phn == "er2" || phn == "er5") && len(newF) > 0 {
			suffix := ""
			if len(chars) >= 2 {
				suffix = string(chars[len(chars)-2:])
			}
			if _, inNot := notErhua[suffix]; inNot {
				newI = append(newI, initials[i])
				newF = append(newF, phn)
				continue
			}
			last := newF[len(newF)-1]
			newF[len(newF)-1] = last[:len(last)-1] + "R" + last[len(last)-1:]
		} else {
			newI = append(newI, initials[i])
			newF = append(newF, phn)
		}
	}
	return newI, newF
}
