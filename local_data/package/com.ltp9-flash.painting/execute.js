/**
 * LTP9 绘图插件（com.yaraflow.art-ltp9 / execute.js）
 * 全部同步：不使用 async/await/Promise/fetch/.then。
 * 网络走 http.get/post；图片 image；LLM agent.chat；休眠 sleep（LTP9-Flash 顶层全局）。
 * 事件订阅与导出函数都返回普通对象（非 Promise）。
 */

// ===== 配置 =====

// 顶层捕获 config 全局（避免与函数内局部变量重名遮蔽）
const __config = config;

function getConfig() {
  let cfg = {};
  try { cfg = __config.read() || {}; } catch (e) { cfg = {}; }
  return cfg;
}
function getConfigValue(path, defaultValue) {
  const config = getConfig();
  const keys = path.split(".");
  let val = config;
  for (let i = 0; i < keys.length; i++) {
    if (val === null || val === undefined || typeof val !== "object") return defaultValue;
    val = val[keys[i]];
  }
  return (val !== undefined && val !== null) ? val : defaultValue;
}

// ===== 每日次数（本地日期零点重置，file 同步持久化） =====

function today() { return new Date(time.now() * 1000).toISOString().slice(0, 10); }
function loadDailyCount() {
  try {
    const r = file.read("daily_count.json");
    const raw = (r && r.success) ? r.text : "";
    if (raw === null || raw === "") return { date: "", count: 0 };
    const d = JSON.parse(raw);
    const c = parseInt(d.count, 10);
    return { date: d.date || "", count: isNaN(c) ? 0 : c };
  } catch (e) { return { date: "", count: 0 }; }
}
function persistDailyCount(r) { try { file.write("daily_count.json", JSON.stringify(r)); } catch (e) {} }
function getDailyCount() {
  let r = loadDailyCount();
  if (r.date !== today()) { r = { date: today(), count: 0 }; persistDailyCount(r); }
  return r.count;
}
function incrementDailyCount() {
  let r = loadDailyCount();
  if (r.date !== today()) r = { date: today(), count: 0 };
  r.count++; persistDailyCount(r); return r.count;
}

// ===== 同步工具 =====

function preview(s, n) { const t = String(s); const m = n || 300; return t.length > m ? t.substring(0, m) + "..." : t; }
function urlEncodeQuery(obj) {
  const parts = [];
  for (const k in obj) { if (Object.prototype.hasOwnProperty.call(obj, k)) parts.push(encodeURIComponent(k) + "=" + encodeURIComponent(String(obj[k]))); }
  return parts.join("&");
}
function guessMime(b64) {
  if (!b64) return "jpeg";
  if (b64.indexOf("/9j/") === 0) return "jpeg";
  if (b64.indexOf("iVBORw") === 0) return "png";
  if (b64.indexOf("UklGR") === 0) return "webp";
  if (b64.indexOf("R0lGOD") === 0) return "gif";
  return "jpeg";
}
function toDataUri(b64) { return "data:image/" + guessMime(b64) + ";base64," + b64; }
function cleanB64(b64) {
  const s = String(b64 || "");
  const i = s.indexOf(",");
  if (i >= 0 && s.substring(0, i).indexOf("base64") >= 0) return s.substring(i + 1);
  return s;
}
function mimeOfB64(b64) {
  const c = cleanB64(b64);
  if (c.indexOf("/9j/") === 0) return "image/jpeg";
  if (c.indexOf("iVBORw") === 0) return "image/png";
  if (c.indexOf("UklGR") === 0) return "image/webp";
  if (c.indexOf("R0lGOD") === 0) return "image/gif";
  return "image/jpeg";
}
function isPngB64(b64) { return !!b64 && mimeOfB64(b64) === "image/png"; }
function isSizeParamError(r) {
  if (!r) return false;
  return /size|尺寸|参数|resolution|档位|不支持|invalid|非法/.test(String(r.error || "").toLowerCase());
}
function stripBearer(k) { return String(k || "").trim().replace(/^Bearer\s+/i, ""); }
function withBearer(k) { const s = stripBearer(k); return s ? "Bearer " + s : ""; }
function buildAuthHeaders(cfg) {
  const h = { "Content-Type": "application/json" };
  const fmt = String(cfg.format || "").toLowerCase();
  if (fmt === "gemini") h["x-goog-api-key"] = stripBearer(cfg.api_key);
  else if (fmt === "comfyui") { /* 免鉴权 */ }
  else { const b = withBearer(cfg.api_key); if (b) h["Authorization"] = b; }
  return h;
}
function isArkBackend(cfg) { return String(cfg && cfg.base_url || "").indexOf("ark.cn-beijing.volces.com") >= 0; }
function gcd(a, b) { while (b) { const t = a % b; a = b; b = t; } return a; }
function parsePixelSize(size) {
  const m = String(size || "").toLowerCase().match(/(\d+)\s*[x*]\s*(\d+)/);
  return m ? { width: parseInt(m[1], 10), height: parseInt(m[2], 10) } : { width: 1024, height: 1024 };
}
function pixelToGeminiAspect(size) {
  const wh = parsePixelSize(size);
  const target = wh.width / wh.height;
  const supported = ["1:1", "16:9", "9:16", "4:3", "3:4", "3:2", "2:3", "4:5", "5:4", "21:9"];
  const g = gcd(wh.width, wh.height);
  const aw = wh.width / g, ah = wh.height / g;
  if (supported.indexOf(aw + ":" + ah) >= 0) return aw + ":" + ah;
  let best = "1:1", bestDiff = Infinity;
  for (const p of supported) { const q = p.split(":"); const d = Math.abs(parseInt(q[0], 10) / parseInt(q[1], 10) - target); if (d < bestDiff) { bestDiff = d; best = p; } }
  return best;
}
function useTransparent(cfg, inputB64) {
  if (Array.isArray(inputB64) || !inputB64) return false;
  return String(cfg.background || "").toLowerCase() === "transparent" && isPngB64(inputB64);
}

// ===== 同步 HTTP =====

function httpPostJson(url, body, headers) {
  const r = http.post(url, JSON.stringify(body), headers || {});
  if (r.error) return { status: 0, body: "", error: r.error };
  return { status: r.status, body: r.body };
}
function httpGetBody(url, headers) {
  const r = http.get(url, headers || {});
  if (r.error) return { status: 0, body: "", error: r.error };
  return { status: r.status, body: r.body };
}
// 统一把 HTTP 失败转成可读错误；status=0 时优先展示底层网络错误（http.post 的 r.error）。
function httpErr(resp) {
  if (resp && resp.error) return resp.error;
  return "状态码 " + (resp ? resp.status : "?") + "，响应: " + preview(resp ? resp.body : "");
}

// ===== 图像输入（同步 image） =====

function readSelfieBase64() {
  try {
    const path = getConfigValue("selfie.image_path", "data/selfie.png");
    if (!path) return null;
    const r = image.loadValid(path);
    if (r && r.success && r.text) return r.text;
    console.error("[绘图] 自我形象图片非有效或读取失败: " + path);
    return null;
  } catch (e) { console.error("[绘图] 读取自我形象失败: " + e.message); return null; }
}
function downloadRefBase64(url, index) {
  try {
    const r = image.download(url, "ref_" + index + ".png");
    if (r && r.success && r.text) return r.text;
    console.error("[绘图] 参考图[图片" + index + "] 下载非有效图片");
    return null;
  } catch (e) { console.error("[绘图] 参考图下载失败: " + e.message); return null; }
}

// ===== 提示词优化（同步 agent.chat） =====

const OPTIMIZER_SYSTEM = "You are a professional AI image prompt engineer. Convert the user's description into ONE line of English image prompt separated by commas: subject, action, scene, lighting, style, quality. Be concrete; avoid abstract words; keep user's intent. End with: masterpiece, best quality";

function optimizePrompt(description) {
  try {
    if (typeof agent !== "object" || !agent.chat) return { ok: false, prompt: description };
    const resp = agent.chat([
      { role: "system", content: OPTIMIZER_SYSTEM },
      { role: "user", content: "Input: " + description + "\nOutput:" }
    ], { temperature: 0.7 });
    if (resp && resp.success && resp.text) {
      const cleaned = String(resp.text).trim().replace(/^\s*(?:Output|Prompt|output|prompt)\s*:\s*/i, "").replace(/^["']|["']$/g, "").replace(/\s+/g, " ");
      if (cleaned) { console.info("[绘图] 提示词优化完成: " + preview(cleaned, 150)); return { ok: true, prompt: cleaned }; }
    }
    console.warn("[绘图] 提示词优化为空，降级使用原始描述");
  } catch (e) { console.error("[绘图] 提示词优化失败，降级: " + e.message); }
  return { ok: false, prompt: description };
}

function applyStyle(prompt, style) {
  if (!style) return prompt;
  const map = { "卡通": "cartoon style", "写实": "photorealistic", "水彩": "watercolor painting", "赛博朋克": "cyberpunk style" };
  const tag = map[style] || style;
  return prompt ? tag + ", " + prompt : tag;
}

function extractImage(data) {
  if (!data) return null;
  if (data.data && data.data[0]) { const f = data.data[0]; if (f.b64_json) return { ok: true, b64: f.b64_json }; if (f.url) return { ok: true, url: f.url }; }
  if (data.images && data.images[0]) { const i = data.images[0]; if (typeof i === "string") return { ok: true, url: i }; if (i.url) return { ok: true, url: i.url }; }
  if (data.url) return { ok: true, url: data.url };
  return null;
}

// ===== 同步后端 =====

function callOpenAI(cfg, prompt, size, inputB64) {
  const endpoint = String(cfg.base_url || "").replace(/\/+$/, "") + "/images/generations";
  const payload = { model: cfg.model, prompt: prompt, size: size, n: 1 };
  if (isArkBackend(cfg)) {
    payload.response_format = "url";
    if (cfg.watermark !== undefined && cfg.watermark !== null && cfg.watermark !== "") {
      payload.watermark = (cfg.watermark === true || String(cfg.watermark).toLowerCase() === "true");
    }
    payload.optimize_prompt_options = { mode: String(cfg.prompt_optimize_mode || "standard").toLowerCase() === "fast" ? "fast" : "standard" };
    if (useTransparent(cfg, inputB64)) { payload.background = "transparent"; payload.output_format = "png"; }
  }
  if (cfg.negative_prompt_add) payload.negative_prompt = cfg.negative_prompt_add;
  if (Array.isArray(inputB64) && isArkBackend(cfg)) { payload.image = inputB64.map(toDataUri); }
  else if (inputB64) {
    const one = Array.isArray(inputB64) ? inputB64[0] : inputB64;
    payload.image = toDataUri(one);
    let raw = cfg.strength; const s = (raw === undefined || raw === null || raw === "") ? 0.5 : parseFloat(raw);
    payload.strength = Math.max(0.1, Math.min(1.0, isNaN(s) ? 0.5 : s));
  }
  const resp = httpPostJson(endpoint, payload, buildAuthHeaders(cfg));
  if (resp.status < 200 || resp.status >= 300) return { ok: false, status: resp.status, error: httpErr(resp) };
  let data; try { data = JSON.parse(resp.body); } catch (e) { return { ok: false, error: "响应非合法JSON" }; }
  const img = extractImage(data); if (img) return img;
  return { ok: false, error: "未找到图片: " + preview(resp.body) };
}

function callModelScope(cfg, prompt, size, inputB64) {
  const apiKey = stripBearer(cfg.api_key);
  const base = String(cfg.base_url || "").replace(/\/+$/, "");
  const headers = { "Authorization": "Bearer " + apiKey, "Content-Type": "application/json", "X-ModelScope-Async-Mode": "true" };
  const body = inputB64 ? { model: cfg.model, prompt: prompt, image_url: [toDataUri(inputB64)] } : { model: cfg.model, prompt: prompt, seed: 42, steps: 30, guidance: 3.5 };
  const resp = httpPostJson(base + "/images/generations", body, headers);
  if (resp.status < 200 || resp.status >= 300) return { ok: false, status: resp.status, error: httpErr(resp) };
  let data; try { data = JSON.parse(resp.body); } catch (e) { return { ok: false, error: "魔搭响应非JSON" }; }
  const taskId = data.task_id;
  if (!taskId) return { ok: false, error: "魔搭未返回任务ID" };
  const checkHeaders = { "Authorization": "Bearer " + apiKey, "Content-Type": "application/json", "X-ModelScope-Task-Type": "image_generation" };
  for (let i = 0; i < 24; i++) {
    const s = httpGetBody(base + "/tasks/" + taskId, checkHeaders);
    if (s.status >= 200 && s.status < 300) {
      let sd; try { sd = JSON.parse(s.body); } catch (e) {}
      if (sd && sd.task_status === "SUCCEED") {
        const url = sd.output_images && sd.output_images[0];
        if (url) { const dl = image.download(url, "gen_" + taskId + ".png"); if (dl && dl.success && dl.text) return { ok: true, b64: dl.text }; return { ok: false, error: "魔搭图片下载失败" }; }
        return { ok: false, error: "魔搭任务成功但未返回图片" };
      }
      if (sd && sd.task_status === "FAILED") return { ok: false, error: "魔搭任务失败: " + (sd.error_message || "未知") };
    }
    sleep(5000);
  }
  return { ok: false, error: "魔搭任务执行超时" };
}

function callGemini(cfg, prompt, size, inputB64) {
  const base = String(cfg.base_url || "https://generativelanguage.googleapis.com").replace(/\/+$/, "");
  const model = cfg.model || "gemini-2.5-flash-image-preview";
  const endpoint = base + "/v1beta/models/" + model + ":generateContent";
  const headers = { "Content-Type": "application/json", "x-goog-api-key": stripBearer(cfg.api_key) };
  const parts = [{ text: prompt }];
  if (inputB64) { const one = Array.isArray(inputB64) ? inputB64[0] : inputB64; parts.push({ inline_data: { mime_type: mimeOfB64(one), data: cleanB64(one) } }); }
  const requestData = { contents: [{ role: "user", parts: parts }], generationConfig: { responseModalities: ["TEXT", "IMAGE"] } };
  if (size) { const a = pixelToGeminiAspect(size); if (a) requestData.generationConfig.imageConfig = { aspectRatio: a }; }
  const resp = httpPostJson(endpoint, requestData, headers);
  if (resp.status < 200 || resp.status >= 300) return { ok: false, error: httpErr(resp) };
  let data; try { data = JSON.parse(resp.body); } catch (e) { return { ok: false, error: "Gemini 响应非JSON" }; }
  const cands = data.candidates || [];
  if (cands[0] && cands[0].content && cands[0].content.parts) {
    for (const part of cands[0].content.parts) { if (part.inlineData && part.inlineData.data) return { ok: true, b64: part.inlineData.data }; if (part.inline_data && part.inline_data.data) return { ok: true, b64: part.inline_data.data }; }
  }
  return { ok: false, error: "Gemini 未返回图片数据" };
}

function callComfyui(cfg, prompt, size, inputB64) {
  const base = String(cfg.base_url || "http://127.0.0.1:8188").replace(/\/+$/, "");
  const wfName = cfg.model || "";
  if (!wfName) return { ok: false, error: "未配置工作流文件名（model 字段）" };
  const fr = file.read("workflow/" + wfName);
  if (!fr || !fr.success || !fr.text) return { ok: false, error: "工作流文件不存在: workflow/" + wfName };
  let wf = String(fr.text).split("${prompt}").join(prompt);
  if (size) { const wh = parsePixelSize(size); wf = wf.split("${width}").join(wh.width).split("${height}").join(wh.height); }
  if (cfg.negative_prompt_add) wf = wf.split("${negative_prompt}").join(cfg.negative_prompt_add);
  let seed = cfg.seed; if (seed === undefined || seed === null || seed === -1) seed = Math.floor(Math.random() * 1e10) + 1;
  wf = wf.split("${seed}").join(seed);
  const submit = httpPostJson(base + "/prompt", { prompt: JSON.parse(wf) }, { "Content-Type": "application/json" });
  if (submit.status < 200 || submit.status >= 300) return { ok: false, error: "ComfyUI 提交失败: " + httpErr(submit) };
  let promptId = ""; try { promptId = (JSON.parse(submit.body)).prompt_id; } catch (e) {}
  if (!promptId) return { ok: false, error: "ComfyUI 未返回 prompt_id" };
  let fileName = "";
  for (let i = 0; i < 120; i++) {
    const h = httpGetBody(base + "/history/" + promptId);
    if (h.status >= 200 && h.status < 300) {
      let hd; try { hd = JSON.parse(h.body); } catch (e) {}
      const entry = hd && hd[promptId];
      if (entry && entry.outputs) {
        for (const nodeId in entry.outputs) { if (Object.prototype.hasOwnProperty.call(entry.outputs, nodeId)) { const no = entry.outputs[nodeId]; if (no.images && no.images[0] && no.images[0].filename) { fileName = no.images[0].filename; break; } } }
        if (fileName) break;
      }
    }
    sleep(1000);
  }
  if (!fileName) return { ok: false, error: "等待 ComfyUI 生成结果超时" };
  const img = httpGetBody(base + "/view?filename=" + encodeURIComponent(fileName));
  if (img.status >= 200 && img.status < 300 && img.body) return { ok: true, b64: cleanB64(img.body) };
  return { ok: false, error: "ComfyUI 图片下载失败" };
}

function invokeBackend(format, cfg, prompt, size, inputB64) {
  switch (String(format).toLowerCase()) {
    case "modelscope": return callModelScope(cfg, prompt, size, inputB64);
    case "gemini": return callGemini(cfg, prompt, size, inputB64);
    case "comfyui": return callComfyui(cfg, prompt, size, inputB64);
    default: return callOpenAI(cfg, prompt, size, inputB64);
  }
}

function callBackend(cfg, prompt, size, inputB64) {
  const useSize = size || cfg.default_size || "1024x1024";
  let result = invokeBackend(cfg.format, cfg, prompt, useSize, inputB64);
  if (!result.ok && isSizeParamError(result)) {
    console.warn("[绘图] 后端尺寸参数错误，自动 2K 重试");
    result = invokeBackend(cfg.format, cfg, prompt, "2K", inputB64);
  }
  return result;
}

// ===== 主流程 generatePicture（同步） =====

function generatePicture(params, context) {
  const groupId = (context && context.groupId) ? context.groupId : "";
  function log(err, msg) { const t = String(msg); if (err) console.warn("[绘图] [群 " + groupId + "] " + t); else console.info("[绘图] [群 " + groupId + "] " + t); return t; }
  try {
    if (!getConfigValue("plugin.enabled", true)) return { ok: false, text: log(false, "绘画功能未启用") };
    let dailyMax = getConfigValue("limits.daily_max", 20); if (!dailyMax || dailyMax <= 0) dailyMax = 20;
    if (getDailyCount() >= dailyMax) return { ok: false, text: log(false, "今日绘图次数已用完（上限 " + dailyMax + " 次）") };

    const prompt = (params && params.prompt) ? String(params.prompt) : "";
    if (!prompt) return { ok: false, text: log(false, "绘图失败: prompt 参数不能为空") };
    const drawSelf = (params && params.draw_self === true);
    const style = (params && params.style) ? String(params.style) : "";
    const modelId = (params && params.model_id) ? String(params.model_id) : "";
    const size = (params && params.size) ? String(params.size) : "";
    const imageUrls = (context && context.image_urls) ? context.image_urls : [];
    const refIdx = [];
    if (params && params.image_refs) {
      const raw = String(params.image_refs).trim();
      let arr = [];
      if (raw.indexOf("[") === 0) { try { const p = JSON.parse(raw); if (Array.isArray(p)) arr = p; } catch (e) {} }
      else arr = raw.split(",");
      for (const x of arr) { const n = parseInt(x, 10); if (!isNaN(n) && n > 0) refIdx.push(n); }
    }

    const backends = getConfigValue("models.backends", {});
    const defaultModelId = getConfigValue("models.default_model_id", "");
    let cfg = null, usedModelId = "";
    if (modelId && backends[modelId]) { cfg = backends[modelId]; usedModelId = modelId; }
    else if (defaultModelId && backends[defaultModelId]) { cfg = backends[defaultModelId]; usedModelId = defaultModelId; }
    else { for (const k in backends) { if (Object.prototype.hasOwnProperty.call(backends, k)) { cfg = backends[k]; usedModelId = k; break; } } }
    if (!cfg) return { ok: false, text: log(false, "未配置绘图后端") };

    let selfieB64 = null;
    if (drawSelf && getConfigValue("selfie.enabled", true)) selfieB64 = readSelfieBase64();
    const refB64s = [];
    for (const r of refIdx) { if (r < 1 || !imageUrls || r > imageUrls.length) continue; const b = downloadRefBase64(imageUrls[r - 1], r); if (b) refB64s.push(b); }

    const inputs = [];
    if (selfieB64) inputs.push(selfieB64);
    for (const b of refB64s) inputs.push(b);

    let supportImg = (cfg.support_img2img !== undefined && cfg.support_img2img !== null)
      ? (cfg.support_img2img === true || String(cfg.support_img2img) === "true") : false;
    let inputB64 = null;
    if (supportImg && inputs.length > 0) {
      const wantFusion = refB64s.length >= 2 || (selfieB64 && refB64s.length >= 1);
      inputB64 = (wantFusion && isArkBackend(cfg)) ? inputs : inputs[0];
    }

    const opt = optimizePrompt(prompt);
    let finalPrompt = opt.prompt;
    finalPrompt = applyStyle(finalPrompt, style);
    const add = cfg.custom_prompt_add || "";
    if (add) finalPrompt = finalPrompt ? finalPrompt + ", " + add : add;

    const used = incrementDailyCount();
    const useSize = size || cfg.default_size || "1024x1024";
    console.info("[绘图] [群 " + groupId + "] 今日第" + used + "/" + dailyMax + "次 后端=" + usedModelId + " format=" + (cfg.format || "openai") + " img2img=" + (inputB64 ? "true" : "false"));
    const result = callBackend(cfg, finalPrompt, useSize, inputB64);
    if (!result.ok) return { ok: false, text: log(true, "绘图失败: " + ((result && result.error) ? result.error : "未知错误")) };

    let b64 = result.b64 || "";
    if (!b64 && result.url) { const dl = image.download(result.url, "result.png"); if (dl && dl.success && dl.text) b64 = dl.text; }
    if (!b64) return { ok: false, text: log(true, "绘图失败: 未获取到图片数据") };

    const remain = dailyMax - getDailyCount();
    return { ok: true, b64: b64, mime: guessMime(b64), text: log(false, "图片生成成功（b64长度=" + b64.length + "，今日剩余 " + remain + " 次）") };
  } catch (e) {
    const msg = (e && e.message) ? e.message : String(e);
    return { ok: false, text: log(true, "绘图出现错误: " + msg) };
  }
}

// ===== 拆图 layer_split（同步） =====

function layerSplit(params, context) {
  const groupId = (context && context.groupId) ? context.groupId : "";
  function log(err, msg) { const t = String(msg); if (err) console.warn("[拆图] [群 " + groupId + "] " + t); else console.info("[拆图] [群 " + groupId + "] " + t); return t; }
  try {
    if (!getConfigValue("plugin.enabled", true)) return { ok: false, text: log(false, "绘画功能未启用") };
    let dailyMax = getConfigValue("limits.daily_max", 20); if (!dailyMax || dailyMax <= 0) dailyMax = 20;
    if (getDailyCount() >= dailyMax) return { ok: false, text: log(false, "今日绘图次数已用完") };
    if (!getConfigValue("layer_split.enabled", true)) return { ok: false, text: log(false, "拆图工具已手动关闭（layer_split.enabled=false）") };

    const imageRef = parseInt(params && params.image_ref, 10) || 0;
    if (imageRef < 1) return { ok: false, text: log(false, "拆图失败: 请指定图片序号（image_ref）") };
    const imageUrls = (context && context.image_urls) ? context.image_urls : [];
    if (!imageUrls || imageUrls.length < imageRef) return { ok: false, text: log(false, "拆图失败: 找不到 [图片" + imageRef + "]") };
    const splitPrompt = (params && params.prompt) ? String(params.prompt) : "";
    const modelId = (params && params.model_id) ? String(params.model_id) : "";

    const backends = getConfigValue("models.backends", {});
    const defaultModelId = getConfigValue("models.default_model_id", "");
    let cfg = null, usedModelId = "";
    if (modelId && backends[modelId]) { cfg = backends[modelId]; usedModelId = modelId; }
    else if (defaultModelId && backends[defaultModelId]) { cfg = backends[defaultModelId]; usedModelId = defaultModelId; }
    else { for (const k in backends) { if (Object.prototype.hasOwnProperty.call(backends, k)) { cfg = backends[k]; usedModelId = k; break; } } }
    if (!cfg) return { ok: false, text: log(false, "拆图失败: 未配置绘图后端") };
    if (!isArkBackend(cfg)) return { ok: false, text: log(false, "拆图失败: 当前后端(" + usedModelId + ")不支持图层拆分，请使用火山方舟 Seedream 5.0 pro（即梦）") };
    if (String(cfg.model || "").indexOf("seedream-5-0-pro") < 0) console.warn("[拆图] 当前模型可能不支持拆图");

    const inputB64 = downloadRefBase64(imageUrls[imageRef - 1], imageRef);
    if (!inputB64) return { ok: false, text: log(false, "拆图失败: 参考图下载失败") };

    const payload = {
      model: cfg.model, image: toDataUri(inputB64), size: cfg.default_size || "2K",
      layer_decomposition: true, response_format: "url", output_format: "png"
    };
    if (splitPrompt) payload.prompt = splitPrompt;
    if (cfg.watermark !== undefined && cfg.watermark !== null && cfg.watermark !== "") {
      payload.watermark = (cfg.watermark === true || String(cfg.watermark).toLowerCase() === "true");
    }

    const used = incrementDailyCount();
    const endpoint = String(cfg.base_url || "").replace(/\/+$/, "") + "/images/generations";
    console.info("[拆图] [群 " + groupId + "] 今日第" + used + "/" + dailyMax + "次 请求拆图 " + endpoint + " model=" + cfg.model);
    const resp = httpPostJson(endpoint, payload, buildAuthHeaders(cfg));
    if (resp.status < 200 || resp.status >= 300) return { ok: false, text: log(true, "拆图失败: " + httpErr(resp)) };
    let data; try { data = JSON.parse(resp.body); } catch (e) { return { ok: false, text: log(true, "拆图失败: 响应非合法JSON") }; }
    const items = (data && data.data) ? data.data : [];
    if (!items.length) return { ok: false, text: log(true, "拆图失败: 响应未包含图层数据") };

    let base = null; const layers = [];
    for (const it of items) { if (it.z_index === 0) { if (!base) base = it; } else { layers.push(it); } }
    if (!base) base = items[0];
    if (!base || !base.url) return { ok: false, text: log(true, "拆图失败: 未获取到底图URL") };

    const segs = []; const labels = [];
    const baseDl = image.download(base.url, "layer_base.png");
    if (baseDl && baseDl.success && baseDl.text) { segs.push({ type: "image", content: baseDl.text }); labels.push("底图"); }
    let layerCount = 0;
    for (let j = 0; j < layers.length && j < 16; j++) {
      const l = layers[j];
      if (!l.url) continue;
      const ld = image.download(l.url, "layer_" + j + ".png");
      if (!ld || !ld.success || !ld.text) continue;
      segs.push({ type: "image", content: ld.text });
      layerCount++;
      let piece = (j + 1) + ". " + (l.name || ("图层" + (j + 1))) + (l.description ? "（" + l.description + "）" : "");
      if (piece.length > 120) piece = piece.substring(0, 120) + "…";
      labels.push(piece);
    }
    if (!segs.length) return { ok: false, text: log(true, "拆图失败: 底图和图层均下载失败") };

    const summary = "已拆出底图 + " + layerCount + " 个图层（图层为透明PNG，可单独编辑/重组）：";
    const remain = dailyMax - getDailyCount();
    return { ok: true, segs: segs, text: log(false, summary + "\n" + labels.join("\n") + "\n（今日剩余 " + remain + " 次）") };
  } catch (e) {
    const msg = (e && e.message) ? e.message : String(e);
    return { ok: false, text: log(true, "拆图出现错误: " + msg) };
  }
}

// ===== 事件订阅 + 导出（全部同步，返回普通对象） =====

event.subscribe("art.generate", function (ev) {
  const p = ev && ev.payload ? ev.payload : {};
  const result = generatePicture(p, p.context);
  signal.all({ topic: "art.generate", result: result });
  return result;
});

event.subscribe("art.layer_split", function (ev) {
  const p = ev && ev.payload ? ev.payload : {};
  const result = layerSplit(p, p.context);
  signal.all({ topic: "art.layer_split", result: result });
  return result;
});

exportFunction("generatePicture", function (params) {
  return generatePicture(params);
});

exportFunction("layerSplit", function (params) {
  return layerSplit(params);
});

// ===== 加载通报 =====

(function () {
  const n = Object.keys(getConfigValue("models.backends", {})).length;
  console.info("[绘图] LTP9 绘图插件已加载（LTP9-Flash 同步），后端数量=" + n);
})();