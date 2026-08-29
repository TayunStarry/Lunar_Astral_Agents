// ============================================================
//  星月智能 · 消息终端 — 画板：合并导出与发送
// ============================================================

function getDrawboardMergedBlob() {
    return new Promise((resolve) => {
        const temp = document.createElement('canvas');
        temp.width = drawboardLayer.width || DRAWBOARD_DEFAULT_W;
        temp.height = drawboardLayer.height || DRAWBOARD_DEFAULT_H;
        const ctx = temp.getContext('2d');
        ctx.fillStyle = '#ffffff';
        ctx.fillRect(0, 0, temp.width, temp.height);
        if (drawboard.hasImage) ctx.drawImage(drawboardBg, 0, 0);
        ctx.drawImage(drawboardLayer, 0, 0);
        temp.toBlob(resolve, 'image/png');
    });
}

function setDrawboardSending(sending) {
    drawboardSendBtn.disabled = sending;
    drawboardSendBtn.innerHTML = sending ? '<i class="fas fa-spinner fa-pulse"></i>' : '<i class="fas fa-paper-plane"></i>';
}

async function sendDrawboardMessage() {
    const text = drawboardInput.value.trim();
    if (!text) {
        showToast('请输入文字内容', 'warning');
        return;
    }
    if (!drawboard.hasImage && !drawboard.dirty) {
        showToast('请先导入背景图或进行绘制', 'warning');
        return;
    }

    setDrawboardSending(true);
    try {
        const blob = await getDrawboardMergedBlob();
        if (!blob) throw new Error('合并画板失败');
        const file = new File([blob], `drawboard-${Date.now()}.png`, { type: 'image/png' });
        const fileUrl = await saveFile(file);
        const relSrc = fileUrl.replace(window.location.origin, '');

        const userMsg = {
            id: generateId(),
            role: 'user',
            categories: ['text', 'image'],
            content: text,
            attachments: [{ type: 'image', src: relSrc, label: '画板' }],
            timestamp: Date.now()
        };
        addMessage(userMsg);

        if (backendConnected) {
            const content = [
                { type: 'text', text },
                { type: 'image_url', image_url: { url: fileUrl } }
            ];
            await sendMessages([{ role: 'user', content }]);
        } else {
            showToast('离线模式：画板消息仅本地展示', 'info');
        }

        drawboardInput.value = '';
    } catch (err) {
        showToast('发送失败：' + (err.message || err), 'error');
    } finally {
        setDrawboardSending(false);
    }
}

function setupDrawboard() {
    initDrawboardCanvas();

    openDrawboardBtn.addEventListener('click', openDrawboard);
    closeDrawboardBtn.addEventListener('click', closeDrawboard);
    drawboardOverlay.addEventListener('click', (e) => {
        if (e.target === drawboardOverlay) closeDrawboard();
    });

    importBgBtn.addEventListener('click', () => bgFileInput.click());
    bgFileInput.addEventListener('change', (e) => {
        const file = e.target.files && e.target.files[0];
        if (file) importDrawboardBackground(file);
        bgFileInput.value = '';
    });
    clearDrawBtn.addEventListener('click', clearDrawboard);
    undoDrawBtn.addEventListener('click', undoDrawboard);

    document.querySelectorAll('.drawboard-tool[data-tool]').forEach(btn => {
        btn.addEventListener('click', () => setDrawboardTool(btn.dataset.tool, btn));
    });
    document.querySelectorAll('.drawboard-color').forEach(el => {
        el.addEventListener('click', () => setDrawboardColor(el.dataset.color, el));
    });
    document.querySelectorAll('.drawboard-size').forEach(el => {
        el.addEventListener('click', () => setDrawboardSize(el.dataset.size, el));
    });

    setDrawboardTool('draw', document.querySelector('.drawboard-tool[data-tool="draw"]'));
    setDrawboardColor('#e74c3c', document.querySelector('.drawboard-color[data-color="#e74c3c"]'));
    setDrawboardSize('8', document.querySelector('.drawboard-size[data-size="8"]'));

    drawboardLayer.addEventListener('mousedown', drawboardStart);
    drawboardLayer.addEventListener('mousemove', drawboardMove);
    drawboardLayer.addEventListener('mouseup', drawboardStop);
    drawboardLayer.addEventListener('mouseleave', drawboardStop);

    drawboardSendBtn.addEventListener('click', sendDrawboardMessage);
    drawboardInput.addEventListener('keydown', (e) => {
        if (e.key === 'Enter' && !e.shiftKey) {
            e.preventDefault();
            sendDrawboardMessage();
        }
    });

    document.addEventListener('keydown', (e) => {
        if (e.key === 'Escape' && drawboardOverlay.classList.contains('active')) {
            closeDrawboard();
        }
    });
}
