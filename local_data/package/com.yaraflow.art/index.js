// 绘画插件 - com.yaraflow.art
// 根据聊天内容调用绘图后端生成图片并发送到群聊。

var DEFAULT_CONFIG_YAML = [
  "# 绘画插件 - 配置文件",
  "# 此文件由插件首次运行时自动生成",
  "# 参数说明（全部含中文翻译）：",
  "# models.default_model_id   默认后端ID（下拉框自动同步下方 backends，附带显示名称与接口地址）",
  "# models.backends           绘图后端列表，键为后端ID（建议 model1、model2...），值含 name 显示名称 / base_url 接口地址 / api_key 密钥 / model 模型标识 / format 接口格式 / default_size 默认尺寸 / support_img2img 是否支持图生图 / strength 图生图强度(0.1~1.0，越低越偏离原图，默认0.5) / watermark 水印(仅火山方舟，true加\"AI生成\"水印，false不加，默认不加) / prompt_optimize_mode 提示词优化模式(仅火山方舟，standard标准质量优先/fast极速，默认standard) / background 背景(仅火山方舟，opaque不透明/transparent透明背景，透明仅PNG图生图生效，默认opaque) / custom_prompt_add 附加正面词 / negative_prompt_add 附加负面词",
  "# format 支持的接口格式：openai(OpenAI兼容，含硅基流动/火山方舟/NewAPI) / openai-chat(走chat/completions生图) / doubao(豆包) / gemini(Google Gemini) / modelscope(魔搭异步) / shatangyun(砂糖云NovelAI) / mengyuai(梦羽AI) / zai(Zai) / comfyui(本地工作流)",
  "# 密钥说明：openai/doubao/modelscope/mengyuai/zai 会自动补 Bearer 前缀（裸密钥即可）；gemini 走 x-goog-api-key 无需 Bearer；shatangyun 的 token 放 URL 无需 Bearer；comfyui 无需密钥",
  "# 尺寸说明：参数错误（如模型不支持的档位，400 状态）会自动用 2K 档位重试一次",
  "# selfie.image_path         自我形象图片路径 / selfie.role_description 角色设定描述 / limits.daily_max 每日最多绘图次数",
  "# layer_split.enabled      拆图工具开关（仅 Seedream 5.0 pro 可用；true 自动检测到该模型即注册，false 手动关闭不注册，手动关闭优先于模型检测）",
  "",
  "plugin:",
  "  enabled: true",
  "",
  "models:",
  '  default_model_id: "model1"',
  "  backends:",
  "    model1:",
  '      name: "可图 Kolors（硅基流动）"',
  '      base_url: "https://api.siliconflow.cn/v1"',
  '      api_key: "your-api-key-here"',
  '      model: "Kwai-Kolors/Kolors"',
  '      format: "openai"',
  '      default_size: "1024x1024"',
  "      support_img2img: true",
  "      strength: 0.5",
  "      watermark: false",
  "      prompt_optimize_mode: standard",
  "      background: opaque",
  '      custom_prompt_add: "best quality, masterpiece, ultra-detailed, high resolution, sharp focus"',
  '      negative_prompt_add: "lowres, bad anatomy, bad hands, extra fingers, watermark, text, blurry, low quality"',
  "    model2:",
  '      name: "通义万象 Qwen-Image（魔搭）"',
  '      base_url: "https://api-inference.modelscope.cn/v1"',
  '      api_key: "Bearer YOUR_MODELSCOPE_TOKEN"',
  '      model: "Qwen/Qwen-Image"',
  '      format: "modelscope"',
  '      default_size: "1024x1536"',
  "      support_img2img: false",
  '      custom_prompt_add: "best quality, masterpiece, ultra-detailed, high resolution"',
  '      negative_prompt_add: "lowres, bad anatomy, bad hands, extra fingers, watermark, text, blurry, low quality"',
  "    model3:",
  '      name: "FLUX.1-schnell 快速（魔搭）"',
  '      base_url: "https://api-inference.modelscope.cn/v1"',
  '      api_key: "Bearer YOUR_MODELSCOPE_TOKEN"',
  '      model: "black-forest-labs/FLUX.1-schnell"',
  '      format: "openai"',
  '      default_size: "1024x1536"',
  "      support_img2img: false",
  '      custom_prompt_add: "best quality, masterpiece, ultra-detailed, high resolution"',
  '      negative_prompt_add: "lowres, bad anatomy, bad hands, extra fingers, watermark, text, blurry, low quality"',
  "",
  "selfie:",
  "  enabled: true",
  '  image_path: "data/selfie.png"',
  '  role_description: "一个银色长发、穿着白色连衣裙的少女"',
  "",
  "limits:",
  "  daily_max: 20",
  "",
  "layer_split:",
  "  enabled: true",
  ""
].join("\n");

// ─── 提示词优化系统提示词 ───

// 常规提示词：按即梦/Seedream 官方公式组织（主体→动作→场景→光影→风格→画质），
// 强调具体描述优于抽象词（即梦对材质/光线/视角敏感，抽象词无法解析）
var OPTIMIZER_SYSTEM_PROMPT = "You are a professional AI image prompt engineer. " +
  "Convert the user's description into ONE single line of English image prompt, " +
  "following this structure separated by commas: subject, action or pose, scene and environment, lighting and atmosphere, visual style, quality tags. " +
  "Be specific and concrete about action, expression, lighting, materials and perspective, avoiding vague abstract words. " +
  "If the input contains reference image descriptions in the form [图片N：...], incorporate the visual details of those reference images into the prompt. " +
  "Keep the user's intent faithfully; do not drop style or emotion details. " +
  "Output only the prompt, no explanations, no quotes, no prefix. " +
  "End with: masterpiece, best quality";

// 自拍模式：身份由自拍图固定，LLM 只描述新动作/表情/场景，动作表情写具体（参考项目只生成场景，
// 此处保留并强化动作/表情描述，解决"动作完全一样只改背景"的问题）
var SELFIE_SCENE_SYSTEM_PROMPT = "You are a professional AI image prompt engineer in selfie mode. " +
  "The character being drawn is yourself. Your IDENTITY features (face, hair, eyes, body type) come ONLY from your selfie reference image and the role description; they MUST stay unchanged. " +
  "However, when the user EXPLICITLY requests a specific outfit, clothing, or costume (e.g. swimwear, kimono, school uniform, armor), you MUST include that outfit in the prompt and override the default outfit from the selfie. " +
  "Only when the user does NOT mention any outfit should you keep the original outfit from the selfie/role description. " +
  "The [图片N：...] reference images are NOT your appearance — they provide ONLY style, composition and layout (e.g. three-view sheet layout, chibi cute style); NEVER copy their character's identity (hair color/style, eye color, species, ears, tail, etc.) into the prompt. " +
  "Describe the requested NEW action, pose and facial expression first, then outfit (if user specified), then scene, environment, lighting, atmosphere and visual style, " +
  "in ONE line of English tags separated by commas. " +
  "If the user input or a reference description conflicts with your fixed IDENTITY (face/hair/eyes/body), your fixed identity always wins; but outfit/clothing changes requested by the user always override the default. " +
  "Be specific about the action and expression (e.g. surprised open mouth, waving hand, throwing a kiss) instead of vague words. " +
  "Output only the tags, no explanations, no quotes.";

// 工具参数定义，与 plugin.json tools[0].parameters 保持一致
var TOOL_PARAMETERS = [
  { name: "prompt", type: "string", description: "绘图的自然语言描述。当 draw_self=true 时，只描述动作、姿势、场景、构图、画风，不要描述角色身份特征（发型、发色、瞳色、体型等，这些由自我形象图片固定）；但如果用户明确要求换装（如泳装、和服、校服等），必须把服装要求写进 prompt；示例: 穿泳装站在海滩上，黄昏逆光", required: true },
  { name: "draw_self", type: "boolean", description: "是否画你自己（用配置的自我形象照片做图生图，保持你的身份特征不变）。当用户要求画你自己、你的三视图/立绘/头像等时设为 true；此时 prompt 不要写角色身份特征（发型发色瞳色体型），但可以写用户要求的服装变更（如泳装、和服等）；示例: true", required: false },
  { name: "image_refs", type: "string", description: "参考图片序号，对应聊天记录里的[图片1]、[图片2]...，可传单个序号\"1\"、逗号分隔\"1,3\"或JSON数组\"[1,3]\"。当只有1张参考图且不是画你自己时做普通图生图；当有多张参考图（>=2张），或1张参考图且同时画你自己(draw_self=true)时，自动进入多图融合模式（仅火山方舟后端支持多图）；示例: 1", required: false },
  { name: "style", type: "string", description: "画风，如: 卡通、写实、水彩、赛博朋克，留空用默认", required: false },
  { name: "model_id", type: "string", description: "指定使用的绘图后端ID（在配置页配置），留空用默认后端", required: false },
  { name: "size", type: "string", description: "图片尺寸，示例: 1024x1024，留空用后端默认", required: false }
];

// 拆图工具参数（与 plugin.json tools[1].parameters 保持一致）
var LAYER_SPLIT_PARAMETERS = [
  { name: "image_ref", type: "integer", description: "要拆解的图片序号，对应聊天记录里的[图片1]、[图片2]...，示例: 1", required: true },
  { name: "prompt", type: "string", description: "描述要拆分哪些元素（可选，留空自动拆出全部主要元素），示例: 把人物、标题文字和装饰图标分开", required: false },
  { name: "model_id", type: "string", description: "指定使用的后端ID（需为火山方舟 Seedream 5.0 pro，即梦），留空用默认后端", required: false }
];

// ─── 配置加载 ───

function ensureConfigFile() {
  try {
    var config = yara.config.getFile();
    if (config && Object.keys(config).length > 0) return config;
  } catch (e) {}

  yara.logger.info("绘画插件: 配置文件不存在，自动生成默认配置");
  try {
    yara.file.write("config.yaml", DEFAULT_CONFIG_YAML);
    yara.logger.info("绘画插件: 默认配置文件已生成");
  } catch (e) {
    yara.logger.error("绘画插件: 无法生成配置文件: " + e.message);
  }

  try {
    return yara.config.getFile();
  } catch (e) {
    yara.logger.error("绘画插件: 无法读取配置文件: " + e.message);
    return {};
  }
}

function getConfigValue(path, defaultValue) {
  var config = ensureConfigFile();
  var keys = path.split(".");
  var val = config;
  for (var i = 0; i < keys.length; i++) {
    if (val === null || val === undefined || typeof val !== "object") return defaultValue;
    val = val[keys[i]];
  }
  return (val !== undefined && val !== null) ? val : defaultValue;
}

// ─── 每日次数（零点重置，持久化到 data/daily_count.json） ───

function today() {
  return yara.time.format(yara.time.now(), "2006-01-02");
}

function loadDailyCount() {
  try {
    var raw = yara.file.readData("daily_count.json");
    if (raw === null || raw === undefined || raw === "") {
      return { date: "", count: 0 };
    }
    var data = JSON.parse(raw);
    var count = parseInt(data.count, 10);
    if (isNaN(count)) count = 0;
    return { date: data.date || "", count: count };
  } catch (e) {
    return { date: "", count: 0 };
  }
}

function persistDailyCount(record) {
  try {
    yara.file.writeData("daily_count.json", JSON.stringify(record));
  } catch (e) {
    yara.logger.error("绘画插件: 写入每日计数失败: " + e.message);
  }
}

function getDailyCount() {
  var record = loadDailyCount();
  if (record.date !== today()) {
    record = { date: today(), count: 0 };
    persistDailyCount(record);
  }
  return record.count;
}

function incrementDailyCount() {
  var record = loadDailyCount();
  if (record.date !== today()) {
    record = { date: today(), count: 0 };
  }
  record.count = record.count + 1;
  persistDailyCount(record);
  return record.count;
}

function remainingCount(dailyMax) {
  return dailyMax - getDailyCount();
}

// ─── 提示词优化 ───

function cleanPrompt(text) {
  if (!text) return "";
  var cleaned = String(text).trim();
  cleaned = cleaned.replace(/^\s*(?:Output|Prompt|output|prompt)\s*:\s*/i, "");
  cleaned = cleaned.replace(/^["']|["']$/g, "");
  cleaned = cleaned.replace(/\s+/g, " ").trim();
  return cleaned;
}

function optimizePrompt(description, sceneOnly, roleDescription, imageDescriptions) {
  var systemPrompt = OPTIMIZER_SYSTEM_PROMPT;
  var userPrompt = "";
  if (sceneOnly) {
    systemPrompt = SELFIE_SCENE_SYSTEM_PROMPT;
    if (roleDescription) {
      systemPrompt = systemPrompt + "\nFixed character IDENTITY (face/hair/eyes/body, MUST be preserved): " + roleDescription;
      // 角色身份放进用户输入里给出，比仅放系统提示更强，避免优化器被参考图描述带偏；
      // 身份特征（脸/发/眼/体型）以自画像参考图为准，但用户明确要求的服装变更允许覆盖
      userPrompt = "Fixed character IDENTITY (face/hair/eyes/body from selfie, do NOT override): " + roleDescription + "\n";
    }
  }
  userPrompt += "Input: " + description;
  if (imageDescriptions) {
    userPrompt += "\nReference image descriptions (style/layout reference only):\n" + imageDescriptions;
  }
  userPrompt += "\nOutput:";
  try {
    yara.logger.info("绘画插件: 正在优化提示词（" + (sceneOnly ? "自我形象场景" : "常规") + (imageDescriptions ? "，含参考图描述" : "") + "）");
    var resp = yara.model.chatWithTask("tool_use", [
      { role: "system", content: systemPrompt },
      { role: "user", content: userPrompt }
    ]);
    var content = (resp && resp.content) ? cleanPrompt(resp.content) : "";
    if (content) {
      yara.logger.info("绘画插件: 提示词优化完成: " + previewText(content, 200));
      return { ok: true, prompt: content };
    }
    yara.logger.warn("绘画插件: 提示词优化返回为空，降级使用原始描述");
  } catch (e) {
    yara.logger.error("绘画插件: 提示词优化失败，降级使用原始描述: " + e.message);
  }
  return { ok: false, prompt: description };
}

// 参考图描述直接取自聊天记录里的 [图片N：描述]（视觉分析已生成），不重复识别图片。

function applyStyle(prompt, style) {
  if (!style) return prompt;
  var map = {
    "卡通": "cartoon style",
    "写实": "photorealistic",
    "水彩": "watercolor painting",
    "赛博朋克": "cyberpunk style"
  };
  var tag = map[style] || style;
  if (!prompt) return tag;
  // 风格词前置：即梦/Seedream 优先识别开头的风格词，放前面更稳定
  return tag + ", " + prompt;
}

// ─── 图片处理 ───

function guessMime(b64) {
  if (!b64) return "jpeg";
  if (b64.indexOf("/9j/") === 0) return "jpeg";
  if (b64.indexOf("iVBORw") === 0) return "png";
  if (b64.indexOf("UklGR") === 0) return "webp";
  if (b64.indexOf("R0lGOD") === 0) return "gif";
  return "jpeg";
}

function toDataUri(b64) {
  return "data:image/" + guessMime(b64) + ";base64," + b64;
}

function readSelfieBase64() {
  try {
    var path = getConfigValue("selfie.image_path", "data/selfie.png");
    if (!path) return null;
    // 优先交给主程序按真实字节校验并转 base64（二叉经 Go→JS 字符串后非 UTF-8 头字节会被
    // 替换成 U+FFFD，JS 侧 indexOf("\x89PNG") 永远失败，故不能在 JS 里校验 PNG/JPEG）
    if (typeof yara.image !== "undefined" && yara.image.loadValid) {
      var b64 = yara.image.loadValid(path);
      if (!b64) {
        yara.logger.error("绘画插件: 自我形象图片不是有效图片或读取失败: " + path);
        return null;
      }
      return b64;
    }
    // 兜底：JS 侧读取
    var fileContent = yara.file.read(path);
    if (!fileContent) return null;
    if (!isImageContent(fileContent)) {
      yara.logger.error("绘画插件: 自我形象图片不是有效图片: " + path);
      return null;
    }
    return yara.encoding.base64Encode(fileContent);
  } catch (e) {
    yara.logger.error("绘画插件: 读取自我形象图片失败: " + e.message);
    return null;
  }
}

// 校验字节流是否为有效图片（检查文件头 magic），避免把下载失败的 JSON/HTML 错误体当图片提交
function isImageContent(content) {
  if (!content) return false;
  // 检查常见图片格式 magic bytes
  if (content.indexOf("\x89PNG") === 0) return true;          // PNG
  if (content.indexOf("\xFF\xD8\xFF") === 0) return true;      // JPEG
  if (content.indexOf("GIF8") === 0) return true;              // GIF
  if (content.indexOf("RIFF") === 0 && content.indexOf("WEBP") === 8) return true; // WebP
  if (content.indexOf("BM") === 0) return true;                // BMP
  return false;
}

function downloadRefBase64(url, index, localPath) {
  // 1) 优先通过主程序接口获取参考图（主程序代读自身缓存、缺失自动回退重新下载），
  //    绕开插件沙箱对主程序 data/images 目录的读取限制
  try {
    if (url && typeof yara.image !== "undefined" && yara.image.getCached) {
      var cachedB64 = yara.image.getCached(url);
      if (cachedB64 && cachedB64 !== "") {
        yara.logger.info("绘画插件: 参考图[图片" + index + "] 从主程序缓存获取成功（长度=" + cachedB64.length + "）");
        return cachedB64;
      }
      yara.logger.warn("绘画插件: 参考图[图片" + index + "] 主程序缓存获取失败，回退直连下载");
    }
  } catch (e) {
    yara.logger.error("绘画插件: 参考图[图片" + index + "] 主程序缓存获取异常: " + e.message);
  }

  // 2) 回退：直接下载 URL（QQ 临时链接可能已过期）
  var fileName = "ref_" + index + ".png";
  try {
    var dl = yara.http.download(url, fileName);
    if (!dl || !dl.success || dl.error) {
      return null;
    }
    // 在 Go 侧按真实字节校验是否为图片，防止 QQ 临时链接过期返回错误 JSON 被当图片提交
    // （JS 侧 indexOf("\x89PNG") 会因非 UTF-8 头字节被替换而误判，故不能在 JS 里校验）
    if (typeof yara.image !== "undefined" && yara.image.loadValid) {
      var refB64 = yara.image.loadValid("data/" + fileName);
      if (refB64) {
        return refB64;
      }
      yara.logger.error("绘画插件: 参考图[图片" + index + "] 下载内容不是有效图片（可能链接过期或已被平台移除），已跳过");
      return null;
    }
    var content = yara.file.readData(fileName);
    if (!content) return null;
    if (!isImageContent(content)) {
      yara.logger.error("绘画插件: 参考图[图片" + index + "] 下载内容不是有效图片（可能链接过期或已被平台移除），已跳过");
      return null;
    }
    return yara.encoding.base64Encode(content);
  } catch (e) {
    yara.logger.error("绘画插件: 参考图下载失败: " + e.message);
    return null;
  }
}

// ─── 工具函数 ───

function previewText(text, maxLen) {
  if (!text) return "";
  var s = String(text);
  var len = maxLen || 300;
  return s.length > len ? s.substring(0, len) + "..." : s;
}

// goja 无 sleep API，用时间戳忙等待实现轮询间隔
function sleepMs(ms) {
  var start = yara.time.nowMs();
  while (yara.time.nowMs() - start < ms) {}
}

// ─── API 密钥处理（不同后端鉴权方式不同，部分不需要 Bearer 前缀） ───
// openai / openai-chat / doubao / modelscope / mengyuai / zai 走 Authorization Bearer，裸密钥自动补前缀；
// gemini 走 x-goog-api-key 请求头（不需要 Bearer）；
// shatangyun 的 token 放在 URL 查询参数（不需要 Bearer）；
// comfyui 无需鉴权。

function stripBearer(key) {
  return String(key || "").trim().replace(/^Bearer\s+/i, "");
}

function withBearer(key) {
  var k = stripBearer(key);
  return k ? "Bearer " + k : "";
}

// ─── 统一鉴权（各后端鉴权方式不同） ───
// openai / openai-chat / doubao / modelscope / mengyuai / zai → Authorization Bearer
// gemini → x-goog-api-key 请求头（不补 Bearer）
// shatangyun → token 在 URL 查询参数（不补 Bearer）
// comfyui → 无需鉴权

function buildAuthHeaders(cfg) {
  var key = cfg.api_key || "";
  var format = String(cfg.format || "").toLowerCase();
  var headers = { "Content-Type": "application/json" };
  if (format === "gemini") {
    headers["x-goog-api-key"] = stripBearer(key);
  } else if (format === "shatangyun" || format === "comfyui") {
    // 无需鉴权头
  } else {
    headers["Authorization"] = withBearer(key);
  }
  return headers;
}

// ─── 尺寸处理 ───

// 判断失败是否为尺寸/参数类错误（400 状态），是则用 2K 重试一次
function isSizeParamError(result) {
  if (!result) return false;
  var is400 = result.status === 400 || /状态码\s*400|status\s*cod\w*\s*400/i.test(String(result.error || ""));
  if (!is400) return false;
  var msg = String(result.error || "").toLowerCase();
  return msg.indexOf("size") >= 0 || msg.indexOf("尺寸") >= 0 || msg.indexOf("参数") >= 0 ||
    msg.indexOf("resolution") >= 0 || msg.indexOf("档位") >= 0 || msg.indexOf("不支持") >= 0 ||
    msg.indexOf("invalid") >= 0 || msg.indexOf("非法") >= 0;
}

// ─── 响应解析助手 ───

function extractImage(data) {
  if (!data) return null;
  if (data.data && data.data[0]) {
    var first = data.data[0];
    if (first.b64_json) return { ok: true, b64: first.b64_json };
    if (first.url) return { ok: true, url: first.url };
  }
  if (data.images && data.images[0]) {
    var img = data.images[0];
    if (typeof img === "string") return { ok: true, url: img };
    if (img.url) return { ok: true, url: img.url };
    if (img.b64_json) return { ok: true, b64: img.b64_json };
  }
  if (data.url) return { ok: true, url: data.url };
  if (data.output) return { ok: true, url: data.output };
  if (data.image) return { ok: true, b64: data.image };
  if (data.base64) return { ok: true, b64: data.base64 };
  return null;
}

function extractImageFromChat(data) {
  var img = extractImage(data);
  if (img) return img;
  var content = "";
  try {
    var choices = (data && data.choices) || [];
    if (choices[0] && choices[0].message) content = choices[0].message.content || "";
  } catch (e) {}
  if (!content) return null;
  if (typeof content !== "string") content = String(content);
  var md = content.match(/!\[[^\]]*\]\((https?:\/\/[^\s)]+)\)/);
  if (md) return { ok: true, url: md[1].replace(/[),;]+$/, "") };
  var datauri = content.match(/data:image\/[a-zA-Z]+;base64,([A-Za-z0-9+/=]+)/);
  if (datauri) return { ok: true, b64: datauri[1] };
  if (content.indexOf("/9j/") === 0 || content.indexOf("iVBORw") === 0 || content.indexOf("UklGR") === 0) {
    return { ok: true, b64: content };
  }
  var urlm = content.match(/https?:\/\/\S+(?:\.(?:png|jpg|jpeg|gif|webp)(?:\?[^\s]*)?)?/i);
  if (urlm) return { ok: true, url: urlm[0].replace(/[),;]+$/, "") };
  return null;
}

function cleanB64(b64) {
  var s = String(b64 || "");
  var i = s.indexOf(",");
  if (i >= 0 && s.substring(0, i).indexOf("base64") >= 0) return s.substring(i + 1);
  return s;
}

function mimeOfB64(b64) {
  var c = cleanB64(b64);
  if (c.indexOf("/9j/") === 0) return "image/jpeg";
  if (c.indexOf("iVBORw") === 0) return "image/png";
  if (c.indexOf("UklGR") === 0) return "image/webp";
  if (c.indexOf("R0lGOD") === 0) return "image/gif";
  return "image/jpeg";
}

function isPngB64(b64) {
  return !!b64 && mimeOfB64(b64) === "image/png";
}

// 火山方舟透明背景开关：仅图生图 + 单张 PNG 输入可用（透明通道模式只支持 PNG 图生图，文生图/非PNG/多图输入自动跳过）
function useTransparentBackground(cfg, inputB64) {
  if (Array.isArray(inputB64)) return false;
  var bg = String(cfg.background || "").toLowerCase();
  return bg === "transparent" && isPngB64(inputB64);
}

// 是否火山方舟后端（唯一支持多图融合的后端）
function isArkBackend(cfg) {
  return String(cfg && cfg.base_url || "").indexOf("ark.cn-beijing.volces.com") >= 0;
}

function gcd(a, b) {
  while (b) { var t = a % b; a = b; b = t; }
  return a;
}

function parsePixelSize(size) {
  var m = String(size || "").toLowerCase().match(/(\d+)\s*[x*]\s*(\d+)/);
  if (m) {
    var w = parseInt(m[1], 10), h = parseInt(m[2], 10);
    if (w > 0 && h > 0) return { width: w, height: h };
  }
  return { width: 1024, height: 1024 };
}

// 像素尺寸 → 最接近的 Gemini 支持宽高比
function pixelToGeminiAspect(size) {
  var wh = parsePixelSize(size);
  if (wh.width <= 0 || wh.height <= 0) return null;
  var target = wh.width / wh.height;
  var supported = ["1:1", "16:9", "9:16", "4:3", "3:4", "3:2", "2:3", "4:5", "5:4", "21:9"];
  var best = "1:1", bestDiff = Infinity;
  for (var i = 0; i < supported.length; i++) {
    var p = supported[i].split(":");
    var diff = Math.abs(parseInt(p[0], 10) / parseInt(p[1], 10) - target);
    if (diff < bestDiff) { bestDiff = diff; best = supported[i]; }
  }
  var g = gcd(wh.width, wh.height);
  var aw = wh.width / g, ah = wh.height / g;
  if (supported.indexOf(aw + ":" + ah) >= 0) return aw + ":" + ah;
  return best;
}

function urlEncodeQuery(obj) {
  var parts = [];
  for (var k in obj) {
    if (obj.hasOwnProperty(k)) {
      parts.push(yara.encoding.urlEncode(k) + "=" + yara.encoding.urlEncode(String(obj[k])));
    }
  }
  return parts.join("&");
}

// Gemini / Zai 的图片尺寸配置：宽高比（aspectRatio / image_aspect_ratio）+ 分辨率（imageSize / image_resolution）
function geminiImageConfig(cfg, size) {
  var s = String(size || "").trim() || String(cfg.default_size || "").trim();
  if (!s) return null;
  var config = {};
  var upper = s.toUpperCase();
  if (upper.indexOf("-") === 0) {
    config.imageSize = upper.substring(1);
    config.aspectRatio = "1:1";
  } else if (s.indexOf("-") > 0 && s.indexOf(":") > 0) {
    var pp = s.split("-", 2);
    config.aspectRatio = pp[0].trim();
    if (["1K", "2K", "4K"].indexOf(pp[1].trim().toUpperCase()) >= 0) config.imageSize = pp[1].trim().toUpperCase();
  } else if (s.indexOf(":") >= 0) {
    config.aspectRatio = s;
  } else if (s.toLowerCase().indexOf("x") >= 0 || s.indexOf("*") >= 0) {
    var a = pixelToGeminiAspect(s);
    if (a) config.aspectRatio = a;
  }
  var ml = String(cfg.model || "").toLowerCase();
  if (config.imageSize && ml.indexOf("gemini-3") < 0) delete config.imageSize;
  if (!config.aspectRatio && !config.imageSize) return null;
  return config;
}

function zaiImageConfig(cfg, size) {
  var s = String(size || "").trim() || String(cfg.default_size || "").trim();
  var config = { image_aspect_ratio: "", image_resolution: "" };
  if (!s) return config;
  var upper = s.toUpperCase();
  if (upper.indexOf("-") === 0) {
    config.image_resolution = upper.substring(1);
  } else if (s.indexOf("-") > 0 && s.indexOf(":") > 0) {
    var pp = s.split("-", 2);
    config.image_aspect_ratio = pp[0].trim();
    if (["1K", "2K", "4K"].indexOf(pp[1].trim().toUpperCase()) >= 0) config.image_resolution = pp[1].trim().toUpperCase();
  } else if (s.indexOf(":") >= 0) {
    config.image_aspect_ratio = s;
  } else if (s.toLowerCase().indexOf("x") >= 0 || s.indexOf("*") >= 0) {
    var a = pixelToGeminiAspect(s);
    if (a) config.image_aspect_ratio = a;
  } else {
    config.image_aspect_ratio = "1:1";
  }
  return config;
}

// ─── 后端调用 ───

function callOpenAI(cfg, prompt, size, inputB64) {
  try {
    var endpoint = String(cfg.base_url || "").replace(/\/+$/, "") + "/images/generations";
    var payload = {
      model: cfg.model,
      prompt: prompt,
      size: size,
      n: 1
    };

    // 平台适配（对齐 1021143806_custom_pic_plugin 的多提供商后端处理）
    var baseUrl = String(cfg.base_url || "");
    var isArk = baseUrl.indexOf("ark.cn-beijing.volces.com") >= 0;
    var isSiliconflow = baseUrl.indexOf("siliconflow") >= 0;

    if (isArk) {
      // 火山方舟：生图接口同步返回结果，显式声明响应格式
      // 水印：true 在右下角加"AI生成"水印，false 不加（默认不加）。
      // 必须转成严格布尔：字符串（如 YAML 带引号的 "false"）会被 API 判为 InvalidParameter
      payload.response_format = "url";
      if (cfg.watermark !== undefined && cfg.watermark !== null && cfg.watermark !== "") {
        payload.watermark = (cfg.watermark === true || String(cfg.watermark).toLowerCase() === "true");
      }
      // 提示词优化模式：standard（标准，质量优先，默认）/ fast（极速，更快）
      payload.optimize_prompt_options = { mode: String(cfg.prompt_optimize_mode || "standard").toLowerCase() === "fast" ? "fast" : "standard" };
      // 透明背景：仅图生图 + PNG 输入可用（文生图/非PNG输入不支持，自动跳过）
      if (useTransparentBackground(cfg, inputB64)) {
        payload.background = "transparent";
        payload.output_format = "png"; // 透明模式输出必须为 png，配 jpeg 会报错
      }
    } else if (isSiliconflow) {
      // 硅基流动：image_size 代替 size，batch_size 代替 n
      payload.image_size = payload.size;
      delete payload.size;
      payload.batch_size = payload.n;
      delete payload.n;
      if (cfg.negative_prompt_add) payload.negative_prompt = cfg.negative_prompt_add;
    } else {
      // 其他 OpenAI 兼容服务：附加负面词与可选采样参数
      if (cfg.negative_prompt_add) payload.negative_prompt = cfg.negative_prompt_add;
      if (cfg.guidance_scale) payload.guidance_scale = cfg.guidance_scale;
      if (cfg.num_inference_steps) payload.num_inference_steps = cfg.num_inference_steps;
    }

    if (inputB64) {
      if (Array.isArray(inputB64) && isArk) {
        // 多图融合：仅火山方舟支持 image 数组（2~10张），不设 strength
        payload.image = inputB64.map(toDataUri);
      } else {
        var singleB64 = Array.isArray(inputB64) ? inputB64[0] : inputB64;
        payload.image = toDataUri(singleB64);
        // 图生图强度（0.1~1.0，越低越偏离原图，越高越贴近原图），默认 0.5
        var rawStrength = cfg.strength;
        var strengthVal = (rawStrength === undefined || rawStrength === null || rawStrength === "") ? 0.5 : parseFloat(rawStrength);
        if (isNaN(strengthVal)) strengthVal = 0.5;
        payload.strength = Math.max(0.1, Math.min(1.0, strengthVal));
      }
    }
    var headers = buildAuthHeaders(cfg);
    yara.logger.info("绘画插件: 请求绘图接口 " + endpoint + " model=" + cfg.model + " size=" + size + " img2img=" + (inputB64 ? "true" : "false"));
    var resp = yara.http.post(endpoint, JSON.stringify(payload), headers, 300);
    // 透明背景回退：某些 PNG 实际不含透明通道会被 API 拒绝，去掉 background/output_format 重试一次
    if (payload.background === "transparent" && resp && resp.status === 400 && /transparent|background|alpha|通道|透明/i.test(String(resp.body || ""))) {
      yara.logger.warn("绘画插件: 透明背景被 API 拒绝，回退为不透明背景重试");
      delete payload.background;
      delete payload.output_format;
      resp = yara.http.post(endpoint, JSON.stringify(payload), headers, 300);
    }
    if (!resp || resp.error) {
      var reqErr = (resp && resp.error) ? resp.error : "OpenAI 格式请求失败";
      yara.logger.error("绘画插件: 绘图请求异常: " + reqErr);
      return { ok: false, error: reqErr };
    }
    yara.logger.info("绘画插件: 绘图接口响应 status=" + resp.status + " body=" + previewText(resp.body, 500));
    if (resp.status >= 200 && resp.status < 300) {
      var data;
      try {
        data = JSON.parse(resp.body);
      } catch (pe) {
        return { ok: false, error: "绘图接口返回的不是合法JSON，响应: " + previewText(resp.body, 300) };
      }
      if (data.data && data.data[0] && data.data[0].b64_json) {
        return { ok: true, b64: data.data[0].b64_json };
      }
      if (data.data && data.data[0] && data.data[0].url) {
        return { ok: true, url: data.data[0].url };
      }
      if (data.images && data.images[0] && data.images[0].url) {
        return { ok: true, url: data.images[0].url };
      }
      if (data.url) {
        return { ok: true, url: data.url };
      }
      return { ok: false, error: "状态码 " + resp.status + "，响应成功但未找到图片URL，响应: " + previewText(resp.body) };
    }
    return { ok: false, error: "状态码 " + resp.status + "，响应: " + previewText(resp.body) };
  } catch (e) {
    return { ok: false, error: (e && e.message) ? e.message : String(e) };
  }
}

function callModelScope(cfg, prompt, size, inputB64) {
  try {
    var apiKey = stripBearer(cfg.api_key);
    var baseUrl = String(cfg.base_url || "").replace(/\/+$/, "");
    var headers = {
      "Authorization": "Bearer " + apiKey,
      "Content-Type": "application/json",
      "X-ModelScope-Async-Mode": "true"
    };

    var body;
    if (inputB64) {
      body = { model: cfg.model, prompt: prompt, image_url: [toDataUri(inputB64)] };
    } else {
      body = { model: cfg.model, prompt: prompt };
      if (cfg.negative_prompt_add) body.negative_prompt = cfg.negative_prompt_add;
      if (size) body.size = size;
      body.seed = 42;
      body.steps = 30;
      body.guidance = 3.5;
    }

    yara.logger.info("绘画插件: 请求绘图接口 " + baseUrl + "/images/generations model=" + cfg.model + " img2img=" + (inputB64 ? "true" : "false"));
    var resp = yara.http.post(baseUrl + "/images/generations", JSON.stringify(body), headers);
    if (!resp || resp.error) {
      var reqErr = (resp && resp.error) ? resp.error : "魔搭请求失败";
      yara.logger.error("绘画插件: 绘图请求异常: " + reqErr);
      return { ok: false, error: reqErr };
    }
    if (!(resp.status >= 200 && resp.status < 300)) {
      var respErr = "状态码 " + resp.status + "，响应: " + previewText(resp.body);
      yara.logger.error("绘画插件: 绘图接口异常: " + respErr);
      return { ok: false, error: respErr };
    }
    yara.logger.info("绘画插件: 绘图接口响应 status=" + resp.status + " body=" + previewText(resp.body, 500));

    var data = JSON.parse(resp.body);
    var taskId = data.task_id;
    if (!taskId) {
      return { ok: false, error: "魔搭未返回任务ID，响应: " + previewText(resp.body, 200) };
    }

    var checkHeaders = {
      "Authorization": "Bearer " + apiKey,
      "Content-Type": "application/json",
      "X-ModelScope-Task-Type": "image_generation"
    };

    var maxAttempts = 24;
    for (var i = 0; i < maxAttempts; i++) {
      var sResp = yara.http.get(baseUrl + "/tasks/" + taskId, checkHeaders);
      if (sResp && !sResp.error && sResp.status >= 200 && sResp.status < 300) {
        var sData = JSON.parse(sResp.body);
        var taskStatus = sData.task_status || "UNKNOWN";
        if (taskStatus === "SUCCEED") {
          if (sData.output_images && sData.output_images[0]) {
            var imgUrl = sData.output_images[0];
            var dl = yara.http.download(imgUrl, "gen_" + taskId + ".png");
            if (dl && dl.success && !dl.error) {
              var content = yara.file.readData("gen_" + taskId + ".png");
              if (content) {
                return { ok: true, b64: yara.encoding.base64Encode(content) };
              }
            }
            return { ok: false, error: "魔搭图片下载失败" };
          }
          return { ok: false, error: "魔搭任务成功但未返回图片" };
        } else if (taskStatus === "FAILED") {
          var errMsg = sData.error_message || "任务执行失败";
          return { ok: false, error: "魔搭任务失败: " + errMsg };
        }
      }
      sleepMs(5000);
    }
    return { ok: false, error: "魔搭任务执行超时" };
  } catch (e) {
    return { ok: false, error: (e && e.message) ? e.message : String(e) };
  }
}

// ─── OpenAI Chat Completions 格式（通过 chat/completions 生图） ───

function callOpenAIChat(cfg, prompt, size, inputB64) {
  try {
    var baseUrl = String(cfg.base_url || "").replace(/\/+$/, "");
    var endpoint = baseUrl + "/chat/completions";
    var headers = buildAuthHeaders(cfg);

    var systemMsg = "You are an image generation assistant. Generate an image based on the user's description.";
    if (size) systemMsg += " Target image size: " + size + ".";

    var messages = [{ role: "system", content: systemMsg }];
    var userContent;
    if (inputB64) {
      userContent = [
        { type: "image_url", image_url: { url: toDataUri(inputB64) } },
        { type: "text", text: "Please modify this image based on the following description: " + prompt }
      ];
    } else {
      userContent = "Please generate an image: " + prompt;
    }
    messages.push({ role: "user", content: userContent });

    var payload = { model: cfg.model, messages: messages };
    if (size) payload.size = size;

    yara.logger.info("绘画插件: 请求绘图接口 " + endpoint + " model=" + cfg.model + " size=" + size + " img2img=" + (inputB64 ? "true" : "false"));
    var resp = yara.http.post(endpoint, JSON.stringify(payload), headers, 300);
    if (!resp || resp.error) {
      return { ok: false, error: (resp && resp.error) ? resp.error : "OpenAI-Chat 格式请求失败" };
    }
    if (!(resp.status >= 200 && resp.status < 300)) {
      return { ok: false, error: "状态码 " + resp.status + "，响应: " + previewText(resp.body, 300) };
    }
    var data = JSON.parse(resp.body);
    var img = extractImageFromChat(data);
    if (img) return img;
    return { ok: false, error: "状态码 " + resp.status + "，响应中未找到图片，响应: " + previewText(resp.body) };
  } catch (e) {
    return { ok: false, error: (e && e.message) ? e.message : String(e) };
  }
}

// ─── Gemini 格式（x-goog-api-key 鉴权，响应含 inlineData base64） ───

function callGemini(cfg, prompt, size, inputB64) {
  try {
    var baseUrl = String(cfg.base_url || "https://generativelanguage.googleapis.com").replace(/\/+$/, "");
    var modelName = cfg.model || "gemini-2.5-flash-image-preview";
    var endpoint = baseUrl + "/v1beta/models/" + modelName + ":generateContent";
    var headers = {
      "Content-Type": "application/json",
      "x-goog-api-key": stripBearer(cfg.api_key)
    };

    var parts = [{ text: prompt }];
    if (inputB64) {
      parts.push({ inline_data: { mime_type: mimeOfB64(inputB64), data: cleanB64(inputB64) } });
    }

    var requestData = {
      contents: [{ role: "user", parts: parts }],
      generationConfig: { responseModalities: ["TEXT", "IMAGE"] }
    };
    var imageConfig = geminiImageConfig(cfg, size);
    if (imageConfig) requestData.generationConfig.imageConfig = imageConfig;

    yara.logger.info("绘画插件: 请求绘图接口 " + endpoint + " model=" + modelName + " size=" + size + " img2img=" + (inputB64 ? "true" : "false"));
    var resp = yara.http.post(endpoint, JSON.stringify(requestData), headers, 300);
    if (!resp || resp.error) {
      return { ok: false, error: (resp && resp.error) ? resp.error : "Gemini 格式请求失败" };
    }
    if (!(resp.status >= 200 && resp.status < 300)) {
      return { ok: false, error: "状态码 " + resp.status + "，响应: " + previewText(resp.body, 300) };
    }
    var data = JSON.parse(resp.body);
    var candidates = data.candidates || [];
    if (candidates[0] && candidates[0].content && candidates[0].content.parts) {
      var partsArr = candidates[0].content.parts;
      for (var i = 0; i < partsArr.length; i++) {
        var part = partsArr[i];
        if (part.inlineData && part.inlineData.data) return { ok: true, b64: part.inlineData.data };
        if (part.inline_data && part.inline_data.data) return { ok: true, b64: part.inline_data.data };
      }
    }
    if (data.error) return { ok: false, error: "API错误: " + (data.error.message || JSON.stringify(data.error)) };
    return { ok: false, error: "未找到图片数据，可能模型不支持图片生成" };
  } catch (e) {
    return { ok: false, error: (e && e.message) ? e.message : String(e) };
  }
}

// ─── 砂糖云格式（GET 请求，参数走 URL 查询，token 在 URL 中） ───

function callShatangyun(cfg, prompt, size, inputB64) {
  try {
    var baseUrl = String(cfg.base_url || "https://std.loliyc.com").replace(/\/+$/, "");
    var params = {
      tag: prompt,
      token: stripBearer(cfg.api_key),
      model: cfg.model || "nai-diffusion-4-5-full",
      size: size || cfg.default_size || "832x1216",
      steps: cfg.num_inference_steps || 23,
      scale: cfg.guidance_scale || 5,
      cfg: cfg.cfg || 0,
      sampler: cfg.sampler || "k_euler_ancestral",
      nocache: cfg.nocache || 0,
      noise_schedule: cfg.noise_schedule || "karras"
    };
    if (cfg.artist) params.artist = cfg.artist;
    if (cfg.negative_prompt_add) params.negative = cfg.negative_prompt_add;

    var endpoint = baseUrl + "/generate?" + urlEncodeQuery(params);
    yara.logger.info("绘画插件: 请求绘图接口 " + baseUrl + "/generate model=" + params.model + " size=" + params.size);
    var resp = yara.http.get(endpoint);
    if (!resp || resp.error) {
      return { ok: false, error: (resp && resp.error) ? resp.error : "砂糖云格式请求失败" };
    }
    if (!(resp.status >= 200 && resp.status < 300)) {
      return { ok: false, error: "状态码 " + resp.status + "，响应: " + previewText(resp.body, 300) };
    }
    // 砂糖云直接返回图片二进制，需下载到本地再转 base64
    var fileName = "shatang_" + Date.now() + ".png";
    var dl = yara.http.download(endpoint, fileName);
    if (dl && dl.success && !dl.error) {
      var content = yara.file.readData(fileName);
      if (content) return { ok: true, b64: yara.encoding.base64Encode(content) };
    }
    return { ok: false, error: "砂糖云未返回图片数据: " + previewText(resp.body, 200) };
  } catch (e) {
    return { ok: false, error: (e && e.message) ? e.message : String(e) };
  }
}

// ─── 梦羽AI 格式（/api/v1/generate_image，model 为模型索引） ───

function callMengyuai(cfg, prompt, size, inputB64) {
  try {
    var baseUrl = String(cfg.base_url || "https://sd.exacg.cc").replace(/\/+$/, "");
    var headers = buildAuthHeaders(cfg);
    var width, height;
    var wh = parsePixelSize(size);
    width = wh.width; height = wh.height;

    var requestData = { prompt: prompt };
    if (inputB64) {
      requestData.model_index = cfg.img2img_model_index || 19;
      requestData.image_source = toDataUri(inputB64);
    } else {
      requestData.model_index = parseInt(cfg.model || 0, 10) || 0;
      if (cfg.negative_prompt_add) requestData.negative_prompt = cfg.negative_prompt_add;
      requestData.width = width;
      requestData.height = height;
      requestData.steps = cfg.num_inference_steps || 20;
      requestData.cfg = cfg.guidance_scale || 7.0;
      requestData.seed = (cfg.seed !== undefined && cfg.seed !== -1) ? cfg.seed : -1;
    }

    var endpoint = baseUrl + "/api/v1/generate_image";
    yara.logger.info("绘画插件: 请求绘图接口 " + endpoint + " model_index=" + requestData.model_index + " size=" + width + "x" + height + " img2img=" + (inputB64 ? "true" : "false"));
    var resp = yara.http.post(endpoint, JSON.stringify(requestData), headers, 300);
    if (!resp || resp.error) {
      return { ok: false, error: (resp && resp.error) ? resp.error : "梦羽AI格式请求失败" };
    }
    if (!(resp.status >= 200 && resp.status < 300)) {
      return { ok: false, error: "状态码 " + resp.status + "，响应: " + previewText(resp.body, 300) };
    }
    var data = JSON.parse(resp.body);
    var img = extractImage(data);
    if (img) return img;
    return { ok: false, error: "响应中未找到图片，响应: " + previewText(resp.body, 300) };
  } catch (e) {
    return { ok: false, error: (e && e.message) ? e.message : String(e) };
  }
}

// ─── Zai 格式（OpenAI 兼容，chat/completions，透传宽高比/分辨率） ───

function callZai(cfg, prompt, size, inputB64) {
  try {
    var baseUrl = String(cfg.base_url || "https://zai.is/api").replace(/\/+$/, "");
    var endpoint = baseUrl + "/chat/completions";
    var headers = buildAuthHeaders(cfg);

    var contents = [{ type: "text", text: prompt }];
    if (inputB64) {
      contents.push({ type: "image_url", image_url: { url: toDataUri(inputB64) } });
    }

    var payload = {
      model: cfg.model,
      messages: [{ role: "user", content: contents }],
      stream: false,
      n: 1
    };
    var imageConfig = zaiImageConfig(cfg, size);
    if (imageConfig.image_aspect_ratio) payload.image_aspect_ratio = imageConfig.image_aspect_ratio;
    if (imageConfig.image_resolution) payload.image_resolution = imageConfig.image_resolution;
    if (cfg.seed !== undefined && cfg.seed !== -1) payload.seed = cfg.seed;

    yara.logger.info("绘画插件: 请求绘图接口 " + endpoint + " model=" + cfg.model + " size=" + size + " img2img=" + (inputB64 ? "true" : "false"));
    var resp = yara.http.post(endpoint, JSON.stringify(payload), headers, 300);
    if (!resp || resp.error) {
      return { ok: false, error: (resp && resp.error) ? resp.error : "Zai 格式请求失败" };
    }
    if (!(resp.status >= 200 && resp.status < 300)) {
      return { ok: false, error: "状态码 " + resp.status + "，响应: " + previewText(resp.body, 300) };
    }
    var data = JSON.parse(resp.body);
    var img = extractImageFromChat(data);
    if (img) return img;
    return { ok: false, error: "状态码 " + resp.status + "，响应中未找到图片，响应: " + previewText(resp.body) };
  } catch (e) {
    return { ok: false, error: (e && e.message) ? e.message : String(e) };
  }
}

// ─── ComfyUI 格式（本地工作流 API） ───
// 说明：受 SSRF 防护限制，仅支持可公网访问或白名单的 ComfyUI 地址；
//       本地 127.0.0.1 实例会被 SSRF 拦截，若需本地使用请在宿主环境配置放行。
// 工作流文件放在插件目录 workflow/ 下，model 字段填写文件名，支持 ${prompt} ${size} ${seed} 占位符。

function callComfyui(cfg, prompt, size, inputB64) {
  try {
    var baseUrl = String(cfg.base_url || "http://127.0.0.1:8188").replace(/\/+$/, "");
    var workflowName = cfg.model || "";
    if (!workflowName) return { ok: false, error: "未配置工作流文件名（model 字段）" };

    var workflowJson = yara.file.read("workflow/" + workflowName);
    if (!workflowJson) return { ok: false, error: "工作流文件不存在: workflow/" + workflowName };

    // 替换占位符
    var wf = workflowJson;
    wf = wf.split("${prompt}").join(prompt);
    if (size) {
      var wh = parsePixelSize(size);
      wf = wf.split("${width}").join(wh.width);
      wf = wf.split("${height}").join(wh.height);
    }
    if (cfg.negative_prompt_add) wf = wf.split("${negative_prompt}").join(cfg.negative_prompt_add);
    if (cfg.guidance_scale) wf = wf.split("${cfg}").join(cfg.guidance_scale);
    if (cfg.num_inference_steps) wf = wf.split("${steps}").join(cfg.num_inference_steps);
    var seed = cfg.seed;
    if (seed === undefined || seed === null || seed === -1) seed = Math.floor(Math.random() * 1e10) + 1;
    wf = wf.split("${seed}").join(seed);

    var workflow = JSON.parse(wf);
    yara.logger.info("绘画插件: 请求绘图接口 " + baseUrl + "/prompt workflow=" + workflowName + " img2img=" + (inputB64 ? "true" : "false"));
    var resp = yara.http.post(baseUrl + "/prompt", JSON.stringify({ prompt: workflow }));
    if (!resp || resp.error) {
      return { ok: false, error: (resp && resp.error) ? resp.error : "ComfyUI 提交任务失败" };
    }
    if (!(resp.status >= 200 && resp.status < 300)) {
      return { ok: false, error: "状态码 " + resp.status + "，响应: " + previewText(resp.body, 300) };
    }
    var data = JSON.parse(resp.body);
    var promptId = data.prompt_id;
    if (!promptId) return { ok: false, error: "ComfyUI 未返回 prompt_id" };

    // 轮询 history 等待生成完成
    var maxAttempts = 120;
    var fileName = "";
    for (var i = 0; i < maxAttempts; i++) {
      var hResp = yara.http.get(baseUrl + "/history/" + promptId);
      if (hResp && !hResp.error && hResp.status >= 200 && hResp.status < 300) {
        var hData = JSON.parse(hResp.body);
        var entry = hData[promptId];
        if (entry && entry.outputs) {
          for (var nodeId in entry.outputs) {
            if (entry.outputs.hasOwnProperty(nodeId)) {
              var nodeOut = entry.outputs[nodeId];
              if (nodeOut.images && nodeOut.images[0] && nodeOut.images[0].filename) {
                fileName = nodeOut.images[0].filename;
                break;
              }
            }
          }
          if (fileName) break;
        }
      }
      sleepMs(1000);
    }
    if (!fileName) return { ok: false, error: "等待 ComfyUI 生成结果超时" };

    var imgResp = yara.http.get(baseUrl + "/view?filename=" + fileName);
    if (imgResp && !imgResp.error && imgResp.status >= 200 && imgResp.status < 300) {
      var bin = imgResp.body;
      if (bin) return { ok: true, b64: yara.encoding.base64Encode(bin) };
    }
    return { ok: false, error: "ComfyUI 图片下载失败" };
  } catch (e) {
    return { ok: false, error: (e && e.message) ? e.message : String(e) };
  }
}

function invokeBackend(format, cfg, prompt, size, inputB64) {
  switch (format) {
    case "modelscope":
      return callModelScope(cfg, prompt, size, inputB64);
    case "openai-chat":
      return callOpenAIChat(cfg, prompt, size, inputB64);
    case "gemini":
      return callGemini(cfg, prompt, size, inputB64);
    case "shatangyun":
      return callShatangyun(cfg, prompt, size, inputB64);
    case "mengyuai":
      return callMengyuai(cfg, prompt, size, inputB64);
    case "zai":
      return callZai(cfg, prompt, size, inputB64);
    case "comfyui":
      return callComfyui(cfg, prompt, size, inputB64);
    default:
      return callOpenAI(cfg, prompt, size, inputB64);
  }
}

function callBackend(cfg, prompt, size, inputB64) {
  var format = String(cfg.format || "openai").toLowerCase();
  // 直接使用调用方指定尺寸或后端默认尺寸，不做模型相关的换算
  var useSize = size || cfg.default_size || "1024x1024";
  var result = invokeBackend(format, cfg, prompt, useSize, inputB64);
  // 参数错误（如尺寸档位不支持，400 状态）时，直接用 2K 重试一次
  if (!result.ok && isSizeParamError(result)) {
    var retrySize = "2K";
    if (String(retrySize).toLowerCase() !== String(useSize).toLowerCase()) {
      yara.logger.warn("绘画插件: 后端尺寸参数错误，自动使用 2K 重试");
      result = invokeBackend(format, cfg, prompt, retrySize, inputB64);
    }
  }
  return result;
}

// ─── 主流程 generate_picture ───

function handleGeneratePicture(params, context) {
  var groupId = (context && context.groupId) ? context.groupId : "";
  // 结果统一出口：记录日志后返回提示文本，保证每次绘图在终端都有可见结果
  function logResult(msg) {
    var text = String(msg);
    if (text.indexOf("失败") >= 0 || text.indexOf("错误") >= 0) {
      yara.logger.error("绘画插件: [群 " + groupId + "] " + text);
    } else {
      yara.logger.info("绘画插件: [群 " + groupId + "] " + text);
    }
    return text;
  }
  try {
    // 1. 插件总开关
    if (!getConfigValue("plugin.enabled", true)) {
      return logResult("绘画功能未启用");
    }

    // 2. 配置加载与每日上限
    ensureConfigFile();
    var dailyMax = getConfigValue("limits.daily_max", 20);
    if (!dailyMax || dailyMax <= 0) dailyMax = 20;

    // 3. 次数检查（调用时判断已生成次数是否达到上限，达到则不调 API）
    if (getDailyCount() >= dailyMax) {
      return logResult("今日绘图次数已用完（上限 " + dailyMax + " 次），零点后自动重置，明天再来吧");
    }

    // 4. 解析参数
    var prompt = (params && params.prompt) ? String(params.prompt) : "";
    if (!prompt) {
      return logResult("绘图失败: prompt 参数不能为空，请描述你想画的画面");
    }
    var drawSelf = (params && params.draw_self === true);
    // 解析参考图序号（image_refs）：可为单个序号 "1"、逗号分隔 "1,3" 或 JSON 数组 "[1,3]"
    var imageRefs = [];
    if (params && params.image_refs !== undefined && params.image_refs !== null && params.image_refs !== "") {
      var raw = String(params.image_refs).trim();
      if (raw.indexOf("[") === 0) {
        try {
          var parsed = JSON.parse(raw);
          if (Array.isArray(parsed)) {
            imageRefs = parsed.map(function(x) { return parseInt(x, 10); }).filter(function(n) { return !isNaN(n) && n > 0; });
          }
        } catch (e) {}
      } else {
        imageRefs = raw.split(",").map(function(s) { return parseInt(s.trim(), 10); }).filter(function(n) { return !isNaN(n) && n > 0; });
      }
    }
    var style = (params && params.style) ? String(params.style) : "";
    var modelId = (params && params.model_id) ? String(params.model_id) : "";
    var size = (params && params.size) ? String(params.size) : "";

    var imageUrls = (context && context.image_urls) ? context.image_urls : [];

    // 5. 选择后端
    var backends = getConfigValue("models.backends", {});
    var defaultModelId = getConfigValue("models.default_model_id", "");
    var cfg = null;
    var usedModelId = "";
    if (modelId && backends[modelId]) {
      cfg = backends[modelId];
      usedModelId = modelId;
    } else if (defaultModelId && backends[defaultModelId]) {
      cfg = backends[defaultModelId];
      usedModelId = defaultModelId;
    } else {
      for (var k in backends) {
        if (backends.hasOwnProperty(k)) {
          cfg = backends[k];
          usedModelId = k;
          break;
        }
      }
    }
    if (!cfg) {
      return logResult("未配置绘图后端，请在插件配置页填写");
    }

    // 6. 图片输入判定（自我形象 + 一张/多张参考图 → 多图融合，仅火山方舟支持多图）
    var inputB64 = null;
    var sceneOnly = false;
    var roleDescription = "";
    var selfieB64 = null;
    if (drawSelf && getConfigValue("selfie.enabled", true)) {
      selfieB64 = readSelfieBase64();
      if (selfieB64) {
        sceneOnly = true;
        roleDescription = getConfigValue("selfie.role_description", "");
        yara.logger.info("绘画插件: [群 " + groupId + "] 已读取自我形象图片");
      } else {
        yara.logger.warn("绘画插件: [群 " + groupId + "] draw_self 但自我形象图片不存在，降级处理");
      }
    }
    // 参考图：优先通过主程序缓存接口获取（主程序代读缓存，缺失自动回退重新下载），
    // 避免 QQ 临时链接过期导致重新下载失败
    var refB64s = [];
    var refLabels = [];
    for (var ri = 0; ri < imageRefs.length; ri++) {
      var ridx = imageRefs[ri];
      if (ridx < 1 || !imageUrls || ridx > imageUrls.length) {
        yara.logger.warn("绘画插件: [群 " + groupId + "] 参考图[图片" + ridx + "]不存在，跳过");
        continue;
      }
      var rb = downloadRefBase64(imageUrls[ridx - 1], ridx);
      if (rb) {
        refB64s.push(rb);
        refLabels.push(ridx);
      }
    }
    // 组合输入：自我形象 + 参考图列表
    var inputs = [];
    if (selfieB64) inputs.push(selfieB64);
    for (var ii = 0; ii < refB64s.length; ii++) inputs.push(refB64s[ii]);
    // 混合模式判定：≥2张参考图，或 1张参考图+画自己 → 多图融合（仅火山方舟后端支持）；否则单图图生图
    var wantFusion = refB64s.length >= 2 || (selfieB64 && refB64s.length >= 1);
    if (wantFusion && isArkBackend(cfg)) {
      inputB64 = inputs;
      sceneOnly = true;
      yara.logger.info("绘画插件: [群 " + groupId + "] " + (selfieB64 ? "自我形象+" : "") + "参考图[图片" + refLabels.join("][图片") + "] 多图融合生图");
    } else if (selfieB64 && refB64s.length === 0) {
      inputB64 = selfieB64;
      sceneOnly = true;
      yara.logger.info("绘画插件: [群 " + groupId + "] 使用自我形象图生图");
    } else if (selfieB64 && refB64s.length >= 1) {
      // 自画+参考图但后端不支持多图融合：用自我形象保身份，参考图描述进提示词
      inputB64 = selfieB64;
      sceneOnly = true;
      yara.logger.warn("绘画插件: [群 " + groupId + "] 当前后端不支持多图融合，仅用自我形象图生图，参考图[图片" + refLabels.join("][图片") + "]的描述将作为提示词参考");
    } else if (refB64s.length === 1) {
      inputB64 = refB64s[0];
      yara.logger.info("绘画插件: [群 " + groupId + "] 使用参考图[图片" + refLabels[0] + "]图生图");
    } else if (refB64s.length > 1) {
      inputB64 = refB64s[0];
      yara.logger.warn("绘画插件: [群 " + groupId + "] 当前后端不支持多图融合，仅使用第一张参考图[图片" + refLabels[0] + "]");
    }

    // 后端不支持图生图时强制降级文生图
    var supportImg2Img = (cfg.support_img2img !== undefined && cfg.support_img2img !== null)
      ? (cfg.support_img2img === true || String(cfg.support_img2img) === "true")
      : false;
    if (!supportImg2Img) {
      inputB64 = null;
    }

    // 7. 参考图描述（取自聊天记录）+ 提示词优化 + 画风
    // 描述直接从聊天记录中的 [图片N：描述] 读取（视觉分析已生成），不重新识别图片；
    // 不管常规还是自我形象优化模式，都会把参考图的描述传给优化器；
    // 自我形象模式额外带有自己的角色设定描述 roleDescription。
    var imageDescText = "";
    var imageDescs = (context && context.image_descriptions) ? context.image_descriptions : [];
    for (var di = 0; di < refLabels.length; di++) {
      var didx = refLabels[di];
      var d = (imageDescs && didx >= 1 && didx <= imageDescs.length) ? String(imageDescs[didx - 1] || "").trim() : "";
      if (d) {
        imageDescText += "[图片" + didx + "：" + d + "]";
      }
    }
    if (imageDescText) {
      yara.logger.info("绘画插件: [群 " + groupId + "] 参考图描述: " + previewText(imageDescText, 200));
    }
    var opt = optimizePrompt(prompt, sceneOnly, roleDescription, imageDescText);
    var finalPrompt = opt.prompt;
    finalPrompt = applyStyle(finalPrompt, style);
    if (!supportImg2Img && drawSelf && roleDescription) {
      finalPrompt = "角色形象参考：" + roleDescription + "，" + finalPrompt;
    }
    // 7.1 附加正面词（提升画质，来自后端配置 custom_prompt_add）
    var promptAdd = cfg.custom_prompt_add || "";
    if (promptAdd) {
      finalPrompt = finalPrompt ? finalPrompt + ", " + promptAdd : promptAdd;
    }

    // 8. 调用后端（发出请求前先占用一次每日额度，避免并发时重复通过次数检查）
    var usedCount = incrementDailyCount();
    var useSize = size || cfg.default_size || "1024x1024";
    yara.logger.info("绘画插件: [群 " + groupId + "] 今日第" + usedCount + "/" + dailyMax + "次 后端=" + usedModelId + " format=" + (cfg.format || "openai") + " img2img=" + (inputB64 ? "true" : "false"));
    var result = callBackend(cfg, finalPrompt, useSize, inputB64);

    // 9. 处理结果
    if (!result || !result.ok) {
      return logResult("绘图失败: " + ((result && result.error) ? result.error : "未知错误"));
    }
    var b64 = result.b64 || "";
    if (!b64 && result.url) {
      var dl = yara.http.download(result.url, "result.png");
      if (dl && dl.success && !dl.error) {
        var content = yara.file.readData("result.png");
        if (content) b64 = yara.encoding.base64Encode(content);
      }
      if (!b64) return logResult("绘图失败: 图片下载失败");
    }
    if (!b64) return logResult("绘图失败: 未获取到图片数据");

    // 10. 发送图片
    yara.logger.info("绘画插件: [群 " + groupId + "] 图片生成成功 base64长度=" + b64.length + "，准备发送");
    var sent = yara.send.image(groupId, b64);
    if (!sent) {
      return logResult("图片生成成功但发送失败");
    }
    var remain = remainingCount(dailyMax);
    yara.logger.info("绘画插件: [群 " + groupId + "] 发送成功，今日剩余 " + remain + " 次");
    return logResult("已生成图片并发送到群聊（今日剩余 " + remain + " 次）");
  } catch (e) {
    var errMsg = (e && e.message) ? e.message : String(e);
    yara.logger.error("绘画插件: [群 " + groupId + "] 绘图出现错误: " + errMsg);
    return "绘图出现错误: " + errMsg;
  }
}

// ─── 主流程 layer_split（火山方舟 Seedream 5.0 pro 图层拆分） ───

function handleLayerSplit(params, context) {
  var groupId = (context && context.groupId) ? context.groupId : "";
  function logResult(msg) {
    var text = String(msg);
    if (text.indexOf("失败") >= 0 || text.indexOf("错误") >= 0) {
      yara.logger.error("绘画插件: [群 " + groupId + "] " + text);
    } else {
      yara.logger.info("绘画插件: [群 " + groupId + "] " + text);
    }
    return text;
  }
  try {
    if (!getConfigValue("plugin.enabled", true)) return logResult("绘画功能未启用");
    ensureConfigFile();
    var dailyMax = getConfigValue("limits.daily_max", 20);
    if (!dailyMax || dailyMax <= 0) dailyMax = 20;
    if (getDailyCount() >= dailyMax) {
      return logResult("今日绘图次数已用完（上限 " + dailyMax + " 次），零点后自动重置，明天再来吧");
    }

    // 参数：image_ref 必填
    var imageRef = 0;
    if (params && params.image_ref !== undefined && params.image_ref !== null) {
      imageRef = parseInt(params.image_ref, 10);
      if (isNaN(imageRef)) imageRef = 0;
    }
    if (imageRef < 1) {
      return logResult("拆图失败: 请指定要拆解的图片序号（image_ref）");
    }
    var imageUrls = (context && context.image_urls) ? context.image_urls : [];
    if (!imageUrls || imageUrls.length < imageRef) {
      return logResult("拆图失败: 找不到对应的[图片" + imageRef + "]，请先发一张图片");
    }
    var splitPrompt = (params && params.prompt) ? String(params.prompt) : "";
    var modelId = (params && params.model_id) ? String(params.model_id) : "";

    // 选择后端并校验：图层拆分仅火山方舟 Seedream 5.0 pro 支持
    var backends = getConfigValue("models.backends", {});
    var defaultModelId = getConfigValue("models.default_model_id", "");
    var cfg = null;
    var usedModelId = "";
    if (modelId && backends[modelId]) {
      cfg = backends[modelId];
      usedModelId = modelId;
    } else if (defaultModelId && backends[defaultModelId]) {
      cfg = backends[defaultModelId];
      usedModelId = defaultModelId;
    } else {
      for (var k in backends) {
        if (backends.hasOwnProperty(k)) {
          cfg = backends[k];
          usedModelId = k;
          break;
        }
      }
    }
    if (!cfg) return logResult("拆图失败: 未配置绘图后端");
    var baseUrl = String(cfg.base_url || "");
    if (baseUrl.indexOf("ark.cn-beijing.volces.com") < 0) {
      return logResult("拆图失败: 当前后端(" + usedModelId + ")不支持图层拆分，请使用火山方舟 Seedream 5.0 pro（即梦）");
    }
    if (String(cfg.model || "").indexOf("seedream-5-0-pro") < 0) {
      yara.logger.warn("绘画插件: [群 " + groupId + "] 当前模型可能不支持拆图（仅 Seedream 5.0 pro 支持），继续尝试");
    }

    // 下载参考图
    var refUrl = imageUrls[imageRef - 1];
    var inputB64 = downloadRefBase64(refUrl, imageRef);
    if (!inputB64) return logResult("拆图失败: 参考图下载失败");

    // 构建拆图请求（仅支持 1 张输入图；输出底图+图层，图层为带透明通道的 PNG，便于单独编辑/重组）
    var payload = {
      model: cfg.model,
      image: toDataUri(inputB64),
      size: cfg.default_size || "2K",
      layer_decomposition: true,
      response_format: "url",
      output_format: "png"
    };
    if (splitPrompt) payload.prompt = splitPrompt;
    if (cfg.watermark !== undefined && cfg.watermark !== null && cfg.watermark !== "") {
      payload.watermark = (cfg.watermark === true || String(cfg.watermark).toLowerCase() === "true");
    }

    var splitUsedCount = incrementDailyCount();
    yara.logger.info("绘画插件: [群 " + groupId + "] 今日第" + splitUsedCount + "/" + dailyMax + "次 请求拆图 " + baseUrl + "/images/generations model=" + cfg.model);
    var resp = yara.http.post(baseUrl + "/images/generations", JSON.stringify(payload), buildAuthHeaders(cfg), 300);
    if (!resp || resp.error) {
      return logResult("拆图失败: " + ((resp && resp.error) ? resp.error : "请求异常"));
    }
    if (!(resp.status >= 200 && resp.status < 300)) {
      return logResult("拆图失败: 状态码 " + resp.status + "，响应: " + previewText(resp.body, 200));
    }
    var data;
    try {
      data = JSON.parse(resp.body);
    } catch (pe) {
      return logResult("拆图失败: 响应不是合法JSON");
    }
    var items = (data && data.data) ? data.data : [];
    if (!items.length) return logResult("拆图失败: 响应未包含图层数据");

    // 拆分底图（z_index=0）与图层
    var base = null;
    var layers = [];
    for (var i = 0; i < items.length; i++) {
      var it = items[i];
      if (it.z_index === 0) {
        if (!base) base = it;
      } else {
        layers.push(it);
      }
    }
    if (!base) base = items[0];
    if (!base || !base.url) return logResult("拆图失败: 未获取到底图URL");

    // 下载底图 + 所有图层，合并到同一个聊天框一起发送（底图在前，图层在后）
    var segs = [];
    var partLabels = [];

    var dl = yara.http.download(base.url, "layer_base.png");
    if (dl && dl.success && !dl.error) {
      var content = yara.file.readData("layer_base.png");
      if (content) {
        segs.push({ type: "image", content: yara.encoding.base64Encode(content) });
        partLabels.push("底图");
      }
    }

    var layerCount = 0;
    for (var j = 0; j < layers.length && j < 16; j++) {
      var l = layers[j];
      if (!l.url) continue;
      var layerDl = yara.http.download(l.url, "layer_" + j + ".png");
      if (!layerDl || !layerDl.success || layerDl.error) continue;
      var layerContent = yara.file.readData("layer_" + j + ".png");
      if (!layerContent) continue;
      segs.push({ type: "image", content: yara.encoding.base64Encode(layerContent) });
      layerCount++;
      var label = l.name || ("图层" + (j + 1));
      var desc = l.description ? "（" + l.description + "）" : "";
      var piece = (j + 1) + ". " + label + desc;
      if (piece.length > 120) piece = piece.substring(0, 120) + "…";
      partLabels.push(piece);
    }

    if (!segs.length) return logResult("拆图失败: 底图和图层均下载失败");

    // 每条消息最多 8 张图，避免图层过多时超出平台单条消息的图片数量上限
    var summary = "已拆出底图 + " + layerCount + " 个图层（图层为透明PNG，可单独编辑/重组）：";
    var firstMsg = true;
    var sentOk = true;
    for (var c = 0; c < segs.length; c += 8) {
      var msgSegs = [];
      if (firstMsg) {
        msgSegs.push({ type: "text", content: summary });
        firstMsg = false;
      }
      for (var s = c; s < segs.length && s < c + 8; s++) {
        msgSegs.push(segs[s]);
      }
      var r = yara.send.hybrid(groupId, msgSegs);
      if (!r) sentOk = false;
    }
    if (!sentOk) return logResult("拆图成功但图层消息发送失败");

    // 汇总图层清单（名称+描述）
    var parts = [summary];
    for (var p = 0; p < partLabels.length; p++) parts.push(partLabels[p]);
    var remain = remainingCount(dailyMax);
    parts.push("（今日剩余 " + remain + " 次）");
    return logResult(parts.join("\n"));
  } catch (e) {
    var errMsg = (e && e.message) ? e.message : String(e);
    yara.logger.error("绘画插件: [群 " + groupId + "] 拆图出现错误: " + errMsg);
    return "拆图出现错误: " + errMsg;
  }
}

// ─── 拆图工具条件注册 ───
// layer_split 仅当配置中存在火山方舟 Seedream 5.0 pro（即梦）后端时才注册。
// 因 yara.tool.register 只追加不判重、且无注销接口，用标志位防止重复注册。

var layerSplitRegistered = false;

function supportsLayerSplit() {
  var config = ensureConfigFile();
  var backends = (config && config.models && config.models.backends) || {};
  for (var id in backends) {
    if (!Object.prototype.hasOwnProperty.call(backends, id)) continue;
    var b = backends[id] || {};
    var baseUrl = String(b.base_url || "").toLowerCase();
    var model = String(b.model || "").toLowerCase();
    if (baseUrl.indexOf("ark.cn-beijing.volces.com") !== -1 && model.indexOf("seedream-5-0-pro") !== -1) {
      return true;
    }
  }
  return false;
}

function registerLayerSplitIfSupported() {
  if (layerSplitRegistered) return;
  // 手动开关优先于模型检测：enabled=false 则不注册拆图工具
  if (!getConfigValue("layer_split.enabled", true)) {
    yara.logger.info("绘画插件: 拆图工具已手动关闭（layer_split.enabled=false），不注册");
    return;
  }
  if (!supportsLayerSplit()) return;
  yara.tool.register("layer_split", {
    description: "把聊天中的图片拆解为底图和多个独立图层（主体、背景、文字、装饰元素等），图层为带透明通道的 PNG，可单独编辑或重新组合。仅火山方舟 Seedream 5.0 pro（即梦）后端支持。当用户要求拆图、分离图层、把图片分层、拆出人物/文字/背景等元素时调用。参数说明：image_ref(必填整数，要拆解的图片序号，对应[图片1]、[图片2]...)；prompt(可选字符串，描述要拆分哪些元素，留空自动全拆)；model_id(可选字符串，指定后端ID，需为火山方舟即梦后端)。",
    briefDescription: "拆图",
    detailedDescription: "调用火山方舟 Seedream 5.0 pro 的图层拆分能力，把一张图片拆成1张底图和最多16个独立图层（透明PNG），发送底图并列出图层清单。",
    parameters: LAYER_SPLIT_PARAMETERS,
    toolType: "agent",
    visibility: "deferred",
    timeoutSeconds: 300,
    async: true
  }, function(params, context) {
    return handleLayerSplit(params, context);
  });
  layerSplitRegistered = true;
  yara.logger.info("绘画插件: 检测到火山方舟 Seedream 5.0 pro 后端，已注册拆图工具 layer_split");
}

// ─── 注册 Agent 工具 ───

yara.tool.register("generate_picture", {
  description: "生成图片并发送到当前群聊。当用户要求你画图、生成图片、画一幅画、或者聊天语境中你判断应该配一张图时，调用此工具。参数说明：prompt(必填，绘图的自然语言描述)；draw_self(可选布尔，是否画你自己，为true时使用配置的自我形象照片做图生图，保持你的外貌不变)；image_refs(可选字符串，参考图片序号，对应聊天记录中的[图片1]、[图片2]...，可传单个序号\"1\"、逗号分隔\"1,3\"或JSON数组\"[1,3]\"。当只有1张参考图且不是画你自己时做普通图生图；当有多张参考图（>=2张），或1张参考图且同时画你自己(draw_self=true)时，自动进入多图融合模式，仅火山方舟后端支持多图)；style(可选字符串，画风如卡通/写实/水彩)；model_id(可选字符串，指定绘图后端ID，留空用默认)；size(可选字符串，图片尺寸如1024x1024)。注意：1)当用户发了一张或多张图片并想参考它们生成时，把图片序号填入image_refs（如\"1\"或\"[1,2]\"）。2)当用户说\u201C画你自己，参考这张图/按照这个风格\u201D时，同时设置 draw_self=true 和 image_refs=图片序号，插件会把你的自我形象和参考图混合生成。3)当用户说\u201C画你的三视图/立绘/头像\u201D时，设置 draw_self=true。",
  briefDescription: "绘图",
  detailedDescription: "根据提示词调用绘图后端生成图片并发送回群聊，支持画自己（自我形象图生图）和用聊天中的图片做参考图生图。",
  parameters: TOOL_PARAMETERS,
  toolType: "agent",
  visibility: "deferred",
  timeoutSeconds: 300,
  async: true
}, function(params, context) {
  return handleGeneratePicture(params, context);
});

// ─── 生命周期 ───

function onLoad() {
  ensureConfigFile();
  registerLayerSplitIfSupported();
  yara.logger.info("绘画插件已加载");
}

function onConfigUpdate(scope, config) {
  ensureConfigFile();
  if (layerSplitRegistered && !supportsLayerSplit()) {
    yara.logger.warn("绘画插件: 配置中已移除 Seedream 5.0 pro 后端，layer_split 工具需重载插件后才能注销");
  }
  registerLayerSplitIfSupported();
  yara.logger.info("绘画插件配置已更新");
}

function onUnload() {
  yara.logger.info("绘画插件已卸载");
}
