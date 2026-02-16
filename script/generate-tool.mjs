import { execSync } from 'child_process';
import fs from 'fs';
import path from 'path';
import { fileURLToPath } from 'url';

/** 工具函数：解析命令行参数 */
function parseArguments() {
	/** 解析命令行参数，获取文件名（无扩展名） */
	const fileName = process.argv[2];
	if (!fileName) {
		console.log('使用方法: node generate-tool.js <文件名>');
		process.exit(1);
	}
	/** 移除文件名中的.ts扩展名 */
	const baseName = fileName.replace(/\.ts$/, '');
	// 返回处理后的文件名（无扩展名）
	return baseName;
}

/** 工具函数：检查文件是否存在 */
function checkFileExists(filePath, description) {
	if (!fs.existsSync(filePath)) {
		console.error(`错误: ${description}不存在: ${filePath}`);
		process.exit(1);
	}
}

/** 工具函数：执行TypeScript编译 */
function compileTypeScript(tsFilePath, cacheDir) {
	/** 构建TypeScript编译命令 */
	const tscCommand = `tsc "${tsFilePath}" --moduleResolution node --target es2022 --outDir "${cacheDir}"`;
	try {
		execSync(tscCommand, { stdio: 'inherit' });
	}
	catch (error) {
		console.error('TypeScript编译失败:', error.message);
		process.exit(1);
	}
}

/** 工具函数：读取JSON工具定义文件 */
function readToolDefinition(jsonFilePath) {
	try {
		const jsonContent = fs.readFileSync(jsonFilePath, 'utf-8');
		// 解析JSON以确保格式正确
		const parsedJson = JSON.parse(jsonContent);
		// 重新格式化为美观的JSON字符串
		return JSON.stringify(parsedJson, null, 2);
	}
	catch (error) {
		console.error(`读取JSON文件失败: ${jsonFilePath}`, error.message);
		process.exit(1);
	}
}

/** 工具函数：读取模块描述Markdown文件（可选） */
function readModuleDescription(mdFilePath) {
	if (!fs.existsSync(mdFilePath)) {
		console.log(`注意: 模块描述文件不存在，使用默认描述: ${mdFilePath}`);
		return null;
	}

	try {
		return fs.readFileSync(mdFilePath, 'utf-8');
	}
	catch (error) {
		console.error(`读取模块描述文件失败: ${mdFilePath}`, error.message);
		// 如果读取失败，使用默认描述
		return null;
	}
}

/** 工具函数：读取和处理JavaScript实现文件 */
function readJavaScriptImplementation(jsFilePath) {
	try {
		const jsContent = fs.readFileSync(jsFilePath, 'utf-8');
		// 替换导入路径
		return jsContent.replace(/\.\.\/EntryAPI\/code/g, './script.js');
	}
	catch (error) {
		console.error(`读取JavaScript文件失败: ${jsFilePath}`, error.message);
		process.exit(1);
	}
}

/** 工具函数：生成Markdown模板 */
function generateMarkdownTemplate(toolDefinition, jsImplementation, moduleDescription, baseName) {
	// 获取当前日期
	const now = new Date();
	const year = now.getFullYear();
	const month = String(now.getMonth() + 1).padStart(2, '0');
	const day = String(now.getDate()).padStart(2, '0');
	const currentDate = `${year}-${month}-${day}`;

	// 默认模块描述
	const defaultModuleDescription = [
		'# 模块概述\n',
		'专为 Lunar-Astral-Agents 项目设计编写的 LTP 协议兼容工具模块。本模块遵循 LTP 协议规范，包含完整的工具定义、实现逻辑及相关依赖说明。\n'
	].join('\n');

	// 使用自定义模块描述或默认描述
	const moduleOverview = moduleDescription || defaultModuleDescription;

	return [
		'# LTP 工具模块\n',
		'## 元数据\n',
		`- **名称**：${baseName}`,
		'- **版本**：1.0.0',
		'- **作者**：[钛宇-星光阁](https://gitee.com/TayunStarry)',
		'- **更新日志**：',
		`  - ${currentDate}：创建初始版本\n`,
		'#' + moduleOverview,
		'## 协议信息\n',
		'- **协议名称**：LTP (Lunar Tool Package Protocol)',
		'- **协议全称**：Lunar Tool Package Protocol',
		'- **协议中文名**：月华工具包协议\n',
		'### 设计意图\n',
		'本协议采用 **"一体化模块文件"** 设计理念，将 JSON 工具定义、JavaScript 实现代码和模块文档整合在单个 Markdown 文件中，为 Lunar-Astral-Agents 项目提供：\n',
		'- **标准化**：统一工具扩展格式与规范',
		'- **可插拔**：即插即用的模块化架构',
		'- **自描述**：代码与文档一体化，便于理解与维护',
		'- **易分发**：单个文件包含完整功能，便于共享与部署\n',
		'### 设计原则\n',
		'遵循以下原则设计AI智能体工具：\n',
		'1. **智能体中心**：为AI智能体设计合适的工具，而不是把AI智能体做成工具',
		'2. **被动调用**：工具应被动等待AI智能体调用，而非主动调用或控制AI智能体',
		'3. **功能专注**：工具应专注于自身功能实现，避免肆意设计工具去调用和控制AI',
		'4. **克制干预**：通过 `import from \'./script.js\'` 可访问智能体数据，但应保持设计克制，不过度干预智能体主体运行逻辑',
		'5. **使用者定位**：工具的使用者应是AI智能体，而非人类用户，按适合AI智能体使用的角度设计接口\n',
		'## 工具定义\n',
		'```json',
		toolDefinition,
		'```\n',
		'## 模块实现\n',
		'```javascript',
		jsImplementation,
		'```\n',
		'## 依赖说明\n',
		'- **环境要求**：Lunar-Astral-Agents（版本 ≥ 2026-01-19）',
		'- **兼容性**：支持 WebSocket 的现代浏览器（如 Chrome、Firefox 最新版本）',
		'- **协议兼容**：LTP v1.0\n'
	].join('\n');
}

// 主函数
async function main() {
	try {
		/** 解析命令行参数 */
		const baseName = parseArguments();

		// 定义文件路径
		const scriptDir = path.join('.', 'script', 'Lunar-Tool-Package');

		/** TypeScript文件路径 */
		const tsFilePath = path.join(scriptDir, `${baseName}.ts`);
		/** JSON工具定义文件路径 */
		const jsonFilePath = path.join(scriptDir, `${baseName}.json`);
		/** Markdown模块描述文件路径 */
		const mdFilePath = path.join(scriptDir, `${baseName}.md`);
		/** 编译输出目录 */
		const deliverableDir = path.join('.', 'deliverable');
		/** 编译后的JS文件路径 */
		const jsFilePath = path.join(deliverableDir, 'Lunar-Tool-Package', `${baseName}.js`);
		/** 生成的LTP Markdown文件路径 */
		const ltpFilePath = path.join('.', 'local_data', 'resources', 'package', `${baseName}.ltp.md`);

		// 检查必要的文件是否存在
		checkFileExists(tsFilePath, 'TypeScript文件');
		checkFileExists(jsonFilePath, 'JSON工具定义文件');

		// 编译TypeScript文件
		compileTypeScript(tsFilePath, deliverableDir);

		// 检查编译后的JS文件是否存在
		checkFileExists(jsFilePath, '编译后的JS文件');

		// 读取三个文件的内容
		const toolDefinition = readToolDefinition(jsonFilePath);
		const moduleDescription = readModuleDescription(mdFilePath);
		const jsImplementation = readJavaScriptImplementation(jsFilePath);

		/** 生成Markdown模板 */
		const markdownTemplate = generateMarkdownTemplate(toolDefinition, jsImplementation, moduleDescription, baseName);

		// 确保目标目录存在
		const ltpDir = path.dirname(ltpFilePath);
		if (!fs.existsSync(ltpDir)) {
			fs.mkdirSync(ltpDir, { recursive: true });
		}

		// 保存Markdown文件
		fs.writeFileSync(ltpFilePath, markdownTemplate, 'utf-8');

		// 打印成功消息
		console.log(`LTP文档已成功生成: ${ltpFilePath}`);
		if (moduleDescription === null) {
			console.log(`注意: 使用了默认模块描述，如需自定义请创建: ${mdFilePath}`);
		}
	}
	catch (error) {
		console.error('处理过程中发生错误:', error.message);
		process.exit(1);
	}
}

// 执行主函数
if (process.argv[1] === fileURLToPath(import.meta.url)) {
	main();
}