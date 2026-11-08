package creation

// plan_catalog.go 预设创作事项列表：类型 + 参数池。
//
// 事项参数全部来自本文件的预设池，由本地随机组合生成（不依赖模型先决定做什么），
// 保证每次抽到的事项都具体到可直接执行。

// researchEvent 一个可研究的特定事件：全名用于指令，简称用于活动文案。
type researchEvent struct {
	Name  string
	Short string
}

var (
	// novelThemes 小说题材池
	novelThemes = []string{"科幻悬疑", "都市奇幻", "武侠江湖", "校园日常", "赛博朋克", "蒸汽朋克", "末日生存", "东方仙侠"}
	// novelLengths 小说篇幅倾向池
	novelLengths = []string{"3000 字左右的短篇", "8000 字左右的中篇", "分章推进的长篇开篇（完成第一章，约 2500 字）"}
	// novelVoices 叙事视角池
	novelVoices = []string{"第一人称", "第三人称限制视角", "双线交替视角"}

	// researchEvents 具体事件池
	researchEvents = []researchEvent{
		{Name: "1986 年切尔诺贝利核事故的经过与后续影响", Short: "切尔诺贝利核事故"},
		{Name: "1908 年通古斯大爆炸的成因争议", Short: "通古斯大爆炸"},
		{Name: "2016 年 AlphaGo 与李世石的五番棋", Short: "AlphaGo 人机大战"},
		{Name: "2020 年嫦娥五号月球采样返回任务", Short: "嫦娥五号任务"},
		{Name: "1918 年大流感在全球的传播", Short: "1918 年大流感"},
		{Name: "2023 年 LK-99 室温超导争议", Short: "LK-99 室温超导争议"},
		{Name: "北宋庆历年间活字印刷的推广", Short: "活字印刷的推广"},
		{Name: "2011 年日本福岛核事故的应急处置", Short: "福岛核事故"},
	}
	// researchAngles 研究角度池
	researchAngles = []string{"时间线梳理与关键节点", "成因与主要争议", "对行业与社会的后续影响", "关键人物与技术细节"}

	// projectTopics 小项目方向池
	projectTopics = []string{"待办清单", "番茄钟", "贪吃蛇", "天气卡片", "像素画板", "打字练习", "简易计算器", "本地倒计时"}
	// projectLayouts 小项目技术形态池
	projectLayouts = []string{"单个 index.html（CSS 与 JS 全部内联）", "index.html + style.css + app.js 三个文件"}

	// storyTones 小故事基调池
	storyTones = []string{"温暖治愈", "黑色幽默", "微恐悬疑", "荒诞讽刺", "童话寓言"}
	// storyLengths 小故事篇幅池
	storyLengths = []string{"800 字左右", "1500 字左右"}
	// storyLeads 小故事主角池
	storyLeads = []string{"一只会说话的猫", "深夜便利店的店员", "迷路的邮差", "退休的钟表匠", "城市里的路灯"}

	// tidyScopes 整理范围池（相对 /document 的具体子目录）
	tidyScopes = []string{"notes", "创作", "研究", "项目", "temp"}
	// tidyActions 整理动作池
	tidyActions = []string{
		"按主题建立子目录并用 file_move 归类可以明确判断的文件",
		"把内容重复的片段合并成一份汇总，并在 README 中标注来源",
		"用 file_move 统一重命名同类文件，让它们按主题排在一起",
		"生成一份目录索引，列出每个文件的用途与最近改动",
	}

	// jokeStyles 笑话风格池
	jokeStyles = []string{"冷笑话", "谐音梗", "程序员笑话", "动物拟人", "日常反差"}
)

// itemSpecs 预设创作事项列表；顺序即随机抽取的候选范围。
var itemSpecs = []itemSpec{
	{Kind: KindNovel, Label: "写小说", Build: buildNovel},
	{Kind: KindResearch, Label: "分析研究特定事件", Build: buildResearch},
	{Kind: KindProject, Label: "编写 JS / HTML 小项目", Build: buildProject},
	{Kind: KindStory, Label: "创作小故事", Build: buildStory},
	{Kind: KindTidy, Label: "整理本地文件夹", Build: buildTidy},
	{Kind: KindJoke, Label: "向 QQ 推送小笑话", NeedsQQ: true, Build: buildJoke},
	{Kind: KindPaint, Label: "自主绘制作画", Build: buildPaint},
}

// kindLabel 返回事项类型的中文名。
func kindLabel(kind ItemKind) string {
	for _, spec := range itemSpecs {
		if spec.Kind == kind {
			return spec.Label
		}
	}
	return string(kind)
}
