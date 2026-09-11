/** 加载脚本依赖列表，返回「全部就绪」的 Promise
 *  单个脚本失败不再中断整条链，只记录并继续加载后续依赖。 */
async function loadScriptDependencies(urls) {
    const results = {};
    for (const url of urls) {
        /** 加载脚本的事件处理函数 */
        results[url] = await new Promise(resolve => {
            /** 创建脚本元素 */
            const script = document.createElement('script');
            // 设置脚本元素的src属性
            script.src = url;
            // 设置脚本元素的onload事件处理函数
            script.onload = () => resolve(true);
            // 设置脚本元素的onerror事件处理函数
            script.onerror = () => {
                console.error('[standard_dependency] 依赖加载失败: ' + url);
                resolve(false);
            };
            // 将脚本元素添加到文档的head元素中
            document.head.appendChild(script);
        });
    }
    return results;
};

/** 脚本依赖列表 */
const scriptDependencies = [
    // Markdown解析库
    '/file/read/package/marked.min.js',
    // ECharts 库
    '/file/read/package/echarts.min.js',
    // 代码高亮库
    '/file/read/package/highlight/highlight.min.js',
    // 二维码库
    '/file/read/package/qrcode.min.js',
    // Katex 数学公式渲染 库
    '/file/read/package/katex/katex.min.js',
    // Katex 数学公式渲染 库 自动渲染插件
    '/file/read/package/katex/contrib/auto-render.min.js',
    // 多媒体预览 库
    '/file/read/package/multimedia_preview/script.js',
];

// 加载脚本依赖列表，并把就绪 Promise 暴露到全局。
// 调用方（如 tool_viewer）必须 await window.scriptDependenciesReady 后再使用
// marked/hljs 等全局对象，否则会与这里的异步注入竞态。
window.scriptDependenciesReady = loadScriptDependencies(scriptDependencies);