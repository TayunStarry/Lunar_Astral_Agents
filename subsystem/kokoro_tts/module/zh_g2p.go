package module

import (
	"strings"

	"github.com/mozillazg/go-pinyin"
)

// zhMap 拼音声母/韵母到注音符号（或特殊汉字）的映射，与 misaki ZHFrontend 一致
var zhMap = map[string]string{
	"b": "ㄅ", "p": "ㄆ", "m": "ㄇ", "f": "ㄈ", "d": "ㄉ", "t": "ㄊ",
	"n": "ㄋ", "l": "ㄌ", "g": "ㄍ", "k": "ㄎ", "h": "ㄏ", "j": "ㄐ",
	"q": "ㄑ", "x": "ㄒ", "zh": "ㄓ", "ch": "ㄔ", "sh": "ㄕ", "r": "ㄖ",
	"z": "ㄗ", "c": "ㄘ", "s": "ㄙ",
	"a": "ㄚ", "o": "ㄛ", "e": "ㄜ", "ie": "ㄝ", "ai": "ㄞ", "ei": "ㄟ",
	"ao": "ㄠ", "ou": "ㄡ", "an": "ㄢ", "en": "ㄣ", "ang": "ㄤ", "eng": "ㄥ",
	"er": "ㄦ", "i": "ㄧ", "u": "ㄨ", "v": "ㄩ", "ii": "ㄭ", "iii": "十",
	"ve": "月", "ia": "压", "ian": "言", "iang": "阳", "iao": "要", "in": "阴",
	"ing": "应", "iong": "用", "iou": "又", "ong": "中", "ua": "穵", "uai": "外",
	"uan": "万", "uang": "王", "uei": "为", "uen": "文", "ueng": "瓮", "uo": "我",
	"van": "元", "vn": "云",
}

// 标点与数字直接透传（词表内含）
func init() {
	for _, r := range ";:,.!?/—…\"()“” 12345R" {
		zhMap[string(r)] = string(r)
	}
}

// phraseOverride 词级拼音覆盖（多音字消歧，参考 misaki phrases_dict 与常见多音词）
var phraseOverride = map[string][]string{
	"开户行":  {"kai1", "hu4", "hang2"},
	"发卡行":  {"fa4", "ka3", "hang2"},
	"放款行":  {"fang4", "kuan3", "hang2"},
	"茧行":   {"jian3", "hang2"},
	"行号":   {"hang2", "hao4"},
	"各地":   {"ge4", "di4"},
	"借还款":  {"jie4", "huan2", "kuan3"},
	"时间为":  {"shi2", "jian1", "wei2"},
	"为准":   {"wei2", "zhun3"},
	"色差":   {"se4", "cha1"},
	"嗲":    {"dia3"},
	"呗":    {"bei5"},
	"不":    {"bu4"},
	"咗":    {"zuo5"},
	"嘞":    {"lei5"},
	"掺和":   {"chan1", "huo5"},
	"地":    {"de5"},
	"银行":   {"yin2", "hang2"},
	"行走":   {"xing2", "zou3"},
	"行长":   {"hang2", "zhang3"},
	"长大":   {"zhang3", "da4"},
	"长度":   {"chang2", "du4"},
	"音乐":   {"yin1", "yue4"},
	"快乐":   {"kuai4", "le4"},
	"觉得":   {"jue2", "de5"},
	"重要":   {"zhong4", "yao4"},
	"重新":   {"chong2", "xin1"},
	"朝阳":   {"zhao1", "yang2"},
	"学校":   {"xue2", "xiao4"},
	"首都":   {"shou3", "du1"},
	"都是":   {"dou1", "shi4"},
	"调节":   {"tiao2", "jie2"},
	"曲子":   {"qu3", "zi5"},
	"藏起来":  {"cang2", "qi3", "lai2"},
	"西藏":   {"xi1", "zang4"},
	"弹琴":   {"tan2", "qin2"},
	"子弹":   {"zi3", "dan4"},
	"单独":   {"dan1", "du2"},
	"血":    {"xue3"},
	"说客":   {"shui4", "ke4"},
	"了了":   {"liao3", "liao3"},
	"了得":   {"liao3", "de5"},
	"方便":   {"fang1", "bian4"},
	"便宜":   {"pian2", "yi2"},
	"长相":   {"chang2", "xiang4"},
	"相片":   {"xiang4", "pian4"},
	"着火":   {"zhao2", "huo3"},
	"着的":   {"zhe5", "de5"},
	"看着":   {"kan4", "zhe5"},
	"听着":   {"ting1", "zhe5"},
	"走着":   {"zou3", "zhe5"},
	"干什么":  {"gan4", "shen2", "me5"},
	"什么":   {"shen2", "me5"},
	"怎么":   {"zen3", "me5"},
	"这么":   {"zhe4", "me5"},
	"那么":   {"na4", "me5"},
	"没有":   {"mei2", "you3"},
	"没落":   {"mo4", "luo4"},
	"出差":   {"chu1", "chai1"},
	"差别":   {"cha1", "bie2"},
	"差不多":  {"cha4", "bu4", "duo1"},
	"处理":   {"chu3", "li3"},
	"到处":   {"dao4", "chu4"},
	"处长":   {"chu4", "zhang3"},
	"称呼":   {"cheng1", "hu5"},
	"称心":   {"chen4", "xin1"},
	"还书":   {"huan2", "shu1"},
	"还有":   {"hai2", "you3"},
	"归还":   {"gui1", "huan2"},
	"数数":   {"shu3", "shu4"},
	"数字":   {"shu4", "zi4"},
	"种地":   {"zhong4", "di4"},
	"种子":   {"zhong3", "zi5"},
	"睡觉":   {"shui4", "jiao4"},
	"觉得觉":  {"jue2", "de5", "jiao4"},
	"角色":   {"jue2", "se4"},
	"角落":   {"jiao3", "luo4"},
	"角度":   {"jiao3", "du4"},
	"应当":   {"ying1", "dang1"},
	"答应":   {"da1", "ying5"},
	"应用":   {"ying4", "yong4"},
	"应该":   {"ying1", "gai1"},
	"供应":   {"gong1", "ying4"},
	"结束":   {"jie2", "shu4"},
	"结实":   {"jie1", "shi5"},
	"头发":   {"tou2", "fa5"},
	"发现":   {"fa1", "xian4"},
	"出发":   {"chu1", "fa1"},
	"落下":   {"luo4", "xia4"},
	"落枕":   {"lao4", "zhen3"},
	"落色":   {"lao4", "shai3"},
	"杭州":   {"hang2", "zhou1"},
	"好恶":   {"hao4", "wu4"},
	"好事":   {"hao3", "shi4"},
	"爱好":   {"ai4", "hao4"},
	"乐趣":   {"le4", "qu4"},
	"快乐了":  {"kuai4", "le4", "le5"},
	"打了":   {"da3", "le5"},
	"打了打":  {"da3", "le5", "da3"},
	"转圈":   {"zhuan4", "quan1"},
	"转身":   {"zhuan3", "shen1"},
	"传达":   {"chuan2", "da2"},
	"传奇":   {"chuan2", "qi2"},
	"传记":   {"zhuan4", "ji4"},
	"重量":   {"zhong4", "liang4"},
	"重复":   {"chong2", "fu4"},
	"重要了":  {"zhong4", "yao4", "le5"},
	"中间":   {"zhong1", "jian1"},
	"命中":   {"ming4", "zhong4"},
	"空间":   {"kong1", "jian1"},
	"空地":   {"kong4", "di4"},
	"有空":   {"you3", "kong4"},
	"空白":   {"kong4", "bai2"},
	"困难":   {"kun4", "nan2"},
	"灾难":   {"zai1", "nan4"},
	"难民":   {"nan4", "min2"},
	"难过":   {"nan2", "guo4"},
	"难兄难弟": {"nan2", "xiong1", "nan2", "di4"},
	"埋头":   {"mai2", "tou2"},
	"埋没":   {"mai2", "mo4"},
	"埋怨":   {"man2", "yuan4"},
	"磨刀":   {"mo2", "dao1"},
	"磨面":   {"mo4", "mian4"},
	"安静":   {"an1", "jing4"},
	"宁可":   {"ning4", "ke3"},
	"宁静":   {"ning2", "jing4"},
	"宁愿":   {"ning4", "yuan4"},
	"浸泡":   {"jin4", "pao4"},
	"泡茶":   {"pao4", "cha2"},
	"气泡":   {"qi4", "pao4"},
	"坚强":   {"jian1", "qiang2"},
	"强迫":   {"qiang3", "po4"},
	"勉强":   {"mian3", "qiang3"},
	"任":    {"ren4"},
	"任何":   {"ren4", "he2"},
	"姓任":   {"xing4", "ren4"},
	"堵塞":   {"du3", "se4"},
	"塞子":   {"sai1", "zi5"},
	"边塞":   {"bian1", "sai4"},
	"塞北":   {"sai4", "bei3"},
	"散布":   {"san4", "bu4"},
	"散文":   {"san3", "wen2"},
	"松散":   {"song1", "san3"},
	"散步":   {"san4", "bu4"},
	"扫帚":   {"sao4", "zhou5"},
	"扫地":   {"sao3", "di4"},
	"打扫":   {"da3", "sao3"},
	"少数":   {"shao3", "shu4"},
	"少年":   {"shao4", "nian2"},
	"多少":   {"duo1", "shao3"},
	"舍弃":   {"she3", "qi4"},
	"宿舍":   {"su4", "she4"},
	"舍身":   {"she3", "shen1"},
	"兴盛":   {"xing1", "sheng4"},
	"盛饭":   {"cheng2", "fan4"},
	"盛开":   {"sheng4", "kai1"},
	"什":    {"shen2"},
	"什么了":  {"shen2", "me5", "le5"},
	"似的":   {"shi4", "de5"},
	"相似":   {"xiang1", "si4"},
	"似的了":  {"shi4", "de5", "le5"},
	"适应":   {"shi4", "ying4"},
	"宿":    {"su4"},
	"住宿":   {"zhu4", "su4"},
	"一宿":   {"yi1", "xiu3"},
	"挑担":   {"tiao1", "dan4"},
	"挑拨":   {"tiao3", "bo1"},
	"挑选":   {"tiao1", "xuan3"},
	"挑战":   {"tiao3", "zhan4"},
	"帖子":   {"tie3", "zi5"},
	"贴纸":   {"tie1", "zhi3"},
	"请帖":   {"qing3", "tie3"},
	"呕吐":   {"ou3", "tu4"},
	"吐痰":   {"tu3", "tan2"},
	"吐露":   {"tu3", "lu4"},
	"开拓":   {"kai1", "tuo4"},
	"拓片":   {"ta4", "pian4"},
	"咽喉":   {"yan1", "hou2"},
	"咽下":   {"yan4", "xia4"},
	"哽咽":   {"geng3", "ye4"},
	"要塞":   {"yao4", "sai4"},
	"要":    {"yao4"},
	"要了":   {"yao4", "le5"},
	"晕车":   {"yun4", "che1"},
	"晕倒":   {"yun1", "dao3"},
	"头晕":   {"tou2", "yun1"},
	"扎实":   {"zha1", "shi5"},
	"扎针":   {"zha1", "zhen1"},
	"挣扎":   {"zheng1", "zha2"},
	"驻扎":   {"zhu4", "zha1"},
	"粘贴":   {"zhan1", "tie1"},
	"粘稠":   {"nian2", "chou2"},
	"粘贴了":  {"zhan1", "tie1", "le5"},
	"涨价":   {"zhang3", "jia4"},
	"涨潮":   {"zhang3", "chao2"},
	"涨红":   {"zhang4", "hong2"},
	"头涨":   {"tou2", "zhang4"},
	"折腾":   {"zhe1", "teng5"},
	"折断":   {"zhe2", "duan4"},
	"折本":   {"she2", "ben3"},
	"折腾了":  {"zhe1", "teng5", "le5"},
	"正确":   {"zheng4", "que4"},
	"正月":   {"zheng1", "yue4"},
	"正中":   {"zheng4", "zhong1"},
	"只是":   {"zhi3", "shi4"},
	"一只":   {"yi1", "zhi1"},
	"只得":   {"zhi3", "de5"},
	"转过":   {"zhuan3", "guo4"},
	"转动":   {"zhuan4", "dong4"},
	"转达":   {"zhuan3", "da2"},
	"转圈儿":  {"zhuan4", "quan1", "r5"},
	"占卜":   {"zhan1", "bu3"},
	"占地":   {"zhan4", "di4"},
	"占便宜":  {"zhan4", "pian2", "yi2"},
	"曾经":   {"ceng2", "jing1"},
	"曾孙":   {"zeng1", "sun1"},
	"姓曾":   {"xing4", "zeng1"},
	"冲着":   {"chong4", "zhe5"},
	"冲劲":   {"chong4", "jin4"},
	"冲刷":   {"chong1", "shua1"},
	"重逢":   {"chong2", "feng2"},
	"行为":   {"xing2", "wei2"},
	"行动":   {"xing2", "dong4"},
	"行业":   {"hang2", "ye4"},
	"硬行":   {"ying4", "xing2"},
	"协调":   {"xie2", "tiao2"},
	"调换":   {"diao4", "huan4"},
	"调整":   {"tiao2", "zheng3"},
	"强调":   {"qiang2", "diao4"},
	"了解":   {"liao3", "jie3"},
	"了却":   {"liao3", "que4"},
	"了解过":  {"liao3", "jie3", "guo4"},
	"切菜":   {"qie1", "cai4"},
	"亲切":   {"qin1", "qie4"},
	"切开":   {"qie1", "kai1"},
	"切切":   {"qie4", "qie4"},
	"散":    {"san4"},
	"散会":   {"san4", "hui4"},
	"散落":   {"san4", "luo4"},
	"骨":    {"gu3"},
	"骨头":   {"gu3", "tou5"},
	"骨干":   {"gu3", "gan4"},
	"骨碌":   {"gu1", "lu5"},
	"骨碌碌":  {"gu1", "lu5", "lu5"},
	"滑头":   {"hua2", "tou2"},
	"滑稽":   {"hua2", "ji1"},
	"露":    {"lu4"},
	"露水":   {"lu4", "shui3"},
	"露脸":   {"lou4", "lian3"},
	"露馅":   {"lou4", "xian4"},
	"露珠":   {"lu4", "zhu1"},
	"露了":   {"lou4", "le5"},
	"落叶":   {"luo4", "ye4"},
	"落下来":  {"luo4", "xia4", "lai2"},
	"乐":    {"le4"},
	"乐队":   {"yue4", "dui4"},
	"乐器":   {"yue4", "qi4"},
	"乐呵":   {"le4", "he5"},
	"乐山":   {"le4", "shan1"},
	"和为贵":  {"he2", "wei2", "gui4"},
	"和面":   {"huo2", "mian4"},
	"和诗":   {"he4", "shi1"},
	"暖和":   {"nuan3", "huo5"},
	"暖和了":  {"nuan3", "huo5", "le5"},
	"停当":   {"ting2", "dang5"},
	"当铺":   {"dang4", "pu4"},
	"当时":   {"dang1", "shi2"},
	"上当":   {"shang4", "dang4"},
	"当天":   {"dang4", "tian1"},
	"当年":   {"dang1", "nian2"},
	"当年了":  {"dang1", "nian2", "le5"},
	"中国":   {"zhong1", "guo2"},
	"中文":   {"zhong1", "wen2"},
	"中间人":  {"zhong1", "jian1", "ren2"},
	"看中":   {"kan4", "zhong4"},
	"中弹":   {"zhong4", "dan4"},
	"击中":   {"ji1", "zhong4"},
	"命中率":  {"ming4", "zhong4", "lv4"},
	"看":    {"kan4"},
	"看守":   {"kan1", "shou3"},
	"看守所":  {"kan1", "shou3", "suo3"},
	"看望":   {"kan4", "wang4"},
	"看见":   {"kan4", "jian4"},
	"看护":   {"kan1", "hu4"},
	"看到":   {"kan4", "dao4"},
	"提":    {"ti2"},
	"提防":   {"di1", "fang5"},
	"提醒":   {"ti2", "xing3"},
	"提早":   {"ti2", "zao3"},
	"提了":   {"ti2", "le5"},
	"都":    {"dou1"},
	"都督":   {"du1", "du1"},
	"都城":   {"du1", "cheng2"},
	"都市":   {"du1", "shi4"},
	"都能":   {"dou1", "neng2"},
	"都是了":  {"dou1", "shi4", "le5"},
	"还":    {"hai2"},
	"还是":   {"hai2", "shi4"},
	"还行":   {"hai2", "xing2"},
	"还是要":  {"hai2", "shi4", "yao4"},
	"得":    {"de5"},
	"得到":   {"de2", "dao4"},
	"得亏":   {"dei3", "kui1"},
	"得去":   {"dei3", "qu4"},
	"来得及":  {"lai2", "de2", "ji2"},
	"觉得很":  {"jue2", "de5", "hen3"},
	"的":    {"de5"},
	"的确":   {"di2", "que4"},
	"的话":   {"de5", "hua4"},
	"的目标":  {"de5", "mu4", "biao1"},
	"行":    {"xing2"},
	"行了":   {"xing2", "le5"},
	"行吗":   {"xing2", "ma5"},
	"行走着":  {"xing2", "zou3", "zhe5"},
	"旅行的":  {"lv3", "xing2", "de5"},
	"飞行":   {"fei1", "xing2"},
	"可以":   {"ke3", "yi3"},
	"以为":   {"yi3", "wei2"},
	"因为":   {"yin1", "wei4"},
	"为了":   {"wei4", "le5"},
	"成为":   {"cheng2", "wei2"},
	"因为了":  {"yin1", "wei4", "le5"},
	"可以了":  {"ke3", "yi3", "le5"},
	"以一":   {"yi3", "yi1"},
	"一起":   {"yi1", "qi3"},
	"一百":   {"yi1", "bai3"},
	"一月":   {"yi1", "yue4"},
	"不要":   {"bu4", "yao4"},
	"不能":   {"bu4", "neng2"},
	"不怕":   {"bu2", "pa4"},
	"不是":   {"bu4", "shi4"},
	"不对":   {"bu2", "dui4"},
	"不安":   {"bu4", "an1"},
	"不三不四": {"bu4", "san1", "bu2", "si4"},
	"不足":   {"bu4", "zu2"},
	"不好":   {"bu4", "hao3"},
	"不错":   {"bu2", "cuo4"},
	"不断":   {"bu2", "duan4"},
	"不要了":  {"bu4", "yao4", "le5"},
	"不用":   {"bu2", "yong4"},
	"不过":   {"bu2", "guo4"},
	"不到":   {"bu2", "dao4"},
	"不必":   {"bu2", "bi4"},
	"不一会儿": {"bu4", "yi1", "hui2", "er5"},
}

// pinyinArgs 声母与带调韵母的 go-pinyin 参数
var (
	pinyinArgsInitials    = pinyin.Args{Style: pinyin.Initials, Heteronym: false}
	pinyinArgsFinalsTone3 = pinyin.Args{Style: pinyin.FinalsTone3, Heteronym: false}
)

// initialsOrder 用于从完整拼音切分声母
var initialsOrder = []string{"zh", "ch", "sh", "b", "p", "m", "f", "d", "t", "n", "l", "g", "k", "h", "j", "q", "x", "r", "z", "c", "s"}

// zeroInitialFinals y/w 开头的零声母音节到韵母的映射（与 zhMap 一致）
var zeroInitialFinals = map[string]string{
	"ya": "ia", "ye": "ie", "yao": "iao", "you": "iou", "yan": "ian",
	"yang": "iang", "yong": "iong", "yin": "in", "ying": "ing", "yi": "i",
	"yu": "v", "yue": "ve", "yuan": "van", "yun": "vn",
	"wa": "ua", "wo": "uo", "wai": "uai", "wei": "uei", "wan": "uan",
	"wen": "uen", "wang": "uang", "weng": "ueng", "wu": "u",
}

// splitPinyin 将完整拼音（含声调数字）切分为声母与带调韵母
// 覆盖词使用 pypinyin 全拼格式，y/w 开头音节映射为对应零声母韵母
func splitPinyin(py string) (string, string) {
	if len(py) < 2 {
		return "", py
	}
	tone := py[len(py)-1]
	body := py[:len(py)-1]
	// y/w 零声母音节
	if body[0] == 'y' || body[0] == 'w' {
		if fin, ok := zeroInitialFinals[body]; ok {
			return "", fin + string(tone)
		}
	}
	for _, ini := range initialsOrder {
		if strings.HasPrefix(body, ini) && len(body) > len(ini) {
			return ini, body[len(ini):] + string(tone)
		}
	}
	return "", py
}

// getInitialsFinals 获取词语每个字的声母与带调韵母（含多音词覆盖与舌尖元音区分）
func getInitialsFinals(word string) ([]string, []string) {
	runes := []rune(word)
	n := len(runes)
	initials := make([]string, n)
	finals := make([]string, n)

	// 用户词典优先，其次内置多音字覆盖
	pys, ok := pronunciationDict.Get(word)
	if !ok {
		pys, ok = phraseOverride[word]
	}
	if ok {
		for i, py := range pys {
			if i >= n {
				break
			}
			c, v := splitPinyin(py)
			initials[i], finals[i] = fixApical(c, v)
		}
		return initials, finals
	}

	pi := pinyin.Pinyin(word, pinyinArgsInitials)
	pf := pinyin.Pinyin(word, pinyinArgsFinalsTone3)
	for i, r := range runes {
		c := ""
		if i < len(pi) && len(pi[i]) > 0 {
			c = pi[i][0]
		}
		v := ""
		if i < len(pf) && len(pf[i]) > 0 {
			v = pf[i][0]
		}
		// 轻声（无数字或 0）统一为 5
		if v == "" {
			// 无韵母（孤立字符），保留空
		} else if v[len(v)-1] < '1' || v[len(v)-1] > '5' {
			v += "5"
		}
		// 嗯 特殊处理（pypinyin 0.44+ 行为）
		if r == '嗯' {
			c = ""
			v = "n2"
		}
		c, v = fixApical(c, v)
		initials[i], finals[i] = c, v
	}
	return initials, finals
}

// finalsNormalize 韵母归一化（un/ui/iu -> uen/uei/iou，与 pypinyin 韵母表一致）
var finalsNormalize = map[string]string{"un": "uen", "ui": "uei", "iu": "iou"}

// fixApical 韵母归一化（un/ui/iu -> uen/uei/iou）并区分舌尖元音 zi/ci/si（ii）与 zhi/chi/shi/ri（iii）
// 注：j/q/x 后的 un 实为 ün（军/群/训），应归一化为 vn 而非 uen
func fixApical(c, v string) (string, string) {
	if len(v) >= 2 {
		if fin, ok := finalsNormalize[v[:2]]; ok && (len(v) == 2 || isToneDigits(v[2:])) {
			if v[:2] == "un" && (c == "j" || c == "q" || c == "x") {
				v = "vn" + v[2:]
			} else {
				v = fin + v[2:]
			}
		}
	}
	if len(v) >= 2 && v[0] == 'i' && isToneDigits(v[1:]) {
		switch c {
		case "z", "s", "c":
			v = strings.Replace(v, "i", "ii", 1)
		case "zh", "ch", "sh", "r":
			v = strings.Replace(v, "i", "iii", 1)
		}
	}
	return c, v
}

// isToneDigits 判断字符串是否为 1~5 数字
func isToneDigits(s string) bool {
	if len(s) != 1 || s < "1" || s > "5" {
		return false
	}
	return true
}

// assemblePhonemes 将声母/韵母组合为注音音素串
func assemblePhonemes(initials, finals []string) string {
	var phones []string
	for i := range initials {
		if i < len(initials) && initials[i] != "" {
			phones = append(phones, initials[i])
		}
		if i < len(finals) && finals[i] != "" {
			phones = append(phones, finals[i])
		}
	}
	s := strings.Join(phones, "_")
	s = strings.ReplaceAll(s, "_eR", "_er")
	s = strings.ReplaceAll(s, "R", "_R")
	// 在数字前插入分隔符
	var b strings.Builder
	for _, ch := range s {
		if ch >= '0' && ch <= '9' {
			b.WriteByte('_')
		}
		b.WriteRune(ch)
	}
	var out strings.Builder
	for _, p := range strings.Split(b.String(), "_") {
		if p == "" {
			continue
		}
		if m, ok := zhMap[p]; ok {
			out.WriteString(m)
		} else {
			out.WriteRune('❓')
		}
	}
	return out.String()
}
