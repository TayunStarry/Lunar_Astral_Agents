/**
 * TTS语速滑块
 */
export const speechSpeedSlider = document.getElementById("speechSpeedSlider") as HTMLInputElement;
/**
 * TTS音量滑块
 */
export const speechVolumeSlider = document.getElementById("speechVolumeSlider") as HTMLInputElement;
/**
 * 滑块 -> 用户消息截断长度
 */
export const messageSliceLengthSlider = document.getElementById('messageSliceLengthSlider') as HTMLInputElement;

import * as EntryAPI from '../EntryAPI/code';

/**
 * 绑定滑块与输入框，实现双向同步功能
 * 此函数会将温度、最大令牌数、语音速度和语音音量对应的滑块与输入框进行绑定
 */
export function bindSlider() {
    /**
     * 绑定单个滑块与输入框，实现双向同步
     *
     * @param {HTMLInputElement} slider - 滑块元素
     *
     * @param {HTMLInputElement} input - 输入框元素
     */
    function event(slider: HTMLInputElement, input: HTMLInputElement) {
        // 当滑块值改变时，将滑块的值同步到输入框
        slider.addEventListener('input', function () { input.value = this.value; });
        // 当输入框值改变时，将输入框的值同步到滑块
        input.addEventListener('input', function () { slider.value = this.value; });
    };
    // 绑定最大令牌数滑块与输入框
    event(messageSliceLengthSlider, EntryAPI.messageSliceLength);
    // 绑定语音速度滑块与输入框
    event(speechSpeedSlider, EntryAPI.speechSpeedValue);
    // 绑定语音音量滑块与输入框
    event(speechVolumeSlider, EntryAPI.speechVolumeValue);
};