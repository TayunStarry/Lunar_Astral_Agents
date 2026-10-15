# 13 控制台 TUI 系统

> [🏠 文档地图](README.md) | [◀ 上一章](12-图像生成参数策略.md)

`subsystem/tui_system`（Go 模块 `LunarSubsystem/TuiSystem`）是核心人格程序的**全屏终端控制台**：把日志管线、运行状态、智能体实况与运维功能（更新包 / LTPX 应用市场 / 打包归档 / 端口释放 / 局域代理 / 前端管理）收敛到单一界面。技术栈为 Bubble Tea v1 + Lip Gloss v1 + bubbles（spinner / textinput）。

控制台是程序的**唯一形态**（`subsystem/core_personality/main.go`，不带任何构建标签）：启动后由它代为打开月华与琉璃两个前端窗口，窗口全部关闭不退出控制台。配置取自 `lunar_config.json` 的 `release` 节（见 [08 §6](08-构建运行与配置.md)）。

---

## 13.1 分层结构

| 层 | 文件 | 职责 |
|----|------|------|
| 主题与排版原语 | `theme.go` | 统一调色板与样式；按**显示列宽**计算的 `dispWidth` / `fitWidth` / `truncTo` / `rowBuilder` / `sectionRule` / `stepsBar` / `progressBar` 等 |
| 框体 | `layout.go` | `frame` 三段式骨架（标题栏 / 定高内容区 / 键位栏）、键位帮助页、跨屏任务指示（`busy` / `notice`） |
| 列表与窗口 | `list.go` | `listPane` 定高滚动窗口 + 滚动条、行窗口 `windowLines` / `scrollKey`、详情窗格 `textPane`、键位语义 `listMove` / `listNavKey` |
| 发布通道契约 | `channel/` | 更新包与 LTPX 应用市场共用仓库时的归属判别（标记 + 附件前缀），两侧子包共用同一份契约 |
| 应用模型 | `tui.go` | 屏幕编号与菜单表、全局键位、消息分发、菜单屏两栏布局、共享组件 |
| 各屏视图 | `view_market.go` · `view_archive.go`（含更新包屏）· `view_logs.go`（含实况 / 绘图 / 运行状态）· `view_frontend.go`（含端口释放）· `view_proxy.go` | 各屏状态、事件处理与渲染 |
| 功能子包 | `archive` · `ltpxmarket` · `portrelease` · `proxy` · `updatepack` | 纯后台能力，经 `SetLogHook` / `SetProgressHook` 把日志与进度回灌 TUI，不反向依赖界面 |

---

## 13.2 排版契约（改动前请先读）

这五条是界面不「歪」、不「顶出屏幕」的根因约束，均有测试守护。

1. **内容不得越界。** 每屏只负责返回内容行，由 `frame.render` 裁剪 / 补齐到恰好 `bodySize()` 行、每行恰好等于内容宽度。整屏行数恒等于终端高度。
   → 想加内容就直接 append，不必自己数字数。
2. **宽度按显示列宽计算。** 中文、emoji 占 2 列，ANSI 转义序列占 0 列，因此禁止用 `len()` 或 `[]rune` 长度代替宽度；一律走 `dispWidth` / `fitWidth` / `truncTo`。
3. **列表行按「窗口宽度 − `listGutter`」构造。** 定高窗口在超屏时把最右 2 列用于滚动条，行若按全宽构造会被截掉结尾（典型症状：体积信息显示成 `1.0…`）。`listGutter` 常驻让出，保证有 / 无滚动条时行宽都稳定。
4. **底栏右侧优先。** 空间不足时先压缩左侧键位提示、再截断右侧状态：位置 / 进度 / 错误这类状态信息不可丢。
5. **派生状态必须在 `Update` 路径刷新。** View 收到的模型是副本，在 View 里写回的字段不会留存。日志行数、实况行数等「滚动上限」由 `syncViewMetrics()` 在 `Update` 中刷新，否则按方向键时行数恒为 0，滚动静默失效。

---

## 13.3 屏幕与键位总表

| 键 | 屏幕 | 关键操作 |
|----|------|----------|
| `0` | 资源更新 管理 | 发布更新包 / 拉取最新 / 按 tag / 按哈希 / 版本列表（版本列表可直接回车拉取） |
| `m` | LTPX 应用市场 | `↑/↓` 选择、`Enter` 详情、`i` 安装、`/` 关键词搜索（本地）、`t` 标签筛选（远端）、`u` 发布本地包、`r` 刷新 |
| `1` | 核心智能 实况 | 思考 / 输出 / 工具调用流式显示；`f` 跟随开关；回看时自动暂停跟随 |
| `2` | 运行状态 | 运行环境与模型计量双栏，1s 采样 |
| `3`~`7` | 原始 / 月华 JS / 插件 JS / Go 警告 / 绘图引擎日志 | `↑/↓ PgUp/PgDn` 滚动、`Home/End` 首尾、`f` 跟随、`c` 清空；长行按显示宽度软折行 |
| `8` | 前端页面 管理 | `1/2` 或回车重启月华 / 琉璃窗口 |
| `9` | 局域共享 管理 | 配置 WSS→WS 反代并展示扫码二维码 |
| `a` | 打包归档 管理 | 选择配置 → 确认 → 7z 分卷打包 |
| `b` | 端口释放 管理 | 扫描端口范围并终止占用进程 |
| `z` | 退出程序 | 优雅停止全部子系统 |

全局键位：`↑/↓ k/j` 移动、`Enter` 确认、`Esc` 返回上级、`?` 键位帮助（模态）、`Ctrl+C` 退出。自由文本输入框聚焦时 `?` 让位为普通字符；端口一类受控输入仍以 `?` 打开帮助。

发布 LTPX 包为三步向导（步骤条常驻显示）：① 选择本地包（或 `f` 上传已打包 `.ltpx`）→ ② 填写描述（`#标签` 自动识别）→ ③ 输入 Gitee 令牌（仅本次注入副本，不落盘）。

---

## 13.4 Gitee 发布通道：更新包 / LTPX 应用市场

更新包与 LTPX 应用市场**共用同一仓库**（同为 Gitee Release），因此每条发布都必须能判定归属；
判别契约集中在 `channel/` 包一处，两个子包各自映射 Gitee 响应结构：

| 通道 | 描述标记 | 附件名前缀 | 发布端 |
|------|----------|------------|--------|
| 更新包 | `[UPDATE]` | `lunar_updatepack_<版本>.7z.001` | `updatepack.Publish` |
| LTPX 应用市场 | `[LTPX]` | `ltpx_<id>.ltpx` | `ltpxmarket.Publish` / `PublishArchive` |

判定依据两条且互为兜底——只看附件会漏掉「发布已建、附件未传完」的空发布；
只看标记会漏掉隔离上线前发布的历史更新包（当年没有 `[UPDATE]`，但附件前缀一直在）。
两者冲突时以**附件为准**（附件事务性更强）。

隔离落点：

| 场景 | 行为 |
|------|------|
| 「拉取最新」 | 取排序后最新**且确有更新包附件**的一条；夹在中间的市场发布与空发布自动跳过 |
| 按 tag 拉取 | 命中非更新包通道时直接报错并说明因由，不会下载成工具包 |
| 按哈希拉取 | tag 匹配阶段逐条回查 Release 校验通道，只认更新包 tag，否则回落 zipball 通道 |
| 版本列表 | 只列更新包通道的发布（最新在前），并记录跳过数量 |
| 市场浏览 | 只解析 `[LTPX]` 且附件前缀相符的发布，更新包发布一律跳过 |
| 发布（两侧） | tag 撞车时拦下：市场侧不会删除更新包的 Release，更新包侧不会复用市场的 Release |

> **响应顺序不可信（实测故障）**：Gitee 的发布列表接口**默认 `direction=asc`，最旧在前**。
> 两条通道原先都假设「响应即倒序」，于是「拉取最新」取到的是**最旧**的那条——仓库里同时有
> `v2026.10.15-2232` 与 `v2026.10.15-2250` 时，实测拉到 2232；版本列表也把最旧的排在最前，
> 而列表默认选中第一项，回车等于拉最旧的包（市场侧更糟：先按 `limit` 截断，新包根本进不了列表）。
>
> 做法：请求显式带 `direction=desc`，**并且**拿到列表后一定再用 `channel.Newer` 自己排一遍
> （时间戳优先，缺失或相同时退化为标签自然序——数字段按数值比，避免 `-930` 被判成大于 `-1030`）。
> 凡「取最新 / 截断列表 / 默认选中」都必须排在排序之后。

发行说明（更新包 Release 描述）由 `updatepack/releasenotes.go` 生成：`RELEASE.md` 的精简投影
（是什么发行版 / 适用平台 / 本次内容 / 安装方式 / 包内文件与 SHA-256），首行固定带 `[UPDATE]` 标记。
维护约定：`RELEASE.md` 的定位与安装方式如有调整，请同步该文件的常量。

### 主程序更新的交接（易踩的三个坑）

含主程序文件的更新不能在进程内覆盖（自身 exe 与已加载 DLL 被系统锁定），必须
「控制台退出 → 独立更新器覆盖 → 重启」。这条链路上有三处必须守住：

| 约束 | 原因与做法 |
|------|-----------|
| **更新器永不等待自身镜像** | `program_files` 含 `Lunar_Update_Applier.exe`，若把它也列入 `-wait-file`，更新器会等自己变成可写——运行中的 exe 不允许以写方式打开（镜像加载器不共享写），必然等满超时（实测空转 60 秒）。而覆盖自身并不需要它可写：改名/删除只需 DELETE 共享。两侧都做过滤：`updatepack.waitFileArgs` 与更新器内的 `os.SameFile` 判定 |
| **退出前必须给出交接画面** | 控制台一退终端就空，用户只看到「黑了」；屏上给倒计时 + 更新包/目标/日志路径，退出后再把同样信息打印到真实终端（备用屏幕恢复之后，可回看可复制） |
| **更新器输出必须接回终端** | 主程序接管 stdout 后，子进程若继承 NUL 设备，用户看不到「已启动 → 正在覆盖 → 已重启」，整条链路成黑盒。由 `updatepack.SetConsoleWriter` 注入真实终端句柄 |
| **单实例锁** | 看不到反馈时用户会再点一次程序 → 第二个更新器同时覆盖同一批文件并各自重启（文件错乱 + 双实例）。锁文件 `.update_apply.lock` 置于覆盖目标下，15 分钟过期后可由下一个更新器接管 |
| **重启必须接回控制台句柄** | 更新器用 `exec` 重启主程序时若 Std* 为 nil，Go 会把它们接到 NUL 设备 → 重启后的实例照常运行、前端窗口照常打开，**控制台却永远空白也收不到按键**。做法：由 `CONOUT$`/`CONIN$` 取控制台句柄交给子进程，且**两个都必须 `O_RDWR`**——只写打开 `CONOUT$` 会让 `GetConsoleMode`/尺寸查询报 Access denied（子进程据此判定"输出不是终端"），只读打开 `CONIN$` 会让 bubbletea 进原始模式时报 `error making raw: Access is denied` |
| **句柄不可信 → 控制台自愈** | 继承来的标准流可能是管道、NUL 或只写句柄，因此 TUI 在 `Install` 时与进事件循环前都会**自己重新取** `CONOUT$`/`CONIN$`（见 `console.go`）：渲染目标与输入来源都不依赖上游交接质量。只要进程还挂在控制台上，界面就一定出得来——即便上游是旧版更新器（交 NUL 句柄）也一样 |
| **窗口缩放必须自己轮询** | bubbletea 在 Windows 上没有 SIGWINCH（`signals_windows.go` 里 `listenForResize` 直接关闭），**尺寸只在启动时取一次**。用户把窗口改小后，框架仍按旧尺寸渲染整屏：多出来的行被终端顶掉，每帧都滚一次（spinner 约 10 次/秒），表现为「控制台不停向上滚、把字滚没了」，而且尺寸再也不会被重新读取，**永远不会自愈**。做法：`watchTerminalSize` 每 250 ms 读一次 `GetConsoleScreenBufferInfo` 的可视窗口尺寸，变化即投递 `WindowSizeMsg`（顺带补上了 Windows 上原本完全缺失的缩放跟随能力） |
| **控制台必须是 UTF-8 代码页** | TUI 输出的是 UTF-8 字节，而经典 conhost（双击 exe / `conhost cmd`）默认用系统 OEM 代码页——中文 Windows 上是 **936/GBK**。不切代码页时中文与 `╭ ─ │ ◆` 全被按 GBK 逐字节解释：屏幕乱码，且「一个 UTF-8 字形」被解成别的字符后实际占位与排版计算完全对不上，每行都超出终端宽度 → 折行 → 整屏翻倍 → 每帧上滚。**这就是用户实测的「一打开就滚动 + 显示混乱」**。做法：`ensureConsoleEncoding` 在渲染前把输入/输出代码页切到 65001（退出时恢复），并开启 VT 处理 |
| **字形宽度必须实测，不能只用内置宽度表** | 中文控制台字体（新宋体等）把一批「东亚歧义宽度」字形渲染成 **2 列**（实测 `◆ ■ ▶ · → ↑ ↓ — × ① …` 共 35 个），而 x/ansi 的宽度表记 1 列。界面按「恰好铺满」排版，度量若按 1 列算，整行就比终端宽 → 折行（实测 140 列窗口渲染出 142 列的行）。做法：启动时 `calibrateGlyphWidths` 在真实控制台上逐个字形实测（写一个字形读光标位移，事后原样恢复屏幕），`dispWidth`/`truncTo` 按实测宽度计算；`TestAllUIGlyphsAreProbed` 保证界面上每个非 ASCII 装饰字形都已登记进探测表。**只做字形替换是不够的**——行宽在排版阶段就已算出，度量为1列时替换只能维持那个已经错误的宽度 |
| **末列留空** | 经典 conhost 上「正好写满整行」的行在换行时会丢掉最后一个字符（实测两条框线的右角 `╮ ╯` 直接消失）。`frame.renderWidth()` 让出最后一列，框线恒为 `终端宽度 - 1` 列 |
| **隐形实例必须变成显式失败** | 只有「连控制台设备都取不到」（服务/计划任务等无控制台环境）时才报错退出，绝不留下一个看不见、按不动的后台进程去占端口。诊断写 `CONOUT$`（不依赖已被接管的标准流） |
| **实例端口必须独占绑定** | Windows 的 bind 默认允许同一地址:端口被多个套接字绑定（后绑定者接管新连接），而 Go 自 1.19 起在 Windows 上**不再**设置 `SO_EXCLUSIVEADDRUSE`。缺了这道闸门，重复启动的两个实例会同时「监听成功」、各自拉起一个模型服务并互抢端口——用户看到的是「前端 404 → 前后端一起崩溃退出」。做法：`server.ListenInstance` 在 `net.ListenConfig` 里显式设置 `SO_EXCLUSIVEADDRUSE`（Winsock 定义为 `~SO_REUSEADDR` = -5），main 在任何装配动作**之前**预检并复用它；失败时用 `exitVisible` 打印原因（此时日志管线尚未接管 stdout，且留 15 秒阅读时间，双击启动也读得到） |

排查入口：更新器把每一步写进 `<程序根>/local_data/logs/update_apply.log`（带时间戳），
覆盖前的旧文件备份在 `<程序根>/.backup/<时间戳>/`。

---

## 13.5 日志与流式管线

| 环节 | 实现 |
|------|------|
| 安装 | `Install()`（幂等）：用管道接管 `os.Stdout` / `os.Stderr`，并为 `LoggerGeneral` 挂日志钩子；**必须在任何子系统装配前调用** |
| 落盘 | 管道扫到的每一行同时写入 `{程序根}/local_data/logs/console.log`（剥 ANSI、4 MB 滚动、保留 3 份）。**进程退出后仍有据可查**——含 Go 运行时的 panic 栈，它们同样经 stderr 进入本管道 |
| 分路由 | `routeLogRecord` 按模块 / 子模块 / 级别把记录投递到原始日志、月华 JS、LTP9、绘图、Go 警告等来源缓冲 |
| 外部入口 | `PushTaskLog`（运维任务回显）、`PushEngineLog`（sd.cpp 引擎行） |
| 缓冲 | 每来源 1000 条环形缓冲，`wakeTUI` 以容量 1 的非阻塞通知唤醒界面 |
| 渲染 | 各日志屏按 `(条数, 折行宽度)` 缓存渲染结果，只在变化时重建 |
| 智能体流式 | `StreamBegin` / `StreamDelta` / `StreamTool` / `StreamEnd` 写入轮次缓冲，`GetTurns` 供实况屏渲染；`RecordModelCall` 计入运行状态屏的计量 |
| 真实终端 | `GetRealStdout()` 返回接管前的终端句柄；`Run()` 用它同时作为 Bubble Tea 输出与 **Lip Gloss 渲染器**的写入目标——否则渲染器会因「输出不是 TTY」剥掉全部颜色 |

---

## 13.6 新增一个屏幕

1. `tui.go`：在 `screenID` 增加常量；以**带字段名**的字面量向 `menuItems` 增加条目（键 / 标题 / 说明 / 分组 / 目标屏）。
2. 新建或就地扩展 `view_*.go`：定义状态结构体（列表用 `listPane`，长文本用 `textPane`）、`updateX`、`viewX`。
3. `viewX` 的骨架固定为：

   ```go
   f := m.frameFor("屏幕名", 上下文, xKeys(m.x))  // 标题栏 + 底栏
   w, h := f.bodySize()                          // 内容区尺寸
   body := ...                                   // 任意行数的内容
   f.status = dimStyle.Render(posText(...))       // 底栏右侧（可选）
   return f.render(body)                          // 裁剪/补齐后成屏
   ```

4. 在 `tui.go` 的 `View` / `Update` 分发与 `screenMeta()` 中登记；键位提示只写一处 `xKeys`，底栏与帮助页共用。
5. 在 `layout_test.go` 的 `screenCases()` 补一个用例（`TestEveryScreenReachable` 会强制要求覆盖）。

---

## 13.7 测试与验证

| 测试 | 覆盖 |
|------|------|
| `TestAllScreensFitTerminal` | 全屏 × 6 种终端尺寸（含 30×8 过小态）的边界守卫：行数 ≤ 高度、每行宽度 ≤ 宽度、正常尺寸下框体恰好满屏 |
| `TestKeyWalkNoPanic` | 交互遍历：各屏依次投递 35 种按键 + 中途缩放，全程不 panic 且不越界（命令一律丢弃，无网络/磁盘副作用） |
| `TestMarketPublishListScrollsWithinTerminal` | 发布选包列表 40 项 / 24 行终端：不越界，且选中项逐条下移时窗口跟随滚动 |
| `TestLogScreenScrollsAndPausesFollow` | 日志屏默认跟随、上翻自动暂停、`End` 恢复跟随 |
| `TestListPane*` · `TestWindowLinesFollowAndScroll` | 定高窗口契约：选中项可见、行宽守恒、滚动条、跟随语义 |
| `TestWidthHelpersCJK` · `TestRowBuilderExactWidth` | 中文宽度对齐、行组装恰好铺满、选中行底色逐片段覆盖 |
| `TestClassify` · `TestIsUpdateRelease` · `TestUpdateAssets` · `TestReleaseKindIsolation` | 发布通道判别：附件优先于标记、历史更新包可识别、附件过滤不混入市场包 |
| `TestPullLatestPicksNewestUpdatePack` · `TestPullLatestSkipsMarketAndEmpty` · `TestListVersionsNewestFirst` · `TestReleasesQueryAsksDescending` · `TestSortReleasesNewestFirstFallback` | 发布排序：响应按 asc 给出时「拉取最新」仍取最新、跳过市场/空发布、列表最新在前、请求带 direction=desc、时间戳缺失时按标签自然序 |
| `TestListPackagesNewestFirstWithLimit` · `TestListPackagesNewestFirstNoLimit` · `TestListPackagesRequestsDescending` | 市场侧同样最新在前，且 `limit` 截断的是「最新的 N 个」 |
| `TestCompareTagNatural` · `TestNewer` | 排序原语：数字段按数值比较（`-930` < `-1030`）、时间戳优先、缺失时退化到标签 |
| `TestBuildReleaseBody` | 发行说明含通道标记与「这是什么/怎么装」，且 SHA-256 行格式与拉取端解析约定一致 |
| `TestWaitFileArgs` · `TestSamePath` · `TestAcquireApplyLock` · `TestConsoleHandles` | 更新交接：不解锁等待自身镜像、路径等价判定、单实例锁与过期锁接管、重启前的控制台句柄获取 |
| `TestConsoleHandleHandoff` · `TestResolveRenderTarget` | 控制台句柄交接：`consoleOut()`（O_RDWR）交给子进程后子进程必须认作终端；继承句柄不可用时渲染目标必须自愈 |
| `TestWatchTerminalSizePushesChanges` · `TestWatchTerminalSizeNoConsole` | 终端尺寸轮询：尺寸变化必须上报（否则 Windows 上窗口缩小后界面会一直向上滚）；无控制台时立即退出不留空转 goroutine |
| `TestScreensFitCJKConsole` · `TestDispWidthFollowsCalibration` · `TestTruncToTerminalWidth` · `TestFitWidthTerminalWidth` · `TestRuneTermWidth` | 中文控制台字体（歧义字形占 2 列）下每屏都不折行、度量与截断/补齐都按终端宽度收敛 |
| `TestAllUIGlyphsAreProbed` · `TestCalibrateGlyphWidthsWorks` · `TestCalibrateGlyphWidthsRestoresScreen` | 界面上每个非 ASCII 装饰字形都已登记进探测表；字形宽度校准真的产出结果且不破坏屏幕内容 |
| `TestListenInstanceExclusive` | 实例端口独占：同一端口第二次绑定必须失败（重复启动不得同时对外服务） |
| `TestMenuMappingProbe` · `TestMarket*` · `TestTokenInputAcceptsRunes` | 菜单与功能映射、市场向导状态机、输入框焦点与按键投递 |

```powershell
# 全量测试（含排版边界守卫）
go test ./subsystem/tui_system/...
# 人工核对排版：在终端里打印各屏在固定尺寸下的实际渲染
go test -run TestRenderPreview -v ./subsystem/tui_system/
```

测试通过 `TestMain` 强制真彩色渲染：无 TTY 时默认渲染器会剥掉样式，宽度断言将退化为纯文本比较而失去意义。

---

## 13.8 取舍与后续方向

- **列表首尾循环**：`↑/↓` 在首尾循环（沿用既有语义），`PgUp/PgDn/Home/End` 则止于两端。
- **搜索 vs 筛选**：`/` 关键词搜索为本地过滤（不触网），`t` 标签筛选走远端接口重新拉取。
- **颜色**：界面样式依赖终端色彩能力，`Run()` 已显式绑定真实终端；重定向输出到文件时会自动退回无色。
- **通道隔离的边界**：判别依赖「标记 + 附件前缀」，因此**手工在 Gitee 网页上新建的发布**不会带标记，
  只有附件前缀能识别；若连附件也不带约定前缀，两条通道都会把它视为「未识别」而跳过（不会串台，但也不会被消费）。
- **可继续演进**：日志级别过滤与关键字高亮、市场安装进度分项展示、屏幕宽度自适应阈值集中化（当前两栏阈值散落在各屏）。
