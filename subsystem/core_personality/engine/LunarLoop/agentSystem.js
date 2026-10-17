var agentSystem = (function (exports) {
    'use strict';

    class GlobalConfig {
        static customConfig = { agent: {}, server: {} };
        static imageFormatsExtensions = ['.jpg', '.jpeg', '.png', '.gif', '.bmp', '.webp'];
        static videoFormatsExtensions = ['.mp4', '.avi', '.mov', '.wmv', '.flv', '.mkv'];
        static LTPfunction = new Map();
        static unreadRecords = [];
        static unreadContext = [];
        static reasoningInProgress = false;
        static finalResponse = "";
        static memoryReady = false;
        static currentAddress = [];
        static get MultimodalUrl() {
            return GlobalConfig.customConfig?.agent?.multimodal_url || url()[0] + '/v1';
        }
        ;
        static get MultimodalKey() {
            return GlobalConfig.customConfig?.agent?.multimodal_key || 'key-520-1314-2000-02-18';
        }
        ;
        static get MultimodalName() {
            return GlobalConfig.customConfig?.agent?.multimodal_model || "system-multimodal";
        }
        ;
        static get EmbeddingName() {
            return GlobalConfig.customConfig?.agent?.embedding_model || "system-embedding";
        }
        ;
        static get AsrName() {
            return GlobalConfig.customConfig?.agent?.asr_model || "system-asr";
        }
        ;
        static get debugMode() {
            return GlobalConfig.customConfig?.server?.developer ?? false;
        }
        ;
        static get VideoInputMode() {
            return GlobalConfig.customConfig?.multimodal?.video_input === 'frames' ? 'frames' : 'file';
        }
        ;
        static get ImageInputMode() {
            return GlobalConfig.customConfig?.agent?.image_input === 'base64' ? 'base64' : 'url';
        }
        ;
        static get VisionMaxSide() {
            const side = Number(GlobalConfig.customConfig?.agent?.vision_max_side);
            return Number.isFinite(side) && side > 0 ? side : 1080;
        }
        ;
    }

    const FALLBACK_PATTERN = '@([DIVAT])\\.(\\d{3,})(?:\\|(?:§(\\d+)|L(\\d+)-(\\d+)))?';
    const HANDLE_PATTERN = (() => {
        try {
            const grammar = resourceGrammar();
            if (grammar && typeof grammar.pattern === 'string' && grammar.pattern.length > 0)
                return grammar.pattern;
        }
        catch {
        }
        return FALLBACK_PATTERN;
    })();
    const HANDLE_EXACT = new RegExp(`^${HANDLE_PATTERN}$`);
    const KIND_LABELS = {
        D: '文档',
        I: '图片',
        V: '视频',
        A: '音频',
        T: '表格'
    };
    function parseHandle(raw) {
        if (!raw)
            return null;
        let text = String(raw).trim();
        text = text.replace(/^[[【]/, '').replace(/[\]】]$/, '');
        const match = HANDLE_EXACT.exec(text);
        if (!match)
            return null;
        const seq = Number(match[2]);
        if (!Number.isFinite(seq) || seq <= 0)
            return null;
        const handle = {
            kind: match[1],
            seq,
            section: 0,
            lineStart: 0,
            lineEnd: 0,
            anchor: 'none'
        };
        if (match[3] !== undefined && match[3] !== '') {
            handle.anchor = 'section';
            handle.section = Number(match[3]);
        }
        else if (match[4] !== undefined && match[4] !== '') {
            handle.anchor = 'line';
            handle.lineStart = Number(match[4]);
            handle.lineEnd = Number(match[5] ?? match[4]);
        }
        return handle;
    }
    function padSeq(seq) {
        const width = seq >= 1000 ? 4 : 3;
        return String(seq).padStart(width, '0');
    }
    function formatHandle(handle) {
        let text = `@${handle.kind}.${padSeq(handle.seq)}`;
        if (handle.anchor === 'section' && handle.section)
            text += `|§${handle.section}`;
        else if (handle.anchor === 'line' && handle.lineStart) {
            text += `|L${handle.lineStart}-${handle.lineEnd || handle.lineStart}`;
        }
        return text;
    }
    function handleOf(kind, seq) {
        return `@${kind}.${padSeq(seq)}`;
    }
    function sectionHandle(kind, seq, ordinal) {
        return `${handleOf(kind, seq)}|§${ordinal}`;
    }
    function grammarCheatsheet() {
        return '句柄形如 @D.003（文档）、@I.007（图片）、@V.001（视频）、@A.002（音频）；'
            + '段落锚点可写 @D.003|§4（第 4 节）或 @D.003|L120-168（第 120-168 行）';
    }

    const BASE64_ALPHABET = 'ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/';
    function bytesToBase64(bytes) {
        let out = '';
        for (let i = 0; i < bytes.length; i += 3) {
            const b0 = bytes[i];
            const b1 = i + 1 < bytes.length ? bytes[i + 1] : 0;
            const b2 = i + 2 < bytes.length ? bytes[i + 2] : 0;
            out += BASE64_ALPHABET[b0 >> 2];
            out += BASE64_ALPHABET[((b0 & 0x03) << 4) | (b1 >> 4)];
            out += i + 1 < bytes.length ? BASE64_ALPHABET[((b1 & 0x0f) << 2) | (b2 >> 6)] : '=';
            out += i + 2 < bytes.length ? BASE64_ALPHABET[b2 & 0x3f] : '=';
        }
        return out;
    }
    function admitResource(input) {
        try {
            const payload = input.data instanceof Uint8Array
                ? { ...input, data: bytesToBase64(input.data) }
                : input;
            const [result, error] = resourceAdmit(payload);
            if (error || !result) {
                console.error('[资源] 入库失败:', error?.message || '返回为空', input.sourceKind, input.name || input.source || '');
                return null;
            }
            return result;
        }
        catch (error) {
            console.error('[资源] 入库抛异常:', error);
            return null;
        }
    }
    function admitBytes(data, name, origin = 'tool-output', retain) {
        return admitResource({ sourceKind: 'bytes', data, name, origin, retain });
    }
    function readResource(handle, maxBytes) {
        const parsed = parseHandle(handle);
        if (!parsed) {
            console.error('[资源] 非法句柄:', handle);
            return null;
        }
        try {
            const [result, error] = resourceRead(formatHandle(parsed), maxBytes);
            if (error || !result) {
                console.error('[资源] 取回失败:', error?.message || '返回为空', handle);
                return null;
            }
            return result;
        }
        catch (error) {
            console.error('[资源] 取回抛异常:', error);
            return null;
        }
    }
    function outlineResource(handle) {
        const parsed = parseHandle(handle);
        if (!parsed)
            return [];
        try {
            const [sections, , error] = resourceOutline(formatHandle(parsed));
            if (error || !sections) {
                console.error('[资源] 读大纲失败:', error?.message || '返回为空', handle);
                return [];
            }
            return sections;
        }
        catch (error) {
            console.error('[资源] 读大纲抛异常:', error);
            return [];
        }
    }
    function listResources(kind = '', limit = 50) {
        try {
            const [entries, error] = resourceList(kind, limit);
            if (error || !entries) {
                console.error('[资源] 列目录失败:', error?.message || '返回为空');
                return [];
            }
            return entries;
        }
        catch (error) {
            console.error('[资源] 列目录抛异常:', error);
            return [];
        }
    }
    function searchResources(query, kind = '', limit = 10) {
        if (!query || !query.trim())
            return [];
        try {
            const [hits, error] = resourceSearch(query.trim(), kind, limit);
            if (error || !hits) {
                console.error('[资源] 搜索失败:', error?.message || '返回为空');
                return [];
            }
            return hits;
        }
        catch (error) {
            console.error('[资源] 搜索抛异常:', error);
            return [];
        }
    }
    function resourceStatistics() {
        try {
            const [stats, error] = resourceStats();
            return error ? null : stats;
        }
        catch (error) {
            console.error('[资源] 统计抛异常:', error);
            return null;
        }
    }
    function setResourceSummary(id, summary, tags) {
        try {
            const [ok, error] = resourceSetSummary(id, summary, tags ?? []);
            if (error || !ok)
                console.error('[资源] 写摘要失败:', error?.message || '返回失败');
        }
        catch (error) {
            console.error('[资源] 写摘要抛异常:', error);
        }
    }
    function setResourceSections(id, sections) {
        try {
            const [ok, error] = resourceSetSections(id, sections);
            if (error || !ok) {
                console.error('[资源] 写段落表失败:', error?.message || '返回失败');
                return false;
            }
            return true;
        }
        catch (error) {
            console.error('[资源] 写段落表抛异常:', error);
            return false;
        }
    }
    function getResourceEntry(handle) {
        const parsed = parseHandle(handle);
        if (!parsed)
            return null;
        try {
            const [entries, error] = resourceList(parsed.kind, 1000);
            if (error || !entries)
                return null;
            return entries.find(entry => entry.seq === parsed.seq) ?? null;
        }
        catch (error) {
            console.error('[资源] 查条目抛异常:', error);
            return null;
        }
    }
    function deleteResource(handle) {
        const parsed = parseHandle(handle);
        if (!parsed)
            return false;
        try {
            const [ok, error] = resourceDelete(formatHandle(parsed));
            if (error || !ok) {
                console.error('[资源] 删除失败:', error?.message || '返回失败', handle);
                return false;
            }
            return true;
        }
        catch (error) {
            console.error('[资源] 删除抛异常:', error);
            return false;
        }
    }
    function admitImageDataURI(imageData, name, origin = 'client-upload') {
        if (!imageData)
            return null;
        if (imageData.startsWith('data:')) {
            return admitResource({ sourceKind: 'data', source: imageData, name, origin, kindHint: 'I', dedup: true });
        }
        const sourceKind = /^https?:\/\//i.test(imageData) ? 'url' : 'path';
        return admitResource({ sourceKind, source: imageData, name, origin, kindHint: 'I', dedup: true });
    }
    function admitDocumentByName(fileName) {
        const target = String(fileName || '').trim().replace(/^[#@]/, '');
        if (!target)
            return null;
        const name = target.replace(/^\.\//, '');
        const fromWorkspace = admitResource({ sourceKind: 'workspace', source: target, name, origin: 'client-upload', retain: 'normal' });
        if (fromWorkspace)
            return fromWorkspace;
        return admitResource({ sourceKind: 'path', source: target, name, origin: 'client-upload', retain: 'normal' });
    }
    function degradeNotice(handle, reason) {
        const parsed = parseHandle(handle);
        const label = parsed ? formatHandle(parsed) : handle;
        return `${label}（${reason}）`;
    }

    const IMAGE_WINDOW_SIZE = 20;
    const VISION_READ_PATH = '/file/read-multimodal/';
    const TAG_PATTERN = /^@[A-Z]\.\d{3,}$/;
    function guessImageExt(source) {
        const lower = (source || '').toLowerCase();
        const dataMatch = /^data:image\/(\w+)[;,]/.exec(lower);
        if (dataMatch)
            return `.${dataMatch[1] === 'jpeg' ? 'jpg' : dataMatch[1]}`;
        const path = lower.split('?')[0].split('#')[0];
        const ext = path.slice(path.lastIndexOf('.'));
        return /^\.[a-z0-9]{2,5}$/.test(ext) ? ext : '.png';
    }
    function handleFromVisionURL(url) {
        if (typeof url !== 'string')
            return '';
        const match = /[?&]tag=([^&]+)/.exec(url);
        if (!match)
            return '';
        let tag = match[1];
        try {
            tag = decodeURIComponent(tag);
        }
        catch {
        }
        return TAG_PATTERN.test(tag) ? tag : '';
    }
    function attachImageTags(message) {
        if (!message || typeof message.content === 'string' || !Array.isArray(message.content))
            return;
        const content = message.content;
        for (const item of content) {
            if (item?.type !== 'image_url')
                continue;
            const source = item.image_url?.url || '';
            if (!source)
                continue;
            const lower = source.toLowerCase();
            if (lower.startsWith('data:video/') || GlobalConfig.videoFormatsExtensions.some(ext => lower.endsWith(ext)))
                continue;
            const admitted = admitImageDataURI(source, `image${guessImageExt(source)}`);
            if (!admitted?.handle || !admitted.blob_path) {
                console.error('[图片标签] 入库失败，该图片以原始来源流转（无句柄）:', source.substring(0, 60));
                continue;
            }
            item.image_url.url = visionReference(admitted.handle, admitted.blob_path);
        }
    }
    function visionReference(handle, blobPath) {
        try {
            if (GlobalConfig.ImageInputMode === 'base64') {
                const [dataURI, error] = visionRead(blobPath, handle);
                if (!error && dataURI)
                    return dataURI;
                console.error('[图片标签] 视觉读取失败，退化为端点 URL:', error?.message ?? String(error));
            }
            return `${url()[0]}${VISION_READ_PATH}${blobPath}?tag=${encodeURIComponent(handle)}`;
        }
        catch (error) {
            console.error('[图片标签] 生成视觉引用失败:', error);
            return '';
        }
    }
    function sanitizeImageDesc(raw, maxChars) {
        if (typeof raw !== 'string')
            return '';
        let text = raw.replace(/\s+/g, ' ').trim();
        if (!text)
            return '';
        text = text.replace(/^[「『"'“”‘’]+/, '').replace(/[」』"'“”‘’]+$/, '').trim();
        text = text.replace(/[[\]【】〔〕（）()]/g, '').trim();
        text = text.replace(/^[，。、；：,.!?！？~～…\-\s]+/, '').replace(/[，。、；：,.!?！？~～…\-\s]+$/, '').trim();
        if (!text)
            return '';
        if (text.length > maxChars)
            text = text.slice(0, maxChars).trim() + '…';
        return text;
    }
    function composeImagePlaceholder(tag, desc) {
        const clean = sanitizeImageDesc(desc, 40);
        return clean ? `[${tag}]（${clean}）` : `[${tag}]`;
    }
    function resolveImageRef(raw) {
        const trimmed = String(raw || '').trim();
        if (!trimmed)
            return null;
        const handle = parseHandle(trimmed);
        if (!handle || handle.kind !== 'I')
            return null;
        const normalized = formatHandle(handle);
        const entry = getResourceEntry(normalized);
        if (!entry)
            return null;
        const source = entry.state === 'active' && entry.blob_path ? entry.blob_path : '';
        return { handle: normalized, source, desc: entry.summary || '' };
    }

    let agentTargets = null;
    function registerAgentTargets(targets) {
        agentTargets = targets;
    }
    const agentControlTools = [
        {
            type: "function",
            function: {
                name: "dispatch_painter",
                description: "向绘图者子智能体发布绘画创作任务。绘图者会完善需求并调用专业工具生成图像，完成后将作品直接推送至前端展示。若用户要求参照图片创作（改画风、换场景、改服饰、保留角色容貌等），通过 reference_tag 指定图片句柄（形如 @I.003），绘图者将以该图为图生图基座。",
                parameters: {
                    type: 'object',
                    properties: {
                        description: {
                            type: 'string',
                            description: "绘画需求描述，如'画一只在樱花树下的白猫'、'把 @I.003 画成雪景'。描述越详细，绘画效果越好。"
                        },
                        reference_tag: {
                            type: 'string',
                            description: "图生图基座图的图片句柄（如 @I.003）。用户要求参照某张图片创作时，填该图片在消息中出现的 [@I.00x] 句柄；纯文生图时省略。"
                        }
                    },
                    required: ["description"]
                }
            }
        },
        {
            type: "function",
            function: {
                name: "dispatch_musician",
                description: "向演奏者子智能体发布音乐创作任务。演奏者会完善需求并调用专业工具创作音乐，完成后将乐谱和音频直接推送至前端展示。",
                parameters: {
                    type: "object",
                    properties: {
                        description: {
                            type: "string",
                            description: "音乐需求描述，如'创作一首轻快的钢琴曲'、'写一首抒情的钢琴与大提琴二重奏'。描述越详细，音乐创作效果越好。"
                        }
                    },
                    required: ["description"]
                }
            }
        }
    ];
    function parseArgs$1(args) {
        return typeof args === 'string' ? JSON.parse(args) : (args || {});
    }
    async function handleDispatchPainter(args) {
        const { description, reference_tag } = parseArgs$1(args);
        if (!description || typeof description !== 'string' || description.trim().length === 0) {
            return ['绘画任务调度失败：创作描述不能为空，请提供具体的绘画需求', ''];
        }
        if (!agentTargets?.painterRole) {
            return ['绘画任务调度失败：绘制者子智能体未就绪，请稍后重试', ''];
        }
        let reference;
        if (reference_tag) {
            const resolved = resolveImageRef(String(reference_tag));
            if (!resolved) {
                return [`绘画任务调度失败：图片句柄 ${reference_tag} 不存在，请用 list_images 查看可引用的图片句柄后重试`, ''];
            }
            if (!resolved.source) {
                return [`绘画任务调度失败：${resolved.handle} 的图片本体已从内容仓库回收（仅存描述${resolved.desc ? `：${resolved.desc}` : ''}），请让用户重新发送该图片，或换一张图片的句柄`, ''];
            }
            reference = resolved;
        }
        console.log(`[智能体控制] 调度绘制者${reference ? `（图生图，参考图 ${reference.handle}）` : ''}: ${description.trim().split('\n')[0]}`);
        const result = await agentTargets.painterRole.createCreativeWork(description.trim(), reference);
        console.log(`[智能体控制] 绘制者完成: ${result}`);
        return [result, ''];
    }
    async function handleDispatchMusician(args) {
        const { description } = parseArgs$1(args);
        if (!description || typeof description !== 'string' || description.trim().length === 0) {
            return ['音乐任务调度失败：创作描述不能为空，请提供具体的音乐需求', ''];
        }
        if (!agentTargets?.musicianRole) {
            return ['音乐任务调度失败：演奏者子智能体未就绪，请稍后重试', ''];
        }
        console.log(`[智能体控制] 调度演奏者: ${description}`);
        const result = await agentTargets.musicianRole.createCreativeWork(description.trim());
        console.log(`[智能体控制] 演奏者完成: ${result}`);
        return [result, ''];
    }
    GlobalConfig.LTPfunction.set('dispatch_painter', handleDispatchPainter);
    GlobalConfig.LTPfunction.set('dispatch_musician', handleDispatchMusician);

    let describeRole = null;
    function registerDescribeRole(role) {
        describeRole = role;
    }
    function storedSummary(handle) {
        const entry = getResourceEntry(handle);
        return entry?.summary || '';
    }
    function describeResource(handle, question = '') {
        const parsed = parseHandle(handle);
        if (!parsed)
            return '';
        const normalized = formatHandle(parsed);
        const cached = storedSummary(normalized);
        if (cached && !question)
            return sanitizeImageDesc(cached, 200);
        if (parsed.kind === 'I') {
            const source = resourceSource(normalized);
            if (!describeRole || !source)
                return sanitizeImageDesc(cached, 200);
            try {
                const instruction = question
                    ? `用户想知道：${question}。请观察这张图片，用不超过100字的中文描述其中与该问题相关的关键信息，直接输出描述本身。`
                    : '用一句不超过20字的中文描述这张图片的主体与场景，只输出描述本身。';
                describeRole.coverContext({
                    role: 'user',
                    content: [
                        { type: 'text', text: instruction },
                        { type: 'image_url', image_url: { url: source } }
                    ]
                });
                const response = describeRole.run([], []);
                const desc = sanitizeImageDesc(response.body?.choices?.[0]?.message?.content || '', 200);
                if (desc && !question) {
                    const entry = getResourceEntry(normalized);
                    if (entry)
                        setResourceSummary(entry.id, desc);
                }
                return desc || sanitizeImageDesc(cached, 200);
            }
            catch (error) {
                console.error(`[资源描述] ${normalized} 生成失败:`, error);
                return sanitizeImageDesc(cached, 200);
            }
        }
        return sanitizeImageDesc(cached, 200);
    }
    function resourceSource(handle) {
        const result = readResource(handle, 8 * 1024 * 1024);
        if (!result || result.lost || !result.base64)
            return '';
        if (result.truncated)
            return '';
        const mime = result.mime || 'image/png';
        return `data:${mime};base64,${result.base64}`;
    }

    const LIST_LIMIT_MAX$1 = 100;
    const imageRecallTools = [
        {
            type: "function",
            function: {
                name: "list_images",
                description: "列出当前可引用的图片句柄清单（句柄 + 内容描述），按登记时间倒序，只返回文字不含图片。当你不确定某张图片的句柄、或用户提到一张你已看不到的图片时，先用本工具查看清单，再用 describe_images 按句柄取内容描述；需要正文式的统一清单（含文档/视频/音频）请用 list_resources。",
                parameters: {
                    type: "object",
                    properties: {
                        limit: {
                            type: "number",
                            description: "返回条目数量上限，默认 30"
                        }
                    },
                    required: []
                }
            }
        },
        {
            type: "function",
            function: {
                name: "describe_images",
                description: "按图片句柄获取图片的内容描述（视觉模型解读，不注入图片本体，上下文成本极低）。占位文本 [@I.00x]（…）括号内已是内容描述，无需再调用；需要了解描述之外的细节（如「图上写了什么字」、画面里第二个人的穿着）时可带 question 定向提问。句柄可从消息中的旁注占位、list_images 清单或合并转发文本中获得；交给绘图者参照创作时无需调用，直接在 dispatch_painter 的 reference_tag 里写句柄。",
                parameters: {
                    type: "object",
                    properties: {
                        tags: {
                            type: "array",
                            items: { type: "string" },
                            description: "图片句柄数组，如 [\"@I.003\", \"@I.007\"]（写作 [@I.003] 也可以）"
                        },
                        question: {
                            type: "string",
                            description: "可选：用户当前关心的具体问题（如'图上写了什么字'、'第二个人穿什么'），描述将围绕该问题定向生成"
                        }
                    },
                    required: ["tags"]
                }
            }
        }
    ];
    function describeEntry(tag, question) {
        const entry = getResourceEntry(tag);
        if (entry?.summary && !question)
            return sanitizeImageDesc(entry.summary, 200);
        const described = describeResource(tag, question);
        return described || sanitizeImageDesc(entry?.summary || '', 200);
    }
    async function handleListImages(args) {
        const parsed = typeof args === 'string' ? safeParse$1(args) : (args || {});
        const limit = Math.min(Math.max(Number(parsed.limit) || 30, 1), LIST_LIMIT_MAX$1);
        const lines = [];
        for (const entry of listResources('I', limit)) {
            const desc = entry.summary ? sanitizeImageDesc(entry.summary, 40) : '（暂无描述，可用 describe_images 获取）';
            const lost = entry.state === 'active' ? '' : ' [本体已回收]';
            lines.push(`[${entry.handle}] ${desc}${lost}`);
        }
        if (lines.length === 0)
            return ['当前没有可引用的图片：资源目录中还没有任何图片入库', ''];
        const text = `当前可引用的图片句柄（共 ${lines.length} 条，越靠前越新）：`
            + `\n${lines.join('\n')}`
            + '\n句柄形如 @I.003，来自统一资源目录：重启后依然有效，本体被回收时会降级为描述。';
        return [text, ''];
    }
    async function handleDescribeImages(args) {
        const parsed = typeof args === 'string' ? safeParse$1(args) : (args || {});
        const rawTags = Array.isArray(parsed.tags) ? parsed.tags : (parsed.tag ? [parsed.tag] : []);
        const resolved = rawTags
            .map(item => resolveImageRef(String(item || '')))
            .filter((item) => item !== null);
        if (resolved.length === 0) {
            return ['取图失败：请提供图片句柄数组，如 ["@I.003"]；句柄可用 list_images 或 list_resources 查看', ''];
        }
        const question = String(parsed.question || '').trim().slice(0, 100);
        const lines = [];
        for (const ref of resolved) {
            const desc = describeEntry(ref.handle, question);
            lines.push(`[${ref.handle}] ${desc || '（描述生成失败，源图可能已失效）'}`);
        }
        let text = `共取回 ${lines.length} 张图片的内容描述：\n${lines.join('\n')}`;
        if (resolved.length !== rawTags.length) {
            text += `\n（有 ${rawTags.length - resolved.length} 个句柄无法解析，已跳过；可用 list_images 查看当前可引用句柄）`;
        }
        console.log(`[图片描述] 已取回 ${lines.length} 张图片描述`);
        return [text, ''];
    }
    function safeParse$1(text) {
        try {
            const parsed = JSON.parse(text);
            return parsed && typeof parsed === 'object' ? parsed : {};
        }
        catch {
            return {};
        }
    }
    GlobalConfig.LTPfunction.set('list_images', handleListImages);
    GlobalConfig.LTPfunction.set('describe_images', handleDescribeImages);

    const SCHEDULE_TRIGGER_PREFIX = '[计划提醒]';
    const SCHEDULE_OVERDUE_ABANDON_MS = 30 * 60 * 1000;
    const PRESET_DAILY_TASKS = [
        { id: 'daily_greeting_0630', type: 'daily', time: '06:30', content: '向用户发送清晨早安问候，关心其今日安排' },
        { id: 'daily_greeting_0800', type: 'daily', time: '08:00', content: '向用户发送早间问候，精神饱满开启一天' },
        { id: 'daily_greeting_1000', type: 'daily', time: '10:00', content: '向用户发送"上午好，该喝水啦"的补水提醒' },
        { id: 'daily_greeting_1200', type: 'daily', time: '12:00', content: '向用户发送午安问候，关心其午餐情况' },
        { id: 'daily_greeting_1500', type: 'daily', time: '15:00', content: '向用户发送"下午好，该喝水啦"的补水提醒' },
        { id: 'daily_greeting_1730', type: 'daily', time: '17:30', content: '向用户发送傍晚问候，关心一天收尾与晚间安排' },
        { id: 'daily_greeting_2230', type: 'daily', time: '22:30', content: '向用户发送晚安问候，祝其好眠' },
    ];
    const SCHEDULE_FILE_PATH = 'database/schedule.json';
    const scheduleTools = [
        {
            type: "function",
            function: {
                name: "create_schedule",
                description: "在计划表中创建新的计划项，指定执行时间点和对应的工作内容。时间格式为 ISO 8601 (如 '2026-06-14T15:30:00') 或中文日期时间格式 (如 '2026年6月14日 15:30')。",
                parameters: {
                    type: "object",
                    properties: {
                        time: {
                            type: "string",
                            description: "计划执行的时间点，支持 ISO 8601 格式或中文日期时间格式，例如 '2026-06-14T15:30:00' 或 '2026年6月14日 15:30'"
                        },
                        content: {
                            type: "string",
                            description: "计划执行的工作内容描述，应清晰说明需要完成的事项"
                        }
                    },
                    required: ["time", "content"]
                }
            }
        },
        {
            type: "function",
            function: {
                name: "edit_schedule",
                description: "编辑计划表中已存在的计划项，可修改其执行时间和/或工作内容。需要提供计划项的ID，至少提供 time 或 content 中的一个进行修改。",
                parameters: {
                    type: "object",
                    properties: {
                        id: {
                            type: "string",
                            description: "要编辑的计划项ID，从 query_schedule 返回结果中获得"
                        },
                        time: {
                            type: "string",
                            description: "修改后的执行时间点，不修改则留空"
                        },
                        content: {
                            type: "string",
                            description: "修改后的工作内容描述，不修改则留空"
                        }
                    },
                    required: ["id"]
                }
            }
        },
        {
            type: "function",
            function: {
                name: "delete_schedule",
                description: "从计划表中删除指定的计划项。需要提供计划项的ID。",
                parameters: {
                    type: "object",
                    properties: {
                        id: {
                            type: "string",
                            description: "要删除的计划项ID，从 query_schedule 返回结果中获得"
                        }
                    },
                    required: ["id"]
                }
            }
        },
        {
            type: "function",
            function: {
                name: "query_schedule",
                description: "查询计划表中已有的计划项列表，可按关键词筛选。返回所有匹配的计划项及其ID、时间和内容。",
                parameters: {
                    type: "object",
                    properties: {
                        keyword: {
                            type: "string",
                            description: "用于筛选计划项的关键词，留空则返回全部计划项"
                        }
                    },
                    required: []
                }
            }
        }
    ];

    const CN_DATE_REGEX = /^(\d{4})年(\d{1,2})月(\d{1,2})日\s+(\d{1,2}):(\d{2})(?::(\d{2}))?$/;
    function formatISO(date) {
        const y = date.getFullYear();
        const mo = String(date.getMonth() + 1).padStart(2, '0');
        const d = String(date.getDate()).padStart(2, '0');
        const h = String(date.getHours()).padStart(2, '0');
        const mi = String(date.getMinutes()).padStart(2, '0');
        const s = String(date.getSeconds()).padStart(2, '0');
        return `${y}-${mo}-${d}T${h}:${mi}:${s}`;
    }
    function formatDate(date) {
        const y = date.getFullYear();
        const mo = String(date.getMonth() + 1).padStart(2, '0');
        const d = String(date.getDate()).padStart(2, '0');
        return `${y}-${mo}-${d}`;
    }
    function normalizeTime(raw) {
        if (!raw || raw.trim().length === 0) {
            return null;
        }
        const trimmed = raw.trim();
        const stdDate = new Date(trimmed);
        if (!isNaN(stdDate.getTime())) {
            return formatISO(stdDate);
        }
        const cnMatch = trimmed.match(CN_DATE_REGEX);
        if (cnMatch) {
            const [, year, month, day, hour, minute, second] = cnMatch;
            return `${year.padStart(4, '0')}-${month.padStart(2, '0')}-${day.padStart(2, '0')}T${hour.padStart(2, '0')}:${minute.padStart(2, '0')}:${(second || '00').padStart(2, '0')}`;
        }
        return null;
    }
    function dailyTaskTime(item, now) {
        const match = item.time.trim().match(/^(\d{1,2}):(\d{2})$/);
        if (!match)
            return null;
        return new Date(now.getFullYear(), now.getMonth(), now.getDate(), Number(match[1]), Number(match[2]));
    }

    function loadSchedulesFromDisk() {
        const [fileData, , , readErr] = readFile(SCHEDULE_FILE_PATH);
        if (readErr) {
            return [];
        }
        try {
            const jsonStr = atob(fileData);
            const data = JSON.parse(jsonStr);
            return Array.isArray(data) ? data : [];
        }
        catch (e) {
            console.error('[计划表] 计划表数据解析失败:', e);
            return [];
        }
    }
    function saveSchedulesToDisk(schedules) {
        try {
            const jsonStr = JSON.stringify(schedules, null, 2);
            const [, , saveErr] = saveFile(SCHEDULE_FILE_PATH, true, jsonStr);
            if (saveErr) {
                console.error('[计划表] 保存计划表失败:', saveErr);
                return false;
            }
            return true;
        }
        catch (e) {
            console.error('[计划表] 序列化计划表失败:', e);
            return false;
        }
    }

    let scheduleCache = [];
    function initSchedules() {
        const raw = loadSchedulesFromDisk();
        const existingIds = new Set(raw.map(item => item.id));
        let added = 0;
        for (const preset of PRESET_DAILY_TASKS) {
            if (existingIds.has(preset.id))
                continue;
            raw.push({ ...preset });
            added++;
        }
        if (raw.length === 0) {
            saveSchedulesToDisk([]);
            scheduleCache = [];
            console.log('[计划表] 初始化完成，计划表为空');
            return;
        }
        let needsRewrite = added > 0;
        for (const item of raw) {
            if (item.type === 'daily')
                continue;
            const normalized = normalizeTime(item.time);
            if (normalized && normalized !== item.time) {
                item.time = normalized;
                needsRewrite = true;
            }
        }
        scheduleCache = raw;
        if (needsRewrite) {
            saveSchedulesToDisk(scheduleCache);
            if (added > 0)
                console.log(`[计划表] 已补充 ${added} 个预设每日任务`);
            console.log('[计划表] 已修正历史数据中的非标准时间格式');
        }
        console.log(`[计划表] 初始化完成，共加载 ${scheduleCache.length} 个计划项`);
    }
    function generateId() {
        function execute(character) {
            const randomValue = (Math.random() * 16) | 0;
            const maskedRandomValue = character === 'x' ? randomValue : (randomValue & 0x3 | 0x8);
            return maskedRandomValue.toString(16);
        }
        return 'xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx'.replace(/[xy]/g, execute);
    }
    function parseArgs(args) {
        return typeof args === 'string' ? JSON.parse(args) : (args || {});
    }
    async function handleCreateSchedule(args) {
        const { time, content } = parseArgs(args);
        if (!time || time.trim().length === 0) {
            return ['创建计划项失败：执行时间不能为空，请提供有效的时间点', ''];
        }
        if (!content || content.trim().length === 0) {
            return ['创建计划项失败：工作内容不能为空，请提供具体的计划内容', ''];
        }
        const normalizedTime = normalizeTime(time);
        if (!normalizedTime) {
            return [`创建计划项失败：无法解析时间格式 "${time}"，请使用 ISO 8601 格式 (如 "2026-06-14T15:30:00") 或中文格式 (如 "2026年6月14日 15:30")`, ''];
        }
        const newItem = {
            id: generateId(),
            time: normalizedTime,
            content: content.trim()
        };
        scheduleCache.push(newItem);
        if (!saveSchedulesToDisk(scheduleCache)) {
            scheduleCache.pop();
            return ['创建计划项失败：保存到磁盘时出错，请稍后重试', ''];
        }
        console.log(`[计划表] 创建成功: [${newItem.id}] ${newItem.time} - ${newItem.content}`);
        return [`计划项创建成功：ID为 ${newItem.id}，执行时间: ${newItem.time}，内容: ${newItem.content}`, ''];
    }
    async function handleEditSchedule(args) {
        const { id, time, content } = parseArgs(args);
        if (!id || id.trim().length === 0) {
            return ['编辑计划项失败：计划项ID不能为空，请从 query_schedule 获取有效ID', ''];
        }
        if ((!time || time.trim().length === 0) && (!content || content.trim().length === 0)) {
            return ['编辑计划项失败：至少需要提供 time 或 content 中的一个进行修改', ''];
        }
        const index = scheduleCache.findIndex(item => item.id === id.trim());
        if (index === -1) {
            return [`编辑计划项失败：未找到ID为 ${id} 的计划项，请使用 query_schedule 确认正确的ID`, ''];
        }
        const snapshot = { ...scheduleCache[index] };
        if (time && time.trim().length > 0) {
            const normalizedTime = normalizeTime(time);
            if (!normalizedTime) {
                return [`编辑计划项失败：无法解析时间格式 "${time}"，请使用 ISO 8601 或中文日期时间格式`, ''];
            }
            scheduleCache[index].time = normalizedTime;
        }
        if (content && content.trim().length > 0) {
            scheduleCache[index].content = content.trim();
        }
        if (!saveSchedulesToDisk(scheduleCache)) {
            scheduleCache[index] = snapshot;
            return ['编辑计划项失败：保存到磁盘时出错，请稍后重试', ''];
        }
        console.log(`[计划表] 编辑成功: [${id}] -> ${scheduleCache[index].time} - ${scheduleCache[index].content}`);
        return [`计划项编辑成功：ID为 ${id}，已更新为 执行时间: ${scheduleCache[index].time}，内容: ${scheduleCache[index].content}`, ''];
    }
    async function handleDeleteSchedule(args) {
        const { id } = parseArgs(args);
        if (!id || id.trim().length === 0) {
            return ['删除计划项失败：计划项ID不能为空', ''];
        }
        const index = scheduleCache.findIndex(item => item.id === id.trim());
        if (index === -1) {
            return [`删除计划项失败：未找到ID为 ${id} 的计划项`, ''];
        }
        const deletedItem = scheduleCache[index];
        scheduleCache.splice(index, 1);
        if (!saveSchedulesToDisk(scheduleCache)) {
            scheduleCache.splice(index, 0, deletedItem);
            return ['删除计划项失败：保存到磁盘时出错，请稍后重试', ''];
        }
        console.log(`[计划表] 删除成功: [${id}] ${deletedItem.time} - ${deletedItem.content}`);
        return [`计划项删除成功：已移除 [${deletedItem.id}] ${deletedItem.time} - ${deletedItem.content}`, ''];
    }
    async function handleQuerySchedule(args) {
        const { keyword } = parseArgs(args);
        if (scheduleCache.length === 0) {
            return ['当前计划表为空，没有任何计划项，可以放心创建新计划。', ''];
        }
        const sorted = [...scheduleCache].sort((a, b) => new Date(a.time).getTime() - new Date(b.time).getTime());
        const filtered = keyword && keyword.trim().length > 0
            ? sorted.filter(item => item.content.includes(keyword.trim()) || item.time.includes(keyword.trim()))
            : sorted;
        if (filtered.length === 0) {
            return [`未找到包含关键词 "${keyword}" 的计划项，当前共有 ${scheduleCache.length} 个计划项。`, ''];
        }
        return [`当前共有 ${scheduleCache.length} 个计划项` + (keyword ? `，匹配 "${keyword}" 的有 ${filtered.length} 条` : '') + ':\n' +
                filtered.map((item, i) => `[计划项${i + 1}] ID:${item.id} | 时间:${item.time} | 内容:${item.content}`).join('\n'), ''];
    }
    function checkDueItems() {
        if (scheduleCache.length === 0)
            return [];
        const now = new Date();
        const todayStr = formatDate(now);
        const kept = [];
        const removed = [];
        const candidates = [];
        for (const item of scheduleCache) {
            if (item.type === 'daily') {
                const todayTime = dailyTaskTime(item, now);
                if (todayTime && now >= todayTime && item.completedDate !== todayStr) {
                    if (now.getTime() - todayTime.getTime() > SCHEDULE_OVERDUE_ABANDON_MS) {
                        item.completedDate = todayStr;
                        console.log(`[计划表] 每日任务超时放弃: [${item.id}] ${item.time} - ${item.content}`);
                    }
                    else
                        candidates.push({ item, dueTime: todayTime });
                }
                kept.push(item);
                continue;
            }
            const itemTime = new Date(item.time);
            if (isNaN(itemTime.getTime())) {
                console.warn(`[计划表] 无效的时间格式，跳过: [${item.id}] ${item.time}`);
                kept.push(item);
                continue;
            }
            if (now < itemTime) {
                kept.push(item);
                continue;
            }
            if (now.getTime() - itemTime.getTime() > SCHEDULE_OVERDUE_ABANDON_MS) {
                removed.push(item);
                console.log(`[计划表] 一次性任务超时放弃: [${item.id}] ${item.time} - ${item.content}`);
                continue;
            }
            candidates.push({ item, dueTime: itemTime });
        }
        const executed = [];
        if (candidates.length > 0) {
            candidates.sort((a, b) => Math.abs(now.getTime() - a.dueTime.getTime()) - Math.abs(now.getTime() - b.dueTime.getTime()));
            const chosen = candidates[0].item;
            if (chosen.type === 'daily') {
                chosen.completedDate = todayStr;
                console.log(`[计划表] 触发每日任务: [${chosen.id}] ${chosen.time} - ${chosen.content}`);
            }
            else {
                removed.push(chosen);
                console.log(`[计划表] 触发到期计划项: [${chosen.id}] ${chosen.time} - ${chosen.content}`);
            }
            executed.push(chosen);
        }
        if (executed.length > 0 || removed.length > 0) {
            scheduleCache = kept;
            saveSchedulesToDisk(scheduleCache);
        }
        return executed;
    }
    initSchedules();
    GlobalConfig.LTPfunction.set('create_schedule', handleCreateSchedule);
    GlobalConfig.LTPfunction.set('edit_schedule', handleEditSchedule);
    GlobalConfig.LTPfunction.set('delete_schedule', handleDeleteSchedule);
    GlobalConfig.LTPfunction.set('query_schedule', handleQuerySchedule);

    const screenshotTools = [
        {
            type: "function",
            function: {
                name: "screenshot",
                description: "截取当前屏幕画面。默认优先截取当前焦点应用窗口，无法识别焦点窗口时自动降级为全屏截图。支持指定显示器、全屏、绝对坐标区域，以及窗口内精准子区域（偏移量+区域大小）。截取的图片会自动缩放处理并展示给用户。",
                parameters: {
                    type: "object",
                    properties: {
                        mode: {
                            type: "string",
                            enum: ["auto", "window", "fullscreen", "display", "region"],
                            description: "截图模式：auto=焦点窗口优先（默认，失败降级全屏）；window=强制焦点窗口；fullscreen=全屏；display=指定显示器；region=绝对坐标区域"
                        },
                        display_index: {
                            type: "number",
                            description: "显示器索引（mode=display 时生效，-1 表示全部）"
                        },
                        offset_x: {
                            type: "number",
                            description: "窗口相对 X 偏移（mode=auto/window，配合 width/height 使用，缺省为 0）"
                        },
                        offset_y: {
                            type: "number",
                            description: "窗口相对 Y 偏移（mode=auto/window，配合 width/height 使用，缺省为 0）"
                        },
                        width: {
                            type: "number",
                            description: "窗口相对区域宽度（>0 且 height>0 时启用精准区域覆盖）"
                        },
                        height: {
                            type: "number",
                            description: "窗口相对区域高度"
                        },
                        region_x: {
                            type: "number",
                            description: "绝对屏幕区域 X 坐标（mode=region）"
                        },
                        region_y: {
                            type: "number",
                            description: "绝对屏幕区域 Y 坐标（mode=region）"
                        },
                        region_w: {
                            type: "number",
                            description: "绝对屏幕区域宽度（mode=region）"
                        },
                        region_h: {
                            type: "number",
                            description: "绝对屏幕区域高度（mode=region）"
                        },
                        format: {
                            type: "string",
                            description: "输出图片格式：'png' 或 'jpg'，默认为 'png'",
                            enum: ["png", "jpg"]
                        }
                    },
                    required: []
                }
            }
        }
    ];
    async function handleScreenshot(args) {
        console.log(`========== 开始处理截图工具调用 ==========`);
        console.log(`原始参数: ${JSON.stringify(args)}`);
        const parsed = typeof args === 'string' ? JSON.parse(args) : (args || {});
        console.log(`参数解析完成: ${JSON.stringify(parsed)}`);
        console.log(`准备执行截图操作...`);
        const [results, captureErr] = screenshotCapture(parsed);
        if (captureErr) {
            console.error(`截图失败: ${captureErr.message || String(captureErr)}`);
            console.log(`========== 截图工具调用结束(失败) ==========`);
            return [`截图失败：${captureErr.message || String(captureErr)}`, ''];
        }
        if (!results || results.length === 0) {
            console.error(`截图失败: 未获取到截图数据`);
            console.log(`========== 截图工具调用结束(失败) ==========`);
            return ['截图失败：未获取到截图数据', ''];
        }
        const firstFrame = results[0];
        console.log(`截图处理成功: ${firstFrame.width}x${firstFrame.height}, 格式=${firstFrame.format}, 帧数=${results.length}`);
        const base64List = results.map((r) => r.base64);
        pushImage(base64List);
        console.log(`图片已推送: ${firstFrame.width}x${firstFrame.height}, 格式=${firstFrame.format}, 帧数=${results.length}`);
        const sizeInfo = `${firstFrame.width}x${firstFrame.height}`;
        const frameInfo = results.length > 1 ? `（共${results.length}帧）` : '';
        const textResponse = `截图完成，已获取当前屏幕画面（${sizeInfo}）${frameInfo}，图片已展示给用户。`;
        console.log(`返回响应: ${sizeInfo}`);
        console.log(`========== 截图工具调用结束(成功) ==========`);
        return [textResponse, firstFrame.base64];
    }
    GlobalConfig.LTPfunction.set('screenshot', handleScreenshot);

    class ModelBuilder {
        roleTag = "月华";
        enableTools = true;
        reasoningStrength = "low";
        messages = [];
        ragMessages = [];
        runtimeMessages = [];
        systemPrompt = "你的名字叫做月华, 是一个女孩子";
        promptCompletion(prompt) {
            let addressText = "";
            if (GlobalConfig.currentAddress.length === 0) {
                const addressResult = address();
                GlobalConfig.currentAddress = addressResult[0];
                addressText = GlobalConfig.currentAddress.join(' ');
            }
            else
                addressText = GlobalConfig.currentAddress.join(' ');
            return prompt
                .replace(/{current-address}/g, addressText);
        }
        writeContext(context) {
            const cleaned = this.stripReasoningContent(context);
            if (this.messages.length >= 48) {
                const discarded = this.messages.slice(0, this.messages.length - 47);
                this.messages = this.messages.slice(-47).concat(cleaned);
                GlobalConfig.unreadRecords.push(...discarded);
            }
            else
                this.messages.push(cleaned);
            return this;
        }
        stripReasoningContent(message) {
            if ('reasoning_content' in message) {
                const { reasoning_content, ...rest } = message;
                return rest;
            }
            return message;
        }
        coverContext(context) {
            this.messages = Array.isArray(context) ? context : [context];
            return this;
        }
        appendPrompt(content) {
            const block = '\n' + content.replace(/^\n+|\n+$/g, '');
            if (this.systemPrompt.includes(block))
                return this;
            this.systemPrompt = this.systemPrompt.replace(/\s*$/, '') + block;
            return this;
        }
        removePrompt(content) {
            const block = '\n' + content.replace(/^\n+|\n+$/g, '');
            this.systemPrompt = this.systemPrompt.replace(block, '');
            return this;
        }
        run(appendContext, toolCall) {
            const rawMessages = [
                { role: 'system', content: this.systemPrompt },
                ...this.runtimeMessages,
                ...appendContext,
                ...this.messages
            ];
            const requestBody = {
                model: GlobalConfig.MultimodalName,
                messages: rawMessages,
                stream: true,
                tools: toolCall,
                tool_choice: 'auto',
                chat_template_kwargs: {
                    reasoning_strength: this.reasoningStrength
                }
            };
            if (!this.enableTools || toolCall.length === 0) {
                delete requestBody.tool_choice;
                delete requestBody.tools;
            }
            const headers = {
                Authorization: `Bearer ${encodeURIComponent(GlobalConfig.MultimodalKey)}`,
                "Content-Type": "application/json",
                "X-Lunar-Role": this.roleTag,
            };
            const modelRequest = {
                method: "POST",
                crossDomain: true,
                headers,
                body: JSON.stringify(requestBody)
            };
            const endpoint = "/chat/completions";
            const [result, error] = syncFetch({ url: GlobalConfig.MultimodalUrl + endpoint, execute: modelRequest });
            if (error)
                throw error;
            if (result?.body?.error) {
                const errMsg = typeof result.body.error === 'string'
                    ? result.body.error
                    : result.body.error.message || JSON.stringify(result.body.error);
                throw new Error(`模型服务错误 [${result.status}]: ${errMsg}`);
            }
            if (!result?.body?.choices) {
                throw new Error(`模型响应异常: status=${result?.status}, body=${JSON.stringify(result?.body)?.substring(0, 200)}`);
            }
            return result;
        }
        constructor(prompt) {
            this.systemPrompt = this.promptCompletion(prompt);
        }
    }

    function loadKnowledge(table) {
        const [entries, error] = knowledgeLoad(table);
        const map = new Map();
        if (error || !Array.isArray(entries)) {
            if (error)
                console.error(`[知识库] 读取表 ${table} 失败:`, error);
            return map;
        }
        for (const pair of entries) {
            if (Array.isArray(pair) && pair.length >= 1 && typeof pair[0] === 'string') {
                map.set(pair[0], typeof pair[1] === 'string' ? pair[1] : String(pair[1] ?? ''));
            }
        }
        return map;
    }
    function saveKnowledge(table, map) {
        const [ok, error] = knowledgeSave(table, [...map.entries()]);
        if (error)
            console.error(`[知识库] 写入表 ${table} 失败:`, error);
        return !error && !!ok;
    }
    function getPromptFromKnowledge(key) {
        try {
            return loadKnowledge('video_summary').get(key) ?? null;
        }
        catch (error) {
            console.error('[知识库] 读取视频摘要失败:', error);
            return null;
        }
    }
    function savePromptToKnowledge(key, prompt) {
        try {
            return saveKnowledge('video_summary', loadKnowledge('video_summary').set(key, prompt));
        }
        catch (error) {
            console.error('[知识库] 存储视频摘要失败:', error);
            return false;
        }
    }
    function getFileIndexFromKnowledge(key) {
        try {
            return loadKnowledge('file_mapping').get(key) ?? null;
        }
        catch (error) {
            console.error('[知识库] 读取文件映射失败:', error);
            return null;
        }
    }
    function saveFileIndexToKnowledge(key, value) {
        try {
            return saveKnowledge('file_mapping', loadKnowledge('file_mapping').set(key, value));
        }
        catch (error) {
            console.error('[知识库] 写入文件映射失败:', error);
            return false;
        }
    }

    let mediaRoles = null;
    function registerMediaRoles(roles) {
        mediaRoles = roles;
    }
    const VIDEO_FALLBACK_TEXT = '月华看不了这个视频呢';
    const AUDIO_FALLBACK_TEXT = '月华听不懂这段语音呢';
    function useFrameSequence() {
        return GlobalConfig.VideoInputMode === 'frames';
    }
    async function understandVideo(videoUrl) {
        const cachedPrompt = getPromptFromKnowledge(videoUrl);
        if (cachedPrompt) {
            console.log('[感知者] 命中视频缓存，直接返回');
            return cachedPrompt;
        }
        if (useFrameSequence()) {
            console.log('[感知者] 开始将视频抽取为图片序列帧...');
            const [groups, framesError] = videoFrames(videoUrl);
            if (!groups || groups.length === 0 || framesError) {
                console.error('[感知者] 视频抽帧失败:', framesError);
                throw new Error('视频抽帧失败');
            }
            const frameCount = groups.reduce((sum, group) => sum + group.frames.length, 0);
            console.log(`[感知者] 视频抽帧完成，共 ${groups.length} 组 / ${frameCount} 帧`);
            return understandMediaSegments(() => mediaRoles.perceiverRole.watchFrames(groups), videoUrl);
        }
        console.log('[感知者] 开始将视频写入媒体目录...');
        const [segments, mediaError] = videoMedia(videoUrl);
        if (!segments || segments.length === 0 || mediaError) {
            console.error('[感知者] 视频媒体化失败:', mediaError);
            throw new Error('视频媒体化失败');
        }
        const mediaUrls = segments.map(segment => `file://${segment.file}`);
        console.log(`[感知者] 视频媒体化完成，共 ${mediaUrls.length} 个片段`);
        return understandMediaSegments(() => mediaRoles.perceiverRole.watchVideo(mediaUrls), videoUrl);
    }
    function admitMedia(source, name, origin, kindHint) {
        if (!source)
            return null;
        const lower = source.toLowerCase();
        const sourceKind = lower.startsWith('data:') ? 'data' : (/^https?:\/\//i.test(source) ? 'url' : 'path');
        const result = admitResource({ sourceKind, source, name, origin, kindHint });
        return result ? { handle: result.handle, id: result.id } : null;
    }
    function storeMediaSummary(media, summary) {
        if (!media || !summary || !summary.trim())
            return;
        setResourceSummary(media.id, summary.trim().slice(0, 1000));
    }
    function existingSummary(media) {
        const entry = getResourceEntry(media.handle);
        return entry?.summary || '';
    }
    async function resourceVideo(originalSource, name, origin, admitSource) {
        const media = admitMedia(originalSource, name, origin, 'V');
        if (!media)
            return '';
        const cached = existingSummary(media);
        if (cached)
            return `${media.handle}【视频理解】${cached}`;
        try {
            const summary = await understandVideo(originalSource);
            if (!summary || !summary.trim())
                return `${media.handle}`;
            storeMediaSummary(media, summary);
            return `${media.handle}【视频理解】${summary}`;
        }
        catch (error) {
            console.error('[感知者] 视频理解失败（句柄已保留）:', error);
            return `${media.handle}${VIDEO_FALLBACK_TEXT}`;
        }
    }
    async function resourceAudio(originalSource, name, origin) {
        const media = admitMedia(originalSource, name, origin, 'A');
        if (!media)
            return '';
        const cached = existingSummary(media);
        if (cached)
            return `${media.handle}${cached}`;
        try {
            const transcript = await understandAudio(originalSource);
            storeMediaSummary(media, transcript);
            return `${media.handle}${transcript}`;
        }
        catch (error) {
            console.error('[感知者] 音频理解失败（句柄已保留）:', error);
            return `${media.handle}${AUDIO_FALLBACK_TEXT}`;
        }
    }
    function mediaName(source, fallbackExt) {
        const lower = (source || '').toLowerCase();
        const dataMatch = /^data:(image|video|audio)\/(\w+)[;,]/.exec(lower);
        if (dataMatch)
            return `media.${dataMatch[2] === 'jpeg' ? 'jpg' : dataMatch[2]}`;
        const path = lower.split('?')[0].split('#')[0];
        const base = path.slice(path.lastIndexOf('/') + 1);
        if (base && /\.[a-z0-9]{2,5}$/.test(base))
            return base;
        return `media${fallbackExt}`;
    }
    async function understandMediaSegments(watch, cacheKey) {
        console.log('[感知者] 开始观看...');
        const videoSummary = await watch();
        console.log('[感知者] 观看完成');
        if (videoSummary && videoSummary.trim().length > 0 && cacheKey) {
            savePromptToKnowledge(cacheKey, videoSummary);
            console.log('[感知者] 理解文本已缓存');
        }
        return videoSummary;
    }
    async function understandAnimatedImage(imageSource, cacheKey) {
        const cachedPrompt = cacheKey ? getPromptFromKnowledge(cacheKey) : '';
        if (cachedPrompt) {
            console.log('[感知者] 命中动态图缓存，直接返回');
            return cachedPrompt;
        }
        console.log('[感知者] 检测到动态图，开始编码为视频...');
        if (useFrameSequence()) {
            const [groups, framesError] = animatedImageToFrames(imageSource);
            if (!groups || groups.length === 0 || framesError) {
                console.error('[感知者] 动态图抽帧失败:', framesError);
                throw new Error('动态图抽帧失败');
            }
            const frameCount = groups.reduce((sum, group) => sum + group.frames.length, 0);
            console.log(`[感知者] 动态图抽帧完成，共 ${groups.length} 组 / ${frameCount} 帧`);
            return understandMediaSegments(() => mediaRoles.perceiverRole.watchFrames(groups), cacheKey);
        }
        const [segments, mediaError] = animatedImageToVideo(imageSource);
        if (!segments || segments.length === 0 || mediaError) {
            console.error('[感知者] 动态图转视频失败:', mediaError);
            throw new Error('动态图转视频失败');
        }
        const mediaUrls = segments.map(segment => `file://${segment.file}`);
        console.log(`[感知者] 动态图转视频完成，共 ${mediaUrls.length} 个片段`);
        return understandMediaSegments(() => mediaRoles.perceiverRole.watchVideo(mediaUrls), cacheKey);
    }
    async function LiteImageFile() {
        for (let message of GlobalConfig.unreadContext) {
            if (typeof message.content === 'string')
                continue;
            const newContent = [];
            for (let item of message.content) {
                if (item.type == 'text') {
                    newContent.push(item);
                    continue;
                }
                if (item.type === 'video_url') {
                    newContent.push({ type: 'text', text: await resourceVideoOrFallback(item.video_url.url) });
                    continue;
                }
                if (item.type === 'audio_url') {
                    newContent.push({ type: 'text', text: await resourceAudioOrFallback(item.audio_url.url) });
                    continue;
                }
                if ('input_audio' in item) {
                    const format = item.input_audio.format || 'wav';
                    newContent.push({ type: 'text', text: await resourceAudioOrFallback(`data:audio/${format};base64,${item.input_audio.data}`) });
                    continue;
                }
                if (!item.image_url)
                    continue;
                const imageSource = item.image_url.url;
                if (imageSource.toLowerCase().startsWith('data:video/') || GlobalConfig.videoFormatsExtensions.some(format => imageSource.toLowerCase().endsWith(format))) {
                    newContent.push({ type: 'text', text: await resourceVideoOrFallback(imageSource) });
                    continue;
                }
                const [isAnimated, animatedError] = isAnimatedImage(imageSource);
                if (animatedError)
                    console.error('[动态图判定失败]:', animatedError.message);
                if (!animatedError && isAnimated) {
                    let handle = handleFromVisionURL(imageSource);
                    if (!handle) {
                        const admitted = admitImageDataURI(imageSource, `image${guessImageExt(imageSource)}`);
                        handle = admitted?.handle || '';
                    }
                    newContent.push({ type: 'text', text: await replaceAnimatedImage(imageSource, handle) });
                    continue;
                }
                newContent.push(item);
            }
            message.content = newContent;
        }
    }
    async function replaceAnimatedImage(imageSource, handle) {
        const media = resolveAdmittedMedia(handle);
        let summary = '';
        try {
            summary = await understandAnimatedImage(imageSource, imageSource.startsWith('data:') ? '' : imageSource);
        }
        catch (error) {
            console.error('[感知者] 动态图理解失败，以兜底文本置换:', error);
        }
        if (media && summary.trim().length > 0)
            storeMediaSummary(media, summary);
        const body = summary.trim().length > 0 ? `【动态图理解】${summary}` : mediaRoles.randomDefaultMessage();
        return media ? `${media.handle}${body}` : body;
    }
    function resolveAdmittedMedia(handle) {
        const entry = getResourceEntry(handle);
        return entry ? { handle: entry.handle, id: entry.id } : null;
    }
    async function understandVideoOrFallback(videoUrl) {
        try {
            const summary = await understandVideo(videoUrl);
            if (summary.trim().length > 0)
                return summary;
            return mediaRoles.randomDefaultMessage();
        }
        catch (error) {
            console.error('[感知者] 视频理解失败，以兜底文本置换:', error);
            return VIDEO_FALLBACK_TEXT;
        }
    }
    async function resourceVideoOrFallback(videoSource) {
        try {
            const text = await resourceVideo(videoSource, mediaName(videoSource, '.mp4'), videoSource.startsWith('data:') ? 'client-upload' : 'channel');
            if (text)
                return text;
        }
        catch (error) {
            console.error('[感知者] 视频资源化失败，退化为纯理解文本:', error);
        }
        return understandVideoOrFallback(videoSource);
    }
    async function resourceAudioOrFallback(audioSource) {
        try {
            const text = await resourceAudio(audioSource, mediaName(audioSource, '.wav'), audioSource.startsWith('data:') ? 'client-upload' : 'channel');
            if (text)
                return text;
        }
        catch (error) {
            console.error('[感知者] 音频资源化失败，退化为纯转写文本:', error);
        }
        return understandAudioOrFallback(audioSource);
    }
    async function understandAudioOrFallback(audioSource) {
        try {
            return await understandAudio(audioSource);
        }
        catch (error) {
            console.error('[感知者] 音频理解失败，以兜底文本置换:', error);
            return AUDIO_FALLBACK_TEXT;
        }
    }
    async function understandAudio(audioSource) {
        const cacheable = !audioSource.startsWith('data:');
        const cachedPrompt = cacheable ? getPromptFromKnowledge(audioSource) : '';
        if (cachedPrompt) {
            console.log('[感知者] 命中音频缓存，直接返回');
            return cachedPrompt;
        }
        console.log('[感知者] 开始转换音频...');
        const [wavBase64, audioError] = audioWav(audioSource);
        if (!wavBase64 || audioError) {
            console.error('[感知者] 音频转换失败:', audioError);
            throw new Error('音频转换失败');
        }
        console.log('[感知者] 开始语音转写...');
        const transcript = await transcribeAudio(wavBase64);
        if (!transcript.trim()) {
            console.error('[感知者] 语音转写为空');
            throw new Error('语音转写为空');
        }
        console.log(`[感知者] 语音转写完成: ${transcript.substring(0, 50)}`);
        const text = `【语音转写】${transcript}`;
        if (cacheable) {
            savePromptToKnowledge(audioSource, text);
            console.log('[感知者] 语音转写文本已缓存');
        }
        return text;
    }
    async function transcribeAudio(wavBase64) {
        const [result, error] = syncFetch({
            url: GlobalConfig.MultimodalUrl + '/chat/completions',
            execute: {
                method: 'POST',
                crossDomain: true,
                headers: {
                    Authorization: `Bearer ${encodeURIComponent(GlobalConfig.MultimodalKey)}`,
                    'Content-Type': 'application/json',
                },
                body: JSON.stringify({
                    model: GlobalConfig.AsrName,
                    messages: [{
                            role: 'user',
                            content: [{ type: 'input_audio', input_audio: { data: wavBase64, format: 'wav' } }]
                        }],
                    temperature: 0,
                }),
            },
        });
        if (error) {
            console.error('[感知者] 语音转写请求失败:', error.message);
            return '';
        }
        const raw = result?.body?.choices?.[0]?.message?.content || '';
        const marker = '<asr_text>';
        return raw.includes(marker) ? raw.substring(raw.indexOf(marker) + marker.length).trim() : raw.trim();
    }

    const IMAGE_KEEP_CHARS = 0;
    const DUMP_MAX_CHARS = 32 * 1024 * 1024;
    let roleRegistry = {};
    function registerDumpRoles(entries) {
        roleRegistry = entries;
    }
    function dumpTargets() {
        return ['lunar', ...Object.keys(roleRegistry)];
    }
    function hasDumpTarget(agent) {
        return agent === 'lunar' || !!roleRegistry[agent];
    }
    function shrinkContent(content, includeImages) {
        if (typeof content === 'string')
            return content;
        if (!Array.isArray(content))
            return content;
        return content.map(item => {
            if (!item || typeof item !== 'object')
                return item;
            const anyItem = item;
            if (anyItem.type !== 'image_url' || !anyItem.image_url)
                return item;
            const url = anyItem.image_url.url || '';
            if (includeImages || url.length <= IMAGE_KEEP_CHARS)
                return item;
            const head = url.slice(0, 64);
            return {
                type: 'image_url',
                image_url: { url: `${head}…（base64 已省略 ${url.length} 字符；需要原图请用 includeImages）` },
            };
        });
    }
    function exportMessages(messages, includeImages) {
        return messages.map(message => ({
            role: message.role,
            content: shrinkContent(message.content, includeImages),
            tool_call_id: message.tool_call_id || undefined,
            tool_calls: message.tool_calls || undefined,
        }));
    }
    function snapshotAgent(agent, includeImages) {
        if (agent === 'lunar') {
            return {
                target: 'lunar',
                label: '月华主上下文（未读队列 + 长期记忆摘要）',
                messages: exportMessages(GlobalConfig.unreadContext, includeImages),
            };
        }
        const entry = roleRegistry[agent];
        return {
            target: agent,
            label: entry.label,
            messages: exportMessages(entry.role.messages, includeImages),
            systemPrompt: entry.role.systemPrompt,
        };
    }
    function buildContextDump(argsJSON) {
        let args = {};
        try {
            args = argsJSON ? JSON.parse(argsJSON) : {};
        }
        catch (e) {
            return JSON.stringify({ error: '导出参数解析失败: ' + String(e) });
        }
        const agent = String(args.agent || '').trim() || 'lunar';
        const includeImages = args.includeImages === true;
        if (!hasDumpTarget(agent)) {
            return JSON.stringify({
                error: `未知导出目标: ${agent}`,
                available: dumpTargets(),
            });
        }
        const snap = snapshotAgent(agent, includeImages);
        const overview = { lunar: GlobalConfig.unreadContext.length };
        for (const [name, entry] of Object.entries(roleRegistry)) {
            overview[name] = entry.role.messages.length;
        }
        const payload = {
            kind: 'lunar-agent-context',
            generated_at: new Date().toISOString(),
            target: snap.target,
            label: snap.label,
            include_images: includeImages,
            message_count: snap.messages.length,
            context_overview: overview,
            system_prompt: snap.systemPrompt,
            messages: snap.messages,
        };
        let text = JSON.stringify(payload, null, 2);
        if (text.length > DUMP_MAX_CHARS) {
            text = text.slice(0, DUMP_MAX_CHARS) + `\n…（导出超过 ${DUMP_MAX_CHARS} 字符，已截断）`;
        }
        return text;
    }
    function ensureDumpBridge() {
        return typeof dumpForJS === 'function';
    }
    function bindDumpBridge() {
        if (!ensureDumpBridge()) {
            console.warn('[数据导出] 宿主未注册 dumpForJS，TUI 数据导出不可用');
            return;
        }
        globalThis.buildContextDump = buildContextDump;
        console.log('[数据导出] 上下文导出入口已注册（目标：' + dumpTargets().join('、') + '）');
    }

    function RandomFloor(min, max) {
        return Math.floor(Math.random() * (max - min + 1) + min);
    }
    function RandomFloat(min, max, length = 2) {
        return Number((Math.random() * (max - min) + min).toFixed(length));
    }

    class CreativeRoleBase extends ModelBuilder {
        OWN_HISTORY_LIMIT = 5;
        MAX_ITERATIONS = 3;
        constructor(prompt) {
            super(prompt);
        }
        buildTaskContent(taskDescription) {
            return taskDescription;
        }
        consumeHistory() {
            const result = [...this.messages];
            this.messages = [];
            return result;
        }
        writeContext(context) {
            const cleaned = this.stripReasoningContent(context);
            if (this.messages.length >= 40) {
                this.messages = this.messages.slice(-39).concat(cleaned);
            }
            else
                this.messages.push(cleaned);
            return this;
        }
        async createCreativeWork(taskDescription) {
            const taskContent = this.buildTaskContent(taskDescription);
            if (typeof taskContent === 'string')
                this.writeContext({ role: 'user', content: taskContent });
            else
                this.writeContext({ role: 'user', content: taskContent });
            const details = [];
            let rejectionReason = '';
            for (let i = 0; i < this.MAX_ITERATIONS; i++) {
                console.log(`[${this.roleName}] 第 ${i + 1} 轮推理`);
                let response;
                try {
                    response = this.run([], this.getToolDefinitions());
                }
                catch (error) {
                    console.error(`[${this.roleName}] 第 ${i + 1} 轮推理失败:`, error);
                    break;
                }
                const choice = response.body?.choices?.[0];
                if (!choice) {
                    console.log(`[${this.roleName}] 模型返回空结果，结束循环`);
                    break;
                }
                const toolCalls = choice.message?.tool_calls;
                if (!toolCalls || toolCalls.length === 0) {
                    if (choice.message?.content && choice.message.content.trim()) {
                        rejectionReason = choice.message.content;
                    }
                    this.writeContext(choice.message);
                    break;
                }
                this.writeContext(choice.message);
                for (const toolCall of toolCalls) {
                    console.log(`[${this.roleName}] 执行工具: ${toolCall.function.name}`);
                    const result = this.executeTool(toolCall);
                    this.writeContext({ role: 'tool', content: result, tool_call_id: toolCall.id });
                    this.collectDetail(toolCall, details);
                }
            }
            if (details.length === 0) {
                const reason = rejectionReason || '月华认为此次无需进行创作';
                console.log(`[${this.roleName}] 未产出作品，原因: ${reason}`);
                return reason;
            }
            const summary = this.buildSummary(details);
            console.log(`[${this.roleName}] 已完成 ${details.length} 件作品创作`);
            return summary;
        }
    }

    function interactEvent(topic, payload) {
        try {
            const resultJSON = ltpInteractEvent(topic, JSON.stringify(payload ?? {}));
            if (!resultJSON)
                return {};
            const res = JSON.parse(resultJSON);
            if (res && res.returned && res.return !== null && res.return !== undefined) {
                return { return: res.return };
            }
        }
        catch (e) {
            console.error('LTPX 事件交互失败:', e);
        }
        return {};
    }

    const SIDE_BASE = 1024;
    const SIDE_MAX = 2816;
    const SIDE_ALIGN = 32;
    const REF_SIDE_MIN = 512;
    const DEFAULT_PIXEL_BUDGET_MP = 1.6;
    const PIXEL_BUDGET_SLACK = 1.15;
    function pixelBudget() {
        const configured = Number(GlobalConfig.customConfig?.diffusion?.pixel_budget_mp);
        const budgetMp = Number.isFinite(configured) && configured > 0 ? configured : DEFAULT_PIXEL_BUDGET_MP;
        return budgetMp * 1_000_000;
    }
    function alignSide(value) {
        const clamped = Math.min(Math.max(Math.round(value), 256), SIDE_MAX);
        return Math.min(Math.round(clamped / SIDE_ALIGN) * SIDE_ALIGN, SIDE_MAX);
    }
    const RATIO_BASE = {
        '1:1': { ratio: 1, horizontal: true },
        '4:3': { ratio: 4 / 3, horizontal: true },
        '3:4': { ratio: 4 / 3, horizontal: false },
        '16:9': { ratio: 16 / 9, horizontal: true },
        '9:16': { ratio: 16 / 9, horizontal: false },
        '3:2': { ratio: 3 / 2, horizontal: true },
        '2:3': { ratio: 3 / 2, horizontal: false },
    };
    const RATIO_PRESETS = Object.fromEntries(Object.entries(RATIO_BASE).map(([key, item]) => {
        const long = alignSide(SIDE_BASE * item.ratio);
        return [key, item.horizontal ? { w: long, h: SIDE_BASE } : { w: SIDE_BASE, h: long }];
    }));
    const DEFAULT_RATIO = '1:1';
    const LIST_LIMIT_MAX = 100;
    const DEFAULT_STRENGTH = 0.5;
    const STEPS_MIN = 30;
    const STEPS_MAX = 60;
    function planDimensions(reference) {
        if (!reference || reference.width <= 0 || reference.height <= 0) {
            const preset = RATIO_PRESETS[DEFAULT_RATIO];
            return { width: preset.w, height: preset.h, origin: `${DEFAULT_RATIO} 预设` };
        }
        const longRef = Math.max(reference.width, reference.height);
        const shortRef = Math.min(reference.width, reference.height);
        const ratio = longRef / shortRef;
        const desired = Math.min(SIDE_BASE, Math.sqrt((pixelBudget() * PIXEL_BUDGET_SLACK) / ratio), SIDE_MAX / ratio);
        const lowerBound = Math.min(REF_SIDE_MIN, desired);
        const upperBound = Math.max(lowerBound, desired);
        const budgetLimit = pixelBudget() * PIXEL_BUDGET_SLACK * 1.02;
        let best = null;
        for (let s = alignSide(lowerBound); s <= upperBound + SIDE_ALIGN; s += SIDE_ALIGN) {
            if (s > SIDE_BASE)
                break;
            const l = alignSide(s * ratio);
            if (l > SIDE_MAX)
                continue;
            const overBudget = s * l > budgetLimit;
            const err = Math.abs(l / s - ratio) / ratio;
            let better = false;
            if (!best)
                better = true;
            else if (best.overBudget !== overBudget)
                better = !overBudget;
            else if (!overBudget && Math.abs(err - best.err) <= Math.max(best.err, err) * 0.01 && s * l > best.short * best.long)
                better = true;
            else if (err < best.err - 1e-9)
                better = true;
            if (better)
                best = { short: s, long: l, err, overBudget };
        }
        if (!best) {
            const l = alignSide(Math.min(SIDE_MAX, Math.max(REF_SIDE_MIN, SIDE_BASE) * ratio));
            best = { short: alignSide(Math.max(REF_SIDE_MIN, l / ratio)), long: l, err: 1, overBudget: false };
        }
        const vertical = reference.height >= reference.width;
        const capped = best.err > 0.08;
        return {
            width: vertical ? best.short : best.long,
            height: vertical ? best.long : best.short,
            origin: `参照图 ${reference.width}x${reference.height} 比例${capped ? '（受长边/像素上限限制，比例已尽力贴近）' : ''}`,
        };
    }
    function resolveDimensionPlan(ratio, reference) {
        const key = String(ratio || '').trim().replace(/[×xX]/g, ':').replace(/\s/g, '');
        const preset = RATIO_PRESETS[key];
        if (preset)
            return { width: preset.w, height: preset.h, origin: `${key} 预设` };
        return planDimensions(reference);
    }
    function normalizeStrength(value, label) {
        const num = Number(value);
        if (Number.isFinite(num)) {
            const clamped = Math.min(Math.max(num, 0.05), 0.95);
            if (Math.abs(clamped - num) > 1e-6)
                console.log(`[绘制者] ${label}强度 ${num} 越界，已收进 ${clamped}`);
            return clamped;
        }
        console.log(`[绘制者] ${label}未指定强度，按中性值 ${DEFAULT_STRENGTH} 处理`);
        return DEFAULT_STRENGTH;
    }
    function normalizeSteps(value, label) {
        const num = Number(value);
        if (!Number.isFinite(num) || num <= 0) {
            console.log(`[绘制者] ${label}未指定步数，按配置 sd_steps 处理`);
            return 0;
        }
        const steps = Math.round(Math.min(Math.max(num, STEPS_MIN), STEPS_MAX));
        if (steps !== Math.round(num))
            console.log(`[绘制者] ${label}步数 ${num} 越界，已收进 ${steps}`);
        return steps;
    }
    class PainterRole extends CreativeRoleBase {
        roleTag = "绘制者";
        defaultExpressionPrompt = [
            '温柔的表情,开心的笑容,脸颊泛红',
            '害羞的表情,抿嘴微笑,眼神躲闪',
            '俏皮的表情,单眼眨眼,嘴角微微上扬,略带调皮的笑容',
            '平静的表情,眼神略微向下看向一侧,嘴唇轻抿,无笑容,若有所思',
            '惊讶的表情,双眼睁大,眉毛抬高,嘴巴微张成O形,脸颊泛红',
            '非常开心的表情,双眼弯成月牙形,张大嘴巴欢笑,脸颊泛红明显',
            '害羞的表情,眼神向下看,眉毛呈八字形,抿嘴微笑,脸颊大面积泛红',
            '自信的表情,眼神直视前方,眉毛微微上扬,嘴角带有一抹浅笑,眼神明亮'
        ];
        defaultPosturePrompt = [
            '一条腿轻轻抬起,俏皮姿势',
            '双手背在身后,身体微微前倾,双脚并拢',
            '一手插在外套口袋里,另一只手轻抬至脸颊旁比"V"字手势,身体略侧,双脚前后交叉站立',
            '双手自然垂放于身前,手指轻轻交握,双肩微微内收,双脚并拢,站姿端正',
            '手抬起至嘴前,十指轻轻触碰"做捂嘴状",身体微微后仰,一条腿向后小半步,重心落在后脚',
            '双手举过头顶比心或张开五指,一条腿向后踢起,身体微微前倾,脚尖离地呈跳跃瞬间',
            '双手食指在胸前互点,头部微微低下,双膝内扣,两脚脚尖向内呈内八站姿',
            '双手叉腰,挺胸收腹,一条腿向侧方伸出,脚尖点地,身体笔直有力',
        ];
        appearanceBasePrompt = fileView('prompts/appearanceBase.md')[0];
        chibiAppearancePrompt = fileView('prompts/selfAppearanceChibi.md')[0];
        fullAppearancePrompt = fileView('prompts/selfAppearanceFull.md')[0];
        groupAppearancePrompt = fileView('prompts/selfAppearanceGroup.md')[0];
        crystalAppearancePrompt = fileView('prompts/crystalAppearance.md')[0];
        defaultOutfitChibiPrompt = '穿着纯黑色洛丽塔风连衣裙，长泡泡袖，袖口缀黑色蕾丝荷叶边，胸前系黑色蕾丝蝴蝶结且蝴蝶结中央镶嵌红色宝石，裙摆层叠黑色蕾丝花边，无任何白色蕾丝或白色装饰';
        defaultOutfitFullPrompt = '穿着黑色哥特萝莉风长裙，黑色蕾丝立领，胸前系黑色蕾丝蝴蝶结且蝴蝶结中央镶嵌红色宝石饰扣，长泡泡袖收黑色蕾丝袖口，多层黑色蕾丝裙摆，无任何白色蕾丝或白色装饰';
        defaultNegativePrompt = '低分辨率, 糙噪点, 超现实主义, 丑陋的面部特征, 失真表情, 模糊轮廓, 颜色失衡, 不均匀光影, 强烈对比度, 过曝或欠曝, 杂乱背景, 像素化, 彩虹效果, 畸形肢体, 错位比例, 低质感纹理';
        referenceImage = '';
        referenceHandle = '';
        referenceSize = null;
        lastResultPath = '';
        roleTool = [
            {
                type: "function",
                function: {
                    name: "diffusion_generation",
                    description: "根据文本描述生成与月华本人无关的图像，纯文生图。仅适用于风景、物品、其他角色、场景等通用创作;严禁用于绘制月华自己的形象（绘制月华必须调用 self_portrait 或 chibi_self_portrait）。若任务消息附带参考图（图生图任务），请改用 image_to_image",
                    parameters: {
                        type: "object",
                        properties: {
                            "prompt": {
                                type: "string",
                                description: "图像生成的正向描述文本"
                            },
                            "ratio": {
                                type: "string",
                                enum: ["1:1", "4:3", "3:4", "16:9", "9:16", "3:2", "2:3"],
                                description: "画面比例预设,默认 1:1。按题材选择:人物/物品用 2:3 或 3:4,风景/横构图用 3:2 或 16:9,方形构图用 1:1。短边固定 1024px"
                            },
                            "steps": {
                                type: "number",
                                description: "迭代步数,取值 30~60:快速草图 30,常规 40（省略时用系统默认）,精细打磨 50~60。越高细节越好但耗时线性增加,不影响构图"
                            },
                            "negative_prompt": {
                                type: "string",
                                description: "负面提示文本,用于排除图像中不希望出现的元素"
                            },
                            "cfg_scale": {
                                type: "number",
                                description: "提示词权重调节参数,取值范围为 0 到 2,默认值为 1.0"
                            },
                            "super_resolution": {
                                type: "boolean",
                                description: "生成完成后追加 RealESRGAN 超分辨率重建（4x）,提升清晰度与细节。用户要求高清、精细画质时开启,耗时增加"
                            }
                        },
                        required: [
                            "prompt"
                        ]
                    }
                }
            },
            {
                type: "function",
                function: {
                    name: "image_to_image",
                    description: "基于本次任务附带的参考图进行图生图创作:以参考图为底图,按文本描述改变风格、场景、服饰或细节,同时保留参考图的主体特征。输出尺寸自动按参考图比例推导(短边不低于 1024px),无需也无法另行指定比例。任务消息未附带参考图时不可调用,请改用 diffusion_generation 进行文生图",
                    parameters: {
                        type: "object",
                        properties: {
                            "prompt": {
                                type: "string",
                                description: "图像生成的正向描述文本,描述希望参考图变成什么样子"
                            },
                            "strength": {
                                type: "number",
                                description: "图生图强度,取值范围 0 到 1,越大越偏离参考图。必须由你根据创作意图指定:只改画风/光照/质感用 0.2~0.4;换服饰/换配色/微调细节用 0.4~0.6;换背景/换地点/大幅改场景 用 0.8~0.9 推荐 0.85;替换画面里的一个对象(如把北极熊换成熊猫)同样用 0.8~0.9——实测 0.65/0.75 换不掉大物体。注意:强度≥0.8 时提示词主导画面,没写进提示词的东西会消失,因此提示词必须描述整个画面(含所有要保留的主体)。缺省按中性值 0.5"
                            },
                            "steps": {
                                type: "number",
                                description: "迭代步数,取值 30~60:快速草图 30,常规 40（省略时用系统默认）,精细打磨 50~60。越高细节越好但耗时线性增加,不影响构图"
                            },
                            "negative_prompt": {
                                type: "string",
                                description: "负面提示文本,用于排除图像中不希望出现的元素（换场景时点名去掉旧场景元素,如 beach, ocean, water, sea）"
                            },
                            "cfg_scale": {
                                type: "number",
                                description: "提示词引导系数(0~2),默认 1.0。图生图建议 2.0~3.0:起点是参考图加噪后的潜变量、天然偏向原构图,cfg 太低时提示词拉不动画面。系统对图生图设有 1.5 下限与 2.0 缺省"
                            },
                            "super_resolution": {
                                type: "boolean",
                                description: "生成完成后追加 RealESRGAN 超分辨率重建（4x）,提升清晰度与细节。用户要求高清、精细画质时开启,耗时增加"
                            }
                        },
                        required: [
                            "prompt"
                        ]
                    }
                }
            },
            {
                type: "function",
                function: {
                    name: "self_portrait",
                    description: "生成月华的全尺寸自画像（全身立绘,标准少女头身比）。凡是要求绘制月华、你自己、\"我\"的形象,且未特别要求Q版时,都必须且只能调用此函数,禁止调用 diffusion_generation",
                    parameters: {
                        type: "object",
                        properties: {
                            "expression": {
                                type: "string",
                                description: "表情提示词,描述想要展现的表情"
                            },
                            "posture": {
                                type: "string",
                                description: "动作提示词,描述想要展现的姿势或动作"
                            },
                            "outfit": {
                                type: "string",
                                description: "服装提示词,描述想要穿着的服装样式。不提供则使用默认服装"
                            },
                            "environment": {
                                type: "string",
                                description: "环境提示词,描述背景环境或场景"
                            },
                            "ratio": {
                                type: "string",
                                enum: ["1:1", "4:3", "3:4", "16:9", "9:16", "3:2", "2:3"],
                                description: "画面比例预设,默认 1:1。全身立绘/坐姿建议 2:3,横构图场景建议 3:2 或 16:9。短边固定 1024px"
                            },
                            "steps": {
                                type: "number",
                                description: "迭代步数,取值 30~60:快速草图 30,常规 40（省略时用系统默认）,精细打磨 50~60。越高细节越好但耗时线性增加,不影响构图"
                            },
                            "negative_prompt": {
                                type: "string",
                                description: "负面提示文本,用于排除图像中不希望出现的元素"
                            },
                            "cfg_scale": {
                                type: "number",
                                description: "提示词权重调节参数,取值范围为 0 到 2,默认值为 1.0"
                            },
                            "super_resolution": {
                                type: "boolean",
                                description: "生成完成后追加 RealESRGAN 超分辨率重建（4x）,提升清晰度与细节。用户要求高清、精细画质时开启,耗时增加"
                            }
                        },
                        required: [
                            "expression",
                            "posture",
                            "environment"
                        ]
                    }
                }
            },
            {
                type: "function",
                function: {
                    name: "chibi_self_portrait",
                    description: "生成月华的Q版自画像（二头身可爱比例）。当要求绘制Q版、二头身、萌系或大头娃娃风格的月华形象时调用此函数,禁止调用 diffusion_generation",
                    parameters: {
                        type: "object",
                        properties: {
                            "expression": {
                                type: "string",
                                description: "表情提示词,描述想要展现的表情"
                            },
                            "posture": {
                                type: "string",
                                description: "动作提示词,描述想要展现的姿势或动作"
                            },
                            "outfit": {
                                type: "string",
                                description: "服装提示词,描述想要穿着的服装样式。不提供则使用默认服装"
                            },
                            "environment": {
                                type: "string",
                                description: "环境提示词,描述背景环境或场景"
                            },
                            "ratio": {
                                type: "string",
                                enum: ["1:1", "4:3", "3:4", "16:9", "9:16", "3:2", "2:3"],
                                description: "画面比例预设,默认 1:1（Q版二头身适合方形或略宽）。短边固定 1024px"
                            },
                            "steps": {
                                type: "number",
                                description: "迭代步数,取值 30~60:快速草图 30,常规 40（省略时用系统默认）,精细打磨 50~60。越高细节越好但耗时线性增加,不影响构图"
                            },
                            "negative_prompt": {
                                type: "string",
                                description: "负面提示文本,用于排除图像中不希望出现的元素"
                            },
                            "cfg_scale": {
                                type: "number",
                                description: "提示词权重调节参数,取值范围为 0 到 2,默认值为 1.0"
                            },
                            "super_resolution": {
                                type: "boolean",
                                description: "生成完成后追加 RealESRGAN 超分辨率重建（4x）,提升清晰度与细节。用户要求高清、精细画质时开启,耗时增加"
                            }
                        },
                        required: [
                            "expression",
                            "posture",
                            "environment"
                        ]
                    }
                }
            },
            {
                type: "function",
                function: {
                    name: "group_photo",
                    description: "生成月华与其他人的合照。当要求绘制月华和某人（朋友、家人、其他角色等）的合影时调用此函数。合照对象为琉璃时,companion 直接填写\"琉璃\"即可,系统会自动注入琉璃的固定形象。若本次任务附带参考图,将以该参考图为底图进行图生图,尽量保留合照对象的容貌;未附带则按文本描述绘制",
                    parameters: {
                        type: "object",
                        properties: {
                            "companion": {
                                type: "string",
                                description: "合照对象描述,描述对方的性别、外貌、发型、服装、身份等特征。合照对象为琉璃时直接填写\"琉璃\""
                            },
                            "expression": {
                                type: "string",
                                description: "月华的表情提示词,描述想要展现的表情"
                            },
                            "posture": {
                                type: "string",
                                description: "月华的动作提示词,描述想要展现的姿势或动作"
                            },
                            "outfit": {
                                type: "string",
                                description: "月华的服装提示词,描述想要穿着的服装样式。不提供则使用默认服装"
                            },
                            "environment": {
                                type: "string",
                                description: "环境提示词,描述背景环境或场景"
                            },
                            "ratio": {
                                type: "string",
                                enum: ["1:1", "4:3", "3:4", "16:9", "9:16", "3:2", "2:3"],
                                description: "画面比例预设,默认 1:1。仅在未选定参考图时生效;已选定参考图时输出尺寸自动随参考图比例"
                            },
                            "strength": {
                                type: "number",
                                description: "已选定参考图时的图生图强度,取值 0 到 1,越大越偏离参考图。保留对方容貌建议 0.3~0.5;缺省按中性值 0.5。未选定参考图时忽略"
                            },
                            "steps": {
                                type: "number",
                                description: "迭代步数,取值 30~60:快速草图 30,常规 40（省略时用系统默认）,精细打磨 50~60。越高细节越好但耗时线性增加,不影响构图"
                            },
                            "negative_prompt": {
                                type: "string",
                                description: "负面提示文本,用于排除图像中不希望出现的元素"
                            },
                            "cfg_scale": {
                                type: "number",
                                description: "提示词权重调节参数,取值范围为 0 到 2,默认值为 1.0"
                            },
                            "super_resolution": {
                                type: "boolean",
                                description: "生成完成后追加 RealESRGAN 超分辨率重建（4x）,提升清晰度与细节。用户要求高清、精细画质时开启,耗时增加"
                            }
                        },
                        required: [
                            "companion",
                            "expression",
                            "posture",
                            "environment"
                        ]
                    }
                }
            },
            {
                type: "function",
                function: {
                    name: "list_images",
                    description: "查看资源目录中最近的图片句柄与内容描述（按登记时间倒序，越靠前越新）。清单只含文字信息，可用于了解历史图片、确认句柄或为超分定位目标。需要图生图基座时由对话者经 dispatch_painter 的 reference_tag 指定，任务消息会直接附上图片本体",
                    parameters: {
                        type: "object",
                        properties: {}
                    }
                }
            },
            {
                type: "function",
                function: {
                    name: "upscale_image",
                    description: "对已有图片执行 RealESRGAN 超分辨率放大（4x）,提升清晰度与细节。用户要求把某张图片变清晰、放大或提升画质时调用;不指定句柄时对最近一次生成的作品超分。与生成工具的 super_resolution 参数不同,本工具不重新生成图像,只做纯超分",
                    parameters: {
                        type: "object",
                        properties: {
                            "image_tag": {
                                type: "string",
                                description: "图片句柄（如 @I.003），对目录中指定图片超分。不填时对最近一次生成的作品超分"
                            }
                        }
                    }
                }
            }
        ];
        constructor() {
            super(fileView('prompts/painterRole.md')[0]);
        }
        async createCreativeWork(taskDescription, reference) {
            this.referenceImage = '';
            this.referenceHandle = '';
            this.referenceSize = null;
            if (reference?.source) {
                this.referenceHandle = reference.handle || '';
                this.referenceImage = reference.source;
                this.referenceSize = this.loadReferenceSize(reference.source);
                console.log(`[绘制者] 本次任务参考图: ${this.referenceHandle || '(无句柄)'} → ${reference.source}${this.referenceSize ? `（${this.referenceSize.width}x${this.referenceSize.height}）` : ''}`);
            }
            return super.createCreativeWork(taskDescription);
        }
        buildTaskContent(taskDescription) {
            if (!this.referenceImage)
                return taskDescription;
            const imageURI = this.loadReferenceDataURI();
            if (!imageURI) {
                console.log('[绘制者] 参考图本体读取失败，退回纯文本任务描述');
                return taskDescription;
            }
            console.log(`[绘制者] 参考图已附入任务上下文（${Math.round(imageURI.length / 1024)}KB）`);
            return [
                { type: 'text', text: taskDescription },
                { type: 'image_url', image_url: { url: imageURI } }
            ];
        }
        loadReferenceDataURI() {
            if (!this.referenceHandle)
                return '';
            try {
                const result = readResource(this.referenceHandle, 8 * 1024 * 1024);
                if (!result || result.lost || result.truncated || !result.base64)
                    return '';
                return `data:${result.mime || 'image/png'};base64,${result.base64}`;
            }
            catch (error) {
                console.error('[绘制者] 读取参考图失败:', error);
                return '';
            }
        }
        loadReferenceSize(relPath) {
            try {
                const [resized, err] = resizeImage(relPath);
                if (err || !resized || resized.length === 0)
                    return null;
                const size = { width: resized[0].width, height: resized[0].height };
                return size.width > 0 && size.height > 0 ? size : null;
            }
            catch (error) {
                console.error('[绘制者] 参考图尺寸读取失败:', error);
                return null;
            }
        }
        planFor(ratio) {
            const reference = this.referenceImage ? (this.referenceSize || undefined) : undefined;
            return resolveDimensionPlan(ratio, reference);
        }
        referenceStatus() {
            if (this.referenceImage) {
                const size = this.referenceSize ? `（${this.referenceSize.width}x${this.referenceSize.height}）` : '';
                const plan = this.planFor();
                return `本次任务参考图：${this.referenceHandle || this.referenceImage}${size}，输出尺寸将按参考图比例取 ${plan.width}x${plan.height}`;
            }
            return '本次任务尚未附带参考图';
        }
        resetReference() {
            this.referenceImage = '';
            this.referenceHandle = '';
            this.referenceSize = null;
        }
        get roleName() { return '绘制者'; }
        getToolDefinitions() { return this.roleTool; }
        executeTool(toolCall) {
            const funcName = toolCall.function.name;
            let args = {};
            try {
                args = typeof toolCall.function.arguments === 'string' ? JSON.parse(toolCall.function.arguments) : toolCall.function.arguments;
            }
            catch (parseError) {
                console.error(`[绘制者] 工具调用参数解析失败:`, toolCall.function.arguments);
                return `工具调用参数解析失败，请确保传入合法的 JSON 字符串。错误: ${parseError}`;
            }
            switch (funcName) {
                case 'diffusion_generation': return this.handleDiffusionGeneration(args);
                case 'image_to_image': return this.handleImageToImage(args);
                case 'self_portrait': return this.handleSelfPortrait(args);
                case 'chibi_self_portrait': return this.handleChibiSelfPortrait(args);
                case 'group_photo': return this.handleGroupPhoto(args);
                case 'list_images': return this.handleListImages();
                case 'upscale_image': return this.handleUpscaleImage(args);
                default: return `未知工具: ${funcName}，可用工具为 diffusion_generation、image_to_image、self_portrait、chibi_self_portrait、group_photo、list_images、upscale_image`;
            }
        }
        collectDetail(toolCall, paintings) {
            try {
                const args = typeof toolCall.function.arguments === 'string'
                    ? JSON.parse(toolCall.function.arguments)
                    : toolCall.function.arguments;
                const name = toolCall.function.name;
                if (name === 'self_portrait' || name === 'chibi_self_portrait') {
                    paintings.push({
                        toolName: name,
                        promptSummary: name === 'self_portrait' ? '全尺寸自画像' : 'Q版自画像',
                        expression: args.expression || '',
                        posture: args.posture || '',
                        environment: args.environment || '',
                    });
                }
                else if (name === 'group_photo') {
                    paintings.push({
                        toolName: name,
                        promptSummary: `与${args.companion || '他人'}的合照`,
                        expression: args.expression || '',
                        posture: args.posture || '',
                        environment: args.environment || '',
                        companion: args.companion || '',
                    });
                }
                else if (name === 'diffusion_generation' || name === 'image_to_image') {
                    const prompt = args.prompt || '';
                    const strengthNote = name === 'image_to_image' && args.strength !== undefined
                        ? `（强度 ${normalizeStrength(args.strength, '图生图')}）`
                        : '';
                    paintings.push({
                        toolName: name,
                        promptSummary: (prompt.length > 100 ? prompt.slice(0, 97) + '...' : prompt) + strengthNote,
                    });
                }
                else if (name === 'upscale_image') {
                    const resolved = args.image_tag ? resolveImageRef(String(args.image_tag)) : null;
                    paintings.push({
                        toolName: name,
                        promptSummary: resolved ? `${resolved.handle} 超分重建` : '最近作品超分重建',
                    });
                }
            }
            catch {
            }
        }
        buildSummary(paintings) {
            if (paintings.length === 0)
                return '月华没有绘制任何作品';
            const feedback = interactEvent('draw_painting_before', { paintings }).return;
            if (feedback && Array.isArray(feedback) && feedback.every(p => p.toolName && p.promptSummary)) {
                paintings = feedback;
            }
            const parts = [];
            for (let i = 0; i < paintings.length; i++) {
                const p = paintings[i];
                if (p.toolName === 'self_portrait' || p.toolName === 'chibi_self_portrait') {
                    let desc = p.toolName === 'self_portrait' ? '月华绘制了一幅全尺寸自画像' : '月华绘制了一幅Q版自画像';
                    if (p.expression)
                        desc += `，展现了${p.expression}`;
                    if (p.environment)
                        desc += `，背景是${p.environment}`;
                    parts.push(desc + '。');
                }
                else if (p.toolName === 'group_photo') {
                    let desc = `月华绘制了一幅与${p.companion || '他人'}的合照`;
                    if (p.expression)
                        desc += `，展现了${p.expression}`;
                    if (p.environment)
                        desc += `，背景是${p.environment}`;
                    parts.push(desc + '。');
                }
                else if (p.toolName === 'image_to_image')
                    parts.push(`月华参照参考图绘制了一幅图像：${p.promptSummary}。`);
                else if (p.toolName === 'upscale_image')
                    parts.push(`月华为${p.promptSummary}提升了清晰度，图像已变得更精细。`);
                else
                    parts.push(`月华绘制了一幅图像：${p.promptSummary}。`);
            }
            parts.push('图像已通过前端推送给用户。');
            return parts.join('\n');
        }
        renderAppearance(template, parts, defaultOutfit) {
            const expression = parts.expression || this.defaultExpressionPrompt[RandomFloor(0, this.defaultExpressionPrompt.length - 1)];
            const posture = parts.posture || this.defaultPosturePrompt[RandomFloor(0, this.defaultPosturePrompt.length - 1)];
            const outfit = parts.outfit || defaultOutfit;
            return template
                .replace('{base}', this.appearanceBasePrompt)
                .replace('{expression}', expression)
                .replace('{outfit}', outfit)
                .replace('{posture}', posture)
                .replace('{companion}', parts.companion || '')
                .replace('{environment}', parts.environment || '');
        }
        paint(imageParams, label) {
            const [result, error] = generateImage(imageParams);
            if (error) {
                console.error(`[绘制者] ${label}失败:`, error);
                return `${label}失败: ${error}`;
            }
            if (!result || !result.base64)
                return `${label}失败：引擎返回空结果`;
            this.lastResultPath = result.path;
            this.recordPainting(label, result.base64);
            const stepsNote = imageParams.steps ? `，${imageParams.steps} 步` : '';
            console.log(`[绘制者] ${label}成功，尺寸: ${result.width}x${result.height}${stepsNote}${imageParams.superResolution ? '（含超分重建）' : ''}`);
            if (!pushImage([result.base64]))
                console.warn(`[绘制者] 推送${label}结果到前端失败`);
            return `${label}成功。图片尺寸: ${result.width}x${result.height}，seed: ${result.seed}${stepsNote}${imageParams.superResolution ? '，已附带超分辨率重建' : ''}`;
        }
        recordPainting(label, base64) {
            if (!base64)
                return;
            try {
                const uri = base64.startsWith('data:') ? base64 : `data:image/png;base64,${base64}`;
                this.writeContext({
                    role: 'user',
                    content: [
                        { type: 'text', text: `【${label}】这是你刚完成的作品` },
                        { type: 'image_url', image_url: { url: uri } }
                    ]
                });
            }
            catch (error) {
                console.error('[绘制者] 画作写入历史失败:', error);
            }
        }
        handleDiffusionGeneration(args) {
            try {
                const prompt = args.prompt || '';
                if (!prompt.trim())
                    return '扩散生成失败：正向提示词不能为空';
                const plan = resolveDimensionPlan(args.ratio);
                const steps = normalizeSteps(args.steps, '扩散生成');
                console.log(`[绘制者] 扩散生成 - 正向提示词: ${prompt.slice(0, 100)}...，画面比例: ${plan.origin} → ${plan.width}x${plan.height}，步数: ${steps || '默认'}`);
                return this.paint({
                    prompt: prompt,
                    negativePrompt: args.negative_prompt || '',
                    width: plan.width,
                    height: plan.height,
                    steps: steps,
                    cfgScale: args.cfg_scale ?? 1.0,
                    superResolution: args.super_resolution === true,
                }, '扩散图像生成');
            }
            catch (error) {
                console.error('[绘制者] 扩散生成处理异常:', error);
                return `扩散图像生成异常: ${error}`;
            }
        }
        resolveImg2ImgCfg(value) {
            const CFG_FLOOR = 1.5;
            if (typeof value === 'number' && Number.isFinite(value) && value > 0) {
                return Math.max(value, CFG_FLOOR);
            }
            return 2.0;
        }
        handleImageToImage(args) {
            try {
                const prompt = args.prompt || '';
                if (!prompt.trim())
                    return '图生图失败：正向提示词不能为空';
                if (!this.referenceImage) {
                    return `图生图失败：${this.referenceStatus()}。图生图基座由对话者经 dispatch_painter 的 reference_tag 指定；当前任务无参考图时请改用 diffusion_generation 进行文生图`;
                }
                const strength = normalizeStrength(args.strength, '图生图');
                const steps = normalizeSteps(args.steps, '图生图');
                const cfgScale = this.resolveImg2ImgCfg(args.cfg_scale);
                const plan = this.planFor();
                console.log(`[绘制者] 图生图 - 正向提示词: ${prompt.slice(0, 100)}...，参考图: ${this.referenceImage}，强度: ${strength}，cfg: ${cfgScale}，尺寸: ${plan.origin} → ${plan.width}x${plan.height}，步数: ${steps || '默认'}`);
                return this.paint({
                    prompt: prompt,
                    negativePrompt: args.negative_prompt || '',
                    width: plan.width,
                    height: plan.height,
                    steps: steps,
                    cfgScale: cfgScale,
                    initImg: this.referenceImage,
                    strength: strength,
                    superResolution: args.super_resolution === true,
                }, '图生图生成');
            }
            catch (error) {
                console.error('[绘制者] 图生图处理异常:', error);
                return `图生图生成异常: ${error}`;
            }
        }
        handleSelfPortrait(args) {
            return this.handlePortrait(args, this.fullAppearancePrompt, this.defaultOutfitFullPrompt, '自画像');
        }
        handleChibiSelfPortrait(args) {
            return this.handlePortrait(args, this.chibiAppearancePrompt, this.defaultOutfitChibiPrompt, 'Q版自画像');
        }
        handlePortrait(args, template, defaultOutfit, label) {
            try {
                console.log(`[绘制者] -> ${label}生成`);
                console.log(`表情: "${args.expression}"`);
                console.log(`姿势: "${args.posture}"`);
                console.log(`服装: "${args.outfit}"`);
                console.log(`环境: "${args.environment}"`);
                console.log(`负面提示词: "${args.negative_prompt}"`);
                console.log(`提示词引导系数: "${args.cfg_scale}"`);
                const plan = resolveDimensionPlan(args.ratio);
                const steps = normalizeSteps(args.steps, label);
                const fullPrompt = this.renderAppearance(template, args, defaultOutfit);
                return this.paint({
                    prompt: fullPrompt,
                    negativePrompt: args.negative_prompt || this.defaultNegativePrompt,
                    width: plan.width,
                    height: plan.height,
                    steps: steps,
                    cfgScale: args.cfg_scale ?? 1.0,
                    superResolution: args.super_resolution === true,
                }, `${label}生成`);
            }
            catch (error) {
                console.error(`[绘制者] ${label}处理异常:`, error);
                return `${label}生成异常: ${error}`;
            }
        }
        handleGroupPhoto(args) {
            try {
                console.log(`[绘制者] -> 合照生成`);
                console.log(`合照对象: "${args.companion}"`);
                console.log(`表情: "${args.expression}"`);
                console.log(`姿势: "${args.posture}"`);
                console.log(`服装: "${args.outfit}"`);
                console.log(`环境: "${args.environment}"`);
                let companion = args.companion || '';
                if (companion.includes('琉璃'))
                    companion = this.crystalAppearancePrompt;
                const fullPrompt = this.renderAppearance(this.groupAppearancePrompt, { ...args, companion }, this.defaultOutfitFullPrompt);
                const useReference = !!this.referenceImage;
                const plan = useReference ? this.planFor() : resolveDimensionPlan(args.ratio);
                const steps = normalizeSteps(args.steps, '合照');
                const imageParams = {
                    prompt: fullPrompt,
                    negativePrompt: args.negative_prompt || this.defaultNegativePrompt,
                    width: plan.width,
                    height: plan.height,
                    steps: steps,
                    cfgScale: args.cfg_scale ?? 1.0,
                    superResolution: args.super_resolution === true,
                };
                if (useReference) {
                    imageParams.initImg = this.referenceImage;
                    imageParams.strength = normalizeStrength(args.strength, '合照');
                    console.log(`[绘制者] 合照图生图模式，参考图: ${this.referenceImage}，强度: ${imageParams.strength}，尺寸: ${plan.origin} → ${plan.width}x${plan.height}，步数: ${steps || '默认'}`);
                }
                return this.paint(imageParams, '合照生成');
            }
            catch (error) {
                console.error('[绘制者] 合照生成处理异常:', error);
                return `合照生成异常: ${error}`;
            }
        }
        handleListImages() {
            const entries = listResources('I', LIST_LIMIT_MAX);
            if (entries.length === 0)
                return '资源目录中还没有任何图片：当前没有可引用的图片句柄';
            const lines = entries.map(entry => {
                const desc = entry.summary ? sanitizeImageDesc(entry.summary, 40) : '（暂无描述）';
                const lost = entry.state === 'active' ? '' : ' [本体已回收]';
                return `${entry.handle} ${desc}${lost}`;
            });
            return `资源目录中最近的 ${entries.length} 张图片（句柄越新越靠前）：\n${lines.join('\n')}\n可调用 upscale_image 对指定图片超分；图生图基座由对话者经 reference_tag 指定。${this.referenceStatus()}。`;
        }
        handleUpscaleImage(args) {
            try {
                const resolved = args.image_tag ? resolveImageRef(String(args.image_tag)) : null;
                let source = '';
                let handleLabel = '';
                if (args.image_tag) {
                    if (!resolved)
                        return '超分失败：句柄不存在或已失效，请先调用 list_images 或 list_resources 查看可用图片';
                    if (!resolved.source)
                        return `超分失败：${resolved.handle} 的本体已从内容仓库回收，无法超分`;
                    source = resolved.source;
                    handleLabel = resolved.handle;
                }
                else {
                    if (!this.lastResultPath)
                        return '超分失败：还没有生成过作品，请通过 image_tag 指定图片句柄，或先完成一次图像生成';
                    source = this.lastResultPath;
                    handleLabel = '最近作品';
                }
                console.log(`[绘制者] 超分重建 - 来源: ${handleLabel} ${source.substring(0, 60)}...`);
                const [result, error] = upscaleImage(source);
                if (error) {
                    console.error('[绘制者] 超分失败:', error);
                    return `超分失败: ${error}`;
                }
                if (!result || !result.base64)
                    return '超分失败：引擎返回空结果';
                this.recordPainting('超分重建', result.base64);
                if (!pushImage([result.base64]))
                    console.warn('[绘制者] 推送超分结果到前端失败');
                return `超分成功。图片尺寸: ${result.width}x${result.height}，已推送给用户`;
            }
            catch (error) {
                console.error('[绘制者] 超分处理异常:', error);
                return `超分处理异常: ${error}`;
            }
        }
    }

    class MusicianRole extends CreativeRoleBase {
        roleTag = "演奏者";
        MAX_ITERATIONS = 4;
        MIN_MELODY_BARS = 12;
        DEFAULT_TEMPO = 100;
        draft = null;
        assembledDetail = null;
        musicTool = [
            {
                type: "function",
                function: {
                    name: "submit_melody",
                    description: "创作流程第一步：提交完整的主旋律声部。先规划风格、调式、节拍与段落结构，再写出贯穿全曲的主旋律。主旋律是全曲的根基，伴奏与和弦将在其基础上依次生成",
                    parameters: {
                        type: "object",
                        properties: {
                            "title": {
                                type: "string",
                                description: "音乐作品标题，应具有诗意与美感，与音乐风格匹配"
                            },
                            "instrument": {
                                type: "string",
                                description: "主旋律乐器，单个乐器名，如：钢琴(piano)、小提琴(violin)、长笛(flute)、萨克斯(sax)、竖琴(harp)、大提琴(cello)、8bit"
                            },
                            "tempo": {
                                type: "number",
                                description: "演奏速度（BPM）。抒情曲建议60-80，轻快曲建议90-120，激昂曲建议130-150。默认100"
                            },
                            "key": {
                                type: "string",
                                description: "调式，如 C、G、D、F、a、e、d"
                            },
                            "meter": {
                                type: "string",
                                description: "拍号，如 4/4、3/4、6/8。默认 4/4"
                            },
                            "structure": {
                                type: "string",
                                description: "段落结构，如：'前奏(4小节)-A段主旋律(8小节)-B段展开(8小节)-A'再现(8小节)-尾声(4小节)'"
                            },
                            "melody": {
                                type: "string",
                                description: "主旋律ABC片段：仅一行音符，用 | 分隔小节，以 |] 结尾。禁止包含 X:/T:/M:/L:/Q:/K: 等头部字段与 [V:N] 声部标记。应有起伏与乐句呼吸感（每4-8小节一个乐句）"
                            }
                        },
                        required: [
                            "title",
                            "melody"
                        ]
                    }
                }
            },
            {
                type: "function",
                function: {
                    name: "submit_accompaniment",
                    description: "创作流程第二步：在已提交的主旋律基础上生成伴奏声部。使用柱式和弦、分解和弦或阿尔贝蒂低音为旋律提供和声织体，小节数必须与主旋律完全一致",
                    parameters: {
                        type: "object",
                        properties: {
                            "instrument": {
                                type: "string",
                                description: "伴奏乐器，单个乐器名，如：钢琴(piano)、竖琴(harp)、吉他(guitar)、贝斯(bass)、氛围铺底(弦乐群)"
                            },
                            "accompaniment": {
                                type: "string",
                                description: "伴奏ABC片段：仅一行音符，用 | 分隔小节，小节数必须与主旋律完全一致，以 |] 结尾。禁止包含头部字段与 [V:N] 声部标记。和弦用方括号包裹同时发音的音符，如 [C,,E,,G,,]4 或分解和弦 C,,2 E,2 G,2 c2"
                            }
                        },
                        required: [
                            "accompaniment"
                        ]
                    }
                }
            },
            {
                type: "function",
                function: {
                    name: "submit_chords",
                    description: "创作流程第三步（最后一步）：在主旋律与伴奏基础上生成和弦进行声部，提供低音支撑与和声骨架，完成整部作品。小节数必须与主旋律完全一致",
                    parameters: {
                        type: "object",
                        properties: {
                            "instrument": {
                                type: "string",
                                description: "和弦声部乐器，单个乐器名，如：贝斯(bass)、大提琴(cello)、钢琴(piano)、鼓组(打击乐)"
                            },
                            "chords": {
                                type: "string",
                                description: "和弦进行ABC片段：仅一行音符，用 | 分隔小节，小节数必须与主旋律完全一致，以 |] 结尾。禁止包含头部字段与 [V:N] 声部标记。使用根音、柱式和弦或分解和弦，明确体现和声进行（如 C-F-G-C）"
                            }
                        },
                        required: [
                            "chords"
                        ]
                    }
                }
            }
        ];
        constructor() {
            super(fileView('prompts/musicianRole.md')[0]);
        }
        get roleName() { return '演奏者'; }
        getToolDefinitions() { return this.musicTool; }
        executeTool(toolCall) {
            const funcName = toolCall.function.name;
            let args = {};
            try {
                args = typeof toolCall.function.arguments === 'string'
                    ? JSON.parse(toolCall.function.arguments)
                    : toolCall.function.arguments;
            }
            catch (parseError) {
                console.error(`[演奏者] 工具调用参数解析失败:`, toolCall.function.arguments);
                return `工具调用参数解析失败，请确保传入合法的 JSON 字符串。错误: ${parseError}`;
            }
            if (this.draft?.chords)
                return '作品已创作完成，无需重复提交任何声部';
            switch (funcName) {
                case 'submit_melody': return this.handleSubmitMelody(args);
                case 'submit_accompaniment': return this.handleSubmitAccompaniment(args);
                case 'submit_chords': return this.handleSubmitChords(args);
                default: return `未知工具: ${funcName}，请按流程依次调用 submit_melody → submit_accompaniment → submit_chords`;
            }
        }
        collectDetail(toolCall, pieces) {
            if (toolCall.function.name !== 'submit_chords' || !this.assembledDetail)
                return;
            pieces.push(this.assembledDetail);
            this.assembledDetail = null;
        }
        buildSummary(pieces) {
            if (pieces.length === 0)
                return '月华没有演奏任何作品';
            const feedback = interactEvent('play_music_before', { pieces }).return;
            if (feedback && Array.isArray(feedback) && feedback.every(p => p.title && p.instruments && p.tempo && p.structure && p.key && p.meter && p.abcLength !== undefined)) {
                pieces = feedback;
            }
            const parts = [];
            for (let i = 0; i < pieces.length; i++) {
                const p = pieces[i];
                let desc = `月华演奏了《${p.title}》`;
                const details = [];
                if (p.instruments)
                    details.push(`使用${p.instruments}`);
                if (p.key)
                    details.push(`${p.key}${p.key === p.key.toLowerCase() ? '小调' : '大调'}`);
                if (p.tempo > 0)
                    details.push(`${p.tempo}BPM`);
                if (p.structure)
                    details.push(`结构为${p.structure}`);
                if (details.length > 0)
                    desc += `（${details.join('，')}）`;
                parts.push(desc + '。');
            }
            parts.push('乐谱已通过音乐播放器推送给用户，可以查看和播放。');
            return parts.join('\n');
        }
        sanitizeFragment(fragment) {
            return fragment
                .replace(/^[A-Za-z]:.*$/gm, '')
                .replace(/\[[Vv]:\d+\]/g, '')
                .replace(/\s+/g, ' ')
                .trim();
        }
        countBars(fragment) {
            const body = fragment
                .replace(/^[A-Za-z]:.*$/gm, '')
                .replace(/\[[Vv]:\d+\]/g, '')
                .replace(/![^!|]*!/g, '')
                .replace(/\|\]|:\||\|\|/g, '|');
            return body.split('|').filter(segment => /[A-Ga-gz]/.test(segment)).length;
        }
        handleSubmitMelody(args) {
            try {
                const title = (args.title || '').trim();
                const melody = this.sanitizeFragment(args.melody || '');
                if (!title)
                    return '主旋律提交失败：作品标题不能为空';
                if (!melody)
                    return '主旋律提交失败：旋律内容不能为空';
                const bars = this.countBars(melody);
                if (bars < this.MIN_MELODY_BARS) {
                    return `主旋律提交失败：当前仅 ${bars} 小节，音乐展开不足。请扩展至至少 ${this.MIN_MELODY_BARS} 小节（建议 16 小节以上），保持完整的段落结构后重新提交`;
                }
                this.draft = {
                    title: title,
                    structure: args.structure || '',
                    key: args.key || 'C',
                    meter: args.meter || '4/4',
                    tempo: args.tempo || this.DEFAULT_TEMPO,
                    melody: melody,
                    melodyInstrument: args.instrument || '钢琴',
                    melodyBars: bars,
                    accompaniment: '',
                    accompanimentInstrument: '',
                    chords: '',
                    chordInstrument: '',
                };
                console.log(`[演奏者] 主旋律已提交: "${title}"，${bars} 小节，${this.draft.key} 调，${this.draft.tempo}BPM`);
                return `主旋律提交成功：共 ${bars} 小节。下一步请在此旋律基础上调用 submit_accompaniment 生成伴奏声部，小节数必须恰好为 ${bars} 小节并逐小节对齐`;
            }
            catch (error) {
                console.error('[演奏者] 主旋律提交处理异常:', error);
                return `主旋律提交异常: ${error}`;
            }
        }
        handleSubmitAccompaniment(args) {
            try {
                if (!this.draft)
                    return '伴奏提交失败：尚未提交主旋律，请先调用 submit_melody';
                const accompaniment = this.sanitizeFragment(args.accompaniment || '');
                if (!accompaniment)
                    return '伴奏提交失败：伴奏内容不能为空';
                const bars = this.countBars(accompaniment);
                if (bars !== this.draft.melodyBars) {
                    return `伴奏提交失败：伴奏为 ${bars} 小节，与主旋律 ${this.draft.melodyBars} 小节不一致。请逐小节对齐主旋律后重新提交完整的伴奏声部`;
                }
                this.draft.accompaniment = accompaniment;
                this.draft.accompanimentInstrument = args.instrument || this.draft.melodyInstrument;
                console.log(`[演奏者] 伴奏已提交: ${bars} 小节，乐器: ${this.draft.accompanimentInstrument}`);
                return `伴奏提交成功：${bars} 小节，与主旋律对齐。最后一步请调用 submit_chords 生成和弦进行声部，小节数必须恰好为 ${bars} 小节`;
            }
            catch (error) {
                console.error('[演奏者] 伴奏提交处理异常:', error);
                return `伴奏提交异常: ${error}`;
            }
        }
        handleSubmitChords(args) {
            try {
                if (!this.draft)
                    return '和弦提交失败：尚未提交主旋律，请先调用 submit_melody';
                if (!this.draft.accompaniment)
                    return '和弦提交失败：尚未提交伴奏声部，请先调用 submit_accompaniment';
                const chords = this.sanitizeFragment(args.chords || '');
                if (!chords)
                    return '和弦提交失败：和弦进行内容不能为空';
                const bars = this.countBars(chords);
                if (bars !== this.draft.melodyBars) {
                    return `和弦提交失败：和弦进行为 ${bars} 小节，与主旋律 ${this.draft.melodyBars} 小节不一致。请逐小节对齐后重新提交完整的和弦声部`;
                }
                this.draft.chords = chords;
                this.draft.chordInstrument = args.instrument || '贝斯';
                const detail = this.assembleAndPush();
                console.log(`[演奏者] 和弦已提交: ${bars} 小节，乐器: ${this.draft.chordInstrument}，作品《${detail.title}》拼装完成`);
                this.assembledDetail = detail;
                this.draft = null;
                return `和弦进行提交成功，作品《${detail.title}》创作完成：主旋律、伴奏与和弦进行三声部已对齐拼装（共 ${detail.abcLength} 小节）。乐谱已推送到前端，可通过音乐播放器查看和播放。`;
            }
            catch (error) {
                console.error('[演奏者] 和弦提交处理异常:', error);
                return `和弦提交异常: ${error}`;
            }
        }
        assembleAndPush() {
            const d = this.draft;
            const instruments = [d.melodyInstrument, d.accompanimentInstrument, d.chordInstrument]
                .map(inst => inst.trim())
                .filter(Boolean)
                .join(',');
            let enrichedAbc = [
                `X:1`,
                `T:${d.title}`,
                `M:${d.meter}`,
                `L:1/8`,
                `Q:1/4=${d.tempo}`,
                `K:${d.key}`,
                `[V:1] ${d.melody}`,
                `[V:2] ${d.accompaniment}`,
                `[V:3] ${d.chords}`,
                ''
            ].join('\n');
            enrichedAbc = this.injectInstrumentDirective(enrichedAbc, instruments);
            const pushSuccess = pushContext('music', enrichedAbc, '');
            if (!pushSuccess) {
                console.warn('[演奏者] 推送乐谱到前端失败');
            }
            return {
                title: d.title,
                instruments: instruments,
                tempo: d.tempo,
                structure: d.structure,
                key: d.key,
                meter: d.meter,
                abcLength: d.melodyBars,
            };
        }
        injectInstrumentDirective(abcNotation, instruments) {
            if (!instruments)
                return abcNotation;
            const list = instruments
                .replace(/，/g, ',')
                .split(',')
                .map(s => s.trim())
                .filter(Boolean);
            if (list.length === 0)
                return abcNotation;
            if (/^%%voice\s+/m.test(abcNotation))
                return abcNotation;
            const directives = [];
            for (let i = 0; i < list.length; i++) {
                const inst = list[i];
                const voiceNum = i + 1;
                directives.push(`%%voice ${voiceNum} ${inst}`);
            }
            const directive = directives.join('\n') + '\n';
            const xMatch = abcNotation.match(/^X:\s*\d+/m);
            if (xMatch && xMatch.index !== undefined) {
                const before = abcNotation.substring(0, xMatch.index);
                const after = abcNotation.substring(xMatch.index);
                return before + directive + after;
            }
            return directive + abcNotation;
        }
    }

    const FALLBACK_TOOL_PARAMETERS = {
        type: 'object',
        properties: {
            tool: { type: 'string', description: '要使用的目标工具名（从工具描述清单中选择）' },
            instruction: { type: 'string', description: '要对目标工具发出的自然语言指令' }
        },
        required: ['tool', 'instruction']
    };
    let crystalToolDefinition = null;
    function getCrystalRemoteTools() {
        return crystalToolDefinition ? [crystalToolDefinition] : [];
    }
    function syncLTPXRemoteStatus() {
        try {
            const statusJSON = getLTPXRemoteStatus();
            const tool = statusJSON ? JSON.parse(statusJSON).tool : undefined;
            if (!tool || !tool.name)
                return;
            if (!GlobalConfig.LTPfunction.has(tool.name)) {
                GlobalConfig.LTPfunction.set(tool.name, async (args) => {
                    const argsJSON = typeof args === 'string' ? args : JSON.stringify(args || {});
                    const text = await callLTPXRemoteTool(tool.name, argsJSON);
                    return [text, ''];
                });
            }
            crystalToolDefinition = {
                type: 'function',
                function: {
                    name: tool.name,
                    description: tool.description || '',
                    parameters: tool.parameters || FALLBACK_TOOL_PARAMETERS
                },
            };
        }
        catch (e) {
            console.error('LTPX 工具链同步失败:', e);
        }
    }

    const READ_CHAR_BUDGET = 12000;
    const OUTLINE_PREVIEW_LIMIT = 60;
    function kindLabel(kind) {
        return KIND_LABELS[kind] ?? kind;
    }
    function stateNote(entry) {
        return entry.state === 'active' ? '' : ' [本体已回收，仅存描述]';
    }
    function entryLine(entry) {
        const desc = entry.summary ? `：${entry.summary}` : '';
        const sectionNote = entry.section_count > 0 ? ` ${entry.section_count}节` : '';
        return `${entry.handle} [${kindLabel(entry.kind)}] ${entry.title || entry.name}${sectionNote}${stateNote(entry)}${desc}`;
    }
    async function handleListResources(args) {
        const parsed = typeof args === 'string' ? safeParse(args) : (args || {});
        const kind = String(parsed.kind || '').trim().toUpperCase();
        const limit = Math.min(Math.max(Number(parsed.limit) || 30, 1), 200);
        if (kind && !KIND_LABELS[kind]) {
            return [`类型码无效：${kind}。可用类型码：D 文档 / I 图片 / V 视频 / A 音频 / T 表格`, ''];
        }
        const entries = listResources(kind, limit);
        if (entries.length === 0) {
            return ['资源目录为空：当前没有已收录的资源。用户可以上传图片、发送文本文件，或让我生成图片后再引用', ''];
        }
        const stats = resourceStatistics();
        const lines = entries.map(entryLine);
        const head = `当前可用资源（共 ${entries.length} 条，按收录时间倒序，越靠前越新${stats ? `；目录总计 ${stats.total} 条 / ${formatBytes(stats.bytes)}` : ''}）：`;
        const tail = `\n${grammarCheatsheet()}。需要正文时用 read_resource 按句柄取回；文档可先用 outline_resource 看结构再定向读取。`;
        return [`${head}\n${lines.join('\n')}${tail}`, ''];
    }
    async function handleReadResource(args) {
        const parsed = typeof args === 'string' ? safeParse(args) : (args || {});
        const rawHandles = Array.isArray(parsed.handles)
            ? parsed.handles
            : (parsed.handle ? [parsed.handle] : []);
        if (rawHandles.length === 0) {
            return ['请提供资源句柄，例如 ["@D.003"]。可用清单见 list_resources', ''];
        }
        const question = String(parsed.question || '').trim().slice(0, 200);
        const blocks = [];
        const missing = [];
        for (const raw of rawHandles) {
            const handle = String(raw || '').trim();
            const parsedHandle = parseHandle(handle);
            if (!parsedHandle) {
                missing.push(handle || '(空)');
                continue;
            }
            const normalized = formatHandle(parsedHandle);
            const entry = getResourceEntry(normalized);
            if (parsedHandle.kind === 'I' || parsedHandle.kind === 'V' || parsedHandle.kind === 'A') {
                if (!entry) {
                    missing.push(normalized);
                    continue;
                }
                const desc = describeResource(normalized, question);
                blocks.push(`【${normalized}｜${kindLabel(parsedHandle.kind)}｜${entry.title || entry.name}】\n${desc || '（暂无描述，且本体不可直接解读）'}`);
                continue;
            }
            const result = readResource(normalized, READ_CHAR_BUDGET);
            if (!result) {
                missing.push(normalized);
                continue;
            }
            if (result.lost) {
                const sections = entry?.section_count ?? 0;
                blocks.push(`【${normalized}｜${result.title}】\n正文已从内容仓库回收（句柄仍然有效）。${entry?.summary ? `目录摘要：${entry.summary}` : ''}${sections > 0 ? `\n可用 outline_resource 查看 ${sections} 节结构` : ''}`);
                continue;
            }
            const rangeNote = result.line_start && result.line_end ? `｜第 ${result.line_start}-${result.line_end} 行` : '';
            const sectionNote = result.section_title ? `｜${result.section_title}` : '';
            const truncated = result.truncated ? '\n（内容较长已截断，可用 outline_resource 查看结构后按节定向读取）' : '';
            blocks.push(`【${normalized}${sectionNote}${rangeNote}｜共 ${result.total_lines || 0} 行】\n${result.content || ''}${truncated}`);
        }
        if (blocks.length === 0) {
            return [`取回失败：${missing.join('、')} 均不存在或句柄非法。请先用 list_resources 确认可用句柄`, ''];
        }
        let text = blocks.join('\n\n---\n\n');
        if (missing.length > 0)
            text += `\n\n以下句柄无法取回：${missing.join('、')}（请用 list_resources 确认）`;
        return [text, ''];
    }
    async function handleOutlineResource(args) {
        const parsed = typeof args === 'string' ? safeParse(args) : (args || {});
        const handle = String(parsed.handle || '').trim();
        const parsedHandle = parseHandle(handle);
        if (!parsedHandle)
            return ['请提供文档句柄，例如 "@D.003"', ''];
        const normalized = formatHandle(parsedHandle);
        const entry = getResourceEntry(normalized);
        if (!entry)
            return [`资源不存在：${normalized}。请用 list_resources 确认可用句柄`, ''];
        if (entry.kind !== 'D' && entry.kind !== 'T') {
            return [`${normalized} 是${kindLabel(entry.kind)}，没有段落大纲；图片/视频/音频请用 read_resource 取描述`, ''];
        }
        const before = listResources(entry.kind, 1000).find(item => item.seq === entry.seq);
        const sections = outlineFor(normalized);
        if (sections.length === 0) {
            return [`${normalized}（${entry.title || entry.name}）没有可用的段落结构，可用 read_resource 直接读取全文`, ''];
        }
        const lines = sections.slice(0, OUTLINE_PREVIEW_LIMIT).map(section => {
            const anchor = sectionHandle('D', entry.seq, section.ordinal);
            const label = section.path || section.title || `第 ${section.ordinal} 段`;
            const level = section.level > 0 ? `${'#'.repeat(section.level)} ` : '';
            return `${anchor} ${level}${label}（第 ${section.line_start}-${section.line_end} 行，${section.chars} 字）`;
        });
        const head = `【${normalized}｜${entry.title || entry.name}｜共 ${before?.section_count ?? sections.length} 节${before?.state === 'active' ? '' : '，正文已回收'}】`;
        const tail = sections.length > OUTLINE_PREVIEW_LIMIT ? `\n（仅显示前 ${OUTLINE_PREVIEW_LIMIT} 节，共 ${sections.length} 节）` : '';
        return [`${head}\n${lines.join('\n')}${tail}\n\n读取某一节：read_resource({"handles":["${sectionHandle('D', entry.seq, sections[0].ordinal)}"]})`, ''];
    }
    async function handleSearchResources(args) {
        const parsed = typeof args === 'string' ? safeParse(args) : (args || {});
        const query = String(parsed.query || '').trim();
        if (!query)
            return ['请提供搜索关键词', ''];
        const kind = String(parsed.kind || '').trim().toUpperCase();
        const limit = Math.min(Math.max(Number(parsed.limit) || 10, 1), 50);
        const hits = searchResources(query, kind, limit);
        if (hits.length === 0) {
            return [`未找到与「${query}」相关的资源。可用 list_resources 查看全部资源，或让用户重新提供资料`, ''];
        }
        const lines = [];
        for (const hit of hits) {
            if (hit.sections && hit.sections.length > 0) {
                const sectionLines = hit.sections.slice(0, 5).map(section => {
                    const parsed = parseHandle(hit.handle);
                    const anchor = parsed ? sectionHandle(parsed.kind, parsed.seq, section.ordinal) : hit.handle;
                    return `    ${anchor} ${section.path || section.title || `第 ${section.ordinal} 段`}（第 ${section.line_start}-${section.line_end} 行）`;
                });
                lines.push(`${hit.handle} [${kindLabel(hit.kind)}] ${hit.title}${hit.summary ? `：${hit.summary}` : ''}\n  命中章节：\n${sectionLines.join('\n')}`);
            }
            else {
                lines.push(`${hit.handle} [${kindLabel(hit.kind)}] ${hit.title}${hit.summary ? `：${hit.summary}` : ''}`);
            }
        }
        return [`与「${query}」相关的资源（共 ${hits.length} 条）：\n${lines.join('\n')}\n\n下一步：用 read_resource 按上述句柄取回正文`, ''];
    }
    function outlineFor(handle) {
        try {
            const [sections, , error] = resourceOutline(handle);
            if (error || !sections)
                return [];
            return sections;
        }
        catch (error) {
            console.error('[资源工具] 读大纲失败:', error);
            return [];
        }
    }
    function safeParse(text) {
        try {
            const parsed = JSON.parse(text);
            return parsed && typeof parsed === 'object' ? parsed : {};
        }
        catch {
            return {};
        }
    }
    function formatBytes(bytes) {
        if (!Number.isFinite(bytes) || bytes <= 0)
            return '0 B';
        const units = ['B', 'KB', 'MB', 'GB'];
        let value = bytes;
        let unit = 0;
        while (value >= 1024 && unit < units.length - 1) {
            value /= 1024;
            unit++;
        }
        return `${value >= 100 || unit === 0 ? Math.round(value) : value.toFixed(1)} ${units[unit]}`;
    }
    const resourceTools = [
        {
            type: 'function',
            function: {
                name: 'list_resources',
                description: `列出当前可用的全部资源（文本文件、图片、视频、音频共用一份目录），返回句柄 + 类型 + 标题/描述 + 内容状态。当你需要确认某个句柄是否还有效、或想知道现在手上有哪些资料时调用。${grammarCheatsheet()}。`,
                parameters: {
                    type: 'object',
                    properties: {
                        kind: {
                            type: 'string',
                            description: '可选：只列出某一类资源，取 D 文档 / I 图片 / V 视频 / A 音频 / T 表格'
                        },
                        limit: {
                            type: 'number',
                            description: '返回条目数量上限，默认 30'
                        }
                    },
                    required: []
                }
            }
        },
        {
            type: 'function',
            function: {
                name: 'read_resource',
                description: '按句柄取回资源内容：文档返回完整段落正文（绝不半句截断），图片/视频/音频返回内容描述（不注入本体，上下文成本极低）。句柄可带锚点定向读取某一节或某几行，例如 @D.003|§4 或 @D.003|L120-168。正文已从内容仓库回收时返回目录摘要与结构提示，句柄依然有效。',
                parameters: {
                    type: 'object',
                    properties: {
                        handles: {
                            type: 'array',
                            items: { type: 'string' },
                            description: '资源句柄数组，如 ["@D.003", "@D.003|§4", "@I.007"]'
                        },
                        question: {
                            type: 'string',
                            description: '可选：用户当前关心的具体问题（图片/视频/音频按此定向生成描述）'
                        }
                    },
                    required: ['handles']
                }
            }
        },
        {
            type: 'function',
            function: {
                name: 'outline_resource',
                description: '取文档的段落结构（目录树），每节返回规范锚点句柄 @D.003|§n 与行区间、字数。不知道文档里有什么、或需要定位到具体章节时先调用本工具，再用 read_resource 按锚点整段读取 —— 这比一次性读全文更省上下文。',
                parameters: {
                    type: 'object',
                    properties: {
                        handle: {
                            type: 'string',
                            description: '文档句柄，如 "@D.003"'
                        }
                    },
                    required: ['handle']
                }
            }
        },
        {
            type: 'function',
            function: {
                name: 'search_resources',
                description: '跨资源搜索：在全部资源的标题、摘要、标签与段落标题中匹配关键词，返回「哪份资源的哪一节」命中了。本工具只做定位，取正文请随后调用 read_resource。适合用户问「我之前给过你的那份配置里…」这类跨资料检索。',
                parameters: {
                    type: 'object',
                    properties: {
                        query: {
                            type: 'string',
                            description: '搜索关键词'
                        },
                        kind: {
                            type: 'string',
                            description: '可选：限定资源类型，取 D / I / V / A / T'
                        },
                        limit: {
                            type: 'number',
                            description: '返回条数上限，默认 10'
                        }
                    },
                    required: ['query']
                }
            }
        }
    ];
    GlobalConfig.LTPfunction.set('list_resources', handleListResources);
    GlobalConfig.LTPfunction.set('read_resource', handleReadResource);
    GlobalConfig.LTPfunction.set('outline_resource', handleOutlineResource);
    GlobalConfig.LTPfunction.set('search_resources', handleSearchResources);

    const dialogueBaseTools = [...agentControlTools, ...imageRecallTools, ...resourceTools, ...scheduleTools, ...screenshotTools];
    const MAX_TOOL_ROUNDS = 5;
    class DialogueRole extends ModelBuilder {
        descriptionRole;
        roleTag = "对话者";
        async generateDialogue(cache) {
            try {
                await LiteImageFile();
                if (this.messages.length === 0)
                    this.toolRounds = 0;
                GlobalConfig.unreadContext.forEach(context => this.writeContext(context));
                GlobalConfig.unreadContext = [];
                this.formatHistoricalMessages();
                this.runtimeMessages = [{ role: 'user', content: `当前时间: ${new Date().toLocaleString()}` }];
                const response = this.run(this.ragMessages, [...dialogueBaseTools, ...getCrystalRemoteTools()]);
                this.analyzeMessageResponse(response.body, cache);
                if (cache.toolCalls.length > 0) {
                    this.writeContext(response.body.choices?.[0]?.message);
                    if (this.toolRounds >= MAX_TOOL_ROUNDS) {
                        console.warn(`[对话者] 工具调用轮次达到上限（${MAX_TOOL_ROUNDS}），停止继续调用`);
                        GlobalConfig.finalResponse = cache.descriptionContent;
                        return;
                    }
                    const hasProcessedToolCalls = await this.batchExecutionToolCall(cache);
                    if (hasProcessedToolCalls) {
                        this.toolRounds++;
                        return await this.generateDialogue(cache);
                    }
                }
                this.writeContext(response.body.choices?.[0]?.message);
            }
            catch (error) {
                console.error('请求处理错误:', error);
            }
            GlobalConfig.finalResponse = cache.descriptionContent;
        }
        IMAGE_WINDOW = IMAGE_WINDOW_SIZE;
        toolRounds = 0;
        formatHistoricalMessages() {
            if (this.messages.length === 0)
                return;
            this.demoteOverflowImages();
            if (this.messages.length === 0)
                return;
            if (this.messages.slice(-1)[0].role === 'user')
                return;
            this.writeContext({ role: 'user', content: "继续" });
        }
        demoteOverflowImages() {
            const countImages = () => this.messages.reduce((sum, message) => typeof message.content === 'string'
                ? sum
                : sum + message.content.filter((c) => c.type === 'image_url').length, 0);
            for (let i = 0; i < this.messages.length && countImages() > this.IMAGE_WINDOW; i++) {
                const message = this.messages[i];
                if (typeof message.content === 'string')
                    continue;
                if (!message.content.some((c) => c.type === 'image_url'))
                    continue;
                this.demoteMessageImages(message);
            }
        }
        demoteMessageImages(message) {
            if (typeof message.content === 'string')
                return;
            const items = message.content;
            const rebuilt = [];
            for (const item of items) {
                const content = item;
                if (content.type !== 'image_url') {
                    rebuilt.push(item);
                    continue;
                }
                const tag = handleFromVisionURL(content.image_url?.url);
                rebuilt.push({ type: 'text', text: tag
                        ? composeImagePlaceholder(tag, this.describeImage(tag, content.image_url?.url || ''))
                        : '（一张未能登记句柄的图片，已超出窗口）' });
            }
            message.content = rebuilt;
        }
        describeImage(tag, source) {
            const entry = getResourceEntry(tag);
            if (entry?.summary)
                return sanitizeImageDesc(entry.summary, 20);
            const generated = this.generateImageDesc(source, tag);
            if (generated && entry)
                setResourceSummary(entry.id, generated);
            return generated;
        }
        generateImageDesc(source, tag) {
            if (!source)
                return '';
            try {
                this.descriptionRole.coverContext({
                    role: 'user',
                    content: [
                        { type: 'text', text: '用一句不超过20字的中文描述这张图片的主体与场景，只输出描述本身。' },
                        { type: 'image_url', image_url: { url: source } }
                    ]
                });
                const response = this.descriptionRole.run([], []);
                return sanitizeImageDesc(response.body?.choices?.[0]?.message?.content || '', 20);
            }
            catch (error) {
                console.error('[对话者] 图片描述生成失败:', error);
                return '';
            }
        }
        analyzeMessageResponse(message, cache) {
            try {
                if (message.choices?.[0]?.message?.reasoning_content) {
                    cache.thinkingContent = message.choices[0].message.reasoning_content;
                }
                if (message.timings?.predicted_per_second) {
                    console.log(`词元生成速度: ${message.timings.predicted_per_second}`);
                }
                if (message.timings?.cache_n !== undefined) {
                    console.log(`缓存命中数量: ${message.timings.cache_n}`);
                }
                if (message.choices?.[0]?.message?.tool_calls) {
                    for (const toolCall of message.choices[0].message.tool_calls) {
                        try {
                            toolCall.function.arguments = JSON.parse(toolCall.function.arguments);
                            cache.toolCalls.push(toolCall);
                        }
                        catch (parseError) {
                            console.error('工具调用参数解析错误:', parseError);
                        }
                    }
                }
                if (message.choices?.[0]?.message?.content) {
                    cache.descriptionContent = message.choices[0].message.content;
                }
            }
            catch (error) {
                console.error('聊天消息响应处理错误:', error);
            }
        }
        async batchExecutionToolCall(state) {
            let hasToolCalls = false;
            for (const toolCall of state.toolCalls) {
                const functionName = toolCall.function.name;
                const functionArgs = toolCall.function.arguments;
                const lunarToolPackage = GlobalConfig.LTPfunction.get(functionName);
                if (!lunarToolPackage) {
                    this.messages.push({ role: "tool", content: `未找到工具包: ${functionName}`, tool_call_id: toolCall.id });
                    continue;
                }
                try {
                    const toolResult = await lunarToolPackage(functionArgs);
                    const textContent = Array.isArray(toolResult) ? toolResult[0] : String(toolResult);
                    const base64Image = Array.isArray(toolResult) ? toolResult[1] : '';
                    this.messages.push({ role: "tool", content: textContent, tool_call_id: toolCall.id });
                    if (base64Image && typeof base64Image === 'string' && base64Image.length > 0) {
                        const message = {
                            role: "user",
                            content: [
                                { type: "text", text: `【工具返回的图片】${textContent}` },
                                { type: "image_url", image_url: { url: base64Image } }
                            ]
                        };
                        attachImageTags(message);
                        this.messages.push(message);
                        console.log(`[工具调用] ${functionName} 返回图片数据，长度=${base64Image.length} 字节`);
                    }
                    hasToolCalls = true;
                }
                catch (error) {
                    this.messages.push({ role: "tool", content: `调用${functionName}失败: ${error}`, tool_call_id: toolCall.id });
                }
            }
            state.currentToolCallIndex = -1;
            state.currentFunctionArgs = "";
            state.currentFunctionName = "";
            state.currentToolCall = null;
            state.toolCalls = [];
            return hasToolCalls;
        }
        ;
        constructor(descriptionRole) {
            super(fileView('prompts/dialogueRole.md')[0]);
            this.descriptionRole = descriptionRole;
        }
    }

    class PerceiverRole extends ModelBuilder {
        roleTag = "感知者";
        SUMMARY_THRESHOLD = 4096;
        summaryTaskTemplate = fileView('prompts/perceiverSummaryTask.md')[0];
        MAX_SEGMENTS = 8;
        RETRY_WAIT_MS = 25000;
        MAX_ATTEMPTS = 2;
        constructor() {
            super(fileView('prompts/perceiverRole.md')[0]);
        }
        async watchVideo(mediaUrls) {
            if (mediaUrls.length === 0) {
                console.warn('[感知者] 未收到任何视频片段');
                return '';
            }
            const watched = this.sampleSegments(mediaUrls);
            if (watched.length < mediaUrls.length) {
                console.log(`[感知者] 片段数 ${mediaUrls.length} 超过单次上限 ${this.MAX_SEGMENTS}，等距抽样观看 ${watched.length} 段`);
            }
            console.log(`[感知者] 开始观看视频，共 ${watched.length} 个片段`);
            const understandings = [];
            for (let index = 0; index < watched.length; index++) {
                console.log(`[感知者] 观看第 ${index + 1}/${watched.length} 段`);
                const understanding = await this.evaluateSegment(watched[index], index + 1, watched.length);
                if (understanding.trim().length > 0) {
                    understandings.push(understanding);
                    console.log(`[感知者] 第 ${index + 1} 段理解完成`);
                }
            }
            return this.finalizeUnderstandings(understandings);
        }
        async watchFrames(groups) {
            if (groups.length === 0) {
                console.warn('[感知者] 未收到任何采样帧分组');
                return '';
            }
            const watched = this.sampleSegments(groups);
            if (watched.length < groups.length) {
                console.log(`[感知者] 分组数 ${groups.length} 超过单次上限 ${this.MAX_SEGMENTS}，等距抽样观看 ${watched.length} 组`);
            }
            console.log(`[感知者] 开始观看视频采样帧，共 ${watched.length} 个分组`);
            const understandings = [];
            for (let index = 0; index < watched.length; index++) {
                console.log(`[感知者] 观看第 ${index + 1}/${watched.length} 组（${watched[index].frames.length} 帧）`);
                const understanding = await this.evaluateFrameGroup(watched[index], index + 1, watched.length);
                if (understanding.trim().length > 0) {
                    understandings.push(understanding);
                    console.log(`[感知者] 第 ${index + 1} 组理解完成`);
                }
            }
            return this.finalizeUnderstandings(understandings);
        }
        sampleSegments(items) {
            if (items.length <= this.MAX_SEGMENTS)
                return items;
            const picked = [];
            for (let i = 0; i < this.MAX_SEGMENTS; i++) {
                picked.push(items[Math.round(i * (items.length - 1) / (this.MAX_SEGMENTS - 1))]);
            }
            return [...new Set(picked)];
        }
        async finalizeUnderstandings(understandings) {
            if (understandings.length === 0) {
                console.warn('[感知者] 未产生任何片段理解');
                return '';
            }
            const concatenated = understandings.join('\n\n');
            if (concatenated.length <= this.SUMMARY_THRESHOLD) {
                console.log(`[感知者] 拼接理解文本 ${concatenated.length} 字符，未超 ${this.SUMMARY_THRESHOLD}，直接返回`);
                return concatenated;
            }
            console.log(`[感知者] 拼接理解文本 ${concatenated.length} 字符，超过 ${this.SUMMARY_THRESHOLD}，触发客观摘要`);
            const summary = await this.generateObjectiveSummary(concatenated);
            if (summary.trim().length > 0) {
                console.log(`[感知者] 客观摘要完成（${summary.length} 字符）`);
                return summary;
            }
            console.warn(`[感知者] 客观摘要失败，退回拼接理解文本（硬切断至 ${this.SUMMARY_THRESHOLD} 字符）`);
            return concatenated.slice(0, this.SUMMARY_THRESHOLD);
        }
        async evaluateSegment(mediaUrl, index, total) {
            const position = total === 1 ? '' : `（第 ${index}/${total} 段）`;
            const prompt = `请观看这段视频${position}，按「片段理解任务」的格式输出该片段的客观内容理解。`;
            this.coverContext({
                role: 'user',
                content: [
                    { type: 'text', text: prompt },
                    { type: 'image_url', image_url: { url: mediaUrl } }
                ]
            });
            this.runtimeMessages = [];
            for (let attempt = 1; attempt <= this.MAX_ATTEMPTS; attempt++) {
                try {
                    const response = this.run([], []);
                    const content = response.body?.choices?.[0]?.message?.content || '';
                    if (!content.trim()) {
                        console.warn(`[感知者] 第 ${index} 段返回空内容`);
                    }
                    return content;
                }
                catch (error) {
                    console.error(`[感知者] 第 ${index} 段第 ${attempt} 次推理失败:`, error);
                    if (attempt < this.MAX_ATTEMPTS) {
                        console.warn(`[感知者] 等待 ${this.RETRY_WAIT_MS / 1000} 秒后重试第 ${index} 段`);
                        await new Promise(resolve => setTimeout(resolve, this.RETRY_WAIT_MS));
                    }
                }
            }
            return '';
        }
        async evaluateFrameGroup(group, index, total) {
            const position = total === 1 ? '' : `（第 ${index}/${total} 段）`;
            const prompt = `以下是同一段视频${position}按时间顺序抽取的采样帧序列，请按「片段理解任务」的格式输出这段视频的客观内容理解。`;
            this.coverContext({
                role: 'user',
                content: [
                    { type: 'text', text: prompt },
                    ...group.frames.map((frame) => ({ type: 'image_url', image_url: { url: frame.image } }))
                ]
            });
            this.runtimeMessages = [];
            for (let attempt = 1; attempt <= this.MAX_ATTEMPTS; attempt++) {
                try {
                    const response = this.run([], []);
                    const content = response.body?.choices?.[0]?.message?.content || '';
                    if (!content.trim()) {
                        console.warn(`[感知者] 第 ${index} 组返回空内容`);
                    }
                    return content;
                }
                catch (error) {
                    console.error(`[感知者] 第 ${index} 组第 ${attempt} 次推理失败:`, error);
                    if (attempt < this.MAX_ATTEMPTS) {
                        console.warn(`[感知者] 等待 ${this.RETRY_WAIT_MS / 1000} 秒后重试第 ${index} 组`);
                        await new Promise(resolve => setTimeout(resolve, this.RETRY_WAIT_MS));
                    }
                }
            }
            return '';
        }
        async generateObjectiveSummary(concatenated) {
            const prompt = this.summaryTaskTemplate.replace('{content}', concatenated);
            this.coverContext({ role: 'user', content: prompt });
            this.runtimeMessages = [];
            try {
                const response = this.run([], []);
                return response.body?.choices?.[0]?.message?.content || '';
            }
            catch (error) {
                console.error('[感知者] 客观摘要推理失败:', error);
                return '';
            }
        }
    }

    const STICKER_COLLECTION = 'stickers';
    let stickerCollectionReady = false;
    const ACTION_COLLECTION = 'actions';
    let actionCollectionReady = false;
    function syncActionsToMemory(actionNames) {
        if (!actionNames || actionNames.length === 0)
            return false;
        try {
            if (!actionCollectionReady) {
                const [ready] = memoryInit(ACTION_COLLECTION);
                if (!ready)
                    return false;
                actionCollectionReady = true;
            }
            for (const name of actionNames) {
                const trimmed = (name || '').trim();
                if (!trimmed)
                    continue;
                memoryAddWithTags(ACTION_COLLECTION, 'assistant', trimmed, [trimmed]);
            }
            return true;
        }
        catch (error) {
            console.error('[动作库] 动作组入库失败:', error);
            return false;
        }
    }
    function queryBestAction(text) {
        if (!text || !text.trim())
            return null;
        try {
            const [results, error] = memoryQuery(ACTION_COLLECTION, text.trim(), 1);
            if (error || !results || results.length === 0)
                return null;
            return results[0].content || null;
        }
        catch (error) {
            console.error('[动作库] 匹配失败:', error);
            return null;
        }
    }
    async function queryEmotionSticker(query) {
        if (!query || !query.trim())
            return null;
        try {
            if (!stickerCollectionReady) {
                const [ready] = memoryInit(STICKER_COLLECTION);
                if (!ready)
                    return null;
                stickerCollectionReady = true;
            }
            const [results, error] = memoryQuery(STICKER_COLLECTION, query.trim(), 3);
            if (error || !results || results.length === 0)
                return null;
            const image = results[RandomFloor(0, results.length - 1)].base64;
            return image || null;
        }
        catch (error) {
            console.error('[表情包] 检索失败:', error);
            return null;
        }
    }
    function extractTextFromMessage(message) {
        if (typeof message.content === 'string')
            return message.content;
        if (Array.isArray(message.content)) {
            return message.content.filter(item => item.type === 'text').map(item => item.text).join(' ');
        }
        return '';
    }
    function extractMemoryText(message) {
        return extractTextFromMessage(message);
    }
    function initMemory() {
        if (GlobalConfig.memoryReady)
            return;
        const [_, err] = memoryInit('lunar_messages');
        if (err)
            console.error('记忆库初始化失败:', err);
        else
            GlobalConfig.memoryReady = true;
    }
    function ensureMemoryReady() {
        if (!GlobalConfig.memoryReady)
            initMemory();
        return GlobalConfig.memoryReady;
    }

    const MEMORY_COLLECTION = 'lunar_messages';
    const RAG_SUMMARY_HARD_LIMIT = 4096;
    const RAG_TIMELINE_HARD_LIMIT = 2048;
    const RAG_PER_QUERY_TOP_K = 10;
    const RAG_MAX_RECORDS = 24;
    class MemorizerRole extends ModelBuilder {
        roleTag = "记忆者";
        constructor() {
            super(fileView('prompts/memorizerRole.md')[0]);
        }
        persistUnreadRecords() {
            if (GlobalConfig.unreadRecords.length === 0)
                return;
            if (!ensureMemoryReady()) {
                console.warn('[记忆] 记忆库未就绪，保留缓冲消息待下次触发');
                return;
            }
            let written = 0;
            for (const message of GlobalConfig.unreadRecords) {
                if (message.role === 'tool')
                    continue;
                const content = extractMemoryText(message).trim();
                if (!content || content.length <= 5)
                    continue;
                if (content.includes(SCHEDULE_TRIGGER_PREFIX))
                    continue;
                const [, error] = memoryAdd(MEMORY_COLLECTION, message.role, content);
                if (error)
                    console.error('[记忆] 写入记忆库失败:', error);
                else
                    written++;
            }
            console.log(`[记忆] 已写入 ${written} 条消息到记忆库`);
            GlobalConfig.unreadRecords = [];
        }
        queryRagDigest(userMessages, summarize) {
            if (!userMessages || userMessages.length === 0 || !ensureMemoryReady())
                return '';
            let records = this.retrieveRagRecords(userMessages);
            if (records.length === 0)
                return '';
            const feedback = interactEvent('build_memory_before', { userMessages, records }).return;
            if (feedback && Array.isArray(feedback) && feedback.length > 0 && feedback.every(r => r.id && r.role && r.content && r.similarity !== undefined)) {
                records = feedback;
            }
            return summarize ? this.summarizeRecords(records) : this.buildTimeline(records);
        }
        buildTimeline(records) {
            const sorted = [...records].sort((a, b) => (a.timestamp ?? 0) - (b.timestamp ?? 0));
            let timeline = '';
            for (const r of sorted) {
                const time = r.timestamp ? new Date(r.timestamp * 1000).toLocaleString('zh-CN', { hour12: false }) : '未知';
                const line = `[ 时间: ${time} ] [ 内容: ${r.content || '(空)'} ]`;
                if (timeline.length + line.length + 1 > RAG_TIMELINE_HARD_LIMIT)
                    break;
                timeline += (timeline ? '\n' : '') + line;
            }
            console.log(`[记忆] 已生成时间线（${timeline.length}/${RAG_TIMELINE_HARD_LIMIT} 字，${timeline.split('\n').length} 行）`);
            return timeline;
        }
        retrieveRagRecords(userMessages) {
            const allResults = [];
            for (const userMessage of userMessages) {
                const [results, error] = memoryQuery(MEMORY_COLLECTION, userMessage, RAG_PER_QUERY_TOP_K);
                if (error) {
                    console.error('记忆库查询失败:', error);
                    continue;
                }
                if (results && results.length > 0)
                    allResults.push(...results);
            }
            const seen = new Map();
            for (const r of allResults) {
                const content = r.content || '';
                const existing = seen.get(content);
                if (!existing || r.similarity > existing.similarity)
                    seen.set(content, r);
            }
            const uniqueResults = Array.from(seen.values()).sort((a, b) => b.similarity - a.similarity);
            console.log(`[记忆] 检索到 ${uniqueResults.length} 条相关记录，相似度范围: ${uniqueResults[0]?.similarity?.toFixed(4) ?? 'N/A'} ~ ${uniqueResults[uniqueResults.length - 1]?.similarity?.toFixed(4) ?? 'N/A'}`);
            return uniqueResults.slice(0, RAG_MAX_RECORDS);
        }
        summaryTaskTemplate = fileView('prompts/memorizerSummaryTask.md')[0];
        summarizeRecords(records) {
            const fragments = records
                .map((r, i) => {
                let header = `--- 片段 ${i + 1}（${r.role}`;
                if (r.timestamp)
                    header += `，${new Date(r.timestamp * 1000).toLocaleString('zh-CN', { hour12: false })}`;
                header += `）---`;
                return `${header}\n${r.content || '(空)'}`;
            })
                .join('\n\n');
            this.coverContext({ role: 'user', content: this.summaryTaskTemplate.replace('{fragments}', fragments) });
            this.runtimeMessages = [];
            let response;
            try {
                response = this.run([], []);
            }
            catch (error) {
                console.error('[记忆] 摘要推理失败:', error);
                return '';
            }
            let digest = (response.body?.choices?.[0]?.message?.content || '').trim();
            if (digest.length > RAG_SUMMARY_HARD_LIMIT)
                digest = digest.slice(0, RAG_SUMMARY_HARD_LIMIT);
            console.log(`[记忆] 已生成摘要（${digest.length}/${RAG_SUMMARY_HARD_LIMIT} 字）`);
            return digest;
        }
    }

    const USE_THE_PROGRAM_PROMPT = '- 当被要求或需要(打开/启动/运行/使用)某个程序或操作时，必须调用工具"use_the_program"（tool 参数填目标工具名、instruction 参数填自然语言指令），不得凭空编造操作结果，必须等待工具真实返回。';
    const descriptionRole = new ModelBuilder(fileView('prompts/descriptionRole.md')[0]);
    const memorizerRole = new MemorizerRole();
    const painterRole = new PainterRole();
    const musicianRole = new MusicianRole();
    const dialogueRole = new DialogueRole(descriptionRole);
    const perceiverRole = new PerceiverRole();
    registerMediaRoles({ perceiverRole, randomDefaultMessage });
    registerAgentTargets({ painterRole, musicianRole });
    registerDescribeRole(descriptionRole);
    registerDumpRoles({
        dialogue: { role: dialogueRole, label: '对话者' },
        painter: { role: painterRole, label: '绘制者' },
        musician: { role: musicianRole, label: '演奏者' },
        perceiver: { role: perceiverRole, label: '感知者' },
        memorizer: { role: memorizerRole, label: '记忆者' },
        description: { role: descriptionRole, label: '描述者' },
    });
    dialogueRole.appendPrompt(USE_THE_PROGRAM_PROMPT);
    const roleInstances = {
        dialogue: dialogueRole,
        painter: painterRole,
        musician: musicianRole,
        perceiver: perceiverRole,
        memorizer: memorizerRole,
        description: descriptionRole,
    };
    try {
        const host = globalThis;
        host.agentSystem = Object.assign(host.agentSystem || {}, roleInstances, { randomDefaultMessage });
    }
    catch (e) {
        console.warn('[角色] 宿主全局挂载失败（不影响运行）:', e);
    }
    function randomDefaultMessage() {
        return ['月华在哦', '怎么了吗?', '详细说说?'][RandomFloor(0, 2)];
    }

    async function fetchDocumentCallback(url, initializeContent = '{}', callback) {
        const defaultCallback = (content) => JSON.parse(content);
        const applyCallback = defaultCallback;
        const fallback = async () => {
            saveFile(url.toString(), true, initializeContent);
            return applyCallback(initializeContent);
        };
        try {
            const filePath = url.toString().split(/[\/\\]/);
            const [list, err1] = fileList(filePath.slice(0, -1).join('/'));
            if (err1)
                return await fallback();
            const exists = list.some(item => item.name === filePath.slice(-filePath.length)[0] && !item.isDir);
            if (!exists)
                return await fallback();
            const [content, size, mimeType, err2] = readFile(url.toString());
            if (err2)
                return await fallback();
            const text = atob(String(content));
            if (!text)
                return await fallback();
            return applyCallback(text);
        }
        catch (error) {
            if (error instanceof Error)
                return await fallback();
        }
    }

    const SECTION_BLOCK_LINES = 60;
    function parseDocumentSections(text) {
        const lines = String(text ?? '').replace(/\r\n/g, '\n').split('\n');
        if (lines.length === 0)
            return [];
        const headings = [];
        for (let i = 0; i < lines.length; i++) {
            const parsed = parseHeadingLine(lines[i]);
            if (parsed)
                headings.push({ index: i, level: parsed.level, title: parsed.title });
        }
        if (headings.length === 0)
            return fallbackSections(lines);
        const drafts = [];
        const pathStack = [];
        for (let ordinal = 0; ordinal < headings.length; ordinal++) {
            const heading = headings[ordinal];
            while (pathStack.length > 0 && pathStack[pathStack.length - 1].level >= heading.level)
                pathStack.pop();
            pathStack.push(heading);
            const endIndex = ordinal + 1 < headings.length ? headings[ordinal + 1].index : lines.length;
            drafts.push({
                ordinal: ordinal + 1,
                level: heading.level,
                title: heading.title,
                path: pathStack.map(item => item.title).join(' / '),
                lineStart: heading.index + 1,
                lineEnd: endIndex,
                chars: countChars(lines.slice(heading.index, endIndex))
            });
        }
        return drafts;
    }
    function fallbackSections(lines) {
        const drafts = [];
        const total = lines.length;
        for (let start = 0; start < total; start += SECTION_BLOCK_LINES) {
            const end = Math.min(start + SECTION_BLOCK_LINES, total);
            drafts.push({
                ordinal: drafts.length + 1,
                level: 0,
                title: '',
                path: '',
                lineStart: start + 1,
                lineEnd: end,
                chars: countChars(lines.slice(start, end))
            });
        }
        return drafts;
    }
    function parseHeadingLine(line) {
        const trimmed = (line ?? '').replace(/^[ \t]+/, '');
        let level = 0;
        while (level < trimmed.length && trimmed[level] === '#')
            level++;
        if (level === 0 || level > 6)
            return null;
        const rest = trimmed.slice(level);
        if (rest === '')
            return { level, title: '' };
        if (rest[0] !== ' ' && rest[0] !== '\t')
            return null;
        return { level, title: rest.trim() };
    }
    function countChars(lines) {
        let total = 0;
        for (const line of lines)
            total += Array.from(line).length;
        return total;
    }

    const FILE_WHITELIST = [
        'ts', 'js', 'tsx', 'jsx', 'go', 'py', 'java', 'c', 'h', 'cpp', 'cxx', 'hpp', 'cs',
        'md', 'markdown', 'txt', 'json', 'xml', 'yaml', 'yml', 'toml', 'ini', 'cfg', 'log',
        'csv', 'rs', 'rb', 'sh', 'ps1', 'bat', 'sql', 'html', 'htm', 'css', 'vue'
    ];
    const BINARY_EXTENSIONS = [
        'png', 'jpg', 'jpeg', 'webp', 'gif', 'bmp', 'tiff', 'tif', 'ico', 'svg',
        'mp4', 'webm', 'avi', 'mov', 'mkv', 'flv', 'm4v', 'mp3', 'wav', 'flac', 'ogg', 'm4a',
        'zip', '7z', 'rar', 'gz', 'exe', 'dll', 'pdf', 'docx', 'xlsx', 'pptx'
    ];
    const FENCE = '```';
    const QUERY_TOP_K = 8;
    const QUERY_CHAR_BUDGET = 12000;
    const SCORE_BODY_CHARS = 4000;
    const FULLTEXT_CHAR_LIMIT = 6000;
    function extOf(fileName) {
        const parts = (fileName || '').split('.');
        return parts.length > 1 ? parts.pop().toLowerCase() : '';
    }
    function looksLikeFileHeader(firstLine) {
        return /^[A-Za-z0-9_\-.\u4e00-\u9fff]+\.\w+$/.test((firstLine || '').trim());
    }
    function toUTF8Bytes(text) {
        const bytes = new Uint8Array(text.length * 3);
        let offset = 0;
        for (let i = 0; i < text.length; i++) {
            let code = text.charCodeAt(i);
            if (code >= 0xd800 && code <= 0xdbff && i + 1 < text.length) {
                const next = text.charCodeAt(i + 1);
                if (next >= 0xdc00 && next <= 0xdfff) {
                    code = ((code - 0xd800) << 10) + (next - 0xdc00) + 0x10000;
                    i++;
                }
            }
            if (code < 0x80)
                bytes[offset++] = code;
            else if (code < 0x800) {
                bytes[offset++] = 0xc0 | (code >> 6);
                bytes[offset++] = 0x80 | (code & 0x3f);
            }
            else if (code < 0x10000) {
                bytes[offset++] = 0xe0 | (code >> 12);
                bytes[offset++] = 0x80 | ((code >> 6) & 0x3f);
                bytes[offset++] = 0x80 | (code & 0x3f);
            }
            else {
                bytes[offset++] = 0xf0 | (code >> 18);
                bytes[offset++] = 0x80 | ((code >> 12) & 0x3f);
                bytes[offset++] = 0x80 | ((code >> 6) & 0x3f);
                bytes[offset++] = 0x80 | (code & 0x3f);
            }
        }
        return bytes.subarray(0, offset);
    }
    const CODE_LANGS = {
        ts: 'TypeScript', tsx: 'TypeScript', js: 'JavaScript', jsx: 'JavaScript',
        go: 'Go', py: 'Python', java: 'Java', c: 'C', h: 'C', cpp: 'C++', cxx: 'C++',
        hpp: 'C++', cs: 'C#', rs: 'Rust', rb: 'Ruby', sh: 'Shell', ps1: 'PowerShell',
        bat: 'Batch', sql: 'SQL', html: 'HTML', htm: 'HTML', css: 'CSS', vue: 'Vue'
    };
    function loadFileIndex(handle, fileName, sourcePath) {
        const keys = [handle];
        if (sourcePath)
            keys.push(`@path:${sourcePath}`);
        for (const key of keys) {
            const cached = getFileIndexFromKnowledge(key);
            if (!cached)
                continue;
            try {
                return JSON.parse(cached);
            }
            catch {
                continue;
            }
        }
        return null;
    }
    function storeFileIndex(handle, fileName, index) {
        saveFileIndexToKnowledge(handle, JSON.stringify(index));
        if (index.path)
            saveFileIndexToKnowledge(`@path:${index.path}`, JSON.stringify(index));
        if (fileName && `#${fileName}` !== handle)
            saveFileIndexToKnowledge(`#${fileName}`, JSON.stringify(index));
    }
    function admitDocumentContent(fileName, content) {
        const admitted = admitBytes(toUTF8Bytes(content), fileName, 'client-upload', 'normal');
        if (!admitted)
            return null;
        if (!admitted.deduped)
            return admitted;
        const existing = listResources(admitted.kind, 500).find(entry => entry.name === fileName && entry.content_hash === admitted.content_hash);
        if (!existing || existing.id === admitted.id)
            return admitted;
        deleteResource(admitted.handle);
        return {
            ...admitted,
            id: existing.id,
            handle: existing.handle,
            seq: existing.seq,
            deduped: true
        };
    }
    async function importDocumentBlock(fileName, content) {
        const ext = extOf(fileName);
        if (BINARY_EXTENSIONS.includes(ext))
            return { handle: '', skipped: true };
        if (!FILE_WHITELIST.includes(ext))
            return { handle: '', skipped: true };
        const admitted = admitDocumentContent(fileName, content);
        if (!admitted)
            return { handle: '', skipped: true };
        if (!admitted.deduped) {
            const drafts = parseDocumentSections(content);
            if (drafts.length > 0)
                setResourceSections(admitted.id, drafts);
            const firstLine = content.replace(/^\uFEFF/, '')
                .split('\n')
                .map(line => line.replace(/^#+\s*/, '').trim())
                .find(line => line.length > 0) || '';
            if (firstLine)
                setResourceSummary(admitted.id, firstLine.slice(0, 200));
            console.log(`[阅读者] 已收录 ${admitted.handle}（${fileName}，${drafts.length} 段，${admitted.size_bytes} 字节）`);
        }
        const index = {
            collection: '',
            name: fileName,
            path: fileName,
            contentHash: admitted.content_hash,
            ext,
            lang: CODE_LANGS[ext] ?? null,
            chunkCount: 0,
            processedAt: Date.now(),
            version: 2,
            resourceId: admitted.id,
            handle: admitted.handle
        };
        storeFileIndex(admitted.handle, fileName, index);
        return { handle: admitted.handle, skipped: false };
    }
    function migrateLegacyFileIndex(fileName) {
        const cached = getFileIndexFromKnowledge(`#${fileName}`);
        if (!cached)
            return '';
        let legacy = null;
        try {
            legacy = JSON.parse(cached);
        }
        catch {
            return '';
        }
        if (!legacy)
            return '';
        if (legacy.version === 2 && legacy.handle)
            return legacy.handle;
        if (!legacy.collection)
            return '';
        try {
            const [ready] = memoryInit(legacy.collection);
            if (!ready) {
                console.error(`[阅读者] 迁移失败：记忆库集合不可用 ${legacy.collection}`);
                return '';
            }
            const [results, queryError] = memoryQuery(legacy.collection, '文件内容 结构 概览 摘要', Math.max(legacy.chunkCount || 0, 64));
            if (queryError || !results || results.length === 0) {
                console.error(`[阅读者] 迁移失败：旧集合无可用切片 ${legacy.collection}`);
                return '';
            }
            const chunks = results
                .map(item => item.content || '')
                .filter(text => text.length > 0);
            if (chunks.length === 0)
                return '';
            const rebuilt = rebuildFromChunks(chunks);
            const admitted = admitBytes(toUTF8Bytes(rebuilt), fileName, 'client-upload', 'normal');
            if (!admitted)
                return '';
            const drafts = parseDocumentSections(rebuilt);
            if (drafts.length > 0)
                setResourceSections(admitted.id, drafts);
            setResourceSummary(admitted.id, `（由历史切片重建，${chunks.length} 片 → ${drafts.length} 段）`);
            const index = {
                collection: legacy.collection,
                name: fileName,
                path: fileName,
                contentHash: admitted.content_hash,
                ext: extOf(fileName),
                lang: legacy.lang ?? null,
                chunkCount: drafts.length,
                processedAt: Date.now(),
                version: 2,
                resourceId: admitted.id,
                handle: admitted.handle
            };
            storeFileIndex(admitted.handle, fileName, index);
            console.log(`[阅读者] 历史索引已迁移：${fileName} → ${admitted.handle}（${chunks.length} 片 → ${drafts.length} 段）`);
            return admitted.handle;
        }
        catch (error) {
            console.error(`[阅读者] 迁移历史索引异常 ${fileName}:`, error);
            return '';
        }
    }
    const CHUNK_PATH_PREFIX = /^\*[^*]*>\s*([\s\S]*?)\*(?:\n|$)/;
    function rebuildFromChunks(chunks) {
        const lines = [];
        let previousSegments = [];
        for (const chunk of chunks) {
            const match = CHUNK_PATH_PREFIX.exec(chunk);
            const body = match ? chunk.slice(match[0].length) : chunk;
            const path = match ? match[1].trim() : '';
            const segments = path ? path.split('/').map(segment => segment.split('(')[0].trim()).filter(Boolean) : [];
            if (segments.length > 0) {
                let common = 0;
                while (common < segments.length && common < previousSegments.length && segments[common] === previousSegments[common])
                    common++;
                for (let i = common; i < segments.length; i++) {
                    lines.push('#'.repeat(Math.min(i + 1, 6)) + ' ' + segments[i]);
                }
                previousSegments = segments;
            }
            const trimmed = body.replace(/\s+$/, '');
            if (trimmed)
                lines.push(trimmed);
        }
        return lines.join('\n') + '\n';
    }
    async function processImportBlocksInText(raw) {
        if (!raw.includes(FENCE))
            return { text: raw, imported: [] };
        let out = '';
        let i = 0;
        const imported = [];
        while (true) {
            const start = raw.indexOf(FENCE, i);
            if (start < 0) {
                out += raw.slice(i);
                break;
            }
            out += raw.slice(i, start);
            const rest = raw.slice(start + FENCE.length);
            const nl = rest.indexOf('\n');
            const eol = nl < 0 ? rest.length : nl;
            const firstLine = rest.slice(0, eol).trim();
            if (!looksLikeFileHeader(firstLine)) {
                out += FENCE;
                i = start + FENCE.length;
                continue;
            }
            const closeIdx = rest.lastIndexOf(FENCE);
            if (closeIdx <= eol) {
                out += raw.slice(start, start + FENCE.length);
                i = start + FENCE.length;
                continue;
            }
            const content = rest.slice(eol + 1, closeIdx);
            const result = await importDocumentBlock(firstLine, content);
            if (result.skipped) {
                out += raw.slice(start, start + FENCE.length + closeIdx + FENCE.length);
            }
            else {
                out += `[已收录] ${result.handle} ${firstLine}`;
                if (!imported.includes(result.handle))
                    imported.push(result.handle);
            }
            i = start + FENCE.length + closeIdx + FENCE.length;
        }
        return { text: out, imported };
    }
    function ensureSections(handle) {
        const sections = outlineResource(handle);
        return sections && sections.length > 0 ? sections : [];
    }
    function lexicalScore(handle, section, query) {
        if (!query)
            return 0;
        const head = query.trim().toLowerCase();
        if (!head)
            return 0;
        let score = 0;
        const headText = `${section.title} ${section.path}`.toLowerCase();
        if (headText.includes(head))
            score += 10;
        const body = readResource(`${handle}|L${section.line_start}-${section.line_end}`, SCORE_BODY_CHARS);
        const bodyText = (body?.content || '').toLowerCase();
        if (bodyText) {
            if (bodyText.includes(head))
                score += 5;
            for (let i = 0; i + 1 < head.length; i++) {
                const bigram = head.slice(i, i + 2);
                if (bigram.trim().length < 2)
                    continue;
                let from = 0;
                while (true) {
                    const found = bodyText.indexOf(bigram, from);
                    if (found < 0)
                        break;
                    score += 1;
                    from = found + bigram.length;
                }
            }
        }
        return score;
    }
    function selectSections(handle, query) {
        const sections = ensureSections(handle);
        if (sections.length === 0)
            return [];
        if (!query || !query.trim()) {
            return sections.slice(0, 3);
        }
        const scored = sections.map(section => ({ section, score: lexicalScore(handle, section, query) }));
        scored.sort((a, b) => b.score - a.score || a.section.ordinal - b.section.ordinal);
        const picked = scored.filter(item => item.score > 0).slice(0, QUERY_TOP_K).map(item => item.section);
        return picked.length > 0 ? picked : sections.slice(0, 3);
    }
    function expandReference(ref, query) {
        const parsed = parseHandle(ref.handle);
        if (!parsed)
            return ref.raw;
        if (parsed.anchor === 'line') {
            const result = readResource(ref.handle);
            if (!result)
                return ref.raw;
            if (result.lost)
                return `${ref.handle}\n${degradeNotice(ref.handle, '内容已不可取，仅保留目录条目')}`;
            return `【${ref.handle}｜${result.title}｜第 ${result.line_start}-${result.line_end} 行】\n${result.content || ''}`;
        }
        const exists = listResources(parsed.kind, 500).some(entry => entry.handle === ref.handle.replace(/\|.*$/, ''));
        if (!exists && parsed.kind !== 'D' && parsed.kind !== 'T') {
            const described = describeResource(ref.handle, query);
            return described ? `【${ref.handle}】${described}` : ref.raw;
        }
        if (!exists)
            return ref.raw;
        if (parsed.anchor === 'section') {
            const result = readResource(ref.handle);
            if (!result)
                return ref.raw;
            if (result.lost)
                return `${ref.handle}\n${degradeNotice(ref.handle, '内容已不可取，仅保留目录条目')}`;
            const label = result.section_title ? `｜${result.section_title}` : '';
            return `【${ref.handle}${label}｜第 ${result.line_start}-${result.line_end} 行】\n${result.content || ''}`;
        }
        const sections = selectSections(ref.handle, query);
        if (sections.length === 0) {
            return expandFlatResource(ref, query);
        }
        const parts = [];
        let budget = QUERY_CHAR_BUDGET;
        for (const section of sections) {
            if (budget <= 0)
                break;
            const slice = readResource(`${ref.handle.replace(/\|.*$/, '')}|L${section.line_start}-${section.line_end}`, budget);
            const body = slice?.content || '';
            if (!body)
                continue;
            budget -= body.length;
            const label = section.path || section.title || `第 ${section.ordinal} 节`;
            parts.push(`【${ref.handle.replace(/\|.*$/, '')}|§${section.ordinal}｜${label}｜第 ${section.line_start}-${section.line_end} 行】\n${body}`);
        }
        if (parts.length === 0)
            return ref.raw;
        return parts.join('\n---\n');
    }
    function expandFlatResource(ref, query) {
        const handle = ref.handle.replace(/\|.*$/, '');
        const entry = listResources('', 500).find(item => item.handle === handle);
        const result = readResource(handle, FULLTEXT_CHAR_LIMIT + 1);
        if (!result)
            return ref.raw;
        if (result.lost)
            return `${handle}\n${degradeNotice(handle, '内容已不可取，仅保留目录条目')}`;
        const content = result.content || '';
        if (!content) {
            const described = describeResource(handle, query);
            return described ? `【${handle}】${described}` : ref.raw;
        }
        if (content.length <= FULLTEXT_CHAR_LIMIT && !result.truncated) {
            return `【${handle}｜${result.title}｜共 ${result.total_lines || 0} 行】\n${content}`;
        }
        const sizeNote = `约 ${content.length} 字符 / ${result.total_lines || 0} 行`;
        const sectionNote = entry?.section_count ? `，${entry.section_count} 节` : '';
        const summary = entry?.summary ? `\n摘要：${entry.summary}` : '';
        return `【${handle}｜${result.title}｜${sizeNote}${sectionNote}】体积较大，未自动注入正文。`
            + `${summary}\n需要时请用 read_resource 按句柄读取（可带 |§n 章节锚点或 |L<起>-<止> 行区间）`
            + `${entry?.section_count ? '，先用 outline_resource 查看结构再定向读取' : ''}。`;
    }
    const HANDLE_REF = /@[DIVAT]\.\d{3,}(?:\|(?:§\d+|L\d+-\d+))?/g;
    function resolveFreeTextReference(fileName) {
        const target = String(fileName || '').replace(/^#/, '').trim();
        if (!target)
            return '';
        const indexed = loadFileIndex('', undefined, target);
        if (indexed?.handle && indexed.version === 2)
            return indexed.handle;
        const candidates = listResources('', 500).filter(item => item.name === target);
        if (candidates.length > 0) {
            const samePath = indexed?.contentHash
                ? candidates.find(item => item.content_hash === indexed.contentHash)
                : undefined;
            if (samePath)
                return samePath.handle;
            if (!indexed && candidates.length === 1) {
                storeFileIndex(candidates[0].handle, target, {
                    collection: '',
                    name: target,
                    path: target,
                    contentHash: candidates[0].content_hash,
                    ext: extOf(target),
                    lang: CODE_LANGS[extOf(target)] ?? null,
                    chunkCount: candidates[0].section_count,
                    processedAt: Date.now(),
                    version: 2,
                    resourceId: candidates[0].id,
                    handle: candidates[0].handle
                });
                return candidates[0].handle;
            }
        }
        const admitted = admitDocumentByName(target);
        if (admitted) {
            const content = readResource(admitted.handle, 4 * 1024 * 1024)?.content || '';
            const drafts = parseDocumentSections(content);
            if (drafts.length > 0)
                setResourceSections(admitted.id, drafts);
            storeFileIndex(admitted.handle, target, {
                collection: '',
                name: target,
                path: target,
                contentHash: admitted.content_hash,
                ext: extOf(target),
                lang: CODE_LANGS[extOf(target)] ?? null,
                chunkCount: drafts.length,
                processedAt: Date.now(),
                version: 2,
                resourceId: admitted.id,
                handle: admitted.handle
            });
            console.log(`[阅读者] 按需收录 ${admitted.handle}（${target}，${drafts.length} 段）`);
            return admitted.handle;
        }
        return migrateLegacyFileIndex(target);
    }
    const FILE_REF = /(?<![\w.@])@([A-Za-z0-9_\-.]+\.\w{1,8})/g;
    function processReferencesInText(raw) {
        if (!raw || (!raw.includes('@') && !raw.includes('(#')))
            return { text: raw, changed: false };
        const feedback = interactEvent('read_file_before', { text: raw }).return;
        if (feedback && typeof feedback.text === 'string' && feedback.text !== raw)
            raw = feedback.text;
        const refs = [];
        HANDLE_REF.lastIndex = 0;
        let match;
        while ((match = HANDLE_REF.exec(raw)) !== null) {
            const normalized = formatHandle(parseHandle(match[0]));
            refs.push({ raw: match[0], start: match.index, end: match.index + match[0].length, handle: normalized });
        }
        FILE_REF.lastIndex = 0;
        while ((match = FILE_REF.exec(raw)) !== null) {
            const start = match.index;
            const end = start + match[0].length;
            if (refs.some(ref => start < ref.end && end > ref.start))
                continue;
            const handle = resolveFreeTextReference(match[1]);
            if (!handle)
                continue;
            refs.push({ raw: match[0], start, end, handle });
        }
        const LEGACY_REF = /\(#([\w.\-]+)\)/g;
        while ((match = LEGACY_REF.exec(raw)) !== null) {
            const start = match.index;
            const end = start + match[0].length;
            if (refs.some(ref => start < ref.end && end > ref.start))
                continue;
            const handle = resolveFreeTextReference(match[1]);
            if (!handle)
                continue;
            refs.push({ raw: match[0], start, end, handle });
        }
        if (refs.length === 0)
            return { text: raw, changed: false };
        refs.sort((a, b) => a.start - b.start);
        const parts = [];
        const lead = raw.slice(0, refs[0].start).trim();
        if (lead)
            parts.push(lead + '\n');
        let changed = false;
        for (let i = 0; i < refs.length; i++) {
            const ref = refs[i];
            const gapStart = ref.end;
            const gapEnd = i + 1 < refs.length ? refs[i + 1].start : raw.length;
            let query = raw.slice(gapStart, gapEnd).trim();
            query = query.replace(/^[：:]/, '').trim();
            const expanded = expandReference(ref, query);
            if (expanded !== ref.raw)
                changed = true;
            parts.push(expanded);
            if (query)
                parts.push('\n' + query + '\n');
        }
        return { text: parts.join('').trimEnd(), changed };
    }
    async function processMessage(message) {
        if (typeof message.content === 'string') {
            const original = message.content;
            const imported = await processImportBlocksInText(original);
            const ref = processReferencesInText(imported.text);
            message.content = ref.text;
            return { changed: message.content !== original, imported: imported.imported };
        }
        if (Array.isArray(message.content)) {
            const textParts = message.content.filter(it => it.type === 'text').map(it => it.text);
            const others = message.content.filter(it => it.type !== 'text');
            if (textParts.length === 0)
                return { changed: false, imported: [] };
            const raw = textParts.join('\n');
            const imported = await processImportBlocksInText(raw);
            const ref = processReferencesInText(imported.text);
            message.content = [...others, { type: 'text', text: ref.text }];
            return { changed: true, imported: imported.imported };
        }
        return { changed: false, imported: [] };
    }
    async function processUnreadFiles() {
        for (const message of GlobalUnreadContext()) {
            try {
                await processMessage(message);
            }
            catch (error) {
                console.error('[阅读者] 处理消息失败:', error);
            }
        }
    }
    function GlobalUnreadContext() {
        return GlobalConfig.unreadContext;
    }

    function cleanTextForTTS(text) {
        if (!text)
            return '';
        let processed = text;
        processed = processed.replace(/`[^`]*`/g, '');
        processed = processed.replace(/!\[.*?\]\(.*?\)/g, '');
        processed = processed.replace(/\[([^\]]*)\]\(.*?\)/g, '$1');
        processed = processed.replace(/<[^>]*>/g, '');
        processed = processed.replace(/[\u{1F600}-\u{1F64F}\u{1F300}-\u{1F5FF}\u{1F680}-\u{1F6FF}\u{1F900}-\u{1F9FF}\u{2600}-\u{26FF}\u{2700}-\u{27BF}\u{FE00}-\u{FE0F}\u{1F1E0}-\u{1F1FF}\u{200D}\u{20E3}\u{FE0F}]/gu, '');
        processed = processed.replace(/\*/g, '');
        processed = processed.replace(/\r?\n/g, ' ');
        processed = processed.replace(/\（[^）]*\）/g, '');
        processed = processed.replace(/\([^)]*\)/g, '');
        const allowed = '\\u4e00-\\u9fff' + 'a-zA-Z0-9' + '\\s_~\\-' + '\uFF0C\u3002\uFF1F\uFF1A\uFF01\uFF1B\u3001\u2014\u2026\u300A\u300B\u3008\u3009\u201C\u201D\u2018\u2019\uFF08\uFF09\u3010\u3011' + ',.\'\"?:!;()\\[\\]';
        const whitelist = new RegExp(`[^${allowed}]`, 'g');
        processed = processed.replace(whitelist, ',');
        processed = processed.replace(/,{2,}/g, ',');
        processed = processed.replace(/\s+/g, ' ');
        return processed.trim();
    }
    function removeEmojiSymbols(text) {
        return text.replace(/[\u{1F600}-\u{1F64F}\u{1F300}-\u{1F5FF}\u{1F680}-\u{1F6FF}\u{1F900}-\u{1F9FF}\u{2600}-\u{26FF}\u{2700}-\u{27BF}\u{FE00}-\u{FE0F}\u{1F1E0}-\u{1F1FF}\u{200D}\u{20E3}\u{FE0F}]/gu, '');
    }
    function formatChunk(chunk) {
        const LEADING_PUNCT = /^[。，、：；:;,?!？！—～"'""''()（）\[\]【】{}<>…\s]+/;
        const TRAILING_COMMA = /[，,]+$/;
        const result = chunk.replace(LEADING_PUNCT, '').replace(TRAILING_COMMA, '');
        return result.trim();
    }
    function splitByPunct(source, punctRegex) {
        const result = [];
        let start = 0;
        for (let i = 0; i < source.length; i++) {
            if (!punctRegex.test(source[i]))
                continue;
            let end = i + 1;
            while (end < source.length && punctRegex.test(source[end]))
                end++;
            const fragment = source.slice(start, end).trim();
            if (fragment.length > 0)
                result.push(fragment);
            start = end;
            i = end - 1;
        }
        if (start < source.length) {
            const fragment = source.slice(start).trim();
            if (fragment.length > 0)
                result.push(fragment);
        }
        return result;
    }
    function isInsideBracket(source, pos) {
        let depth = 0;
        for (let i = 0; i < pos; i++) {
            if (source[i] === '\uFF08' || source[i] === '(')
                depth++;
            else if (source[i] === '\uFF09' || source[i] === ')')
                depth--;
        }
        return depth > 0;
    }
    function splitSentences(text) {
        if (!text)
            return [];
        const LEVEL1_PUNCT = /[。？！—～?!]/;
        const LEVEL2_PUNCT = /[，,、：；:;]/;
        const IDEAL_MAXIMUM_LENGTH = 35;
        const level1 = splitByPunct(text, LEVEL1_PUNCT);
        const result = [];
        for (let content of level1) {
            if (content.length <= IDEAL_MAXIMUM_LENGTH) {
                const formatted = formatChunk(content);
                if (formatted.length > 0)
                    result.push(formatted);
                continue;
            }
            while (content.length > IDEAL_MAXIMUM_LENGTH) {
                let splitPos = -1;
                for (let i = Math.min(content.length - 1, IDEAL_MAXIMUM_LENGTH - 1); i >= 0; i--) {
                    if (!LEVEL2_PUNCT.test(content[i]) || isInsideBracket(content, i))
                        continue;
                    let end = i + 1;
                    while (end < content.length && LEVEL2_PUNCT.test(content[end]))
                        end++;
                    splitPos = end;
                    break;
                }
                if (splitPos === -1)
                    break;
                const slice = formatChunk(content.slice(0, splitPos));
                if (slice.length > 0)
                    result.push(slice);
                content = content.slice(splitPos);
            }
            const tail = formatChunk(content);
            if (tail.length > 0)
                result.push(tail);
        }
        return result;
    }

    const EMOJI_REGEX = /[\u{1F600}-\u{1F64F}\u{1F300}-\u{1F5FF}\u{1F680}-\u{1F6FF}\u{1F900}-\u{1F9FF}\u{1FA70}-\u{1FAFF}\u{2600}-\u{26FF}\u{2700}-\u{27BF}\u{FE00}-\u{FE0F}\u{1F1E0}-\u{1F1FF}\u{200D}\u{20E3}\u{FE0F}]/gu;
    const KAOMOJI_MARKS = '_^･・。.、；;～~ノﾉゞヾω￣▽△□○●☆★°´｀♪♫＞＜><｡一≧≦∇∀ﾟ⌒⌣◕';
    const KAOMOJI_REGEX = new RegExp(`[<＜＼]?[（(](?:[^\\s（()）\u4e00-\u9fff]|\u4e00){0,5}[${KAOMOJI_MARKS}](?:[^\\s（()）\u4e00-\u9fff]|\u4e00){0,5}[）)](?:[<＜>＞／\u30ce\u309e\u266a\u266b]*)?` +
        `|[>＜^TtOo0vV][_\\-=^><。.・oO][<＞^TtOo0vV]`, 'gu');
    const EMOTION_REGEX = new RegExp(`${EMOJI_REGEX.source}|${KAOMOJI_REGEX.source}`, 'gu');
    function extractThinkingBlocks(text) {
        const blocks = [];
        const regex = /<think>([\s\S]*?)<\/think>/gi;
        let match;
        while ((match = regex.exec(text)) !== null) {
            const content = match[1].trim();
            if (content.length > 0)
                blocks.push(content);
        }
        const remaining = text.replace(/<think>[\s\S]*?<\/think>/gi, '');
        return [blocks, remaining];
    }
    function extractCodeBlocks(text) {
        const blocks = [];
        const codeBlockRegex = /```[a-zA-Z0-9+#-]*[\s\S]*?```/g;
        let match;
        while ((match = codeBlockRegex.exec(text)) !== null) {
            blocks.push(match[0]);
        }
        const remaining = text.replace(/```[a-zA-Z0-9+#-]*[\s\S]*?```/g, '');
        return [blocks, remaining];
    }
    function extractActionBlocks(text) {
        const blocks = [];
        const stack = [];
        const ranges = [];
        for (let i = 0; i < text.length; i++) {
            const ch = text[i];
            if (ch === '(' || ch === '\uFF08')
                stack.push(i);
            else if (ch === ')' || ch === '\uFF09') {
                if (stack.length === 0)
                    continue;
                const start = stack.pop();
                if (stack.length !== 0)
                    continue;
                const content = text.slice(start + 1, i).trim();
                if (content.length > 0)
                    blocks.push(content);
                ranges.push([start, i + 1]);
            }
        }
        let remaining = '';
        let lastEnd = 0;
        for (const [start, end] of ranges) {
            remaining += text.slice(lastEnd, start);
            lastEnd = end;
        }
        remaining += text.slice(lastEnd);
        return [blocks, remaining];
    }
    function extractEmotionBlocks(text) {
        const blocks = [];
        const regex = new RegExp(EMOTION_REGEX.source, 'gu');
        let match;
        let lastEnd = -1;
        let current = '';
        while ((match = regex.exec(text)) !== null) {
            if (current.length > 0 && match.index === lastEnd) {
                current += match[0];
            }
            else {
                if (current.length > 0)
                    blocks.push(current);
                current = match[0];
            }
            lastEnd = match.index + match[0].length;
        }
        if (current.length > 0)
            blocks.push(current);
        const remaining = text.replace(regex, '');
        return [blocks, remaining];
    }

    function parseContent(rawText) {
        if (!rawText)
            return { thinkingBlocks: [], codeBlocks: [], actionBlocks: [], textChunks: [] };
        const [thinkingBlocks, textAfterThinking] = extractThinkingBlocks(rawText);
        const [codeBlocks, textAfterCode] = extractCodeBlocks(textAfterThinking);
        const [emotionBlocks, textAfterEmotion] = extractEmotionBlocks(textAfterCode);
        const [actionZoneBlocks, textAfterAction] = extractActionBlocks(textAfterEmotion);
        const actionBlocks = [...actionZoneBlocks, ...emotionBlocks];
        const displayText = removeEmojiSymbols(textAfterAction);
        const displayChunks = splitSentences(displayText);
        const textChunks = displayChunks.map(chunk => ({ display: chunk, tts: cleanTextForTTS(chunk), }));
        return { thinkingBlocks, codeBlocks, actionBlocks, textChunks };
    }

    function pushDecreeResponse(text) {
        const [audio, err] = tts(text);
        pushContext('text', text, (err || !audio) ? '' : audio);
        console.log(`[律令] 默认应答: ${text}`);
    }
    const decreeRegistry = {
        '全记住': () => {
            GlobalConfig.unreadRecords.push(...dialogueRole.messages);
            memorizerRole.persistUnreadRecords();
            pushDecreeResponse('全部记住啦~');
        },
    };
    function extractDecreeText(message) {
        if (typeof message.content === 'string')
            return message.content;
        return message.content
            .filter((c) => c.type === 'text')
            .map((c) => c.text)
            .join('\n');
    }
    function matchKnownDecrees(text) {
        const found = [];
        for (const tag of Object.keys(decreeRegistry)) {
            if (text.includes(`<${tag}>`))
                found.push(tag);
        }
        return found;
    }
    function processDecrees() {
        let hit = false;
        const remaining = [];
        for (const message of GlobalConfig.unreadContext) {
            const decrees = matchKnownDecrees(extractDecreeText(message));
            if (decrees.length === 0) {
                remaining.push(message);
                continue;
            }
            hit = true;
            for (const tag of decrees) {
                try {
                    decreeRegistry[tag](message);
                }
                catch (error) {
                    console.error(`[律令] 指令 <${tag}> 执行失败:`, error);
                }
            }
        }
        GlobalConfig.unreadContext = remaining;
        return hit;
    }

    async function createChatMessage() {
        const cache = { currentToolCallIndex: -1, currentFunctionArgs: '', currentFunctionName: '', descriptionContent: '', thinkingContent: '', currentToolCall: null, toolCalls: [], };
        await dialogueRole.generateDialogue(cache);
        return GlobalConfig.finalResponse;
    }
    let asrUnloaded = false;
    function guardASRUnload() {
        asrUnloaded = GlobalConfig.unreadContext.some(message => {
            const items = Array.isArray(message.content) ? message.content : [];
            return items.some(item => item.type === 'input_audio' || item.type === 'audio_url');
        });
        if (asrUnloaded || GlobalConfig.unreadContext.length === 0)
            return;
        const [_, error] = syncFetch({
            url: url()[0] + '/vram/unload',
            execute: {
                method: 'POST',
                crossDomain: true,
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ model: GlobalConfig.AsrName }),
            },
        });
        if (error)
            console.error('ASR 卸载执行失败:', error.message ?? String(error));
    }
    const DEEP_RECALL_PATTERN = /(回忆|记忆|记得|回想|想起来)/;
    function updatePreviousMemories() {
        if (GlobalConfig.unreadRecords.length >= 1)
            memorizerRole.persistUnreadRecords();
        const userMessages = GlobalConfig.unreadContext.filter(message => message.role === 'user').map(message => extractTextFromMessage(message).trim()).filter(text => text.length > 0);
        const clear = () => { dialogueRole.ragMessages = []; };
        if (userMessages.length === 0)
            return clear();
        const deepRecall = userMessages.some(text => DEEP_RECALL_PATTERN.test(text));
        const digest = memorizerRole.queryRagDigest(userMessages, deepRecall);
        if (!digest)
            return clear();
        dialogueRole.ragMessages = [{ role: 'user', content: `【长期记忆摘要】\n${digest}` }];
    }
    const FALLBACK_ACTIONS = ['荡秋千', '翻花绳'];
    function getAvailableActionNames() {
        try {
            const raw = getAvailableActions();
            if (!raw || raw === '{}')
                return FALLBACK_ACTIONS;
            const parsed = JSON.parse(raw);
            if (parsed.actions && Array.isArray(parsed.actions) && parsed.actions.length > 0) {
                return parsed.actions.map(item => item.name);
            }
        }
        catch {
        }
        return FALLBACK_ACTIONS;
    }
    async function performMatchedAction(query) {
        const actionNames = getAvailableActionNames();
        if (!syncActionsToMemory(actionNames))
            return;
        const bestAction = queryBestAction(query);
        if (!bestAction)
            return;
        const pos = getAgentPosition();
        sendToEngine('movement', JSON.stringify({ position: { x: pos.x + RandomFloat(-0.5, 0.5), y: pos.y, z: pos.z + RandomFloat(-0.5, 0.5), }, resumeTracking: true, }));
        sendToEngine('action', JSON.stringify({ action: bestAction }));
        pushContext('action', `月华${bestAction}了`, '');
    }
    async function thoughtLoopTickEvent() {
        if (GlobalConfig.reasoningInProgress)
            return;
        try {
            GlobalConfig.reasoningInProgress = true;
            pullContext().forEach(message => writeMessage(message.role, message.content));
            guardASRUnload();
            const messageLength = GlobalConfig.unreadContext.length;
            if (messageLength === 0) {
                checkDueItems().forEach(item => {
                    const feedback = interactEvent('execution_schedule_before', { plan: item.content }).return;
                    const content = (feedback && typeof feedback.plan === 'string') ? feedback.plan : item.content;
                    GlobalConfig.unreadContext.push({ role: 'user', content: `${SCHEDULE_TRIGGER_PREFIX} 预约时间已到，请执行以下计划：${content}` });
                });
                GlobalConfig.reasoningInProgress = false;
                return;
            }
            updatePreviousMemories();
            syncLTPXRemoteStatus();
            const feedback = interactEvent('message_received_before', { messages: GlobalConfig.unreadContext }).return;
            if (feedback && Array.isArray(feedback.messages))
                GlobalConfig.unreadContext = feedback.messages;
            await processUnreadFiles();
            if (processDecrees()) {
                GlobalConfig.reasoningInProgress = false;
                return;
            }
            await createChatMessage();
            if (!GlobalConfig.finalResponse.trim().length) {
                pushContext('text', randomDefaultMessage(), tts(randomDefaultMessage())[0]);
                return;
            }
            ;
            const { thinkingBlocks, codeBlocks, actionBlocks, textChunks } = parseContent(GlobalConfig.finalResponse);
            const validMessage = textChunks.map(chunk => chunk.display).join('\n').trim();
            if (!validMessage.length)
                throw new Error('清洗后的文本为空');
            if (actionBlocks.length) {
                const feedback = interactEvent('take_action_before', { actions: actionBlocks }).return;
                const actBlocks = (feedback && Array.isArray(feedback.actions)) ? feedback.actions : actionBlocks;
                await performMatchedAction(actBlocks.join(' ').trim() || validMessage);
                pushImage([await queryEmotionSticker(validMessage)], true);
            }
            else if (validMessage.length <= 36 && Math.random() < 0.15) {
                const feedback = interactEvent('express_emotions_before', { text: validMessage }).return;
                const emoText = (feedback && typeof feedback.text === 'string') ? feedback.text : validMessage;
                pushImage([await queryEmotionSticker(emoText)], true);
            }
            thinkingBlocks.forEach(thinking => pushContext('text', thinking, ''));
            codeBlocks.forEach(code => pushContext('text', code, ''));
            for (const chunk of textChunks) {
                let audio = '';
                const [audioData, err] = tts(chunk.tts);
                if (!err && audioData)
                    audio = audioData;
                pushContext('text', chunk.display, audio);
            }
        }
        catch (error) {
            const [promptSound, , , readErr] = readFile('audios/cartoon-fail.mp3');
            if (readErr)
                console.error('读取提示音失败:', readErr);
            console.error(error.message, ' || ', error.stack);
            pushContext('text', randomDefaultMessage(), promptSound);
            resetAgentState();
        }
        GlobalConfig.reasoningInProgress = false;
    }
    function writeMessage(role, content) {
        if (content && typeof content === 'object' && !Array.isArray(content) && (content.type === 'video_url' || content.type === 'audio_url'))
            content = [content];
        const incoming = { role, content };
        attachImageTags(incoming);
        GlobalConfig.unreadContext.push(incoming);
        const items = typeof content === 'string' ? [{ type: 'text', text: content }] : Array.isArray(content) ? content : [];
        for (const message of items) {
            if (message.type === 'text')
                console.log('收到文本: ' + message.text);
            else if (message.type === 'image_url')
                console.log('收到图片: ' + message.image_url?.url?.substring(0, 50));
            else if (message.type === 'input_audio')
                console.log('收到音频: ' + message.input_audio?.data?.substring(0, 30));
            else if (message.type === 'video_url')
                console.log('收到视频: ' + message.video_url?.url);
            else if (message.type === 'audio_url')
                console.log('收到音频: ' + message.audio_url?.url);
        }
    }
    function resetAgentState() {
        descriptionRole.coverContext([]);
        dialogueRole.coverContext([]);
        painterRole.coverContext([]);
        musicianRole.coverContext([]);
        perceiverRole.coverContext([]);
        memorizerRole.coverContext([]);
        painterRole.resetReference();
        GlobalConfig.unreadContext = [];
    }

    fetchDocumentCallback('lunar_config.json').then(content => GlobalConfig.customConfig = content);
    bindDumpBridge();
    setInterval(() => thoughtLoopTickEvent(), 1000);

    exports.LiteImageFile = LiteImageFile;
    exports.descriptionRole = descriptionRole;
    exports.ensureMemoryReady = ensureMemoryReady;
    exports.extractTextFromMessage = extractTextFromMessage;
    exports.memorizerRole = memorizerRole;
    exports.musicianRole = musicianRole;
    exports.painterRole = painterRole;
    exports.perceiverRole = perceiverRole;
    exports.randomDefaultMessage = randomDefaultMessage;

    return exports;

})({});
