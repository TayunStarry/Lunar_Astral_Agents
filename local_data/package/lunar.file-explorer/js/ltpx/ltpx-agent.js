/**
 * LTPX AtoA 专用智能体模块
 * 「Agent to Agent」：月华将自然语言指令交给本文件管理器的专用智能体，
 * 由大语言模型（LLM）作为最终执行者完成意图识别与文件操作调度。
 *
 * 特性：
 * - 独立上下文历史：保留最近 40 轮对话，超出丢弃最早的轮次
 * - 专用系统提示词：定义「文件管理器执行者」角色与操作边界
 * - 多轮工具调用循环：LLM 通过 OpenAI function calling 逐次调用文件操作工具，
 *   观察工具结果并持续决策，直到给出最终答复
 * - 模型调用走琉璃后端 /v1 代理（OpenAI v1 协议），由后端按 lunar_config.json
 *   解析模型 name/key/url，前端不接触模型配置
 */

// ===== 智能体常量与状态 =====

/** 独立上下文历史：最多保留的对话轮数（1 轮 = 用户指令 + 智能体答复） */
const LTPX_AGENT_MAX_ROUNDS = 40;
/** 单条指令允许的最大工具调用循环次数（防模型无限调用工具） */
const LTPX_AGENT_MAX_TOOL_LOOPS = 8;
/** 提交给模型的目录内容 / 列表类结果截断行数（控制 token 消耗） */
const LTPX_AGENT_MAX_LIST_LINES = 120;

/** 已完成的对话历史：数组元素为 { user, assistant }（各为纯文本） */
let ltpxAgentHistory = [];

/** 模型配置加载 Promise（从 lunar_config.json 的 agent 字段读取，不硬编码） */
let ltpModelConfigPromise = null;

/**
 * 读取 lunar_config.json 的 agent.multimodal_model 作为模型名
 * （通过 /file/read/ 文件接口；读取失败回退默认占位值，仍走同源 /v1 代理解析）
 * @returns {Promise<string>}
 */
async function loadLTPXAgentModel() {
    if (ltpModelConfigPromise) return ltpModelConfigPromise;
    ltpModelConfigPromise = (async () => {
        try {
            const resp = await fetch('/file/read/lunar_config.json', { cache: 'no-store' });
            if (!resp.ok) throw new Error('读取配置失败 HTTP ' + resp.status);
            const cfg = await resp.json();
            const agent = (cfg && cfg.agent) || {};
            return (agent.multimodal_model && String(agent.multimodal_model)) || 'system-multimodal';
        } catch (e) {
            return 'system-multimodal';
        }
    })();
    return ltpModelConfigPromise;
}

// ===== 工具定义（OpenAI function calling 格式） =====

const ltpxAgentTools = [
    {
        type: 'function',
        function: {
            name: 'list_files',
            description: '列出指定目录（默认当前目录）下的文件与子文件夹，并切换工作目录到该目录。目录内容会显示为文本列表供你确认文件名。path 支持：绝对路径（d:/xxx）、含 local_data 前缀的完整路径、根相对路径（images/avatars）或相对当前目录的子目录名（如当前在 images 目录时传 DeepSeek）。',
            parameters: {
                type: 'object',
                properties: {
                    path: { type: 'string', description: '要查看的目录路径（如 images/avatars 或相对当前目录的子目录名），不传则查看当前目录' }
                },
                required: []
            }
        }
    },
    {
        type: 'function',
        function: {
            name: 'view_file',
            description: '预览/打开指定文件：图片、视频、音频打开媒体预览；文本文件打开文本预览；ZIP 压缩包打开压缩包预览（可解压）；GGUF 模型可改用 gguf_metadata。',
            parameters: {
                type: 'object',
                properties: {
                    name: { type: 'string', description: '文件名（含扩展名，如 report.md）' }
                },
                required: ['name']
            }
        }
    },
    {
        type: 'function',
        function: {
            name: 'read_file_content',
            description: '读取文件内容/图片摘要：文本文件返回内容要点，图片文件返回画面内容描述（通过多模态模型解读）；ZIP 压缩包返回包内条目与大小信息；GGUF 模型返回元数据（均会打开对应预览模态框）。当月华想知道某个文件里写了什么、图片里是什么、压缩包里有什么或模型元数据时使用。',
            parameters: {
                type: 'object',
                properties: {
                    name: { type: 'string', description: '文件名（含扩展名，如 README.md 或 photo.png）' }
                },
                required: ['name']
            }
        }
    },
    {
        type: 'function',
        function: {
            name: 'search_files',
            description: '在整个项目文件系统中按名称关键字模糊搜索文件，返回匹配结果列表。',
            parameters: {
                type: 'object',
                properties: {
                    query: { type: 'string', description: '搜索关键字，如 报告' }
                },
                required: ['query']
            }
        }
    },
    {
        type: 'function',
        function: {
            name: 'rename_item',
            description: '重命名当前目录下的文件或文件夹（支持中英文名，建议保留原扩展名）。',
            parameters: {
                type: 'object',
                properties: {
                    name: { type: 'string', description: '要重命名的原文件名（含扩展名）' },
                    new_name: { type: 'string', description: '新的文件名（含扩展名）' }
                },
                required: ['name', 'new_name']
            }
        }
    },
    {
        type: 'function',
        function: {
            name: 'move_items',
            description: '将一个或多个文件/文件夹移动到指定目标目录（目标不存在时自动创建）。',
            parameters: {
                type: 'object',
                properties: {
                    names: {
                        type: 'array',
                        items: { type: 'string' },
                        description: '要移动的文件名列表'
                    },
                    target_dir: { type: 'string', description: '目标目录相对路径，如 docs/archive（空表示根目录）' }
                },
                required: ['names', 'target_dir']
            }
        }
    },
    {
        type: 'function',
        function: {
            name: 'delete_item',
            description: '删除当前目录下的文件或文件夹（不可恢复，仅在指令明确要求删除时使用）。',
            parameters: {
                type: 'object',
                properties: {
                    name: { type: 'string', description: '要删除的文件名（含扩展名）' }
                },
                required: ['name']
            }
        }
    },
    {
        type: 'function',
        function: {
            name: 'compress_items',
            description: '将当前目录下指定的一个或多个文件/文件夹压缩为 ZIP 压缩包（保存在当前目录）。不传 names 则压缩当前目录下全部非压缩文件。',
            parameters: {
                type: 'object',
                properties: {
                    names: {
                        type: 'array',
                        items: { type: 'string' },
                        description: '要压缩的文件名列表；不传则压缩当前目录下全部非压缩文件'
                    },
                    zip_name: { type: 'string', description: '压缩包文件名（建议以 .zip 结尾），如 表情包.zip' }
                },
                required: []
            }
        }
    },
    {
        type: 'function',
        function: {
            name: 'extract_zip',
            description: '解压当前目录下的 ZIP 压缩包到指定目录（默认解压到当前目录）。',
            parameters: {
                type: 'object',
                properties: {
                    name: { type: 'string', description: 'ZIP 压缩包文件名（含扩展名）' },
                    target_dir: { type: 'string', description: '解压目标目录相对路径，不传则解压到当前目录' }
                },
                required: ['name']
            }
        }
    },
    {
        type: 'function',
        function: {
            name: 'create_folder',
            description: '在当前目录下新建文件夹。',
            parameters: {
                type: 'object',
                properties: {
                    name: { type: 'string', description: '文件夹名称' }
                },
                required: ['name']
            }
        }
    },
    {
        type: 'function',
        function: {
            name: 'smart_organize',
            description: '对当前目录启动「智能整理」：由 AI 逐个识别文件并自动移动/重命名到语义文件夹（异步执行，可查看进度）。',
            parameters: {
                type: 'object',
                properties: {},
                required: []
            }
        }
    },
    {
        type: 'function',
        function: {
            name: 'hash_rename',
            description: '对当前目录下全部文件基于内容 MD5 前 16 位批量重命名。',
            parameters: {
                type: 'object',
                properties: {},
                required: []
            }
        }
    },
    {
        type: 'function',
        function: {
            name: 'gguf_metadata',
            description: '打开当前目录下 GGUF 模型文件的元数据预览。',
            parameters: {
                type: 'object',
                properties: {
                    name: { type: 'string', description: 'GGUF 文件名（含扩展名）' }
                },
                required: ['name']
            }
        }
    },
    {
        type: 'function',
        function: {
            name: 'select_item',
            description: '在当前目录中选中指定文件（高亮并计入批量操作）。',
            parameters: {
                type: 'object',
                properties: {
                    name: { type: 'string', description: '要选中的文件名（含扩展名）' }
                },
                required: ['name']
            }
        }
    }
];

// ===== 系统提示词（专用角色） =====

function buildLTPXSystemPrompt() {
    return [
        '你是星月智能「文件管理」的专用智能体（AtoA 执行者），负责解析月华发来的自然语言指令，',
        '并作为文件管理器的最終执行者，通过工具完成文件操作。',
        '',
        '【能力范围】',
        '查看/跳转目录、列出文件、搜索、预览文件（图片/视频/音频/文本/ZIP/GGUF 元数据）、压缩、解压、',
        '移动、重命名、删除、新建文件夹、智能整理、哈希命名、选中文件、读取文件内容/图片摘要（多模态解读）。',
        '',
        '【执行规则】',
        '1. 每轮用户消息会附上【当前目录】【目录内容】【月华指令】；目录可能较长，必要时先用 list_files 进入子目录或刷新确认。',
        '2. 文件名一律以【目录内容】/ list_files 返回的列表为准，使用完整准确的文件名（含扩展名），不要臆造。',
        '3. 找不到目标文件时，先尝试 list_files 或 search_files 确认实际名称，再重试；确实不存在则如实说明。',
        '4. 多步任务拆成多个工具调用依次完成；工具执行结果会回传给你，失败时根据错误调整参数重试或如实汇报。',
        '5. 删除/移动等破坏性操作仅在指令明确要求时执行；指令模糊时在答复中说明并请月华确认。',
        '6. 全部操作完成后，用一两句简洁的中文总结做了什么与结果，不要输出额外内容或代码块。',
        '7. 月华想知道文件内容（如「这个文件写了什么」「图片里是什么」「压缩包里有什么」「模型元数据」）时，',
        '   用 read_file_content 解读：文本/图片走多模态模型，ZIP/GGUF 返回元数据并打开预览模态框；',
        '   若返回「暂时不方便看这个文件」，如实转告月华即可。'
    ].join('\n');
}

// ===== 上下文与消息组装 =====

/**
 * 构造紧凑的目录内容列表文本（供模型识别文件名）
 * @param {Array<Object>} files - 文件列表
 * @param {number} max - 最多展示的条目数
 * @returns {string}
 */
function buildLTPXCompactListing(files, max = LTPX_AGENT_MAX_LIST_LINES) {
    if (!files || files.length === 0) return '（空目录）';
    const sorted = [...files].sort((a, b) => {
        if (a.isDir && !b.isDir) return -1;
        if (!a.isDir && b.isDir) return 1;
        return String(a.name).localeCompare(String(b.name));
    });
    const lines = sorted.slice(0, max).map(f =>
        f.isDir ? `- [目录] ${f.name}/` : `- [文件] ${f.name}（${formatFileSize(f.size || 0)}）`
    );
    if (sorted.length > max) lines.push(`... 共 ${sorted.length} 项，其余可用 list_files 查看`);
    return lines.join('\n');
}

/**
 * 组装本轮用户消息（文本内容数组：当前目录 + 目录内容 + 月华指令）
 */
function buildLTPXUserMessage(fm, instruction) {
    return [
        { type: 'text', text: `【当前目录】${fm.currentPath || '（根目录）'}` },
        { type: 'text', text: `【目录内容】\n${buildLTPXCompactListing(fm.files)}` },
        { type: 'text', text: `【月华指令】${instruction}` }
    ];
}

// ===== 模型调用（OpenAI v1 协议，琉璃后端 /v1 代理解析模型配置） =====

/**
 * 调用 /v1/chat/completions（crystal_astral 将 /v1/ 代理到月华后端，
 * 由后端读取 lunar_config.json 获得模型 name/key/url）
 * @param {Array<Object>} messages - 完整消息数组（含 system / user / assistant / tool）
 * @returns {Promise<Object>} OpenAI 响应中的 message 对象
 */
async function callLTPXAgentModel(messages) {
    const model = await loadLTPXAgentModel();
    const response = await fetch('/v1/chat/completions', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
            model: model,
            messages: messages,
            tools: ltpxAgentTools,
            stream: false
        })
    });
    if (!response.ok) throw new Error(`AI 调用失败: HTTP ${response.status}`);
    const data = await response.json();
    // OpenAI 兼容原始响应
    if (data.choices && data.choices[0] && data.choices[0].message) {
        return data.choices[0].message;
    }
    // 代理包装响应
    if (data.success && data.data && data.data.choices && data.data.choices[0]) {
        return data.data.choices[0].message;
    }
    throw new Error('AI 响应格式异常');
}

/**
 * 安全解析工具参数（兼容 JSON 字符串与已解析对象，解析失败回退空对象）
 */
function safeParseLTPXArgs(jsonStr) {
    if (jsonStr && typeof jsonStr === 'object') return jsonStr;
    if (!jsonStr) return {};
    try {
        const parsed = JSON.parse(jsonStr);
        return parsed && typeof parsed === 'object' ? parsed : {};
    } catch (e) {
        return {};
    }
}

/**
 * 将工具参数中的名称归一化为数组（兼容单字符串与数组）
 */
function toLTPXNameArray(v) {
    if (Array.isArray(v)) return v.map(String).filter(Boolean);
    if (typeof v === 'string' && v.trim()) return [v.trim()];
    return [];
}

// ===== 工具执行器（结构化参数 → 复用现有文件操作实现） =====

/** 列出/切换目录 */
async function execLTPXListFiles(fm, args) {
    const raw = (args && args.path) ? String(args.path).trim() : '';
    // 目标目录候选：绝对路径（盘符 / 斜杠开头 / 含 local_data）归一化为根相对路径；
    // 相对路径优先基于当前工作目录拼合（如当前在 images 传入 DeepSeek → images/DeepSeek），
    // 同时保留根相对路径作为回退（如直接传入 images/avatars），兼容两种写法
    const candidates = [];
    if (raw) {
        if (/^[A-Za-z]:/.test(raw) || raw.startsWith('/') || raw.includes('/local_data')) {
            candidates.push(normalizeLTPXPath(raw));
        } else {
            const base = fm.currentPath ? fm.currentPath.replace(/\/+$/, '') : '';
            candidates.push(base ? `${base}/${raw}` : raw);
            const rootRel = normalizeLTPXPath(raw);
            if (!candidates.includes(rootRel)) candidates.push(rootRel);
        }
    } else {
        candidates.push(fm.currentPath);
    }

    let files = null;
    let path = '';
    let lastErr = null;
    for (const c of candidates) {
        try {
            files = await loadFiles(c);
            path = c;
            break;
        } catch (e) {
            lastErr = e;
        }
    }
    if (!files) throw lastErr || new Error('加载文件失败');

    fm.currentPath = path;
    fm.files = files;
    fm.currentPage = 1;
    fm.isSearching = false;
    fm.updateFileGrid();
    fm.updateStats();
    fm.updateBreadcrumb();
    const lines = formatLTPXList(files, path).split('\n');
    const text = lines.length > LTPX_AGENT_MAX_LIST_LINES + 2
        ? lines.slice(0, LTPX_AGENT_MAX_LIST_LINES).join('\n') + `\n... 共 ${files.length} 项（已截断）`
        : lines.join('\n');
    return { success: true, text: text };
}

/** 预览/打开文件 */
async function execLTPXViewFile(fm, args) {
    const file = matchLTPXFile(fm.files, args.name);
    if (!file) return { success: false, error: `未找到文件「${args.name}」` };
    if (file.isDir) {
        return { success: true, text: `「${file.name}」是文件夹，如需查看内容请调用 list_files` };
    }
    if (isZipFile(file.name)) {
        await showZipModal(file, fm);
        return { success: true, text: `已打开「${file.name}」的压缩包预览` };
    }
    if (isGGUFFile(file.name)) {
        await showGGUFModal(file);
        return { success: true, text: `已打开「${file.name}」的 GGUF 元数据预览` };
    }
    if (isImageFile(file.name) || isVideoFile(file.name) || isAudioFile(file.name)) {
        previewImage(`/file/read/${file.path}`, file.name);
        return { success: true, text: `已打开「${file.name}」的预览` };
    }
    if (isTextFile(file.name)) {
        await showTextModal(file, fm.currentPath, async () => { await fm.loadFiles(); });
        return { success: true, text: `已打开「${file.name}」的文本预览` };
    }
    return { success: true, text: `已定位到「${file.name}」` };
}

/** 读取文件内容/图片摘要（多模态模型解读） */
async function execLTPXReadFileContent(fm, args) {
    const file = matchLTPXFile(fm.files, args.name);
    if (!file) return { success: false, error: `未找到文件「${args.name}」` };
    if (file.isDir) {
        return { success: false, error: `「${file.name}」是文件夹，无法读取内容，如需查看内容请调用 list_files` };
    }

    // 组装多模态消息：图片以 image_url(base64) 提交，文本以内容样本提交
    const userParts = [];
    try {
        if (isImageFile(file.name)) {
            // 通过 /resize 缩放图片为 base64，控制 token 消耗
            const base64 = await resizeImageData(file.path);
            userParts.push({ type: 'text', text: `请解读这张图片「${file.name}」的内容，用简洁中文描述画面主体与细节：` });
            userParts.push({ type: 'image_url', image_url: { url: `data:image/jpeg;base64,${base64}` } });
        } else if (isTextFile(file.name)) {
            const content = await readTextSample(file.path);
            userParts.push({ type: 'text', text: `请阅读文件「${file.name}」并总结其内容要点（简洁中文）：\n\n${content}` });
        } else if (isZipFile(file.name)) {
            // ZIP 压缩包：打开预览模态框，并返回压缩包元数据信息
            showZipModal(file, fm); // 展示预览模态框（内部自行请求并渲染）
            const zipResp = await fetch('/file/archive/metadata', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ path: file.path })
            });
            if (!zipResp.ok) throw new Error('读取压缩包信息失败');
            const zipData = await zipResp.json();
            if (!zipData.success) throw new Error(zipData.error || '读取压缩包信息失败');
            const dirCount = zipData.entries.filter(e => e.isDir).length;
            const fileCount = zipData.file_count - dirCount;
            const lines = [`「${file.name}」的压缩包信息：`];
            lines.push(`- 压缩包大小：${zipFormatSize(zipData.zip_size)}`);
            lines.push(`- 解压后大小：${zipFormatSize(zipData.total_size)}`);
            lines.push(`- 文件数：${fileCount}，文件夹数：${dirCount}`);
            if (zipData.entries.length > 0) {
                lines.push('- 内容条目（前 20 项）：');
                zipData.entries.slice(0, 20).forEach(e => {
                    lines.push(e.isDir ? `  [文件夹] ${e.name}/` : `  [文件] ${e.name}（${zipFormatSize(e.size)}）`);
                });
                if (zipData.entries.length > 20) {
                    lines.push(`  ... 共 ${zipData.entries.length} 个条目，可在预览中查看全部`);
                }
            }
            return { success: true, text: lines.join('\n') };
        } else if (isGGUFFile(file.name)) {
            // GGUF 模型：打开元数据预览模态框，并返回模型元数据信息
            showGGUFModal(file); // 展示元数据预览模态框（内部自行请求并渲染）
            const ggufResp = await fetch('/gguf/metadata', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ filePath: file.path })
            });
            if (!ggufResp.ok) throw new Error('解析 GGUF 元数据失败');
            const ggufData = await ggufResp.json();
            if (!ggufData.success) throw new Error(ggufData.error || '解析 GGUF 元数据失败');
            const lines = [`「${file.name}」的 GGUF 模型元数据：`];
            if (ggufData.summary) {
                const orderedKeys = GGUF_KEY_ORDER.filter(k => ggufData.summary[k]);
                for (const key of Object.keys(ggufData.summary)) {
                    if (!orderedKeys.includes(key)) orderedKeys.push(key);
                }
                for (const key of orderedKeys) {
                    const value = ggufData.summary[key];
                    if (value === undefined || value === '') continue;
                    const label = GGUF_LABEL_MAP[key] || key;
                    lines.push(`- ${label}：${value}`);
                }
            }
            lines.push(`- 元数据共 ${ggufData.count || 0} 项，可在预览中查看全部`);
            return { success: true, text: lines.join('\n') };
        } else {
            // 不支持的文件类型（媒体/二进制等）无法解读
            return { success: false, error: '暂时不方便看这个文件' };
        }
    } catch (e) {
        // 文件读取/预处理失败：无法解读
        return { success: false, error: '暂时不方便看这个文件' };
    }

    // 调用多模态模型解读（与智能整理同一套调用，模型名 system-multimodal，走 /v1 代理）
    try {
        const summary = await callOrganizeAI([
            { role: 'system', content: '你是星月智能的文档/图片解读助手，只输出对内容的简洁中文解读，不要输出其他内容或代码块。' },
            { role: 'user', content: userParts }
        ]);
        if (!summary || !summary.trim()) return { success: false, error: '暂时不方便看这个文件' };
        return { success: true, text: `「${file.name}」的内容解读：\n${summary.trim()}` };
    } catch (e) {
        // 多模态模型调用失败/网络异常：无法完成解读
        return { success: false, error: '暂时不方便看这个文件' };
    }
}

/** 搜索文件 */
async function execLTPXSearch(fm, args) {
    const query = String(args.query || '').trim();
    if (!query) return { success: false, error: '请提供搜索关键字' };
    await fm.searchFiles(query);
    const results = fm.isSearching ? fm.searchResults : [];
    if (results.length === 0) {
        fm.clearSearch();
        return { success: true, text: `未找到与「${query}」相关的文件` };
    }
    const lines = [`搜索「${query}」共找到 ${results.length} 个结果：`];
    results.slice(0, LTPX_AGENT_MAX_LIST_LINES).forEach(f => {
        lines.push(f.isDir ? `- [目录] ${f.path}/` : `- [文件] ${f.path}（${formatFileSize(f.size || 0)}）`);
    });
    if (results.length > LTPX_AGENT_MAX_LIST_LINES) {
        lines.push(`... 其余 ${results.length - LTPX_AGENT_MAX_LIST_LINES} 个结果请在文件管理器中查看`);
    }
    return { success: true, text: lines.join('\n') };
}

/** 重命名 */
async function execLTPXRename(fm, args) {
    const file = matchLTPXFile(fm.files, args.name);
    if (!file) return { success: false, error: `未找到要重命名的文件「${args.name}」` };
    const newName = String(args.new_name || '').trim();
    if (!newName) return { success: false, error: '缺少新名称 new_name' };
    try {
        if (file.isDir) await renameDirectory(file, newName, fm.currentPath);
        else await renameSingleFile(file, newName, fm.currentPath);
        await fm.loadFiles();
        return { success: true, text: `已将「${file.name}」重命名为「${newName}」` };
    } catch (e) {
        return { success: false, error: '重命名失败: ' + (e.message || e) };
    }
}

/** 移动 */
async function execLTPXMove(fm, args) {
    const names = toLTPXNameArray(args.names);
    if (names.length === 0) return { success: false, error: '缺少要移动的文件名' };
    const sources = names
        .map(n => matchLTPXFile(fm.files, n))
        .filter(Boolean)
        .map(f => f.path);
    if (sources.length === 0) return { success: false, error: '未找到要移动的文件' };
    const targetDir = normalizeLTPXPath(args.target_dir || '');
    const result = await callMoveApi(sources, targetDir, 'auto_rename');
    if (!result) return { success: false, error: '移动失败' };
    if (result.success) {
        fm.selectedFiles.clear();
        fm.updateBatchActions();
        await fm.loadFiles();
        return { success: true, text: `已将 ${sources.length} 个项目移动到 ${targetDir || '根目录'}` };
    }
    return { success: false, error: result.error || '移动失败' };
}

/** 删除 */
async function execLTPXDelete(fm, args) {
    const file = matchLTPXFile(fm.files, args.name);
    if (!file) return { success: false, error: `未找到要删除的文件「${args.name}」` };
    const resp = await fetch(`/file/delete/${file.path}`, { method: 'DELETE' });
    if (!resp.ok) return { success: false, error: '删除失败' };
    await fm.loadFiles();
    return { success: true, text: `已删除「${file.name}」` };
}

/** 压缩 */
async function execLTPXCompress(fm, args) {
    const names = toLTPXNameArray(args.names);
    let targets = names.map(n => matchLTPXFile(fm.files, n)).filter(Boolean);
    if (targets.length === 0) {
        // 未指定目标：压缩当前目录全部非压缩文件（与手动批量压缩行为一致）
        targets = fm.files.filter(f => !f.isDir && !isZipFile(f.name)).slice(0, 50);
    }
    if (targets.length === 0) return { success: false, error: '当前目录没有可压缩的文件' };
    targets.forEach(f => {
        fm.selectedFiles.add(f.path);
        updateFileCardSelection(f, true);
    });
    fm.updateBatchActions();
    const zipName = String(args.zip_name || '').trim() || `压缩文件_${new Date().getTime()}.zip`;
    const resp = await fetch('/file/archive/create', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ paths: targets.map(f => f.path), zip_name: zipName, save_path: fm.currentPath || '' })
    });
    const data = await resp.json();
    if (!resp.ok || !data.success) return { success: false, error: (data && data.error) || '压缩失败' };
    await fm.loadFiles();
    fm.updateStats();
    return { success: true, text: `已压缩 ${targets.length} 个项目到 ${data.path}` };
}

/** 解压 */
async function execLTPXExtract(fm, args) {
    const file = matchLTPXFile(fm.files, args.name) || fm.files.find(f => isZipFile(f.name));
    if (!file) return { success: false, error: '未找到要解压的 ZIP 压缩包' };
    await showZipModal(file, fm);
    let targetDir = args.target_dir ? normalizeLTPXPath(args.target_dir) : '';
    if (!targetDir) {
        targetDir = (document.getElementById('zip-extract-path').value || '').trim();
    }
    const resp = await fetch('/file/archive/extract', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ path: file.path, target_dir: targetDir })
    });
    const data = await resp.json();
    if (!resp.ok || !data.success) return { success: false, error: (data && data.error) || '解压失败' };
    await fm.loadFiles();
    fm.updateStats();
    closeZipModal();
    return { success: true, text: `已解压 ${data.file_count} 个文件到 ${data.target_dir}` };
}

/** 新建文件夹 */
async function execLTPXCreateFolder(fm, args) {
    const name = String(args.name || '').trim();
    if (!name) return { success: false, error: '缺少文件夹名称' };
    await createDirectory(name, fm.currentPath);
    await fm.loadFiles();
    return { success: true, text: `已新建文件夹「${name}」` };
}

/** 智能整理（异步） */
async function execLTPXSmartOrganize(fm, args) {
    const nonDir = fm.files.filter(f => !f.isDir);
    if (nonDir.length === 0) return { success: false, error: '当前目录没有可整理的文件' };
    startSmartOrganize(fm);
    return { success: true, text: `已对当前目录（${fm.currentPath || '根目录'}）启动智能整理，可在文件管理器中查看进度` };
}

/** 哈希命名 */
async function execLTPXHashRename(fm, args) {
    const resp = await fetch('/file/hash-rename', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ path: fm.currentPath || '' })
    });
    const data = await resp.json();
    if (!resp.ok || !data.success) return { success: false, error: (data && data.error) || '哈希命名失败' };
    await fm.loadFiles();
    return { success: true, text: `哈希命名完成：${data.renamed || 0} 个文件已重命名` };
}

/** GGUF 元数据 */
async function execLTPXGGUF(fm, args) {
    const file = matchLTPXFile(fm.files, args.name) || fm.files.find(f => isGGUFFile(f.name));
    if (!file) return { success: false, error: '未找到 GGUF 模型文件' };
    await showGGUFModal(file);
    return { success: true, text: `已打开「${file.name}」的 GGUF 元数据预览` };
}

/** 选中文件 */
async function execLTPXSelect(fm, args) {
    const file = matchLTPXFile(fm.files, args.name);
    if (!file) return { success: false, error: `未找到要选中的文件「${args.name}」` };
    fm.selectedFiles.add(file.path);
    updateFileCardSelection(file, true);
    fm.updateBatchActions();
    return { success: true, text: `已选中「${file.name}」` };
}

/** 工具名 → 执行器映射 */
const ltpxAgentToolExecutors = {
    list_files: execLTPXListFiles,
    view_file: execLTPXViewFile,
    read_file_content: execLTPXReadFileContent,
    search_files: execLTPXSearch,
    rename_item: execLTPXRename,
    move_items: execLTPXMove,
    delete_item: execLTPXDelete,
    compress_items: execLTPXCompress,
    extract_zip: execLTPXExtract,
    create_folder: execLTPXCreateFolder,
    smart_organize: execLTPXSmartOrganize,
    hash_rename: execLTPXHashRename,
    gguf_metadata: execLTPXGGUF,
    select_item: execLTPXSelect
};

/**
 * 执行单个工具并返回结构化结果（供回填给模型）
 * @param {FileManager} fm - 文件管理器实例
 * @param {string} name - 工具名
 * @param {Object} args - 结构化参数
 * @returns {Promise<Object>} { success, text?, error? }
 */
async function executeLTPXTool(fm, name, args) {
    const executor = ltpxAgentToolExecutors[name];
    if (!executor) return { success: false, error: `未知工具: ${name}` };
    try {
        return await executor(fm, args || {});
    } catch (e) {
        return { success: false, error: (e && e.message) || String(e) };
    }
}

// ===== 主流程：多轮工具调用循环 =====

/**
 * 运行文件管理器 LLM 智能体，处理一条月华指令
 * 返回 { success, text, error }；模型/网络调用失败时抛出异常（由外层捕获并回传失败结果）
 * @param {FileManager} fm - 文件管理器实例
 * @param {string} instruction - 月华发来的自然语言指令
 * @returns {Promise<Object>}
 */
async function runLTPXAgent(fm, instruction) {
    const text = String(instruction || '').trim();
    if (!text) throw new Error('空指令');

    // 确保目录内容已加载（供构造上下文）
    if (!fm.files || fm.files.length === 0) {
        try { fm.files = await loadFiles(fm.currentPath); } catch (e) { /* 后续工具可自行刷新 */ }
    }

    // 消息骨架：系统提示 + 独立上下文历史（最近 40 轮）+ 本轮（附当前目录上下文）
    const messages = [{ role: 'system', content: buildLTPXSystemPrompt() }];
    for (const round of ltpxAgentHistory) {
        messages.push({ role: 'user', content: round.user });
        messages.push({ role: 'assistant', content: round.assistant });
    }
    messages.push({ role: 'user', content: buildLTPXUserMessage(fm, text) });

    let lastReply = '';
    for (let loop = 0; loop < LTPX_AGENT_MAX_TOOL_LOOPS; loop++) {
        const message = await callLTPXAgentModel(messages);
        const toolCalls = Array.isArray(message.tool_calls) ? message.tool_calls : [];

        // 无工具调用：智能体给出最终答复
        if (toolCalls.length === 0) {
            lastReply = String(message.content || '').trim() || '已完成';
            break;
        }

        // 记录助手工具调用（供工具结果回填时关联）
        messages.push({
            role: 'assistant',
            content: message.content || '',
            tool_calls: toolCalls.map(tc => ({
                id: tc.id,
                type: 'function',
                function: { name: tc.function.name, arguments: tc.function.arguments }
            }))
        });

        // 依次执行工具，将结果作为 tool 消息回填
        for (const tc of toolCalls) {
            const args = safeParseLTPXArgs(tc.function.arguments);
            const result = await executeLTPXTool(fm, tc.function.name, args);
            messages.push({
                role: 'tool',
                tool_call_id: tc.id,
                content: JSON.stringify(result)
            });
        }
    }

    // 工具循环耗尽仍未给出最终答复（模型持续调用工具）
    if (!lastReply) {
        lastReply = '已完成相关文件操作';
    }

    // 记录本轮对话并裁剪历史（保留最近 40 轮，超出丢弃最早的）
    ltpxAgentHistory.push({ user: text, assistant: lastReply });
    if (ltpxAgentHistory.length > LTPX_AGENT_MAX_ROUNDS) {
        ltpxAgentHistory.splice(0, ltpxAgentHistory.length - LTPX_AGENT_MAX_ROUNDS);
    }

    return { success: true, text: lastReply, error: '' };
}
