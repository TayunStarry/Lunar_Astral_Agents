/**
 * 撤销 / 重做：整体快照式历史。
 *
 * 节点图规模有限（数百节点），快照比增量补丁更不容易出错：
 * 每次可撤销操作提交一份「操作后的完整状态」，撤销即回放上一份状态。
 *
 * 通过注入 snapshot/apply 两个回调，本类保持纯逻辑、可在 Node 中测试。
 */

export class History {
	/**
	 * @param {{snapshot:()=>any, apply:(state:any)=>void, limit?:number}} options
	 */
	constructor(options) {
		this.snapshot = options.snapshot;
		this.apply = options.apply;
		this.limit = Math.max(2, options.limit || 200);
		/** @type {Array<{label:string, state:any}>} */
		this.entries = [];
		this.index = -1;
	}

	/** 以当前状态作为历史起点（载入文档/新建文档后调用） */
	reset() {
		this.entries = [{ label: "初始状态", state: this.snapshot() }];
		this.index = 0;
		return this;
	}

	/** 提交一次操作后的状态；与当前状态相同则忽略（避免拖拽抖动污染历史） */
	commit(label = "编辑") {
		if (this.index < 0) return this.reset();
		const state = this.snapshot();
		if (sameState(this.entries[this.index].state, state)) return this;
		this.entries = this.entries.slice(0, this.index + 1);
		this.entries.push({ label, state });
		if (this.entries.length > this.limit) {
			this.entries.splice(0, this.entries.length - this.limit);
		}
		this.index = this.entries.length - 1;
		return this;
	}

	undo() {
		if (!this.canUndo()) return false;
		this.index -= 1;
		this.apply(this.entries[this.index].state);
		return true;
	}

	redo() {
		if (!this.canRedo()) return false;
		this.index += 1;
		this.apply(this.entries[this.index].state);
		return true;
	}

	canUndo() {
		return this.index > 0;
	}

	canRedo() {
		return this.index >= 0 && this.index < this.entries.length - 1;
	}

	/** 将被撤销的操作名（用于按钮提示） */
	undoLabel() {
		return this.canUndo() ? this.entries[this.index].label : "";
	}

	/** 将被重做的操作名 */
	redoLabel() {
		return this.canRedo() ? this.entries[this.index + 1].label : "";
	}

	get length() {
		return this.entries.length;
	}
}

function sameState(a, b) {
	if (a === b) return true;
	try {
		return JSON.stringify(a) === JSON.stringify(b);
	} catch (err) {
		return false;
	}
}

/** 深拷贝快照：JSON 往返，保证历史状态绝不与活动对象共享引用 */
export function cloneState(value) {
	return JSON.parse(JSON.stringify(value));
}
