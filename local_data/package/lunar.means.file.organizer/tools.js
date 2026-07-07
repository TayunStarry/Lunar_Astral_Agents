// ==== 文件整理 - AI 工具定义 ====

// organize_files 工具：提交文件整理操作方案
const ORGANIZE_TOOL = {
    type: 'function',
    function: {
        name: 'organize_files',
        description: '提交文件整理操作方案。每个文件必须有一条操作，不能遗漏任何文件。',
        parameters: {
            type: 'object',
            properties: {
                operations: {
                    type: 'array',
                    description: '文件整理操作列表，每个文件一条操作',
                    items: {
                        type: 'object',
                        properties: {
                            type: {
                                type: 'string',
                                enum: ['move', 'rename', 'merge', 'delete'],
                                description: '操作类型：move=移动文件到分类子文件夹, rename=重命名文件, merge=合并文件夹, delete=删除文件'
                            },
                            source: {
                                type: 'string',
                                description: '文件的原始完整路径'
                            },
                            target: {
                                type: 'string',
                                description: '目标完整路径（必须包含文件名，不能只是文件夹路径）。delete操作不需要此字段。'
                            }
                        },
                        required: ['type', 'source']
                    }
                }
            },
            required: ['operations']
        }
    }
};