/** 加载脚本的事件处理函数 */
async function loadScriptDependencies(urls) {
    for (const url of urls) {
        /** 加载脚本的事件处理函数 */
        function event(resolve, reject) {
            /** 创建脚本元素 */
            const script = document.createElement('script');
            // 设置脚本元素的src属性
            script.src = url;
            // 设置脚本元素的onload事件处理函数
            script.onload = resolve;
            // 设置脚本元素的onerror事件处理函数
            script.onerror = reject;
            // 将脚本元素添加到文档的head元素中
            document.head.appendChild(script);
        }
        // 等待脚本加载完成
        await new Promise(event);
    }
};

/** 脚本依赖列表 */
const scriptDependencies = [
    // Markdown解析库
    '/read/package/marked.min.js',
    // ECharts 库
    '/read/package/echarts.min.js',
    // 代码高亮库
    '/read/package/highlight/highlight.min.js',
    // Live2D Cubism 核心库
    '/read/package/live2dcubismcore.min.js',
    // Pixi.js 库
    '/read/package/pixi.5.3.12.min.js',
    // Pixi.js 库 Live2D Cubism 插件
    '/read/package/pixi-live2d-display-cubism4.min.js',
    // 二维码库
    '/read/package/qrcode.min.js',
    // Katex 数学公式渲染 库
    '/read/package/katex/katex.min.js',
    // Katex 数学公式渲染 库 自动渲染插件
    '/read/package/katex/contrib/auto-render.min.js',
    // 多媒体预览 库
    '/read/package/multimedia_preview/script.js',
];

// 加载脚本依赖列表
loadScriptDependencies(scriptDependencies);