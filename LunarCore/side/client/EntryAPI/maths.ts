/*
 * 导出模块
 */
export { CalculateMedian, CalculateModes, RandomFloor, RandomFloat, Clamp, calculateCosineSimilarity };

/**
 * * 将数值限制在指定的最小值和最大值范围内
 *
 * @param {type.Vertex} input 包含数字范围的 Vertex 对象
 *
 * @param {number} value 用于测试的数值
 *
 * @returns {number} 限制后的数值, 确保在 [range.min, range.max] 区间内
 */
function Clamp(input: { min: number, max: number }, value: number): number {
    return Math.max(input.min, Math.min(input.max, value));
};

/**
 * * 生成指定范围内的随机整数
 *
 * @param {number} min - 范围的最小值（包含在内）
 *
 * @param {number} max - 范围的最大值（包含在内）
 *
 * @returns {number} 返回 min 和 max 之间的一个随机整数, 包括 min 和 max
 */
function RandomFloor(min: number, max: number): number {
    return Math.floor(Math.random() * (max - min + 1) + min);
};

/**
 * * 生成一个在指定范围内的随机浮点数, 并保留指定的小数位数
 *
 * @param {number} min - 随机数范围的最小值（包含）
 *
 * @param {number} max - 随机数范围的最大值（包含）
 *
 * @param {number} length - 返回的浮点数的小数位数, 默认为2
 *
 * @returns {number} 在指定范围内的随机浮点数, 保留指定的小数位数
 */
function RandomFloat(min: number, max: number, length: number = 2): number {
    return Number((Math.random() * (max - min) + min).toFixed(length));
};

/**
 * * 计算数组的中位数
 *
 * @param {number[]} numbers - 输入的数字数组
 *
 * @returns {number} - 返回数组的中位数
 */
function CalculateMedian(numbers: number[]): number {
    /**
     * * 复制输入的数组并排序, 避免修改原数组
     */
    const sortedNumbers = [...numbers].sort((a, b) => a - b);
    /**
     * * 计算中位数索引
     */
    const middleIndex = Math.floor(sortedNumbers.length / 2);
    // 如果数组长度是偶数, 返回中间两个数的平均
    if (sortedNumbers.length % 2 === 0) return (sortedNumbers[middleIndex - 1] + sortedNumbers[middleIndex]) / 2;
    // 如果数组长度是奇数, 返回中间的数
    else return sortedNumbers[middleIndex];
};

/**
 * * 计算数组中的众数
 *
 * @param {number[]} numbers - 输入的数字数组
 *
 * @returns {number[]} - 返回一个包含所有众数的数组
 */
function CalculateModes(numbers: number[]): number[] {
    /**
     * * 用于存储数字出现的频率
     */
    const frequencyMap = new Map<number, number>();
    /**
     * * 用于存储最大频率
     */
    let maxFrequency = 0;
    /**
     * * 用于存储所有众数
     */
    const modes: number[] = [];
    // 遍历数组, 统计每个数字出现的频率
    for (const number of numbers) {
        /**
         * * 获取当前数字的频率
         */
        const frequency = (frequencyMap.get(number) || 0) + 1;
        // 更新频率映射
        frequencyMap.set(number, frequency);
        // 更新最大频率
        if (frequency > maxFrequency) maxFrequency = frequency;
    };
    // 再次遍历频率映射, 找出所有众数
    frequencyMap.forEach(
        (frequency, number) => {
            if (frequency === maxFrequency) modes.push(number);
        }
    );
    // 返回所有众数
    return modes;
};

/**
 * 计算两个向量的余弦相似度
 * @param a 第一个向量
 * @param b 第二个向量
 * @returns 余弦相似度值
 */
function calculateCosineSimilarity(a: number[], b: number[]): number {
    // 确保两个向量长度相同
    if (a.length !== b.length) {
        throw new Error("向量长度不匹配");
    }

    let dotProduct = 0;
    let normA = 0;
    let normB = 0;

    // 计算点积和向量的范数
    for (let i = 0; i < a.length; i++) {
        dotProduct += a[i] * b[i];
        normA += a[i] * a[i];
        normB += b[i] * b[i];
    }

    // 避免除以零
    if (normA === 0 || normB === 0) {
        return 0;
    }

    // 计算余弦相似度
    return dotProduct / (Math.sqrt(normA) * Math.sqrt(normB));
};