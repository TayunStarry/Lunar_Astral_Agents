// ============================================================
//  星月智能 · 消息终端 — 截图功能
// ============================================================

// ---------- 截图功能 ----------
async function captureScreen() {
    showToast('正在截图…', 'info');
    try {
        const res = await fetch('/capture', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ mode: 'fullscreen', format: 'png' })
        });
        if (!res.ok) {
            const errText = await res.text();
            throw new Error(errText || `截图失败 (${res.status})`);
        }
        const blob = await res.blob();
        const file = new File([blob], `screenshot-${Date.now()}.png`, { type: blob.type || 'image/png' });
        openCaptureModal(file);
    } catch (err) {
        showToast('截图失败：' + (err.message || err), 'error');
    }
}

function openCaptureModal(file) {
    captureFile = file;
    capturePreviewUrl = URL.createObjectURL(file);
    capturePreviewImg.src = capturePreviewUrl;
    captureModal.classList.add('active');
    captureModal.setAttribute('aria-hidden', 'false');
}

function closeCaptureModal() {
    captureModal.classList.remove('active');
    captureModal.setAttribute('aria-hidden', 'true');
    if (capturePreviewUrl) {
        URL.revokeObjectURL(capturePreviewUrl);
        capturePreviewUrl = null;
    }
    captureFile = null;
    capturePreviewImg.removeAttribute('src');
}

async function sendCaptureToPending() {
    const file = captureFile;
    if (!file) return;
    closeCaptureModal();
    await addPendingFiles([file]);
    showToast('截图已加入待发送列表', 'success');
}

async function sendCaptureToDrawboard() {
    const file = captureFile;
    if (!file) return;
    closeCaptureModal();
    openDrawboard();
    await importDrawboardBackground(file);
}

function setupCapture() {
    captureBtn.addEventListener('click', captureScreen);
    captureToSendBtn.addEventListener('click', sendCaptureToPending);
    captureToDrawboardBtn.addEventListener('click', sendCaptureToDrawboard);
    captureCloseBtn.addEventListener('click', closeCaptureModal);
    captureModal.addEventListener('click', (e) => {
        if (e.target === captureModal) closeCaptureModal();
    });
    document.addEventListener('keydown', (e) => {
        if (e.key === 'Escape' && captureModal.classList.contains('active')) {
            closeCaptureModal();
        }
    });
}
