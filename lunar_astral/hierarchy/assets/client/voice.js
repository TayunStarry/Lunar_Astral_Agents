import { AudioQueue } from './tts.js';

/**
 * 语音对话管理器
 *
 * 基于 Web Speech API 实现语音识别，将用户语音实时转换为文字输入。
 * 默认关闭状态，需手动开启。
 *
 * 设计参考：local_data/package/different_lunar/script.js 语音识别实现
 *
 * 逻辑控制：
 * - 使用 continuous=false，每次识别完成后自动重启
 * - 语音识别超时时自动重启识别进程
 * - 当音频播放列表不为空或正在播放音频时，立即停止语音识别
 * - 待音频播放完成后，自动恢复语音识别功能
 * - not-allowed / aborted 错误不触发重启，通过 onErrorCallback 通知 UI 层
 */

// ==== 语音识别超时时间（毫秒） ====
const RECOGNITION_TIMEOUT = 8000;

// ==== 语音静默超时（无结果返回时视为超时） ====
const SILENCE_TIMEOUT = 10000;

// ==== 错误后重启延迟 ====
const ERROR_RESTART_DELAY = 500;

export class VoiceChatManager {
    /** @type {SpeechRecognition|null} */
    recognition = null;
    /** @type {boolean} 是否已启用语音对话 */
    enabled = false;
    /** @type {boolean} 是否正在识别 */
    listening = false;
    /** @type {boolean} 因音频播放而暂停 */
    pausedByAudio = false;
    /** @type {boolean} 发生不可恢复错误（如 not-allowed），需用户手动重新开启 */
    errorState = false;
    /** @type {number|null} 超时定时器 */
    timeoutTimer = null;
    /** @type {number|null} 静默超时定时器 */
    silenceTimer = null;
    /** @type {number|null} 重启定时器 */
    restartTimer = null;
    /** @type {Function|null} 识别结果回调 */
    onResultCallback = null;
    /** @type {Function|null} 临时识别结果回调（用于实时显示） */
    onInterimResultCallback = null;
    /** @type {Function|null} 状态变更回调 */
    onStatusChangeCallback = null;
    /** @type {Function|null} 错误回调（用于通知UI层显示友好提示） */
    onErrorCallback = null;

    /**
     * 获取 SpeechRecognition 构造函数
     *
     * @returns {SpeechRecognitionConstructor|null}
     */
    get RecognitionCtor() {
        return window.SpeechRecognition || window.webkitSpeechRecognition || null;
    }

    /**
     * 检查当前环境是否支持语音识别
     * - 需要 Web Speech API
     * - 需要安全上下文（HTTPS 或 localhost）
     *
     * @returns {{ available: boolean, reason: string }}
     */
    checkAvailability() {
        if (!this.RecognitionCtor) {
            return { available: false, reason: '当前浏览器不支持语音识别' };
        }
        if (!window.isSecureContext) {
            return { available: false, reason: '语音识别需要 HTTPS 安全连接' };
        }
        return { available: true, reason: '' };
    }

    /**
     * 创建新的语音识别实例（每次启动时重新创建，避免复用导致的状态残留）
     *
     * @returns {SpeechRecognition|null}
     */
    createRecognition() {
        const Ctor = this.RecognitionCtor;
        if (!Ctor) return null;

        const rec = new Ctor();
        rec.lang = 'zh-CN';
        rec.interimResults = true;
        // 参考 different_lunar 使用 continuous=false，每次识别结束后自动重建
        rec.continuous = false;
        rec.maxAlternatives = 1;

        rec.onresult = (event) => {
            this.handleResult(event);
        };

        rec.onerror = (event) => {
            this.handleError(event);
        };

        rec.onend = () => {
            this.handleEnd();
        };

        rec.onaudiostart = () => {
            this.listening = true;
        };

        rec.onspeechstart = () => {
            this.resetSilenceTimer();
        };

        return rec;
    }

    // ==== 事件处理 ====

    /**
     * 处理语音识别结果
     */
    handleResult(event) {
        this.resetSilenceTimer();

        let interimTranscript = '';
        let finalTranscript = '';

        for (let i = event.resultIndex; i < event.results.length; i++) {
            const result = event.results[i];
            if (result.isFinal) {
                finalTranscript += result[0].transcript;
            } else {
                interimTranscript += result[0].transcript;
            }
        }

        // 实时显示临时结果
        if (interimTranscript && this.onInterimResultCallback) {
            this.onInterimResultCallback(interimTranscript.trim());
        }

        if (finalTranscript && this.onResultCallback) {
            this.onResultCallback(finalTranscript.trim());
        }
    }

    /**
     * 处理语音识别错误
     *
     * 参考 different_lunar 的 SpeechRecognitionErrorOccurred：
     * - aborted / not-allowed：直接返回，不做任何操作（避免触发自动重启循环）
     * - 其他错误：清除计时器，延迟后尝试重启
     */
    handleError(event) {
        // 清除重启计时器
        this.clearRestartTimer();

        switch (event.error) {
            case 'aborted':
                // 用户手动停止或代码主动 stop()，静默处理
                return;

            case 'not-allowed':
                // 麦克风权限被拒绝，标记错误状态，通知UI层
                this.errorState = true;
                this.listening = false;
                this.recognition = null;
                this.enabled = false;
                this.notifyStatusChange();
                // 通知UI层显示友好提示
                if (this.onErrorCallback) {
                    this.onErrorCallback('not-allowed');
                }
                return;

            case 'no-speech':
                // 无人声，允许重启
                break;

            default:
                // network 等其他错误，允许重启
                break;
        }

        // 延迟后尝试重启
        if (this.enabled && !this.pausedByAudio && !this.errorState) {
            this.restartTimer = setTimeout(() => {
                if (this.enabled && !this.pausedByAudio && !this.errorState) {
                    this.startListening();
                }
            }, ERROR_RESTART_DELAY);
        }
    }

    /**
     * 处理语音识别结束事件
     *
     * continuous=false 时，每次识别完成都会触发 onend，
     * 需要在此处判断是否需要自动重启。
     */
    handleEnd() {
        this.listening = false;

        // 发生不可恢复错误，不重启
        if (this.errorState) return;

        // 用户关闭或音频暂停中，不重启
        if (!this.enabled || this.pausedByAudio) return;

        // 自动重启下一轮识别
        this.clearRestartTimer();
        this.restartTimer = setTimeout(() => {
            if (this.enabled && !this.pausedByAudio && !this.errorState) {
                this.startListening();
            }
        }, ERROR_RESTART_DELAY);
    }

    // ==== 公共接口 ====

    /**
     * 开启语音对话
     */
    enable() {
        // 先检查环境是否支持
        const check = this.checkAvailability();
        if (!check.available) {
            if (this.onErrorCallback) {
                this.onErrorCallback('unsupported', check.reason);
            }
            return false;
        }

        this.enabled = true;
        this.pausedByAudio = false;
        this.errorState = false;
        this.startListening();
        this.notifyStatusChange();
        return true;
    }

    /**
     * 关闭语音对话
     */
    disable() {
        this.enabled = false;
        this.pausedByAudio = false;
        this.errorState = false;
        this.stopListening();
        this.notifyStatusChange();
    }

    /**
     * 切换语音对话状态
     *
     * @returns {boolean} 切换后的启用状态
     */
    toggle() {
        if (this.enabled) {
            this.disable();
        } else {
            this.enable();
        }
        return this.enabled;
    }

    /**
     * 开始语音识别（每次重建实例，避免复用导致的 not-allowed 等状态残留）
     */
    startListening() {
        if (this.listening) return;
        if (this.errorState) return;

        // 检查音频播放状态，如果正在播放则暂停识别
        if (this.isAudioPlaying()) {
            this.pausedByAudio = true;
            return;
        }

        // 每次启动时重新创建实例（参考 different_lunar 的 createSpeechRecognition 模式）
        this.recognition = this.createRecognition();
        if (!this.recognition) return;

        try {
            this.recognition.start();
            this.listening = true;
            this.resetTimeout();
            this.resetSilenceTimer();
        } catch (err) {
            this.recognition = null;
            // 延迟后重试
            this.clearRestartTimer();
            this.restartTimer = setTimeout(() => {
                if (this.enabled && !this.pausedByAudio && !this.errorState) {
                    this.startListening();
                }
            }, ERROR_RESTART_DELAY);
        }
    }

    /**
     * 停止语音识别
     */
    stopListening() {
        this.clearTimers();
        this.clearRestartTimer();

        if (this.recognition) {
            try {
                this.recognition.stop();
            } catch (e) {
                // 忽略已停止的错误
            }
            this.recognition = null;
        }
        this.listening = false;
    }

    // ==== 定时器管理 ====

    /**
     * 清除重启定时器
     */
    clearRestartTimer() {
        if (this.restartTimer) {
            clearTimeout(this.restartTimer);
            this.restartTimer = null;
        }
    }

    /**
     * 重置超时定时器
     */
    resetTimeout() {
        if (this.timeoutTimer) {
            clearTimeout(this.timeoutTimer);
        }

        this.timeoutTimer = setTimeout(() => {
            if (this.enabled && !this.pausedByAudio && !this.errorState) {
                this.stopListening();
                // handleEnd 会自动重启
            }
        }, RECOGNITION_TIMEOUT);
    }

    /**
     * 重置静默超时定时器
     */
    resetSilenceTimer() {
        if (this.silenceTimer) {
            clearTimeout(this.silenceTimer);
        }

        this.silenceTimer = setTimeout(() => {
            if (this.enabled && !this.pausedByAudio && !this.errorState) {
                this.stopListening();
                // stopListening 清除了 recognition，handleEnd 不会触发
                // 因此这里手动调度重启
                this.clearRestartTimer();
                this.restartTimer = setTimeout(() => {
                    if (this.enabled && !this.pausedByAudio && !this.errorState && !this.listening) {
                        this.startListening();
                    }
                }, ERROR_RESTART_DELAY);
            }
        }, SILENCE_TIMEOUT);
    }

    /**
     * 清除所有超时/静默定时器
     */
    clearTimers() {
        if (this.timeoutTimer) {
            clearTimeout(this.timeoutTimer);
            this.timeoutTimer = null;
        }
        if (this.silenceTimer) {
            clearTimeout(this.silenceTimer);
            this.silenceTimer = null;
        }
    }

    // ==== 音频协调 ====

    /**
     * 检查音频是否正在播放
     *
     * @returns {boolean}
     */
    isAudioPlaying() {
        const status = AudioQueue.getStatus();
        return status.playing || status.queueLength > 0;
    }

    /**
     * 音频播放状态变化时的处理
     * 由外部（AudioQueue播放/停止时）调用
     */
    onAudioPlaybackChange() {
        if (!this.enabled || this.errorState) return;

        if (this.isAudioPlaying()) {
            // 音频开始播放，暂停语音识别
            if (this.listening) {
                this.pausedByAudio = true;
                this.stopListening();
            }
        } else {
            // 音频播放完成，恢复语音识别
            if (this.pausedByAudio) {
                this.pausedByAudio = false;
                this.startListening();
            }
        }
    }

    // ==== 回调注册 ====

    /**
     * 注册识别结果回调
     *
     * @param {(text: string) => void} callback
     */
    onResult(callback) {
        this.onResultCallback = callback;
    }

    /**
     * 注册临时识别结果回调（用于实时显示识别中的文字）
     *
     * @param {(text: string) => void} callback
     */
    onInterimResult(callback) {
        this.onInterimResultCallback = callback;
    }

    /**
     * 注册状态变更回调
     *
     * @param {(enabled: boolean) => void} callback
     */
    onStatusChange(callback) {
        this.onStatusChangeCallback = callback;
    }

    /**
     * 注册错误回调
     *
     * @param {(errorType: string, reason?: string) => void} callback
     * - errorType: 'not-allowed' | 'unsupported'
     * - reason: 不可用时的原因描述
     */
    onError(callback) {
        this.onErrorCallback = callback;
    }

    /**
     * 通知状态变更
     */
    notifyStatusChange() {
        if (this.onStatusChangeCallback) {
            this.onStatusChangeCallback(this.enabled);
        }
    }

    /**
     * 销毁语音识别实例
     */
    destroy() {
        this.disable();
        this.onResultCallback = null;
        this.onInterimResultCallback = null;
        this.onStatusChangeCallback = null;
        this.onErrorCallback = null;
    }
}

/** 全局语音对话单例 */
export const VoiceChat = new VoiceChatManager();