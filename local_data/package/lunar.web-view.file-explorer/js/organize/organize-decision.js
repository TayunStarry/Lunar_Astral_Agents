/**
 * 智能整理：单文件 AI 决策模块
 * 每个文件独立询问 AI（携带当前已知文件夹列表，含之前文件拟新增的文件夹），
 * 并解析决策、生成 move / rename 操作
 */

/**
 * 询问 AI 对单个文件的处理决定
 * @param {Object} file - 预处理后的文件描述
 * @param {Array<string>} knownFolders - 当前已知文件夹列表（含拟新增的）
 * @returns {Promise<{rename_to: string|null, target_folder: string|null}>}
 */
async function askFileDecision(file, knownFolders) {
    const messages = buildFileDecisionMessages(file, knownFolders);
    const response = await callOrganizeAI(messages);
    return parseOrganizeDecision(response);
}

/**
 * 组装单文件决策的 AI 消息（多模态内容数组，图片以 image_url 提交）
 * @param {Object} file - 预处理后的文件描述
 * @param {Array<string>} knownFolders - 当前已知文件夹列表
 * @returns {Array<Object>} 消息数组
 */
function buildFileDecisionMessages(file, knownFolders) {
    const system = [
        '你是文件整理助手。当前正在进行逐个文件的智能整理，你会依次看到每个待整理的文件。',
        '对当前这个文件，你需要决定：',
        '1. rename_to：是否重命名（保持扩展名不变；无需重命名时为 null）；',
        '2. target_folder：放到哪个文件夹（从「当前已知文件夹」中选择，或新建一个语义清晰的新文件夹名；留在当前目录时为 null）。',
        '规则：',
        '- 优先复用「当前已知文件夹」，语义匹配时不要重复新建同义文件夹；',
        '- 新建的文件夹名会加入「当前已知文件夹」，供后续文件复用；',
        '- 只返回 JSON 对象，不要输出其他内容，格式：{"rename_to":"新文件名或null","target_folder":"文件夹名或null"}'
    ].join('\n');

    const userParts = [
        { type: 'text', text: `当前已知文件夹列表：${knownFolders.length ? knownFolders.join('、') : '（无，可新建）'}\n请决定下列文件的整理方式：` }
    ];
    if (file.type === 'image' && file.base64) {
        userParts.push({ type: 'text', text: `文件：${file.name}（${file.size}，${file.ext}）` });
        userParts.push({ type: 'image_url', image_url: { url: `data:image/jpeg;base64,${file.base64}` } });
    } else if (file.type === 'text' && file.content) {
        userParts.push({ type: 'text', text: `文件：${file.name}（${file.size}，${file.ext}）\n内容样本：\n${file.content}` });
    } else {
        const note = file.note ? ` — ${file.note}` : '';
        userParts.push({ type: 'text', text: `文件：${file.name}（${file.size}，${file.ext}）${note}` });
    }

    return [
        { role: 'system', content: system },
        { role: 'user', content: userParts }
    ];
}

/**
 * 解析 AI 返回的单文件决策 JSON（兼容 ```json 代码块包裹）
 * @param {string} response - AI 响应文本
 * @returns {{rename_to: string|null, target_folder: string|null}}
 */
function parseOrganizeDecision(response) {
    const cleaned = response.replace(/```json\s*|```\s*/g, '').trim();
    const match = cleaned.match(/\{[\s\S]*\}/);
    const obj = match ? JSON.parse(match[0]) : JSON.parse(cleaned);
    return {
        rename_to: obj.rename_to != null ? String(obj.rename_to).trim() : null,
        target_folder: obj.target_folder != null ? String(obj.target_folder).trim() : null
    };
}

/**
 * 根据 AI 决策生成整理操作（move / rename），并将拟新增文件夹加入 knownFolders
 * @param {string} fileName - 原文件名
 * @param {{rename_to: string|null, target_folder: string|null}} decision - AI 决策
 * @param {Array<string>} knownFolders - 当前已知文件夹列表（会被追加新增文件夹）
 * @returns {Object|null} 操作对象或 null（保持不动）
 */
function buildFileOperation(fileName, decision, knownFolders) {
    const dotIdx = fileName.lastIndexOf('.');
    const ext = dotIdx >= 0 ? fileName.slice(dotIdx) : '';

    // 重命名（保持扩展名不变）
    let finalName = fileName;
    let renamed = false;
    if (decision.rename_to && decision.rename_to !== fileName) {
        let newName = decision.rename_to;
        if (ext && !newName.toLowerCase().endsWith(ext.toLowerCase())) {
            newName += ext;
        }
        if (newName && newName !== fileName) {
            finalName = newName;
            renamed = true;
        }
    }

    // 目标文件夹（去除首尾斜杠）
    const folder = decision.target_folder
        ? decision.target_folder.replace(/^\/+|\/+$/g, '').replace(/\\/g, '/')
        : '';

    if (folder && folder !== '.') {
        // 拟新增的文件夹加入已知集合，供后续文件复用
        if (!knownFolders.includes(folder)) {
            knownFolders.push(folder);
        }
        const target = `${folder}/${finalName}`;
        if (target !== fileName) {
            return { type: 'move', source: fileName, target: target };
        }
        return null;
    }

    if (renamed) {
        return { type: 'rename', source: fileName, target: finalName };
    }
    return null;
}
