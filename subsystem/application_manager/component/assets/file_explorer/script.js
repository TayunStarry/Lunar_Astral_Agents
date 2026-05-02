class FileManager {
    constructor() {
        this.currentPath = '';
        this.selectedFiles = new Set();
        this.files = [];
        this.isSearching = false;
        this.searchResults = [];
        this.allFiles = [];
        this.currentMediaIndex = 0;
        this.currentMediaList = [];
        this.currentPage = 1;       // 当前页码
        this.pageSize = 10;         // 每页显示数量
        this.init();
    }

    init() {
        this.bindEvents();
        this.loadFiles();
    }

    bindEvents() {
        document.getElementById('file-upload').addEventListener('change', (e) => this.handleFileUpload(e));
        document.getElementById('zip-upload').addEventListener('change', (e) => this.handleZipUpload(e));
        document.getElementById('new-folder').addEventListener('click', () => this.createNewFolder());
        document.getElementById('batch-delete').addEventListener('click', () => this.batchDelete());
        document.getElementById('batch-compress').addEventListener('click', () => this.batchCompress());
        document.getElementById('back-button').addEventListener('click', () => this.goBack());
        document.getElementById('qrcode-button').addEventListener('click', () => this.showQRCode());
        document.getElementById('text-modal').addEventListener('click', (e) => this.closeTextModal(e));
        document.getElementById('qrcode-modal').addEventListener('click', (e) => this.closeQRCodeModal(e));
        document.addEventListener('keydown', (e) => this.handleKeyboard(e));
        document.getElementById('search-input').addEventListener('input', (e) => this.handleSearch(e));
        document.getElementById('search-clear').addEventListener('click', () => this.clearSearch());
    }

    async loadFiles() {
        try {
            const response = await fetch(`/file_list/${this.currentPath}`);
            if (!response.ok) return;
            this.files = await response.json();
            if (this.isSearching) return;
            this.currentPage = 1;        // 重置为第一页
            this.updateFileGrid();
            this.updateStats();
            this.updateBreadcrumb();
        } catch (error) {
            this.showToast('加载文件失败', 'error');
        }
    }

    updateFileGrid() {
        const fileGrid = document.getElementById('file-grid');
        fileGrid.innerHTML = '';

        const displayFiles = this.isSearching ? this.searchResults : this.files;
        const sortedFiles = [...displayFiles].sort((a, b) => {
            if (a.isDir && !b.isDir) return -1;
            if (!a.isDir && b.isDir) return 1;
            return a.name.localeCompare(b.name);
        });

        // 媒体预览列表基于全部结果
        this.currentMediaList = displayFiles.filter(file => !file.isDir && (this.isImageFile(file.name) || this.isVideoFile(file.name) || this.isAudioFile(file.name)));

        // 分页计算
        const totalPages = Math.ceil(sortedFiles.length / this.pageSize);
        if (this.currentPage > totalPages) this.currentPage = totalPages || 1;
        const start = (this.currentPage - 1) * this.pageSize;
        const pageFiles = sortedFiles.slice(start, start + this.pageSize);

        // 渲染当前页
        for (const file of pageFiles) {
            const fileCard = this.createFileCard(file);
            fileGrid.appendChild(fileCard);
        }

        this.renderPagination(totalPages);

        if (this.isSearching) this.updateSearchStats();
        else this.updateStats();
    }

    renderPagination(totalPages) {
        const pagination = document.getElementById('pagination');
        if (totalPages <= 1) {
            pagination.innerHTML = '';
            return;
        }

        let html = '';
        html += `<button class="btn btn-small btn-pagination" data-page="prev" ${this.currentPage === 1 ? 'disabled' : ''}>‹ 上一页</button>`;

        const maxPagesToShow = 7;
        let startPage, endPage;
        if (totalPages <= maxPagesToShow) {
            startPage = 1;
            endPage = totalPages;
        } else {
            if (this.currentPage <= 4) {
                startPage = 1;
                endPage = maxPagesToShow;
            } else if (this.currentPage + 3 >= totalPages) {
                startPage = totalPages - maxPagesToShow + 1;
                endPage = totalPages;
            } else {
                startPage = this.currentPage - 3;
                endPage = this.currentPage + 3;
            }
        }

        for (let i = startPage; i <= endPage; i++) {
            html += `<button class="btn btn-small btn-pagination ${i === this.currentPage ? 'btn-primary' : ''}" data-page="${i}">${i}</button>`;
        }

        html += `<button class="btn btn-small btn-pagination" data-page="next" ${this.currentPage === totalPages ? 'disabled' : ''}>下一页 ›</button>`;

        pagination.innerHTML = html;

        pagination.querySelectorAll('.btn-pagination').forEach(btn => {
            btn.addEventListener('click', () => {
                const page = btn.dataset.page;
                if (page === 'prev') this.currentPage--;
                else if (page === 'next') this.currentPage++;
                else this.currentPage = parseInt(page);
                this.updateFileGrid();
            });
        });
    }

    updateSearchStats() {
        const totalFilesElement = document.getElementById('total-files');
        const totalFoldersElement = document.getElementById('total-folders');
        const totalSizeElement = document.getElementById('total-size');
        const folders = this.searchResults.filter(f => f.isDir).length;
        const files = this.searchResults.filter(f => !f.isDir).length;
        const totalSize = this.searchResults.reduce((sum, file) => sum + file.size, 0);
        totalFilesElement.textContent = files;
        totalFoldersElement.textContent = folders;
        totalSizeElement.textContent = this.formatFileSize(totalSize);
    }

    handleSearch(e) {
        const query = e.target.value.trim();
        const searchClear = document.getElementById('search-clear');
        if (!query) return this.clearSearch();
        searchClear.style.display = 'block';
        this.searchFiles(query);
    }

    clearSearch() {
        const searchInput = document.getElementById('search-input');
        const searchClear = document.getElementById('search-clear');
        searchInput.value = '';
        searchClear.style.display = 'none';
        this.isSearching = false;
        this.searchResults = [];
        this.currentPage = 1;
        this.updateFileGrid();
        this.updateBreadcrumb();
    }

    async searchFiles(query) {
        if (!query) return this.clearSearch();
        this.isSearching = true;
        this.showToast('正在搜索...', 'info');
        try {
            if (this.allFiles.length === 0) await this.traverseAllFiles();
            this.searchResults = this.allFiles.filter(file => file.name.toLowerCase().includes(query.toLowerCase()));
            this.currentPage = 1;
            this.updateFileGrid();
            this.updateBreadcrumb(true);
            this.showToast(`找到 ${this.searchResults.length} 个结果`, 'success');
        } catch (error) {
            this.showToast('搜索失败', 'error');
            console.error('搜索失败:', error);
        }
    }

    async traverseAllFiles() {
        this.allFiles = [];
        const queue = [''];
        while (queue.length > 0) {
            const currentPath = queue.shift();
            try {
                const response = await fetch(`/file_list/${currentPath}`);
                if (!response.ok) continue;
                const files = await response.json();
                this.allFiles.push(...files);
                const subDirs = files.filter(file => file.isDir);
                for (const dir of subDirs) {
                    queue.push(dir.path);
                }
            } catch (error) {
                console.error('遍历文件失败:', error);
            }
        }
    }

    createFileCard(file) {
        const card = document.createElement('div');
        card.className = 'file-card';
        card.dataset.path = file.path;

        const checkbox = document.createElement('input');
        checkbox.type = 'checkbox';
        checkbox.className = 'file-checkbox';
        checkbox.checked = false;
        checkbox.addEventListener('change', (e) => {
            e.stopPropagation();
            this.toggleFileSelection(file, checkbox.checked);
        });
        card.appendChild(checkbox);

        if (file.isDir) {
            const icon = document.createElement('div');
            icon.className = 'file-icon';
            icon.innerHTML = '<i class="fas fa-folder"></i>';
            card.appendChild(icon);
        } else if (this.isImageFile(file.name)) {
            const img = document.createElement('img');
            img.className = 'file-thumbnail';
            img.src = `/read/${file.path}`;
            img.alt = file.name;
            card.appendChild(img);
        } else {
            const icon = document.createElement('div');
            icon.className = 'file-icon';
            icon.innerHTML = this.getFileIcon(file.name);
            card.appendChild(icon);
        }

        const name = document.createElement('div');
        name.className = 'file-name';
        name.textContent = file.name;
        card.appendChild(name);

        const meta = document.createElement('div');
        meta.className = 'file-meta';
        meta.innerHTML = `
            <div>大小: ${this.formatFileSize(file.size)}</div>
            <div>修改: ${this.formatDate(file.lastModified)}</div>
        `;
        card.appendChild(meta);

        const actions = document.createElement('div');
        actions.className = 'file-actions';

        const renameBtn = document.createElement('button');
        renameBtn.className = 'btn btn-small btn-info';
        renameBtn.innerHTML = '<i class="fas fa-edit"></i>';
        renameBtn.title = '重命名';
        renameBtn.addEventListener('click', (e) => {
            e.stopPropagation();
            this.renameFile(file);
        });
        actions.appendChild(renameBtn);

        if (!file.isDir) {
            const downloadBtn = document.createElement('button');
            downloadBtn.className = 'btn btn-small btn-secondary';
            downloadBtn.innerHTML = '<i class="fas fa-download"></i>';
            downloadBtn.title = '下载';
            downloadBtn.addEventListener('click', (e) => {
                e.stopPropagation();
                this.downloadFile(file);
            });
            actions.appendChild(downloadBtn);
        }

        const deleteBtn = document.createElement('button');
        deleteBtn.className = 'btn btn-small btn-danger';
        deleteBtn.innerHTML = '<i class="fas fa-trash"></i>';
        deleteBtn.title = '删除';
        deleteBtn.addEventListener('click', (e) => {
            e.stopPropagation();
            this.deleteFile(file);
        });
        actions.appendChild(deleteBtn);

        card.appendChild(actions);

        card.addEventListener('click', (e) => {
            if (!e.target.closest('.file-checkbox')) this.handleFileClick(file);
        });

        return card;
    }

    handleFileClick(file) {
        if (file.isDir) this.navigateToDirectory(file);
        else {
            if (this.isImageFile(file.name) || this.isVideoFile(file.name) || this.isAudioFile(file.name)) {
                const mediaIndex = this.currentMediaList.findIndex(media => media.path === file.path);
                this.currentMediaIndex = mediaIndex;
                previewImage(`/read/${file.path}`, file.name);
            } else if (this.isTextFile(file.name)) {
                this.showTextModal(file);
            }
        }
    }

    navigateToDirectory(directory) {
        this.currentPath = directory.path;
        this.selectedFiles.clear();
        this.updateBatchActions();
        this.currentPage = 1;
        this.loadFiles();
    }

    goBack() {
        if (!this.currentPath) return;
        const pathParts = this.currentPath.split('\\');
        pathParts.pop();
        this.currentPath = pathParts.join('\\');
        this.selectedFiles.clear();
        this.updateBatchActions();
        this.currentPage = 1;
        this.loadFiles();
    }

    updateBreadcrumb(isSearching = false) {
        const backButton = document.getElementById('back-button');
        backButton.style.display = this.currentPath ? 'inline-block' : 'none';
        const breadcrumb = document.querySelector('.breadcrumb');
        breadcrumb.innerHTML = '';
        breadcrumb.appendChild(backButton);

        const rootItem = document.createElement('a');
        rootItem.className = 'breadcrumb-item';
        rootItem.href = '#';
        rootItem.dataset.path = '';
        rootItem.innerHTML = '<i class="fas fa-home"></i> 根目录';
        rootItem.addEventListener('click', (e) => {
            e.preventDefault();
            this.clearSearch();
            this.currentPath = '';
            this.selectedFiles.clear();
            this.updateBatchActions();
            this.currentPage = 1;
            this.loadFiles();
        });
        breadcrumb.appendChild(rootItem);

        if (isSearching) {
            const searchItem = document.createElement('span');
            searchItem.className = 'breadcrumb-item';
            searchItem.innerHTML = '<i class="fas fa-search"></i> 搜索结果';
            searchItem.style.color = '#3498db';
            searchItem.style.fontWeight = '600';
            breadcrumb.appendChild(searchItem);
        } else if (this.currentPath) {
            const pathParts = this.currentPath.split(/[\\/]/);
            let currentPath = '';
            pathParts.forEach(part => {
                if (!part) return;
                currentPath += (currentPath ? '\\' : '') + part;
                const breadcrumbItem = document.createElement('a');
                breadcrumbItem.className = 'breadcrumb-item';
                breadcrumbItem.href = '#';
                breadcrumbItem.dataset.path = currentPath;
                breadcrumbItem.textContent = part;
                breadcrumbItem.addEventListener('click', (e) => {
                    e.preventDefault();
                    this.currentPath = breadcrumbItem.dataset.path;
                    this.selectedFiles.clear();
                    this.updateBatchActions();
                    this.currentPage = 1;
                    this.loadFiles();
                });
                breadcrumb.appendChild(breadcrumbItem);
            });
        }
    }


	/**
	 * 处理文件上传
	 *
	 * @param {Event} event - 文件上传事件对象
	 */
	async handleFileUpload(event) {
		/** 获取上传文件 */
		const files = event.target.files;
		// 如果没有选择文件，直接返回
		if (files.length === 0) return;
		/** 获取上传进度条 */
		const uploadProgress = document.getElementById('upload-progress');
		/** 获取上传进度条填充元素 */
		const progressFill = document.getElementById('progress-fill');
		/** 获取上传进度条文本元素 */
		const progressText = document.getElementById('progress-text');
		// 显示上传进度条
		uploadProgress.style.display = 'block';
		/** 已上传文件计数器 */
		let uploadedFiles = 0;
		// 遍历每个文件
		for (const file of files) {
			try {
				// 对单个文件进行逐一上传
				await this.uploadFile(file,
					progress => {
						/** 计算当前文件的上传进度 */
						const totalProgress = ((uploadedFiles + progress) / files.length) * 100;
						/** 更新上传进度条填充宽度 */
						progressFill.style.width = `${totalProgress}%`;
						/** 更新上传进度条文本内容 */
						progressText.textContent = `${Math.round(totalProgress)}%`;
					}
				);
				uploadedFiles++;
			}
			catch (error) {
				this.showToast(`上传失败: ${file.name}`, 'error');
				console.error('上传失败:', error);
			}
		}
		// 上传完成后，隐藏上传进度条
		uploadProgress.style.display = 'none';
		// 刷新文件列表
		this.loadFiles();
		this.showToast(`成功上传 ${uploadedFiles} 个文件`, 'success');
		// 上传完成后，重置文件输入字段
		event.target.value = '';
	}
	/**
	 * 上传单个文件
	 *
	 * @param {File} file - 要上传的文件对象
	 * @param {Function} onProgress - 上传进度回调函数
	 * @param {boolean} overwrite - 是否覆盖已存在文件
	 * @returns {Promise} - 上传完成后的 Promise 对象
	 */
	async uploadFile(file, onProgress, overwrite = true) {
		/** 创建 FormData 对象 */
		const formData = new FormData();
		// 将文件添加到 FormData 对象
		formData.append('file', file);
		/** 构造完整路径 */
		const fullPath = this.currentPath ? `${this.currentPath}/${file.name}` : file.name;
		/** 上传文件事件处理函数 */
		const uploadFileEvent = async (resolve, reject) => {
			/** 创建 XMLHttpRequest 对象 */
			const xhr = new XMLHttpRequest();
			// 为上传进度事件添加监听器
			xhr.upload.addEventListener('progress',
				e => {
					// 如果事件对象不可计算长度，直接返回
					if (!e.lengthComputable) return;
					/** 计算当前文件的上传进度 */
					const progress = e.loaded / e.total;
					// 调用进度回调函数，更新上传进度
					onProgress(progress);
				}
			);
			// 为加载完成事件添加监听器
			xhr.addEventListener('load',
				() => {
					if (xhr.status === 200) resolve(JSON.parse(xhr.responseText));
					else reject(new Error(xhr.responseText || '上传失败'));
				}
			);
			// 为错误事件添加监听器
			xhr.addEventListener('error', () => reject(new Error('上传失败')));
			// 为超时事件添加监听器
			xhr.addEventListener('timeout', () => reject(new Error('上传超时')));
			// 初始化 POST 请求，目标接口为 /save
			xhr.open('POST', '/save');
			// 设置请求头，携带经过编码处理的文件名，防止中文或特殊字符乱码
			xhr.setRequestHeader('X-File-Name', this.encodeFileName(fullPath));
			// 设置是否覆盖同名文件的标志位，服务端根据此值决定是否覆盖
			xhr.setRequestHeader('X-Overwrite', overwrite.toString());
			// 将文件数据作为请求体发送，开始实际上传
			xhr.send(file);
		}
		// 返回 Promise 对象，等待上传完成
		return new Promise(uploadFileEvent);
	}
	/**
	 * 创建新文件夹
	 *
	 * @returns {Promise} - 创建文件夹后的 Promise 对象
	 */
	async createNewFolder() {
		/** 提示用户输入文件夹名称 */
		const folderName = prompt('请输入文件夹名称:');
		// 如果用户取消输入或输入为空，直接返回
		if (!folderName) return;
		// 检查文件夹名是否合法
		if (!this.isValidFileName(folderName)) {
			this.showToast('文件夹名称不合法', 'error');
			return;
		}
		try {
			/** 构造临时文件名，用于创建文件夹 */
			const tempFileName = `${folderName}/.temp`;
			/** 构造完整路径，包含当前路径和临时文件名 */
			const fullPath = this.currentPath ? `${this.currentPath}/${tempFileName}` : tempFileName;
			/** 创建一个空的 Blob 对象，用于表示空文件夹 */
			const blob = new Blob([''], { type: 'text/plain' });
			/** 上传空文件，实际上创建了一个空文件夹 */
			await this.uploadFile(new File([blob], tempFileName, { type: 'text/plain' }), () => { }, true);
			// 删除临时文件
			await fetch(`/delete/${fullPath}`, { method: 'DELETE' });
			// 刷新文件列表，确保新文件夹出现在列表中
			this.loadFiles();
			this.showToast(`文件夹 "${folderName}" 创建成功`, 'success');
		}
		catch (error) {
			this.showToast('创建文件夹失败', 'error');
			console.error('创建文件夹失败:', error);
		}
	}
	/**
	 * 删除文件或目录
	 *
	 * @param {Object} file - 要删除的文件或目录对象
	 * @returns {Promise} - 删除操作后的 Promise 对象
	 */
	async deleteFile(file) {
		// 确认用户是否要删除文件或目录
		if (!confirm(`确定要删除 ${file.isDir ? '目录' : '文件'} "${file.name}" 吗？`)) {
			return;
		}
		try {
			/** 发送删除请求，删除文件或目录 */
			const response = await fetch(`/delete/${file.path}`, { method: 'DELETE' });
			// 检查响应状态是否成功
			if (!response.ok) throw new Error('删除失败');
			// 刷新文件列表，确保删除后的文件列表更新
			this.loadFiles();
			this.showToast(`删除成功`, 'success');
		}
		catch (error) {
			this.showToast('删除失败', 'error');
			console.error('删除失败:', error);
		}
	}
	/**
	 * 重命名文件或目录
	 *
	 * @param {Object} file - 要重命名的文件或目录对象
	 * @returns {Promise} - 重命名操作后的 Promise 对象
	 */
	async renameFile(file) {
		/** 提示用户输入新的文件名或目录名 */
		const newName = prompt(`请输入新的${file.isDir ? '目录' : '文件'}名称:`, file.name);
		// 如果用户取消输入或输入为空，直接返回
		if (!newName || newName === file.name) return;
		// 检查名称是否合法
		if (!this.isValidFileName(newName)) {
			this.showToast('名称不合法', 'error');
			return;
		}
		try {
			// 根据文件类型调用不同的重命名方法
			if (file.isDir) await this.renameDirectory(file, newName);
			// 如果是文件，调用重命名文件方法
			else await this.renameSingleFile(file, newName);
			// 刷新文件列表，确保重命名后的文件列表更新
			this.loadFiles();
			this.showToast(`重命名成功`, 'success');
		}
		catch (error) {
			this.showToast(`重命名失败: ${error.message}`, 'error');
			console.error('重命名失败:', error);
		}
	}
	/**
	 * 重命名单个文件
	 *
	 * @param {Object} file - 要重命名的文件对象
	 * @param {string} newName - 新的文件名
	 * @returns {Promise} - 重命名操作后的 Promise 对象
	 */
	async renameSingleFile(file, newName) {
		/** 读取文件内容 */
		const response = await fetch(`/read/${file.path}`);
		// 检查响应状态是否成功
		if (!response.ok) throw new Error('读取文件失败');
		/** 解析响应体为 Blob 对象 */
		const content = await response.blob();
		// 删除旧文件
		await fetch(`/delete/${file.path}`, { method: 'DELETE' });
		/** 创建新文件 */
		const uploadFile = new File([content], newName, { type: content.type });
		// 上传新文件
		await this.uploadFile(uploadFile, () => { }, true);
	}
	/**
	 * 重命名目录
	 *
	 * @param {Object} directory - 要重命名的目录对象
	 * @param {string} newName - 新的目录名
	 * @returns {Promise} - 重命名操作后的 Promise 对象
	 */
	async renameDirectory(directory, newName) {
		// 创建新目录
		await this.createDirectory(newName);
		// 复制所有内容到新目录
		await this.copyDirectoryContent(directory.path, newName);
		// 删除旧目录
		await fetch(`/delete/${directory.path}`, { method: 'DELETE' });
	}
	/**
	 * 获取目录内容
	 *
	 * @param {string} dirPath - 目录路径
	 * @returns {Promise} - 目录内容的 Promise 对象
	 */
	async getDirectoryContent(dirPath) {
		/** 发送请求获取目录内容 */
		const response = await fetch(`/file_list/${dirPath}`);
		// 检查响应状态是否成功
		if (!response.ok) throw new Error('读取目录失败');
		// 解析响应体为 JSON 对象
		return await response.json();
	}
	/**
	 * 创建目录
	 *
	 * @param {string} dirName - 目录名
	 * @returns {Promise} - 创建目录后的 Promise 对象
	 */
	async createDirectory(dirName) {
		/** 创建临时文件 */
		const tempFileName = `${dirName}/.temp`;
		/** 构建完整路径 */
		const fullPath = this.currentPath ? `${this.currentPath}/${tempFileName}` : tempFileName;
		/** 创建空文件 */
		const blob = new Blob([''], { type: 'text/plain' });
		// 上传空文件作为占位符
		await this.uploadFile(new File([blob], tempFileName, { type: 'text/plain' }), () => { }, true);
		// 删除临时文件
		await fetch(`/delete/${fullPath}`, { method: 'DELETE' });
	}
	/**
	 * 复制目录内容
	 *
	 * @param {string} sourceDirPath - 源目录路径
	 * @param {string} targetDirName - 目标目录名
	 * @returns {Promise} - 复制目录内容后的 Promise 对象
	 */
	async copyDirectoryContent(sourceDirPath, targetDirName) {
		/** 获取源目录内容 */
		const filesInDir = await this.getDirectoryContent(sourceDirPath);
		// 遍历目录中的每个文件或子目录
		for (const file of filesInDir) {
			/** 提取文件名 */
			const fileName = file.path.split('\\').pop();
			/** 构建目标路径 */
			const targetPath = `${targetDirName}/${fileName}`;
			// 判断是否是目录
			if (file.isDir) {
				// 递归复制子目录
				await this.createDirectory(targetPath);
				await this.copyDirectoryContent(file.path, targetPath);
			}
			// 复制单个文件
			else await this.copySingleFile(file, targetPath);
		}
	}
	/**
	 * 复制单个文件
	 *
	 * @param {Object} file - 要复制的文件对象
	 * @param {string} targetPath - 目标文件路径
	 * @returns {Promise} - 复制文件后的 Promise 对象
	 */
	async copySingleFile(file, targetPath) {
		/** 读取文件内容 */
		const fileResponse = await fetch(`/read/${file.path}`);
		// 检查响应状态是否成功
		if (!fileResponse.ok) return;
		/** 解析响应体为 Blob 对象 */
		const fileBlob = await fileResponse.blob();
		/** 创建新文件 */
		const uploadFile = new File([fileBlob], targetPath, { type: fileBlob.type });
		// 上传新文件
		await this.uploadFile(uploadFile, () => { }, false);
	}
	/**
	 * 批量删除
	 *
	 * @returns {Promise} - 批量删除后的 Promise 对象
	 */
	async batchDelete() {
		// 检查是否有选中的文件或目录
		if (this.selectedFiles.size === 0) return;
		// 确认删除
		if (!confirm(`确定要删除选中的 ${this.selectedFiles.size} 个项目吗？`)) {
			return;
		}
		try {
			/** 记录已删除的项目数量 */
			let deletedCount = 0;
			// 遍历选中的文件或目录路径
			for (const filePath of this.selectedFiles) {
				/** 发送删除请求 */
				const response = await fetch(`/delete/${filePath}`, { method: 'DELETE' });
				// 检查响应状态是否成功
				if (response.ok) deletedCount++;
			}
			// 清除选中的文件或目录
			this.selectedFiles.clear();
			// 更新批量操作按钮状态
			this.updateBatchActions();
			// 刷新文件列表
			this.loadFiles();
			this.showToast(`成功删除 ${deletedCount} 个项目`, 'success');
		}
		catch (error) {
			this.showToast('批量删除失败', 'error');
			console.error('批量删除失败:', error);
		}
	}
	/**
	 * 批量压缩
	 *
	 * @returns {Promise} - 批量压缩后的 Promise 对象
	 */
	async batchCompress() {
		// 检查是否有选中的文件或目录
		if (this.selectedFiles.size === 0) {
			this.showToast('请先选择要压缩的文件', 'info');
			return;
		}
		try {
			/** 获取选中的文件对象 */
			const selectedFileObjects = this.files.filter(file => this.selectedFiles.has(file.path));
			/** 构建表单数据 */
			const formData = new FormData();
			// 下载每个文件并添加到表单中
			for (const fileObj of selectedFileObjects) {
				// 跳过目录
				if (fileObj.isDir) continue;
				/** 读取文件内容 */
				const response = await fetch(`/read/${fileObj.path}`);
				/** 解析响应体为 Blob 对象 */
				const blob = await response.blob();
				/** 创建 File 对象 */
				const file = new File([blob], fileObj.name, { type: blob.type });
				// 添加到表单数据
				formData.append('files', file);
			}
			/** 设置ZIP文件名 */
			const zipName = `压缩文件_${new Date().getTime()}.zip`;
			// 添加ZIP文件名到表单数据
			formData.append('zip_name', zipName);
			/** 发送压缩请求 */
			const response = await fetch('/archive',
				{
					method: 'POST',
					body: formData
				}
			);
			// 检查响应状态是否成功
			if (!response.ok) throw new Error('压缩失败');
			/** 解析响应体为 Blob 对象 */
			const blob = await response.blob();
			/** 创建下载链接 */
			const url = URL.createObjectURL(blob);
			/** 创建下载链接元素 */
			const a = document.createElement('a');
			// 设置下载链接
			a.href = url;
			a.download = zipName;
			document.body.appendChild(a);
			a.click();
			// 移除下载链接元素
			document.body.removeChild(a);
			// 释放下载链接对象
			URL.revokeObjectURL(url);
			// 显示成功提示
			this.showToast(`成功压缩 ${this.selectedFiles.size} 个项目`, 'success');
		}
		catch (error) {
			this.showToast('压缩失败', 'error');
			console.error('压缩失败:', error);
		}
	}
	/**
	 * 处理ZIP文件上传解压
	 *
	 * @param {Event} event - 文件上传事件对象
	 * @returns {Promise} - 处理ZIP文件上传解压后的 Promise 对象
	 */
	async handleZipUpload(event) {
		/** 获取上传的文件对象 */
		const file = event.target.files[0];
		// 检查文件是否存在
		if (!file) return;
		try {
			/** 构建表单数据 */
			const formData = new FormData();
			// 添加ZIP文件到表单数据
			formData.append('zip_file', file);
			/** 发送解压请求 */
			const response = await fetch('/archive',
				{
					method: 'PUT',
					body: formData
				}
			);
			// 检查响应状态是否成功
			if (!response.ok) throw new Error('解压失败');
			/** 解析响应体为 JSON 对象 */
			const result = await response.json();
			// 处理解压后的文件，逐个上传到当前目录
			for (const extractedFile of result.extracted_files) {
				// 跳过目录
				if (extractedFile.is_dir) continue;
				/** 将base64内容转换为Blob */
				const contentBytes = Uint8Array.from(atob(extractedFile.content), c => c.charCodeAt(0));
				/** 创建 Blob 对象 */
				const blob = new Blob([contentBytes]);
				/** 创建 File 对象 */
				const uploadFile = new File([blob], extractedFile.name, { type: this.getFileType(extractedFile.extension) });
				// 上传文件到当前目录
				await this.uploadFile(uploadFile, () => { }, false);
			}
			// 刷新文件列表
			this.loadFiles();
			this.showToast(`成功解压 ${result.total_files} 个文件`, 'success');
		}
		catch (error) {
			this.showToast('解压失败', 'error');
			console.error('解压失败:', error);
		}
		// 重置文件输入
		finally {
			event.target.value = '';
		}
	}
	/**
	 * 获取文件类型
	 *
	 * @param {string} extension - 文件扩展名
	 * @returns {string} - 文件类型的 MIME 字符串
	 */
	getFileType(extension) {
		/** 定义文件扩展名与 MIME 类型的映射关系 */
		const mimeTypes = {
			'.txt': 'text/plain',
			'.md': 'text/markdown',
			'.json': 'application/json',
			'.html': 'text/html',
			'.css': 'text/css',
			'.js': 'application/javascript',
			'.jpg': 'image/jpeg',
			'.jpeg': 'image/jpeg',
			'.png': 'image/png',
			'.gif': 'image/gif',
			'.svg': 'image/svg+xml',
			'.webp': 'image/webp',
			'.mp4': 'video/mp4',
			'.avi': 'video/x-msvideo',
			'.mov': 'video/quicktime',
			'.wmv': 'video/x-ms-wmv',
			'.flv': 'video/x-flv',
			'.webm': 'video/webm',
			'.pdf': 'application/pdf',
			'.zip': 'application/zip'
		};
		// 返回文件类型的 MIME 字符串，默认返回 'application/octet-stream'
		return mimeTypes[extension.toLowerCase()] || 'application/octet-stream';
	}
	/**
	 * 显示文本预览
	 *
	 * @param {Object} file - 要预览的文件对象
	 * @returns {Promise} - 显示文本预览后的 Promise 对象
	 */
	async showTextModal(file) {
		try {
			/** 发送读取文件请求 */
			const response = await fetch(`/read/${file.path}`);
			// 检查响应状态是否成功
			if (!response.ok) throw new Error('读取文件失败');
			/** 解析响应体为文本 */
			const content = await response.text();
			/** 获取文本预览模态框元素 */
			const modal = document.getElementById('text-modal');
			/** 获取文本预览模态框标题元素 */
			const modalTitle = document.getElementById('text-modal-title');
			/** 获取文本预览模态框文件信息元素 */
			const modalFileInfo = document.getElementById('text-modal-file-info');
			/** 获取文本预览模态框内容元素 */
			const modalContent = document.getElementById('text-modal-content');
			/** 获取文本预览模态框编辑器元素 */
			const modalEditor = document.getElementById('text-modal-editor');
			/** 获取文本预览模态框保存按钮元素 */
			const modalSave = document.getElementById('text-modal-save');
			/** 获取文本预览模态框编辑按钮元素 */
			const modalEdit = document.getElementById('text-modal-edit');
			/** 获取文本预览模态框复制按钮元素 */
			const modalCopy = document.getElementById('text-modal-copy');
			/** 获取文本预览模态框下载按钮元素 */
			const modalDownload = document.getElementById('text-modal-download');
			// 设置模态框标题为文件名
			modalTitle.textContent = file.name;
			/** 获取文件大小 */
			const fileSize = this.formatFileSize(file.size);
			/** 获取最后修改时间 */
			const lastModified = this.formatDate(file.lastModified);
			// 设置文件信息
			modalFileInfo.textContent = `${fileSize} · ${lastModified}`;
			// 设置编辑器内容为文件内容
			modalEditor.value = content;
			// 显示预览，隐藏编辑器和保存按钮
			modalContent.style.display = 'block';
			modalEditor.style.display = 'none';
			modalSave.style.display = 'none';
			// 尝试对代码文件应用语法高亮
			try {
				// 检查是否为文本文件
				if (this.isTextFile(file.name)) {
					/** 获取文件扩展名 */
					const ext = file.name.toLowerCase().slice(file.name.lastIndexOf('.') + 1);
					/** 对内容进行语法高亮 */
					const highlighted = hljs.highlight(content, { language: ext }).value;
					// 设置模态框内容为高亮后的代码
					modalContent.innerHTML = highlighted;
					// 添加代码高亮样式类
					modalContent.className = 'hljs';
				}
				else {
					// 如果不是文本文件，直接显示内容
					modalContent.textContent = content;
					modalContent.className = '';
				}
			}
			catch (error) {
				// 如果高亮失败，直接显示原始内容
				modalContent.textContent = content;
				modalContent.className = '';
			}
			// 显示模态框
			modal.classList.add('show');
			// 编辑按钮点击事件
			modalEdit.onclick = () => {
				modalContent.style.display = 'none';
				modalEditor.style.display = 'block';
				modalSave.style.display = 'inline-block';
			};
			// 复制按钮点击事件
			modalCopy.onclick = () => {
				navigator.clipboard.writeText(content)
					.then(() => this.showToast('已复制到剪贴板', 'success'))
					.catch(() => this.showToast('复制失败', 'error'));
			};
			// 下载按钮点击事件
			modalDownload.onclick = () => {
				// 创建下载链接
				const link = document.createElement('a');
				link.href = `/download/${file.path}`;
				link.download = file.name;
				document.body.appendChild(link);
				link.click();
				document.body.removeChild(link);
			};
			// 保存按钮点击事件
			modalSave.onclick = async () => {
				try {
					const newContent = modalEditor.value;
					await this.saveTextFile(file, newContent);
					// 保存成功后，更新预览内容
					modalContent.textContent = newContent;
					modalContent.style.display = 'block';
					modalEditor.style.display = 'none';
					modalSave.style.display = 'none';
					// 重新应用语法高亮
					try {
						if (this.isTextFile(file.name)) {
							const ext = file.name.toLowerCase().slice(file.name.lastIndexOf('.') + 1);
							const highlighted = hljs.highlight(newContent, { language: ext }).value;
							modalContent.innerHTML = highlighted;
							modalContent.className = 'hljs';
						}
					}
					catch (error) {
						modalContent.textContent = newContent;
						modalContent.className = '';
					}
					this.showToast('保存成功', 'success');
				}
				catch (error) {
					this.showToast('保存失败', 'error');
					console.error('保存失败:', error);
				}
			};
			// 存储当前文件对象，用于后续操作
			modal.dataset.filePath = file.path;
		}
		catch (error) {
			this.showToast('预览文件失败', 'error');
			console.error('预览文件失败:', error);
		}
	}
	/**
	 * 关闭文本预览
	 *
	 * @param {Event} event - 点击事件对象
	 */
	closeTextModal(event) {
		/** 获取文本预览模态框元素 */
		const modal = document.getElementById('text-modal');
		// 检查点击事件目标是否为模态框本身或关闭按钮
		if (event.target === modal || event.target.closest('.modal-close-btn') || event.target.classList.contains('close')) {
			modal.classList.remove('show');
		}
	}
	/**
	 * 保存文本文件
	 *
	 * @param {Object} file - 要保存的文件对象
	 * @param {string} content - 新的文件内容
	 * @returns {Promise} - 保存完成后的 Promise 对象
	 */
	async saveTextFile(file, content) {
		try {
			/** 创建一个包含新内容的 Blob 对象 */
			const blob = new Blob([content], { type: 'text/plain' });
			/** 创建一个新的 File 对象 */
			const uploadFile = new File([blob], file.name, { type: 'text/plain' });
			// 上传新文件，覆盖原文件
			await this.uploadFile(uploadFile, () => { }, true);
			// 刷新文件列表
			await this.loadFiles();
		}
		catch (error) {
			throw new Error('保存文件失败');
		}
	}
	/**
	 * 下载文件
	 *
	 * @param {Object} file - 要下载的文件对象
	 */
	async downloadFile(file) {
		try {
			/** 发送下载文件请求 */
			const response = await fetch(`/download/${file.path}`);
			// 检查响应状态是否成功
			if (!response.ok) throw new Error('下载失败');
			/** 解析响应体为 Blob 对象 */
			const blob = await response.blob();
			/** 创建对象 URL 用于下载 */
			const url = URL.createObjectURL(blob);
			/** 创建下载链接元素 */
			const a = document.createElement('a');
			// 设置下载链接属性
			a.href = url;
			a.download = file.name;
			document.body.appendChild(a);
			// 模拟点击下载链接
			a.click();
			// 移除下载链接元素
			document.body.removeChild(a);
			URL.revokeObjectURL(url);
		}
		catch (error) {
			this.showToast('下载失败', 'error');
			console.error('下载失败:', error);
		}
	}
	/**
	 * 切换文件选择
	 *
	 * @param {Object} file - 要切换选择的文件对象
	 * @param {boolean} isSelected - 是否选中文件
	 */
	toggleFileSelection(file, isSelected) {
		/** 选择所有文件卡片元素 */
		const cards = document.querySelectorAll('.file-card');
		/** 初始化文件卡片元素 */
		let card = null;
		// 遍历文件卡片元素，查找匹配的文件卡片
		for (const c of cards) {
			if (c.dataset.path === file.path) {
				card = c;
				break;
			}
		}
		// 更新选中状态集合
		if (isSelected) this.selectedFiles.add(file.path);
		// 取消选中文件时，从选中集合中删除文件路径
		else this.selectedFiles.delete(file.path);
		if (card) {
			// 更新卡片样式
			if (isSelected) card.classList.add('selected');
			else card.classList.remove('selected');
			/** 选择文件卡片中的复选框元素 */
			const checkbox = card.querySelector('.file-checkbox');
			if (checkbox) checkbox.checked = isSelected;
		}
		// 更新批量操作按钮
		this.updateBatchActions();
	}
	/**
	 * 更新批量操作按钮
	 */
	updateBatchActions() {
		/** 选择批量操作按钮元素 */
		const batchActions = document.querySelector('.batch-actions');
		// 检查是否有选中文件
		if (this.selectedFiles.size > 0) batchActions.classList.add('show');
		// 没有选中文件时，隐藏批量操作按钮
		else batchActions.classList.remove('show');
	}
	/**
	 * 更新统计信息
	 */
	updateStats() {
		/** 统计总文件数 */
		const totalFiles = this.files.filter(f => !f.isDir).length;
		/** 统计总文件夹数 */
		const totalFolders = this.files.filter(f => f.isDir).length;
		/** 统计总文件大小 */
		const totalSize = this.files.reduce((sum, file) => sum + file.size, 0);
		// 更新统计信息显示
		document.getElementById('total-files').textContent = totalFiles;
		document.getElementById('total-folders').textContent = totalFolders;
		document.getElementById('total-size').textContent = this.formatFileSize(totalSize);
	}
	/**
	 * 显示二维码
	 */
	async showQRCode() {
		try {
			/** 获取当前页面的 origin 部分 */
			const currentUrl = window.location.origin;
			/** 选择二维码容器元素 */
			const qrcodeContainer = document.getElementById('qrcode-container');
			/** 选择二维码URL元素 */
			const qrcodeUrl = document.getElementById('qrcode-url');
			/** 选择二维码模态框元素 */
			const qrcodeModal = document.getElementById('qrcode-modal');
			// 清空二维码容器
			qrcodeContainer.innerHTML = '';
			// 生成二维码
			new QRCode(qrcodeContainer,
				{
					text: currentUrl,
					width: 256,
					height: 256,
					colorDark: '#000000',
					colorLight: '#ffffff',
					correctLevel: QRCode.CorrectLevel.H
				}
			);
			// 显示URL
			qrcodeUrl.textContent = currentUrl;
			// 显示模态框
			qrcodeModal.classList.add('show');
		}
		catch (error) {
			this.showToast('生成二维码失败', 'error');
			console.error('生成二维码失败:', error);
		}
	}
	/**
	 * 关闭二维码模态框
	 *
	 * @param {Event} event - 点击事件对象
	 */
	closeQRCodeModal(event) {
		/** 选择二维码模态框元素 */
		const modal = document.getElementById('qrcode-modal');
		// 点击模态框或关闭按钮时，关闭模态框
		if (event.target === modal || event.target.closest('.modal-close-btn') || event.target.classList.contains('close')) {
			modal.classList.remove('show');
		}
	}
	/**
	 * 处理键盘事件
	 *
	 * @param {Event} event - 键盘事件对象
	 */
	handleKeyboard(event) {
		/** 选择图像预览容器元素 */
		const imagePreviewContainer = document.querySelector('.image-preview-container');
		// 检查图像预览容器是否存在
		if (imagePreviewContainer) {
			/** 选择图像信息元素 */
			const imageInfo = document.querySelector('.image-info');
			/** 选择图像预览元素 */
			const imagePreview = document.querySelector('.image-preview');
			/** 选择视频预览元素 */
			const videoPreview = document.querySelector('.video-preview');
			/** 选择图像拖动容器元素 */
			const imageDragContainer = document.querySelector('.image-drag-container');
			/** 选择当前媒体索引变量 */
			let prevFile = null;
			/** 快速加载媒体预览 */
			const quicklyLoadMediaPreview = () => {
				// 检查是否为图片文件
				if (this.isImageFile(prevFile.name)) {
					imageDragContainer.style.display = 'block';
					imagePreview.style.display = 'block';
					videoPreview.style.display = 'none';
					imagePreview.alt = prevFile.name;
					imagePreview.src = '/read/' + prevFile.path;
				}
				// 否则视为视频或音频文件
				else {
					imageDragContainer.style.display = 'none';
					imagePreview.style.display = 'none';
					videoPreview.style.display = 'block';
					videoPreview.alt = prevFile.name;
					videoPreview.src = '/read/' + prevFile.path;
				};
				// 隐藏多媒体信息
				imageInfo.style.display = 'none';
			};
			// 判断按下的按键类型
			switch (event.key) {
				case 'ArrowLeft':
					// 计算新的索引，确保循环遍历
					this.currentMediaIndex = (this.currentMediaIndex - 1 + this.currentMediaList.length) % this.currentMediaList.length;
					// 获取当前索引对应的媒体文件
					prevFile = this.currentMediaList[this.currentMediaIndex];
					// 快速加载媒体预览
					quicklyLoadMediaPreview();
					break;
				case 'ArrowRight':
					// 计算新的索引，确保循环遍历
					this.currentMediaIndex = (this.currentMediaIndex + 1) % this.currentMediaList.length;
					// 获取当前索引对应的媒体文件
					prevFile = this.currentMediaList[this.currentMediaIndex];
					// 快速加载媒体预览
					quicklyLoadMediaPreview();
					break;
			}
		}
		/** 选择文本模态框元素 */
		const textModal = document.getElementById('text-modal');
		// 检查文本模态框是否显示，且按下的键为 ESC 键
		if (textModal.classList.contains('show') && event.key === 'Escape') {
			textModal.classList.remove('show');
		}
		/** 选择二维码模态框元素 */
		const qrcodeModal = document.getElementById('qrcode-modal');
		// 检查二维码模态框是否显示，且按下的键为 ESC 键
		if (qrcodeModal.classList.contains('show') && event.key === 'Escape') {
			qrcodeModal.classList.remove('show');
		}
	}
	/**
	 * 检查文件是否为图片文件
	 *
	 * @param {string} filename - 文件名
	 * @returns {boolean} - 是否为图片文件
	 */
	isImageFile(filename) {
		/** 图片文件扩展名数组 */
		const imageExtensions = ['.jpg', '.jpeg', '.png', '.gif', '.bmp', '.svg', '.webp'];
		/** 文件名扩展名 */
		const extension = filename.toLowerCase().slice(filename.lastIndexOf('.'));
		// 检查扩展名是否在图片文件扩展名数组中
		return imageExtensions.includes(extension);
	}
	/**
	 * 检查文件是否为视频文件
	 *
	 * @param {string} filename - 文件名
	 * @returns {boolean} - 是否为视频文件
	 */
	isVideoFile(filename) {
		/** 视频文件扩展名数组 */
		const videoExtensions = ['.mp4', '.avi', '.mov', '.wmv', '.flv', '.webm', '.mkv'];
		/** 文件名扩展名 */
		const ext = filename.toLowerCase().slice(filename.lastIndexOf('.'));
		// 检查扩展名是否在视频文件扩展名数组中
		return videoExtensions.includes(ext);
	}
	/**
	 * 检查文件是否为音频文件
	 *
	 * @param {string} filename - 文件名
	 * @returns {boolean} - 是否为音频文件
	 */
	isAudioFile(filename) {
		/** 音频文件扩展名数组 */
		const audioExtensions = ['.mp3', '.wav', '.ogg', '.flac', '.aac', '.m4a', '.wma'];
		/** 文件名扩展名 */
		const ext = filename.toLowerCase().slice(filename.lastIndexOf('.'));
		// 检查扩展名是否在音频文件扩展名数组中
		return audioExtensions.includes(ext);
	}
	/**
	 * 检查文件是否为文本文件
	 *
	 * @param {string} filename - 文件名
	 * @returns {boolean} - 是否为文本文件
	 */
	isTextFile(filename) {
		/** 按用途分类的文本文件扩展名数组 */
		const plainText = ['.txt', '.md', '.log'];
		/** 按用途分类的 Web 相关文件扩展名数组 */
		const web = ['.html', '.css', '.js', '.ts', '.jsx', '.tsx', '.vue'];
		/** 按用途分类的后端相关文件扩展名数组 */
		const backend = ['.py', '.java', '.php', '.rb', '.go', '.rs', '.kt', '.scala', '.cs', '.swift'];
		/** 按用途分类的系统相关文件扩展名数组 */
		const system = ['.c', '.cpp', '.cxx', '.h', '.hpp'];
		/** 按用途分类的数据相关文件扩展名数组 */
		const data = ['.json', '.xml', '.csv', '.sql', '.yml', '.yaml'];
		/** 按用途分类的脚本相关文件扩展名数组 */
		const script = ['.sh', '.bat', '.ps1'];
		/** 按用途分类的配置相关文件扩展名数组 */
		const config = ['.pem'];
		/** 合并所有分类的文本文件扩展名数组 */
		const textExtensions = [...plainText, ...web, ...backend, ...system, ...data, ...script, ...config];
		/** 文件名扩展名 */
		const ext = filename.toLowerCase().slice(filename.lastIndexOf('.'));
		// 检查扩展名是否在合并后的文本文件扩展名数组中
		return textExtensions.includes(ext);
	}
	/**
	 * 获取文件图标
	 *
	 * @param {string} filename - 文件名
	 * @returns {string} - 文件图标 HTML 字符串
	 */
	getFileIcon(filename) {
		/** 文件名扩展名 */
		const ext = filename.toLowerCase().slice(filename.lastIndexOf('.'));
		/** 文件图标映射表 */
		const iconMap = {
			'.txt': '<i class="fas fa-file-alt"></i>',
			'.md': '<i class="fab fa-markdown"></i>',
			'.json': '<i class="fas fa-file-code"></i>',
			'.html': '<i class="fab fa-html5"></i>',
			'.css': '<i class="fab fa-css3-alt"></i>',
			'.js': '<i class="fab fa-js"></i>',
			'.ts': '<i class="fab fa-js"></i>',
			'.pdf': '<i class="fas fa-file-pdf"></i>',
			'.doc': '<i class="fas fa-file-word"></i>',
			'.docx': '<i class="fas fa-file-word"></i>',
			'.xls': '<i class="fas fa-file-excel"></i>',
			'.xlsx': '<i class="fas fa-file-excel"></i>',
			'.ppt': '<i class="fas fa-file-powerpoint"></i>',
			'.pptx': '<i class="fas fa-file-powerpoint"></i>',
			'.zip': '<i class="fas fa-file-archive"></i>',
			'.rar': '<i class="fas fa-file-archive"></i>',
			'.7z': '<i class="fas fa-file-archive"></i>',
			'.mp3': '<i class="fas fa-file-audio"></i>',
			'.mp4': '<i class="fas fa-file-video"></i>',
		};
		// 返回映射表中对应的图标 HTML 字符串，若不存在则返回默认图标
		return iconMap[ext] || '<i class="fas fa-file"></i>';
	}
	/**
	 * 格式化文件大小
	 *
	 * @param {number} bytes - 文件大小（字节）
	 * @returns {string} - 格式化后的文件大小字符串
	 */
	formatFileSize(bytes) {
		// 处理文件大小为 0 的情况
		if (bytes === 0) return '0 B';
		/** 字节转换单位 */
		const k = 1024;
		/** 文件大小单位数组 */
		const sizes = ['B', 'KB', 'MB', 'GB', 'TB'];
		/** 计算文件大小单位索引 */
		const i = Math.floor(Math.log(bytes) / Math.log(k));
		// 格式化文件大小为两位小数，并添加单位
		return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i];
	}
	/**
	 * 格式化日期字符串
	 *
	 * @param {string} dateString - 日期字符串（ISO 8601 格式）
	 * @returns {string} - 格式化后的日期字符串（例如：2023-08-25 14:30）
	 */
	formatDate(dateString) {
		/** 日期对象 */
		const date = new Date(dateString);
		// 格式化日期为中国标准时间（东八区）
		return date.toLocaleString('zh-CN',
			{
				year: 'numeric',
				month: '2-digit',
				day: '2-digit',
				hour: '2-digit',
				minute: '2-digit'
			}
		);
	}
	/**
	 * 对文件名进行编码
	 *
	 * @param {string} filename - 文件名
	 * @returns {string} - 编码后的文件名
	 */
	encodeFileName(filename) {
		/**
		 * 对输入参数进行 URI 编码，确保特殊字符被正确处理
		 */
		const encodedParams = encodeURIComponent(filename);
		/**
		 * 将 URI 编码后的十六进制字符转换为对应的字符
		 */
		const decodedParams = encodedParams.replace(/%([0-9A-F]{2})/g, (_, p1) => String.fromCharCode(parseInt(p1, 16)));
		// 对转换后的字符进行 Base64 编码并返回
		return btoa(decodedParams);
	}
	/**
	 * 检查文件名是否有效
	 *
	 * @param {string} filename - 文件名
	 * @returns {boolean} - 是否有效
	 */
	isValidFileName(filename) {
		/** 检查文件名是否包含无效字符 */
		const invalidChars = /[<>:/"\\|?*]/;
		// 检查文件名是否包含无效字符或为空
		return !invalidChars.test(filename) && filename.trim() !== '';
	}
	/**
	 * 显示 Toast 提示
	 *
	 * @param {string} message - 提示消息
	 * @param {string} type - 提示类型（可选，默认：'info'）
	 */
	showToast(message, type = 'info') {
		/** Toast 元素 */
		const toast = document.getElementById('toast');
		// 设置 Toast 消息内容
		toast.textContent = message;
		toast.className = `toast ${type} show`;
		// 3 秒后移除显示类名
		setTimeout(() => toast.classList.remove('show'), 3000);
	}
}
// 初始化文件管理器
document.addEventListener('DOMContentLoaded', () => new FileManager());