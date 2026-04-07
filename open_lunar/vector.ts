/**
 * 数学工具类导入
 */
import { Clamp, RandomFloat } from "./math";

/**
 * 三维向量接口
 * 表示具有 x、y、z 三个分量的向量
 */
interface Vector3 {
	x: number;
	y: number;
	z: number;
}

/**
 * 二维向量接口（XZ 平面）
 * 表示具有 x、z 两个分量的向量
 */
interface Vector2XZ {
	x: number;
	z: number;
}

/**
 * 二维向量接口（YZ 平面）
 * 表示具有 y、z 两个分量的向量
 */
interface Vector2YZ {
	y: number;
	z: number;
}

/**
 * 二维向量接口（XY 平面）
 * 表示具有 x、y 两个分量的向量
 */
interface Vector2XY {
	x: number;
	y: number;
}

/**
 * 二维向量泛型类型
 * 用于表示不同平面的二维向量
 * @template T - 二维向量的平面类型，可选值为 'xy' | 'xz' | 'yz'
 */
type Vector2<T extends 'xy' | 'xz' | 'yz' = 'xy' | 'xz' | 'yz'> = T extends 'xy' ? Vector2XY : T extends 'xz' ? Vector2XZ : T extends 'yz' ? Vector2YZ : Vector2XY | Vector2XZ | Vector2YZ;

/**
 * 方向向量集合接口
 * 包含前、后、左、右、上、下六个方向的向量
 */
interface VectorDirections {
	right: VectorCase;
	back: VectorCase;
	left: VectorCase;
	front: VectorCase;
	above: VectorCase;
	down: VectorCase
};

/**
 * 相对偏移接口
 * 表示相对于前方、右方和上方的偏移量
 */
interface VectorRelativeOffset {
	front: number;
	right: number;
	above: number;
};

/**
 * 向量字符串化选项接口
 * 用于控制向量转换为字符串时的格式
 */
interface VectorStringOptions {
	decimals?: number; // 小数位数
	delimiter?: string; // 分隔符
};

/**
 * 向量限制接口
 * 用于限制向量各分量的取值范围
 */
interface VectorLimits {
	min?: Partial<Vector3>; // 最小值
	max?: Partial<Vector3>; // 最大值
}

/**
 * 向量基类
 * 实现了 Vector3 接口，提供向量的基本属性和常量
 */
class VectorBase implements Vector3 {
	/**
	 * 常量: 0.5 向量
	 * 返回一个所有分量都为 0.5 的向量
	 */
	static get CONSTANT_HALF(): VectorCase { return new VectorCase(0.5, 0.5, 0.5) };

	/**
	 * 常量: 0 向量
	 * 返回一个所有分量都为 0 的向量
	 */
	static get CONSTANT_ZERO(): VectorCase { return new VectorCase(0, 0, 0) };

	/**
	 * 常量: 向上单位向量
	 * 返回一个 y 分量为 1, 其余分量为 0 的向上单位向量
	 */
	static get CONSTANT_UP(): VectorCase { return new VectorCase(0, 1, 0) };

	/**
	 * 常量: 向下单位向量
	 * 返回一个 y 分量为 -1, 其余分量为 0 的向下单位向量
	 */
	static get CONSTANT_DOWN(): VectorCase { return new VectorCase(0, -1, 0) };

	/**
	 * 常量: 正一向量
	 * 返回一个所有分量都是 1 的向量
	 */
	static get CONSTANT_ONE(): VectorCase { return new VectorCase(1, 1, 1) };

	/**
	 * 常量: 负一向量
	 * 返回一个所有分量都是 -1 的向量
	 */
	static get CONSTANT_NEGATIVE_ONE(): VectorCase { return new VectorCase(-1, -1, -1) };

	/**
	 * 常量: 西方向单位向量
	 * 返回一个 x 分量为 -1, 其余分量为 0 的向西方向单位向量
	 */
	static get CONSTANT_WEST(): VectorCase { return new VectorCase(-1, 0, 0) };

	/**
	 * 常量: 东方向单位向量
	 * 返回一个 x 分量为 1, 其余分量为 0 的向东方向单位向量
	 */
	static get CONSTANT_EAST(): VectorCase { return new VectorCase(1, 0, 0) };

	/**
	 * 常量: 南方向单位向量
	 * 返回一个 z 分量为 1, 其余分量为 0 的向南方向单位向量
	 */
	static get CONSTANT_SOUTH(): VectorCase { return new VectorCase(0, 0, 1) };

	/**
	 * 常量: 北方向单位向量
	 * 返回一个 z 分量为 -1, 其余分量为 0 的向北方向单位向量
	 */
	static get CONSTANT_NORTH(): VectorCase { return new VectorCase(0, 0, -1) };

	/**
	 * 常量: 水平方向单位向量数组
	 * 包含四个水平方向（北、南、东、西）的单位向量
	 */
	static get CONSTANT_HORIZONTAL(): VectorCase[] {
		return [
			VectorBase.CONSTANT_NORTH,
			VectorBase.CONSTANT_SOUTH,
			VectorBase.CONSTANT_EAST,
			VectorBase.CONSTANT_WEST
		]
	};

	/**
	 * 常量: 垂直方向单位向量数组
	 * 包含两个垂直方向（上、下）的单位向量
	 */
	static get CONSTANT_VERTICAL(): VectorCase[] {
		return [
			VectorBase.CONSTANT_DOWN,
			VectorBase.CONSTANT_UP,
		]
	};

	/**
	 * 常量: 所有方向单位向量数组
	 * 包含所有方向（垂直和水平）的单位向量
	 */
	static get CONSTANT_ALL(): VectorCase[] {
		return [
			...VectorBase.CONSTANT_VERTICAL,
			...VectorBase.CONSTANT_HORIZONTAL,
		]
	};

	/**
	 * 常量: 向下及水平方向单位向量数组
	 * 包含向下方向（下）和所有水平方向（北、南、东、西）的单位向量
	 */
	static get CONSTANT_DOWN_HORIZONTAL(): VectorCase[] {
		return [
			VectorBase.CONSTANT_DOWN,
			...VectorBase.CONSTANT_HORIZONTAL,
		]
	};

	/**
	 * 构建一个三维向量实例
	 * @param x x 轴分量
	 * @param y y 轴分量
	 * @param z z 轴分量
	 */
	constructor(public x: number, public y: number, public z: number) { };
}

/**
 * 向量实现类
 * 继承自 VectorBase，提供向量的各种操作方法
 */
class VectorCase extends VectorBase implements Vector3 {
	/**
	 * * 生成立方体阵列向量数组
	 *
	 * @param scope 向量坐标的范围大小（非负整数）
	 *
	 * @returns {Vector[]} 立方体阵列向量数组, 包含所有可能的 (x, y, z) 组合, 其中每个坐标值都在 [-scope, +scope] 范围内
	 */
	public createCubeLattice(scope: number): VectorCase[] {
		return VectorTool.createCubeLattice(scope, this);
	}
	/**
	 * * 比较当前 Vector3 对象与另一个 Vector3 对象是否相等
	 *
	 * @param {Vector3} vector - 要比较的 Vector3 对象
	 *
	 * @returns {boolean} 如果两个 Vector3 对象的 x、y 和 z 属性都相等, 则返回 true；否则返回 false
	 */
	public equals(vector: Vector3): boolean {
		return VectorTool.equals(this, vector);
	};
	/**
	 * * 将当前 Vector3 对象与另一个 Vector3 对象相加
	 *
	 * @param {Vector3} vector - 要相加的 Vector3 对象
	 *
	 * @returns {Vector} - 相加结果的新 Vector3 对象
	 */
	public add(vector: Vector3): VectorCase {
		return VectorTool.add(this, vector);
	};
	/**
	 * * 从当前 Vector3 对象中减去另一个 Vector3 对象
	 *
	 * @param {Vector3} vector - 要减去的 Vector3 对象
	 *
	 * @returns {Vector} - 减法结果的新 Vector 对象
	 */
	public subtract(vector: Vector3): VectorCase {
		return VectorTool.subtract(this, vector);
	};
	/**
	 * * 将当前 Vector 对象的每个分量乘以一个标量
	 *
	 * @param {number} scale - 用于缩放的标量值
	 *
	 * @returns {Vector} - 缩放后的新 Vector 对象
	 */
	public multiply(scale: number): VectorCase {
		return VectorTool.multiply(this, scale);
	};
	/**
	 * 计算当前 Vector3 对象与另一个 Vector3 对象的点积
	 *
	 * @param {Vector3} vector - 要计算点积的 Vector3 对象
	 *
	 * @returns {number} - 两个向量的点积结果
	 */
	public dot(vector: Vector3): number {
		return VectorTool.dot(this, vector);
	};
	/**
	 * 将当前 Vector3 对象的每个分量除以一个标量
	 *
	 * @param {number} divisor - 用于除法的标量值
	 *
	 * @returns {Vector} - 除法结果的新 Vector3 对象
	 */
	public division(divisor: number): VectorCase {
		return VectorTool.division(this, divisor);
	};
	/**
	 * * 计算当前 Vector3 对象与另一个 Vector3 对象的叉积
	 *
	 * @param {Vector3} vector - 要计算叉积的 Vector3 对象
	 *
	 * @returns {Vector} - 两个向量的叉积结果
	 */
	public cross(vector: Vector3): VectorCase {
		return VectorTool.cross(this, vector);
	};
	/**
	 * * 返回 Vector3 对象的一个副本	
	 *
	 * @returns {Vector} 当前 Vector 对象的副本
	 */
	public copy(): VectorCase {
		return VectorTool.copy(this);
	};
	/**
	 * * 根据指定步数返回当前向量在y轴方向上的偏移结果
	 *
	 * @param {number} [steps = 1] - 垂直方向偏移量（可选, 默认为1）
	 *
	 * @returns {Vector} - 偏移后的新的 Vector 对象
	 */
	public above(steps?: number): VectorCase {
		return VectorTool.above(this, steps);
	};
	/**
	 * * 根据指定步数返回当前向量在x轴正方向的偏移结果
	 *
	 * @param {number} [steps=1] - 水平方向偏移量（可选, 默认为1）
	 *
	 * @returns {Vector} - 偏移后的新的 Vector 对象
	 */
	public east(steps?: number): VectorCase {
		return VectorTool.east(this, steps);
	}
	/**
	 * * 根据指定步数返回当前向量在z轴正方向的偏移结果
	 *
	 * @param {number} [steps=1] - 水平方向偏移量（可选, 默认为1）
	 *
	 * @returns {Vector} - 偏移后的新的 Vector 对象
	 */
	public north(steps?: number): VectorCase {
		return VectorTool.north(this, steps);
	}
	/**
	 * * 获取当前 Vector3 对象的模（长度）
	 *
	 * @returns {number} 向量的模
	 */
	public magnitude(): number {
		return VectorTool.magnitude(this);
	}
	/**
	 * * 计算当前 Vector3 对象与另一个 Vector3 对象之间的距离
	 *
	 * @param {Vector3} vector - 要计算距离的 Vector3 对象
	 *
	 * @returns {number} - 两个向量之间的距离
	 */
	public distance(vector: Vector3): number {
		return VectorTool.distance(this, vector);
	}
	/**
	 * * 获取当前 Vector3 对象的归一化向量
	 *
	 * @returns {Vector} 归一化的单位向量
	 */
	public normalize(): VectorCase {
		return VectorTool.normalize(this);
	}
	/**
	 * * 将当前 Vector3 对象的每个分量向下取整到指定的小数位数
	 *
	 * @param {number} decimals - 小数位数, 默认为 2
	 *
	 * @returns {Vector} - 取整后的新 Vector 对象
	 */
	public floor(decimals: number = 2): VectorCase {
		return VectorTool.floor(this, decimals);
	}
	/**
	 * 将当前 Vector3 对象转换为字符串
	 *
	 * @param {VectorStringOptions} [options] - 字符串化选项
	 *
	 * @returns {string} - 向量的字符串表示
	 */
	public toString(options?: VectorStringOptions): string {
		return VectorTool.toString(this, options);
	}
	/**
	 * 将当前 Vector3 对象的每个分量限制在指定的范围内
	 *
	 * @param {VectorLimits} [limits] - 包含最小值和最大值的对象
	 *
	 * @returns {VectorCase} - 计算后的新 Vector 对象
	 */
	public clamp(limits?: VectorLimits): VectorCase {
		return VectorTool.clamp(this, limits);
	}
	/**
	 * * 向量 线性插值
	 *
	 * @param {Vector3} vector - 向量对象
	 *
	 * @param {number} time - 时间系数
	 *
	 * @returns {Vector3} - 计算后的向量对象
	 */
	public lerp(vector: Vector3, time: number): VectorCase {
		return VectorTool.lerp(this, vector, time);
	}
	/**
	 * * 向量 球面线性插值
	 *
	 * @param {Vector3} vector - 向量对象
	 *
	 * @param {number} time - 时间系数
	 *
	 * @returns {Vector3} - 计算后的向量对象
	 */
	public slerp(vector: Vector3, time: number): VectorCase {
		return VectorTool.slerp(this, vector, time);
	}
	/**
	 * * 计算两个向量的最大值
	 *
	 * @param {Vector3} vector - 向量对象
	 *
	 * @returns {Vector3} - 计算后的向量对象
	 */
	public max(vector: Vector3): VectorCase {
		return VectorTool.max(this, vector);
	}
	/**
	 * * 计算两个向量的最小值
	 *
	 * @param {Vector3} vector - 向量对象
	 *
	 * @returns {Vector3} - 计算后的向量对象
	 */
	public min(vector: Vector3): VectorCase {
		return VectorTool.min(this, vector);
	}
	/**
	 * 在当前 Vector3 对象和另一个 Vector3 对象之间随机生成一个向量
	 *
	 * @param {Vector3} vector - 另一个 Vector3 对象
	 *
	 * @returns {Vector} - 计算后的 Vector 对象
	 */
	public rangeRandom(vector: Vector3): VectorCase {
		return VectorTool.rangeRandom(this, vector);
	};
	/**
	 * * 在当前位置周围生成一个随机向量
	 *
	 * @param {number} range - 随机偏移范围（每个轴的偏移值在 [-range, +range] 之间）
	 *
	 * @param {Vector3} [offset] - 偏移量（默认为 { x: 0, y: 0, z: 0 }）
	 *
	 * @returns {Vector} - 计算后的 Vector 对象
	 */
	public random(range: number, offset: Vector3 = VectorCase.CONSTANT_ZERO): VectorCase {
		return VectorTool.random(this, range, offset);
	}
	/**
	 * 计算当前 Vector3 对象与另一个 Vector3 对象之间的归一化差向量
	 *
	 * @param {Vector3} target - 目标 Vector3 对象
	 *
	 * @returns {VectorCase} - 归一化后的差向量（从当前向量指向目标向量）
	 */
	public difference(target: Vector3): VectorCase {
		return VectorTool.difference(this, target);
	}
	/**
	 * 基于实体旋转角度获取指向方向的三维向量
	 *
	 * @param {Vector2XY} rotation - 实体旋转角度（x 为偏航角，y 为俯仰角）
	 *
	 * @returns {VectorCase} - 指向方向的归一化三维向量
	 */
	public AngleToPlace(rotation: Vector2XY): VectorCase {
		return VectorTool.AngleToPlace(rotation);
	}
	/**
	 * 将三维方向向量转换为实体旋转角度
	 *
	 * @param {Vector3} direction - 标准化后的三维方向向量
	 *
	 * @returns {VectorCase} - 实体旋转角度（x 为俯仰角，y 为偏航角，z 为 0）
	 */
	public Vector3ToAngle(direction: Vector3): VectorCase {
		return VectorTool.Vector3ToAngle(direction);
	}
	/**
	 * 计算目标方向相关的方向向量集
	 *
	 * @param {Vector3} front - 前方方向向量（归一化）
	 *
	 * @returns {VectorDirections} - 包含前、后、左、右、上、下六个方向的向量对象
	 */
	public directions(front: Vector3): VectorDirections {
		return VectorTool.directions(front);
	}
	/**
	 * 计算目标方向相关的坐标偏移
	 *
	 * @param {Vector3} front - 前方方向向量（归一化）
	 *
	 * @param {VectorRelativeOffset} offset - 相对于前方、右方和上方的偏移量
	 *
	 * @returns {VectorCase} - 计算后的偏移坐标
	 */
	public relativeOffset(front: Vector3, offset: VectorRelativeOffset): VectorCase {
		return VectorTool.relativeOffset(this, front, offset);
	}
	/**
	 * 计算提前量，用于预测移动目标的位置
	 *
	 * @param {Vector3} targetPosition - 目标 B 的位置
	 *
	 * @param {number} projectileSpeed - 发射物的速度
	 *
	 * @param {Vector3} targetVelocity - 目标 B 的速度
	 *
	 * @returns {VectorCase} - 发射物的最佳速度向量
	 */
	public calculateLeadVelocity(targetPosition: Vector3, projectileSpeed: number, targetVelocity: Vector3): VectorCase {
		return VectorTool.calculateLeadVelocity(this, targetPosition, projectileSpeed, targetVelocity);
	}
	/**
	 * 获取区块坐标
	 *
	 * @param {boolean} Yzero - 是否将 Y 轴的值设置为 0
	 *
	 * @param {number} size - 区块大小（默认为 16）
	 *
	 * @returns {VectorCase} - 计算后的区块坐标
	 */
	public chunkLocation(Yzero: boolean = true, size: number = 16): VectorCase {
		return VectorTool.chunkLocation(this, Yzero, size);
	}
}

/**
 * 向量工具类
 * 继承自 VectorCase，提供静态向量操作方法
 */
class VectorTool extends VectorCase implements Vector3 {
	/**
	 * * 生成立方体阵列向量数组
	 *
	 * @param scope 向量坐标的范围大小（非负整数）
	 *
	 * @returns {Vector[]} 立方体阵列向量数组, 包含所有可能的 (x, y, z) 组合, 其中每个坐标值都在 [-scope, +scope] 范围内
	 */
	public static createCubeLattice(scope: number, vector: Vector3 = VectorCase.CONSTANT_ZERO): VectorCase[] {
		/** 计算立方体的边长, 范围为 [-scope, +scope], 因此边长为 2 * scope + 1 */
		const size: number = 2 * scope + 1;
		/** 存储生成的向量数组 */
		const vectors: VectorCase[] = [];
		// 遍历所有可能的坐标组合
		for (let i: number = 0; i < size ** 3; i++) {
			/** 计算 x 坐标：通过整数除法和取模运算确定 x 的值 */
			const x: number = -scope + Math.floor(i / (size ** 2)) % size;
			/** 计算 y 坐标：通过整数除法和取模运算确定 y 的值 */
			const y: number = -scope + Math.floor((i / size) % size);
			/** 计算 z 坐标：通过取模运算确定 z 的值 */
			const z: number = -scope + i % size;
			// 将计算出的坐标封装为 Vector 对象并添加到数组中
			vectors.push(new VectorCase(x, y, z));
		}
		// 返回生成的向量数组
		return vectors.map(anchor => anchor.add(vector));
	};
	/**
	 * * 将当前 Vector3 对象与另一个 Vector3 对象相加
	 *
	 * @param {Vector3} vector1 - 要相加的第一个 Vector3 对象
	 *
	 * @param {Vector3} vector2 - 要相加的第二个 Vector3 对象
	 *
	 * @returns {Vector} - 相加结果的新 Vector3 对象
	 */
	public static add(vector1: Vector3, vector2: Vector3): VectorCase {
		return new VectorCase(vector1.x + vector2.x, vector1.y + vector2.y, vector1.z + vector2.z);
	};
	/**
	 * * 从 Vector3 对象中减去另一个 Vector3 对象
	 *
	 * @param {Vector3} vector1 - 被减去的 Vector3 对象
	 *
	 * @param {Vector3} vector2 - 要减去的 Vector3 对象
	 *
	 * @returns {Vector} - 减法结果的新 Vector 对象
	 */
	public static subtract(vector1: Vector3, vector2: Vector3): VectorCase {
		return new VectorCase(vector1.x - vector2.x, vector1.y - vector2.y, vector1.z - vector2.z);
	};
	/**
	 * * 将 Vector3 对象的每个分量乘以一个标量
	 *
	 * @param {Vector3} vector - 向量对象
	 *
	 * @param {number} scale - 用于缩放的标量值
	 *
	 * @returns {Vector} - 缩放后的新 Vector 对象
	 */
	public static multiply(vector: Vector3, scale: number): VectorCase {
		return new VectorCase(vector.x * scale, vector.y * scale, vector.z * scale);
	};
	/**
	 * * 比较当前 Vector3 对象与另一个 Vector3 对象是否相等
	 *
	 * @param {Vector3} vector1 - 要比较的 Vector3 对象
	 *
	 * @param {Vector3} vector2 - 要比较的 Vector3 对象
	 *
	 * @returns {boolean} - 如果两个 Vector3 对象的 x、y 和 z 属性都相等, 则返回 true；否则返回 false
	 */
	public static equals(vector1: Vector3, vector2: Vector3): boolean {
		return vector1.x === vector2.x && vector1.y === vector2.y && vector1.z === vector2.z;
	};
	/**
	 * * 计算 Vector3 对象与另一个 Vector3 对象的点积
	 *
	 * @param {Vector3} vector1 - 要计算点积的 Vector3 对象
	 *
	 * @param {Vector3} vector2 - 要计算点积的 Vector3 对象
	 *
	 * @returns {number} - 两个向量的点积结果
	 */
	public static dot(vector1: Vector3, vector2: Vector3): number {
		return vector1.x * vector2.x + vector1.y * vector2.y + vector1.z * vector2.z;
	};
	/**
	 * * 计算 Vector3 对象与另一个 Vector3 对象的叉积
	 *
	 * @param {Vector3} vector1 - 要计算叉积的 Vector3 对象
	 *
	 * @param {Vector3} vector2 - 要计算叉积的 Vector3 对象
	 *
	 * @returns {Vector3} - 两个向量的叉积结果
	 */
	public static cross(vector1: Vector3, vector2: Vector3): VectorCase {
		return new VectorCase(
			vector1.y * vector2.z - vector1.z * vector2.y,
			vector1.z * vector2.x - vector1.x * vector2.z,
			vector1.x * vector2.y - vector1.y * vector2.x,
		)
	};
	/**
	 * * 将 Vector3 对象的每个分量除以一个标量
	 *
	 * @param {Vector3} vector - 进行除法计算的 Vector3 对象
	 *
	 * @param {number} divisor - 用于除法的标量值
	 *
	 * @returns {VectorBase} - 除法结果的新 Vector3 对象
	 */
	public static division(vector: Vector3, divisor: number): VectorCase {
		if (divisor === 0) return new VectorCase(vector.x, vector.y, vector.z);
		return new VectorCase(vector.x / divisor, vector.y / divisor, vector.z / divisor);
	};
	/**
	 * * 返回 Vector3 对象的一个副本
	 *
	 * @param {Vector3} vector - 需要拷贝的 Vector3 对象
	 *
	 * @returns {Vector} - 拷贝后的新 Vector 对象
	 */
	public static copy(vector: Vector3): VectorCase {
		return new VectorCase(vector.x, vector.y, vector.z);
	};
	/**
	 * * 根据指定步数返回当前向量在y轴方向上的偏移结果
	 * 
	 * @param {Vector3} vector - 要偏移的 Vector3 对象
	 *
	 * @param {number} [steps = 1] - 垂直方向偏移量（可选, 默认为1）
	 *
	 * @returns {Vector} - 偏移后的新的 Vector 对象
	 */
	public static above(vector: Vector3, steps?: number): VectorCase {
		/**
		 * 获取偏移量
		 */
		const offset = steps ?? 1;
		// 返回当前 Vector3 对象与偏移量相加后的新 Vector3 对象
		return this.add(vector, { x: 0, y: offset, z: 0 });
	};
	/**
	 * * 根据指定步数返回当前向量在x轴正方向的偏移结果
	 *
	 * @param {number} [steps=1] - 水平方向偏移量（可选, 默认为1）
	 *
	 * @returns {Vector} - 偏移后的新的 Vector 对象
	 */
	public static east(vector: Vector3, steps?: number): VectorCase {
		/**
		 * 获取偏移量
		 */
		const offset = steps ?? 1;
		// 返回当前 Vector3 对象与偏移量相加后的新 Vector3 对象
		return this.add(vector, { x: offset, y: 0, z: 0 });
	};
	/**
	 * * 根据指定步数返回当前向量在z轴正方向的偏移结果
	 *
	 * @param {number} [steps=1] - 水平方向偏移量（可选, 默认为1）
	 *
	 * @returns {Vector} - 偏移后的新的 Vector 对象
	 */
	public static north(vector: Vector3, steps?: number): VectorCase {
		/** 获取偏移量 */
		const offset = steps ?? 1;
		// 返回当前 Vector3 对象与偏移量相加后的新 Vector3 对象
		return this.add(vector, { x: 0, y: 0, z: offset });
	};
	/**
	 * * 获取 Vector3 对象的模（长度）
	 *
	 * @param {Vector3} vector - 进行计算的 Vector3 对象
	 *
	 * @returns {number} - 向量的模
	 */
	public static magnitude(vector: Vector3): number {
		return Math.sqrt(vector.x ** 2 + vector.y ** 2 + vector.z ** 2);
	};
	/**
	 * * 计算 Vector3 对象与另一个 Vector3 对象之间的距离
	 *
	 * @param {Vector3} start - 要计算距离的 Vector3 对象
	 *
	 * @param {Vector3} done - 要计算距离的 Vector3 对象
	 *
	 * @returns {number} - 两个向量之间的距离
	 */
	public static distance(start: Vector3, done: Vector3): number {
		return this.magnitude(this.subtract(start, done));
	};
	/**
	 * * 获取 Vector3 对象的归一化向量
	 *
	 * @param {Vector3} vector - 进行计算的 Vector3 对象
	 *
	 * @returns {Vector3} - 计算后的向量对象
	 */
	public static normalize(vector: Vector3): VectorCase {
		/**
		 * * 计算向量模长
		 */
		const magnitude = this.magnitude(vector);
		return new VectorCase(vector.x / magnitude, vector.y / magnitude, vector.z / magnitude);
	};
	/**
	 * * 将 Vector3 对象的每个分量向下取整到指定的小数位数
	 *
	 * @param {Vector3} vector - 进行计算的 Vector3 对象
	 *
	 * @param {number} decimals - 小数位数, 默认为 2
	 *
	 * @returns {Vector} - 取整后的新 Vector 对象
	 */
	public static floor(vector: Vector3, decimals: number = 2): VectorCase {
		/**
		 * * 获取乘数
		 */
		const multiplier = Math.pow(10, decimals);
		return new VectorCase(
			Math.floor(vector.x * multiplier) / multiplier,
			Math.floor(vector.y * multiplier) / multiplier,
			Math.floor(vector.z * multiplier) / multiplier
		);
	};
	/**
	 * 将 Vector 对象转换为字符串
	 *
	 * @param {Vector3 | Vector2} vector - 向量对象
	 *
	 * @param {VectorStringOptions} [options] - 字符串化选项
	 *
	 * @returns {string} - 向量的字符串表示
	 */
	public static toString(vector: Vector3 | Vector2, options?: VectorStringOptions): string {
		// 默认小数位数
		const decimals = options?.decimals ?? 2;
		// 向量分隔字符串
		const delimiter = options?.delimiter ?? ', ';
		// 根据向量的类型, 获取不同的属性
		let components: string[];
		// 3D向量
		if ('x' in vector && 'y' in vector && 'z' in vector) components = [vector.x.toFixed(decimals), vector.y.toFixed(decimals), vector.z.toFixed(decimals)];
		// Vector2XY
		else if ('x' in vector && 'y' in vector) components = [vector.x.toFixed(decimals), vector.y.toFixed(decimals)];
		// Vector2XZ
		else if ('x' in vector && 'z' in vector) components = [vector.x.toFixed(decimals), vector.z.toFixed(decimals)];
		// Vector2YZ
		else if ('y' in vector && 'z' in vector) components = [vector.y.toFixed(decimals), vector.z.toFixed(decimals)];
		// 默认为空数组
		else components = [];
		// 将向量组件连接成字符串
		return components.join(delimiter);
	};
	/**
	 * 将 Vector3 对象的每个分量限制在指定的范围内
	 *
	 * @param {Vector3} vector - 进行计算的 Vector3 对象
	 *
	 * @param {VectorLimits} [limits] - 包含最小值和最大值的对象
	 *
	 * @returns {VectorCase} - 计算后的新 Vector 对象
	 */
	public static clamp(vector: Vector3, limits?: VectorLimits): VectorCase {
		return new VectorCase(
			Clamp({ min: limits?.min?.x ?? Number.MIN_SAFE_INTEGER, max: limits?.max?.x ?? Number.MAX_SAFE_INTEGER }, vector.x),
			Clamp({ min: limits?.min?.y ?? Number.MIN_SAFE_INTEGER, max: limits?.max?.y ?? Number.MAX_SAFE_INTEGER }, vector.y),
			Clamp({ min: limits?.min?.z ?? Number.MIN_SAFE_INTEGER, max: limits?.max?.z ?? Number.MAX_SAFE_INTEGER }, vector.z),
		)
	};
	/**
	 * * 向量 线性插值
	 *
	 * @param {Vector3} start - 向量对象
	 *
	 * @param {Vector3} done - 向量对象
	 *
	 * @param {number} time - 时间系数
	 *
	 * @returns {Vector3} - 计算后的向量对象
	 */
	public static lerp(start: Vector3, done: Vector3, time: number): VectorCase {
		return new VectorCase(
			start.x + (done.x - start.x) * time,
			start.y + (done.y - start.y) * time,
			start.z + (done.z - start.z) * time,
		)
	};
	/**
	 * * 向量 球面线性插值
	 *
	 * @param {Vector3} start - 向量对象
	 *
	 * @param {Vector3} done - 向量对象
	 *
	 * @param {number} time - 时间系数
	 *
	 * @returns {Vector3} - 计算后的向量对象
	 */
	public static slerp(start: Vector3, done: Vector3, time: number): VectorCase {
		/**
		 * * 计算两个向量点积
		 */
		const angleCosine = this.dot(start, done);
		/**
		 * * 确保角度余弦值在 [-1, 1] 区间内
		 */
		const safeAngleCosine = Math.min(Math.max(angleCosine, -1), 1);
		/**
		 * * 计算角度余弦值
		 */
		const angleTheta = Math.acos(safeAngleCosine);
		/**
		 * * 计算角度正弦值
		 */
		const angleSin = Math.sin(angleTheta);
		/**
		 * * 处理几乎平行的向量
		 */
		if (Math.abs(angleSin) < Number.EPSILON) return this.copy(start);
		/**
		 * * 计算插值比例 对应 vectorA
		 */
		const ratioA = Math.sin((1.0 - time) * angleTheta) / angleSin;
		/**
		 * * 计算插值比例 对应 vectorB
		 */
		const ratioB = Math.sin(time * angleTheta) / angleSin;
		// 根据插值比例对两个向量进行缩放, 然后相加得到插值结果
		return this.add(this.multiply(start, ratioA), this.multiply(done, ratioB));
	};
	/**
	 * * 计算两个向量的最大值
	 *
	 * @param {Vector3} vector1 - 向量对象
	 *
	 * @param {Vector3} vector2 - 向量对象
	 *
	 * @returns {Vector3} - 计算后的向量对象
	 */
	public static max(vector1: Vector3, vector2: Vector3): VectorCase {
		return new VectorCase(Math.max(vector1.x, vector2.x), Math.max(vector1.y, vector2.y), Math.max(vector1.z, vector2.z));
	};
	/**
	 * * 计算两个向量的最小值
	 *
	 * @param {Vector3} vector1 - 向量对象
	 *
	 * @param {Vector3} vector2 - 向量对象
	 *
	 * @returns {Vector3} - 计算后的向量对象
	 */
	public static min(vector1: Vector3, vector2: Vector3): VectorCase {
		return new VectorCase(Math.min(vector1.x, vector2.x), Math.min(vector1.y, vector2.y), Math.min(vector1.z, vector2.z));
	};
	/**
	 * * 在 Vector3 对象和另一个 Vector3 对象之间随机生成一个向量
	 *
	 * @param {Vector3} start - 进行计算的 Vector3 对象
	 *
	 * @param {Vector3} done - 进行计算的 Vector3 对象
	 *
	 * @returns {Vector} - 计算后的 Vector 对象
	 */
	public static rangeRandom(start: Vector3, done: Vector3): VectorCase {
		/** 计算向量最大值 */
		const maxVector = VectorTool.max(start, done);
		/** 计算向量最小值 */
		const minVector = VectorTool.min(start, done);
		// 随机生成一个向量
		return new VectorCase(
			RandomFloat(maxVector.x, minVector.x),
			RandomFloat(minVector.y, maxVector.y),
			RandomFloat(minVector.z, maxVector.z)
		);
	};
	/**
	 * * 在指定锚点周围生成一个随机向量
	 *
	 * @param {Vector3} [anchor] - 基准位置坐标
	 *
	 * @param {number} [range] - 随机偏移范围（每个轴的偏移值在 [-range, +range] 之间）
	 *
	 * @param {Vector3} [offset] - 偏移量（默认为 { x: 0, y: 0, z: 0 }）
	 *
	 * @returns {Vector} - 计算后的 Vector 对象
	 */
	public static random(anchor: Vector3, range: number, offset: Vector3 = VectorCase.CONSTANT_ZERO): VectorCase {
		return this.add(anchor, { x: RandomFloat(-range, range), y: RandomFloat(-range, range), z: RandomFloat(-range, range) }).add(offset);
	};
	/**
	 * 计算 Vector3 对象与另一个 Vector3 对象之间的归一化差向量
	 *
	 * @param {Vector3} from - 起始 Vector3 对象
	 *
	 * @param {Vector3} to - 目标 Vector3 对象
	 *
	 * @returns {VectorCase} - 归一化后的差向量（从起始向量指向目标向量）
	 */
	public static difference(from: Vector3, to: Vector3): VectorCase {
		// 处理向量相等的情况
		if (VectorTool.equals(from, to)) return VectorCase.CONSTANT_ZERO;
		// 计算两个向量的差向量
		const direction = this.subtract(to, from);
		// 返回归一化后的差向量
		return this.normalize(direction);
	};
	/**
	 * 基于实体旋转角度获取指向方向的三维向量
	 *
	 * @param {Vector2XY} rotation - 实体旋转角度（x 为偏航角，y 为俯仰角）
	 *
	 * @returns {VectorCase} - 指向方向的归一化三维向量
	 */
	public static AngleToPlace(rotation: Vector2XY): VectorCase {
		// 转换俯仰角为弧度（取反）
		const pitchRadians = -rotation.y * Math.PI / 180;
		// 转换偏航角为弧度（取反）
		const yawRadians = -rotation.x * Math.PI / 180;

		// 计算 x 轴分量
		const x = Math.sin(yawRadians) * Math.cos(pitchRadians);
		// 计算 y 轴分量
		const y = Math.sin(pitchRadians);
		// 计算 z 轴分量
		const z = Math.cos(yawRadians) * Math.cos(pitchRadians);

		// 返回指向的三维向量
		return new VectorCase(x, y, z);
	};
	/**
	 * 将三维方向向量转换为实体旋转角度（基于Minecraft坐标系规则）
	 *
	 * @param {Vector3} direction - 标准化后的三维方向向量
	 *
	 * @returns {Vector2XY} 实体旋转角度（pitch/x轴, yaw/y轴）
	 */
	public static Vector3ToAngle(direction: Vector3): VectorCase {
		/**
		 * 计算水平面投影长度
		 */
		const horizontalDist = Math.sqrt(direction.x ** 2 + direction.z ** 2);
		/**
		 * 计算偏航角（yaw）调整参数顺序和符号
		 */
		let yaw = Math.atan2(-direction.x, direction.z) * (180 / Math.PI);
		/**
		 * 计算俯仰角（pitch）并取反（Minecraft向下为正方向）
		 */
		let pitch = -Math.atan2(direction.y, horizontalDist) * (180 / Math.PI);
		// 处理极端情况（垂直方向）
		if (isNaN(pitch)) {
			pitch = direction.y > 0 ? -90 : 90; // 符号与原始计算相反
		}
		// 规范yaw到[-180, 180]范围
		yaw = ((yaw + 180) % 360 + 360) % 360 - 180;
		// 返回实体旋转角度
		return new VectorCase(pitch, yaw, 0);
	};
	/**
	 * 计算目标方向相关的方向向量集
	 *
	 * @param {Vector3} front - 前方方向向量（归一化）
	 *
	 * @returns {VectorDirections} - 包含前、后、左、右、上、下六个方向的向量对象
	 */
	public static directions(front: Vector3): VectorDirections {
		// 定义常量向量
		const sample: Vector3 = this.CONSTANT_UP;
		// 计算后方方向向量
		const back = this.normalize(this.multiply(front, -1));
		// 计算右方方向向量
		const right = this.normalize(this.cross(front, sample));
		// 计算左方方向向量
		const left = this.normalize(this.multiply(right, -1));
		// 计算下方方向向量
		const down = this.normalize(this.cross(front, right));
		// 计算上方方向向量
		const above = this.normalize(this.multiply(down, -1));
		// 返回计算结果
		return { front: VectorTool.copy(front), back, left, right, above, down };
	};
	/**
	 * 计算目标方向相关的坐标偏移
	 *
	 * @param {Vector3} source - 进行计算的源坐标
	 *
	 * @param {Vector3} front - 前方向向量（归一化）
	 *
	 * @param {VectorRelativeOffset} offset - 相对于前方、右方和上方的偏移量
	 *
	 * @returns {VectorCase} - 计算后的偏移坐标
	 */
	public static relativeOffset(source: Vector3, front: Vector3, offset: VectorRelativeOffset): VectorCase {
		// 计算方向向量集
		const directions = this.directions(front);
		// 计算前方偏移量
		const frontScale = this.multiply(front, offset.front);
		// 计算右方偏移量
		const rightScale = this.multiply(directions.right, offset.right);
		// 计算上方偏移量
		const upScale = this.multiply(directions.above, offset.above);
		// 返回偏移量
		return this.add(source, this.add(frontScale, this.add(upScale, rightScale)));
	};
	/**
	 * 计算提前量，用于预测移动目标的位置
	 *
	 * @param {Vector3} shooterPosition - 发射者 A 的位置
	 *
	 * @param {Vector3} targetPosition - 目标 B 的位置
	 *
	 * @param {number} projectileSpeed - 发射物的速度
	 *
	 * @param {Vector3} targetVelocity - 目标 B 的速度
	 *
	 * @returns {VectorCase} - 发射物的最佳速度向量
	 */
	public static calculateLeadVelocity(shooterPosition: Vector3, targetPosition: Vector3, projectileSpeed: number, targetVelocity: Vector3): VectorCase {
		// 计算从发射者到目标的向量
		const vecBA = this.subtract(targetPosition, shooterPosition);
		// 计算归一化的方向向量
		const normVecBA = this.normalize(vecBA);
		// 计算目标速度在方向向量上的分量
		const compVbBA = this.dot(targetVelocity, normVecBA);
		// 计算目标速度沿方向向量的分量
		const vbAlongBA = this.multiply(normVecBA, compVbBA);
		// 计算目标速度垂直于方向向量的分量
		const perpCompVa = this.subtract(targetVelocity, vbAlongBA);
		// 计算发射物速度在方向向量上的分量
		const magVaBA = Math.sqrt(projectileSpeed * projectileSpeed - this.dot(perpCompVa, perpCompVa));
		// 如果发射物速度在方向向量上的分量为负数，则返回目标速度的垂直分量
		if (magVaBA <= 0) return perpCompVa;
		// 计算发射物沿方向向量的速度分量（取反）
		const vaAlongBA = this.multiply(normVecBA, -magVaBA);
		// 计算最终的速度向量
		return this.add(perpCompVa, vaAlongBA);
	};
	/**
	 * * 获取区块坐标
	 *
	 * @param {Vector3} vector - 计算前的原始坐标
	 *
	 * @param {boolean} Yzero - 是否将 Y轴 的值设置为 0
	 *
	 * @param {number} size - 区块大小
	 *
	 * @returns {Vector3} - 计算后的区块坐标
	 */
	public static chunkLocation(vector: Vector3, Yzero: boolean = true, size: number = 16): VectorCase {
		/**
		 * * 计算 Y轴 的值
		 */
		const y = Yzero ? 0 : vector.y;
		// 返回计算结果
		return new VectorCase(Math.floor(vector.x / size) * size, y, Math.floor(vector.z / size) * size);
	};
}

export default VectorTool;