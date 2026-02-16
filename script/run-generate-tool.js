import fs from 'fs';
import path from 'path';
import { fileURLToPath } from 'url';
import { execSync } from 'child_process';

// 获取当前文件的目录路径
const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);

// 获取Lunar-Tool-Package目录下的所有.ts文件
const toolPackageDir = path.join(__dirname, 'Lunar-Tool-Package');
const files = fs.readdirSync(toolPackageDir);

// 过滤出.ts文件
const tsFiles = files.filter(file => path.extname(file) === '.ts');

// 遍历每个.ts文件，提取文件名并调用generate-tool.mjs
tsFiles.forEach(file => {
    // 提取文件名（不含扩展名）
    const fileName = path.basename(file, '.ts');

    // 构建命令
    const command = `node ${path.join(__dirname, 'generate-tool.mjs')} ${fileName}`;

    console.log(`Executing: ${command}`);

    try {
        // 执行命令
        const output = execSync(command, { encoding: 'utf-8' });
        console.log(output);
    } catch (error) {
        console.error(`Error executing command for ${fileName}: ${error.message}`);
        if (error.stdout) console.error(error.stdout);
        if (error.stderr) console.error(error.stderr);
    }
});
