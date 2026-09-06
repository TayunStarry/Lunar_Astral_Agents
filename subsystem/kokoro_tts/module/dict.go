package module

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"unicode/utf8"

	logger "LunarSubsystem/LoggerGeneral"
)

// PronunciationDict 用户读音词典（词语 -> 带调拼音，如 "行长": "hang2 zhang3"）
type PronunciationDict struct {
	// mu 保护词典并发读写
	mu sync.RWMutex
	// entries 词条映射
	entries map[string]string
	// path 词典文件路径
	path string
}

// LoadPronunciationDict 从 JSON 文件加载读音词典；文件不存在时返回空词典
func LoadPronunciationDict(path string) (*PronunciationDict, error) {
	d := &PronunciationDict{entries: make(map[string]string), path: path}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return d, nil
		}
		return nil, err
	}
	if err := json.Unmarshal(data, &d.entries); err != nil {
		logger.SubWarn("KOKORO-TTS", "DICT", "读音词典解析失败，使用空词典: %v", err)
		d.entries = make(map[string]string)
	}
	return d, nil
}

// Get 获取词语的拼音覆盖（返回归一化后的带调拼音音节列表）
func (d *PronunciationDict) Get(word string) ([]string, bool) {
	if d == nil {
		return nil, false
	}
	d.mu.RLock()
	defer d.mu.RUnlock()
	py, ok := d.entries[word]
	if !ok {
		return nil, false
	}
	parts := strings.Fields(py)
	if len(parts) == 0 {
		return nil, false
	}
	out := make([]string, len(parts))
	for i, p := range parts {
		out[i] = normalizePinyinSyllable(p)
	}
	return out, true
}

// Set 添加或更新词语读音；拼音音节数必须与字数一致
func (d *PronunciationDict) Set(word, pinyin string) error {
	if d == nil {
		return fmt.Errorf("读音词典未初始化")
	}
	word = strings.TrimSpace(word)
	if word == "" {
		return fmt.Errorf("词语不能为空")
	}
	syllables := strings.Fields(pinyin)
	if len(syllables) == 0 {
		return fmt.Errorf("拼音不能为空")
	}
	if len(syllables) != utf8.RuneCountInString(word) {
		return fmt.Errorf("拼音音节数(%d)与字数(%d)不一致", len(syllables), utf8.RuneCountInString(word))
	}
	normalized := make([]string, len(syllables))
	for i, s := range syllables {
		normalized[i] = normalizePinyinSyllable(s)
	}
	d.mu.Lock()
	d.entries[word] = strings.Join(normalized, " ")
	err := d.save()
	d.mu.Unlock()
	return err
}

// Delete 删除词语读音
func (d *PronunciationDict) Delete(word string) error {
	if d == nil {
		return fmt.Errorf("读音词典未初始化")
	}
	word = strings.TrimSpace(word)
	d.mu.Lock()
	defer d.mu.Unlock()
	if _, ok := d.entries[word]; !ok {
		return fmt.Errorf("词语 %s 不在词典中", word)
	}
	delete(d.entries, word)
	return d.save()
}

// All 返回全部词条（副本）
func (d *PronunciationDict) All() map[string]string {
	out := make(map[string]string)
	if d == nil {
		return out
	}
	d.mu.RLock()
	defer d.mu.RUnlock()
	for k, v := range d.entries {
		out[k] = v
	}
	return out
}

// save 原子写回词典文件
func (d *PronunciationDict) save() error {
	if d.path == "" {
		return nil
	}
	data, err := json.MarshalIndent(d.entries, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(d.path), 0755); err != nil {
		return err
	}
	tmp := d.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return err
	}
	return os.Rename(tmp, d.path)
}

// toneMarkMap 带调元音 -> (元音, 声调数字)
var toneMarkMap = map[rune]string{
	'ā': "a1", 'á': "a2", 'ǎ': "a3", 'à': "a4",
	'ē': "e1", 'é': "e2", 'ě': "e3", 'è': "e4",
	'ī': "i1", 'í': "i2", 'ǐ': "i3", 'ì': "i4",
	'ō': "o1", 'ó': "o2", 'ǒ': "o3", 'ò': "o4",
	'ū': "u1", 'ú': "u2", 'ǔ': "u3", 'ù': "u4",
	'ǖ': "v1", 'ǘ': "v2", 'ǚ': "v3", 'ǜ': "v4",
	'ü': "v", 'ê': "e",
}

// normalizePinyinSyllable 归一化拼音音节：转小写、声调符号转末尾数字、无调补 5（轻声）
func normalizePinyinSyllable(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	tone := byte(0)
	for _, r := range s {
		if mark, ok := toneMarkMap[r]; ok {
			// 带调元音：记录声调，写出元音
			if len(mark) == 2 {
				tone = mark[1]
			}
			b.WriteByte(mark[0])
			continue
		}
		b.WriteRune(r)
	}
	out := b.String()
	if tone != 0 {
		return out + string(tone)
	}
	// 无调：末尾有数字则保留，否则补 5
	if out != "" {
		last := out[len(out)-1]
		if last >= '1' && last <= '5' {
			return out
		}
	}
	return out + "5"
}

// zeroInitialReverse 零声母韵母 -> 完整拼音（y/w 声母）反向映射，用于读音查询展示
var zeroInitialReverse = map[string]string{
	"ia": "ya", "ie": "ye", "iao": "yao", "iou": "you", "ian": "yan",
	"iang": "yang", "iong": "yong", "in": "yin", "ing": "ying", "i": "yi",
	"v": "yu", "ve": "yue", "van": "yuan", "vn": "yun",
	"ua": "wa", "uo": "wo", "uai": "wai", "uei": "wei", "uan": "wan",
	"uen": "wen", "uang": "wang", "ueng": "weng", "u": "wu",
}

// reconstructPinyin 将声母/韵母列表还原为带调拼音（供 /dict/guess 展示当前读音）
func reconstructPinyin(initials, finals []string) []string {
	out := make([]string, len(finals))
	for i, f := range finals {
		c := ""
		if i < len(initials) {
			c = initials[i]
		}
		out[i] = syllableFromInitialFinal(c, f)
	}
	return out
}

// syllableFromInitialFinal 组合声母与带调韵母为拼音音节
func syllableFromInitialFinal(c, f string) string {
	if f == "" {
		return c
	}
	// 分离韵母主体与声调
	tone := f[len(f)-1]
	body := f
	if tone >= '1' && tone <= '5' {
		body = f[:len(f)-1]
	} else {
		tone = 0
	}
	// 舌尖元音 ii/iii 还原为 i
	switch body {
	case "ii":
		body = "i"
	case "iii":
		body = "i"
	}
	if c != "" {
		// 韵母归一化的反向还原（显示用）：uen/uei/iou/vn -> un/ui/iu/un
		switch body {
		case "uen":
			body = "un"
		case "uei":
			body = "ui"
		case "iou":
			body = "iu"
		case "vn":
			body = "un"
		}
		s := c + body
		if tone > 0 {
			s += string(tone)
		}
		return s
	}
	// 零声母：反向映射
	syl, ok := zeroInitialReverse[body]
	if !ok {
		syl = body
	}
	if tone > 0 {
		syl += string(tone)
	}
	return syl
}
