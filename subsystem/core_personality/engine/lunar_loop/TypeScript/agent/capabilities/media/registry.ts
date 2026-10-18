import type { PerceiverRole } from '../../roles/perceiver';

/** 媒体理解所需的角色单例（由 roles.ts 在实例化后注入，解除 media → roles 的静态循环引用） */
export interface MediaRoles {
	/** 感知者（视频 / 动态图理解） */
	perceiverRole: PerceiverRole;
	/** 随机默认应答（理解结果为空时兜底） */
	randomDefaultMessage: () => string;
}

/** 角色单例注册表 */
let mediaRoles: MediaRoles | null = null;

/** 注入角色单例（roles.ts 模块初始化时调用一次） */
export function registerMediaRoles(roles: MediaRoles): void {
	mediaRoles = roles;
}

/** 取角色单例（未注入时抛错，由 roles.ts 保证注入顺序） */
export function mediaRoleContext(): MediaRoles {
	if (!mediaRoles) throw new Error('媒体角色尚未注入（roles.ts 未初始化）');
	return mediaRoles;
}
