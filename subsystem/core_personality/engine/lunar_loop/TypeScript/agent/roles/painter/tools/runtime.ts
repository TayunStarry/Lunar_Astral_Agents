import { ToolCall } from '../../../../config/tool';

/** 运行时管理工具：模型方案切换、图片清单与超分 */
export const runtimeTools: ToolCall[] = [
	{
		type: "function",
		function: {
			name: "switch_diffusion_scheme",
			description: "切换后续绘图使用的模型方案。quality=质量优先(默认,细节更好、耗时更长);turbo=速度优先(出图更快,适合草图、快速确认与批量尝试,推荐 8~12 步)。切换后省略 steps 即使用该方案的推荐步数。切换状态由宿主记录,对所有后续绘图工具生效;任务本身不需要画得快时不要切换,保持当前方案即可",
			parameters: {
				type: "object",
				properties: {
					"scheme": {
						type: "string",
						enum: ["turbo", "quality"],
						description: "目标方案:turbo=速度优先,quality=质量优先"
					}
				},
				required: [
					"scheme"
				]
			}
		}
	},
	{
		type: "function",
		function: {
			name: "list_images",
			description: "查看资源目录中最近的图片句柄与内容描述（按登记时间倒序，越靠前越新）。清单只含文字信息，可用于了解历史图片、确认句柄或为超分定位目标。需要图生图基座时由对话者经 dispatch_painter 的 reference_tag 指定，任务消息会直接附上图片本体",
			parameters: {
				type: "object",
				properties: {}
			}
		}
	},
	{
		type: "function",
		function: {
			name: "upscale_image",
			description: "对已有图片执行 RealESRGAN 超分辨率放大（4x）,提升清晰度与细节。用户要求把某张图片变清晰、放大或提升画质时调用;不指定句柄时对最近一次生成的作品超分。与生成工具的 super_resolution 参数不同,本工具不重新生成图像,只做纯超分",
			parameters: {
				type: "object",
				properties: {
					"image_tag": {
						type: "string",
						description: "图片句柄（如 @I.003），对目录中指定图片超分。不填时对最近一次生成的作品超分"
					}
				}
			}
		}
	}
];
