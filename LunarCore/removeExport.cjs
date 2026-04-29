/** 文件系统模块 */
const fs = require('fs');
/** 路径模块 */
const path = require('path');

/** 目标文件列表 */
const targetFiles = [
    path.join(__dirname, 'hierarchy', 'assets', 'agentSystem.js'),
    path.join(__dirname, 'hierarchy', 'assets', 'client', 'script.js')
];

// 遍历目标文件列表
for (const targetFile of targetFiles) {
    /** 读取文件内容 */
    let content = fs.readFileSync(targetFile, 'utf-8');
    /** 按行分割文件内容 */
    const lines = content.split('\n');
    /** 过滤出不包含 export 语句的行 */
    const filteredLines = lines.filter(line => !/^\s*export\s*\{/.test(line));
    // 如果过滤后的行数与原始行数不同，说明有 export 语句
    if (lines.length !== filteredLines.length) {
        // 写入新的文件内容
        fs.writeFileSync(targetFile, filteredLines.join('\n'), 'utf-8');
        // 打印处理信息 
        console.log(`Processed: ${targetFile}`);
        // 继续处理下一个文件
        continue;
    }
    // 打印处理信息 
    console.log(`No export line found in: ${targetFile}`);
}
