# 星月智能（Lunar Astral Agents）

基于 **Go + TypeScript + C/C++** 的纯本地化桌面 AI 智能体平台，集成文本推理、图像生成、语音识别、语音合成等功能，采用纯客户端部署方案，无需任何 Python 环境。

---

## 目录

- [项目结构](#项目结构)
- [环境要求](#环境要求)
- [编译流程](#编译流程)
- [系统架构](#系统架构)
- [子系统导航](#子系统导航)
- [常见问题](#常见问题)

---

## 项目结构

<div style="font-family: 'Cascadia Code', 'SF Mono', Consolas, monospace; font-size: 0.9em; line-height: 1.6;">
  <ul style="list-style-type: none; padding-left: 0;">
    <li><strong>Lunar_Astral_Agents/</strong></li>
    <li style="padding-left: 1.5em;"><code>README.md</code> <span style="color: #6a737d;">— 项目主文档（本文件）</span></li>
    <li style="padding-left: 1.5em;"><strong>image/</strong> <span style="color: #6a737d;">— 项目图片资源目录</span>
      <ul style="list-style-type: none; padding-left: 1.5em;">
        <li><code>月华-主页面.webp</code> <span style="color: #6a737d;">— 月华系统主界面截图</span></li>
        <li><code>月华-主界面-手机端.webp</code> <span style="color: #6a737d;">— 月华系统移动端界面</span></li>
        <li><code>月华-聊天记录.webp</code> <span style="color: #6a737d;">— 月华系统聊天记录界面</span></li>
        <li><code>星图-月华-人设图-1.webp</code> <span style="color: #6a737d;">— 月华角色人设图</span></li>
        <li><code>琉璃-主页面.webp</code> <span style="color: #6a737d;">— 琉璃系统主界面截图</span></li>
        <li><code>琉璃-参数管理-配置预览.webp</code> <span style="color: #6a737d;">— 琉璃配置预览界面</span></li>
        <li><code>琉璃-图像生成-参数配置.webp</code> <span style="color: #6a737d;">— 琉璃图像生成参数配置</span></li>
        <li><code>琉璃-图像生成-图片预览.webp</code> <span style="color: #6a737d;">— 琉璃图像生成预览</span></li>
        <li><code>琉璃-截图标注.webp</code> <span style="color: #6a737d;">— 琉璃截图标注界面</span></li>
        <li><code>琉璃-数据管理-主页面.webp</code> <span style="color: #6a737d;">— 琉璃数据管理界面</span></li>
        <li><code>琉璃-数据管理-配置说明.webp</code> <span style="color: #6a737d;">— 琉璃数据配置说明</span></li>
        <li><code>琉璃-文件管理-主页面.webp</code> <span style="color: #6a737d;">— 琉璃文件管理界面</span></li>
        <li><code>琉璃-文件管理-文本编辑.webp</code> <span style="color: #6a737d;">— 琉璃文本编辑界面</span></li>
        <li><code>琉璃-消息渲染.webp</code> <span style="color: #6a737d;">— 琉璃消息渲染界面</span></li>
        <li><code>星图-琉璃-人设图-0.webp</code> <span style="color: #6a737d;">— 琉璃角色人设图</span></li>
        <li><code>多媒体预览-图片0.webp</code> <span style="color: #6a737d;">— 多媒体图片预览</span></li>
        <li><code>多媒体预览-图片1.webp</code> <span style="color: #6a737d;">— 多媒体图片预览</span></li>
        <li><code>多媒体预览-视频.webp</code> <span style="color: #6a737d;">— 多媒体视频预览</span></li>
        <li><code>独立模块-语音合成-0.webp</code> <span style="color: #6a737d;">— 语音合成独立界面</span></li>
        <li><code>独立模块-语音合成-1.webp</code> <span style="color: #6a737d;">— 语音合成独立界面</span></li>
        <li><code>独立模块-语音识别-0.webp</code> <span style="color: #6a737d;">— 语音识别独立界面</span></li>
        <li><code>独立模块-语音识别-1.webp</code> <span style="color: #6a737d;">— 语音识别独立界面</span></li>
        <li><code>旧版宣传图.jpg</code> <span style="color: #6a737d;">— 旧版宣传图片</span></li>
      </ul>
    </li>
    <li style="padding-left: 1.5em;"><strong>local_data/</strong> <span style="color: #6a737d;">— 本地数据与前端资源目录</span>
      <ul style="list-style-type: none; padding-left: 1.5em;">
        <li><code>lunar_package.json</code> <span style="color: #6a737d;">— 前端包配置</span></li>
        <li><strong>models/</strong> <span style="color: #6a737d;">— AI 模型文件目录（GGUF/SafeTensors）</span></li>
        <li><strong>package/</strong> <span style="color: #6a737d;">— 前端共享资源库</span>
          <ul style="list-style-type: none; padding-left: 1.5em;">
            <li><strong>standard_dependency/</strong> <span style="color: #6a737d;">— 全局标准依赖（CSS/JS）</span></li>
            <li><strong>different_lunar/</strong> <span style="color: #6a737d;">— 月华前端主界面（含 Live2D/Markdown/Mermaid 等组件）</span></li>
            <li><strong>database_manager/</strong> <span style="color: #6a737d;">— 数据库管理界面</span></li>
            <li><strong>file_explorer/</strong> <span style="color: #6a737d;">— 文件浏览器界面</span></li>
            <li><strong>image_generation/</strong> <span style="color: #6a737d;">— 图像生成界面</span></li>
            <li><strong>message_rendering/</strong> <span style="color: #6a737d;">— 消息渲染界面</span></li>
            <li><strong>model_query/</strong> <span style="color: #6a737d;">— 模型查询界面</span></li>
            <li><strong>multimedia_preview/</strong> <span style="color: #6a737d;">— 多媒体预览界面</span></li>
            <li><strong>parameter_assistant/</strong> <span style="color: #6a737d;">— 参数助手界面</span></li>
            <li><strong>qwen3_tts/</strong> <span style="color: #6a737d;">— 语音合成界面</span></li>
            <li><strong>screenshot_manager/</strong> <span style="color: #6a737d;">— 截图管理界面</span></li>
            <li><strong>vector_db_manager/</strong> <span style="color: #6a737d;">— 向量数据库管理界面</span></li>
            <li><strong>fontAwesome/</strong> <span style="color: #6a737d;">— Font Awesome 6.4.0 图标库</span></li>
            <li><strong>highlight/</strong> <span style="color: #6a737d;">— highlight.js 代码高亮库</span></li>
            <li><strong>katex/</strong> <span style="color: #6a737d;">— KaTeX 数学公式库</span></li>
            <li><strong>archive/</strong> <span style="color: #6a737d;">— 归档与许可文件</span></li>
            <li><code>echarts.min.js</code> <span style="color: #6a737d;">— ECharts 图表库</span></li>
            <li><code>marked.min.js</code> <span style="color: #6a737d;">— Markdown 渲染库</span></li>
            <li><code>mermaid.min.js</code> <span style="color: #6a737d;">— Mermaid 流程图库</span></li>
            <li><code>live2dcubismcore.min.js</code> <span style="color: #6a737d;">— Live2D Cubism 核心库</span></li>
            <li><code>pixi.5.3.12.min.js</code> <span style="color: #6a737d;">— PixiJS 渲染引擎</span></li>
            <li><code>pixi-live2d-display-cubism4.min.js</code> <span style="color: #6a737d;">— PixiJS Live2D 显示插件</span></li>
            <li><code>qrcode.min.js</code> <span style="color: #6a737d;">— 二维码生成库</span></li>
          </ul>
        </li>
      </ul>
    </li>
    <li style="padding-left: 1.5em;"><strong>lunar_astral/</strong> <span style="color: #6a737d;">— 核心系统：星图·月华</span>
      <ul style="list-style-type: none; padding-left: 1.5em;">
        <li><code>README.md</code> <span style="color: #6a737d;">— 月华系统文档</span></li>
        <li><code>main.go</code> <span style="color: #6a737d;">— 程序入口</span></li>
        <li><code>go.mod</code> <span style="color: #6a737d;">— Go 模块定义</span></li>
        <li><code>build.ps1</code> <span style="color: #6a737d;">— 编译脚本</span></li>
        <li><code>icon.ico</code> <span style="color: #6a737d;">— 应用图标</span></li>
        <li><code>package.json</code> <span style="color: #6a737d;">— Node.js 前端构建配置</span></li>
        <li><code>rollup.config.js</code> <span style="color: #6a737d;">— 前端打包配置</span></li>
        <li><code>tsconfig.json</code> <span style="color: #6a737d;">— TypeScript 配置</span></li>
        <li><code>removeExport.cjs</code> <span style="color: #6a737d;">— 构建后处理脚本</span></li>
        <li><strong>adapters/</strong> <span style="color: #6a737d;">— Go↔JS 适配器层（CGO 桥接）</span>
          <ul style="list-style-type: none; padding-left: 1.5em;">
            <li><code>type.go</code> <span style="color: #6a737d;">— 类型定义</span></li>
            <li><code>create.go</code> <span style="color: #6a737d;">— JS 运行时创建</span></li>
            <li><code>database.go</code> <span style="color: #6a737d;">— 数据库适配</span></li>
            <li><code>file.go</code> <span style="color: #6a737d;">— 文件系统适配</span></li>
            <li><code>message.go</code> <span style="color: #6a737d;">— 消息处理适配</span></li>
            <li><code>network.go</code> <span style="color: #6a737d;">— 网络请求适配</span></li>
            <li><code>vision.go</code> <span style="color: #6a737d;">— 视觉处理适配</span></li>
            <li><code>narrator.go</code> <span style="color: #6a737d;">— 叙述者角色适配</span></li>
            <li><code>chromem.go</code> <span style="color: #6a737d;">— Chromem 向量嵌入适配</span></li>
          </ul>
        </li>
        <li><strong>model/</strong> <span style="color: #6a737d;">— 模型服务层</span>
          <ul style="list-style-type: none; padding-left: 1.5em;">
            <li><code>type.go</code> <span style="color: #6a737d;">— 模型类型定义</span></li>
            <li><code>core.go</code> <span style="color: #6a737d;">— 核心模型逻辑</span></li>
            <li><code>variable.go</code> <span style="color: #6a737d;">— 模型变量</span></li>
            <li><strong>llama/</strong> <span style="color: #6a737d;">— llama.cpp 代理</span>
              <ul style="list-style-type: none; padding-left: 1.5em;">
                <li><code>proxy.go</code> <span style="color: #6a737d;">— 代理核心实现</span></li>
              </ul>
            </li>
            <li><strong>tts/</strong> <span style="color: #6a737d;">— TTS 语音合成引擎</span>
              <ul style="list-style-type: none; padding-left: 1.5em;">
                <li><code>handler.go</code> <span style="color: #6a737d;">— TTS 请求处理</span></li>
                <li><code>cache.go</code> <span style="color: #6a737d;">— 音频缓存</span></li>
              </ul>
            </li>
          </ul>
        </li>
        <li><strong>server/</strong> <span style="color: #6a737d;">— HTTP 服务器层</span>
          <ul style="list-style-type: none; padding-left: 1.5em;">
            <li><code>type.go</code> <span style="color: #6a737d;">— 服务器类型定义</span></li>
            <li><code>create.go</code> <span style="color: #6a737d;">— 服务器创建与启动</span></li>
            <li><code>manage.go</code> <span style="color: #6a737d;">— 服务器管理</span></li>
            <li><code>variable.go</code> <span style="color: #6a737d;">— 端点与变量</span></li>
            <li><strong>handlers/</strong> <span style="color: #6a737d;">— HTTP 请求处理器</span>
              <ul style="list-style-type: none; padding-left: 1.5em;">
                <li><code>type.go</code> <span style="color: #6a737d;">— 处理器类型</span></li>
                <li><code>generate.go</code> <span style="color: #6a737d;">— 图像生成处理</span></li>
                <li><code>message.go</code> <span style="color: #6a737d;">— 消息处理</span></li>
                <li><code>proxy.go</code> <span style="color: #6a737d;">— 代理转发处理</span></li>
                <li><code>video.go</code> <span style="color: #6a737d;">— 视频处理</span></li>
              </ul>
            </li>
          </ul>
        </li>
        <li><strong>release/</strong> <span style="color: #6a737d;">— 进程/端口管理</span>
          <ul style="list-style-type: none; padding-left: 1.5em;">
            <li><code>execute.go</code> <span style="color: #6a737d;">— 命令执行</span></li>
            <li><code>kill.go</code> <span style="color: #6a737d;">— 进程终止</span></li>
            <li><code>network.go</code> <span style="color: #6a737d;">— 网络状态监控</span></li>
            <li><code>processes.go</code> <span style="color: #6a737d;">— 进程列表</span></li>
            <li><code>query.go</code> <span style="color: #6a737d;">— 查询功能</span></li>
          </ul>
        </li>
        <li><strong>hierarchy/</strong> <span style="color: #6a737d;">— 前端资源与脚本</span>
          <ul style="list-style-type: none; padding-left: 1.5em;">
            <li><code>embedded.go</code> <span style="color: #6a737d;">— Go embed 资源嵌入</span></li>
            <li><strong>image/</strong> <span style="color: #6a737d;">— 图像生成模块</span>
              <ul style="list-style-type: none; padding-left: 1.5em;">
                <li><strong>generate/</strong> <span style="color: #6a737d;">— 图像生成</span>
                  <ul style="list-style-type: none; padding-left: 1.5em;">
                    <li><code>generate.go</code> <span style="color: #6a737d;">— 生成逻辑</span></li>
                    <li><code>type.go</code> <span style="color: #6a737d;">— 生成类型</span></li>
                  </ul>
                </li>
                <li><code>video.go</code> <span style="color: #6a737d;">— 视频工具</span></li>
              </ul>
            </li>
            <li><strong>assets/</strong> <span style="color: #6a737d;">— 前端资源</span>
              <ul style="list-style-type: none; padding-left: 1.5em;">
                <li><code>agentSystem.js</code> <span style="color: #6a737d;">— 智能体系统核心 JS</span></li>
                <li><strong>prompts/</strong> <span style="color: #6a737d;">— AI 提示词模板</span>
                  <ul style="list-style-type: none; padding-left: 1.5em;">
                    <li><code>dialogueRole.md</code> <span style="color: #6a737d;">— 对话角色设定</span></li>
                    <li><code>descriptionRole.md</code> <span style="color: #6a737d;">— 描述角色设定</span></li>
                    <li><code>emotionManager.md</code> <span style="color: #6a737d;">— 情绪管理设定</span></li>
                    <li><code>imagePrompt.md</code> <span style="color: #6a737d;">— 图像生成提示</span></li>
                    <li><code>organizeRole.md</code> <span style="color: #6a737d;">— 整理角色设定</span></li>
                    <li><code>painterRole.md</code> <span style="color: #6a737d;">— 画师角色设定</span></li>
                    <li><code>queryKeywords.md</code> <span style="color: #6a737d;">— 关键词查询</span></li>
                    <li><code>recorderRole.md</code> <span style="color: #6a737d;">— 记录角色设定</span></li>
                    <li><code>selfAppearance.md</code> <span style="color: #6a737d;">— 角色外观设定</span></li>
                    <li><code>summaryRole.md</code> <span style="color: #6a737d;">— 摘要角色设定</span></li>
                  </ul>
                </li>
                <li><strong>client/</strong> <span style="color: #6a737d;">— 前端客户端</span>
                  <ul style="list-style-type: none; padding-left: 1.5em;">
                    <li><code>index.html</code> <span style="color: #6a737d;">— 主页面</span></li>
                    <li><code>app.js</code> <span style="color: #6a737d;">— 主应用逻辑</span></li>
                    <li><code>chat.js</code> <span style="color: #6a737d;">— 聊天模块</span></li>
                    <li><code>fetch.js</code> <span style="color: #6a737d;">— 网络请求</span></li>
                    <li><code>file.js</code> <span style="color: #6a737d;">— 文件处理</span></li>
                    <li><code>file-handler.js</code> <span style="color: #6a737d;">— 文件拖拽处理</span></li>
                    <li><code>live2d.js</code> <span style="color: #6a737d;">— Live2D 角色渲染</span></li>
                    <li><code>socket.js</code> <span style="color: #6a737d;">— WebSocket 通信</span></li>
                    <li><code>touch.js</code> <span style="color: #6a737d;">— 触摸交互</span></li>
                    <li><code>tts.js</code> <span style="color: #6a737d;">— 语音合成前端</span></li>
                    <li><code>util.js</code> <span style="color: #6a737d;">— 工具函数</span></li>
                    <li><code>styles.css</code> <span style="color: #6a737d;">— 样式表</span></li>
                    <li><code>favicon.ico</code> <span style="color: #6a737d;">— 网站图标</span></li>
                  </ul>
                </li>
              </ul>
            </li>
          </ul>
        </li>
        <li><strong>websocket/</strong> <span style="color: #6a737d;">— WebSocket 通信层</span>
          <ul style="list-style-type: none; padding-left: 1.5em;">
            <li><code>type.go</code> <span style="color: #6a737d;">— WebSocket 类型</span></li>
            <li><code>variable.go</code> <span style="color: #6a737d;">— WebSocket 变量</span></li>
            <li><code>websocket.go</code> <span style="color: #6a737d;">— WebSocket 核心</span></li>
          </ul>
        </li>
      </ul>
    </li>
    <li style="padding-left: 1.5em;"><strong>crystal_astral/</strong> <span style="color: #6a737d;">— 扩展系统：星图·琉璃</span>
      <ul style="list-style-type: none; padding-left: 1.5em;">
        <li><code>README.md</code> <span style="color: #6a737d;">— 琉璃系统文档</span></li>
        <li><code>main.go</code> <span style="color: #6a737d;">— 程序入口</span></li>
        <li><code>go.mod</code> <span style="color: #6a737d;">— Go 模块定义</span></li>
        <li><code>create.go</code> <span style="color: #6a737d;">— 服务器创建</span></li>
        <li><code>embedded.go</code> <span style="color: #6a737d;">— 资源嵌入</span></li>
        <li><code>endpoint.go</code> <span style="color: #6a737d;">— API 端点定义</span></li>
        <li><code>handler.go</code> <span style="color: #6a737d;">— 请求处理</span></li>
        <li><code>type.go</code> <span style="color: #6a737d;">— 类型定义</span></li>
        <li><code>build.ps1</code> <span style="color: #6a737d;">— 编译脚本</span></li>
        <li><code>icon.ico</code> <span style="color: #6a737d;">— 应用图标</span></li>
        <li><strong>assets/</strong> <span style="color: #6a737d;">— 前端资源</span>
          <ul style="list-style-type: none; padding-left: 1.5em;">
            <li><code>index.html</code> <span style="color: #6a737d;">— 主页面</span></li>
            <li><code>script.js</code> <span style="color: #6a737d;">— 应用逻辑</span></li>
            <li><code>style.css</code> <span style="color: #6a737d;">— 样式表</span></li>
            <li><code>favicon.ico</code> <span style="color: #6a737d;">— 网站图标</span></li>
          </ul>
        </li>
      </ul>
    </li>
    <li style="padding-left: 1.5em;"><strong>subsystem/</strong> <span style="color: #6a737d;">— 可复用子系统模块</span>
      <ul style="list-style-type: none; padding-left: 1.5em;">
        <li><strong>config/</strong> <span style="color: #6a737d;">— 子系统：配置管理</span>
          <ul style="list-style-type: none; padding-left: 1.5em;">
            <li><code>README.md</code> <span style="color: #6a737d;">— 配置模块文档</span></li>
            <li><code>go.mod</code> <span style="color: #6a737d;">— 模块定义</span></li>
            <li><code>init.go</code> <span style="color: #6a737d;">— 配置初始化入口</span></li>
            <li><code>allow.go</code> <span style="color: #6a737d;">— 功能开关</span></li>
            <li><code>engine.go</code> <span style="color: #6a737d;">— 外部引擎配置</span></li>
            <li><code>image.go</code> <span style="color: #6a737d;">— 图像参数</span></li>
            <li><code>model.go</code> <span style="color: #6a737d;">— 模型路径</span></li>
            <li><code>path.go</code> <span style="color: #6a737d;">— 路径配置</span></li>
            <li><code>port.go</code> <span style="color: #6a737d;">— 端口配置</span></li>
            <li><code>system.go</code> <span style="color: #6a737d;">— 运行时状态</span></li>
            <li><code>webview.go</code> <span style="color: #6a737d;">— WebView 窗口配置</span></li>
          </ul>
        </li>
        <li><strong>browser/</strong> <span style="color: #6a737d;">— 子系统：网页前端启动</span>
          <ul style="list-style-type: none; padding-left: 1.5em;">
            <li><code>README.md</code> <span style="color: #6a737d;">— 浏览器模块文档</span></li>
            <li><code>go.mod</code> <span style="color: #6a737d;">— 模块定义</span></li>
            <li><code>execute.go</code> <span style="color: #6a737d;">— IP 发现与启动</span></li>
            <li><code>type.go</code> <span style="color: #6a737d;">— 类型与状态</span></li>
            <li><code>webView.go</code> <span style="color: #6a737d;">— WebView 窗口管理</span></li>
          </ul>
        </li>
        <li><strong>storage/</strong> <span style="color: #6a737d;">— 子系统：文件管理</span>
          <ul style="list-style-type: none; padding-left: 1.5em;">
            <li><code>README.md</code> <span style="color: #6a737d;">— 存储模块文档</span></li>
            <li><code>go.mod</code> <span style="color: #6a737d;">— 模块定义</span></li>
            <li><strong>module/</strong> <span style="color: #6a737d;">— 核心逻辑层</span></li>
            <li><strong>server/</strong> <span style="color: #6a737d;">— HTTP 服务层</span></li>
          </ul>
        </li>
        <li><strong>screenshot/</strong> <span style="color: #6a737d;">— 子系统：屏幕截图</span>
          <ul style="list-style-type: none; padding-left: 1.5em;">
            <li><code>README.md</code> <span style="color: #6a737d;">— 截图模块文档</span></li>
            <li><code>go.mod</code> <span style="color: #6a737d;">— 模块定义</span></li>
            <li><code>type.go</code> <span style="color: #6a737d;">— 类型定义</span></li>
            <li><code>module.go</code> <span style="color: #6a737d;">— 核心逻辑</span></li>
            <li><code>server.go</code> <span style="color: #6a737d;">— HTTP 服务</span></li>
          </ul>
        </li>
        <li><strong>logger/</strong> <span style="color: #6a737d;">— 子系统：彩色日志</span>
          <ul style="list-style-type: none; padding-left: 1.5em;">
            <li><code>go.mod</code> <span style="color: #6a737d;">— 模块定义</span></li>
            <li><code>logger.go</code> <span style="color: #6a737d;">— 彩色终端日志输出</span></li>
          </ul>
        </li>
        <li><strong>LunarTick/</strong> <span style="color: #6a737d;">— 子系统：通用程序执行引擎</span>
          <ul style="list-style-type: none; padding-left: 1.5em;">
            <li><code>README.md</code> <span style="color: #6a737d;">— LunarTick 文档</span></li>
            <li><strong>api/</strong> <span style="color: #6a737d;">— HTTP API 服务层</span></li>
            <li><strong>cmd/lunartick/</strong> <span style="color: #6a737d;">— CLI 入口</span></li>
            <li><strong>engine/</strong> <span style="color: #6a737d;">— 核心引擎（tick 调度/变量/指针/指令）</span></li>
          </ul>
        </li>
        <li><strong>bridge_adapter/</strong> <span style="color: #6a737d;">— 子系统：QQ 群聊适配器</span>
          <ul style="list-style-type: none; padding-left: 1.5em;">
            <li><code>DEVELOPMENT_GUIDE.md</code> <span style="color: #6a737d;">— 开发指南</span></li>
            <li><code>main.go</code> <span style="color: #6a737d;">— 程序入口</span></li>
            <li><strong>pkg/</strong> <span style="color: #6a737d;">— 核心包（config/logger/lunar/message/napcat/types）</span></li>
            <li><strong>template/</strong> <span style="color: #6a737d;">— 消息模板</span></li>
          </ul>
        </li>
        <li><strong>gguf_metadata_viewer/</strong> <span style="color: #6a737d;">— 子系统：GGUF 元数据查看器</span>
          <ul style="list-style-type: none; padding-left: 1.5em;">
            <li><code>README.md</code> <span style="color: #6a737d;">— 查看器文档</span></li>
            <li><code>main.go</code> <span style="color: #6a737d;">— 程序入口</span></li>
            <li><strong>gguf/</strong> <span style="color: #6a737d;">— GGUF 二进制解析</span></li>
            <li><strong>server/</strong> <span style="color: #6a737d;">— HTTP 服务 + 前端界面</span></li>
          </ul>
        </li>
        <li><strong>proxy/</strong> <span style="color: #6a737d;">— 子系统：HTTPS 代理服务器</span>
          <ul style="list-style-type: none; padding-left: 1.5em;">
            <li><code>proxy.go</code> <span style="color: #6a737d;">— 代理核心逻辑</span></li>
            <li><code>certs.go</code> <span style="color: #6a737d;">— TLS 证书管理</span></li>
            <li><strong>cmd/</strong> <span style="color: #6a737d;">— CLI 入口</span></li>
            <li><strong>frontend/proxy_ui/</strong> <span style="color: #6a737d;">— 代理管理界面</span></li>
          </ul>
        </li>
        <li><strong>sd_lunar/</strong> <span style="color: #6a737d;">— 子系统：Stable Diffusion 图像生成</span>
          <ul style="list-style-type: none; padding-left: 1.5em;">
            <li><strong>assets/</strong> <span style="color: #6a737d;">— 前端界面</span></li>
            <li><strong>cpp/</strong> <span style="color: #6a737d;">— C++ GGML 推理引擎</span></li>
          </ul>
        </li>
        <li><strong>volume_archive/</strong> <span style="color: #6a737d;">— 子系统：卷归档管理</span>
          <ul style="list-style-type: none; padding-left: 1.5em;">
            <li><code>main.go</code> <span style="color: #6a737d;">— 程序入口</span></li>
            <li><strong>component/</strong> <span style="color: #6a737d;">— 核心组件（配置/检查/创建/执行/清理等）</span></li>
          </ul>
        </li>
        <li><strong>qwen3_tts_lunar/</strong> <span style="color: #6a737d;">— 独立系统：语音合成</span>
          <ul style="list-style-type: none; padding-left: 1.5em;">
            <li><code>README.md</code> <span style="color: #6a737d;">— TTS 模块文档</span></li>
            <li><code>main.go</code> <span style="color: #6a737d;">— 程序入口</span></li>
            <li><code>go.mod</code> <span style="color: #6a737d;">— 模块定义</span></li>
            <li><code>server.go</code> <span style="color: #6a737d;">— HTTP 服务</span></li>
            <li><code>build.ps1</code> <span style="color: #6a737d;">— 编译脚本</span></li>
            <li><code>build_cpp.ps1</code> <span style="color: #6a737d;">— C++ 编译脚本</span></li>
            <li><code>build_ggml.ps1</code> <span style="color: #6a737d;">— GGML 编译脚本</span></li>
            <li><code>icon.ico</code> <span style="color: #6a737d;">— 应用图标</span></li>
            <li><strong>module/</strong> <span style="color: #6a737d;">— Go 逻辑层</span>
              <ul style="list-style-type: none; padding-left: 1.5em;">
                <li><code>generate.go</code> <span style="color: #6a737d;">— 语音生成</span></li>
                <li><code>variable.go</code> <span style="color: #6a737d;">— 变量定义</span></li>
                <li><code>stream.go</code> <span style="color: #6a737d;">— 流式处理</span></li>
              </ul>
            </li>
            <li><strong>client/</strong> <span style="color: #6a737d;">— 前端界面</span>
              <ul style="list-style-type: none; padding-left: 1.5em;">
                <li><code>index.html</code> <span style="color: #6a737d;">— 主页面</span></li>
                <li><code>app.js</code> <span style="color: #6a737d;">— 应用逻辑</span></li>
                <li><code>style.css</code> <span style="color: #6a737d;">— 样式表</span></li>
                <li><code>picture.webp</code> <span style="color: #6a737d;">— 背景图</span></li>
                <li><code>favicon.ico</code> <span style="color: #6a737d;">— 图标</span></li>
              </ul>
            </li>
            <li><strong>cpp/</strong> <span style="color: #6a737d;">— C++ 推理引擎</span>
              <ul style="list-style-type: none; padding-left: 1.5em;">
                <li><code>CMakeLists.txt</code> <span style="color: #6a737d;">— CMake 构建</span></li>
                <li><strong>src/</strong> <span style="color: #6a737d;">— 引擎源码</span>
                  <ul style="list-style-type: none; padding-left: 1.5em;">
                    <li><code>qwen3_tts.cpp/h</code> <span style="color: #6a737d;">— TTS 主引擎</span></li>
                    <li><code>qwen3tts_c_api.cpp/h</code> <span style="color: #6a737d;">— C API 接口</span></li>
                    <li><code>tts_transformer.cpp/h</code> <span style="color: #6a737d;">— Transformer 层</span></li>
                    <li><code>audio_tokenizer_*.cpp/h</code> <span style="color: #6a737d;">— 音频分词器</span></li>
                    <li><code>gguf_loader.cpp/h</code> <span style="color: #6a737d;">— GGUF 模型加载</span></li>
                    <li><code>text_tokenizer.cpp/h</code> <span style="color: #6a737d;">— 文本分词</span></li>
                    <li><code>main.cpp</code> <span style="color: #6a737d;">— 独立可执行文件入口</span></li>
                    <li><code>coreml_*.cpp/h</code> <span style="color: #6a737d;">— Apple CoreML 加速</span></li>
                    <li><code>qwen3tts.def</code> <span style="color: #6a737d;">— Windows DLL 导出</span></li>
                  </ul>
                </li>
                <li><strong>ggml/</strong> <span style="color: #6a737d;">— GGML 张量计算库</span></li>
              </ul>
            </li>
          </ul>
        </li>
        <li><strong>qwen_asr_lunar/</strong> <span style="color: #6a737d;">— 独立系统：语音识别</span>
          <ul style="list-style-type: none; padding-left: 1.5em;">
            <li><code>README.md</code> <span style="color: #6a737d;">— ASR 模块文档</span></li>
            <li><code>main.go</code> <span style="color: #6a737d;">— 程序入口</span></li>
            <li><code>go.mod</code> <span style="color: #6a737d;">— 模块定义</span></li>
            <li><code>asr.go</code> <span style="color: #6a737d;">— Go↔C 桥接层</span></li>
            <li><code>handler.go</code> <span style="color: #6a737d;">— HTTP 处理</span></li>
            <li><code>build.ps1</code> <span style="color: #6a737d;">— 编译脚本</span></li>
            <li><code>icon.ico</code> <span style="color: #6a737d;">— 应用图标</span></li>
            <li><strong>static/</strong> <span style="color: #6a737d;">— 前端界面</span>
              <ul style="list-style-type: none; padding-left: 1.5em;">
                <li><code>index.html</code> <span style="color: #6a737d;">— 主页面</span></li>
                <li><code>app.js</code> <span style="color: #6a737d;">— 应用逻辑</span></li>
                <li><code>style.css</code> <span style="color: #6a737d;">— 样式表</span></li>
                <li><code>picture.webp</code> <span style="color: #6a737d;">— 背景图</span></li>
                <li><code>favicon.ico</code> <span style="color: #6a737d;">— 图标</span></li>
              </ul>
            </li>
            <li><strong>openblas/</strong> <span style="color: #6a737d;">— OpenBLAS 线性代数库</span>
              <ul style="list-style-type: none; padding-left: 1.5em;">
                <li><strong>include/</strong> <span style="color: #6a737d;">— C 头文件</span></li>
              </ul>
            </li>
            <li><strong>C 推理源码</strong> <span style="color: #6a737d;">— 纯 C 推理引擎</span>
              <ul style="list-style-type: none; padding-left: 1.5em;">
                <li><code>qwen_asr.h/c</code> <span style="color: #6a737d;">— 主入口与管线</span></li>
                <li><code>qwen_asr_audio.h/c</code> <span style="color: #6a737d;">— 音频预处理</span></li>
                <li><code>qwen_asr_encoder.c</code> <span style="color: #6a737d;">— 编码器实现</span></li>
                <li><code>qwen_asr_decoder.c</code> <span style="color: #6a737d;">— 解码器实现</span></li>
                <li><code>qwen_asr_tokenizer.h/c</code> <span style="color: #6a737d;">— GPT-2 BPE 分词</span></li>
                <li><code>qwen_asr_safetensors.h/c</code> <span style="color: #6a737d;">— SafeTensors 加载</span></li>
                <li><code>qwen_asr_kernels.h/c</code> <span style="color: #6a737d;">— 数学核心分发</span></li>
                <li><code>qwen_asr_kernels_avx.c</code> <span style="color: #6a737d;">— x86 SIMD 优化</span></li>
                <li><code>qwen_asr_kernels_neon.c</code> <span style="color: #6a737d;">— ARM NEON 优化</span></li>
                <li><code>qwen_asr_kernels_generic.c</code> <span style="color: #6a737d;">— 通用实现</span></li>
              </ul>
            </li>
          </ul>
        </li>
      </ul>
    </li>
    <li style="padding-left: 1.5em;"><strong>.trae/</strong> <span style="color: #6a737d;">— 项目规则配置</span>
      <ul style="list-style-type: none; padding-left: 1.5em;">
        <li><strong>rules/</strong> <span style="color: #6a737d;">— 代码规范</span>
          <ul style="list-style-type: none; padding-left: 1.5em;">
            <li><code>git-commit-message.md</code> <span style="color: #6a737d;">— Git 提交规范</span></li>
          </ul>
        </li>
      </ul>
    </li>
  </ul>
</div>

### 层级关系说明

<div style="font-family: 'Cascadia Code', 'SF Mono', Consolas, monospace; font-size: 0.9em; line-height: 1.6;">
  <ul style="list-style-type: none; padding-left: 0;">
    <li><strong>星月智能平台 (Lunar Astral Agents)</strong></li>
    <li style="padding-left: 1.5em;"><strong>核心系统: 星图·月华</strong> <span style="color: #6a737d;">(lunar_astral)</span>
      <ul style="list-style-type: none; padding-left: 1.5em;">
        <li><span style="color: #6a737d;">依赖: config, browser, storage, screenshot, qwen3_tts_lunar</span></li>
        <li><span style="color: #6a737d;">功能: AI 对话、Live2D 角色、TTS 语音、图像生成</span></li>
        <li><span style="color: #6a737d;">入口: <code>Lunar_Astral.exe</code></span></li>
      </ul>
    </li>
    <li style="padding-left: 1.5em;"><strong>扩展系统: 星图·琉璃</strong> <span style="color: #6a737d;">(crystal_astral)</span>
      <ul style="list-style-type: none; padding-left: 1.5em;">
        <li><span style="color: #6a737d;">依赖: config, browser, storage, screenshot</span></li>
        <li><span style="color: #6a737d;">功能: 文件管理、数据库管理、截图标注、AI 代理</span></li>
        <li><span style="color: #6a737d;">入口: <code>Crystal_Astral.exe</code></span></li>
      </ul>
    </li>
    <li style="padding-left: 1.5em;"><strong>独立系统: 语音合成</strong> <span style="color: #6a737d;">(qwen3_tts_lunar)</span>
      <ul style="list-style-type: none; padding-left: 1.5em;">
        <li><span style="color: #6a737d;">依赖: C++ GGML 引擎</span></li>
        <li><span style="color: #6a737d;">功能: Qwen3-TTS 文本转语音</span></li>
        <li><span style="color: #6a737d;">入口: <code>Qwen3_TTS_Lunar.exe</code></span></li>
      </ul>
    </li>
    <li style="padding-left: 1.5em;"><strong>独立系统: 语音识别</strong> <span style="color: #6a737d;">(qwen_asr_lunar)</span>
      <ul style="list-style-type: none; padding-left: 1.5em;">
        <li><span style="color: #6a737d;">依赖: 纯 C 引擎 + OpenBLAS</span></li>
        <li><span style="color: #6a737d;">功能: Qwen3-ASR 语音转文本</span></li>
        <li><span style="color: #6a737d;">入口: <code>Qwen_ASR_Lunar.exe</code></span></li>
      </ul>
    </li>
    <li style="padding-left: 1.5em;"><strong>公共子系统</strong> <span style="color: #6a737d;">(subsystem/)</span>
      <ul style="list-style-type: none; padding-left: 1.5em;">
        <li><code>config</code> <span style="color: #6a737d;">— 全局配置中枢</span></li>
        <li><code>browser</code> <span style="color: #6a737d;">— WebView 窗口 + 本地 IP 发现</span></li>
        <li><code>storage</code> <span style="color: #6a737d;">— 文件存储 + SQLite 数据库</span></li>
        <li><code>screenshot</code> <span style="color: #6a737d;">— 屏幕截图 + 图片缩放</span></li>
        <li><code>logger</code> <span style="color: #6a737d;">— 彩色终端日志</span></li>
      </ul>
    </li>
    <li style="padding-left: 1.5em;"><strong>扩展子系统</strong> <span style="color: #6a737d;">(subsystem/)</span>
      <ul style="list-style-type: none; padding-left: 1.5em;">
        <li><code>LunarTick</code> <span style="color: #6a737d;">— 通用程序执行引擎（tick 驱动）</span></li>
        <li><code>bridge_adapter</code> <span style="color: #6a737d;">— QQ 群聊适配器（NapCat ↔ 月华）</span></li>
        <li><code>gguf_metadata_viewer</code> <span style="color: #6a737d;">— GGUF 模型元数据查看器</span></li>
        <li><code>proxy</code> <span style="color: #6a737d;">— HTTPS 代理服务器</span></li>
        <li><code>sd_lunar</code> <span style="color: #6a737d;">— Stable Diffusion 图像生成引擎</span></li>
        <li><code>volume_archive</code> <span style="color: #6a737d;">— 卷归档管理</span></li>
      </ul>
    </li>
  </ul>
</div>

---

## 环境要求

### 操作系统支持

| 系统 | 版本 | 架构 | 状态 |
|------|------|------|------|
| Windows 10 | 21H2 及以上 | x64 | ✅ 支持 |
| Windows 11 | 所有版本 | x64 | ✅ 支持 |
| Windows 10/11 | 32 位 | x86 | ❌ 不支持 |
| Linux | 任意版本 | 任意 | ❌ 不支持 |
| macOS | 任意版本 | 任意 | ❌ 不支持 |

### 开发环境

| 工具 | 最低版本 | 用途 | 安装指南 |
|------|---------|------|---------|
| Go | ≥ 1.25.0 | Go 后端编译 | [go.dev/dl](https://go.dev/dl/) |
| Node.js | ≥ 20.x | TypeScript 前端编译 | [nodejs.org](https://nodejs.org/en/download/) |
| GCC (MinGW-w64) | ≥ 8.1.0 | C/C++ 编译（ASR/TTS） | [mingw-w64.org](https://www.mingw.org/mingw64) |
| CMake | ≥ 3.29.0 | C++ 项目构建（TTS） | [cmake.org](https://cmake.org/download/) |

#### 验证方法

```powershell
# Go 版本验证
go version
# 期望输出: go version go1.25.x windows/amd64

# Node.js 版本验证
node --version
# 期望输出: v20.x.x 或更高

# GCC 版本验证
gcc --version
# 期望输出: gcc (MinGW-W64 ...) 8.1.0 或更高

# CMake 版本验证
cmake --version
# 期望输出: cmake version 3.29.0 或更高
```

### 运行时依赖

| 依赖项 | 版本要求 | 用途 | 下载链接 |
|--------|---------|------|---------|
| CUDA Toolkit | 12.x 或 13.x | GPU 加速推理 | [developer.nvidia.com/cuda-downloads](https://developer.nvidia.com/cuda-downloads) |
| NVIDIA CUDA 驱动 | 与 CUDA 版本匹配 | GPU 驱动支持 | [developer.nvidia.com/cuda-downloads](https://developer.nvidia.com/cuda-downloads) |
| WebView2 Runtime | ≥ 109.0 | 桌面嵌入式浏览器 | [developer.microsoft.com/en-us/microsoft-edge/webview2/](https://developer.microsoft.com/en-us/microsoft-edge/webview2/) |
| FFmpeg | ≥ 5.0.0 | 音频/视频格式转换 | [ffmpeg.org/download.html](https://ffmpeg.org/download.html) |
| Vulkan SDK | ≥ 1.3 | GPU 推理加速 | [lunarg.com/sdk-downloads/vulkan-sdk](https://www.lunarg.com/sdk-downloads/vulkan-sdk) |

#### 验证方法

```powershell
# CUDA 验证
nvidia-smi
# 期望输出: 显示 CUDA 版本 12.x 或 13.x

# WebView2 验证
reg query "HKLM\SOFTWARE\WOW6432Node\Microsoft\EdgeUpdate\Clients\{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}" /v pv 2>nul
# 有输出表示已安装

# FFmpeg 验证
ffmpeg -version
# 期望输出: ffmpeg version 5.0.0 或更高

# Vulkan 验证
vulkaninfo --summary
# 期望输出: 显示 Vulkan Instance Version 1.3 或更高
```

> **注意**：FFmpeg 需要添加到系统环境变量 `PATH` 中，或在 `lunar_config.json` 中配置自定义路径。

---

## 编译流程

### 前置准备

1. 确保已安装所有[开发环境](#开发环境)中的工具
2. 确保已安装所有[运行时依赖](#运行时依赖)
3. 将本仓库克隆到本地（避免路径中包含中文或空格）

### 编译步骤

#### 方式一：一键编译全部（推荐）

```powershell
cd d:\Lunar_Astral_Agents
.\build.ps1
```

根目录的 `build.ps1` 是**统一构建入口**，自动完成环境检查后按顺序编译所有子系统：
月华 → 琉璃 → 桥接适配器 → 卷归档 → 语音合成。每个子系统的 `build.ps1` 均为自包含脚本，内部已处理所有前置步骤（前端编译、GGML 库构建、C++ 引擎编译等）。

#### 方式二：单独编译某个子系统

```powershell
# 编译核心系统——月华（含前端 TypeScript 编译）
cd d:\Lunar_Astral_Agents\lunar_astral
.\build.ps1

# 编译扩展系统——琉璃
cd d:\Lunar_Astral_Agents\crystal_astral
.\build.ps1

# 编译语音识别
cd d:\Lunar_Astral_Agents\subsystem\qwen_asr_lunar
.\build.ps1

# 编译语音合成（含 GGML + C++ 引擎 + Go 服务）
cd d:\Lunar_Astral_Agents\subsystem\qwen3_tts_lunar
.\build.ps1
```

> 各子系统的 `build.ps1` 均为自包含脚本，无需手动执行 `npm install`、`build_ggml.ps1`、`build_cpp.ps1` 等前置步骤，它们已在脚本内部自动处理。

### 编译输出

所有编译产物默认输出到项目根目录：

| 文件 | 所属系统 | 说明 |
|------|---------|------|
| `Lunar_Astral.exe` | 星图·月华 | AI 桌面智能体主程序 |
| `Crystal_Astral.exe` | 星图·琉璃 | 工具集扩展程序 |
| `Qwen_ASR_Lunar.exe` | 语音识别 | 独立语音识别程序 |
| `Qwen3_TTS_Lunar.exe` | 语音合成 | 独立语音合成程序 |

### 编译参数说明

各模块的编译参数已内置于各自的 `build.ps1` 脚本中，无需手动设置。常见编译标志如下（供高级用户参考）：

| 参数 | 适用模块 | 说明 |
|------|---------|------|
| `CGO_ENABLED=1` | ASR、TTS、月华、琉璃 | 启用 CGO（调用 C/C++ 推理引擎） |
| `GOARCH=amd64` | 全部 | 指定目标架构为 64 位 |
| `-tags webview` | 月华、琉璃 | 启用 WebView 桌面窗口支持 |
| `-ldflags="-s -w"` | 全部 | 去除调试符号和 DWARF 信息以减小体积 |
| `-O3 -march=native` | ASR | GCC 最高优化 + 本机指令集 |
| `-DUSE_BLAS` | ASR（可选） | 启用 OpenBLAS 加速 |

### 编译验证

```powershell
# 检查编译产物是否存在
Test-Path d:\Lunar_Astral_Agents\Lunar_Astral.exe   # 应返回 True
Test-Path d:\Lunar_Astral_Agents\Crystal_Astral.exe # 应返回 True
Test-Path d:\Lunar_Astral_Agents\Qwen_ASR_Lunar.exe  # 应返回 True

# 检查文件大小（应大于 10MB）
Get-Item d:\Lunar_Astral_Agents\Lunar_Astral.exe | Select-Object Length
```

### 常见编译错误

| 错误信息 | 原因 | 解决方案 |
|---------|------|---------|
| `go: go.mod file indicates go 1.25, but maximum version is 1.xx` | Go 版本过低 | 升级 Go 到 1.25 或更高版本 |
| `cannot find module for path config` | 未设置 Go workspace | 在项目根目录执行 `go work init` 并添加各个模块 |
| `gcc: command not found` | GCC 未安装 | 安装 MinGW-w64 并确保 `gcc` 在 PATH 中 |
| `CMake Error: Could not find cmake version 3.29` | CMake 版本过低 | 升级 CMake 到 3.29 以上 |
| `undefined reference to cblas_sgemm` | OpenBLAS 未正确链接 | 检查 OpenBLAS 头文件和库文件路径 |
| `CGO_ENABLED=0` 时 C 代码编译失败 | CGO 未启用 | 确保执行 `$env:CGO_ENABLED=1` |

---

## 系统架构

### 整体架构图

```
┌─────────────────────────────────────────────────────────────┐
│                      星月智能平台                             │
│                                                              │
│  ┌─────────────────┐  ┌─────────────────┐                   │
│  │  星图·月华        │  │  星图·琉璃        │                  │
│  │  (AI 桌面智能体)   │  │  (工具集扩展程序)  │                 │
│  │                  │  │                  │                  │
│  │  · AI 对话角色   │  │  · 文件管理      │                  │
│  │  · Live2D 展示   │  │  · 数据库管理    │                  │
│  │  · TTS 语音合成  │  │  · 截图标注      │                  │
│  │  · 图像生成     │  │  · AI 代理转发   │                  │
│  │  · WebSocket    │  │  · 应用加载器    │                  │
│  └────────┬────────┘  └────────┬────────┘                   │
│           │                    │                             │
│           └────────┬───────────┘                             │
│                    │                                         │
│  ┌─────────────────┼─────────────────────────────┐          │
│  │          公共子系统 (subsystem)                 │          │
│  │                                                │          │
│  │  ┌──────────┐ ┌─────────┐ ┌──────────┐        │          │
│  │  │ config   │ │ browser │ │ storage  │        │          │
│  │  │ 配置管理  │ │ 网页前端 │ │ 文件管理  │        │          │
│  │  └──────────┘ └─────────┘ └──────────┘        │          │
│  │  ┌──────────┐ ┌──────────────────┐            │          │
│  │  │screenshot│ │ 独立 AI 引擎      │            │          │
│  │  │屏幕截图   │ │ TTS · ASR        │            │          │
│  │  └──────────┘ └──────────────────┘            │          │
│  └───────────────────────────────────────────────┘          │
│                                                              │
│  ┌──────────────────────────────────────────────────┐      │
│  │              外部推理引擎                          │      │
│  │  ┌──────────────┐ ┌──────────────┐                │      │
│  │  │ llama.cpp    │ │ stable-      │                │      │
│  │  │ (GGUF 推理)   │ │ diffusion.cpp│               │      │
│  │  └──────────────┘ └──────────────┘                │      │
│  └──────────────────────────────────────────────────┘      │
└─────────────────────────────────────────────────────────────┘
```

### 数据流概要

```
用户输入 (前端界面)
    │
    ├─→ HTTP API → llm_proxy → llama-server.exe → GGUF 模型推理
    │                                          ↓
    │                                    推理结果返回
    │                                          ↓
    ├─→ JS 智能体 (goja 运行时) → 角色逻辑处理 → 生成回复
    │                                          ↓
    ├─→ TTS 引擎 → WAV 音频合成
    │
    └─→ WebSocket 推送 → 前端实时渲染 (Markdown/Mermaid/ECharts/Live2D)
```

### 技术栈总览

| 层级 | 技术 | 说明 |
|------|------|------|
| 前端 UI | HTML5 + CSS3 + JavaScript | 嵌入式 WebView 桌面界面 |
| AI 智能体 | TypeScript (goja 运行时) | 在 Go 进程中运行的 JS 智能体 |
| 后端服务 | Go 1.25 | HTTP API + WebSocket + 业务逻辑 |
| 图像生成 | stable-diffusion.cpp | 外部 SD 推理引擎 |
| 文本推理 | llama.cpp (llama-server) | 外部 GGUF 模型推理 |
| 语音合成 | C++ GGML 引擎 | Qwen3-TTS 模型推理 |
| 语音识别 | 纯 C 引擎 + OpenBLAS | Qwen3-ASR 模型推理 |
| 数据存储 | SQLite (go-sqlite3) | 本地嵌入式数据库 |

---

## 子系统导航

| 子系统 | 文档链接 | 功能概要 |
|--------|---------|---------|
| 星图·月华 | [lunar_astral/README.md](lunar_astral/README.md) | AI 桌面智能体核心系统，含对话角色、Live2D、TTS |
| 星图·琉璃 | [crystal_astral/README.md](crystal_astral/README.md) | 工具集扩展系统，含文件/数据库管理、截图标注 |
| 配置管理 | [subsystem/config/README.md](subsystem/config/README.md) | 全局配置中枢，命令行参数 + JSON 双层配置 |
| 网页前端 | [subsystem/browser/README.md](subsystem/browser/README.md) | WebView 窗口管理 + 本地 IP 自动发现 |
| 文件管理 | [subsystem/storage/README.md](subsystem/storage/README.md) | 文件 CRUD + SQLite 数据库 + ZIP 归档 |
| 屏幕截图 | [subsystem/screenshot/README.md](subsystem/screenshot/README.md) | 多显示器截图 + 区域截图 + 图片缩放 |
| 语音合成 | [subsystem/qwen3_tts_lunar/README.md](subsystem/qwen3_tts_lunar/README.md) | Qwen3-TTS 文本转语音引擎 |
| 语音识别 | [subsystem/qwen_asr_lunar/README.md](subsystem/qwen_asr_lunar/README.md) | Qwen3-ASR 语音转文本引擎 |
| LunarTick | [subsystem/LunarTick/README.md](subsystem/LunarTick/README.md) | tick 驱动的通用程序执行引擎 |
| QQ 适配器 | [subsystem/bridge_adapter/DEVELOPMENT_GUIDE.md](subsystem/bridge_adapter/DEVELOPMENT_GUIDE.md) | NapCat ↔ 月华 QQ 群聊消息转发 |
| GGUF 查看器 | [subsystem/gguf_metadata_viewer/README.md](subsystem/gguf_metadata_viewer/README.md) | GGUF 模型文件元数据查看工具 |
| HTTPS 代理 | [subsystem/proxy/](subsystem/proxy/) | HTTPS 代理服务器 + 证书管理 |
| SD 图像生成 | [subsystem/sd_lunar/](subsystem/sd_lunar/) | Stable Diffusion C++ GGML 推理引擎 |
| 卷归档 | [subsystem/volume_archive/](subsystem/volume_archive/) | 卷归档管理工具 |
| 项目架构 | [ARCHITECTURE.md](ARCHITECTURE.md) | 项目架构说明（文件夹层级 + 功能描述） |

---

## 常见问题

### Q: 项目需要 Python 环境吗？

不需要。本项目的设计理念是「零 Python 依赖」。所有 AI 模型推理均由纯 C/C++ 或 Go 实现的本地引擎完成。

### Q: 可以离线使用吗？

完全支持离线使用。所有模型文件均为本地 GGUF 格式，推理过程不需要网络连接。

### Q: 支持哪些 GPU？

通过 llama.cpp 和 stable-diffusion.cpp 支持 NVIDIA CUDA GPU。Vulkan 后端也可用于兼容的 GPU。

### Q: 前端如何修改？

月华系统的前端位于 `lunar_astral/hierarchy/assets/client/` 和 `lunar_astral/server_side/`，修改后重新执行 `lunar_astral\build.ps1` 即可（脚本内部自动处理 TypeScript 编译与打包）。

### Q: 如何添加新的 AI 模型？

将 GGUF 格式的模型文件放入 `{LocalDir}/models/` 目录，并在 `lunar_config.json` 中配置模型路径即可。

---

## 许可证

本项目仅限个人学习与研究使用，未经授权不得用于商业用途。