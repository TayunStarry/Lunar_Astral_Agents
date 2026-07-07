# bedrock_render_engine — 设计决策报告

> **状态**：分析阶段，待用户批准 → 进入实现阶段
> **范围**：本报告仅做架构级决策分析，不含 PoC 代码、不含最终实现
> **生成时间**：2026-06-29
> **依据**：Blockbench 5.1.4 源码（`c:\Users\196530\Downloads\blockbench-master`）+ 纳西妲样本文件（`local_data/documents/`）

---

## 1. 执行摘要

**推荐策略**：**D — 混合重写**（参考 Blockbench 算法 + vendor `molangjs` + 自实现控制器 VM + 自带 Three.js）

**核心判断**：
- Blockbench 源码 Vue 耦合是**渗透式**的（连抽象基类 `OutlinerNode` 都 `import Vue`），策略 B「仅打包运行时」**不可行**
- Blockbench 实际**未实现** Bedrock render method（源码 grep 不到 `opaque/alphatest/translucent`），只做 emissive 字符串嗅探 → 材质系统必须自建
- 样本 .bbmodel **不携带**动画/控制器/MoLang，所有"有趣"特性都在外部 JSON → bedrock_render_engine 必须是**完整 Bedrock 运行时**，不只是 .bbmodel 查看器
- 项目开源 → GPL 不阻断，但策略 A「整包打包」会引入 Three.js 0.129 老旧版本 + 完整编辑器 UI + 全局状态污染，性价比最差

**预期产出**：标准 package（§4.3 骨架），独立 Three.js 实例，完整 Bedrock 几何/骨骼/动画/控制器/MoLang/render method 运行时，通过纳西妲样本（242 骨骼 / 64 动画 / 13 控制器）的播放验证。

---

## 2. 样本与目标

### 2.1 样本清单

| 类型 | 路径 | 规模 |
|---|---|---|
| 几何 | `documents/基岩版实体-纳西妲.bbmodel` | format_version 5.0, bedrock, 1378 cube, 242 骨骼, 5-6 层深度, 1 张 76×72 纹理 |
| 动画 | `documents/纳西妲动画/实体动画_*.json` × 8 | format_version 1.8.0, **64 个动画**, catmullrom 主导, 大量 MoLang |
| 控制器 | `documents/animation_controllers/*.json` × 2 | format_version 1.10.0, **13 个控制器**, ~59 状态, blend_transition + 嵌套引用 |

### 2.2 样本用到的 Bedrock 特性

- MoLang：`q.state_time / life_time / anim_time / delta_time / is_sneaking / is_moving / is_swiming / is_item_name_any / equipped_item_is_attachable / camera_rotation`、`v.*`、`c.is_first_person`、`math.*`、三元 `? :`
- 控制器：`initial_state`（含非 default 的 `jump`）、`blend_transition` + `blend_via_shortest_path`、`on_entry`/`on_exit` 变量赋值、嵌套 controller 引用、`particle_effects`（控制器层）
- 动画：`loop: true` / `hold_on_last_frame`、`lerp_mode: catmullrom`、`pre`/`post` 双值关键帧
- 几何：per-face UV（`box_uv: false`）、纯 cube（无 mesh）

### 2.3 .bbmodel 不携带的特性（必须由运行时支持）

动画数组、控制器状态机、MoLang 表达式、blend_transition、particle_effects、on_entry/exit、sound_effects（样本未用）、嵌套 controller 引用、mesh 几何（样本未用）、多层纹理（样本未用）。

**含义**：bedrock_render_engine 的 UI 必须支持加载**多文件装配**（1 个 .bbmodel + N 个动画 JSON + M 个控制器 JSON），而非单文件查看器。

---

## 3. Blockbench 源码架构关键发现

### 3.1 技术栈

| 组件 | 版本 | 备注 |
|---|---|---|
| Blockbench | 5.1.4 | GPL-3.0-or-later |
| Vue | 2.7.16 | 渗透到数据模型层 |
| Three.js | 0.129.0 | 2021 年版本，ShaderMaterial API 与 r150+ 不兼容 |
| molangjs | 1.7.0 | npm 外部库，**独立可复用** |
| wintersky | 1.3.2 | 粒子系统，**独立可复用** |
| 构建 | esbuild | bundle 为单文件 |

### 3.2 全局状态耦合（关键阻断点）

Blockbench 无 DI 容器，全部走 window 全局：

- `Blockbench.*`：findFileFromContent / showMessageBox / dispatchEvent / read / export / writeFile
- `Project.*`：geometry_name / texture_width / groups / elements / export_path / bedrock_animation_mode
- `Format.*`：box_uv / single_texture / bone_rig / quaternion_interpolation
- `Animator.*`：MolangParser / preview / _last_values
- `Group.all` / `Outliner.elements` / `Canvas` / `Texture.getDefault()` / `Preview.selected.camera`

**最严重的耦合**：`js/outliner/abstract/outliner_node.ts` 第 2 行 `import { Vue }` —— 即使是抽象基类也强依赖 Vue，导致所有 Outliner 类型（Cube/Group/Locator/Mesh 等）都间接依赖 Vue。这是策略 B 不可行的根本原因。

### 3.3 Blockbench 的控制器 VM 真相

`js/animations/animation_controllers.js:1119` 的 `updatePreview()` 是真 VM：

```js
if (mode == 'play' && this.selected_state) {
  for (let transition of this.selected_state.transitions) {
    let match = Animator.MolangParser.parse(transition.condition);
    let target_state = match && this.states.find(s => s.uuid == transition.target);
    if (match && target_state) { target_state.select(); break; }
  }
}
```

**执行模型**：MoLang 求值条件 → 状态切换 → `select()` 触发 on_entry / 动画启停 → `Animator.preview()` 应用骨骼变换。

**不可直接抽离**：触发由 UI 按钮 `BarItems.animation_controller_preview_mode` 驱动，依赖 Animator/Timeline/Preview/Group.scene_object，无独立 tick 循环。**算法可参考，代码必须重写。**

### 3.4 Blockbench 未实现 Bedrock render method

源码 `js/` 全目录 grep `render_method|opaque|alphatest|translucent|double_sided` **零匹配**。仅 `bedrock.js:95-100` 嗅探 material 字符串中的 `emissive`/`multitexture` 关键字切换预览 shader。实际材质差异由：
- `texture.frag.glsl`：alpha < 0.01 discard（事实上的 alphatest 阈值）
- uniform `EMISSIVE`：发光
- `layered.frag.glsl`：三纹理 t0/t1/t2 alpha 混合（multitexture）

**含义**：Bedrock 4 种 render method（opaque / alphatest / translucent / double_sided）必须由 bedrock_render_engine **从零实现**，不能从 Blockbench 移植。

---

## 4. 可抽离模块清单

| 模块 | 职责 | Vue | BB.* | Project | 可抽离性 | 工作量 | 备注 |
|---|---|---|---|---|---|---|---|
| `js/lib/easing.js` | 缓动函数 | 否 | 否 | 否 | **高** | XS | 纯算法，直接复用 |
| `js/util/math_util.js` | 数学工具 | 否 | 否 | 否 | **高** | XS | 纯算法 |
| `js/util/three_custom.js` | Three.js 原型扩展 | 否 | 否 | 否 | **高** | S | 猴子补丁，需同版本 THREE |
| `js/shaders/*.glsl` + `shader.ts` | GLSL 源 + prepareShader | 否 | 否 | 否 | **高** | XS | 纯文本资源 |
| `js/animations/animation_codec.ts` | 动画编解码协议 | 否 | 否 | 否 | **高** | XS | 纯接口 |
| `js/animations/keyframe.js` | 关键帧数据结构 | 否 | 否 | 否 | **高** | S | molang 属性需 Parser 注入 |
| `molangjs`（npm） | MoLang 表达式引擎 | 否 | 否 | 否 | **高** | XS | vendor 即可 |
| `wintersky`（npm） | 粒子系统 | 否 | 否 | 否 | **高** | XS | 样本仅 3 处粒子，可选 |
| `js/formats/bedrock/bedrock.js` `parseGeometry` | Bedrock geo → Cube 树 | 否 | 是 | 是 | 中 | M | 需 mock Project/Group/Cube |
| `js/formats/bbmodel.js` | .bbmodel 解析 | 否 | 是 | 是 | 中 | M | 版本迁移逻辑需保留 |
| `js/animations/timeline_animators.js` `interpolate` | 关键帧插值 | 否 | 是 | 否 | 中 | M | 算法纯，读 Format/Animator |
| `js/formats/bedrock/animation_controller_codec.js` | 控制器 JSON 编解码 | 否 | 是 | 是 | 中 | S | 纯编解码 |
| `js/animations/molang.js` | MoLang 变量绑定 | 否 | 是 | 是 | **低** | L | 绑定层深度耦合，需重写 |
| `js/animations/animation_controllers.js` 运行时 | 状态机 VM | 是 | 是 | 是 | **低** | L | 算法可参考，代码必须重写 |
| `js/texturing/textures.js` | 纹理 + THREE.Material | 否 | 是 | 是 | 中 | M | 强依赖 THREE + canvas |
| `js/preview/preview.js` + `canvas.js` | 预览渲染管线 | 是 | 是 | 是 | **低** | XL | DOM+Vue+全局 scene 深度交织 |
| `js/animations/animation_transform.js` | 编辑器 gizmo→关键帧 | 是 | 是 | 是 | **不可** | — | 编辑器专用 |
| render method 处理 | （无） | — | — | — | **不可** | — | 源码未实现 |

**结论**：高可抽离模块（XS-S 工作量）合计仅覆盖**算法层与协议层**；中可抽离模块需重写适配层；低可抽离模块（MoLang 绑定、控制器 VM、预览渲染）必须从零重写。

---

## 5. 移植策略对比

| 维度 | A 整包打包 | B 仅运行时 | C 纯重写 | **D 混合重写（推荐）** |
|---|---|---|---|---|
| 实现工作量 | XS | L（理论） | XL | **L** |
| 可行性 | 高 | **不可行**（Vue 渗透） | 高 | 高 |
| 维护成本 | 低（跟上游） | 高（rebase patch） | 中 | 中 |
| 性能 | 差（带 UI + Vue 响应式） | 中 | 优 | **优** |
| GPL 合规 | 风险高（传染） | 风险同 A | **无义务** | **无义务**（仅 vendor molangjs/wintersky，它们是 MIT） |
| Three.js 版本 | 锁死 0.129 | 锁死 0.129 | 自由 | **自由（推荐 r160+）** |
| 全局污染 | 严重 | 严重 | 无 | **无** |
| 风险 | UI 残留 + 升级锁死 | 边界难划清 | MoLang 语义还原 | 同 C，但 molangjs 已成熟 |

### 5.1 为何排除 A、B

- **A**：整包打包 5MB+，带完整编辑器 UI、jquery-ui、Vue runtime，启动慢；Three.js 0.129 与项目其他包不兼容；GPL 传染宿主。
- **B**：`OutlinerNode` 抽象基类 `import Vue`，所有 Outliner 类型（Cube/Group/Locator/Mesh/...）间接依赖 Vue；`Animation`/`AnimationController`/`Texture` 通过 `Property` 系统与 `Interface.Panels.*.inside_vue.$forceUpdate` 通信；剥 UI 边界极难划清。**性价比最差。**

### 5.2 为何选 D 而非 C

- **D = C + vendor molangjs + 参考算法**：MoLang 是 Bedrock 的核心表达式语言，自写引擎易出错；`molangjs` 是成熟 MIT 库，vendor 后零依赖运行。
-Blockbench 的 `parseGeometry` 轴翻转规则、`interpolate` 的 catmullrom/bezier 分支、`updatePreview` 的状态机模型，作为**算法参考**重写为独立 ES Module，**不复制代码**，规避 GPL。
- Three.js、wintersky 等也以 vendor 形式打入包内，符合项目「禁止 CDN、本地资源路径」规则。

---

## 6. 推荐方案：策略 D 详解

### 6.1 vendor 资源

| 资源 | 来源 | 许可 | 位置 |
|---|---|---|---|
| `three.module.js` | Three.js r160+ | MIT | `bedrock_render_engine/vendor/three.module.js` |
| `molangjs.min.js` | npm molangjs 1.7.0 | MIT | `bedrock_render_engine/vendor/molangjs.min.js` |
| `wintersky.min.js`（可选） | npm wintersky 1.3.2 | MIT | `bedrock_render_engine/vendor/wintersky.min.js` |
| GLSL shaders | 参考 Blockbench 改写 | 自有 | 内联到 JS 模块 |

### 6.2 自实现模块（参考 Blockbench 算法，不复制代码）

| 模块 | 参考来源 | 职责 |
|---|---|---|
| `geometry-loader.js` | `bedrock.js:parseGeometry` | .bbmodel / Bedrock geo JSON → 骨骼树 + cube 网格 |
| `outliner.js` | `outliner/types/*` | 骨骼层级数据结构（去 Vue） |
| `keyframe.js` | `animations/keyframe.js` | 关键帧数据结构 |
| `interpolator.js` | `timeline_animators.js:interpolate` | 关键帧插值（linear/step/catmullrom/bezier/slerp） |
| `molang-runtime.js` | `animations/molang.js` | MoLang query.* 绑定层 + v.* 存储 |
| `animation-runtime.js` | `timeline_animators.js:BoneAnimator` | 动画 tick → 骨骼变换应用 |
| `controller-vm.js` | `animation_controllers.js:updatePreview` | 控制器状态机 VM（条件求值/切换/blend） |
| `controller-codec.js` | `animation_controller_codec.js` | 控制器 JSON 编解码 |
| `animation-codec.js` | `animation_codec.ts` + `bedrock_animation.js` | 动画 JSON 编解码 |
| `texture-manager.js` | `textures.js` | 纹理加载 + THREE.Texture 封装 |
| `material-system.js` | （自建，Blockbench 未实现） | 4 种 render method（opaque/alphatest/translucent/double_sided） |
| `renderer.js` | `preview.js` + `canvas.js`（重写） | Three.js scene + camera + WebGLRenderer + OrbitControls |
| `host-api.js` | （自建） | 暴露给外部的主机状态（is_sneaking/camera_rotation 等） |

### 6.3 验收测试

以纳西妲样本为唯一验收基准：

1. 加载 `基岩版实体-纳西妲.bbmodel` → 显示 1378 cube + 242 骨骼 + 1 张纹理
2. 加载 8 个动画 JSON → 列出 64 个动画
3. 加载 2 个控制器 JSON → 列出 13 个控制器
4. 播放 `controller.action_in_air` → 状态机按 MoLang 条件转移（default → move → jump → standby...）
5. 验证 `blend_transition` 平滑过渡（无跳变）
6. 验证 `on_entry`/`on_exit` 变量赋值生效（v.standby_animation 等）
7. 60fps @ 1080p（参考硬件：中端独显）

---

## 7. bedrock_render_engine 包架构

```
bedrock_render_engine/
├── DESIGN.md                      ← 本文档
├── metadata.json                  ← 标准元数据
├── index.html                     ← §4.3 骨架，标题『 星月智能 』基岩渲染引擎
├── styles.css                     ← 玻璃拟态 + 暗色模式
├── script.js                      ← 主入口（init/bindEvents/loadData 三段式）
├── app.js                         ← App 类聚合各模块
├── vendor/                        ← 第三方库（本地化）
│   ├── three.module.js
│   ├── molangjs.min.js
│   └── wintersky.min.js（可选）
├── core/                          ← 核心运行时
│   ├── geometry-loader.js         ← .bbmodel / geo JSON 解析
│   ├── outliner.js                ← 骨骼层级
│   ├── keyframe.js                ← 关键帧数据结构
│   ├── interpolator.js            ← 插值算法
│   ├── molang-runtime.js          ← MoLang 绑定层
│   ├── animation-runtime.js       ← 动画 tick
│   ├── controller-vm.js           ← 控制器状态机
│   ├── controller-codec.js        ← 控制器编解码
│   ├── animation-codec.js         ← 动画编解码
│   ├── texture-manager.js         ← 纹理管理
│   ├── material-system.js         ← render method
│   ├── renderer.js                ← Three.js 渲染
│   └── host-api.js                ← 主机状态 API
├── ui/                            ← UI 模块
│   ├── file-loader.js             ← 多文件装配 UI
│   ├── animation-panel.js         ← 动画列表/播放控制
│   ├── controller-panel.js        ← 控制器状态可视化
│   ├── material-panel.js          ← 材质切换
│   └── viewport.js                ← 3D 视口
├── shaders/                       ← GLSL（内联到 JS 也可）
│   ├── texture.vert
│   ├── texture.frag
│   ├── layered.frag
│   └── solid.frag
└── _samples/                      ← 验收样本（符号链接或复制）
    ├── 基岩版实体-纳西妲.bbmodel
    ├── animations/
    └── controllers/
```

### 7.1 与项目规则的合规性

| 规则 | 合规方式 |
|---|---|
| HTML5 + CSS3 + Vanilla JS (ES6+) | ✓ 全部 ES Module |
| 禁止 Python | ✓ |
| 标准依赖库仅 2 个文件 | ✓ 不修改 standard_dependency，自带 vendor/ |
| 禁止外部 CDN | ✓ vendor 本地化 |
| 本地资源路径 `/file/read/package/` | ✓ |
| 标题 `『 星月智能 』模块名` | ✓ |
| 玻璃拟态 + 暗色模式 | ✓ |
| Font Awesome 6.4.0 | ✓（通过 standard_dependency 加载） |
| 响应式 1024/768/480 | ✓ |
| 全局隐藏滚动条 | ✓ |

### 7.2 与 engine_studio 的隔离

- bedrock_render_engine 自带 `vendor/three.module.js`，**不引用** engine_studio 的 Three.js
- 两个包可在同一项目中并存，互不干扰
- engine_studio 继续作为通用 3D 场景/物理运行时；bedrock_render_engine 专注 Bedrock 实体渲染

---

## 8. 风险评估

| 风险 | 概率 | 影响 | 缓解 |
|---|---|---|---|
| **MoLang query.* 语义还原偏差** | 高 | 高 | 对照 Bedrock 官方文档 + 用纳西妲样本逐条验证；molangjs 已实现核心语法，仅需补 query.* 绑定 |
| **控制器 VM blend_transition 曲线实现错误** | 中 | 中 | 参考 `animation_controllers.js:getStateTime` + `blend_transition_curve`；写单元测试对比 Blockbench 预览 |
| **242 骨骼 × 1378 cube 性能不足** | 中 | 高 | Three.js InstancedMesh / 合并几何体；catmullrom 插值查表；MoLang 表达式编译缓存 |
| **catmullrom 关键帧密度极高（基础动画 1705 处）** | 中 | 中 | 插值循环优化；避免每帧创建对象；pre/post 双值处理 |
| **Three.js r160+ 与 Blockbench shader 不兼容** | 高 | 中 | 不直接复用 Blockbench shader，按 r160 API 重写 |
| **嵌套 controller 引用解析（controller → sub-controller）** | 中 | 中 | 装配阶段构建引用图，运行时按图查找 |
| **样本 .bbmodel format_version 5.0 边界 case** | 低 | 低 | 参考 `bbmodel.js:processCompatibility` 的版本迁移逻辑 |
| **bedrock_render_engine 包体过大（vendor 三库）** | 中 | 低 | three.module.js ~600KB + molangjs ~50KB + wintersky ~100KB，合计 < 1MB，可接受 |
| **多文件装配 UI 复杂度** | 中 | 低 | 参考 Blockbench 的 MultiFileRuleset 思路，提供文件拖拽 + 自动关联 |
| **未来 Bedrock 格式演进（1.20+ 新特性）** | 低 | 中 | 模块化设计，编解码与运行时分离，便于扩展 |

### 8.1 不可降级风险

- **MoLang 语义**：必须 100% 还原样本中出现的 query.* 与 v.* 行为，否则控制器状态机无法转移
- **catmullrom 插值**：样本关键帧主导模式，实现错误会直接表现为动画抖动
- **blend_transition**：控制器切换的核心平滑机制，缺失会暴露跳变

---

## 9. 预期效果

### 9.1 显示质量

- per-face UV 正确映射（76×72 纹理 → 1378 cube）
- 骨骼层级变换链正确（5-6 层深度，pivot/rotation 取反轴翻转）
- catmullrom 平滑插值，无抖动
- 4 种 render method 可切换（opaque/alphatest/translucent/double_sided）

### 9.2 动画播放

- 64 个动画可独立播放
- 13 个控制器可按 MoLang 条件自动转移状态
- blend_transition 平滑过渡
- on_entry/on_exit 变量赋值生效
- 嵌套 controller 引用正确解析

### 9.3 性能

- 60fps @ 1080p（纳西妲样本，中端独显）
- 启动时间 < 2s（不含模型加载）
- 模型加载时间 < 1s（1378 cube + 242 骨骼）

### 9.4 材质管理

- 4 种 Bedrock render method 切换
- 纹理加载 + UV 映射
- （可选）multitexture 三层混合

---

## 10. 实施路线图

> **当前阶段**：分析报告完成，等待用户批准
> **下一阶段**：用户批准后进入实现

### Phase 1：基础骨架（XS）
- 创建包结构（metadata.json / index.html / styles.css / script.js）
- vendor three.module.js + molangjs.min.js
- 实现 §4.3 标准骨架 + 玻璃拟态 UI
- 验收：包可被项目主入口识别并打开

### Phase 2：几何与渲染（M）
- `geometry-loader.js`：解析 .bbmodel → 骨骼树 + cube
- `outliner.js`：骨骼层级数据结构
- `renderer.js`：Three.js scene + camera + OrbitControls
- `texture-manager.js`：加载 PNG 纹理 + UV 映射
- 验收：纳西妲 .bbmodel 静态显示，纹理正确

### Phase 3：动画系统（L）
- `keyframe.js` + `interpolator.js`：关键帧 + catmullrom/linear/step/bezier
- `animation-codec.js`：解析 8 个动画 JSON
- `animation-runtime.js`：tick 循环 + 骨骼变换应用
- `molang-runtime.js`：molangjs + query.* 绑定 + v.* 存储
- 验收：64 个动画可独立播放，MoLang 表达式求值正确

### Phase 4：控制器 VM（L）
- `controller-codec.js`：解析 2 个控制器 JSON
- `controller-vm.js`：状态机 + 条件转移 + blend_transition + on_entry/exit
- 嵌套 controller 引用解析
- 验收：13 个控制器自动运行，状态转移正确

### Phase 5：材质系统（M）
- `material-system.js`：4 种 render method
- shader 实现（参考 Blockbench texture.frag + 自建 alpha 逻辑）
- 验收：4 种材质模式可切换

### Phase 6：UI 完善（M）
- 多文件装配 UI（拖拽 + 自动关联）
- 动画列表 + 播放控制
- 控制器状态可视化
- 材质切换面板
- 验收：完整 UX 闭环

### Phase 7：性能优化（S）
- InstancedMesh / 几何体合并
- MoLang 表达式编译缓存
- 插值循环优化
- 验收：60fps @ 1080p

---

## 11. 待决策项（需用户确认）

1. **Three.js 版本**：推荐 r160+（最新稳定）。是否接受？
2. **wintersky 粒子**：样本仅 3 处 particle_effects（splash/impact/ripples）。是否实现？或先跳过，列为 Phase 8？
3. **_samples 目录**：是否将纳西妲样本复制到 `bedrock_render_engine/_samples/`？或仅在 UI 中默认指向 `local_data/documents/`？
4. **主机 API 形态**：`is_sneaking / is_moving / camera_rotation` 等外部状态，通过 (a) UI 按钮模拟、(b) WebSocket 推送、(c) JS API 调用？推荐 (a) + (c) 双模式。
5. **实施启动条件**：本报告批准后立即开始 Phase 1？还是等用户额外确认每个 Phase？

---

## 12. 参考资料

- Blockbench 源码：`c:\Users\196530\Downloads\blockbench-master`
  - `js/formats/bedrock/bedrock.js`（parseGeometry 行 3）
  - `js/formats/bbmodel.js`（codec 行 101、parse 行 397）
  - `js/outliner/abstract/outliner_node.ts` / `outliner_element.ts`
  - `js/animations/timeline_animators.js`（BoneAnimator.interpolate 行 423）
  - `js/animations/animation_controllers.js`（updatePreview 行 1119）
  - `js/animations/molang.js`（query.* 绑定）
  - `js/util/three_custom.js`（THREE 原型补丁）
  - `js/preview/preview.js`（class Preview 行 162）
- 样本文件：`d:\Lunar_Astral_Agents\local_data\documents\`
  - `基岩版实体-纳西妲.bbmodel`
  - `纳西妲动画/实体动画_*.json` × 8
  - `animation_controllers/动画控制器_*.json` × 2
- 项目规则：`.trae/rules/frontend-development-guide.md`
