import type { CustomCharacterProfile } from '../../capabilities/character';
import { saveCustomCharacter } from '../../capabilities/character';
import { sanitizeImageDesc, resolveImageRef } from '../../capabilities/image-tag';
import { listResources } from '../../capabilities/resource';
import type { CreateCharacterParams, ImageTagParams, SchemeSwitchParams } from '../../config/image';
import { LIST_LIMIT_MAX } from './frame';
import type { PainterHost } from './host';
import { pushAssistantImage } from '../../io/conversation';

/** 创建或更新自定义角色（写入 SQLite 角色库，供立绘与合照共用） */
export function handleCreateCharacter(host: PainterHost, args: CreateCharacterParams): string {
	const name = String(args.name || '').trim();
	const appearance = String(args.appearance || '').trim();
	if (!name) return '创建角色失败：角色名不能为空';
	if (!appearance) return '创建角色失败：appearance 不能为空（写清年龄感、发型发色、瞳色、面部特征、配饰、体型）';
	if (name === '月华' || name === '琉璃' || name === '通用-女' || name === '通用-男') {
		return `创建角色失败：「${name}」是内置角色名，请换一个名字`;
	}
	const existed = host.customCharacters.has(name);
	const profile: CustomCharacterProfile = {
		name,
		gender: args.gender === '男' ? '男' : '女',
		appearance,
		outfitFull: String(args.outfit_full || '').trim() || undefined,
		outfitChibi: String(args.outfit_chibi || '').trim() || undefined,
	};
	if (!saveCustomCharacter(profile)) return '创建角色失败：角色库写入失败，请稍后重试';
	host.customCharacters.set(name, profile);
	console.log(`[绘制者] 自定义角色${existed ? '更新' : '创建'}成功: ${name}`);
	return `${existed ? '已更新' : '已创建'}自定义角色「${name}」（${profile.gender}）。之后绘制立绘或合照时可直接用该名字引用。`;
}

/** 切换绘图模型方案（状态由 Go 层记录，与图像工坊前端共享） */
export function handleSwitchDiffusionScheme(args: SchemeSwitchParams): string {
	const name = String(args.scheme || '').trim();
	if (!name) return '切换模型方案失败：请指定 turbo 或 quality';
	const [current, error] = setDiffusionScheme(name);
	if (error) return `切换模型方案失败：${error.message ?? String(error)}`;
	return `已切换到 ${current}（${schemeLabel(current)}）模型方案，后续绘图将使用该方案。`;
}

/** 方案键名 → 人话 */
function schemeLabel(name: string): string {
	switch (name) {
		case 'turbo': return '速度优先';
		case 'quality': return '质量优先';
		default: return name || '默认';
	}
}

/**
 * 处理图片句柄清单查询（了解历史图片、确认句柄或为超分定位目标）
 *
 * 清单来源＝**资源目录数据库**（唯一权威）。需要图生图基座时由对话者经
 * dispatch_painter 的 reference_tag 指定，任务消息会直接附上图片本体。
 */
export function handleListImages(host: PainterHost): string {
	const entries = listResources('I', LIST_LIMIT_MAX);
	if (entries.length === 0) return '资源目录中还没有任何图片：当前没有可引用的图片句柄';
	const lines = entries.map(entry => {
		const desc = entry.summary ? sanitizeImageDesc(entry.summary, 40) : '（暂无描述）';
		const lost = entry.state === 'active' ? '' : ' [本体已回收]';
		return `${entry.handle} ${desc}${lost}`;
	});
	return `资源目录中最近的 ${entries.length} 张图片（句柄越新越靠前）：\n${lines.join('\n')}\n可调用 upscale_image 对指定图片超分；图生图基座由对话者经 reference_tag 指定。${host.referenceStatus()}。`;
}

/** 处理独立超分：对最近生成作品或按句柄取图片执行 RealESRGAN 重建 */
export function handleUpscaleImage(host: PainterHost, args: ImageTagParams): string {
	try {
		const resolved = args.image_tag ? resolveImageRef(String(args.image_tag)) : null;
		let source = '';
		let handleLabel = '';
		if (args.image_tag) {
			if (!resolved) return '超分失败：句柄不存在或已失效，请先调用 list_images 或 list_resources 查看可用图片';
			if (!resolved.source) return `超分失败：${resolved.handle} 的本体已从内容仓库回收，无法超分`;
			source = resolved.source;
			handleLabel = resolved.handle;
		}
		else {
			if (!host.lastResultPath) return '超分失败：还没有生成过作品，请通过 image_tag 指定图片句柄，或先完成一次图像生成';
			source = host.lastResultPath;
			handleLabel = '最近作品';
		}
		console.log(`[绘制者] 超分重建 - 来源: ${handleLabel} ${source.substring(0, 60)}...`);
		const [result, error] = upscaleImage(source);
		if (error) {
			console.error('[绘制者] 超分失败:', error);
			return `超分失败: ${error}`;
		}
		if (!result || !result.base64) return '超分失败：引擎返回空结果';
		if (!pushAssistantImage([result.base64], false, [result.path])) console.warn('[绘制者] 推送超分结果到前端失败');
		return `超分成功。图片尺寸: ${result.width}x${result.height}，已推送给用户`;
	}
	catch (error) {
		console.error('[绘制者] 超分处理异常:', error);
		return `超分处理异常: ${error}`;
	}
}
