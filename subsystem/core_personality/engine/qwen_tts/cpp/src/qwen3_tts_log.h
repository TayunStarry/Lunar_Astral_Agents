#ifndef QWEN3_TTS_LOG_H
#define QWEN3_TTS_LOG_H

#include <cstdio>
#include <cstdlib>

// 原生库诊断输出开关：默认静默。
// 宿主（星月智能 TUI 控制台）独占终端屏幕，原生库直写 stderr 会撕裂界面，
// 故引擎的诊断信息（模型加载、内存采样、合成耗时统计）默认不输出；
// 需要排查性能时以环境变量 QWEN3_TTS_VERBOSE=1 开启（行为与关闭前一致）。
namespace qwen3_tts {
inline bool verbose_log() {
    static const bool enabled = [] {
        const char * v = std::getenv("QWEN3_TTS_VERBOSE");
        return v && (v[0] == '1' || v[0] == 'y' || v[0] == 'Y' || v[0] == 't' || v[0] == 'T');
    }();
    return enabled;
}
} // namespace qwen3_tts

#define TTS_LOG(...)                      \
    do {                                  \
        if (qwen3_tts::verbose_log()) {   \
            fprintf(stderr, __VA_ARGS__); \
        }                                 \
    } while (0)

#endif // QWEN3_TTS_LOG_H
