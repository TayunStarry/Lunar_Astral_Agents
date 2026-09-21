// ============================================================
// 『 星月智能 』图表工坊 — 图表样式参考库（提示词库 / 技能库）
// 智能体在正式输出图表内容前，必须先声明【技术格式】与【图表类型】；
// 本库根据声明检索对应的样式参考（排版布局 / 色彩搭配 / 专业规范），
// 由 ltpx-agent.js 将参考资料完整载入智能体上下文，指导图表输出。
// 每条参考 = { id, name, format, match[], guide, skeleton }：
//   format   - 'echarts' | 'mermaid' | 'any'（通用规范）
//   match    - 类型声明关键词（命中得分 = 关键词长度，取最高分条目）
//   guide    - 注入上下文的文字规范（讲「为什么」）
//   skeleton - 样式骨架示例：只含样式字段与格式结构的合法 JSON / Mermaid 文本，
//              数据一律占位（讲「照什么做」；构建时必须替换为真实数据）
// ============================================================

// ---- 共享色彩体系（各参考引用，保证全库配色和谐统一） ----
const CS_PALETTE_NOTES = [
    '【共享色彩体系】',
    '- 和谐主色板（多系列首选，Tableau 10）：#4E79A7 #F28E2B #E15759 #76B7B2 #59A14F #EDC948 #B07AA1 #FF9DA7 #9C755F #BAB0AC',
    '- 语义色：成功 #2E8B6E / 警告 #F39C12 / 危险 #E15759；强调 #4E79A7',
    '- 文字层级：标题 #1F2937（15px 半粗）/ 轴标签与图例 #6B7280（12px）/ 辅助注记 #9CA3AF（11px）',
    '- 网格线与分隔：#E5E7EB 虚线（dashed），坐标轴线 #D1D5DB，禁用轴刻度小突起（axisTick.show:false）',
    '- 背景：不设置 backgroundColor，透出玻璃容器底色；除面积/环形的淡渐变外禁用装饰性渐变'
].join('\n');

// ---- 共享骨架片段（各骨架复用，保持全库格式一致） ----
const CS_SKELETON_TITLE_LEGEND = [
    '  "title": { "text": "图表标题", "left": 0, "top": 0, "textStyle": { "fontSize": 15, "fontWeight": 600, "color": "#1F2937" } },',
    '  "legend": { "top": 0, "right": 0, "icon": "circle", "itemWidth": 8, "itemGap": 16, "textStyle": { "color": "#6B7280", "fontSize": 12 } },',
    '  "tooltip": { "trigger": "axis", "axisPointer": { "type": "shadow" } },',
    '  "grid": { "left": 16, "right": 28, "top": 56, "bottom": 12, "containLabel": true },'
].join('\n');
const CS_SKELETON_XAXIS_CAT = [
    '  "xAxis": { "type": "category", "data": ["类目1", "类目2", "类目3"], "axisTick": { "show": false }, "axisLine": { "lineStyle": { "color": "#D1D5DB" } }, "axisLabel": { "color": "#6B7280" } },',
    '  "yAxis": { "type": "value", "splitLine": { "lineStyle": { "color": "#E5E7EB", "type": "dashed" } }, "axisLabel": { "color": "#6B7280" } },'
].join('\n');

const CHART_STYLE_LIBRARY = [
    // ============ ECharts 数据图表 ============
    {
        id: 'echarts-line', name: '折线图专业规范', format: 'echarts',
        match: ['折线', '趋势', '走势', '变化', '曲线', 'line', 'trend'],
        guide: [
            '【折线图专业规范】',
            CS_PALETTE_NOTES,
            '',
            '【排版布局】',
            '- grid：{ left:16, right:28, top:56, bottom:12, containLabel:true }，标题左上、图例右上（legend.top:0, right:0, icon:"circle", itemWidth:8）',
            '- xAxis：boundaryGap:false（贴边起线）；类目多时 axisLabel 自动抽稀（interval:"auto"），>30 点用 dataZoom（type:"slider", height:16, bottom:4）',
            '- yAxis：scale:true（不从 0 起线，突出趋势）；splitLine:{ lineStyle:{ color:"#E5E7EB", type:"dashed" } }',
            '',
            '【系列样式】',
            '- lineStyle:{ width:2.5 }；smooth:true 或 0.4（适度圆滑，禁用阶梯感过强的转折）',
            '- 点密集（>20 点）时 showSymbol:false + emphasis 聚焦显示；稀疏时 symbol:"circle", symbolSize:7',
            '- 单系列可在线下加淡渐变面积：areaStyle:{ opacity:0.12, color:线性渐变[主色#4E79A7, 透明] }',
            '- 多系列 ≤4 条；>4 条建议拆分图或用图例交互，禁止 >6 条',
            '',
            '【标注增强】',
            '- 关键拐点用 markPoint（pin，symbolSize:44）；目标线/均值线用 markLine（dashed，#E15759），标签置顶',
            '- tooltip：trigger:"axis", axisPointer:{ type:"line", lineStyle:{ color:"#94A3B8" } }'
        ].join('\n'),
        skeleton: [
            '{',
            CS_SKELETON_TITLE_LEGEND,
            '  "xAxis": { "type": "category", "boundaryGap": false, "data": ["类目1", "类目2", "类目3"], "axisTick": { "show": false }, "axisLine": { "lineStyle": { "color": "#D1D5DB" } }, "axisLabel": { "color": "#6B7280" } },',
            '  "yAxis": { "type": "value", "scale": true, "splitLine": { "lineStyle": { "color": "#E5E7EB", "type": "dashed" } }, "axisLabel": { "color": "#6B7280" } },',
            '  "series": [',
            '    { "name": "系列A", "type": "line", "smooth": true, "lineStyle": { "width": 2.5 }, "symbol": "circle", "symbolSize": 7, "data": [10, 25, 16] },',
            '    { "name": "系列B", "type": "line", "smooth": true, "lineStyle": { "width": 2.5 }, "symbol": "circle", "symbolSize": 7, "data": [8, 18, 22] }',
            '  ]',
            '}'
        ].join('\n')
    },
    {
        id: 'echarts-area', name: '面积图专业规范', format: 'echarts',
        match: ['面积', '堆叠趋势', '占比趋势', 'area'],
        guide: [
            '【面积图专业规范】',
            CS_PALETTE_NOTES,
            '',
            '【排版布局】',
            '- 与折线图同基准（grid/图例/轴样式一致）；多系列堆叠用 stack:"total" 且各系列面积透明度递减',
            '',
            '【系列样式】',
            '- 面积渐变：color: { type:"linear", x:0, y:0, x2:0, y2:1, colorStops:[主色 opacity 0.28 → opacity 0.02] }，禁止实色填充',
            '- 顶层系列 lineStyle width 2.5；堆叠时下方系列线宽 1.5 弱化',
            '- 堆叠面积总数 ≤4 层，层数更多改堆叠柱状图',
            '',
            '【注意事项】',
            '- 堆叠面积只表达总量与构成趋势，禁止读者误读为精确占比（占比诉求改环形图）'
        ].join('\n'),
        skeleton: [
            '{',
            CS_SKELETON_TITLE_LEGEND,
            '  "xAxis": { "type": "category", "boundaryGap": false, "data": ["类目1", "类目2", "类目3"], "axisTick": { "show": false }, "axisLine": { "lineStyle": { "color": "#D1D5DB" } }, "axisLabel": { "color": "#6B7280" } },',
            '  "yAxis": { "type": "value", "splitLine": { "lineStyle": { "color": "#E5E7EB", "type": "dashed" } }, "axisLabel": { "color": "#6B7280" } },',
            '  "series": [',
            '    { "name": "系列A", "type": "line", "stack": "total", "smooth": true, "lineStyle": { "width": 2.5 }, "symbol": "none",',
            '      "areaStyle": { "color": { "type": "linear", "x": 0, "y": 0, "x2": 0, "y2": 1, "colorStops": [ { "offset": 0, "color": "rgba(78,121,167,0.28)" }, { "offset": 1, "color": "rgba(78,121,167,0.02)" } ] } },',
            '      "data": [10, 25, 16] }',
            '  ]',
            '}'
        ].join('\n')
    },
    {
        id: 'echarts-bar', name: '柱状图专业规范', format: 'echarts',
        match: ['柱状', '柱形', '条形', '直方', '对比柱', 'bar', 'column'],
        guide: [
            '【柱状图专业规范】',
            CS_PALETTE_NOTES,
            '',
            '【排版布局】',
            '- grid：{ left:16, right:16, top:56, bottom:12, containLabel:true }',
            '- 类目 >12 或类目名过长 → 改横向条形（xAxis value / yAxis category，value 轴放右侧留白）并按数值降序排列',
            '- xAxis：axisTick:{ show:false }, axisLine:{ lineStyle:{ color:"#D1D5DB" } }；长类目名 rotate:30~45 或 interval 截断',
            '',
            '【系列样式】',
            '- barWidth:18~32（类目多取窄）；单系列所有柱同色（主色 #4E79A7），禁止彩虹柱',
            '- 需要突出单项时，仅该项用强调色 #F28E2B，其余用主色的 35% 透明版本',
            '- itemStyle:{ borderRadius:[6,6,0,0] }（横向条形为 [0,6,6,0]）',
            '- showBackground:true, backgroundStyle:{ color:"rgba(0,0,0,0.04)" } 增强可读性',
            '- 数据标签：label:{ show:true, position:"top", color:"#6B7280", fontSize:11 }（横向为 position:"right"）；柱子 >24 根时关闭标签',
            '',
            '【对比增强】',
            '- 同类目多系列 ≤3 组，barGap:"20%"；分组 >3 改堆叠或分面',
            '- tooltip：trigger:"axis", axisPointer:{ type:"shadow" }'
        ].join('\n'),
        skeleton: [
            '{',
            CS_SKELETON_TITLE_LEGEND,
            CS_SKELETON_XAXIS_CAT,
            '  "series": [',
            '    { "name": "系列A", "type": "bar", "barWidth": 26, "barGap": "20%",',
            '      "itemStyle": { "borderRadius": [6, 6, 0, 0], "color": "#4E79A7" },',
            '      "showBackground": true, "backgroundStyle": { "color": "rgba(0,0,0,0.04)" },',
            '      "label": { "show": true, "position": "top", "color": "#6B7280", "fontSize": 11 },',
            '      "data": [10, 25, 16] }',
            '  ]',
            '}'
        ].join('\n')
    },
    {
        id: 'echarts-pie', name: '饼状图专业规范', format: 'echarts',
        match: ['饼状', '饼图', '占比构成', '构成比', 'pie'],
        guide: [
            '【饼状图专业规范】',
            CS_PALETTE_NOTES,
            '',
            '【排版布局】',
            '- radius:"62%", center:["50%","54%"]；标题左上、图例纵向置于右侧（legend:{ orient:"vertical", right:8, top:"middle" }）',
            '- itemStyle:{ borderColor:"rgba(255,255,255,0.9)", borderWidth:2, borderRadius:4 } 扇区间留呼吸缝',
            '',
            '【数据规范】',
            '- 扇区 ≤6 个直接标签：label:{ formatter:"{b}\\n{d}%", color:"#6B7280" }, labelLine:{ length:12, length2:8 }',
            '- 扇区 >6 个：小占比合并为「其他」（灰色 #BAB0AC 放最后），或改用环形图+图例；>10 个必须改条形图（按占比降序）',
            '- 起始角 startAngle:90，按占比顺时针降序排列',
            '',
            '【色彩】',
            '- 按主色板顺序取色；同类分层用同色系明度渐变（如 #4E79A7 → #A0C1E3）；「其他」永远灰色'
        ].join('\n'),
        skeleton: [
            '{',
            '  "title": { "text": "图表标题", "left": 0, "top": 0, "textStyle": { "fontSize": 15, "fontWeight": 600, "color": "#1F2937" } },',
            '  "legend": { "orient": "vertical", "right": 8, "top": "middle", "icon": "circle", "itemWidth": 8, "textStyle": { "color": "#6B7280", "fontSize": 12 } },',
            '  "tooltip": { "trigger": "item", "formatter": "{b}: {d}%" },',
            '  "series": [',
            '    { "name": "占比", "type": "pie", "radius": "62%", "center": ["42%", "54%"], "startAngle": 90,',
            '      "itemStyle": { "borderColor": "rgba(255,255,255,0.9)", "borderWidth": 2, "borderRadius": 4 },',
            '      "label": { "formatter": "{b}\\n{d}%", "color": "#6B7280" }, "labelLine": { "length": 12, "length2": 8 },',
            '      "data": [',
            '        { "name": "分类A", "value": 40, "itemStyle": { "color": "#4E79A7" } },',
            '        { "name": "分类B", "value": 30, "itemStyle": { "color": "#F28E2B" } },',
            '        { "name": "分类C", "value": 20, "itemStyle": { "color": "#59A14F" } },',
            '        { "name": "其他", "value": 10, "itemStyle": { "color": "#BAB0AC" } }',
            '      ] }',
            '  ]',
            '}'
        ].join('\n')
    },
    {
        id: 'echarts-donut', name: '环形图专业规范', format: 'echarts',
        match: ['环形', '圆环', '甜甜圈', 'donut', 'doughnut'],
        guide: [
            '【环形图专业规范】',
            CS_PALETTE_NOTES,
            '',
            '【排版布局】',
            '- radius:["46%","72%"], center:["50%","54%"]；itemStyle:{ borderRadius:6, borderColor:"rgba(255,255,255,0.9)", borderWidth:2 }',
            '- 中心放核心数值：title:{ text:"68%", subtext:"总达成率", left:"center", top:"46%", textStyle:{ fontSize:26, fontWeight:700, color:"#1F2937" }, subtextStyle:{ color:"#9CA3AF" } }',
            '',
            '【数据规范】',
            '- 适合强调单一核心占比 + 构成对比；份额明细靠图例与 tooltip（label 默认关闭或只显示 {d}%）',
            '- 环段 ≤8；超过改条形图',
            '',
            '【色彩】',
            '- 按主色板取色；需要突出某段时该段用强调色，其余统一淡灰蓝 #B9CCE4，emphasis 时恢复',
            '- 扇区宽度适中：内径不小于外径的 60%，避免读成仪表盘'
        ].join('\n'),
        skeleton: [
            '{',
            '  "title": { "text": "58%", "subtext": "核心指标名", "left": "center", "top": "46%",',
            '    "textStyle": { "fontSize": 26, "fontWeight": 700, "color": "#1F2937" }, "subtextStyle": { "color": "#9CA3AF", "fontSize": 12 } },',
            '  "legend": { "orient": "vertical", "right": 8, "top": "middle", "icon": "circle", "itemWidth": 8, "textStyle": { "color": "#6B7280", "fontSize": 12 } },',
            '  "tooltip": { "trigger": "item", "formatter": "{b}: {d}%" },',
            '  "series": [',
            '    { "name": "构成", "type": "pie", "radius": ["46%", "72%"], "center": ["50%", "54%"],',
            '      "itemStyle": { "borderRadius": 6, "borderColor": "rgba(255,255,255,0.9)", "borderWidth": 2 },',
            '      "label": { "show": false },',
            '      "data": [',
            '        { "name": "分类A", "value": 45, "itemStyle": { "color": "#4E79A7" } },',
            '        { "name": "分类B", "value": 30, "itemStyle": { "color": "#F28E2B" } },',
            '        { "name": "分类C", "value": 25, "itemStyle": { "color": "#76B7B2" } }',
            '      ] }',
            '  ]',
            '}'
        ].join('\n')
    },
    {
        id: 'echarts-scatter', name: '散点图专业规范', format: 'echarts',
        match: ['散点', '分布', '相关性', 'cluster', 'scatter'],
        guide: [
            '【散点图专业规范】',
            CS_PALETTE_NOTES,
            '',
            '【排版布局】',
            '- grid 四周留白收紧（left:16, right:24, containLabel:true）；两轴均 scale:true 不强制零点',
            '- splitLine 双向虚线 #E5E7EB 极淡（opacity 0.6），营造坐标纸感',
            '',
            '【系列样式】',
            '- symbolSize 按值映射：4 ~ 26 线性映射；单色时用主色 opacity 0.65，重叠深浅自然表达密度',
            '- 分类散点每类一色（主色板），symbolSize 统一 10~14，禁止大小与颜色同时编码不同变量造成误读',
            '- 连续维度用 visualMap 单色渐变（#B9CCE4 → #4E79A7），calculable:false，置于底部',
            '',
            '【标注增强】',
            '- 离群点用 markPoint 或 label 单独标注；趋势参考线用 markLine（线性回归/均值，dashed #E15759）'
        ].join('\n'),
        skeleton: [
            '{',
            CS_SKELETON_TITLE_LEGEND,
            '  "xAxis": { "type": "value", "scale": true, "name": "自变量", "nameTextStyle": { "color": "#9CA3AF" }, "splitLine": { "lineStyle": { "color": "#E5E7EB", "type": "dashed", "opacity": 0.6 } }, "axisLabel": { "color": "#6B7280" } },',
            '  "yAxis": { "type": "value", "scale": true, "name": "因变量", "nameTextStyle": { "color": "#9CA3AF" }, "splitLine": { "lineStyle": { "color": "#E5E7EB", "type": "dashed", "opacity": 0.6 } }, "axisLabel": { "color": "#6B7280" } },',
            '  "series": [',
            '    { "name": "样本组A", "type": "scatter", "symbolSize": 12, "itemStyle": { "color": "rgba(78,121,167,0.65)" },',
            '      "data": [[10, 20], [25, 32], [40, 28]] }',
            '  ]',
            '}'
        ].join('\n')
    },
    {
        id: 'echarts-radar', name: '雷达图专业规范', format: 'echarts',
        match: ['雷达', '多维', '能力评估', 'radar'],
        guide: [
            '【雷达图专业规范】',
            CS_PALETTE_NOTES,
            '',
            '【排版布局】',
            '- radar.center:["50%","54%"], radius:"62%"；indicator.name.textStyle:{ color:"#6B7280", fontSize:12 }',
            '- splitArea 交替底色 rgba(78,121,167,0.04) / 透明；splitLine #E5E7EB；axisLine #D1D5DB',
            '',
            '【系列样式】',
            '- 系列数 ≤4；每系列 lineStyle width 2 + areaStyle opacity 0.15~0.2，颜色取主色板前 N 色',
            '- 指标维度 4~8 个为宜；各指标量纲差异大时先归一化（max/min 设定一致刻度）',
            '',
            '【注意事项】',
            '- 禁止用雷达图表达精确数值对比（人眼对多边形面积不敏感），结论性数据改分组柱状图'
        ].join('\n'),
        skeleton: [
            '{',
            '  "title": { "text": "图表标题", "left": 0, "top": 0, "textStyle": { "fontSize": 15, "fontWeight": 600, "color": "#1F2937" } },',
            '  "legend": { "top": 0, "right": 0, "icon": "circle", "itemWidth": 8, "textStyle": { "color": "#6B7280", "fontSize": 12 } },',
            '  "tooltip": { "trigger": "item" },',
            '  "radar": {',
            '    "center": ["50%", "54%"], "radius": "62%", "splitNumber": 4,',
            '    "indicator": [',
            '      { "name": "维度A", "max": 100 }, { "name": "维度B", "max": 100 }, { "name": "维度C", "max": 100 },',
            '      { "name": "维度D", "max": 100 }, { "name": "维度E", "max": 100 }',
            '    ],',
            '    "axisName": { "color": "#6B7280", "fontSize": 12 },',
            '    "splitArea": { "areaStyle": { "color": ["rgba(78,121,167,0.04)", "transparent"] } },',
            '    "splitLine": { "lineStyle": { "color": "#E5E7EB" } }, "axisLine": { "lineStyle": { "color": "#D1D5DB" } }',
            '  },',
            '  "series": [',
            '    { "type": "radar", "data": [',
            '      { "name": "对象A", "value": [80, 65, 90, 70, 85], "lineStyle": { "width": 2, "color": "#4E79A7" }, "areaStyle": { "color": "rgba(78,121,167,0.18)" }, "symbol": "circle", "symbolSize": 4 },',
            '      { "name": "对象B", "value": [60, 75, 70, 82, 66], "lineStyle": { "width": 2, "color": "#F28E2B" }, "areaStyle": { "color": "rgba(242,142,43,0.15)" }, "symbol": "circle", "symbolSize": 4 }',
            '    ] }',
            '  ]',
            '}'
        ].join('\n')
    },
    {
        id: 'echarts-gauge', name: '仪表盘专业规范', format: 'echarts',
        match: ['仪表', '进度盘', '完成率表', 'gauge'],
        guide: [
            '【仪表盘专业规范】',
            CS_PALETTE_NOTES,
            '',
            '【排版布局】',
            '- 单值一表；多指标改横向进度条形图，禁止一屏多表盘拥挤',
            '- center:["50%","58%"], radius:"78%"；startAngle:210, endAngle:-30（标准表盘视角）',
            '',
            '【系列样式】',
            '- 进度式：progress:{ show:true, width:14, roundCap:true }，axisLine:{ lineStyle:{ width:14, color:[[1,"#E9EDF3"]] } }',
            '- 阈值分段色带：color:[[0.6,"#2E8B6E"],[0.85,"#F39C12"],[1,"#E15759"]]（绿→黄→红，语义明确）',
            '- pointer 用细针或隐藏（progress 模式下 pointer:{ icon:"circle", width:6 }），anchor 弱化',
            '- detail：fontSize:26, fontWeight:700, color 取当前阈值段颜色, formatter:"{value}%"；title 距盘下方 offsetCenter:[0,"62%"]，#6B7280'
        ].join('\n'),
        skeleton: [
            '{',
            '  "series": [',
            '    { "type": "gauge", "center": ["50%", "58%"], "radius": "78%", "startAngle": 210, "endAngle": -30, "min": 0, "max": 100,',
            '      "progress": { "show": true, "width": 14, "roundCap": true, "itemStyle": { "color": "#4E79A7" } },',
            '      "axisLine": { "roundCap": true, "lineStyle": { "width": 14, "color": [[1, "#E9EDF3"]] } },',
            '      "axisTick": { "show": false }, "splitLine": { "show": false }, "axisLabel": { "color": "#9CA3AF", "distance": 20 },',
            '      "pointer": { "icon": "circle", "width": 6, "itemStyle": { "color": "#4E79A7" } },',
            '      "title": { "offsetCenter": [0, "62%"], "color": "#6B7280", "fontSize": 13 },',
            '      "detail": { "valueAnimation": true, "offsetCenter": [0, "38%"], "fontSize": 26, "fontWeight": 700, "color": "#1F2937", "formatter": "{value}%" },',
            '      "data": [{ "value": 0, "name": "指标名" }] }',
            '  ]',
            '}'
        ].join('\n')
    },
    {
        id: 'echarts-generic', name: 'ECharts 通用视觉规范', format: 'echarts',
        match: [],
        guide: [
            '【ECharts 通用视觉规范】（未命中具体图表类型时的兜底参考）',
            CS_PALETTE_NOTES,
            '',
            '【排版层级】',
            '- 视觉动线：标题（左上 15px 半粗 #1F2937）→ 图例（右上 12px）→ 数据区 → 轴标签；四周留白通过 grid containLabel 保证',
            '- legend:{ top:0, right:0, icon:"circle", itemWidth:8, itemGap:16, textStyle:{ color:"#6B7280", fontSize:12 } }',
            '- tooltip 统一：backgroundColor:"rgba(255,255,255,0.96)", borderColor:"#E5E7EB", textStyle:{ color:"#1F2937", fontSize:12 }, extraCssText:"box-shadow:0 4px 16px rgba(0,0,0,0.08);border-radius:8px;"',
            '',
            '【专业底线】',
            '- 系列 ≤6；数据标签只在必要处出现；坐标轴最多两套；禁用 3D、阴影、发光、彩虹色',
            '- animationDuration:600, animationEasing:"cubicOut"',
            '- 数值格式化：大数用 formatter 缩写（万/亿），百分比保留 1 位小数'
        ].join('\n'),
        skeleton: [
            '{',
            CS_SKELETON_TITLE_LEGEND,
            CS_SKELETON_XAXIS_CAT,
            '  "series": [',
            '    { "name": "系列A", "type": "line", "data": [10, 25, 16] }',
            '  ],',
            '  "animationDuration": 600, "animationEasing": "cubicOut"',
            '}'
        ].join('\n')
    },

    // ============ Mermaid 结构图 ============
    {
        id: 'mermaid-flowchart', name: '流程图专业规范', format: 'mermaid',
        match: ['流程', 'flowchart', 'graph', '步骤', '判定', '工作流'],
        guide: [
            '【流程图专业规范（Mermaid flowchart）】',
            '【共享主题变量（置于首行注入）】',
            '%%{init: { "theme":"base", "themeVariables": { "primaryColor":"#E8F0FE", "primaryTextColor":"#1F2937", "primaryBorderColor":"#4E79A7", "lineColor":"#94A3B8", "secondaryColor":"#FDF3E3", "tertiaryColor":"#EFF6F3", "fontSize":"14px" } } }%%',
            '',
            '【排版布局】',
            '- 方向：步骤 ≤5 用 TD（纵向）；链路长/分支多用 LR（横向）；整体宽高比控制在 4:3 附近',
            '- 节点形状语义：圆角矩形[过程] / 菱形{判断} / 平行四边形[/输入输出/] / 圆柱[(数据存储)] / 圆角胶囊(起止)',
            '- 节点数 ≤15；超过必须用 subgraph 分组收纳或拆分层级',
            '',
            '【文字与连线】',
            '- 节点文本 ≤10 字；边标签 ≤6 字（-- 是 -->）；判定分支的是/否标签必须写全',
            '- 连线统一 lineColor #94A3B8；关键路径可用 -.-> 虚线表达可选分支，实线为主',
            '',
            '【配色】',
            '- 默认节点淡蓝 #E8F0FE；起止节点淡绿 #EFF6F3；判断节点淡橙 #FDF3E3；每类仅一种底色，文字统一 #1F2937',
            '- 禁止逐节点随机换色；强调节点用描边加粗（stroke-width via classDef）而非换色'
        ].join('\n'),
        skeleton: [
            '%%{init: { "theme":"base", "themeVariables": { "primaryColor":"#E8F0FE", "primaryTextColor":"#1F2937", "primaryBorderColor":"#4E79A7", "lineColor":"#94A3B8", "secondaryColor":"#FDF3E3", "tertiaryColor":"#EFF6F3", "fontSize":"14px" } } }%%',
            'flowchart TD',
            '    A([开始]) --> B[过程节点]',
            '    B --> C{是否通过?}',
            '    C -- 是 --> D[处理分支]',
            '    C -- 否 --> E[修正来源] --> B',
            '    D --> F([结束])',
            '',
            '    classDef startEnd fill:#EFF6F3,stroke:#59A14F,stroke-width:1.5px;',
            '    classDef judge fill:#FDF3E3,stroke:#F28E2B,stroke-width:1.5px;',
            '    class A,F startEnd; class C judge'
        ].join('\n')
    },
    {
        id: 'mermaid-sequence', name: '时序图专业规范', format: 'mermaid',
        match: ['时序', '顺序图', '交互', 'sequence', '调用链'],
        guide: [
            '【时序图专业规范（Mermaid sequenceDiagram）】',
            '【共享主题变量】',
            '%%{init: { "theme":"base", "themeVariables": { "actorBkg":"#E8F0FE", "actorTextColor":"#1F2937", "actorBorder":"#4E79A7", "signalColor":"#4B5563", "signalTextColor":"#6B7280", "noteBkgColor":"#FDF3E3", "noteTextColor":"#1F2937", "activationBkgColor":"#D6E4F7", "fontSize":"14px" } } }%%',
            '',
            '【排版布局】',
            '- 参与者 ≤6 个，从左到右按交互频率/调用顺序排列；autonumber 开启便于引用步骤',
            '- 消息文本 ≤16 字；激活条（activate/deactivate 成对）标出处理中的调用',
            '',
            '【语义着色】',
            '- 同步调用实线 →，返回虚线 -->>；异步用 ->>；Notes（#FDF3E3）补充时序说明',
            '- 关键消息用 rect rgba(78,121,167,0.06) 底色框圈出阶段（如「鉴权阶段」「回调阶段」）'
        ].join('\n'),
        skeleton: [
            '%%{init: { "theme":"base", "themeVariables": { "actorBkg":"#E8F0FE", "actorTextColor":"#1F2937", "actorBorder":"#4E79A7", "signalColor":"#4B5563", "signalTextColor":"#6B7280", "noteBkgColor":"#FDF3E3", "noteTextColor":"#1F2937", "activationBkgColor":"#D6E4F7", "fontSize":"14px" } } }%%',
            'sequenceDiagram',
            '    autonumber',
            '    participant U as 发起方',
            '    participant S as 服务方',
            '    U->>S: 请求（≤16字）',
            '    activate S',
            '    S-->>U: 响应',
            '    deactivate S',
            '    Note over U,S: 阶段说明注记'
        ].join('\n')
    },
    {
        id: 'mermaid-gantt', name: '甘特图专业规范', format: 'mermaid',
        match: ['甘特', '排期', '计划表', '进度计划', 'gantt', '里程碑'],
        guide: [
            '【甘特图专业规范（Mermaid gantt）】',
            '【共享主题变量】',
            '%%{init: { "theme":"base", "themeVariables": { "cScale0":"#4E79A7", "cScale1":"#59A14F", "cScale2":"#F28E2B", "cScale3":"#76B7B2", "cScale4":"#B07AA1", "taskTextOutsideColor":"#1F2937", "gridColor":"#E5E7EB", "fontSize":"13px" } } }%%',
            '',
            '【排版布局】',
            '- dateFormat YYYY-MM-DD，axisFormat %m-%d；spanning ≤2 个月（更长改按周 %W）',
            '- section 分组 3~6 个；每组任务 2~8 项，任务名 ≤12 字',
            '',
            '【语义规范】',
            '- 关键路径任务加 crit（红色系标识），已完成 done（灰绿），进行中 active（主色高亮）',
            '- 里程碑用 milestone 语法单独一行；excludes weekends 避免工时误读',
            '',
            '【配色】',
            '- 每个 section 一种主色板色系，任务条同色系深浅；禁止逐任务随机取色'
        ].join('\n'),
        skeleton: [
            '%%{init: { "theme":"base", "themeVariables": { "cScale0":"#4E79A7", "cScale1":"#59A14F", "cScale2":"#F28E2B", "taskTextOutsideColor":"#1F2937", "gridColor":"#E5E7EB", "fontSize":"13px" } } }%%',
            'gantt',
            '    title 项目排期标题',
            '    dateFormat YYYY-MM-DD',
            '    axisFormat %m-%d',
            '    excludes weekends',
            '',
            '    section 阶段一',
            '    已完成任务 :done, t1, 2026-01-05, 3d',
            '    进行中任务 :active, t2, after t1, 4d',
            '    关键任务   :crit, t3, after t2, 5d',
            '',
            '    section 阶段二',
            '    后续任务   :t4, after t3, 4d',
            '    交付节点   :milestone, m1, after t4, 0d'
        ].join('\n')
    },
    {
        id: 'mermaid-tree', name: '树状图专业规范', format: 'mermaid',
        match: ['树状', '树形', '层级', '组织结构', '思维导图', '脑图', 'mindmap', '分类体系'],
        guide: [
            '【树状图专业规范（Mermaid graph TD / mindmap）】',
            '【共享主题变量】',
            '%%{init: { "theme":"base", "themeVariables": { "primaryColor":"#E8F0FE", "primaryTextColor":"#1F2937", "primaryBorderColor":"#4E79A7", "lineColor":"#B9C4D4", "fontSize":"14px" } } }%%',
            '',
            '【排版布局】',
            '- 树根在上（TD）或居左（LR）：层级 ≤4 层；同级节点 ≤7 个，超出按语义再分组',
            '- 对称树用 TD；目录/依赖展开用 LR；纯脑图优先 mindmap 语法（自动均衡布局）',
            '',
            '【节点规范】',
            '- 父节点概括词 ≤6 字，子节点 ≤10 字；同层节点词性/粒度保持一致',
            '- 层级配色：根 #D6E4F7 深些、二级 #E8F0FE、叶级 #F5F8FC 逐层变浅；文字统一 #1F2937',
            '- 强调分支用描边色区分（classDef 强调色 #F28E2B 描边），不改文字颜色'
        ].join('\n'),
        skeleton: [
            '%%{init: { "theme":"base", "themeVariables": { "primaryColor":"#E8F0FE", "primaryTextColor":"#1F2937", "primaryBorderColor":"#4E79A7", "lineColor":"#B9C4D4", "fontSize":"14px" } } }%%',
            'graph TD',
            '    ROOT[根主题] --> A[分支A]',
            '    ROOT --> B[分支B]',
            '    A --> A1[叶子A1]',
            '    A --> A2[叶子A2]',
            '    B --> B1[叶子B1]',
            '',
            '    classDef root fill:#D6E4F7,stroke:#4E79A7,stroke-width:2px;',
            '    classDef leaf fill:#F5F8FC,stroke:#B9C4D4,stroke-width:1px;',
            '    class ROOT root; class A1,A2,B1 leaf'
        ].join('\n')
    },
    {
        id: 'mermaid-state', name: '状态图专业规范', format: 'mermaid',
        match: ['状态', '状态机', '生命周期', 'state'],
        guide: [
            '【状态图专业规范（Mermaid stateDiagram-v2）】',
            '【共享主题变量】',
            '%%{init: { "theme":"base", "themeVariables": { "primaryColor":"#E8F0FE", "primaryTextColor":"#1F2937", "primaryBorderColor":"#4E79A7", "lineColor":"#94A3B8", "fontSize":"14px" } } }%%',
            '',
            '【排版布局】',
            '- 状态 ≤10 个；初始态 [*] 置顶，终态 [*] 置底，主流转环居中',
            '- 状态名用大写驼峰或中文短语 ≤8 字，转移标签 ≤10 字',
            '',
            '【语义规范】',
            '- 组合状态用 state 嵌套表达子状态机；并发分支用 -- 分隔线',
            '- 异常/回退转移用虚线注释说明触发条件，主流程转移保持实线简洁'
        ].join('\n'),
        skeleton: [
            '%%{init: { "theme":"base", "themeVariables": { "primaryColor":"#E8F0FE", "primaryTextColor":"#1F2937", "primaryBorderColor":"#4E79A7", "lineColor":"#94A3B8", "fontSize":"14px" } } }%%',
            'stateDiagram-v2',
            '    [*] --> 初始态',
            '    初始态 --> 处理中: 触发条件',
            '    处理中 --> 处理中: 自循环',
            '    处理中 --> 已完成: 成功',
            '    处理中 --> 已失败: 异常',
            '    已失败 --> 初始态: 重试',
            '    已完成 --> [*]'
        ].join('\n')
    },
    {
        id: 'mermaid-generic', name: 'Mermaid 通用视觉规范', format: 'mermaid',
        match: [],
        guide: [
            '【Mermaid 通用视觉规范】（未命中具体结构图类型时的兜底参考）',
            '【主题注入方式（所有结构图统一）】',
            '- 首行注入 %%{init: { "theme":"base", "themeVariables": { "primaryColor":"#E8F0FE", "primaryTextColor":"#1F2937", "primaryBorderColor":"#4E79A7", "lineColor":"#94A3B8", "fontSize":"14px" } } }%%',
            '- 配色哲学：底色低饱和淡彩 + 深灰文字 + 单一蓝灰强调，杜绝高饱和撞色',
            '',
            '【排版通用】',
            '- 节点/参与方 ≤15；文本 ≤12 字/行；方向按内容长宽比选 TD/LR',
            '- 同类元素统一形状与颜色语义；分组用 subgraph 并给分组标题',
            '- 连线只保留必要交叉，label 简短；整体留白由容器滚动区自然承载'
        ].join('\n'),
        skeleton: [
            '%%{init: { "theme":"base", "themeVariables": { "primaryColor":"#E8F0FE", "primaryTextColor":"#1F2937", "primaryBorderColor":"#4E79A7", "lineColor":"#94A3B8", "fontSize":"14px" } } }%%',
            'flowchart LR',
            '    A[起始节点] --> B[目标节点]',
            '    B --> C{分支判断?}',
            '    C -- 是 --> D[分支一]',
            '    C -- 否 --> E[分支二]'
        ].join('\n')
    }
];

/**
 * 按智能体声明的格式与类型检索样式参考
 * 匹配规则：类型文本命中条目 match 关键词的累计长度最高者；
 * 无命中时回退到对应格式的通用规范（echarts-generic / mermaid-generic）
 * @param {string} format - 'echarts' | 'mermaid'
 * @param {string} typeText - 智能体声明的图表类型文本
 * @returns {{ id, name, guide, skeleton }} 命中的参考条目
 */
function lookupCSStyleGuide(format, typeText) {
    const text = String(typeText || '').toLowerCase();
    let best = null, bestScore = 0;
    for (const entry of CHART_STYLE_LIBRARY) {
        let score = 0;
        for (const kw of entry.match || []) {
            if (text.includes(kw.toLowerCase())) score += kw.length;
        }
        if (score > bestScore) { bestScore = score; best = entry; }
    }
    if (best) return best;
    const fallbackId = format === 'mermaid' ? 'mermaid-generic' : 'echarts-generic';
    return CHART_STYLE_LIBRARY.find(e => e.id === fallbackId) || CHART_STYLE_LIBRARY[0];
}

/**
 * 组装注入智能体上下文的完整参考文本：文字规范 + 样式骨架示例
 * 骨架仅示范样式字段与格式结构，数据为占位，智能体构建时必须替换为真实数据
 * @param {{ guide, skeleton }} entry - 命中的参考条目
 * @returns {string}
 */
function buildCSStyleGuideText(entry) {
    if (!entry.skeleton) return entry.guide;
    return entry.guide
        + '\n\n【样式骨架示例（仅示范样式字段与格式结构；骨架内数据均为占位，必须替换为指令对应的真实数据，禁止照抄）】\n'
        + entry.skeleton;
}
