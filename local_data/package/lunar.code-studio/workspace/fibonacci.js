/**
 * 计算斐波那契数列
 * @param {number} n - 第几个位置的数字
 * @returns {number} - 斐波那契数列中的数值
 */
function fibonacci(n) {
    if (n <= 1) return n;
    let prev = 0;
    let curr = 1;
    for (let i = 2; i <= n; i++) {
        let next = prev + curr;
        prev = curr;
        curr = next;
    }
    return curr;
}

// 测试并输出结果
const target = 10;
const result = fibonacci(target);
console.log(`斐波那契数列第 ${target} 位的值是: ${result}`);

// 同时在页面上显示结果
document.body.innerHTML = `<h1>斐波那契计算结果</h1>
<p>第 ${target} 位的值是：<strong>${result}</strong></p>`;
