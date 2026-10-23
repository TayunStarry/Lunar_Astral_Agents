var lunarBundle = (function (exports) {
    'use strict';

    class GlobalConfig {
        static customConfig = { agent: {} };
        static imageFormatsExtensions = ['.jpg', '.jpeg', '.png', '.gif', '.bmp', '.webp'];
        static videoFormatsExtensions = ['.mp4', '.avi', '.mov', '.wmv', '.flv', '.mkv'];
        static LTPfunction = new Map();
        static unreadRecords = [];
        static unreadContext = [];
        static reasoningInProgress = false;
        static fastAckSent = false;
        static turnFromQQ = false;
        static finalResponse = "";
        static lastFailure = "";
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
            return GlobalConfig.customConfig?.system?.developer ?? false;
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
    function admitImageDataURI(imageData, name, origin = 'client-upload') {
        if (!imageData)
            return null;
        if (imageData.startsWith('data:')) {
            return admitResource({ sourceKind: 'data', source: imageData, name, origin, kindHint: 'I', dedup: true });
        }
        const sourceKind = /^https?:\/\//i.test(imageData) ? 'url' : 'path';
        return admitResource({ sourceKind, source: imageData, name, origin, kindHint: 'I', dedup: true });
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
    function parseResourceQuery(raw) {
        let text = String(raw || '').trim();
        text = text.replace(/^[[【]/, '').replace(/[\]】]$/, '');
        const index = text.indexOf('&');
        if (index < 0)
            return { handle: text.trim(), ask: '' };
        return {
            handle: text.slice(0, index).trim(),
            ask: text.slice(index + 1).trim()
        };
    }
    function grammarCheatsheet() {
        return '句柄形如 @D.003（文档）、@I.007（图片）、@V.001（视频）、@A.002（音频）；'
            + '段落锚点可写 @D.003|§4（第 4 节）或 @D.003|L120-168（第 120-168 行）；'
            + '需要描述者探查时可写成 @D.003&自然语言需求';
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

    const LOST_FILE_NOTICE = '文件损毁或丢失,请更换句柄';
    const HANDLE_INVALID_NOTICE = '句柄无效,文件不存在';
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
            if (result.lost) {
                console.warn('[资源] 本体丢失/损坏，移除句柄:', formatHandle(parsed));
                deleteResource(formatHandle(parsed));
            }
            return result;
        }
        catch (error) {
            console.error('[资源] 取回抛异常:', error);
            return null;
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

    const IMAGE_WINDOW_SIZE = 20;
    const VISION_READ_PATH$1 = '/file/miniature/';
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
            if (lower.includes(VISION_READ_PATH$1))
                continue;
            const admitted = admitImageDataURI(source, `image${guessImageExt(source)}`);
            if (!admitted?.handle || !admitted.blob_path) {
                console.error('[图片标签] 入库失败，该图片已从思维链剔除（禁止原图流转）:', source.substring(0, 60));
                item.type = 'text';
                item.text = '[一张图片入库失败，已忽略]';
                delete item.image_url;
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
            return `${url()[0]}${VISION_READ_PATH$1}${blobPath}?tag=${encodeURIComponent(handle)}`;
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
        if (entry.state === 'lost') {
            deleteResource(normalized);
            return null;
        }
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
                            description: "绘画需求描述，如'画一只在樱花树下的白猫'、'把 @I.003 画成雪景'。描述越详细，绘画效果越好。以第一人称书写时，「我」「我的」指月华本人（自画像），「我的妹妹」指琉璃。"
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
        },
        {
            type: "function",
            function: {
                name: "dispatch_screenshoter",
                description: "把截图诉求交给截图者子智能体。截图者理解自然语言要求并精确取景，支持全屏、指定显示器、指定窗口（含被其他窗口遮挡但未最小化的窗口）、窗口内相对区域、绝对屏幕坐标区域；截图会直接展示给用户，同时把主图回传给你查看。用户要求看屏幕、截某个窗口、截某块区域时调用它。",
                parameters: {
                    type: "object",
                    properties: {
                        requirement: {
                            type: "string",
                            description: "截图要求，保留用户原话里的关键细节：截哪个窗口（标题/应用名）、截哪块区域（上下左右/坐标）、全屏还是单屏。例如「截一下 Chrome 窗口」「截屏幕右下角任务栏」「把微信聊天窗口上半部分截给我」"
                        }
                    },
                    required: ["requirement"]
                }
            }
        }
    ];
    function parseArgs$2(args) {
        return typeof args === 'string' ? JSON.parse(args) : (args || {});
    }
    async function handleDispatchPainter(args) {
        const { description, reference_tag } = parseArgs$2(args);
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
                return [`绘画任务调度失败：图片句柄 ${reference_tag} 不存在，请用 browse_resources（kind="I"）查看可引用的图片句柄后重试`, ''];
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
        const { description } = parseArgs$2(args);
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
    async function handleDispatchScreenshoter(args) {
        const { requirement } = parseArgs$2(args);
        if (!requirement || typeof requirement !== 'string' || requirement.trim().length === 0) {
            return ['截图任务调度失败：截图要求不能为空，请说明要截哪个窗口或哪块区域', ''];
        }
        if (!agentTargets?.screenshoterRole) {
            return ['截图任务调度失败：截图者子智能体未就绪，请稍后重试', ''];
        }
        console.log(`[智能体控制] 调度截图者: ${requirement.trim()}`);
        const result = await agentTargets.screenshoterRole.createCreativeWork(requirement.trim());
        const image = agentTargets.screenshoterRole.takePrimaryImage();
        console.log(`[智能体控制] 截图者完成: ${result}${image ? '（含主图）' : ''}`);
        return [result, image];
    }
    GlobalConfig.LTPfunction.set('dispatch_painter', handleDispatchPainter);
    GlobalConfig.LTPfunction.set('dispatch_musician', handleDispatchMusician);
    GlobalConfig.LTPfunction.set('dispatch_screenshoter', handleDispatchScreenshoter);

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

    const SCHEDULE_TRIGGER_PREFIX = '[计划提醒]';
    const SCHEDULE_OVERDUE_ABANDON_MS = 30 * 60 * 1000;
    const PRESET_DAILY_TASKS = [
        { id: 'daily_greeting_0630', type: 'daily', time: '07:00', content: '向用户发送清晨早安问候，关心其今日安排' },
        { id: 'daily_greeting_1000', type: 'daily', time: '10:00', content: '向用户发送"上午好，该喝水啦"的补水提醒' },
        { id: 'daily_greeting_1200', type: 'daily', time: '12:00', content: '向用户发送午安问候，关心其午餐情况' },
        { id: 'daily_greeting_1500', type: 'daily', time: '15:00', content: '向用户发送"下午好，该喝水啦"的补水提醒' },
        { id: 'daily_greeting_1730', type: 'daily', time: '18:00', content: '向用户发送傍晚问候，关心一天收尾与晚间安排' },
        { id: 'daily_greeting_2230', type: 'daily', time: '22:00', content: '向用户发送晚安问候，祝其好眠' },
    ];
    const SCHEDULE_FILE_PATH = 'database/schedule.json';
    const scheduleTools = [
        {
            type: "function",
            function: {
                name: "schedule",
                description: "计划表综合工具：创建、编辑、删除与查询计划项。查询支持关键词数组（内容/时间混合检索，多条结果按相关度排序）与时间范围（某天/某月/某年，查某天不填日期默认今天）；可按内容查时间、按时间查内容、按关键词查完整计划。",
                parameters: {
                    type: "object",
                    properties: {
                        action: {
                            type: "string",
                            enum: ["create", "edit", "delete", "query"],
                            description: "要执行的动作：create 创建计划 / edit 编辑计划 / delete 删除计划 / query 查询计划"
                        },
                        id: {
                            type: "string",
                            description: "计划项ID（edit/delete 必填），从 action=query 的返回结果中获得"
                        },
                        time: {
                            type: "string",
                            description: "计划执行时间点（create 必填；edit 选填）。支持 ISO 8601（'2026-06-14T15:30:00'）或中文格式（'2026年6月14日 15:30'）"
                        },
                        content: {
                            type: "string",
                            description: "计划工作内容（create 必填；edit 选填，提供时覆盖原内容）"
                        },
                        keywords: {
                            type: "array",
                            items: { type: "string" },
                            description: "查询关键词数组（query 选填）：同时检索计划内容与时间，命中越多排序越靠前；可传单个关键词；不传则返回范围内全部计划"
                        },
                        date: {
                            type: "string",
                            description: "查询日期范围（query 选填）：'2026-06-14' 查某天 / '2026-06' 查某月 / '2026' 查某年；支持中文（'6月14日'）与相对词（今天/明天/昨天）。查某天计划不填时默认今天"
                        },
                        scope: {
                            type: "string",
                            enum: ["day", "month", "year", "all"],
                            description: "查询粒度（query 选填）：day 某天 / month 某月 / year 某年 / all 全部计划；缺省时按 date 格式推断，date 与 keywords 都未提供时也可用 all 列出全部计划"
                        }
                    },
                    required: ["action"]
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
    function dayRange(base, labelSuffix = '') {
        const start = new Date(base.getFullYear(), base.getMonth(), base.getDate());
        const end = new Date(base.getFullYear(), base.getMonth(), base.getDate(), 23, 59, 59, 999);
        return { start, end, label: `${formatDate(base)}${labelSuffix}` };
    }
    function monthRange(year, month, label = '') {
        return {
            start: new Date(year, month - 1, 1),
            end: new Date(year, month, 0, 23, 59, 59, 999),
            label: label || `${year}年${month}月`,
        };
    }
    function yearRange(year, label = '') {
        return {
            start: new Date(year, 0, 1),
            end: new Date(year, 11, 31, 23, 59, 59, 999),
            label: label || `${year}年`,
        };
    }
    const RELATIVE_DAYS = { '今天': 0, '今日': 0, '明天': 1, '明日': 1, '后天': 2, '昨天': -1, '昨日': -1 };
    function toHalfWidth(raw) {
        return raw.replace(/[０-９]/g, ch => String.fromCharCode(ch.charCodeAt(0) - 0xFEE0)).replace(/[－—–]/g, '-');
    }
    function parseDateRange(raw, scope) {
        const now = new Date();
        const normalized = toHalfWidth(String(raw || '').trim());
        const effectiveScope = ['day', 'month', 'year', 'all'].includes(scope) ? scope : undefined;
        if (normalized && normalized in RELATIVE_DAYS) {
            const base = new Date(now.getFullYear(), now.getMonth(), now.getDate() + RELATIVE_DAYS[normalized]);
            const suffix = normalized === '今天' ? '（今天）' : `（${normalized}）`;
            return { range: dayRange(base, suffix), error: null };
        }
        if (!normalized) {
            if (effectiveScope === 'month')
                return { range: monthRange(now.getFullYear(), now.getMonth() + 1), error: null };
            if (effectiveScope === 'year')
                return { range: yearRange(now.getFullYear()), error: null };
            if (effectiveScope === 'all')
                return { range: null, error: null };
            return { range: dayRange(now, '（今天）'), error: null };
        }
        let match = normalized.match(/^(\d{4})[年\-/.](\d{1,2})[月\-/.](\d{1,2})日?$/);
        if (match) {
            const [, y, m, d] = match;
            const date = new Date(Number(y), Number(m) - 1, Number(d));
            if (isNaN(date.getTime()))
                return { range: null, error: `无法识别日期 "${raw}"` };
            return { range: dayRange(date), error: null };
        }
        match = normalized.match(/^(\d{4})[年\-/.](\d{1,2})月?$/);
        if (match) {
            const [, y, m] = match;
            return { range: monthRange(Number(y), Number(m)), error: null };
        }
        match = normalized.match(/^(\d{1,2})月(\d{1,2})日?$/);
        if (match) {
            const [, m, d] = match;
            return { range: dayRange(new Date(now.getFullYear(), Number(m) - 1, Number(d))), error: null };
        }
        match = normalized.match(/^(\d{1,2})月$/);
        if (match) {
            return { range: monthRange(now.getFullYear(), Number(match[1])), error: null };
        }
        match = normalized.match(/^(\d{4})年?$/);
        if (match) {
            return { range: yearRange(Number(match[1])), error: null };
        }
        const std = new Date(normalized);
        if (!isNaN(std.getTime()))
            return { range: dayRange(std), error: null };
        return { range: null, error: `无法识别日期 "${raw}"，请使用如 "2026-06-14"、"2026-06"、"2026" 或 "今天"` };
    }

    function loadSchedulesFromDisk() {
        const [fileData, readErr] = readTextFile(SCHEDULE_FILE_PATH);
        if (readErr || !fileData) {
            return [];
        }
        try {
            const data = JSON.parse(fileData);
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

    const CONTENT_HIT_SCORE = 2;
    const TIME_HIT_SCORE = 1;
    function formatLine(item, index) {
        const time = item.type === 'daily' ? `每日 ${item.time}` : item.time;
        return `[计划项${index + 1}] ID:${item.id} | 时间:${time} | 内容:${item.content}`;
    }
    function sortKey(item, now) {
        if (item.type === 'daily')
            return dailyTaskTime(item, now)?.getTime() ?? Number.MAX_SAFE_INTEGER;
        const t = new Date(item.time).getTime();
        return isNaN(t) ? Number.MAX_SAFE_INTEGER : t;
    }
    function scoreItem(item, keywords) {
        const content = item.content.toLowerCase();
        let score = 0;
        for (const keyword of keywords) {
            const kw = keyword.toLowerCase();
            if (!kw)
                continue;
            if (content.includes(kw))
                score += CONTENT_HIT_SCORE;
            else if (item.time.includes(keyword))
                score += TIME_HIT_SCORE;
        }
        return score;
    }
    function inRange(item, start, end, now) {
        if (item.type === 'daily')
            return true;
        const t = new Date(item.time).getTime();
        if (isNaN(t))
            return false;
        return t >= start.getTime() && t <= end.getTime();
    }
    function querySchedules(cache, keywords, date, scope) {
        const rawList = Array.isArray(keywords) ? keywords : (keywords ? [keywords] : []);
        const keywordList = rawList.map(k => String(k ?? '').trim()).filter(k => k.length > 0);
        const { range, error } = parseDateRange(date === undefined || date === null ? '' : String(date), scope === undefined || scope === null ? undefined : String(scope));
        if (error)
            return [`查询计划失败：${error}`, ''];
        const now = new Date();
        const candidates = range
            ? cache.filter(item => inRange(item, range.start, range.end))
            : [...cache];
        let matched;
        if (keywordList.length > 0) {
            matched = candidates
                .map(item => ({ item, score: scoreItem(item, keywordList) }))
                .filter(entry => entry.score > 0)
                .sort((a, b) => b.score - a.score ||
                sortKey(a.item, now) - sortKey(b.item, now));
        }
        else {
            matched = candidates
                .map(item => ({ item, score: 0 }))
                .sort((a, b) => sortKey(a.item, now) - sortKey(b.item, now));
        }
        if (matched.length === 0) {
            if (candidates.length === 0) {
                const scopeText = range ? `${range.label}没有安排任何计划` : '当前计划表为空';
                return [`${scopeText}，没有任何计划项。`, ''];
            }
            const scopeText = range ? `${range.label}` : '计划表';
            return [`${scopeText}共有 ${candidates.length} 个计划项，但没有命中关键词「${keywordList.join('、')}」的计划。`, ''];
        }
        const parts = [];
        parts.push(range
            ? `${range.label}共有 ${candidates.length} 个计划项` + (keywordList.length ? `，命中「${keywordList.join('、')}」的有 ${matched.length} 条（按相关度排序）` : '（按时间排序）')
            : `当前计划表共有 ${candidates.length} 个计划项` + (keywordList.length ? `，命中「${keywordList.join('、')}」的有 ${matched.length} 条（按相关度排序）` : ''));
        matched.forEach((entry, i) => parts.push(formatLine(entry.item, i)));
        return [parts.join('\n'), ''];
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
    function parseArgs$1(args) {
        return typeof args === 'string' ? JSON.parse(args) : (args || {});
    }
    async function handleCreate(args) {
        const { time, content } = parseArgs$1(args);
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
    async function handleEdit(args) {
        const { id, time, content } = parseArgs$1(args);
        if (!id || id.trim().length === 0) {
            return ['编辑计划项失败：计划项ID不能为空，请先用 action=query 查询获取有效ID', ''];
        }
        if ((!time || time.trim().length === 0) && (!content || content.trim().length === 0)) {
            return ['编辑计划项失败：至少需要提供 time 或 content 中的一个进行修改', ''];
        }
        const index = scheduleCache.findIndex(item => item.id === id.trim());
        if (index === -1) {
            return [`编辑计划项失败：未找到ID为 ${id} 的计划项，请先用 action=query 查询确认正确的ID`, ''];
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
    async function handleDelete(args) {
        const { id } = parseArgs$1(args);
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
    async function handleSchedule(args) {
        const { action } = parseArgs$1(args);
        switch (String(action || '').trim()) {
            case 'create': return handleCreate(args);
            case 'edit': return handleEdit(args);
            case 'delete': return handleDelete(args);
            case 'query': {
                const { keywords, date, scope } = parseArgs$1(args);
                return querySchedules(scheduleCache, keywords, date, scope);
            }
            default:
                return [`计划表工具调用失败：未知的动作 "${action}"，action 必须是 create / edit / delete / query 之一`, ''];
        }
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
    function flushDueScheduleItems() {
        const due = checkDueItems();
        for (const item of due) {
            const feedback = interactEvent('execution_schedule_before', { plan: item.content }).return;
            const content = (feedback && typeof feedback.plan === 'string') ? feedback.plan : item.content;
            GlobalConfig.unreadContext.push({ role: 'user', content: `${SCHEDULE_TRIGGER_PREFIX} 预约时间已到，请执行以下计划：${content}` });
        }
        return due;
    }
    initSchedules();
    GlobalConfig.LTPfunction.set('schedule', handleSchedule);

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
    function shrinkContent$1(content, includeImages) {
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
    function exportMessages$1(messages, includeImages) {
        return messages.map(message => ({
            role: message.role,
            content: shrinkContent$1(message.content, includeImages),
            tool_call_id: message.tool_call_id || undefined,
            tool_calls: message.tool_calls || undefined,
        }));
    }
    function snapshotAgent(agent, includeImages) {
        if (agent === 'lunar') {
            return {
                target: 'lunar',
                label: '月华主上下文（未读队列 + 长期记忆摘要）',
                messages: exportMessages$1(GlobalConfig.unreadContext, includeImages),
            };
        }
        const entry = roleRegistry[agent];
        return {
            target: agent,
            label: entry.label,
            messages: exportMessages$1(entry.role.messages, includeImages),
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

    const RECOVERY_VERSION = 1;
    const IMAGE_PLACEHOLDER = '[图片：引擎重载时已省略]';
    let recoveryRoles = null;
    function registerRecoveryRoles(roles) {
        recoveryRoles = roles;
    }
    function shrinkContent(content) {
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
            return { type: 'image_url', image_url: { url: IMAGE_PLACEHOLDER } };
        });
    }
    function exportMessages(messages) {
        return messages.map(message => ({
            role: message.role,
            content: shrinkContent(message.content),
            tool_call_id: message.tool_call_id || undefined,
            tool_calls: message.tool_calls || undefined,
        }));
    }
    function importMessages(raw) {
        if (!Array.isArray(raw))
            return [];
        const out = [];
        for (const item of raw) {
            if (!item || typeof item !== 'object')
                continue;
            const row = item;
            if (!row.role)
                continue;
            const message = {
                role: row.role,
                content: row.content === undefined ? '' : row.content,
                tool_call_id: row.tool_call_id,
                tool_calls: row.tool_calls,
            };
            out.push(message);
        }
        return out;
    }
    function snapshotAgentContext() {
        try {
            const dialogue = recoveryRoles ? exportMessages(recoveryRoles.dialogue.messages) : [];
            const payload = {
                kind: 'lunar-agent-recovery',
                version: RECOVERY_VERSION,
                at: new Date().toISOString(),
                unread: exportMessages(GlobalConfig.unreadContext),
                dialogue: dialogue,
            };
            return JSON.stringify(payload);
        }
        catch (error) {
            console.error('[引擎恢复] 生成上下文快照失败:', error?.message ?? error);
            return '';
        }
    }
    function restoreAgentContext(jsonText) {
        GlobalConfig.reasoningInProgress = false;
        GlobalConfig.lastFailure = '';
        if (!jsonText || !jsonText.trim())
            return '没有可恢复的上下文（快照为空）';
        let payload;
        try {
            payload = JSON.parse(jsonText);
        }
        catch (error) {
            return `快照解析失败，未恢复上下文: ${error?.message ?? error}`;
        }
        if (payload?.kind !== 'lunar-agent-recovery' || payload?.version !== RECOVERY_VERSION) {
            return `快照版本不符（期望 v${RECOVERY_VERSION}），未恢复上下文`;
        }
        const unread = importMessages(payload.unread);
        const dialogue = importMessages(payload.dialogue);
        if (unread.length > 0)
            GlobalConfig.unreadContext = [...unread, ...GlobalConfig.unreadContext];
        if (dialogue.length > 0 && recoveryRoles)
            recoveryRoles.dialogue.coverContext(dialogue);
        return `已恢复 ${dialogue.length} 条对话上下文、${unread.length} 条未读消息`;
    }

    const CHARACTER_TABLE = 'painter_characters';
    function loadCustomCharacters() {
        const characters = new Map();
        try {
            const [entries, error] = knowledgeLoad(CHARACTER_TABLE);
            if (error) {
                console.error(`[角色库] 读取 ${CHARACTER_TABLE} 失败:`, error.message ?? String(error));
                return characters;
            }
            for (const pair of entries ?? []) {
                if (!Array.isArray(pair) || typeof pair[0] !== 'string' || !pair[0])
                    continue;
                const name = pair[0];
                try {
                    const raw = JSON.parse(String(pair[1] ?? ''));
                    const profile = normalizeProfile(raw, name);
                    if (profile)
                        characters.set(profile.name, profile);
                }
                catch (parseError) {
                    console.warn(`[角色库] 角色「${name}」的档案不是合法 JSON，已跳过:`, parseError);
                }
            }
        }
        catch (error) {
            console.error('[角色库] 读取自定义角色失败:', error);
        }
        return characters;
    }
    function saveCustomCharacter(profile) {
        try {
            const characters = loadCustomCharacters();
            characters.set(profile.name, profile);
            const entries = [...characters.entries()]
                .map(([name, value]) => [name, JSON.stringify(value)]);
            const [ok, error] = knowledgeSave(CHARACTER_TABLE, entries);
            if (error) {
                console.error(`[角色库] 写入 ${CHARACTER_TABLE} 失败:`, error.message ?? String(error));
                return false;
            }
            return !!ok;
        }
        catch (error) {
            console.error('[角色库] 保存自定义角色失败:', error);
            return false;
        }
    }
    function normalizeProfile(raw, fallbackName) {
        if (!raw || typeof raw !== 'object')
            return null;
        const name = String(raw.name || fallbackName).trim();
        const appearance = String(raw.appearance || '').trim();
        if (!name || !appearance)
            return null;
        const gender = raw.gender === '男' ? '男' : '女';
        return {
            name,
            gender,
            appearance,
            outfitFull: String(raw.outfitFull || '').trim() || undefined,
            outfitChibi: String(raw.outfitChibi || '').trim() || undefined,
        };
    }

    class ModelBuilder {
        roleTag = "月华";
        enableTools = true;
        reasoningStrength = "low";
        disableThinking = false;
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
            if ('reasoning_content' in message || 'source' in message) {
                const { reasoning_content, source, ...rest } = message;
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
                    reasoning_strength: this.reasoningStrength,
                    ...(this.disableThinking ? { enable_thinking: false } : {})
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

    const RATIO_PARAM = {
        type: "string",
        enum: ["1:1", "4:3", "3:4", "16:9", "9:16", "3:2", "2:3"],
        description: "画面比例预设。人物/物品用 2:3 或 3:4，风景/横构图用 3:2 或 16:9，方形构图用 1:1"
    };
    const STEPS_PARAM = {
        type: "number",
        description: "迭代步数，取值 8~60：quality 方案建议 30~60（常规 40，精细 50~60）；turbo 方案建议 8~12。省略时使用当前模型方案的推荐步数。越高细节越好但耗时线性增加，不影响构图"
    };
    const NEGATIVE_PROMPT_PARAM = {
        type: "string",
        description: "负面提示文本，用于排除不希望出现的元素"
    };
    const CFG_SCALE_PARAM = {
        type: "number",
        description: "提示词引导系数（0~2），默认 1.0。当前为引导蒸馏模型，保持 1.0；调高会发灰、过曝、出现塑料感"
    };
    const SUPER_RESOLUTION_PARAM = {
        type: "boolean",
        description: "生成完成后追加 RealESRGAN 超分辨率重建（4x），提升清晰度与细节。用户要求高清、精细画质时开启，耗时增加"
    };

    const generationTools = [
        {
            type: "function",
            function: {
                name: "diffusion_generation",
                description: "根据文本描述进行文生图。仅适用于风景、物品、场景、氛围图等与人物形象无关的创作；严禁用于绘制任何人物形象(月华、琉璃或其他人物都必须调用 portrait)。",
                parameters: {
                    type: "object",
                    properties: {
                        "prompt": {
                            type: "string",
                            description: "图像生成的正向描述文本"
                        },
                        "ratio": {
                            ...RATIO_PARAM,
                            description: RATIO_PARAM.description + "；缺省 1:1"
                        },
                        "steps": STEPS_PARAM,
                        "negative_prompt": NEGATIVE_PROMPT_PARAM,
                        "cfg_scale": CFG_SCALE_PARAM,
                        "super_resolution": SUPER_RESOLUTION_PARAM
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
                description: "基于参考图进行图生图创作:以参考图为底图,按文本描述改变风格、场景、服饰或细节,同时保留参考图的主体特征。无参考图时不可调用:人物形象改用 portrait,非人物改用 diffusion_generation,或说明需要用户提供参考图片。",
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
                        "steps": STEPS_PARAM,
                        "negative_prompt": {
                            ...NEGATIVE_PROMPT_PARAM,
                            description: NEGATIVE_PROMPT_PARAM.description + "（换场景时点名去掉旧场景元素，如 beach, ocean, water, sea）"
                        },
                        "cfg_scale": {
                            ...CFG_SCALE_PARAM,
                            description: CFG_SCALE_PARAM.description + "。需要改得更多时调 strength，不要调 cfg"
                        },
                        "super_resolution": SUPER_RESOLUTION_PARAM
                    },
                    required: [
                        "prompt"
                    ]
                }
            }
        }
    ];

    const portraitTools = [
        {
            type: "function",
            function: {
                name: "portrait",
                description: "生成指定角色的立绘。凡是要求绘制月华、琉璃或其他人物的形象时,都必须且只能调用本函数,禁止用 diffusion_generation 或 image_to_image 代替",
                parameters: {
                    type: "object",
                    properties: {
                        "character": {
                            type: "string",
                            description: "目标角色:内置角色(月华/琉璃)或已创建的自定义角色名,已内置外观,只需给出表情/姿势/环境;通用-女 / 通用-男 或临时角色必须用 appearance 描述其外观。{characters}"
                        },
                        "chibi": {
                            type: "boolean",
                            description: "是否Q版:true=二头身萌系比例(贴纸/表情包/头像),false 或不填=标准头身比的全身立绘"
                        },
                        "appearance": {
                            type: "string",
                            description: "character=通用-女 / 通用-男 或临时角色时填写该人物的外观:性别、年龄感、发型发色、瞳色、服装、配饰、身份。内置/自定义角色不需要填写。"
                        },
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
                            description: "服装提示词。不提供则用该角色的默认服装（通用角色仅在 appearance 未写服装时兜底）"
                        },
                        "environment": {
                            type: "string",
                            description: "环境提示词,描述背景或场景。月华偏暗色与红,琉璃偏淡蓝天空与星光"
                        },
                        "ratio": {
                            ...RATIO_PARAM,
                            description: RATIO_PARAM.description + "；缺省：全尺寸 2:3、Q 版 1:1"
                        },
                        "steps": STEPS_PARAM,
                        "negative_prompt": NEGATIVE_PROMPT_PARAM,
                        "cfg_scale": CFG_SCALE_PARAM,
                        "super_resolution": SUPER_RESOLUTION_PARAM
                    },
                    required: [
                        "character",
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
                name: "create_character",
                description: "创建或更新一个可复用的自定义角色,写入角色库(SQL数据库)。适合会反复出场的原创角色:创建后 portrait 的 character 与 group_photo 的 roles 都能直接用角色名引用,无需每次重写外观。一次性路人不建议创建。内置角色(月华/琉璃)不能重名",
                parameters: {
                    type: "object",
                    properties: {
                        "name": {
                            type: "string",
                            description: "角色名(唯一键,后续绘制时用这个名字引用)"
                        },
                        "gender": {
                            type: "string",
                            enum: ["女", "男"],
                            description: "性别,决定立绘模板中的性别表述"
                        },
                        "appearance": {
                            type: "string",
                            description: "基础外观:年龄感、发型发色、瞳色、面部特征、配饰、体型。这是角色的身份定义,不写表情/动作/场景"
                        },
                        "outfit_full": {
                            type: "string",
                            description: "默认全尺寸服装。省略则使用简洁得体的日常服装"
                        },
                        "outfit_chibi": {
                            type: "string",
                            description: "默认Q版服装。省略则复用全尺寸服装"
                        }
                    },
                    required: [
                        "name",
                        "gender",
                        "appearance"
                    ]
                }
            }
        }
    ];

    const groupTools = [
        {
            type: "function",
            function: {
                name: "group_photo",
                description: "生成月华与一个或多个角色的合照。roles 按顺序列出参与者:内置角色与已创建的自定义角色直接写名字,自动继承外观与默认服装;临时角色写完整外观描述(性别、年龄感、发型发色、瞳色、服装、配饰、身份)。chibi=true 时所有参与者使用Q版二头身比例。planning 统一规划每人的表情/动作/服饰,例如\"月华:俏皮眨眼,比V手势,黑色哥特长裙;琉璃:安静微笑,双手交叠,白色星纹裙\";省略则每人使用随机默认表情/动作与默认服装。environment 省略时从预设背景中随机挑选。若本次任务附带参考图,将以该参考图为底图进行图生图,尽量保留参与者的容貌;未附带则按文本描述绘制",
                parameters: {
                    type: "object",
                    properties: {
                        "roles": {
                            type: "array",
                            items: {
                                type: "string"
                            },
                            description: "合照参与者列表,至少一位。内置角色与已创建的自定义角色直接写名字(自动继承外观与默认服装);临时角色写完整外观描述。{characters}"
                        },
                        "chibi": {
                            type: "boolean",
                            description: "是否Q版:true=所有参与者使用Q版二头身比例,false 或不填=全尺寸合照"
                        },
                        "planning": {
                            type: "string",
                            description: "合照的统一人物规划:逐人写清表情、动作与服饰,用分号分隔。省略则全部使用默认表情/动作与默认服装"
                        },
                        "environment": {
                            type: "string",
                            description: "环境提示词,描述背景环境或场景。省略时随机使用预设背景"
                        },
                        "ratio": {
                            ...RATIO_PARAM,
                            description: RATIO_PARAM.description + "；缺省 1:1，已选定参考图时不生效（输出尺寸随参考图比例）"
                        },
                        "strength": {
                            type: "number",
                            description: "已选定参考图时的图生图强度,取值 0 到 1,越大越偏离参考图。保留参与者容貌建议 0.3~0.5;缺省按中性值 0.5。未选定参考图时忽略"
                        },
                        "steps": STEPS_PARAM,
                        "negative_prompt": NEGATIVE_PROMPT_PARAM,
                        "cfg_scale": CFG_SCALE_PARAM,
                        "super_resolution": SUPER_RESOLUTION_PARAM
                    },
                    required: [
                        "roles"
                    ]
                }
            }
        }
    ];

    const runtimeTools = [
        {
            type: "function",
            function: {
                name: "switch_diffusion_scheme",
                description: "切换后续绘图使用的模型方案。quality=质量优先(细节更好、耗时更长);turbo=速度优先(出图更快,适合草图、快速确认与批量尝试,推荐 8~12 步)。切换后省略 steps 即使用该方案的推荐步数。切换状态由宿主记录,对所有后续绘图工具生效;任务本身不需要画得快时不要切换,保持当前方案即可",
                parameters: {
                    type: "object",
                    properties: {
                        "scheme": {
                            type: "string",
                            enum: ["turbo", "quality"],
                            description: "目标方案:turbo=速度优先,quality=质量优先"
                        }
                    },
                    required: [
                        "scheme"
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

    const painterTools = [
        ...generationTools,
        ...portraitTools,
        ...groupTools,
        ...runtimeTools,
    ];

    function RandomFloor(min, max) {
        return Math.floor(Math.random() * (max - min + 1) + min);
    }
    function RandomFloat(min, max, length = 2) {
        return Number((Math.random() * (max - min) + min).toFixed(length));
    }

    class PromptDocument {
        file;
        sections = new Map();
        constructor(text, file) {
            this.file = file;
            let stack = [];
            let currentPath = '';
            for (const line of text.split(/\r?\n/)) {
                const heading = /^(#{1,6})\s+(.+?)\s*$/.exec(line);
                if (heading) {
                    const level = heading[1].length;
                    const title = heading[2].trim();
                    stack = stack.filter(item => item.level < level);
                    stack.push({ level, title });
                    currentPath = stack.filter(item => item.level >= 2).map(item => item.title).join('/');
                    if (currentPath && !this.sections.has(currentPath))
                        this.sections.set(currentPath, '');
                    continue;
                }
                if (!currentPath)
                    continue;
                this.sections.set(currentPath, (this.sections.get(currentPath) || '') + line + '\n');
            }
        }
        text(path) {
            const body = this.sections.get(path);
            if (body === undefined) {
                throw new Error(`提示词文件 ${this.file} 缺少标题节「${path}」`);
            }
            const trimmed = body.trim();
            if (!trimmed) {
                throw new Error(`提示词文件 ${this.file} 的标题节「${path}」内容为空`);
            }
            return trimmed;
        }
        list(path) {
            const body = this.text(path);
            const items = body
                .split(/\r?\n/)
                .map(line => line.trim())
                .filter(line => /^[-*]\s+/.test(line))
                .map(line => line.replace(/^[-*]\s+/, '').trim())
                .filter(item => item.length > 0);
            if (items.length === 0) {
                throw new Error(`提示词文件 ${this.file} 的标题节「${path}」没有任何列表项`);
            }
            return items;
        }
    }
    function loadPromptDocument(file) {
        const [content, error] = fileView(file);
        if (error)
            throw new Error(`读取提示词 ${file} 失败: ${error.message ?? String(error)}`);
        if (!content)
            throw new Error(`提示词 ${file} 内容为空`);
        return new PromptDocument(content, file);
    }

    let characterPromptsDocument = null;
    function loadCharacterPrompts() {
        if (!characterPromptsDocument)
            characterPromptsDocument = loadPromptDocument('prompts/painterCharacters.md');
        return characterPromptsDocument;
    }
    class PainterPromptLibrary {
        commonPrompts = loadPromptDocument('prompts/painterCommon.md');
        characterPrompts = loadCharacterPrompts();
        defaultExpressionPrompt = this.commonPrompts.list('默认表情');
        defaultPosturePrompt = this.commonPrompts.list('默认姿势');
        appearanceBasePrompt = this.characterPrompts.text('月华/基础外观');
        chibiAppearancePrompt = this.characterPrompts.text('月华/立绘-Q版');
        fullAppearancePrompt = this.characterPrompts.text('月华/立绘-全尺寸');
        crystalBasePrompt = this.characterPrompts.text('琉璃/基础外观');
        crystalFullAppearancePrompt = this.characterPrompts.text('琉璃/立绘-全尺寸');
        crystalChibiAppearancePrompt = this.characterPrompts.text('琉璃/立绘-Q版');
        portraitBaseFemalePrompt = this.characterPrompts.text('通用角色/底板-女');
        portraitBaseMalePrompt = this.characterPrompts.text('通用角色/底板-男');
        genericFullAppearancePrompt = this.characterPrompts.text('通用角色/立绘-全尺寸');
        genericChibiAppearancePrompt = this.characterPrompts.text('通用角色/立绘-Q版');
        defaultOutfitChibiPrompt = this.characterPrompts.text('月华/默认服装-Q版');
        defaultOutfitFullPrompt = this.characterPrompts.text('月华/默认服装-全尺寸');
        crystalDefaultOutfitFullPrompt = this.characterPrompts.text('琉璃/默认服装-全尺寸');
        crystalDefaultOutfitChibiPrompt = this.characterPrompts.text('琉璃/默认服装-Q版');
        groupPhotoTemplate = this.commonPrompts.text('合照模板');
        groupEnvironmentPresets = this.commonPrompts.list('合照背景');
        defaultNegativePrompt = this.commonPrompts.text('默认负面提示词');
        builtinCharacters = new Map([
            ['月华', {
                    name: '月华', gender: '女', appearance: this.appearanceBasePrompt,
                    outfitFull: this.defaultOutfitFullPrompt, outfitChibi: this.defaultOutfitChibiPrompt, builtin: true,
                }],
            ['琉璃', {
                    name: '琉璃', gender: '女', appearance: this.crystalBasePrompt,
                    outfitFull: this.crystalDefaultOutfitFullPrompt, outfitChibi: this.crystalDefaultOutfitChibiPrompt, builtin: true,
                }],
        ]);
        get templates() {
            return {
                chibiSelf: this.chibiAppearancePrompt,
                fullSelf: this.fullAppearancePrompt,
                crystalFull: this.crystalFullAppearancePrompt,
                crystalChibi: this.crystalChibiAppearancePrompt,
                genericFull: this.genericFullAppearancePrompt,
                genericChibi: this.genericChibiAppearancePrompt,
                femaleBase: this.portraitBaseFemalePrompt,
                maleBase: this.portraitBaseMalePrompt,
                defaultOutfitChibi: this.defaultOutfitChibiPrompt,
                defaultOutfitFull: this.defaultOutfitFullPrompt,
                crystalOutfitFull: this.crystalDefaultOutfitFullPrompt,
                crystalOutfitChibi: this.crystalDefaultOutfitChibiPrompt,
                defaultNegative: this.defaultNegativePrompt,
                groupPhoto: this.groupPhotoTemplate,
            };
        }
        renderAppearance(template, parts, defaultOutfit, base, gender) {
            const expression = parts.expression || this.randomExpression();
            const posture = parts.posture || this.randomPosture();
            const outfit = parts.outfit || defaultOutfit;
            return this.renderTemplate(template, {
                base: base || this.appearanceBasePrompt,
                gender: gender || '女孩',
                expression,
                outfit,
                posture,
                environment: parts.environment || '',
            });
        }
        renderTemplate(template, vars) {
            let filled = template;
            for (const [key, value] of Object.entries(vars)) {
                filled = filled.split(`{${key}}`).join(value);
            }
            return this.tidyPrompt(filled);
        }
        tidyPrompt(text) {
            return text
                .replace(/[，,]\s*[，,]+/g, '，')
                .replace(/[，,]\s*。/g, '。')
                .replace(/。\s*。/g, '。')
                .replace(/\s{2,}/g, ' ')
                .trim();
        }
        randomExpression() {
            return this.defaultExpressionPrompt[RandomFloor(0, this.defaultExpressionPrompt.length - 1)];
        }
        randomPosture() {
            return this.defaultPosturePrompt[RandomFloor(0, this.defaultPosturePrompt.length - 1)];
        }
        randomGroupEnvironment() {
            return this.groupEnvironmentPresets[RandomFloor(0, this.groupEnvironmentPresets.length - 1)];
        }
    }

    const SIDE_BASE = 1024;
    const SIDE_MAX = 2816;
    const SIDE_ALIGN = 32;
    const REF_SIDE_MIN = 512;
    const DEFAULT_PIXEL_BUDGET_MP = 1.6;
    const PIXEL_BUDGET_SLACK = 1.15;
    const DEFAULT_RATIO = '1:1';
    const LIST_LIMIT_MAX = 100;
    const DEFAULT_STRENGTH = 0.5;
    const STEPS_MIN = 8;
    const STEPS_MAX = 60;
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
            console.log(`[绘制者] ${label}未指定步数，按当前方案推荐步数处理`);
            return 0;
        }
        const steps = Math.round(Math.min(Math.max(num, STEPS_MIN), STEPS_MAX));
        if (steps !== Math.round(num))
            console.log(`[绘制者] ${label}步数 ${num} 越界，已收进 ${steps}`);
        return steps;
    }

    function mergeProfiles(builtin, custom) {
        const profiles = new Map(builtin);
        for (const [name, profile] of custom) {
            profiles.set(name, { ...profile, builtin: false });
        }
        return profiles;
    }
    function characterLabel(character) {
        const name = String(character ?? '').trim();
        switch (name) {
            case '月华': return '月华';
            case '琉璃': return '琉璃';
            case '通用-男': return '男性角色';
            case '通用-女': return '女性角色';
            default: return name || '角色';
        }
    }
    function characterCatalogText(profiles) {
        const names = [...profiles.keys()];
        return names.length > 0
            ? `当前可用角色：${names.join('、')}；也可用 通用-女 / 通用-男 临时描述外观`
            : '当前可用角色：通用-女 / 通用-男（临时描述外观）';
    }
    function characterOutfit(profile, chibi) {
        const outfit = chibi
            ? (profile.outfitChibi || profile.outfitFull)
            : profile.outfitFull;
        return outfit || '穿着简洁得体的日常服装';
    }
    function portraitPreset(args, prompts, customCharacters) {
        const templates = prompts.templates;
        const raw = String(args.character ?? '').trim();
        const chibi = args.chibi === true;
        const defaultRatio = chibi ? DEFAULT_RATIO : '2:3';
        if (raw === '月华') {
            return {
                kind: '月华', chibi, label: `月华${chibi ? 'Q版' : '全尺寸'}自画像`,
                template: chibi ? templates.chibiSelf : templates.fullSelf,
                base: prompts.builtinCharacters.get('月华').appearance,
                outfit: chibi ? templates.defaultOutfitChibi : templates.defaultOutfitFull,
                gender: '女孩', defaultRatio,
            };
        }
        if (raw === '琉璃') {
            return {
                kind: '琉璃', chibi, label: `琉璃${chibi ? 'Q版' : '全尺寸'}立绘`,
                template: chibi ? templates.crystalChibi : templates.crystalFull,
                base: prompts.builtinCharacters.get('琉璃').appearance,
                outfit: chibi ? templates.crystalOutfitChibi : templates.crystalOutfitFull,
                gender: '女孩', defaultRatio,
            };
        }
        const custom = customCharacters.get(raw);
        if (custom) {
            const profile = { ...custom};
            return {
                kind: raw, chibi, label: `${raw}${chibi ? 'Q版' : '全尺寸'}立绘`,
                template: chibi ? templates.genericChibi : templates.genericFull,
                base: profile.appearance,
                outfit: characterOutfit(profile, chibi),
                gender: profile.gender === '男' ? '男孩' : '女孩',
                defaultRatio,
            };
        }
        const male = raw === '通用-男';
        const named = raw && raw !== '通用-女' && raw !== '通用-男' && raw !== '其他' ? `名为「${raw}」的角色` : '';
        const desc = [named, String(args.appearance ?? '').trim()].filter(t => t).join('，');
        const base = (male ? templates.maleBase : templates.femaleBase) + (desc ? `，${desc}` : '');
        if (!desc)
            console.log('[绘制者] 通用角色未提供 appearance：仅按通用底板绘制，人物特征会很泛');
        return {
            kind: male ? '通用-男' : '通用-女', chibi, label: `${chibi ? 'Q版' : '全尺寸'}${male ? '男性' : '女性'}角色立绘`,
            template: chibi ? templates.genericChibi : templates.genericFull,
            base,
            outfit: desc ? '' : '穿着简洁得体的日常服装',
            gender: male ? '男孩' : '女孩', defaultRatio,
        };
    }
    function normalizeGroupRoles(value) {
        const list = Array.isArray(value) ? value : [value];
        const roles = [];
        for (const item of list) {
            const role = String(item ?? '').trim();
            if (!role || roles.includes(role))
                continue;
            roles.push(role);
        }
        return roles;
    }
    function groupMemberAppearance(profiles, role, chibi, withOutfit) {
        const profile = profiles.get(role);
        if (!profile)
            return role;
        const outfit = withOutfit ? `，${characterOutfit(profile, chibi)}` : '';
        return `${profile.name}，${profile.appearance}${outfit}`;
    }
    function defaultGroupPlanning(roles, chibi, profiles, prompts) {
        const entries = [];
        for (const role of roles) {
            const profile = profiles.get(role);
            if (!profile)
                continue;
            entries.push(`${profile.name}：${prompts.randomExpression()}，${prompts.randomPosture()}，${characterOutfit(profile, chibi)}`);
        }
        return entries.join('；');
    }

    function collectPaintingDetail(toolCall, paintings) {
        try {
            const args = typeof toolCall.function.arguments === 'string'
                ? JSON.parse(toolCall.function.arguments)
                : toolCall.function.arguments;
            const name = toolCall.function.name;
            if (name === 'portrait') {
                const character = String(args.character || '通用-女');
                paintings.push({
                    toolName: name,
                    promptSummary: `${characterLabel(character)}${args.chibi === true ? 'Q版' : '全尺寸'}立绘`,
                    character: character,
                    chibi: args.chibi === true,
                    expression: args.expression || '',
                    posture: args.posture || '',
                    environment: args.environment || '',
                });
            }
            else if (name === 'group_photo') {
                const roles = normalizeGroupRoles(args.roles);
                paintings.push({
                    toolName: name,
                    promptSummary: `${args.chibi === true ? 'Q版' : ''}与${roles.join('、') || '他人'}的合照`,
                    roles,
                    chibi: args.chibi === true,
                    planning: args.planning || '',
                    environment: args.environment || '',
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
    function buildPaintingSummary(paintings) {
        if (paintings.length === 0)
            return '月华没有绘制任何作品';
        const feedback = interactEvent('draw_painting_before', { paintings }).return;
        if (feedback && Array.isArray(feedback) && feedback.every(p => p.toolName && p.promptSummary)) {
            paintings = feedback;
        }
        const parts = [];
        for (const p of paintings) {
            if (p.toolName === 'portrait') {
                let desc;
                if (p.character === '月华')
                    desc = p.chibi ? '月华绘制了一幅Q版自画像' : '月华绘制了一幅全尺寸自画像';
                else if (p.character === '琉璃')
                    desc = `月华为琉璃绘制了一幅${p.chibi ? 'Q版' : '全尺寸'}立绘`;
                else
                    desc = `月华绘制了一幅${p.chibi ? 'Q版' : '全尺寸'}${characterLabel(p.character)}立绘`;
                if (p.expression)
                    desc += `，展现了${p.expression}`;
                if (p.environment)
                    desc += `，背景是${p.environment}`;
                parts.push(desc + '。');
            }
            else if (p.toolName === 'group_photo') {
                let desc = `月华绘制了一幅${p.chibi ? 'Q版' : ''}与${p.roles && p.roles.length > 0 ? p.roles.join('、') : '他人'}的合照`;
                if (p.planning)
                    desc += `，${p.planning}`;
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

    function createReferenceState() {
        return { image: '', handle: '', vision: '', size: null };
    }
    function clearReference(state) {
        state.image = '';
        state.handle = '';
        state.vision = '';
        state.size = null;
    }
    function bindReference(state, reference) {
        clearReference(state);
        if (!reference?.source)
            return;
        state.handle = reference.handle || '';
        state.image = reference.source;
        const [, visionError, visionWidth, visionHeight] = visionRead(reference.source, state.handle);
        const visionURL = visionReference(state.handle, reference.source);
        if (visionError || !visionURL) {
            console.error('[绘制者] 参考图视觉副本生成失败，本次任务模型将看不到参考图:', visionError?.message ?? String(visionError ?? '无法构造视觉副本'));
        }
        else {
            state.vision = visionURL;
            if (visionWidth > 0 && visionHeight > 0)
                state.size = { width: visionWidth, height: visionHeight };
        }
        console.log(`[绘制者] 本次任务参考图: ${state.handle || '(无句柄)'} → ${reference.source}${state.size ? `（视觉副本 ${state.size.width}x${state.size.height}）` : '（视觉副本生成失败）'}`);
    }
    function buildReferenceTaskContent(state, taskDescription) {
        if (!state.vision) {
            if (state.image)
                console.log('[绘制者] 参考图无可用视觉副本，退回纯文本任务描述');
            return taskDescription;
        }
        console.log(`[绘制者] 参考图视觉副本已附入任务上下文（${Math.round(state.vision.length / 1024)}KB）`);
        return [
            { type: 'text', text: taskDescription },
            { type: 'image_url', image_url: { url: state.vision } }
        ];
    }
    function describeReference(state, plan) {
        if (state.image) {
            const size = state.size ? `（${state.size.width}x${state.size.height}）` : '';
            return `本次任务参考图：${state.handle || state.image}${size}，输出尺寸将按参考图比例取 ${plan.width}x${plan.height}`;
        }
        return '本次任务尚未附带参考图';
    }

    function handleDiffusionGeneration(host, args) {
        try {
            const prompt = args.prompt || '';
            if (!prompt.trim())
                return '扩散生成失败：正向提示词不能为空';
            const plan = resolveDimensionPlan(args.ratio);
            const steps = normalizeSteps(args.steps, '扩散生成');
            console.log(`[绘制者] 扩散生成 - 正向提示词: ${prompt.slice(0, 100)}...，画面比例: ${plan.origin} → ${plan.width}x${plan.height}，步数: ${steps || '默认'}`);
            return host.paint({
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
    function resolveImg2ImgCfg(value) {
        const CFG_MIN = 0.5;
        const CFG_MAX = 4;
        const clampCfg = (v) => Math.min(Math.max(v, CFG_MIN), CFG_MAX);
        const configured = Number(GlobalConfig.customConfig?.diffusion?.img2img_cfg);
        if (Number.isFinite(configured) && configured > 0) {
            const cfg = clampCfg(configured);
            if (Math.abs(cfg - configured) > 1e-6)
                console.log(`[绘制者] 配置 img2img_cfg=${configured} 越界，已收进 ${cfg}`);
            return cfg;
        }
        if (typeof value === 'number' && Number.isFinite(value) && value > 0) {
            const cfg = clampCfg(value);
            if (cfg > 1.05)
                console.log(`[绘制者] 图生图 cfg=${value}：本模型为引导蒸馏，建议 1.0（过高会发灰/过曝/塑料感）`);
            return cfg;
        }
        return 1.0;
    }
    function handleImageToImage(host, args) {
        try {
            const prompt = args.prompt || '';
            if (!prompt.trim())
                return '图生图失败：正向提示词不能为空';
            if (!host.referenceImage) {
                return `图生图失败：${host.referenceStatus()}。图生图基座由对话者经 dispatch_painter 的 reference_tag 指定；当前任务无参考图时请改用 diffusion_generation 进行文生图`;
            }
            const strength = normalizeStrength(args.strength, '图生图');
            const steps = normalizeSteps(args.steps, '图生图');
            const cfgScale = resolveImg2ImgCfg(args.cfg_scale);
            const plan = host.planFor();
            console.log(`[绘制者] 图生图 - 正向提示词: ${prompt.slice(0, 100)}...，参考图: ${host.referenceImage}，强度: ${strength}，cfg: ${cfgScale}，尺寸: ${plan.origin} → ${plan.width}x${plan.height}，步数: ${steps || '默认'}`);
            return host.paint({
                prompt: prompt,
                negativePrompt: args.negative_prompt || '',
                width: plan.width,
                height: plan.height,
                steps: steps,
                cfgScale: cfgScale,
                initImg: host.referenceImage,
                strength: strength,
                superResolution: args.super_resolution === true,
            }, '图生图生成');
        }
        catch (error) {
            console.error('[绘制者] 图生图处理异常:', error);
            return `图生图生成异常: ${error}`;
        }
    }

    function handlePortrait(host, args) {
        try {
            const preset = portraitPreset(args, host.prompts, host.customCharacters);
            console.log(`[绘制者] -> ${preset.label}生成`);
            console.log(`角色: "${args.character}" chibi=${preset.chibi}`);
            console.log(`表情: "${args.expression}"`);
            console.log(`姿势: "${args.posture}"`);
            console.log(`服装: "${args.outfit}"`);
            console.log(`环境: "${args.environment}"`);
            console.log(`负面提示词: "${args.negative_prompt}"`);
            console.log(`提示词引导系数: "${args.cfg_scale}"`);
            const plan = resolveDimensionPlan(args.ratio || preset.defaultRatio);
            const steps = normalizeSteps(args.steps, preset.label);
            const fullPrompt = host.prompts.renderAppearance(preset.template, args, preset.outfit, preset.base, preset.gender);
            return host.paint({
                prompt: fullPrompt,
                negativePrompt: args.negative_prompt || host.prompts.templates.defaultNegative,
                width: plan.width,
                height: plan.height,
                steps: steps,
                cfgScale: args.cfg_scale ?? 1.0,
                superResolution: args.super_resolution === true,
            }, `${preset.label}生成`);
        }
        catch (error) {
            console.error('[绘制者] 立绘处理异常:', error);
            return `立绘生成异常: ${error}`;
        }
    }

    function handleGroupPhoto(host, args) {
        try {
            const roles = normalizeGroupRoles(args.roles);
            if (roles.length === 0)
                return '合照生成失败：roles 至少需要一位参与者';
            const chibi = args.chibi === true;
            const profiles = mergeProfiles(host.prompts.builtinCharacters, host.customCharacters);
            const planningProvided = !!String(args.planning || '').trim();
            const planning = String(args.planning || '').trim() || defaultGroupPlanning(roles, chibi, profiles, host.prompts);
            const environment = String(args.environment || '').trim() || host.prompts.randomGroupEnvironment();
            const members = roles
                .map(role => groupMemberAppearance(profiles, role, chibi, !planningProvided))
                .join('；');
            const fullPrompt = host.prompts.renderTemplate(host.prompts.templates.groupPhoto, {
                proportion: chibi ? 'Q版二头身，头身比约1:2，' : '',
                count: String(roles.length),
                members,
                planning: planning ? `人物安排：${planning}。` : '',
                environment,
            });
            console.log(`[绘制者] -> ${chibi ? 'Q版' : '全尺寸'}合照生成（${roles.length}人）`);
            console.log(`参与者: "${roles.join('、')}"`);
            console.log(`人物规划: "${planning || '默认'}"`);
            console.log(`环境: "${environment}"`);
            const useReference = !!host.referenceImage;
            const plan = useReference ? host.planFor() : resolveDimensionPlan(args.ratio);
            const steps = normalizeSteps(args.steps, '合照');
            const imageParams = {
                prompt: fullPrompt,
                negativePrompt: args.negative_prompt || host.prompts.templates.defaultNegative,
                width: plan.width,
                height: plan.height,
                steps: steps,
                cfgScale: args.cfg_scale ?? 1.0,
                superResolution: args.super_resolution === true,
            };
            if (useReference) {
                imageParams.initImg = host.referenceImage;
                imageParams.strength = normalizeStrength(args.strength, '合照');
                console.log(`[绘制者] 合照图生图模式，参考图: ${host.referenceImage}，强度: ${imageParams.strength}，尺寸: ${plan.origin} → ${plan.width}x${plan.height}，步数: ${steps || '默认'}`);
            }
            return host.paint(imageParams, '合照生成');
        }
        catch (error) {
            console.error('[绘制者] 合照生成处理异常:', error);
            return `合照生成异常: ${error}`;
        }
    }

    const VISION_READ_PATH = '/file/miniature/';
    const FILE_READ_PATH = '/file/read/';
    function pathPartOf(source) {
        return String(source || '').split('?')[0].split('#')[0];
    }
    function toDisplaySource(source) {
        const raw = String(source || '');
        if (!raw)
            return '';
        if (raw.startsWith('data:') || raw.startsWith('blob:'))
            return raw;
        const path = pathPartOf(raw);
        const localIndex = path.indexOf(FILE_READ_PATH);
        if (localIndex >= 0)
            return path.slice(localIndex);
        const visionIndex = path.indexOf(VISION_READ_PATH);
        if (visionIndex >= 0) {
            return FILE_READ_PATH + path.slice(visionIndex + VISION_READ_PATH.length);
        }
        if (path.startsWith('multimedia/'))
            return FILE_READ_PATH + path;
        return raw;
    }
    function toVisionSource(source) {
        const raw = String(source || '');
        if (!raw || raw.startsWith('data:') || raw.startsWith('blob:'))
            return '';
        const path = pathPartOf(raw);
        const visionIndex = path.indexOf(VISION_READ_PATH);
        if (visionIndex >= 0)
            return url()[0] + path.slice(visionIndex);
        const readIndex = path.indexOf(FILE_READ_PATH);
        if (readIndex >= 0) {
            const rel = path.slice(readIndex + FILE_READ_PATH.length);
            if (!rel.startsWith('multimedia/'))
                return '';
            return `${url()[0]}${VISION_READ_PATH}${rel}`;
        }
        if (path.startsWith('multimedia/'))
            return `${url()[0]}${VISION_READ_PATH}${path}`;
        return '';
    }
    function localPathToReadURL(source) {
        if (!source)
            return '';
        const normalized = String(source).replace(/\\/g, '/').split('?')[0];
        const marker = normalized.toLowerCase().lastIndexOf('/multimedia/');
        const rel = marker >= 0
            ? normalized.slice(marker + 1)
            : (normalized.toLowerCase().startsWith('multimedia/') ? normalized : '');
        return rel ? FILE_READ_PATH + rel : '';
    }
    function fileNameOf(source) {
        const path = pathPartOf(String(source || ''));
        const name = path.slice(path.lastIndexOf('/') + 1);
        return name || '文件';
    }
    function splitUserPrefix(text) {
        const raw = String(text || '');
        const match = /^\[用户:\s*([^\]]*)\]\s*[:：]\s*/u.exec(raw);
        if (!match)
            return { sender: '', text: raw };
        return { sender: match[1].trim(), text: raw.slice(match[0].length) };
    }
    function guessImageMime(base64) {
        const head = String(base64 || '').slice(0, 16);
        if (head.startsWith('/9j/'))
            return 'image/jpeg';
        if (head.startsWith('iVBOR'))
            return 'image/png';
        if (head.startsWith('R0lG'))
            return 'image/gif';
        if (head.startsWith('UklGR'))
            return 'image/webp';
        if (head.startsWith('Qk'))
            return 'image/bmp';
        return 'image/png';
    }
    function saveBase64Media(base64, mime, kind) {
        const raw = String(base64 || '');
        if (!raw)
            return '';
        const match = /^data:([^;,]+);base64,(.*)$/s.exec(raw);
        const data = match ? match[2] : raw;
        const contentType = match ? match[1] : (kind === 'voice' ? mime : guessImageMime(data));
        try {
            const [decoded, decodeErr] = base64Decode(data);
            if (decodeErr || !decoded) {
                console.warn('[对话缓存] base64 解码失败:', decodeErr);
                return '';
            }
            const bytes = new Uint8Array(decoded);
            const extMap = {
                'image/jpeg': 'jpg', 'image/png': 'png', 'image/webp': 'webp', 'image/gif': 'gif', 'image/bmp': 'bmp',
                'audio/wav': 'wav', 'audio/x-wav': 'wav', 'audio/mpeg': 'mp3', 'audio/mp3': 'mp3', 'audio/flac': 'flac', 'audio/ogg': 'ogg',
            };
            const ext = extMap[contentType] || 'bin';
            const name = `multimedia/${kind}_${Date.now()}_${Math.random().toString(36).slice(2, 8)}.${ext}`;
            const [, , error] = saveFile(name, true, bytes);
            if (error) {
                console.warn('[对话缓存] 媒体落盘失败:', error);
                return raw.startsWith('data:') ? raw : '';
            }
            return FILE_READ_PATH + name;
        }
        catch (error) {
            console.warn('[对话缓存] 媒体解码失败:', error);
            return '';
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
    function getPromptFromKnowledgeAny(keys) {
        if (!keys.length)
            return null;
        try {
            const map = loadKnowledge('video_summary');
            for (const key of keys) {
                const value = map.get(key);
                if (typeof value === 'string' && value.trim())
                    return value;
            }
            return null;
        }
        catch (error) {
            console.error('[知识库] 读取媒体理解缓存失败:', error);
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

    const CACHE_FILE = 'database/messages.json';
    const MAX_CACHE_MESSAGES = 48;
    let cachedMessages = [];
    let lastSnapshot = '';
    const animatedHistoryCache = new Map();
    function newId$1() {
        return Date.now().toString(36) + Math.random().toString(36).slice(2, 8);
    }
    function trimCache() {
        if (cachedMessages.length > MAX_CACHE_MESSAGES)
            cachedMessages = cachedMessages.slice(-MAX_CACHE_MESSAGES);
    }
    function repairHandleImageURL(source) {
        const match = /^\/file\/read\/(@[DIVAT]\.\d{3,})\/?$/.exec(String(source || ''));
        if (!match)
            return source;
        try {
            const [resource, error] = resourceRead(match[1], 1);
            if (!error && resource && !resource.lost && resource.blob_path)
                return '/file/read/' + resource.blob_path;
        }
        catch (error) {
            console.warn('[对话缓存] 句柄图片地址修复失败:', error);
        }
        return source;
    }
    function appendCachedMessage(record) {
        cachedMessages.push(record);
        trimCache();
    }
    function persistConversation() {
        trimCache();
        const text = JSON.stringify(cachedMessages);
        if (text === lastSnapshot)
            return;
        try {
            const [, , error] = saveFile(CACHE_FILE, true, text);
            if (error) {
                console.warn('[对话缓存] 写入失败:', error);
                return;
            }
            lastSnapshot = text;
        }
        catch (error) {
            console.warn('[对话缓存] 写入异常:', error);
        }
    }
    function normalizeRecord(raw) {
        if (!raw || (raw.role !== 'user' && raw.role !== 'assistant'))
            return null;
        let content = typeof raw.content === 'string' ? raw.content : '';
        let sender = typeof raw.sender === 'string' ? raw.sender.trim() : '';
        const parsed = splitUserPrefix(content);
        if (!sender && parsed.sender)
            sender = parsed.sender;
        return {
            id: typeof raw.id === 'string' && raw.id ? raw.id : newId$1(),
            role: raw.role,
            categories: Array.isArray(raw.categories) ? raw.categories.map(String) : ['text'],
            content: content,
            sender: sender || undefined,
            attachments: Array.isArray(raw.attachments) ? raw.attachments : undefined,
            imageSrc: typeof raw.imageSrc === 'string' ? raw.imageSrc : undefined,
            imageLabel: typeof raw.imageLabel === 'string' ? raw.imageLabel : undefined,
            audio: typeof raw.audio === 'string' ? raw.audio : undefined,
            abcNotation: typeof raw.abcNotation === 'string' ? raw.abcNotation : undefined,
            actionType: typeof raw.actionType === 'string' ? raw.actionType : undefined,
            fileRef: typeof raw.fileRef === 'string' ? raw.fileRef : undefined,
            fileLabel: typeof raw.fileLabel === 'string' ? raw.fileLabel : undefined,
            timestamp: Number(raw.timestamp) || Date.now(),
        };
    }
    function animatedSummaryKeys(source) {
        const raw = String(source || '');
        if (!raw)
            return [];
        const normalize = (value) => String(value || '').split('?')[0].split('#')[0];
        const vision = toVisionSource(raw);
        const keys = new Set();
        for (const value of [raw, vision, normalize(raw), normalize(vision)]) {
            if (value)
                keys.add(value);
        }
        const path = normalize(raw);
        const marker = path.indexOf('/multimedia/');
        if (marker >= 0) {
            const rel = path.slice(marker + 1);
            keys.add(rel);
            keys.add('/file/read/' + rel);
            keys.add('/file/miniature/' + rel);
            keys.add('file://' + rel);
            try {
                const base = url()[0];
                if (base) {
                    keys.add(base + '/file/read/' + rel);
                    keys.add(base + '/file/miniature/' + rel);
                }
            }
            catch { }
        }
        return [...keys];
    }
    function cachedToContext(record) {
        if (record.role === 'user') {
            const parsed = splitUserPrefix(record.content);
            const sender = (record.sender || parsed.sender).trim();
            const text = record.content;
            const prefix = sender && !parsed.sender ? `[用户: ${sender}]: ` : '';
            const blocks = [];
            if (text)
                blocks.push({ type: 'text', text: prefix + text });
            else if (sender)
                blocks.push({ type: 'text', text: `[用户: ${sender}]:` });
            for (const attachment of record.attachments || []) {
                if (attachment.type === 'image') {
                    const vision = toVisionSource(attachment.src);
                    if (!vision)
                        continue;
                    const [animated, animatedError] = animatedHistoryCache.has(attachment.src)
                        ? [animatedHistoryCache.get(attachment.src) === true, null]
                        : isAnimatedImage(attachment.src);
                    if (!animatedError && !animatedHistoryCache.has(attachment.src)) {
                        animatedHistoryCache.set(attachment.src, animated);
                    }
                    if (animatedError) {
                        blocks.push({ type: 'text', text: `[历史图片不可用：${attachment.label || fileNameOf(attachment.src)}]` });
                        continue;
                    }
                    if (animated) {
                        const summary = (getPromptFromKnowledgeAny(animatedSummaryKeys(attachment.src)) || '').trim();
                        const text = summary
                            ? `[动态图: ${summary.length > 1200 ? summary.slice(0, 1200) + '…' : summary}]`
                            : '[动态图]';
                        blocks.push({ type: 'text', text });
                        continue;
                    }
                    blocks.push({ type: 'image_url', image_url: { url: vision } });
                }
                else if (attachment.type === 'video') {
                    const display = toDisplaySource(attachment.src);
                    if (display.startsWith('/file/read/'))
                        blocks.push({ type: 'video_url', video_url: { url: url()[0] + display } });
                }
            }
            if (!blocks.length)
                return text ? { role: 'user', content: text } : null;
            if (blocks.length === 1 && blocks[0].type === 'text')
                return { role: 'user', content: blocks[0].text };
            return { role: 'user', content: blocks };
        }
        if (record.categories.includes('image') || record.categories.includes('action') || record.abcNotation)
            return null;
        const text = String(record.content || '').trim();
        if (!text)
            return null;
        return { role: 'assistant', content: text };
    }
    function initConversationCache(loadContext) {
        try {
            const [content, error] = readTextFile(CACHE_FILE);
            if (error || !content)
                return;
            const parsed = JSON.parse(content);
            if (!Array.isArray(parsed))
                return;
            cachedMessages = parsed
                .map(normalizeRecord)
                .filter((item) => !!item)
                .slice(-MAX_CACHE_MESSAGES);
            let migrated = false;
            for (const record of cachedMessages) {
                if (record.imageSrc) {
                    const fixed = repairHandleImageURL(record.imageSrc);
                    if (fixed !== record.imageSrc) {
                        record.imageSrc = fixed;
                        migrated = true;
                    }
                }
                for (const attachment of record.attachments || []) {
                    if (attachment.type !== 'image' || !attachment.src)
                        continue;
                    const fixed = repairHandleImageURL(attachment.src);
                    if (fixed !== attachment.src) {
                        attachment.src = fixed;
                        migrated = true;
                    }
                }
                if (record.role === 'assistant' && record.imageSrc && record.imageSrc.startsWith('data:image/')) {
                    const saved = saveBase64Media(record.imageSrc, 'image/jpeg', 'image');
                    if (saved) {
                        record.imageSrc = saved;
                        migrated = true;
                    }
                }
            }
            lastSnapshot = migrated ? '' : JSON.stringify(cachedMessages);
            if (migrated)
                persistConversation();
            const context = cachedMessages
                .map(cachedToContext)
                .filter((item) => !!item);
            if (context.length)
                loadContext(context);
            console.log(`[对话缓存] 已载入 ${context.length} 条近期记忆`);
        }
        catch (error) {
            console.warn('[对话缓存] 载入失败:', error);
        }
    }

    function newId() {
        return Date.now().toString(36) + Math.random().toString(36).slice(2, 8);
    }
    function buildUserRecord(content) {
        const attachments = [];
        const texts = [];
        let sender = '';
        let hasImage = false;
        let hasVoice = false;
        const pushAttachment = (type, source, label) => {
            const src = toDisplaySource(source);
            if (!src)
                return;
            attachments.push({ type, src, label: label || fileNameOf(src) });
        };
        const pushText = (raw) => {
            const parsed = splitUserPrefix(raw);
            if (!sender && parsed.sender)
                sender = parsed.sender;
            if (!parsed.text || parsed.text === '[视频]' || parsed.text === '[语音]')
                return;
            texts.push(raw);
        };
        if (typeof content === 'string')
            pushText(content);
        else if (Array.isArray(content)) {
            for (const item of content) {
                if (!item)
                    continue;
                if (item.type === 'text') {
                    pushText(String(item.text || ''));
                }
                else if (item.type === 'image_url') {
                    pushAttachment('image', item.image_url?.url || '');
                    hasImage = true;
                }
                else if (item.type === 'video_url') {
                    pushAttachment('video', item.video_url?.url || '');
                    hasImage = true;
                }
                else if (item.type === 'input_audio') {
                    const src = saveBase64Media(item.input_audio?.data || '', `audio/${item.input_audio?.format || 'wav'}`, 'voice');
                    if (src) {
                        attachments.push({ type: 'audio', src, label: '语音' });
                        hasVoice = true;
                    }
                }
                else if (item.type === 'audio_url') {
                    pushAttachment('audio', item.audio_url?.url || '', '语音');
                    hasVoice = true;
                }
            }
        }
        const text = texts.join('\n').trim();
        if (!text && !attachments.length)
            return null;
        const categories = [];
        if (text)
            categories.push('text');
        if (hasImage)
            categories.push('image');
        if (hasVoice)
            categories.push('voice');
        if (/@[DIVAT]\.\d{3,}/.test(text))
            categories.push('file');
        return {
            id: newId(),
            role: 'user',
            categories: categories.length ? categories : ['text'],
            content: text,
            sender: sender || undefined,
            attachments: attachments.length ? attachments : undefined,
            timestamp: Date.now(),
        };
    }
    function appendUserMessage(content) {
        const record = buildUserRecord(content);
        if (!record)
            return;
        appendCachedMessage(record);
        persistConversation();
    }
    function recordAssistantContext(type, content, audio) {
        if (type === 'music_audio')
            return;
        const base = {
            id: newId(),
            role: 'assistant',
            categories: ['text'],
            content: String(content || ''),
            timestamp: Date.now(),
        };
        if (type === 'music') {
            base.categories = ['music'];
            base.content = '';
            base.abcNotation = String(content || '');
        }
        else if (type === 'action') {
            base.categories = ['action'];
            base.actionType = type;
        }
        else if (type === 'text') {
            if (audio) {
                base.categories = ['text', 'voice'];
                base.audio = audio;
            }
            const notice = /^\[已收录\]\s*(@[DIVAT]\.\d{3,})\s*(.*)$/.exec(base.content.trim());
            if (notice) {
                base.categories = ['text', 'file'];
                base.fileRef = notice[1];
                base.fileLabel = notice[2] || notice[1];
            }
        }
        else {
            base.categories = [type];
        }
        appendCachedMessage(base);
    }
    function pushAssistantContext(type, content, audio, auto) {
        const ok = pushContext(type, content, audio, auto);
        recordAssistantContext(type, content, audio);
        persistConversation();
        return ok;
    }
    function pushAssistantImage(images, isSticker, sourcePaths) {
        const canonical = pushImage(images, isSticker);
        const list = Array.isArray(canonical) && canonical.length > 0 ? canonical : images;
        (list || []).forEach((image, index) => {
            if (!image)
                return;
            const src = toDisplaySource(image);
            if (!src || src.startsWith('data:') || src.startsWith('blob:')) {
                console.warn('[对话缓存] 图片不是 URL，已跳过近期记忆写入');
                return;
            }
            appendCachedMessage({
                id: newId(),
                role: 'assistant',
                categories: ['image'],
                content: '',
                imageSrc: src,
                imageLabel: isSticker ? '表情包' : '图片',
                timestamp: Date.now(),
            });
        });
        persistConversation();
        return list;
    }

    function handleCreateCharacter(host, args) {
        const name = String(args.name || '').trim();
        const appearance = String(args.appearance || '').trim();
        if (!name)
            return '创建角色失败：角色名不能为空';
        if (!appearance)
            return '创建角色失败：appearance 不能为空（写清年龄感、发型发色、瞳色、面部特征、配饰、体型）';
        if (name === '月华' || name === '琉璃' || name === '通用-女' || name === '通用-男') {
            return `创建角色失败：「${name}」是内置角色名，请换一个名字`;
        }
        const existed = host.customCharacters.has(name);
        const profile = {
            name,
            gender: args.gender === '男' ? '男' : '女',
            appearance,
            outfitFull: String(args.outfit_full || '').trim() || undefined,
            outfitChibi: String(args.outfit_chibi || '').trim() || undefined,
        };
        if (!saveCustomCharacter(profile))
            return '创建角色失败：角色库写入失败，请稍后重试';
        host.customCharacters.set(name, profile);
        console.log(`[绘制者] 自定义角色${existed ? '更新' : '创建'}成功: ${name}`);
        return `${existed ? '已更新' : '已创建'}自定义角色「${name}」（${profile.gender}）。之后绘制立绘或合照时可直接用该名字引用。`;
    }
    function handleSwitchDiffusionScheme(args) {
        const name = String(args.scheme || '').trim();
        if (!name)
            return '切换模型方案失败：请指定 turbo 或 quality';
        const [current, error] = setDiffusionScheme(name);
        if (error)
            return `切换模型方案失败：${error.message ?? String(error)}`;
        return `已切换到 ${current}（${schemeLabel(current)}）模型方案，后续绘图将使用该方案。`;
    }
    function schemeLabel(name) {
        switch (name) {
            case 'turbo': return '速度优先';
            case 'quality': return '质量优先';
            default: return name || '默认';
        }
    }
    function handleListImages(host) {
        const entries = listResources('I', LIST_LIMIT_MAX);
        if (entries.length === 0)
            return '资源目录中还没有任何图片：当前没有可引用的图片句柄';
        const lines = entries.map(entry => {
            const desc = entry.summary ? sanitizeImageDesc(entry.summary, 40) : '（暂无描述）';
            const lost = entry.state === 'active' ? '' : ' [本体已回收]';
            return `${entry.handle} ${desc}${lost}`;
        });
        return `资源目录中最近的 ${entries.length} 张图片（句柄越新越靠前）：\n${lines.join('\n')}\n可调用 upscale_image 对指定图片超分；图生图基座由对话者经 reference_tag 指定。${host.referenceStatus()}。`;
    }
    function handleUpscaleImage(host, args) {
        try {
            const resolved = args.image_tag ? resolveImageRef(String(args.image_tag)) : null;
            let source = '';
            let handleLabel = '';
            if (args.image_tag) {
                if (!resolved)
                    return '超分失败：句柄不存在或已失效，请先调用 list_images 查看可用图片';
                if (!resolved.source)
                    return `超分失败：${resolved.handle} 的本体已从内容仓库回收，无法超分`;
                source = resolved.source;
                handleLabel = resolved.handle;
            }
            else {
                if (!host.lastResultPath)
                    return '超分失败：还没有生成过作品，请通过 image_tag 指定图片句柄，或先完成一次图像生成';
                source = host.lastResultPath;
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
            const url = localPathToReadURL(result.path);
            const urls = url ? pushAssistantImage([url], false) : [];
            if (urls.length === 0)
                console.warn('[绘制者] 推送超分结果到前端失败');
            else
                host.rememberResult(urls[0]);
            return `超分成功。图片尺寸: ${result.width}x${result.height}，已推送给用户`;
        }
        catch (error) {
            console.error('[绘制者] 超分处理异常:', error);
            return `超分处理异常: ${error}`;
        }
    }

    function composePainterPrompt() {
        const characters = loadCharacterPrompts();
        return fileView('prompts/painterRole.md')[0].trim()
            .split('{lunar-appearance}').join(characters.text('月华/基础外观'))
            .split('{crystal-appearance}').join(characters.text('琉璃/基础外观'));
    }
    class PainterRole extends CreativeRoleBase {
        roleTag = "绘制者";
        prompts = new PainterPromptLibrary();
        customCharacters = loadCustomCharacters();
        reference = createReferenceState();
        resultPath = '';
        constructor() {
            super(composePainterPrompt());
        }
        get referenceImage() { return this.reference.image; }
        set referenceImage(value) { this.reference.image = value; }
        get referenceHandle() { return this.reference.handle; }
        set referenceHandle(value) { this.reference.handle = value; }
        get referenceVision() { return this.reference.vision; }
        set referenceVision(value) { this.reference.vision = value; }
        get referenceSize() { return this.reference.size; }
        set referenceSize(value) { this.reference.size = value; }
        get lastResultPath() { return this.resultPath; }
        rememberResult(path) {
            this.resultPath = path;
        }
        async createCreativeWork(taskDescription, reference) {
            this.customCharacters = loadCustomCharacters();
            bindReference(this.reference, reference);
            return super.createCreativeWork(taskDescription);
        }
        buildTaskContent(taskDescription) {
            return buildReferenceTaskContent(this.reference, taskDescription);
        }
        planFor(ratio) {
            const reference = this.reference.image ? (this.reference.size || undefined) : undefined;
            return resolveDimensionPlan(ratio, reference);
        }
        referenceStatus() {
            return describeReference(this.reference, this.planFor());
        }
        resetReference() {
            clearReference(this.reference);
        }
        get roleName() { return '绘制者'; }
        getToolDefinitions() {
            const catalog = this.characterCatalog();
            const definitions = JSON.stringify(painterTools).split('{characters}').join(catalog);
            return JSON.parse(definitions);
        }
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
                case 'diffusion_generation': return handleDiffusionGeneration(this, args);
                case 'image_to_image': return handleImageToImage(this, args);
                case 'portrait': return handlePortrait(this, args);
                case 'create_character': return handleCreateCharacter(this, args);
                case 'group_photo': return handleGroupPhoto(this, args);
                case 'switch_diffusion_scheme': return handleSwitchDiffusionScheme(args);
                case 'list_images': return handleListImages(this);
                case 'upscale_image': return handleUpscaleImage(this, args);
                default: return `未知工具: ${funcName}，可用工具为 diffusion_generation、image_to_image、portrait、create_character、group_photo、switch_diffusion_scheme、list_images、upscale_image`;
            }
        }
        collectDetail(toolCall, paintings) {
            collectPaintingDetail(toolCall, paintings);
        }
        buildSummary(paintings) {
            return buildPaintingSummary(paintings);
        }
        paint(imageParams, label) {
            const [result, error] = generateImage(imageParams);
            if (error) {
                console.error(`[绘制者] ${label}失败:`, error);
                return `${label}失败: ${error}`;
            }
            if (!result || !result.base64)
                return `${label}失败：引擎返回空结果`;
            this.rememberResult(result.path);
            const stepsNote = imageParams.steps ? `，${imageParams.steps} 步` : '';
            console.log(`[绘制者] ${label}成功，尺寸: ${result.width}x${result.height}${stepsNote}${imageParams.superResolution ? '（含超分重建）' : ''}`);
            const url = localPathToReadURL(result.path);
            const urls = url ? pushAssistantImage([url], false) : [];
            if (urls.length === 0)
                console.warn(`[绘制者] 推送${label}结果到前端失败`);
            else
                this.rememberResult(urls[0]);
            return `${label}成功。图片尺寸: ${result.width}x${result.height}，seed: ${result.seed}${stepsNote}${imageParams.superResolution ? '，已附带超分辨率重建' : ''}`;
        }
        characterCatalog() {
            return characterCatalogText(this.characterProfiles());
        }
        characterProfiles() {
            return mergeProfiles(this.prompts.builtinCharacters, this.customCharacters);
        }
    }

    const ABC_LINE_RULE = '仅一行音符，用 | 分隔小节，以 |] 结尾';
    const ABC_NO_HEADER_RULE = '禁止包含 X:/T:/M:/L:/Q:/K: 等头部字段与 [V:N] 声部标记';
    const ABC_SAME_BARS_RULE = '小节数必须与主旋律完全一致、逐小节对齐';
    const musicTools = [
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
                            description: `主旋律ABC片段：${ABC_LINE_RULE}。${ABC_NO_HEADER_RULE}。应有起伏与乐句呼吸感（每4-8小节一个乐句）`
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
                            description: `伴奏ABC片段：${ABC_LINE_RULE}，${ABC_SAME_BARS_RULE}。${ABC_NO_HEADER_RULE}。和弦用方括号包裹同时发音的音符，如 [C,,E,,G,,]4 或分解和弦 C,,2 E,2 G,2 c2`
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
                            description: `和弦进行ABC片段：${ABC_LINE_RULE}，${ABC_SAME_BARS_RULE}。${ABC_NO_HEADER_RULE}。使用根音、柱式和弦或分解和弦，明确体现和声进行（如 C-F-G-C）`
                        }
                    },
                    required: [
                        "chords"
                    ]
                }
            }
        }
    ];

    function sanitizeFragment(fragment) {
        return fragment
            .replace(/^[A-Za-z]:.*$/gm, '')
            .replace(/\[[Vv]:\d+\]/g, '')
            .replace(/\s+/g, ' ')
            .trim();
    }
    function countBars(fragment) {
        const body = fragment
            .replace(/^[A-Za-z]:.*$/gm, '')
            .replace(/\[[Vv]:\d+\]/g, '')
            .replace(/![^!|]*!/g, '')
            .replace(/\|\]|:\||\|\|/g, '|');
        return body.split('|').filter(segment => /[A-Ga-gz]/.test(segment)).length;
    }
    function assembleScore(draft) {
        const instruments = [draft.melodyInstrument, draft.accompanimentInstrument, draft.chordInstrument]
            .map(inst => inst.trim())
            .filter(Boolean)
            .join(',');
        const abc = [
            `X:1`,
            `T:${draft.title}`,
            `M:${draft.meter}`,
            `L:1/8`,
            `Q:1/4=${draft.tempo}`,
            `K:${draft.key}`,
            `[V:1] ${draft.melody}`,
            `[V:2] ${draft.accompaniment}`,
            `[V:3] ${draft.chords}`,
            ''
        ].join('\n');
        return { abc: injectInstrumentDirective(abc, instruments), instruments };
    }
    function injectInstrumentDirective(abcNotation, instruments) {
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
        const directives = list.map((inst, index) => `%%voice ${index + 1} ${inst}`);
        const directive = directives.join('\n') + '\n';
        const xMatch = abcNotation.match(/^X:\s*\d+/m);
        if (xMatch && xMatch.index !== undefined) {
            const before = abcNotation.substring(0, xMatch.index);
            const after = abcNotation.substring(xMatch.index);
            return before + directive + after;
        }
        return directive + abcNotation;
    }

    function buildMusicSummary(pieces) {
        if (pieces.length === 0)
            return '月华没有演奏任何作品';
        const feedback = interactEvent('play_music_before', { pieces }).return;
        if (feedback && Array.isArray(feedback) && feedback.every(p => p.title && p.instruments && p.tempo && p.structure && p.key && p.meter && p.abcLength !== undefined)) {
            pieces = feedback;
        }
        const parts = [];
        for (const p of pieces) {
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

    class MusicianRole extends CreativeRoleBase {
        roleTag = "演奏者";
        MAX_ITERATIONS = 4;
        MIN_MELODY_BARS = 12;
        DEFAULT_TEMPO = 100;
        draft = null;
        assembledDetail = null;
        constructor() {
            super(fileView('prompts/musicianRole.md')[0]);
        }
        get roleName() { return '演奏者'; }
        getToolDefinitions() { return musicTools; }
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
            return buildMusicSummary(pieces);
        }
        handleSubmitMelody(args) {
            try {
                const title = (args.title || '').trim();
                const melody = sanitizeFragment(args.melody || '');
                if (!title)
                    return '主旋律提交失败：作品标题不能为空';
                if (!melody)
                    return '主旋律提交失败：旋律内容不能为空';
                const bars = countBars(melody);
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
                const accompaniment = sanitizeFragment(args.accompaniment || '');
                if (!accompaniment)
                    return '伴奏提交失败：伴奏内容不能为空';
                const bars = countBars(accompaniment);
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
                const chords = sanitizeFragment(args.chords || '');
                if (!chords)
                    return '和弦提交失败：和弦进行内容不能为空';
                const bars = countBars(chords);
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
            const draft = this.draft;
            const { abc, instruments } = assembleScore(draft);
            if (!pushAssistantContext('music', abc, '')) {
                console.warn('[演奏者] 推送乐谱到前端失败');
            }
            return {
                title: draft.title,
                instruments: instruments,
                tempo: draft.tempo,
                structure: draft.structure,
                key: draft.key,
                meter: draft.meter,
                abcLength: draft.melodyBars,
            };
        }
    }

    const screenshoterTools = [
        {
            type: 'function',
            function: {
                name: 'list_capture_targets',
                description: '列出当前所有显示器与可见窗口（标题、进程、窗口矩形、客户区尺寸、是否最小化、是否焦点）。当用户说的「那个窗口」不明确、或按标题截图前需要确认目标时先调用它。',
                parameters: {
                    type: 'object',
                    properties: {
                        include_minimized: {
                            type: 'boolean',
                            description: '是否也列出已最小化的窗口（最小化窗口无法截图，仅用于告知用户需要先恢复），默认 false'
                        }
                    },
                    required: []
                }
            }
        },
        {
            type: 'function',
            function: {
                name: 'capture_screen',
                description: [
                    '执行一次截图并立即展示给用户。按用户要求选择模式：',
                    'mode=fullscreen 截全部屏幕（多显示器会拼成一张）；',
                    'mode=display 截单个显示器（display_index）；',
                    'mode=window 截指定窗口：window_title 按标题包含匹配，window_handle 按句柄精确匹配，两者都不填则截当前焦点窗口；被其他窗口遮挡但不处于最小化的窗口同样能截到；',
                    'mode=region 截绝对屏幕坐标区域（region_x/region_y/region_w/region_h）；',
                    '在 mode=window 下再给 width/height 可只截窗口内相对区域（offset_x/offset_y 为窗口左上角偏移，client_area=true 时以客户区左上角为原点）。',
                    'mode=auto 为默认：截焦点窗口，失败时自动降级为全屏。'
                ].join('\n'),
                parameters: {
                    type: 'object',
                    properties: {
                        mode: {
                            type: 'string',
                            enum: ['auto', 'window', 'fullscreen', 'display', 'region'],
                            description: '截图模式，默认 auto'
                        },
                        window_title: {
                            type: 'string',
                            description: '目标窗口标题关键字（包含匹配，忽略大小写）。用于截不在前台的指定窗口'
                        },
                        window_handle: {
                            type: 'number',
                            description: '目标窗口句柄（优先于 window_title）。来自 list_capture_targets 的 handle'
                        },
                        client_area: {
                            type: 'boolean',
                            description: '是否只截客户区（去掉标题栏与边框），默认 false；截网页/应用内容时通常设为 true'
                        },
                        display_index: {
                            type: 'number',
                            description: '显示器索引（mode=display 时生效）；-1 表示全部显示器拼接'
                        },
                        offset_x: {
                            type: 'number',
                            description: '窗口内相对 X 偏移（配合 width/height）'
                        },
                        offset_y: {
                            type: 'number',
                            description: '窗口内相对 Y 偏移（配合 width/height）'
                        },
                        width: {
                            type: 'number',
                            description: '窗口内区域宽度（>0 且 height>0 时只截该子区域）'
                        },
                        height: {
                            type: 'number',
                            description: '窗口内区域高度'
                        },
                        region_x: {
                            type: 'number',
                            description: '绝对屏幕区域 X（mode=region）'
                        },
                        region_y: {
                            type: 'number',
                            description: '绝对屏幕区域 Y（mode=region）'
                        },
                        region_w: {
                            type: 'number',
                            description: '绝对屏幕区域宽度（mode=region）'
                        },
                        region_h: {
                            type: 'number',
                            description: '绝对屏幕区域高度（mode=region）'
                        },
                        format: {
                            type: 'string',
                            enum: ['png', 'jpg'],
                            description: '输出格式，默认 png。内容型截图（文字/界面）建议 png'
                        },
                        note: {
                            type: 'string',
                            description: '本次截图的一句话说明（如「Chrome 主窗口」「任务栏右下角」），会写进给用户的汇报'
                        }
                    },
                    required: []
                }
            }
        }
    ];

    const MAX_WINDOW_LINES = 40;
    function parseArgs(args) {
        if (!args)
            return {};
        if (typeof args === 'string') {
            try {
                const parsed = JSON.parse(args);
                return parsed && typeof parsed === 'object' ? parsed : {};
            }
            catch {
                return {};
            }
        }
        return args;
    }
    function formatWindowLine(item) {
        const flags = [item.minimized ? '最小化' : '', item.foreground ? '焦点' : ''].filter(Boolean).join('｜');
        const process = item.process ? `（${item.process}）` : '';
        return `- handle=${item.handle}｜${item.title}${process}｜窗口 ${item.width}x${item.height}｜客户区 ${item.client_w}x${item.client_h}`
            + (flags ? `｜${flags}` : '');
    }
    function describeCaptureTargets(args) {
        const parsed = parseArgs(args);
        const includeMinimized = parsed.include_minimized === true;
        const [displays, displayErr] = screenshotGetDisplays();
        const [windows, windowErr] = screenshotListWindows();
        const sections = [];
        if (displayErr)
            sections.push(`显示器枚举失败：${displayErr.message || String(displayErr)}`);
        else if (displays && displays.length > 0) {
            sections.push(`显示器 ${displays.length} 个：`);
            displays.forEach(d => {
                sections.push(`- index=${d.index}｜${d.width}x${d.height}｜屏幕原点 (${d.x}, ${d.y})`);
            });
        }
        if (windowErr)
            sections.push(`窗口枚举失败：${windowErr.message || String(windowErr)}`);
        else {
            const list = (windows || []).filter(item => includeMinimized || !item.minimized);
            if (list.length === 0)
                sections.push('没有可截图的可见窗口（可用 mode=fullscreen 截整屏）。');
            else {
                const shown = list.slice(0, MAX_WINDOW_LINES);
                sections.push(`可见窗口 ${list.length} 个：`);
                shown.forEach(item => sections.push(formatWindowLine(item)));
                if (list.length > shown.length)
                    sections.push(`……其余 ${list.length - shown.length} 个窗口未列出。`);
            }
        }
        sections.push('提示：按标题截图用 window_title（包含匹配），需要精确锁定用 window_handle；最小化窗口需先恢复才能截取。');
        return sections.join('\n');
    }
    function buildCaptureParams(parsed) {
        const params = {};
        const numberKeys = [
            'display_index', 'window_handle', 'offset_x', 'offset_y', 'width', 'height',
            'region_x', 'region_y', 'region_w', 'region_h'
        ];
        numberKeys.forEach(key => {
            const value = Number(parsed[key]);
            if (Number.isFinite(value))
                params[key] = value;
        });
        if (typeof parsed.mode === 'string' && parsed.mode)
            params.mode = parsed.mode;
        if (typeof parsed.window_title === 'string' && parsed.window_title.trim())
            params.window_title = parsed.window_title.trim();
        if (typeof parsed.format === 'string' && parsed.format)
            params.format = parsed.format;
        if (parsed.client_area === true)
            params.client_area = true;
        return params;
    }
    function persistShot(frame, index) {
        try {
            const base64 = String(frame.base64 || '');
            const payload = base64.includes('base64,') ? base64.split('base64,')[1] : base64;
            if (!payload)
                return '';
            const [decoded, decodeErr] = base64Decode(payload);
            if (decodeErr || !decoded) {
                console.warn('[截图者] base64 解码失败:', decodeErr);
                return '';
            }
            const bytes = new Uint8Array(decoded);
            const ext = frame.format === 'jpg' || frame.format === 'jpeg' ? 'jpg' : 'png';
            const name = `multimedia/screenshot_${Date.now()}_${index}.${ext}`;
            const [, path, error] = saveFile(name, true, bytes);
            if (error) {
                console.warn('[截图者] 落盘失败:', error);
                return '';
            }
            return localPathToReadURL(path);
        }
        catch (error) {
            console.warn('[截图者] 落盘异常:', error);
            return '';
        }
    }
    function captureScreen(args) {
        const parsed = parseArgs(args);
        const params = buildCaptureParams(parsed);
        const note = typeof parsed.note === 'string' ? parsed.note.trim() : '';
        const [results, captureErr] = screenshotCapture(params);
        if (captureErr) {
            return { text: `截图失败：${captureErr.message || String(captureErr)}`, shot: null };
        }
        if (!results || results.length === 0) {
            return { text: '截图失败：未获取到图像数据', shot: null };
        }
        const first = results[0];
        const urls = results.map((frame, index) => persistShot(frame, index)).filter(Boolean);
        if (urls.length > 0)
            pushAssistantImage(urls);
        const modeText = String(first.mode || params.mode || 'auto');
        const windowText = first.window_title ? `窗口「${first.window_title}」` : '';
        const sourceSize = first.source_width && first.source_height
            ? `，原图 ${first.source_width}x${first.source_height}`
            : '';
        const frameText = results.length > 1 ? `，共 ${results.length} 帧` : '';
        const label = note ? `${note}：` : '';
        const text = `截图完成。${label}模式=${modeText}${windowText ? '，' + windowText : ''}，`
            + `输出 ${first.width}x${first.height}${sourceSize}${frameText}，图片已展示给用户。`;
        return {
            text,
            shot: {
                note: note || `${modeText} 截图`,
                mode: modeText,
                windowTitle: first.window_title || '',
                width: first.width,
                height: first.height,
                base64: first.base64,
                url: urls[0] || '',
            }
        };
    }

    function collectScreenshotShot(toolCall, shots, pending) {
        if (!pending)
            return;
        shots.push(pending);
    }
    function pickPrimaryShot(shots) {
        return shots.length > 0 ? shots[shots.length - 1] : null;
    }
    function buildScreenshotSummary(shots) {
        if (shots.length === 0)
            return '月华没有截到画面';
        const primary = shots[shots.length - 1];
        const target = primary.windowTitle ? `「${primary.windowTitle}」` : '屏幕';
        const size = `${primary.width}x${primary.height}`;
        if (shots.length === 1) {
            return `截好了，${primary.note}（${target}，${size}），已经给你看到了。`;
        }
        return `截好了，一共 ${shots.length} 张，最后一张是${primary.note}（${target}，${size}），都已经给你看到了。`;
    }

    function composeScreenshoterPrompt() {
        return fileView('prompts/screenshoterRole.md')[0].trim();
    }
    class ScreenshoterRole extends CreativeRoleBase {
        roleTag = '截图者';
        MAX_ITERATIONS = 4;
        pendingShot = null;
        primaryImage = '';
        constructor() {
            super(composeScreenshoterPrompt());
        }
        get roleName() { return '截图者'; }
        getToolDefinitions() {
            return screenshoterTools;
        }
        executeTool(toolCall) {
            const funcName = toolCall.function.name;
            let args = {};
            try {
                args = typeof toolCall.function.arguments === 'string'
                    ? JSON.parse(toolCall.function.arguments)
                    : (toolCall.function.arguments || {});
            }
            catch (parseError) {
                console.error(`[截图者] 工具调用参数解析失败:`, toolCall.function.arguments);
                return `工具调用参数解析失败，请确保传入合法的 JSON 字符串。错误: ${parseError}`;
            }
            switch (funcName) {
                case 'list_capture_targets':
                    return describeCaptureTargets(args);
                case 'capture_screen': {
                    const result = captureScreen(args);
                    this.pendingShot = result.shot;
                    return result.text;
                }
                default:
                    return `未知工具: ${funcName}，可用工具为 list_capture_targets、capture_screen`;
            }
        }
        collectDetail(toolCall, shots) {
            collectScreenshotShot(toolCall, shots, this.pendingShot);
            this.pendingShot = null;
        }
        buildSummary(shots) {
            const primary = pickPrimaryShot(shots);
            this.primaryImage = primary?.base64 || '';
            return buildScreenshotSummary(shots);
        }
        takePrimaryImage() {
            const image = this.primaryImage;
            this.primaryImage = '';
            return image;
        }
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

    const AUDIO_FALLBACK_TEXT = '月华听不懂这段语音呢';
    async function understandAudio(audioSource) {
        const cacheable = !audioSource.startsWith('data:');
        const cachedPrompt = cacheable ? getPromptFromKnowledge(audioSource) : '';
        if (cachedPrompt) {
            console.log('[语音] 命中音频缓存，直接返回');
            return cachedPrompt;
        }
        console.log('[语音] 开始转换音频...');
        const [wavBase64, audioError] = audioWav(audioSource);
        if (!wavBase64 || audioError) {
            console.error('[语音] 音频转换失败:', audioError);
            throw new Error('音频转换失败');
        }
        console.log('[语音] 开始语音转写...');
        const transcript = await transcribeAudio(wavBase64);
        if (!transcript.trim()) {
            console.error('[语音] 语音转写为空');
            throw new Error('语音转写为空');
        }
        console.log(`[语音] 语音转写完成: ${transcript.substring(0, 50)}`);
        const text = `【语音转写】${transcript}`;
        if (cacheable) {
            savePromptToKnowledge(audioSource, text);
            console.log('[语音] 语音转写文本已缓存');
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
            console.error('[语音] 语音转写请求失败:', error.message);
            return '';
        }
        const raw = result?.body?.choices?.[0]?.message?.content || '';
        const marker = '<asr_text>';
        return raw.includes(marker) ? raw.substring(raw.indexOf(marker) + marker.length).trim() : raw.trim();
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
            console.error('[语音] 音频理解失败（句柄已保留）:', error);
            return `${media.handle}${AUDIO_FALLBACK_TEXT}`;
        }
    }
    async function understandAudioOrFallback(audioSource) {
        try {
            return await understandAudio(audioSource);
        }
        catch (error) {
            console.error('[语音] 音频理解失败，以兜底文本置换:', error);
            return AUDIO_FALLBACK_TEXT;
        }
    }
    async function resourceAudioOrFallback(audioSource) {
        try {
            const text = await resourceAudio(audioSource, mediaName(audioSource, '.wav'), audioSource.startsWith('data:') ? 'client-upload' : 'channel');
            if (text)
                return text;
        }
        catch (error) {
            console.error('[语音] 音频资源化失败，退化为纯转写文本:', error);
        }
        return understandAudioOrFallback(audioSource);
    }

    const VIDEO_FALLBACK_TEXT = '月华看不了这个视频呢';
    function resourceVideo(originalSource, name, origin) {
        const media = admitMedia(originalSource, name, origin, 'V');
        return media ? media.handle : '';
    }
    function resourceVideoOrFallback(videoSource) {
        try {
            const handle = resourceVideo(videoSource, mediaName(videoSource, '.mp4'), videoSource.startsWith('data:') ? 'client-upload' : 'channel');
            if (handle)
                return handle;
        }
        catch (error) {
            console.error('[视频] 资源化失败:', error);
        }
        return VIDEO_FALLBACK_TEXT;
    }

    const ANIMATED_FALLBACK_TEXT = '月华看不了这个动图呢';
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
                    newContent.push({ type: 'text', text: resourceVideoOrFallback(item.video_url.url) });
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
                    newContent.push({ type: 'text', text: resourceVideoOrFallback(imageSource) });
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
                    newContent.push({ type: 'text', text: handle || ANIMATED_FALLBACK_TEXT });
                    continue;
                }
                newContent.push(item);
            }
            message.content = newContent;
        }
    }

    function describeResource(handle, question = '') {
        const parsed = parseHandle(handle);
        if (!parsed)
            return '';
        const normalized = formatHandle(parsed);
        const cached = getResourceEntry(normalized)?.summary || '';
        if (cached && !question)
            return cached;
        if (parsed.kind === 'I' || parsed.kind === 'V') {
            return describeMedia(normalized, question, cached);
        }
        if (parsed.kind === 'A')
            return cached;
        return cached;
    }
    function inspectTextResource(handle, request) {
        try {
            const [text, error] = descriptorInspect(handle, request);
            if (error) {
                console.error('[描述者] 文件探查失败:', error?.message ?? String(error));
                return '';
            }
            return String(text || '').trim();
        }
        catch (error) {
            console.error('[描述者] 文件探查抛异常:', error);
            return '';
        }
    }
    function describeMedia(handle, question, cached) {
        try {
            const [text, error] = descriptorDescribeMedia(handle, question);
            if (error) {
                console.error('[描述者] 影像描述失败:', error?.message ?? String(error));
                return cached;
            }
            const desc = String(text || '').trim();
            return desc || cached;
        }
        catch (error) {
            console.error('[描述者] 影像描述抛异常:', error);
            return cached;
        }
    }

    const CRYSTAL_TOOL_NAME = 'use_the_program';
    let programs = [];
    let crystalTool = buildCrystalTool(programs);
    function programNames(list) {
        return list.map(program => program.name);
    }
    function buildCrystalTool(list) {
        const names = programNames(list);
        return {
            type: 'function',
            function: {
                name: CRYSTAL_TOOL_NAME,
                description: [
                    '调用星月智能的扩展应用（如文件管理、文件编辑、窗口操作、网络搜索）。',
                    'tool 只能取参数枚举中的应用名，填入即表示「打开这个应用」；instruction 填打开后要该应用完成的任务。',
                    '只打开应用时只填 tool；需要先了解某个应用的能力时传 details=true 与 tool，此时不执行任何操作。'
                ].join(''),
                parameters: {
                    type: 'object',
                    properties: {
                        details: {
                            type: 'boolean',
                            description: '为 true 时只返回 tool 所指应用的详细说明，不执行任何操作；此时仍需传 tool'
                        },
                        tool: {
                            type: 'string',
                            enum: names.length ? names : undefined,
                            description: names.length
                                ? '要使用的应用名，只能取枚举值；只填本项表示打开该应用'
                                : '要使用的应用名（当前没有可用应用）'
                        },
                        instruction: {
                            type: 'string',
                            description: '打开应用后交给它执行的具体任务，如：进入 D 盘根目录；查询北京今天的天气；把 README.md 移动到 docs 目录。只打开应用时省略；不要填「打开文件管理器」这类与 tool 重复的内容'
                        }
                    },
                    required: names.length ? ['tool'] : []
                }
            }
        };
    }
    function fetchCrystalPrograms() {
        try {
            const statusJSON = getLTPXRemoteStatus();
            const status = statusJSON ? JSON.parse(statusJSON) : {};
            if (!status?.tool?.name)
                return null;
            const tools = Array.isArray(status.tools) ? status.tools : [];
            return tools
                .map(item => ({
                name: String(item?.name || '').trim(),
                description: String(item?.description || '').trim()
            }))
                .filter(program => program.name);
        }
        catch (e) {
            console.error('[LTPX] 应用清单拉取失败:', e);
            return null;
        }
    }
    function refreshCrystalPrograms() {
        const latest = fetchCrystalPrograms();
        if (!latest)
            return false;
        const oldNames = programNames(programs);
        const newNames = programNames(latest);
        programs = latest;
        if (oldNames.length === newNames.length && oldNames.every((name, index) => name === newNames[index])) {
            return false;
        }
        crystalTool = buildCrystalTool(programs);
        console.info(`[LTPX] 可用应用清单已更新（${programs.length} 个）：${newNames.join('、') || '无'}`);
        return true;
    }
    function queryCrystalProgramDetails(name) {
        if (!name) {
            return `查询应用详情失败：details=true 时还需传 tool（应用名）。当前可用应用：${programNames(programs).join('、') || '无'}。`;
        }
        let program = programs.find(item => item.name === name);
        if (!program) {
            refreshCrystalPrograms();
            program = programs.find(item => item.name === name);
        }
        if (!program) {
            return `未找到应用 ${name}；当前可用应用：${programNames(programs).join('、') || '无'}。`;
        }
        const description = program.description || '（该应用未提供说明）';
        return `【${program.name}】${description}\n\n用法：只打开该应用传 tool=${program.name}；要它执行任务再填 instruction。`;
    }
    GlobalConfig.LTPfunction.set(CRYSTAL_TOOL_NAME, async (args) => {
        const parsed = typeof args === 'string' ? JSON.parse(args || '{}') : (args || {});
        const tool = String(parsed.tool || '').trim();
        const instruction = String(parsed.instruction || '').trim();
        const detailsArg = parsed.details;
        const detailsName = typeof detailsArg === 'string'
            && detailsArg !== 'true' && detailsArg !== 'false'
            ? detailsArg.trim()
            : '';
        if (detailsArg === true || detailsArg === 'true' || detailsName) {
            return [queryCrystalProgramDetails(detailsName || tool), ''];
        }
        if (!tool) {
            const available = programNames(programs).join('、') || '无';
            return [`调用失败：tool 不能为空，只能取以下应用名：${available}。只打开应用时无需 instruction。`, ''];
        }
        const argsJSON = JSON.stringify({ tool, instruction });
        try {
            return [await callLTPXRemoteTool(CRYSTAL_TOOL_NAME, argsJSON), ''];
        }
        finally {
            refreshCrystalPrograms();
        }
    });
    refreshCrystalPrograms();
    function getCrystalRemoteTools() {
        if (programs.length === 0)
            refreshCrystalPrograms();
        return [crystalTool];
    }

    function normalizeTerms(terms) {
        return [...new Set((terms || []).map(term => String(term || '').trim().toLowerCase()).filter(Boolean))];
    }
    function containsAll(content, terms) {
        if (terms.length === 0)
            return true;
        const lower = content.toLowerCase();
        return terms.every(term => lower.includes(term));
    }
    function containsAny$1(content, terms) {
        if (terms.length === 0)
            return false;
        const lower = content.toLowerCase();
        return terms.some(term => lower.includes(term));
    }
    function searchMemoryGlobal(query, limit = 8, options = {}) {
        const text = String(query || '').trim();
        if (!text)
            return [];
        const mode = options.mode || 'vector';
        const safeLimit = Math.min(Math.max(Math.trunc(limit) || 8, 1), 30);
        const filters = normalizeTerms(options.filters);
        const exclude = normalizeTerms(options.exclude);
        const terms = mode === 'regex' ? [text] : text.split(/\s+/).filter(Boolean);
        if (terms.length === 0)
            return [];
        try {
            const [results, error] = mode === 'vector'
                ? memoryQuery(text, 'text', safeLimit, options.forceGlobal === true)
                : memoryKeywordSearch('text', terms, mode === 'regex', safeLimit);
            if (error || !results) {
                if (error)
                    console.error('[记忆检索] 查询失败:', error?.message ?? String(error));
                return [];
            }
            const hits = [];
            for (const item of results) {
                const content = String(item.content || '').trim();
                if (!content)
                    continue;
                if (!containsAll(content, filters))
                    continue;
                if (containsAny$1(content, exclude))
                    continue;
                hits.push({
                    id: String(item.id || ''),
                    type: String(item.type || 'text'),
                    role: String(item.role || 'unknown'),
                    content,
                    similarity: Number(item.similarity) || 0,
                    timestamp: item.timestamp,
                    collection: item.collection,
                    source: item.source,
                });
            }
            const seen = new Set();
            const unique = hits.filter(hit => {
                if (seen.has(hit.content))
                    return false;
                seen.add(hit.content);
                return true;
            });
            const top = unique.slice(0, safeLimit);
            if (top.length > 0) {
                console.log(`[记忆检索] 方式=${mode}，检索「${text.slice(0, 30)}」，返回 ${top.length} 条`);
            }
            return top;
        }
        catch (error) {
            console.error('[记忆检索] 查询异常:', error);
            return [];
        }
    }
    async function queryEmotionSticker(query) {
        if (!query || !query.trim())
            return null;
        try {
            if (!ensureMemoryReady())
                return null;
            const [results, error] = memoryQuery(query.trim(), 'image', 10, true);
            if (error || !results || results.length === 0)
                return null;
            const item = results[RandomFloor(0, results.length - 1)];
            if (item.url)
                return item.url;
            if (item.base64)
                return saveBase64Media(item.base64, 'image/jpeg', 'sticker') || null;
            return null;
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
        const [ok, err] = memoryInit();
        if (err || !ok)
            console.error('记忆库初始化失败:', err ?? '返回失败');
        else
            GlobalConfig.memoryReady = true;
    }
    function ensureMemoryReady() {
        if (!GlobalConfig.memoryReady)
            initMemory();
        return GlobalConfig.memoryReady;
    }

    const DEFAULT_TOP_K = 8;
    const MAX_TOP_K = 30;
    const CONTENT_PREVIEW_CHARS = 400;
    const FALLBACK_MAX_TERMS = 8;
    const memoryTools = [
        {
            type: 'function',
            function: {
                name: 'search_memory',
                description: '从长期记忆库检索历史对话与知识片段。请注意：这是聊天记忆检索，不是文档资源检索——查文件/资料请用 browse_resources。',
                parameters: {
                    type: 'object',
                    properties: {
                        keywords: {
                            type: 'array',
                            items: { type: 'string' },
                            description: '用自然语言描述或句柄视频链接或BV号均可，提供 1-4 种不同表述能显著提高命中率'
                        },
                        exclude: {
                            type: 'array',
                            items: { type: 'string' },
                            description: '结果正文命中任意一个词就丢弃（如 ["表情包", "图片"]），用于排除噪音'
                        },
                        topK: {
                            type: 'number',
                            description: `希望返回的条目数，默认 ${DEFAULT_TOP_K}，最大 ${MAX_TOP_K}`
                        }
                    },
                    required: ['keywords']
                }
            }
        }
    ];
    function safeParse$1(text) {
        try {
            const parsed = JSON.parse(text);
            return parsed && typeof parsed === 'object' ? parsed : {};
        }
        catch {
            return {};
        }
    }
    function roleLabel(role) {
        switch (role) {
            case 'user': return '用户';
            case 'assistant': return '月华';
            case 'system': return '系统';
            case 'tool': return '工具';
            case 'image': return '图片';
            default: return role || '未知';
        }
    }
    function formatTime(timestamp) {
        if (!timestamp)
            return '时间未知';
        try {
            return new Date(timestamp * 1000).toLocaleString('zh-CN', { hour12: false });
        }
        catch {
            return '时间未知';
        }
    }
    function preview(content) {
        return content.length > CONTENT_PREVIEW_CHARS
            ? content.slice(0, CONTENT_PREVIEW_CHARS) + '…'
            : content;
    }
    function parseTerms(value) {
        const list = Array.isArray(value) ? value : (typeof value === 'string' ? [value] : []);
        return list.map(item => String(item || '').trim()).filter(Boolean);
    }
    function parseTopK(value) {
        const raw = Number(value);
        return Number.isFinite(raw) ? Math.min(Math.max(Math.trunc(raw), 1), MAX_TOP_K) : DEFAULT_TOP_K;
    }
    function containsAny(content, terms) {
        if (terms.length === 0)
            return false;
        const lower = content.toLowerCase();
        return terms.some(term => lower.includes(term));
    }
    function extractVideoKeys(keywords) {
        const keys = new Set();
        for (const keyword of keywords) {
            for (const match of String(keyword || '').matchAll(/BV[0-9A-Za-z]{10}/g)) {
                keys.add(match[0]);
            }
        }
        return [...keys];
    }
    function findVideoSummaries(keys) {
        if (!keys.length)
            return [];
        try {
            const [entries, error] = knowledgeLoad('video_summary');
            if (error || !Array.isArray(entries))
                return [];
            const found = [];
            for (const pair of entries) {
                if (!Array.isArray(pair) || typeof pair[0] !== 'string')
                    continue;
                const key = pair[0];
                const upperKey = key.toUpperCase();
                if (!keys.some(id => upperKey.includes(id.toUpperCase())))
                    continue;
                const text = typeof pair[1] === 'string' ? pair[1] : String(pair[1] ?? '');
                if (text.trim())
                    found.push(`[已看视频摘要] ${key}\n${preview(text)}`);
            }
            return found;
        }
        catch (error) {
            console.error('[记忆检索] 视频摘要召回失败:', error);
            return [];
        }
    }
    function vectorSearch(keywords, topK, exclude) {
        const merged = new Map();
        for (const keyword of keywords) {
            try {
                const [results, error] = memoryQuery(keyword, 'text', topK, false);
                if (error || !results)
                    continue;
                for (const item of results) {
                    const content = String(item.content || '').trim();
                    if (!content)
                        continue;
                    if (containsAny(content, exclude))
                        continue;
                    const id = String(item.id || '');
                    if (!id)
                        continue;
                    const hit = {
                        id,
                        type: String(item.type || 'text'),
                        role: String(item.role || 'unknown'),
                        content,
                        similarity: Number(item.similarity) || 0,
                        timestamp: item.timestamp,
                        collection: item.collection,
                        source: item.source
                    };
                    const existing = merged.get(id);
                    if (existing) {
                        existing.hitCount++;
                        if (hit.similarity > existing.hit.similarity)
                            existing.hit = hit;
                    }
                    else {
                        merged.set(id, { hit, hitCount: 1 });
                    }
                }
            }
            catch (error) {
                console.error('[记忆检索] 向量查询失败:', error);
            }
        }
        return [...merged.values()]
            .sort((a, b) => b.hit.similarity - a.hit.similarity)
            .map(entry => entry.hit);
    }
    function keywordFallback(keywords, topK, exclude, knownIds) {
        const terms = [];
        const seen = new Set();
        for (const keyword of keywords) {
            const text = String(keyword || '').trim();
            if (!text)
                continue;
            if (text.length <= 20) {
                if (!seen.has(text)) {
                    seen.add(text);
                    terms.push(text);
                }
            }
            else {
                for (const part of text.split(/\s+/)) {
                    const word = part.trim();
                    if (word && word.length >= 2 && !seen.has(word)) {
                        seen.add(word);
                        terms.push(word);
                    }
                }
            }
        }
        if (!terms.length)
            return [];
        const limited = terms.slice(0, FALLBACK_MAX_TERMS);
        const merged = new Map();
        for (const term of limited) {
            try {
                const [results, error] = memoryKeywordSearch('text', [term], false, topK * 2);
                if (error || !results)
                    continue;
                for (const item of results) {
                    const content = String(item.content || '').trim();
                    if (!content)
                        continue;
                    if (containsAny(content, exclude))
                        continue;
                    const id = String(item.id || '');
                    if (!id || knownIds.has(id))
                        continue;
                    const existing = merged.get(id);
                    if (existing)
                        existing.hitCount++;
                    else {
                        merged.set(id, {
                            hit: {
                                id,
                                type: String(item.type || 'text'),
                                role: String(item.role || 'unknown'),
                                content,
                                similarity: Number(item.similarity) || 0,
                                timestamp: item.timestamp,
                                collection: item.collection,
                                source: item.source
                            },
                            hitCount: 1
                        });
                    }
                }
            }
            catch (error) {
                console.error('[记忆检索] 关键词补足失败:', error);
            }
        }
        return [...merged.values()]
            .sort((a, b) => b.hitCount - a.hitCount || (b.hit.timestamp ?? 0) - (a.hit.timestamp ?? 0))
            .map(entry => entry.hit);
    }
    async function handleSearchMemory(args) {
        const parsed = typeof args === 'string' ? safeParse$1(args) : (args || {});
        const keywords = parseTerms(parsed.keywords);
        if (!keywords.length)
            return ['记忆检索失败：keywords 不能为空，请提供至少一个查询词', ''];
        const topK = parseTopK(parsed.topK);
        const exclude = parseTerms(parsed.exclude);
        if (!ensureMemoryReady()) {
            return ['记忆库未就绪，无法检索。请稍后再试一次。', ''];
        }
        const vectorHits = vectorSearch(keywords, topK, exclude);
        const knownIds = new Set(vectorHits.map(hit => hit.id));
        const fallbackHits = vectorHits.length >= topK
            ? []
            : keywordFallback(keywords, topK, exclude, knownIds);
        const hits = [...vectorHits, ...fallbackHits].slice(0, topK);
        const videoSummaries = findVideoSummaries(extractVideoKeys(keywords));
        if (hits.length === 0 && videoSummaries.length === 0) {
            return ['记忆库中没有检索到相关记录。可以换一种表述（如具体的人名、事件名）再试一次；不要编造记忆内容。', ''];
        }
        const sections = [];
        if (hits.length > 0) {
            const lines = hits.map((hit, index) => {
                const meta = `角色=${roleLabel(hit.role)}｜时间=${formatTime(hit.timestamp)}｜相关度=${hit.similarity.toFixed(3)}`;
                return `[${index + 1}] ${meta}\n${preview(hit.content)}`;
            });
            const viaFallback = fallbackHits.length > 0 && vectorHits.length < topK
                ? `（语义检索 ${vectorHits.length} 条 + 关键词补足 ${fallbackHits.length} 条）`
                : '';
            sections.push(`检索到 ${hits.length} 条相关记忆${viaFallback}：\n${lines.join('\n\n')}`);
        }
        if (videoSummaries.length > 0) {
            sections.push(`已看视频摘要 ${videoSummaries.length} 条（月华自己刷B站时留下的观看理解）：\n${videoSummaries.join('\n\n')}`);
        }
        const text = sections.join('\n\n')
            + '\n\n只依据以上原文回答；原文不足以回答用户问题时，直接说明没有找到相关信息，不要编造或把无关内容当答案。';
        return [text, ''];
    }
    GlobalConfig.LTPfunction.set('search_memory', handleSearchMemory);

    const READ_CHAR_BUDGET = 12000;
    function kindLabel(kind) {
        return KIND_LABELS[kind] ?? kind;
    }
    function stateNote(entry) {
        return entry.state === 'active' ? '' : ` [${LOST_FILE_NOTICE}]`;
    }
    function previewSummary(text, maxChars) {
        const clean = String(text || '').replace(/\s+/g, ' ').trim();
        return clean.length > maxChars ? `${clean.slice(0, maxChars).trim()}…` : clean;
    }
    function entryLine(entry) {
        const desc = entry.summary ? `：${previewSummary(entry.summary, 80)}` : '';
        const sectionNote = entry.section_count > 0 ? ` ${entry.section_count}节` : '';
        return `${entry.handle} [${kindLabel(entry.kind)}] ${entry.title || entry.name}${sectionNote}${stateNote(entry)}${desc}`;
    }
    function finishBlocks(blocks, missing) {
        if (blocks.length === 0) {
            return [`取回失败：${missing.join('、')} —— ${HANDLE_INVALID_NOTICE}。请先用 browse_resources 确认可用句柄`, ''];
        }
        let text = blocks.join('\n\n---\n\n');
        if (missing.length > 0)
            text += `\n\n以下句柄无法取回：${missing.join('、')}（${HANDLE_INVALID_NOTICE}）`;
        return [text, ''];
    }
    function listResourcesText(kind, limit) {
        const entries = listResources(kind, limit);
        const valid = [];
        let purged = 0;
        for (const entry of entries) {
            if (entry.state === 'lost') {
                if (deleteResource(entry.handle))
                    purged++;
                continue;
            }
            valid.push(entry);
        }
        if (valid.length === 0) {
            const purgedNote = purged > 0 ? `（已自动清理 ${purged} 条源文件丢失的失效句柄）` : '';
            return `资源目录为空：当前没有已收录的资源${purgedNote}。用户可以上传图片、发送文本文件，或让我生成图片后再引用`;
        }
        const stats = resourceStatistics();
        const lines = valid.map(entryLine);
        const purgeNote = purged > 0 ? `；已自动清理 ${purged} 条源文件丢失的失效句柄` : '';
        const head = `当前可用资源（共 ${valid.length} 条，按收录时间倒序，越靠前越新${purgeNote}${stats ? `；目录总计 ${stats.total} 条 / ${formatBytes(stats.bytes)}` : ''}）：`;
        const tail = `\n${grammarCheatsheet()}。需要内容时用 use_resource 按句柄取回；文本可写成 @D.003&需求，让描述者探查摘要、结构或引用关系。`;
        return `${head}\n${lines.join('\n')}${tail}`;
    }
    function searchResourcesText(query, kind, limit) {
        const hits = searchResources(query, kind, limit);
        if (hits.length === 0) {
            return `未找到与「${query}」相关的资源。可用 browse_resources 查看全部资源，或让用户重新提供资料`;
        }
        const lines = [];
        for (const hit of hits) {
            if (hit.sections && hit.sections.length > 0) {
                const sectionLines = hit.sections.slice(0, 5).map(section => {
                    const parsed = parseHandle(hit.handle);
                    const anchor = parsed ? sectionHandle(parsed.kind, parsed.seq, section.ordinal) : hit.handle;
                    return `    ${anchor} ${section.path || section.title || `第 ${section.ordinal} 段`}（第 ${section.line_start}-${section.line_end} 行）`;
                });
                lines.push(`${hit.handle} [${kindLabel(hit.kind)}] ${hit.title}${hit.summary ? `：${previewSummary(hit.summary, 120)}` : ''}\n  命中章节：\n${sectionLines.join('\n')}`);
            }
            else {
                lines.push(`${hit.handle} [${kindLabel(hit.kind)}] ${hit.title}${hit.summary ? `：${previewSummary(hit.summary, 120)}` : ''}`);
            }
        }
        return `与「${query}」相关的资源（共 ${hits.length} 条）：\n${lines.join('\n')}\n\n下一步：用 use_resource 按上述句柄取回正文`;
    }
    function readHandleBlock(rawHandle, ask, missing) {
        const parsedHandle = parseHandle(rawHandle);
        if (!parsedHandle) {
            missing.push(rawHandle || '(空)');
            return '';
        }
        const normalized = formatHandle(parsedHandle);
        const entry = getResourceEntry(normalized);
        if (!entry) {
            missing.push(normalized);
            return '';
        }
        if (entry.state === 'lost') {
            deleteResource(normalized);
            return `【${normalized}】\n${LOST_FILE_NOTICE}`;
        }
        if (parsedHandle.kind === 'I' || parsedHandle.kind === 'V' || parsedHandle.kind === 'A') {
            const desc = describeResource(normalized, ask);
            if (!desc && !getResourceEntry(normalized)) {
                return `【${normalized}】\n${LOST_FILE_NOTICE}`;
            }
            return `【${normalized}｜${kindLabel(parsedHandle.kind)}｜${entry.title || entry.name}】\n${desc || '（暂无描述，且本体不可直接解读）'}`;
        }
        if (ask && !parsedHandle.anchor) {
            const answer = inspectTextResource(normalized, ask);
            if (!answer) {
                return `【${normalized}｜${entry.title || entry.name}】\n探查失败：描述者没有给出答复，可改用 "${normalized}|L起始-结束" 直接取原文`;
            }
            return `【${normalized}｜${entry.title || entry.name}】\n${answer}`;
        }
        const result = readResource(normalized, READ_CHAR_BUDGET);
        if (!result) {
            missing.push(normalized);
            return '';
        }
        if (result.lost) {
            deleteResource(normalized);
            return `【${normalized}｜${result.title}】\n${LOST_FILE_NOTICE}`;
        }
        const rangeNote = result.line_start && result.line_end ? `｜第 ${result.line_start}-${result.line_end} 行` : '';
        const sectionNote = result.section_title ? `｜${result.section_title}` : '';
        const truncated = result.truncated ? '\n（内容较长已截断；可带 |L起-止 或 |§n 定向读取）' : '';
        return `【${normalized}${sectionNote}${rangeNote}｜共 ${result.total_lines || 0} 行】\n${result.content || ''}${truncated}`;
    }
    async function handleUseResource(args) {
        const parsed = typeof args === 'string' ? safeParse(args) : (args || {});
        const rawHandles = Array.isArray(parsed.handles)
            ? parsed.handles
            : (parsed.handle ? [parsed.handle] : []);
        if (rawHandles.length === 0) {
            return ['请提供资源句柄，例如 "@D.003" 或 "@D.003|L11-99&这段讲了什么"', ''];
        }
        const globalAsk = String(parsed.ask || '').trim();
        const blocks = [];
        const missing = [];
        for (const raw of rawHandles) {
            const query = parseResourceQuery(String(raw || ''));
            const block = readHandleBlock(query.handle, globalAsk || query.ask, missing);
            if (block)
                blocks.push(block);
        }
        return finishBlocks(blocks, missing);
    }
    async function handleBrowseResources(args) {
        const parsed = typeof args === 'string' ? safeParse(args) : (args || {});
        const kind = String(parsed.kind || '').trim().toUpperCase();
        if (kind && !KIND_LABELS[kind]) {
            return [`类型码无效：${kind}。可用类型码：D 文档 / I 图片 / V 视频 / A 音频 / T 表格`, ''];
        }
        const query = String(parsed.query || '').trim();
        const fallback = query ? 10 : 30;
        const limit = Math.min(Math.max(Number(parsed.limit) || fallback, 1), query ? 50 : 200);
        if (query)
            return [searchResourcesText(query, kind, limit), ''];
        return [listResourcesText(kind, limit), ''];
    }
    async function handleListResources(args) {
        const parsed = typeof args === 'string' ? safeParse(args) : (args || {});
        const kind = String(parsed.kind || '').trim().toUpperCase();
        if (kind && !KIND_LABELS[kind]) {
            return [`类型码无效：${kind}。可用类型码：D 文档 / I 图片 / V 视频 / A 音频 / T 表格`, ''];
        }
        const limit = Math.min(Math.max(Number(parsed.limit) || 30, 1), 200);
        return [listResourcesText(kind, limit), ''];
    }
    async function handleSearchResources(args) {
        const parsed = typeof args === 'string' ? safeParse(args) : (args || {});
        const query = String(parsed.query || '').trim();
        if (!query)
            return ['请提供搜索关键词', ''];
        const kind = String(parsed.kind || '').trim().toUpperCase();
        const limit = Math.min(Math.max(Number(parsed.limit) || 10, 1), 50);
        return [searchResourcesText(query, kind, limit), ''];
    }
    async function handleReadResource(args) {
        const parsed = typeof args === 'string' ? safeParse(args) : (args || {});
        const rawHandles = Array.isArray(parsed.handles)
            ? parsed.handles
            : (parsed.handle ? [parsed.handle] : []);
        if (rawHandles.length === 0) {
            return ['请提供资源句柄，例如 ["@D.003"]。可用清单见 browse_resources', ''];
        }
        const question = String(parsed.question || '').trim().slice(0, 200);
        const blocks = [];
        const missing = [];
        for (const raw of rawHandles) {
            const query = parseResourceQuery(String(raw || ''));
            const block = readHandleBlock(query.handle, question || query.ask, missing);
            if (block)
                blocks.push(block);
        }
        return finishBlocks(blocks, missing);
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
                name: 'use_resource',
                description: `按句柄读取或探查资源。每个句柄可写成 @D.003、@D.003|§4、@D.003|L120-168，也可以写成 @D.003&自然语言需求（或把需求放在 ask 字段）。规则：文档/表格带锚点或没有 ask 时返回原文（带锚点即该段原文）；文档/表格不带锚点且带 ask 时交给描述者探查摘要、结构大纲、引用关系；图片/动图/视频返回描述——已有描述直接返回，没有则由描述者现场生成并入库；音频返回已有转写。${grammarCheatsheet()}。`,
                parameters: {
                    type: 'object',
                    properties: {
                        handles: {
                            type: 'array',
                            items: { type: 'string' },
                            description: '资源句柄数组，如 ["@D.003", "@D.003|§4", "@D.003|L120-168&这段讲了什么", "@I.007", "@V.001"]'
                        },
                        ask: {
                            type: 'string',
                            description: '可选：对所有句柄生效的自然语言需求；单个句柄也可用 handle&需求 的内联写法'
                        }
                    },
                    required: ['handles']
                }
            }
        },
        {
            type: 'function',
            function: {
                name: 'browse_resources',
                description: '浏览资源目录或跨资源检索。不传 query 时按 kind 列出句柄 + 类型 + 标题/描述 + 内容状态；传 query 时在标题、摘要、标签与段落标题中匹配，返回「哪份资源的哪一节」命中了。本工具只做资源目录，不检索聊天记忆——问「谁之前说过什么 / 以前聊过什么」请用 search_memory。拿到句柄后用 use_resource 读取。',
                parameters: {
                    type: 'object',
                    properties: {
                        query: {
                            type: 'string',
                            description: '可选：搜索关键词；省略时按目录列出'
                        },
                        kind: {
                            type: 'string',
                            description: '可选：只处理某一类资源，取 D 文档 / I 图片 / V 视频 / A 音频 / T 表格'
                        },
                        limit: {
                            type: 'number',
                            description: '返回条目数量上限；列目录默认 30，检索默认 10'
                        }
                    },
                    required: []
                }
            }
        }
    ];
    GlobalConfig.LTPfunction.set('use_resource', handleUseResource);
    GlobalConfig.LTPfunction.set('browse_resources', handleBrowseResources);
    GlobalConfig.LTPfunction.set('list_resources', handleListResources);
    GlobalConfig.LTPfunction.set('read_resource', handleReadResource);
    GlobalConfig.LTPfunction.set('search_resources', handleSearchResources);

    function parseToolArgs(raw) {
        if (raw && typeof raw === 'object')
            return raw;
        try {
            const parsed = JSON.parse(String(raw ?? '{}'));
            return parsed && typeof parsed === 'object' ? parsed : {};
        }
        catch {
            return {};
        }
    }
    function describeError(error) {
        if (!error)
            return '未知错误';
        if (typeof error === 'string')
            return error;
        const anyError = error;
        if (typeof anyError.message === 'string' && anyError.message)
            return anyError.message;
        const text = String(error);
        return text && text !== '[object Object]' ? text : '模型调用失败（宿主未提供错误详情）';
    }
    function notifyAgentSuccess() {
        try {
            if (typeof notifyAgentIdle === 'function')
                notifyAgentIdle();
        }
        catch {
        }
    }
    function reportAgentFailure(reason) {
        try {
            if (typeof notifyAgentFailure === 'function')
                notifyAgentFailure(reason);
        }
        catch {
        }
    }

    function applyModelResponse(message, cache) {
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

    function demoteOverflowImagesIn(messages, window, describe) {
        const countImages = () => messages.reduce((sum, message) => typeof message.content === 'string'
            ? sum
            : sum + message.content.filter((c) => c.type === 'image_url').length, 0);
        for (let i = 0; i < messages.length && countImages() > window; i++) {
            const message = messages[i];
            if (typeof message.content === 'string')
                continue;
            if (!message.content.some((c) => c.type === 'image_url'))
                continue;
            demoteMessageImagesIn(message, describe);
        }
    }
    function demoteMessageImagesIn(message, describe) {
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
            rebuilt.push({
                type: 'text', text: tag
                    ? composeImagePlaceholder(tag, describe(tag, content.image_url?.url || ''))
                    : '（一张未能登记句柄的图片，已超出窗口）'
            });
        }
        message.content = rebuilt;
    }

    const ACK_LEADS = ['收到', '好嘞', '交给我吧', '没问题', '这个我来'];
    const ACK_ENDS = ['～', '，稍等我一下～', '，等我好消息～', '，一会儿就好～'];
    const DRAW_ACTIONS = ['这就动笔', '这就开画', '马上开工', '先起个稿'];
    const SEARCH_ACTIONS = ['这就去翻资料', '这就去查', '马上去找', '先去翻一翻'];
    function pickAckPart(parts) {
        return parts[RandomFloor(0, parts.length - 1)];
    }
    function composeAck(actions) {
        return `${pickAckPart(ACK_LEADS)}！${pickAckPart(actions)}${pickAckPart(ACK_ENDS)}`;
    }
    function slowTaskAckText(functionName, rawArgs) {
        if (functionName === 'dispatch_painter')
            return composeAck(DRAW_ACTIONS);
        if (functionName === 'use_the_program') {
            const target = String(parseToolArgs(rawArgs).tool || '').trim().toLowerCase();
            if (target === 'web_search')
                return composeAck(SEARCH_ACTIONS);
        }
        return '';
    }
    function maybeSendFastAck(functionName, rawArgs) {
        if (GlobalConfig.fastAckSent)
            return;
        const text = slowTaskAckText(functionName, rawArgs);
        if (!text)
            return;
        GlobalConfig.fastAckSent = true;
        let audio = '';
        try {
            const [voice, error] = tts(text);
            if (!error && voice)
                audio = voice;
        }
        catch (error) {
            console.error('[对话者] 快速应答 TTS 失败，仅推送文本:', error);
        }
        try {
            pushAssistantContext('text', text, audio, GlobalConfig.turnFromQQ);
            console.log(`[对话者] 已发送高耗时任务快速应答: ${text}`);
        }
        catch (error) {
            console.error('[对话者] 快速应答推送失败:', error);
        }
    }

    async function executeToolCalls(messages, state) {
        let hasToolCalls = false;
        for (const toolCall of state.toolCalls) {
            const functionName = toolCall.function.name;
            const functionArgs = toolCall.function.arguments;
            const lunarToolPackage = GlobalConfig.LTPfunction.get(functionName);
            if (!lunarToolPackage) {
                messages.push({ role: "tool", content: `未找到工具包: ${functionName}`, tool_call_id: toolCall.id });
                continue;
            }
            maybeSendFastAck(functionName, functionArgs);
            try {
                const toolResult = await lunarToolPackage(functionArgs);
                const textContent = Array.isArray(toolResult) ? toolResult[0] : String(toolResult);
                const base64Image = Array.isArray(toolResult) ? toolResult[1] : '';
                messages.push({ role: "tool", content: textContent, tool_call_id: toolCall.id });
                if (base64Image && typeof base64Image === 'string' && base64Image.length > 0) {
                    const message = {
                        role: "user",
                        content: [
                            { type: "text", text: `【工具返回的图片】${textContent}` },
                            { type: "image_url", image_url: { url: base64Image } }
                        ]
                    };
                    attachImageTags(message);
                    messages.push(message);
                    console.log(`[工具调用] ${functionName} 返回图片数据，长度=${base64Image.length} 字节`);
                }
                hasToolCalls = true;
            }
            catch (error) {
                messages.push({ role: "tool", content: `调用${functionName}失败: ${error}`, tool_call_id: toolCall.id });
            }
        }
        state.currentToolCallIndex = -1;
        state.currentFunctionArgs = "";
        state.currentFunctionName = "";
        state.currentToolCall = null;
        state.toolCalls = [];
        return hasToolCalls;
    }

    const dialogueBaseTools = [...agentControlTools, ...memoryTools, ...resourceTools, ...scheduleTools];
    const MAX_TOOL_ROUNDS = 5;
    const ACTION_CLAIM_PATTERN = /(已经|已|这就|马上|现在)[^，。！？\n]{0,12}(打开|启动|运行|调出|叫出|切换|准备好|准备完毕)/;
    const ACTION_CLAIM_GUARD = '【系统校验】你刚才声称已经完成了打开/启动类操作，但本轮没有调用任何工具，用户看不到任何变化。请直接调用 use_the_program 执行（只打开应用只填 tool），或如实说明没有完成，不要只口头宣称。';
    const BUILTIN_CHARACTER_NAMES = new Set(['月华', '琉璃']);
    function buildFriendsAppearanceSection() {
        const lines = [];
        const characters = [...loadCustomCharacters().entries()]
            .sort((a, b) => a[0].localeCompare(b[0], 'zh-CN'));
        for (const [name, profile] of characters) {
            if (BUILTIN_CHARACTER_NAMES.has(name))
                continue;
            const appearance = String(profile.appearance || '').trim();
            if (!appearance)
                continue;
            lines.push(`- ${name}：${appearance}`);
        }
        return lines.length > 0 ? `## 朋友样貌\n\n${lines.join('\n')}\n` : '';
    }
    function applyFriendsAppearance(prompt) {
        const section = buildFriendsAppearanceSection();
        if (!section)
            return prompt.replace(/\n?\{friends-appearance\}\n?/, '\n');
        return prompt.replace('{friends-appearance}', section);
    }
    function composeDialoguePrompt(appearances) {
        const prompt = fileView('prompts/dialogueRole.md')[0]
            .split('{lunar-appearance}').join(appearances.lunar)
            .split('{crystal-appearance}').join(appearances.crystal);
        return applyFriendsAppearance(prompt);
    }
    function readAppendSettings() {
        const settings = GlobalConfig.customConfig?.agent?.append_settings;
        if (!Array.isArray(settings))
            return [];
        return settings
            .map(line => String(line ?? '').replace(/\r\n?/g, '\n').trim())
            .filter(line => line.length > 0);
    }
    class DialogueRole extends ModelBuilder {
        roleTag = "对话者";
        appearances;
        async generateDialogue(cache) {
            this.applyAppendSettings();
            this.toolRounds = 0;
            this.actionClaimGuarded = false;
            GlobalConfig.lastFailure = '';
            await this.continueDialogue(cache);
        }
        async continueDialogue(cache, guardNote = null) {
            try {
                await LiteImageFile();
                GlobalConfig.unreadContext.forEach(context => this.writeContext(context));
                GlobalConfig.unreadContext = [];
                this.formatHistoricalMessages();
                this.runtimeMessages = [{ role: 'user', content: `当前时间: ${new Date().toLocaleString()}` }];
                const lifeProgress = typeof bilibiliLifeProgress === 'function' ? bilibiliLifeProgress() : '';
                if (lifeProgress)
                    this.runtimeMessages.push({ role: 'user', content: `[月华的生活] ${lifeProgress}` });
                const response = this.run(guardNote ? [...this.ragMessages, guardNote] : this.ragMessages, [...dialogueBaseTools, ...getCrystalRemoteTools()]);
                this.analyzeMessageResponse(response.body, cache);
                if (cache.toolCalls.length > 0) {
                    this.writeContext(response.body.choices?.[0]?.message);
                    if (this.toolRounds >= MAX_TOOL_ROUNDS) {
                        console.warn(`[对话者] 工具调用轮次达到上限（${MAX_TOOL_ROUNDS}），停止继续调用`);
                        this.rejectPendingToolCalls(cache);
                        this.finalizeWithoutTools();
                        return;
                    }
                    const hasProcessedToolCalls = await this.batchExecutionToolCall(cache);
                    if (hasProcessedToolCalls) {
                        this.toolRounds++;
                        return await this.continueDialogue(cache);
                    }
                }
                if (!this.actionClaimGuarded
                    && cache.toolCalls.length === 0
                    && this.toolRounds === 0
                    && ACTION_CLAIM_PATTERN.test(cache.descriptionContent)) {
                    this.actionClaimGuarded = true;
                    console.warn('[对话者] 未调用工具却宣称已完成动作，触发纠偏重答');
                    cache.descriptionContent = '';
                    cache.thinkingContent = '';
                    return await this.continueDialogue(cache, { role: 'user', content: ACTION_CLAIM_GUARD });
                }
                this.writeContext(response.body.choices?.[0]?.message);
                notifyAgentSuccess();
            }
            catch (error) {
                const reason = describeError(error);
                console.error('请求处理错误:', reason);
                GlobalConfig.lastFailure = reason;
                reportAgentFailure(reason);
            }
            GlobalConfig.finalResponse = cache.descriptionContent;
        }
        rejectPendingToolCalls(cache) {
            const reason = `本轮工具调用已达上限（${MAX_TOOL_ROUNDS} 次），该调用未执行`;
            for (const toolCall of cache.toolCalls) {
                this.messages.push({ role: 'tool', content: reason, tool_call_id: toolCall.id });
            }
            cache.toolCalls = [];
        }
        finalizeWithoutTools() {
            try {
                const finalResponse = this.run(this.ragMessages, []);
                const content = finalResponse.body?.choices?.[0]?.message?.content;
                if (content) {
                    GlobalConfig.finalResponse = content;
                    this.writeContext({ role: 'assistant', content });
                    return;
                }
            }
            catch (error) {
                console.error('[对话者] 终稿生成失败:', error);
            }
            GlobalConfig.finalResponse = '这轮步骤有点多了，我先停在这里，你可以接着往下说。';
        }
        IMAGE_WINDOW = IMAGE_WINDOW_SIZE;
        toolRounds = 0;
        actionClaimGuarded = false;
        formatHistoricalMessages() {
            if (this.messages.length === 0)
                return;
            this.demoteOverflowImages();
            if (this.messages.length === 0)
                return;
            const lastRole = this.messages.slice(-1)[0].role;
            if (lastRole === 'user' || lastRole === 'tool')
                return;
            this.writeContext({ role: 'user', content: "继续" });
        }
        demoteOverflowImages() {
            demoteOverflowImagesIn(this.messages, this.IMAGE_WINDOW, (tag, source) => this.describeImage(tag, source));
        }
        describeImage(tag, source) {
            const entry = getResourceEntry(tag);
            if (entry?.summary)
                return sanitizeImageDesc(entry.summary, 20);
            return sanitizeImageDesc(describeResource(tag), 20);
        }
        analyzeMessageResponse(message, cache) {
            applyModelResponse(message, cache);
        }
        async batchExecutionToolCall(state) {
            return executeToolCalls(this.messages, state);
        }
        applyAppendSettings() {
            const lines = readAppendSettings();
            const base = composeDialoguePrompt(this.appearances);
            this.systemPrompt = lines.length > 0
                ? base.replace(/\s*$/, '') + '\n' + lines.join('\n')
                : base;
        }
        constructor(appearances) {
            super(composeDialoguePrompt(appearances));
            this.appearances = appearances;
        }
    }

    const RAG_TIMELINE_HARD_LIMIT = 2048;
    const RAG_PER_QUERY_TOP_K = 10;
    const RAG_MAX_RECORDS = 24;
    const RAG_MIN_SIMILARITY = 0.40;
    function memoryQueryText(message) {
        const text = String(message || '').trim();
        const stripped = text.replace(/^(?:\s*\[[^\]]*\])+\s*[:：]?\s*/u, '').trim();
        return stripped || text;
    }
    class MemorizerRole {
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
                const [, error] = memoryAdd(message.role, content);
                if (error)
                    console.error('[记忆] 写入记忆库失败:', error);
                else
                    written++;
            }
            console.log(`[记忆] 已写入 ${written} 条消息到记忆库`);
            GlobalConfig.unreadRecords = [];
        }
        queryRagDigest(userMessages) {
            if (!userMessages || userMessages.length === 0 || !ensureMemoryReady())
                return '';
            let records = this.retrieveRagRecords(userMessages);
            if (records.length === 0)
                return '';
            const feedback = interactEvent('build_memory_before', { userMessages, records }).return;
            if (feedback && Array.isArray(feedback) && feedback.length > 0 && feedback.every(r => r.id && r.role && r.content && r.similarity !== undefined)) {
                records = feedback;
            }
            return this.buildTimeline(records);
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
            const queries = [...new Set(userMessages.map(text => memoryQueryText(text)).filter(text => text.length > 0))].slice(-4);
            const allResults = [];
            for (const query of queries) {
                for (const hit of searchMemoryGlobal(query, RAG_PER_QUERY_TOP_K)) {
                    if (hit.similarity < RAG_MIN_SIMILARITY)
                        continue;
                    allResults.push({
                        id: hit.id,
                        role: hit.role,
                        content: hit.content,
                        similarity: hit.similarity,
                        timestamp: hit.timestamp,
                        collection: hit.collection
                    });
                }
            }
            const seen = new Map();
            for (const r of allResults) {
                const content = r.content || '';
                const existing = seen.get(content);
                if (!existing || r.similarity > existing.similarity)
                    seen.set(content, r);
            }
            const uniqueResults = Array.from(seen.values()).sort((a, b) => b.similarity - a.similarity);
            console.log(`[记忆] 跨集合检索到 ${uniqueResults.length} 条相关记录，相似度范围: ${uniqueResults[0]?.similarity?.toFixed(4) ?? 'N/A'} ~ ${uniqueResults[uniqueResults.length - 1]?.similarity?.toFixed(4) ?? 'N/A'}`);
            return uniqueResults.slice(0, RAG_MAX_RECORDS);
        }
    }

    const memorizerRole = new MemorizerRole();
    const painterRole = new PainterRole();
    const musicianRole = new MusicianRole();
    const screenshoterRole = new ScreenshoterRole();
    const builtinAppearances = {
        lunar: painterRole.prompts.builtinCharacters.get('月华')?.appearance || '',
        crystal: painterRole.prompts.builtinCharacters.get('琉璃')?.appearance || '',
    };
    const dialogueRole = new DialogueRole(builtinAppearances);
    registerAgentTargets({ painterRole, musicianRole, screenshoterRole });
    registerDumpRoles({
        dialogue: { role: dialogueRole, label: '对话者' },
        painter: { role: painterRole, label: '绘制者' },
        musician: { role: musicianRole, label: '演奏者' },
        screenshoter: { role: screenshoterRole, label: '截图者' },
    });
    registerRecoveryRoles({ dialogue: dialogueRole });
    const roleInstances = {
        dialogue: dialogueRole,
        painter: painterRole,
        musician: musicianRole,
        screenshoter: screenshoterRole,
    };
    try {
        const host = globalThis;
        host.agentSystem = Object.assign(host.agentSystem || {}, roleInstances, {
            randomDefaultMessage,
            snapshotContext: snapshotAgentContext,
            restoreContext: restoreAgentContext,
        });
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
            return { thinkingBlocks: [], codeBlocks: [], actionBlocks: [], textChunks: [], displayText: '' };
        const [thinkingBlocks, textAfterThinking] = extractThinkingBlocks(rawText);
        const [codeBlocks, textAfterCode] = extractCodeBlocks(textAfterThinking);
        const [emotionBlocks, textAfterEmotion] = extractEmotionBlocks(textAfterCode);
        const [actionZoneBlocks, textAfterAction] = extractActionBlocks(textAfterEmotion);
        const actionBlocks = [...actionZoneBlocks, ...emotionBlocks];
        const displayText = removeEmojiSymbols(textAfterAction);
        const displayChunks = splitSentences(displayText);
        const textChunks = displayChunks.map(chunk => ({ display: chunk, tts: cleanTextForTTS(chunk), }));
        return { thinkingBlocks, codeBlocks, actionBlocks, textChunks, displayText };
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

    function updatePreviousMemories() {
        if (GlobalConfig.unreadRecords.length >= 1)
            memorizerRole.persistUnreadRecords();
        const userMessages = GlobalConfig.unreadContext.filter(message => message.role === 'user').map(message => extractTextFromMessage(message).trim()).filter(text => text.length > 0);
        const clear = () => { dialogueRole.ragMessages = []; };
        if (userMessages.length === 0)
            return clear();
        const digest = memorizerRole.queryRagDigest(userMessages);
        if (!digest)
            return clear();
        dialogueRole.ragMessages = [{ role: 'user', content: `【长期记忆摘要】\n${digest}` }];
    }

    async function performMatchedAction(query) {
        const bestAction = matchAction(String(query || '').trim());
        if (!bestAction)
            return;
        const pos = getAgentPosition();
        sendToEngine('movement', JSON.stringify({ position: { x: pos.x + RandomFloat(-0.5, 0.5), y: pos.y, z: pos.z + RandomFloat(-0.5, 0.5), }, resumeTracking: true, }));
        sendToEngine('action', JSON.stringify({ action: bestAction }));
        pushAssistantContext('action', `月华${bestAction}了`, '');
    }

    function writeMessage(role, content) {
        if (content && typeof content === 'object' && !Array.isArray(content) && (content.type === 'video_url' || content.type === 'audio_url'))
            content = [content];
        const incoming = { role, content };
        attachImageTags(incoming);
        if (role === 'user')
            appendUserMessage(content);
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

    async function createChatMessage() {
        const cache = { currentToolCallIndex: -1, currentFunctionArgs: '', currentFunctionName: '', descriptionContent: '', thinkingContent: '', currentToolCall: null, toolCalls: [], };
        await dialogueRole.generateDialogue(cache);
        return GlobalConfig.finalResponse;
    }
    const FAILURE_REPLY = '刚才有点走神，你再说一遍好吗？';
    async function dialogueTick() {
        if (GlobalConfig.reasoningInProgress)
            return;
        let incoming = [];
        let ranDialogue = false;
        try {
            flushDueScheduleItems();
            incoming = pullContext();
            if (incoming.length === 0 && GlobalConfig.unreadContext.length === 0) {
                if (typeof bilibiliLifeAutoTick === 'function')
                    bilibiliLifeAutoTick();
                return;
            }
            GlobalConfig.reasoningInProgress = true;
            if (typeof bilibiliLifeSetDialogueBusy === 'function')
                bilibiliLifeSetDialogueBusy(true);
            ranDialogue = true;
            await runDialogueTurn(incoming);
        }
        catch (e) {
            console.error('[对话链] tick 异常:', e);
        }
        finally {
            GlobalConfig.reasoningInProgress = false;
            if (typeof bilibiliLifeSetDialogueBusy === 'function')
                bilibiliLifeSetDialogueBusy(false);
            if (ranDialogue && typeof bilibiliLifeDialogueIdle === 'function')
                bilibiliLifeDialogueIdle();
        }
    }
    async function runDialogueTurn(incoming) {
        GlobalConfig.fastAckSent = false;
        GlobalConfig.turnFromQQ = false;
        try {
            for (const message of incoming) {
                if (message.source === 'qq')
                    GlobalConfig.turnFromQQ = true;
                writeMessage(message.role, message.content);
            }
            guardASRUnload();
            const messageLength = GlobalConfig.unreadContext.length;
            if (messageLength === 0) {
                return;
            }
            updatePreviousMemories();
            const feedback = interactEvent('message_received_before', { messages: GlobalConfig.unreadContext }).return;
            if (feedback && Array.isArray(feedback.messages))
                GlobalConfig.unreadContext = feedback.messages;
            await createChatMessage();
            if (!GlobalConfig.finalResponse.trim().length) {
                const failure = GlobalConfig.lastFailure;
                if (failure) {
                    pushAssistantContext('text', FAILURE_REPLY, '');
                    console.warn('[思考链] 本轮应答失败:', failure);
                }
                else
                    pushAssistantContext('text', randomDefaultMessage(), tts(randomDefaultMessage())[0]);
                return;
            }
            ;
            const { thinkingBlocks, codeBlocks, actionBlocks, textChunks, displayText } = parseContent(GlobalConfig.finalResponse);
            const validMessage = textChunks.map(chunk => chunk.display).join('\n').trim();
            if (!validMessage.length)
                throw new Error('清洗后的文本为空');
            const replyText = displayText.trim() || validMessage;
            if (actionBlocks.length) {
                const feedback = interactEvent('take_action_before', { actions: actionBlocks }).return;
                const actBlocks = (feedback && Array.isArray(feedback.actions)) ? feedback.actions : actionBlocks;
                await performMatchedAction(actBlocks.join(' ').trim() || validMessage);
                pushAssistantImage([await queryEmotionSticker(validMessage)], true);
            }
            else if (validMessage.length <= 36 && Math.random() < 0.15) {
                const feedback = interactEvent('express_emotions_before', { text: validMessage }).return;
                const emoText = (feedback && typeof feedback.text === 'string') ? feedback.text : validMessage;
                pushAssistantImage([await queryEmotionSticker(emoText)], true);
            }
            thinkingBlocks.forEach(thinking => pushAssistantContext('text', thinking, ''));
            codeBlocks.forEach(code => pushAssistantContext('text', code, ''));
            let replyAudio = '';
            const [replyAudioData, replyErr] = tts(cleanTextForTTS(replyText));
            if (!replyErr && replyAudioData)
                replyAudio = replyAudioData;
            pushAssistantContext('text', replyText, replyAudio);
        }
        catch (error) {
            const [promptSound, , , readErr] = readFile('audios/cartoon-fail.mp3');
            if (readErr)
                console.error('读取提示音失败:', readErr);
            console.error('对话链异常:', `${error?.message ?? error}`, ' || ', error?.stack ?? '');
            pushAssistantContext('text', randomDefaultMessage(), promptSound);
        }
        finally {
            persistConversation();
        }
    }

    fetchDocumentCallback('lunar_config.json').then(content => {
        GlobalConfig.customConfig = content;
    });
    bindDumpBridge();
    initConversationCache(messages => dialogueRole.coverContext(messages));
    setInterval(() => { void dialogueTick(); }, 1000);

    exports.LiteImageFile = LiteImageFile;
    exports.ensureMemoryReady = ensureMemoryReady;
    exports.extractTextFromMessage = extractTextFromMessage;
    exports.memorizerRole = memorizerRole;
    exports.musicianRole = musicianRole;
    exports.painterRole = painterRole;
    exports.randomDefaultMessage = randomDefaultMessage;

    return exports;

})({});
