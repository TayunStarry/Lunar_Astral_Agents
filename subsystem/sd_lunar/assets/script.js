(function () {
    var currentTab = 'txt2img';
    var isGenerating = false;
    var serverConfig = null;

    function $(sel) { return document.querySelector(sel); }
    function $$(sel) { return document.querySelectorAll(sel); }

    function init() {
        loadServerConfig();
        setupTabs();
        setupForms();
        setupSliders();
        setupImageUpload();
    }

    function loadServerConfig() {
        fetch('/sd/config')
            .then(function (r) { return r.json(); })
            .then(function (data) {
                serverConfig = data;
                updateStatusBadges(data);
            })
            .catch(function () {
                updateStatusBadge($('#engineStatus'), 'error', '引擎连接失败');
                updateStatusBadge($('#gpuStatus'), 'error', 'GPU状态未知');
            });
    }

    function updateStatusBadges(cfg) {
        if (cfg.allow_diffusion) {
            updateStatusBadge($('#engineStatus'), 'ready', 'SD引擎就绪');
        } else {
            updateStatusBadge($('#engineStatus'), 'error', '扩散生成未启用');
        }
        updateStatusBadge($('#gpuStatus'), 'ready', 'Vulkan GPU加速可用');
    }

    function updateStatusBadge(el, cls, text) {
        el.className = 'status-badge ' + cls;
        el.textContent = text;
    }

    function setupTabs() {
        $$('.tab-btn').forEach(function (btn) {
            btn.addEventListener('click', function () {
                var tab = this.dataset.tab;
                switchTab(tab);
            });
        });
    }

    function switchTab(tab) {
        currentTab = tab;
        $$('.tab-btn').forEach(function (b) {
            b.classList.toggle('active', b.dataset.tab === tab);
        });
        $$('.panel').forEach(function (p) {
            p.classList.toggle('active', p.id === tab + '-panel');
        });
    }

    function setupSliders() {
        var sliderPairs = [
            { slider: 't-steps', val: 't-steps-val' },
            { slider: 't-cfg', val: 't-cfg-val' },
            { slider: 'i-steps', val: 'i-steps-val' },
            { slider: 'i-cfg', val: 'i-cfg-val' },
            { slider: 'i-strength', val: 'i-strength-val' },
        ];

        sliderPairs.forEach(function (pair) {
            var slider = $('#' + pair.slider);
            if (!slider) return;
            slider.addEventListener('input', function () {
                var valEl = $('#' + pair.val);
                if (valEl) {
                    valEl.textContent = this.value;
                }
            });
        });
    }

    function setupImageUpload() {
        var fileInput = $('#i-init-img');
        var uploadArea = $('#upload-area');
        var placeholder = $('#upload-placeholder');
        var preview = $('#upload-preview');

        if (!fileInput || !preview) return;

        fileInput.addEventListener('change', function () {
            if (this.files && this.files[0]) {
                var reader = new FileReader();
                reader.onload = function (e) {
                    preview.src = e.target.result;
                    preview.classList.add('visible');
                    placeholder.classList.add('hidden');
                    uploadArea.classList.add('has-image');
                };
                reader.readAsDataURL(this.files[0]);
            }
        });

        uploadArea.addEventListener('dragover', function (e) {
            e.preventDefault();
            this.style.borderColor = 'var(--accent-purple)';
        });

        uploadArea.addEventListener('dragleave', function () {
            this.style.borderColor = '';
        });

        uploadArea.addEventListener('drop', function (e) {
            e.preventDefault();
            this.style.borderColor = '';
            var dt = e.dataTransfer;
            if (dt.files && dt.files[0]) {
                fileInput.files = dt.files;
                fileInput.dispatchEvent(new Event('change'));
            }
        });
    }

    function setupForms() {
        var txt2imgForm = $('#txt2img-form');
        var img2imgForm = $('#img2img-form');

        if (txt2imgForm) {
            txt2imgForm.addEventListener('submit', function (e) {
                e.preventDefault();
                if (isGenerating) return;
                handleTxt2Img();
            });
        }

        if (img2imgForm) {
            img2imgForm.addEventListener('submit', function (e) {
                e.preventDefault();
                if (isGenerating) return;
                handleImg2Img();
            });
        }
    }

    function handleTxt2Img() {
        var prompt = $('#t-prompt').value.trim();
        var negative = $('#t-negative').value.trim();
        var width = parseInt($('#t-width').value, 10);
        var height = parseInt($('#t-height').value, 10);
        var steps = parseInt($('#t-steps').value, 10);
        var cfg = parseFloat($('#t-cfg').value);
        var seed = parseInt($('#t-seed').value, 10);
        var batch = parseInt($('#t-batch').value, 10);
        var vulkan = $('#t-vulkan').checked;

        if (!prompt) {
            showToast('请输入正向提示词', 'error');
            return;
        }

        if (!validateParams(width, height, steps, cfg, batch)) return;

        var payload = {
            prompt: prompt,
            negative_prompt: negative,
            width: width,
            height: height,
            steps: steps,
            cfg_scale: cfg,
            seed: seed,
            batch_size: batch,
            use_vulkan: vulkan,
            diffusion_model: '',
            vae_model: '',
            refine_model: '',
        };

        submitGeneration('/sd/txt2img', payload, 'txt2img');
    }

    function handleImg2Img() {
        var previewImg = $('#upload-preview');
        var initImgSrc = previewImg.src;

        if (!previewImg.classList.contains('visible') || !initImgSrc || initImgSrc === window.location.href) {
            showToast('请先上传一张初始图像', 'error');
            return;
        }

        var prompt = $('#i-prompt').value.trim();
        var negative = $('#i-negative').value.trim();
        var strength = parseFloat($('#i-strength').value);
        var width = parseInt($('#i-width').value, 10);
        var height = parseInt($('#i-height').value, 10);
        var steps = parseInt($('#i-steps').value, 10);
        var cfg = parseFloat($('#i-cfg').value);
        var seed = parseInt($('#i-seed').value, 10);
        var batch = parseInt($('#i-batch').value, 10);
        var vulkan = $('#i-vulkan').checked;

        if (!prompt) {
            showToast('请输入正向提示词', 'error');
            return;
        }

        if (!validateParams(width, height, steps, cfg, batch)) return;

        var base64Data = initImgSrc;
        if (base64Data.indexOf('base64,') !== -1) {
            base64Data = base64Data.split('base64,')[1];
        }

        var payload = {
            prompt: prompt,
            negative_prompt: negative,
            width: width,
            height: height,
            steps: steps,
            cfg_scale: cfg,
            seed: seed,
            batch_size: batch,
            strength: strength,
            init_img_base64: base64Data,
            use_vulkan: vulkan,
            diffusion_model: '',
            vae_model: '',
            refine_model: '',
        };

        submitGeneration('/sd/img2img', payload, 'img2img');
    }

    function validateParams(width, height, steps, cfg, batch) {
        if (width < 64 || width > 2048 || height < 64 || height > 2048) {
            showToast('图像尺寸应在 64~2048 之间', 'error');
            return false;
        }
        if (steps < 1 || steps > 150) {
            showToast('采样步数应在 1~150 之间', 'error');
            return false;
        }
        if (cfg < 1 || cfg > 30) {
            showToast('CFG 引导值应在 1.0~30.0 之间', 'error');
            return false;
        }
        if (batch < 1 || batch > 8) {
            showToast('批量大小应在 1~8 之间', 'error');
            return false;
        }
        return true;
    }

    function submitGeneration(url, payload, genType) {
        isGenerating = true;
        disableButtons(true);
        showProgress(true, '正在提交生成任务...');

        fetch(url, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(payload),
        })
            .then(function (r) { return r.json(); })
            .then(function (data) {
                if (!data.success) {
                    throw new Error(data.error || '任务提交失败');
                }
                showProgress(true, '任务已提交，等待生成完成...');
                showToast('任务已提交: ' + data.task_id, 'info');
                return pollTask(data.task_id, genType);
            })
            .then(function (result) {
                if (result.success) {
                    addResultCard(result.data, genType);
                    showToast('生成完成！', 'success');
                } else {
                    throw new Error(result.error || '生成失败');
                }
            })
            .catch(function (err) {
                showToast(err.message || '请求出错', 'error');
            })
            .finally(function () {
                isGenerating = false;
                disableButtons(false);
                showProgress(false);
            });
    }

    function pollTask(taskId, genType) {
        return fetch('/sd/poll/' + taskId)
            .then(function (r) { return r.json(); })
            .then(function (data) {
                if (data.success) {
                    showProgress(true, '生成完成，正在加载结果...');
                    return fetchResult(taskId);
                }
                if (data.error === '任务等待超时') {
                    throw new Error('任务执行超时');
                }
                throw new Error(data.error || '任务执行失败');
            });
    }

    function fetchResult(taskId) {
        return fetch('/sd/result/' + taskId)
            .then(function (r) { return r.json(); });
    }

    function addResultCard(task, genType) {
        var grid = $('#results-grid');
        var empty = grid.querySelector('.empty-results');
        if (empty) {
            empty.remove();
        }

        var card = document.createElement('div');
        card.className = 'result-card';

        var header = document.createElement('div');
        header.className = 'result-card-header';

        var tag = document.createElement('span');
        tag.className = 'result-tag ' + genType;
        tag.textContent = genType === 'txt2img' ? '文生图' : '图生图';

        var time = document.createElement('span');
        var d = new Date();
        time.textContent = d.getHours().toString().padStart(2, '0') + ':' +
            d.getMinutes().toString().padStart(2, '0') + ':' +
            d.getSeconds().toString().padStart(2, '0');
        header.appendChild(tag);
        header.appendChild(time);

        var img = document.createElement('img');
        img.className = 'result-card-image';
        if (task.result_base64) {
            img.src = 'data:image/png;base64,' + task.result_base64;
        } else if (task.result_path) {
            img.src = task.result_path;
        } else {
            img.alt = '生成结果';
        }

        var info = document.createElement('div');
        info.className = 'result-card-info';
        if (task.width && task.height) {
            var dims = document.createElement('span');
            dims.textContent = task.width + 'x' + task.height;
            info.appendChild(dims);
        }
        if (task.steps) {
            var st = document.createElement('span');
            st.textContent = task.steps + '步';
            info.appendChild(st);
        }
        if (task.cfg_scale) {
            var cf = document.createElement('span');
            cf.textContent = 'CFG ' + task.cfg_scale;
            info.appendChild(cf);
        }
        if (task.seed) {
            var sd = document.createElement('span');
            sd.textContent = '种子 ' + task.seed;
            info.appendChild(sd);
        }

        card.appendChild(header);
        card.appendChild(img);
        card.appendChild(info);
        grid.insertBefore(card, grid.firstChild);
    }

    function showProgress(show, text) {
        var container = $('#progress-container');
        var textEl = $('#progress-text');
        if (show) {
            container.classList.add('active');
            if (text && textEl) {
                textEl.textContent = text;
            }
        } else {
            container.classList.remove('active');
        }
    }

    function disableButtons(disabled) {
        $$('.generate-btn').forEach(function (btn) {
            btn.disabled = disabled;
        });
    }

    function showToast(message, type) {
        var toast = document.createElement('div');
        toast.className = 'toast ' + (type || 'info');
        toast.textContent = message;
        document.body.appendChild(toast);

        setTimeout(function () {
            toast.style.opacity = '0';
            toast.style.transition = 'opacity 0.3s ease';
            setTimeout(function () {
                if (toast.parentNode) {
                    toast.parentNode.removeChild(toast);
                }
            }, 300);
        }, 4000);
    }

    document.addEventListener('DOMContentLoaded', init);
})();