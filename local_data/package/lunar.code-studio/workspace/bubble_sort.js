/**
 * 冒泡排序算法实现
 * @param {Array} arr - 需要排序的数组
 * @returns {Array} - 排序后的数组
 */
function bubbleSort(arr) {
    let n = arr.length;
    // 创建副本防止修改原数组
    let sortedArr = [...arr];
    
    for (let i = 0; i < n; i++) {
        for (let j = 0; j < n - i - 1; j++) {
            if (sortedArr[j] > sortedArr[j + 1]) {
                // 交换元素
                let temp = sortedArr[j];
                sortedArr[j] = sortedArr[j + 1];
                sortedArr[j + 1] = temp;
            }
        }
    }
    return sortedArr;
}

// 测试数据
const unsortedData = [64, 34, 25, 12, 22, 11, 90];
console.log("原始数组:", unsortedData);

const sortedData = bubbleSort(unsortedData);
console.log("排序后数组:", sortedData);

// 在页面上显示结果
document.body.innerHTML = `
    <div style="font-family: sans-serif; padding: 20px;">
        <h2>冒泡排序结果</h2>
        <p><strong>原始数组:</strong> ${unsortedData.join(', ')}</p>
        <p><strong>排序后数组:</strong> <span style="color: green; font-weight: bold;">${sortedData.join(', ')}</span></p>
    </div>
`;