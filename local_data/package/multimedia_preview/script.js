/**
 * 图片预览组件，支持缩放、拖拽、键盘控制等功能
 * @class ImagePreview
 */
class MultimediaPreview {
	/** 创建图片预览实例 */
	constructor() {
		/**
		 * 模态框元素
		 * @type {HTMLDivElement|null}
		 */
		this.modal = null;
		/**
		 * 图片元素
		 * @type {HTMLImageElement|null}
		 */
		this.img = null;
		/**
		 * 视频元素
		 * @type {HTMLVideoElement|null}
		 */
		this.video = null;
		/**
		 * 图片容器元素
		 * @type {HTMLDivElement|null}
		 */
		this.imgContainer = null;
		/**
		 * 缩放信息显示元素
		 * @type {HTMLElement|null}
		 */
		this.zoomInfo = null;
		/**
		 * 当前媒体类型：'image' 或 'video'
		 * @type {'image'|'video'}
		 */
		this.mediaType = 'image';
		/**
		 * 是否正在拖拽
		 * @type {boolean}
		 */
		this.isDragging = false;
		/**
		 * 拖拽开始时的X坐标
		 * @type {number}
		 */
		this.startX = 0;
		/**
		 * 拖拽开始时的Y坐标
		 * @type {number}
		 */
		this.startY = 0;
		/**
		 * 当前图片X偏移量
		 * @type {number}
		 */
		this.currentX = 0;
		/**
		 * 当前图片Y偏移量
		 * @type {number}
		 */
		this.currentY = 0;
		/**
		 * 当前缩放比例
		 * @type {number}
		 */
		this.currentScale = 1;
		/**
		 * 鼠标移动事件处理函数
		 * @type {function(Event): void}
		 */
		this.handleMouseMove = this.handleMouseMove.bind(this);
		/**
		 * 鼠标抬起事件处理函数
		 * @type {function(Event): void}
		 */
		this.handleMouseUp = this.handleMouseUp.bind(this);
		/**
		 * 键盘按键事件处理函数
		 * @type {function(KeyboardEvent): void}
		 */
		this.handleKeyDown = this.handleKeyDown.bind(this);
		/**
		 * 模板HTML内容
		 * @type {string}
		 */
		this.templateHTML = '';
		// 初始化组件
		this.initTemplateHTML();
	}
	/**
	 * 初始化组件，加载模板HTML
	 * @async
	 * @returns {Promise<void>}
	 */
	async initTemplateHTML() {
		this.templateHTML = await fetch('/read/package/multimedia_preview/index.html').then(res => res.text());
	}
	/**
	 * 检测文件类型
	 * @param {string} path - 文件路径
	 * @returns {string} - 返回 'image', 'video' 或 'audio'
	 */
	detectMediaType(path) {
		/** 视频文件扩展名列表 */
		const videoExtensions = ['.mp4', '.webm', '.ogg', '.mov', '.avi', '.mkv', '.flv', '.wmv', '.m4v'];
		/** 音频文件扩展名列表 */
		const audioExtensions = ['.mp3', '.wav', '.ogg', '.flac', '.aac', '.m4a', '.wma'];
		/** 文件扩展名 */
		const extension = path.toLowerCase().substring(path.lastIndexOf('.'));
		// 检查文件扩展名是否在视频扩展名列表中
		if (videoExtensions.includes(extension)) return 'video';
		// 检查文件扩展名是否在音频扩展名列表中
		if (audioExtensions.includes(extension)) return 'audio';
		// 默认返回图片类型
		return 'image';
	}
	/**
	 * 预览媒体文件
	 * @async
	 * @param {string} path - 文件路径
	 * @param {string} fileName - 文件名
	 * @returns {Promise<void>}
	 */
	async preview(path, fileName) {
		// 如果模板未加载，先加载模板
		if (!this.templateHTML) await this.initTemplateHTML();
		// 检测媒体类型
		this.mediaType = this.detectMediaType(path);
		/** 置换模板中的变量 */
		const html = this.templateHTML
			.replace(/{{path}}/g, path)
			.replace(/{{fileName}}/g, fileName)
			.replace(/{{scale}}/g, '1')
			.replace(/{{x}}/g, '0')
			.replace(/{{y}}/g, '0')
			.replace(/{{scalePercent}}/g, '100')
			.replace(/{{filePath}}/g, path.replace(/^generated[/]/, ''))
			.replace(/{{default-image}}/g, `/read/images/placeholder/unknown_file_icon-0${Math.floor(Math.random() * 5)}.png`);
		// 创建模态框
		this.modal = document.createElement('div');
		this.modal.innerHTML = html;
		this.modal = this.modal.firstElementChild;
		// 加载媒体文件
		document.body.appendChild(this.modal);
		// 获取元素引用
		this.img = this.modal.querySelector('.image-preview');
		this.video = this.modal.querySelector('.video-preview');
		this.imgContainer = this.modal.querySelector('.image-drag-container');
		this.zoomInfo = this.modal.querySelector('.zoom-info');
		// 初始化状态
		this.currentScale = 1;
		this.currentX = 0;
		this.currentY = 0;
		/** 视频模式隐藏缩放控制 */
		const zoomGroup = this.modal.querySelector('.control-group');
		/** 视频模式隐藏重置按钮 */
		const resetBtn = this.modal.querySelector('.reset-button');
		/** 视频模式隐藏缩放控制分隔线 */
		const zoomDivider = this.modal.querySelectorAll('.control-divider')[0];
		/** 视频模式隐藏重置按钮分隔线 */
		const resetDivider = this.modal.querySelectorAll('.control-divider')[1];
		// 根据媒体类型设置显示
		if (this.mediaType === 'video' || this.mediaType === 'audio') {
			// 显示视频/音频，隐藏图片
			this.img.style.display = 'none';
			this.video.style.display = 'block';
			this.video.src = path;
			this.video.load();
			// 视频/音频模式不需要拖拽容器
			if (this.imgContainer) this.imgContainer.style.display = 'none';
			// 视频/音频模式隐藏缩放控制
			if (zoomGroup) zoomGroup.style.display = 'none';
			if (resetBtn) resetBtn.style.display = 'none';
			if (zoomDivider) zoomDivider.style.display = 'none';
			if (resetDivider) resetDivider.style.display = 'none';
		}
		else {
			// 显示图片，隐藏视频
			this.img.style.display = 'block';
			this.video.style.display = 'none';
			// 确保图片容器显示
			if (this.imgContainer) this.imgContainer.style.display = 'block';
			// 图片模式显示所有控制
			if (zoomGroup) zoomGroup.style.display = 'flex';
			if (resetBtn) resetBtn.style.display = 'flex';
			if (zoomDivider) zoomDivider.style.display = 'block';
			if (resetDivider) resetDivider.style.display = 'block';
			/** 图片模式显示信息栏 */
			const infoElement = this.modal.querySelector('.image-info');
			// 更新信息栏显示图片信息
			if (infoElement) infoElement.textContent = fileName;
		}
		// 绑定事件
		this.bindEvents();
		// 添加键盘事件监听
		document.addEventListener('keydown', this.handleKeyDown);
	}
	/**
	 * 绑定事件监听器
	 * @returns {void}
	 */
	bindEvents() {
		// 只在图片模式下绑定拖拽和缩放事件
		if (this.mediaType === 'image' && this.img && this.imgContainer) {
			// 拖拽事件
			this.imgContainer.addEventListener('mousedown', e => this.startDragging(e));
			// 滚轮缩放
			this.modal.addEventListener('wheel',
				e => {
					// 阻止默认滚轮行为
					e.preventDefault();
					/** 计算缩放因子，根据滚轮方向 */
					const scaleFactor = e.deltaY > 0 ? 0.9 : 1.1;
					// 调用缩放函数
					this.zoom(scaleFactor, e.clientX, e.clientY);
				}
			);
		}
		// 图片模式绑定缩放按钮事件
		this.modal.querySelector('.zoom-in-btn').addEventListener('click', () => this.zoom(1.2));
		// 图片模式绑定缩放按钮事件
		this.modal.querySelector('.zoom-out-btn').addEventListener('click', () => this.zoom(0.8));
		// 图片模式绑定重置按钮事件
		this.modal.querySelector('.reset-button').addEventListener('click', () => this.reset());
		// 图片模式绑定关闭按钮事件
		this.modal.querySelector('.close-button').addEventListener('click', () => this.close());
		// 视频/音频播放相关事件
		if ((this.mediaType === 'video' || this.mediaType === 'audio') && this.video) {
			// 视频/音频播放错误处理
			this.video.addEventListener('error',
				e => {
					console.error('媒体加载错误:', e);
					/** 媒体模式显示信息栏 */
					const infoElement = this.modal.querySelector('.image-info');
					// 媒体模式显示信息栏
					if (infoElement) infoElement.textContent = `❌ ${this.mediaType === 'video' ? '视频' : '音频'}加载失败，格式可能不受支持`;
				}
			);
			// 视频/音频加载完成
			this.video.addEventListener('loadeddata',
				() => {
					/** 媒体模式显示信息栏 */
					const infoElement = this.modal.querySelector('.image-info');
					// 媒体模式显示信息栏是否存在
					if (!infoElement) return;
					/** 媒体时长 */
					const duration = this.video.duration;
					/** 媒体时长分钟数 */
					const minutes = Math.floor(duration / 60);
					/** 媒体时长秒数 */
					const seconds = Math.floor(duration % 60);
					// 媒体模式显示信息栏
					infoElement.textContent = `${this.video.src.split('/').pop()} ( ${minutes} : ${seconds.toString().padStart(2, '0')} )`;
				}
			);
		}
	};
	/**
	 * 开始拖拽图片
	 * @param {MouseEvent} e - 鼠标事件对象
	 * @returns {void}
	 */
	startDragging(e) {
		// 仅在图片模式下且鼠标左键点击时触发
		if (e.button !== 0 || this.mediaType !== 'image') return;
		// 标记开始拖拽
		this.isDragging = true;
		// 记录鼠标按下时的初始坐标
		this.startX = e.clientX;
		this.startY = e.clientY;
		// 获取图片当前的偏移量，若无则默认为 0
		this.currentX = parseFloat(this.img.getAttribute('data-x')) || 0;
		this.currentY = parseFloat(this.img.getAttribute('data-y')) || 0;
		// 禁用过渡动画，避免拖拽时卡顿
		this.img.style.transition = 'none';
		// 启用硬件加速，提升拖拽性能
		this.img.style.willChange = 'transform';
		// 改变鼠标样式为“抓取中”
		this.imgContainer.style.cursor = 'grabbing';
		// 绑定鼠标移动与抬起事件到 document，确保拖拽过程流畅
		document.addEventListener('mousemove', this.handleMouseMove);
		document.addEventListener('mouseup', this.handleMouseUp);
		// 阻止默认行为与事件冒泡，避免页面滚动或冲突
		e.preventDefault();
		e.stopPropagation();
	}
	/**
	 * 处理鼠标移动事件，更新图片位置
	 * @param {MouseEvent} e - 鼠标事件对象
	 * @returns {void}
	 */
	handleMouseMove(e) {
		// 仅在拖拽状态下且图片模式下触发
		if (!this.isDragging || this.mediaType !== 'image') return;
		/** 计算鼠标移动距离 */
		const deltaX = e.clientX - this.startX;
		/** 计算鼠标移动距离 */
		const deltaY = e.clientY - this.startY;
		/** 计算新的图片位置 */
		const newPositionX = this.currentX + deltaX;
		/** 计算新的图片位置 */
		const newPositionY = this.currentY + deltaY;
		// 更新图片位置
		this.img.style.transform = `translate3d(${newPositionX}px, ${newPositionY}px, 0) scale(${this.currentScale})`;
	}
	/**
	 * 处理鼠标抬起事件，结束拖拽
	 * @returns {void}
	 */
	handleMouseUp() {
		// 仅在拖拽状态下且图片模式下触发
		if (!this.isDragging || this.mediaType !== 'image') return;
		// 标记为不在拖拽状态
		this.isDragging = false;
		// 设置图片容器样式
		this.imgContainer.style.cursor = 'grab';
		/** 获取图片当前的变换矩阵 */
		const computedStyle = window.getComputedStyle(this.img);
		/** 解析图片当前的变换矩阵 */
		const matrix = new DOMMatrix(computedStyle.transform);
		// 提取图片的新位置
		this.currentX = matrix.m41;
		this.currentY = matrix.m42;
		// 更新图片的 data-x 和 data-y 属性
		this.img.setAttribute('data-x', this.currentX);
		this.img.setAttribute('data-y', this.currentY);
		// 恢复图片的过渡动画
		this.img.style.willChange = 'auto';
		// 移除鼠标移动与抬起事件监听器
		document.removeEventListener('mousemove', this.handleMouseMove);
		document.removeEventListener('mouseup', this.handleMouseUp);
	}
	/**
	 * 缩放图片（仅对图片有效）
	 * @param {number} scaleFactor - 缩放因子
	 * @param {number|null} [centerX=null] - 缩放中心点X坐标
	 * @param {number|null} [centerY=null] - 缩放中心点Y坐标
	 * @returns {void}
	 */
	zoom(scaleFactor, centerX = null, centerY = null) {
		// 仅在图片模式下触发
		if (!this.img || this.mediaType !== 'image') return;
		/** 旧的缩放比例 */
		const oldScale = this.currentScale;
		/** 新的缩放比例 */
		const newScale = this.currentScale * scaleFactor;
		// 限制缩放范围
		if (newScale < 0.1 || newScale > 10) return;
		this.currentScale = newScale;
		this.img.setAttribute('data-scale', this.currentScale);
		// 如果没有指定中心点，使用图片中心
		if (centerX === null || centerY === null) {
			/** 获取图片当前的边界矩形 */
			const imageRect = this.img.getBoundingClientRect();
			centerX = imageRect.left + imageRect.width / 2;
			centerY = imageRect.top + imageRect.height / 2;
		}
		/** 获取图片当前的边界矩形 */
		const imageRect = this.img.getBoundingClientRect();
		/** 图片中心X坐标 */
		const imgCenterX = imageRect.left + imageRect.width / 2;
		/** 图片中心Y坐标 */
		const imgCenterY = imageRect.top + imageRect.height / 2;
		/** 鼠标偏移量X */
		const mouseOffsetX = centerX - imgCenterX;
		/** 鼠标偏移量Y */
		const mouseOffsetY = centerY - imgCenterY;
		/** 缩放比例 */
		const scaleRatio = newScale / oldScale;
		// 计算新的图片位置
		this.currentX = this.currentX + mouseOffsetX * (1 - scaleRatio);
		this.currentY = this.currentY + mouseOffsetY * (1 - scaleRatio);
		// 应用变换
		this.img.style.transition = 'transform 0.15s ease';
		this.img.style.transform = `translate3d(${this.currentX}px, ${this.currentY}px, 0) scale(${this.currentScale})`;
		this.img.setAttribute('data-x', this.currentX);
		this.img.setAttribute('data-y', this.currentY);
		// 更新显示
		this.updateScaleInfo();
		// 清理过渡效果
		setTimeout(() => this.img.style.transition = 'none', 150);
	}
	/**
	 * 重置图片状态（缩放和位置）
	 * @returns {void}
	 */
	reset() {
		// 仅在图片模式下触发
		if (this.mediaType !== 'image') return;
		// 重置缩放比例
		this.currentScale = 1;
		// 重置图片位置X
		this.currentX = 0;
		// 重置图片位置Y
		this.currentY = 0;
		// 应用重置变换
		this.img.style.transition = 'transform 0.3s ease';
		this.img.style.transform = 'translate3d(0px, 0px, 0) scale(1)';
		this.img.setAttribute('data-scale', '1');
		this.img.setAttribute('data-x', '0');
		this.img.setAttribute('data-y', '0');
		// 更新缩放百分比显示
		this.updateScaleInfo();
		// 延迟移除过渡效果，确保动画完成
		setTimeout(() => this.img.style.transition = 'none', 300);
	}
	/**
	 * 更新缩放百分比显示
	 * @returns {void}
	 */
	updateScaleInfo() {
		// 仅在图片模式下更新缩放百分比显示
		if (this.zoomInfo && this.mediaType === 'image') {
			/** 缩放比例百分比 */
			const scalePercent = Math.round(this.currentScale * 100);
			// 更新缩放百分比显示
			this.zoomInfo.textContent = `${scalePercent}%`;
		}
	}
	/**
	 * 处理键盘按键事件
	 * @param {KeyboardEvent} e - 键盘事件对象
	 * @returns {void}
	 */
	handleKeyDown(e) {
		// 仅在预览模态框存在时触发
		if (!this.modal || !this.modal.parentNode) {
			document.removeEventListener('keydown', this.handleKeyDown);
			return;
		}
		// 禁用默认行为
		e.preventDefault();
		// 视频/音频模式下，空格键控制播放/暂停
		if ((this.mediaType === 'video' || this.mediaType === 'audio') && this.video) switch (e.key) {
			// 播放/暂停视频/音频
			case ' ':
				if (this.video.paused) this.video.play();
				else this.video.pause();
				break;
			// 关闭预览
			case 'Escape': this.close(); break;

			default: break;
		}
		// 通用快捷键
		if (this.mediaType === 'image' && this.img) switch (e.key) {
			// 关闭预览
			case 'Escape': this.close(); break;
			// 放大图片
			case 'ArrowUp': this.zoom(1.2); break;
			// 缩小图片
			case 'ArrowDown': this.zoom(0.8); break;
			// 重置图片状态
			case 'Shift': this.reset(); break;
		}
	}
	/**
	 * 关闭预览
	 * @returns {void}
	 */
	close() {
		if (this.modal && this.modal.parentNode) {
			// 停止视频/音频播放
			if (this.video && (this.mediaType === 'video' || this.mediaType === 'audio')) {
				this.video.pause();
				this.video.currentTime = 0;
			}
			// 移除事件监听器
			document.removeEventListener('keydown', this.handleKeyDown);
			document.removeEventListener('mousemove', this.handleMouseMove);
			document.removeEventListener('mouseup', this.handleMouseUp);
			// 移除模态框
			this.modal.remove();
			// 清理引用
			this.modal = null;
			this.img = null;
			this.video = null;
			this.imgContainer = null;
			this.zoomInfo = null;
		}
	}
}

/**
 * 全局媒体预览实例
 * @type {MultimediaPreview}
 */
const multimediaPreview = new MultimediaPreview();

/**
 * 全局预览函数（保持向后兼容）
 * @async
 * @param {string} path - 文件路径
 * @param {string} fileName - 文件名
 * @returns {Promise<void>}
 */
async function previewImage(path, fileName) {
	await multimediaPreview.preview(path, fileName);
}