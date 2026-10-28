package BilibiliViewer

// risk.go 视频风险筛查规则：内置风险词、评论区规则扫描与复核材料。
//
// 三层判据，宁可错杀不可放过：
//  1) 内置风险词（涉政、对立、违法低俗、引流等）命中即判定有风险；
//  2) 评论区出现风险词或引流/联系方式模式，同样判定有风险；
//  3) 交给多模态模型结合封面与文字材料复核（结构化审核见 review.go）。
//
// 命中风险的视频不评论、不分享、不进日记动态；选片阶段命中则直接换一条。

import (
	"fmt"
	"regexp"
	"strings"
)

// riskVerdict 风险筛查结论
type riskVerdict struct {
	Risky  bool
	Reason string
}

const (
	// riskRejectQuotaFactor 单轮允许的风险拒选次数相对观看额度的倍数：
	// 避免风险筛查与推荐池互相拉扯造成无限选片循环。
	riskRejectQuotaFactor = 3
	// riskCommentSample 交给复核模型的评论条数
	riskCommentSample = 10
)

// riskKeywords 内置风险词：宁可错杀，出现即不留言
var riskKeywords = []string{
	// 低价值内容（旧过滤项内化）
	"直播回放", "录播", "付费", "充电专属",
	// 涉政与国家安全
	"政治", "时政", "涉政", "意识形态", "中共", "共产党", "政府", "主席", "总统",
	"选举", "两会", "人大", "政协", "外交", "中美", "俄乌", "乌克兰", "俄罗斯",
	"巴以", "以色列", "巴勒斯坦", "台海", "台湾", "香港", "新疆", "西藏",
	"南海", "钓鱼岛", "领土", "战争", "制裁", "军事", "军队", "政变",
	"抗议", "游行", "示威", "民主", "专制", "独裁", "社会主义", "资本主义",
	"左派", "右派", "粉红", "公知", "殖人", "润人", "汉奸", "卖国",
	"爱国", "辱华", "抵制", "封杀", "六四", "法轮", "台独", "港独", "藏独",
	"疆独", "反共", "反华", "颠覆", "分裂国家", "恐怖主义", "极端主义",
	// 社会对立与引战
	"男女对立", "性别对立", "女权", "男权", "打拳", "拳师", "彩礼", "婚恋",
	"婆媳", "阶级", "贫富", "资本家", "对立", "引战", "对线", "挂人",
	"网暴", "撕逼", "骂战", "饭圈", "粉丝互撕", "地域黑", "地图炮",
	"种族", "民族", "宗教", "邪教", "传教",
	// 违法违规与低俗
	"色情", "裸", "约炮", "嫖娼", "卖淫", "赌博", "博彩", "彩票",
	"毒品", "冰毒", "大麻", "诈骗", "洗钱", "传销", "枪支", "血腥",
	"虐杀", "恐怖袭击", "杀人", "自杀", "自残", "人肉", "开盒", "违法", "犯罪",
}

// riskPatterns 引流与联系方式等模式化风险
var riskPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)(加\s*(微信|vx|v信|qq)|扫码|进群|私聊|电报|telegram|whatsapp)`),
	regexp.MustCompile(`(?i)q{2}\s*[:：]?\s*\d{5,}`),
	regexp.MustCompile(`1[3-9]\d{9}`),
}

// riskRejectLimit 本轮允许的风险拒选次数（候选数 × 倍数）
func riskRejectLimit(candidates int) int {
	if candidates < candidateMin {
		candidates = candidateMin
	}
	return candidates * riskRejectQuotaFactor
}

// screenCommentsRisk 读完评论区后的复查：只做规则扫描，命中即不再留言
func screenCommentsRisk(comments []commentItem) riskVerdict {
	verdict, hit := ruleRiskComments(comments)
	if !hit {
		return riskVerdict{}
	}
	return verdict
}

// ruleRisk 风险词与模式扫描
func ruleRisk(text string) (riskVerdict, bool) {
	lower := strings.ToLower(text)
	for _, word := range riskKeywords {
		if strings.Contains(lower, strings.ToLower(word)) {
			return riskVerdict{Risky: true, Reason: "命中风险词「" + word + "」"}, true
		}
	}
	for _, pattern := range riskPatterns {
		if pattern.MatchString(text) {
			return riskVerdict{Risky: true, Reason: "命中引流/联系方式模式"}, true
		}
	}
	return riskVerdict{}, false
}

// ruleRiskComments 评论区规则扫描：任一评论命中即按风险处理
func ruleRiskComments(comments []commentItem) (riskVerdict, bool) {
	for _, comment := range comments {
		if verdict, hit := ruleRisk(comment.Content); hit {
			return riskVerdict{Risky: true, Reason: "评论区" + verdict.Reason}, true
		}
	}
	return riskVerdict{}, false
}

// riskMaterial 复核材料：标题、UP 主、简介与评论节选
func riskMaterial(info videoInfo, comments []commentItem) string {
	var builder strings.Builder
	fmt.Fprintf(&builder, "标题：《%s》\nUP主：%s\n", info.Title, info.Owner)
	if desc := strings.TrimSpace(info.Desc); desc != "" {
		fmt.Fprintf(&builder, "简介：%s\n", truncate(desc, 600))
	}
	for index, comment := range comments {
		if index >= riskCommentSample {
			break
		}
		fmt.Fprintf(&builder, "评论：%s：%s\n", comment.User, truncate(comment.Content, 120))
	}
	return strings.TrimSpace(builder.String())
}
