(function() {
  var fileInput = document.getElementById('fileInput');
  var uploadZone = document.getElementById('uploadZone');
  var fileInfo = document.getElementById('fileInfo');
  var fileName = document.getElementById('fileName');
  var removeFileBtn = document.getElementById('removeFileBtn');
  var transcribeBtn = document.getElementById('transcribeBtn');
  var recordBtn = document.getElementById('recordBtn');
  var statusBar = document.getElementById('statusBar');
  var statusText = document.getElementById('statusText');
  var spinner = document.getElementById('spinner');
  var resultsSection = document.getElementById('resultsSection');
  var resultText = document.getElementById('resultText');
  var confidenceBadge = document.getElementById('confidenceBadge');
  var confidenceText = document.getElementById('confidenceText');
  var durationBadge = document.getElementById('durationBadge');
  var durationText = document.getElementById('durationText');
  var recordingTimer = document.getElementById('recordingTimer');

  var selectedFile = null;
  var mediaRecorder = null;
  var audioChunks = [];
  var isRecording = false;
  var recordingStartTime = 0;
  var timerInterval = null;

  var audioPlayer = document.getElementById('audioPlayer');
  var playBtn = document.getElementById('playBtn');
  var audioSeek = document.getElementById('audioSeek');
  var audioTime = document.getElementById('audioTime');
  var audioObj = null;
  var audioUrl = null;
  var seekUpdateInterval = null;

  uploadZone.addEventListener('click', function(e) {
    if (e.target !== fileInput) {
      fileInput.click();
    }
  });

  uploadZone.addEventListener('dragover', function(e) {
    e.preventDefault();
    uploadZone.classList.add('drag-over');
  });

  uploadZone.addEventListener('dragleave', function() {
    uploadZone.classList.remove('drag-over');
  });

  uploadZone.addEventListener('drop', function(e) {
    e.preventDefault();
    uploadZone.classList.remove('drag-over');
    var files = e.dataTransfer.files;
    if (files.length > 0) {
      handleFile(files[0]);
    }
  });

  fileInput.addEventListener('change', function() {
    if (fileInput.files.length > 0) {
      handleFile(fileInput.files[0]);
    }
  });

  removeFileBtn.addEventListener('click', function() {
    clearFile();
  });

  function handleFile(file) {
    var ext = file.name.split('.').pop().toLowerCase();
    if (ext !== 'wav') {
      showStatus('仅支持 WAV 格式音频文件', 'error');
      return;
    }
    stopAudio();
    selectedFile = file;
    fileName.textContent = file.name + ' (' + formatSize(file.size) + ')';
    fileInfo.classList.add('visible');
    transcribeBtn.disabled = false;
    hideStatus();
    initAudioPlayer(file);
  }

  function clearFile() {
    stopAudio();
    selectedFile = null;
    fileInput.value = '';
    fileInfo.classList.remove('visible');
    transcribeBtn.disabled = true;
    fileName.textContent = '';
  }

  function formatSize(bytes) {
    if (bytes < 1024) return bytes + ' B';
    if (bytes < 1024 * 1024) return (bytes / 1024).toFixed(1) + ' KB';
    return (bytes / (1024 * 1024)).toFixed(1) + ' MB';
  }

  function initAudioPlayer(file) {
    stopAudio();
    audioUrl = URL.createObjectURL(file);
    audioObj = new Audio(audioUrl);
    audioObj.addEventListener('loadedmetadata', function() {
      audioSeek.max = audioObj.duration;
      audioSeek.value = 0;
      audioTime.textContent = '00:00';
    });
    audioObj.addEventListener('timeupdate', function() {
      if (!audioSeek.dragging) {
        audioSeek.value = audioObj.currentTime;
      }
      audioTime.textContent = formatTime(audioObj.currentTime);
    });
    audioObj.addEventListener('ended', function() {
      setPlayIcon(true);
    });
    audioObj.addEventListener('play', function() {
      setPlayIcon(false);
    });
    audioObj.addEventListener('pause', function() {
      setPlayIcon(true);
    });
    audioPlayer.classList.add('visible');
    audioTime.textContent = '00:00';
    audioSeek.value = 0;
    setPlayIcon(true);
  }

  function stopAudio() {
    if (audioObj) {
      audioObj.pause();
      audioObj.src = '';
      audioObj = null;
    }
    if (audioUrl) {
      URL.revokeObjectURL(audioUrl);
      audioUrl = null;
    }
    audioPlayer.classList.remove('visible');
  }

  function togglePlay() {
    if (!audioObj) return;
    if (audioObj.paused) {
      audioObj.play();
    } else {
      audioObj.pause();
    }
  }

  function setPlayIcon(paused) {
    if (paused) {
      playBtn.querySelector('svg').innerHTML = '<polygon points="5 3 19 12 5 21 5 3"/>';
    } else {
      playBtn.querySelector('svg').innerHTML = '<rect x="6" y="6" width="12" height="12" rx="2"/>';
    }
  }

  function formatTime(seconds) {
    var m = Math.floor(seconds / 60);
    var s = Math.floor(seconds % 60);
    return (m < 10 ? '0' : '') + m + ':' + (s < 10 ? '0' : '') + s;
  }

  playBtn.addEventListener('click', togglePlay);

  audioSeek.addEventListener('input', function() {
    if (audioObj) {
      audioObj.currentTime = parseFloat(audioSeek.value);
      audioTime.textContent = formatTime(audioObj.currentTime);
    }
  });

  audioSeek.addEventListener('mousedown', function() {
    audioSeek.dragging = true;
  });

  audioSeek.addEventListener('mouseup', function() {
    audioSeek.dragging = false;
  });

  transcribeBtn.addEventListener('click', function() {
    if (!selectedFile) return;
    transcribeAudio(selectedFile);
  });

  function transcribeAudio(file) {
    showStatus('正在识别中...', 'processing');

    var formData = new FormData();
    formData.append('audio', file);

    var startTime = Date.now();

    fetch('/asr', {
      method: 'POST',
      body: formData
    })
    .then(function(response) {
      if (!response.ok) {
        return response.json().then(function(data) {
          throw new Error(data.error || '服务器错误 (' + response.status + ')');
        });
      }
      return response.json();
    })
    .then(function(data) {
      var duration = Date.now() - startTime;
      if (data.status === 'success') {
        showResult(data.text, data.confidence, duration);
        showStatus('识别完成', 'success');
        setTimeout(function() { hideStatus(); }, 2500);
      } else {
        showStatus(data.error || '识别失败', 'error');
      }
    })
    .catch(function(err) {
      showStatus(err.message || '网络连接失败，请检查服务器', 'error');
    });
  }

  recordBtn.addEventListener('click', function() {
    if (isRecording) {
      stopRecording();
    } else {
      startRecording();
    }
  });

  function startRecording() {
    if (!navigator.mediaDevices || !navigator.mediaDevices.getUserMedia) {
      showStatus('当前浏览器不支持录音功能', 'error');
      return;
    }

    navigator.mediaDevices.getUserMedia({ audio: true })
      .then(function(stream) {
        audioChunks = [];

        var mimeType = 'audio/webm';
        if (MediaRecorder.isTypeSupported('audio/webm;codecs=opus')) {
          mimeType = 'audio/webm;codecs=opus';
        } else if (MediaRecorder.isTypeSupported('audio/wav')) {
          mimeType = 'audio/wav';
        }

        mediaRecorder = new MediaRecorder(stream, { mimeType: mimeType });

        mediaRecorder.addEventListener('dataavailable', function(e) {
          if (e.data.size > 0) {
            audioChunks.push(e.data);
          }
        });

        mediaRecorder.addEventListener('stop', function() {
          stream.getTracks().forEach(function(track) { track.stop(); });
          processRecording();
        });

        mediaRecorder.start();
        isRecording = true;
        recordingStartTime = Date.now();
        recordBtn.classList.add('recording');
        recordBtn.querySelector('svg').innerHTML = '<rect x="6" y="6" width="12" height="12" rx="2"/>';
        recordBtn.childNodes[recordBtn.childNodes.length - 1].textContent = '停止';
        transcribeBtn.disabled = true;
        recordingTimer.classList.add('visible');
        hideStatus();
        hideResults();
        updateTimer();
        timerInterval = setInterval(updateTimer, 200);
      })
      .catch(function(err) {
        showStatus('无法访问麦克风: ' + err.message, 'error');
      });
  }

  function stopRecording() {
    if (mediaRecorder && mediaRecorder.state === 'recording') {
      mediaRecorder.stop();
    }
    isRecording = false;
    recordBtn.classList.remove('recording');
    recordBtn.querySelector('svg').innerHTML = '<circle cx="12" cy="12" r="6"/><circle cx="12" cy="12" r="6"/>';
    recordBtn.childNodes[recordBtn.childNodes.length - 1].textContent = '录制';
    recordingTimer.classList.remove('visible');
    if (timerInterval) {
      clearInterval(timerInterval);
      timerInterval = null;
    }
    transcribeBtn.disabled = false;
  }

  function updateTimer() {
    var elapsed = Math.floor((Date.now() - recordingStartTime) / 1000);
    var mins = Math.floor(elapsed / 60);
    var secs = elapsed % 60;
    recordingTimer.textContent =
      (mins < 10 ? '0' : '') + mins + ':' + (secs < 10 ? '0' : '') + secs;
  }

  function processRecording() {
    if (audioChunks.length === 0) return;

    var blob = new Blob(audioChunks, { type: audioChunks[0].type });
    var file = new File([blob], 'recording.wav', { type: 'audio/wav' });
    selectedFile = file;
    fileName.textContent = '录音文件 (' + formatSize(blob.size) + ')';
    fileInfo.classList.add('visible');
    transcribeBtn.disabled = false;
  }

  function showStatus(msg, type) {
    statusBar.className = 'status-bar visible ' + type;
    statusText.textContent = msg;
    if (type === 'processing') {
      spinner.style.display = 'block';
    } else {
      spinner.style.display = 'none';
    }
  }

  function hideStatus() {
    statusBar.classList.remove('visible', 'processing', 'success', 'error');
  }

  function showResult(text, confidence, durationMs) {
    resultText.textContent = text || '（无内容）';
    var pct = Math.round((confidence || 0) * 100);
    confidenceText.textContent = '置信度 ' + pct + '%';
    resultsSection.classList.add('visible');

    if (pct >= 80) {
      confidenceBadge.style.background = 'rgba(52, 211, 153, 0.12)';
      confidenceBadge.style.borderColor = 'rgba(52, 211, 153, 0.2)';
      confidenceBadge.style.color = 'rgba(110, 231, 183, 0.9)';
    } else if (pct >= 60) {
      confidenceBadge.style.background = 'rgba(251, 191, 36, 0.12)';
      confidenceBadge.style.borderColor = 'rgba(251, 191, 36, 0.2)';
      confidenceBadge.style.color = 'rgba(252, 211, 77, 0.9)';
    } else {
      confidenceBadge.style.background = 'rgba(239, 68, 68, 0.12)';
      confidenceBadge.style.borderColor = 'rgba(239, 68, 68, 0.2)';
      confidenceBadge.style.color = 'rgba(252, 165, 165, 0.9)';
    }

    if (durationMs !== undefined) {
      var durationSec = (durationMs / 1000).toFixed(2);
      durationText.textContent = '识别耗时 ' + durationSec + 's';
    }
  }

  function hideResults() {
    resultsSection.classList.remove('visible');
  }

  fetch('/health')
    .then(function(r) { return r.json(); })
    .then(function(data) {
      if (data.status === 'healthy') {
        console.log('ASR 服务已连接');
      }
    })
    .catch(function() {
      console.log('等待服务就绪...');
    });
})();
