import { Config, ToolCall } from '../../config/index';

export class OnlyData {
    /** 当前配置面板选项 */
    public static configurePanelOption: string = 'any';
    /** 系统URL */
    public static systemUrl: string = '';
    /** 文件服务URL */
    public static fileServiceUrl: string = '';
    /** 系统默认 API 密钥 */
    private static systemKey = 'key-520-1314-2000-02-18';
    /** 默认的嵌入模型的名称 */
    private static modelEmbedingName = "system-embedding";
    /** 默认的多模态模型名称 */
    private static modelMultimodalName = "system-multimodal";
    /** 自定义配置项 */
    public static customConfig: Config = {};
    /** 工具调用配置 */
    public static toolCall: ToolCall[] = [];
    /** 是否正在拖拽文件，初始值为 false */
    public static isFileDragging: boolean = false;
    /** 支持的图片文件扩展名 */
    public static readonly imageFormatsExtensions: string[] = ['.jpg', '.jpeg', '.png', '.gif', '.bmp', '.webp'];
    /** 支持的视频文件扩展名 */
    public static readonly videoFormatsExtensions: string[] = ['.mp4', '.avi', '.mov', '.wmv', '.flv', '.mkv'];
    /** 支持的文件扩展名 */
    public static readonly fileValidExtensions: string[] = [
        // 纯文本文件
        '.txt', '.md', '.log', '.ini', '.conf',
        // 常见代码文件
        '.js', '.jsx', '.ts', '.tsx', '.py', '.java', '.c', '.cpp', '.h',
        '.cs', '.php', '.rb', '.go', '.rs', '.swift', '.kt', '.dart',
        // 数据格式文件
        '.json', '.csv', '.xml', '.yaml', '.yml',
        // 标记语言和样式文件
        '.html', '.htm', '.css', '.scss', '.less', '.sass', '.styl',
        // 配置文件
        '.env', '.properties', '.toml',
        // 常见图片文件和视频文件
        ...this.imageFormatsExtensions,
        ...this.videoFormatsExtensions
    ];
    /** 支持的文件 MIME 类型 */
    public static readonly fileValidTypes: string[] = [
        // JSON 数据格式
        'application/json',
        // XML 数据格式
        'application/xml',
        // YAML 数据格式
        'application/x-yaml'
    ];
    /** 支持的视觉文件扩展名 */
    public static readonly visionExtensions: string[] = [...this.imageFormatsExtensions, ...this.videoFormatsExtensions];
    /** 获取 多模态模型 URL */
    public static get MultimodalUrl(): string {
        return OnlyData.customConfig.multimodalModelUrl || OnlyData.systemUrl;
    };
    /** 获取 多模态模型 API 密钥 */
    public static get MultimodalKey(): string {
        return OnlyData.customConfig.multimodalModelKey || OnlyData.systemKey;
    };
    /** 获取 多模态模型名称 */
    public static get MultimodalName(): string {
        return OnlyData.customConfig.multimodalModelName || OnlyData.modelMultimodalName;
    };
    /** 获取 嵌入模型 URL */
    public static get EmbeddingUrl(): string {
        return OnlyData.customConfig.embeddingModelUrl || OnlyData.systemUrl;
    };
    /** 获取 嵌入模型 API 密钥 */
    public static get EmbeddingKey(): string {
        return OnlyData.customConfig.embeddingModelKey || OnlyData.systemKey;
    };
    /** 获取 嵌入模型名称 */
    public static get EmbeddingName(): string {
        return OnlyData.customConfig.embeddingModelName || OnlyData.modelEmbedingName;
    };
    /** 设置 全局URL */
    public static set globalUrl(url: string) {
        OnlyData.systemUrl = url;
        OnlyData.fileServiceUrl = url + '/v1';
    }
};