// ============================================================
//  星月智能 · 消息终端 — 文件拖放 / 待发送附件（悬浮气泡）
// ============================================================

// ---------- 文件拖放 / 待发送附件（悬浮气泡） ----------
async function addPendingFiles(files) {
    for (const file of files) {
        const category = getFileCategory(file);
        const entry = { file, category, name: file.name, previewUrl: null };
        if (category === 'image') {
            entry.previewUrl = await readFileAsDataUrl(file);
        } else if (category === 'video' || category === 'audio') {
            entry.previewUrl = URL.createObjectURL(file);
        }
        pendingFiles.push(entry);
    }
    renderPendingAttachments();
}

function renderPendingAttachments() {
    pendingAttachments.innerHTML = '';
    if (!pendingFiles.length) {
        pendingAttachments.hidden = true;
        return;
    }
    pendingAttachments.hidden = false;

    const icons = { audio: 'fa-music', text: 'fa-file-alt', other: 'fa-file' };

    pendingFiles.forEach((pf, index) => {
        const item = document.createElement('div');
        item.className = 'pending-attachment-item';

        const preview = document.createElement('div');
        preview.className = 'pending-attachment-preview';
        if (pf.category === 'image') {
            const img = document.createElement('img');
            img.src = pf.previewUrl;
            img.alt = pf.name;
            img.style.cssText = 'width:100%;height:100%;object-fit:cover;';
            preview.appendChild(img);
        } else if (pf.category === 'video') {
            const video = document.createElement('video');
            video.src = pf.previewUrl;
            video.muted = true;
            video.playsInline = true;
            video.style.cssText = 'width:100%;height:100%;object-fit:cover;';
            preview.appendChild(video);
        } else {
            preview.innerHTML = `<i class="fas ${icons[pf.category] || icons.other}"></i>`;
        }

        const name = document.createElement('div');
        name.className = 'pending-attachment-name';
        name.textContent = pf.name;
        name.title = pf.name;

        const removeBtn = document.createElement('button');
        removeBtn.className = 'pending-attachment-remove';
        removeBtn.title = '移除';
        removeBtn.innerHTML = '<i class="fas fa-times"></i>';
        removeBtn.addEventListener('click', () => removePendingFile(index));

        item.appendChild(preview);
        item.appendChild(name);
        item.appendChild(removeBtn);
        pendingAttachments.appendChild(item);
    });
}

function removePendingFile(index) {
    const removed = pendingFiles.splice(index, 1)[0];
    if (removed && removed.previewUrl && removed.previewUrl.startsWith('blob:')) {
        URL.revokeObjectURL(removed.previewUrl);
    }
    renderPendingAttachments();
}

function clearPendingFiles() {
    pendingFiles.forEach(pf => {
        if (pf.previewUrl && pf.previewUrl.startsWith('blob:')) URL.revokeObjectURL(pf.previewUrl);
    });
    pendingFiles = [];
    renderPendingAttachments();
}

function setupDragEvents() {
    document.addEventListener('dragenter', (e) => {
        e.preventDefault();
        dragCounter++;
        if (dragCounter === 1 && e.dataTransfer && Array.from(e.dataTransfer.types).includes('Files')) {
            dragOverlay.classList.add('active');
        }
    });
    document.addEventListener('dragover', (e) => e.preventDefault());
    document.addEventListener('dragleave', (e) => {
        e.preventDefault();
        dragCounter--;
        if (dragCounter === 0) dragOverlay.classList.remove('active');
    });
    document.addEventListener('drop', (e) => {
        e.preventDefault();
        dragCounter = 0;
        dragOverlay.classList.remove('active');
        const files = e.dataTransfer && e.dataTransfer.files;
        if (files && files.length) addPendingFiles(Array.from(files));
    });
}
