/* qwen3tts_c_api.h — C API wrapper for qwen3-tts.cpp (Nim FFI) */
#ifndef QWEN3TTS_C_API_H
#define QWEN3TTS_C_API_H

#include <stdint.h>

#ifdef __cplusplus
extern "C" {
#endif

/* Opaque handle */
typedef struct Qwen3Tts Qwen3Tts;

/* Generation parameters */
typedef struct Qwen3TtsParams {
    int32_t max_audio_tokens;    /* default: 4096 */
    float   temperature;         /* default: 0.9, 0=greedy */
    float   top_p;               /* default: 1.0 */
    int32_t top_k;               /* default: 50, 0=disabled */
    int32_t n_threads;           /* default: 4 */
    float   repetition_penalty;  /* default: 1.05 */
    int32_t language_id;         /* 2050=en, 2058=ja, 2055=zh, etc. */
} Qwen3TtsParams;

/* Generated audio result */
typedef struct Qwen3TtsAudio {
    const float* samples;  /* PCM float32 mono */
    int32_t n_samples;
    int32_t sample_rate;   /* always 24000 */
} Qwen3TtsAudio;

/* Fill params with defaults */
void qwen3_tts_default_params(Qwen3TtsParams* params);

/* Create TTS engine and load models from directory.
 * model_dir must contain qwen3-tts-0.6b-f16.gguf and
 * qwen3-tts-tokenizer-f16.gguf.
 * Returns NULL on failure. */
Qwen3Tts* qwen3_tts_create(const char* model_dir, int32_t n_threads);

/* Check if models are loaded */
int qwen3_tts_is_loaded(const Qwen3Tts* tts);

/* Synthesize text to audio. Returns NULL on failure.
 * Caller must free with qwen3_tts_free_audio(). */
Qwen3TtsAudio* qwen3_tts_synthesize(
    Qwen3Tts* tts,
    const char* text,
    const Qwen3TtsParams* params);

/* Get sample rate (always 24000) */
int32_t qwen3_tts_sample_rate(const Qwen3Tts* tts);

/* Free generated audio */
void qwen3_tts_free_audio(Qwen3TtsAudio* audio);

/* Destroy TTS engine */
void qwen3_tts_destroy(Qwen3Tts* tts);

/* Synthesize with voice cloning from WAV file.
 * reference_audio_path: path to reference WAV (24kHz mono recommended).
 * Returns NULL on failure. Caller must free with qwen3_tts_free_audio(). */
Qwen3TtsAudio* qwen3_tts_synthesize_with_voice_file(
    Qwen3Tts* tts,
    const char* text,
    const char* reference_audio_path,
    const Qwen3TtsParams* params);

/* Synthesize with voice cloning from raw samples.
 * ref_samples: 24kHz mono float32 normalized to [-1, 1].
 * Returns NULL on failure. Caller must free with qwen3_tts_free_audio(). */
Qwen3TtsAudio* qwen3_tts_synthesize_with_voice_samples(
    Qwen3Tts* tts,
    const char* text,
    const float* ref_samples,
    int32_t n_ref_samples,
    const Qwen3TtsParams* params);

/* Extract speaker embedding from WAV file (for caching).
 * embedding_out: caller-allocated buffer for the embedding.
 * max_size: size of embedding_out in floats.
 * Returns the actual embedding size (typically 1024), or -1 on failure. */
int32_t qwen3_tts_extract_embedding_file(
    Qwen3Tts* tts,
    const char* reference_audio_path,
    float* embedding_out,
    int32_t max_size);

/* Synthesize with pre-computed speaker embedding (skips encoder).
 * embedding: speaker embedding from qwen3_tts_extract_embedding_file().
 * embedding_size: must match the size returned by extract.
 * Returns NULL on failure. Caller must free with qwen3_tts_free_audio(). */
Qwen3TtsAudio* qwen3_tts_synthesize_with_embedding(
    Qwen3Tts* tts,
    const char* text,
    const float* embedding,
    int32_t embedding_size,
    const Qwen3TtsParams* params);

/* Streaming callback: called for each PCM chunk.
 * samples: float32 PCM mono array (normalized [-1, 1])
 * n_samples: number of samples in this chunk
 * sample_rate: always 24000
 * is_final: 1 if this is the last chunk, 0 otherwise
 * user_data: opaque pointer passed to synthesize_streaming
 * Return 0 to continue, non-zero to abort generation. */
typedef int (*qwen3_tts_stream_callback)(
    const float* samples,
    int32_t n_samples,
    int32_t sample_rate,
    int is_final,
    void* user_data);

/* Synthesize text with streaming PCM output.
 * reference_audio_path: path to reference WAV (24kHz mono).
 * callback: invoked for each decoded PCM chunk.
 * user_data: passed through to callback.
 * chunk_frames: number of frames to accumulate before callback (0=default ~50 frames ≈ 1s).
 * Returns 0 on success, non-zero on failure or callback abort. */
int qwen3_tts_synthesize_streaming(
    Qwen3Tts* tts,
    const char* text,
    const char* reference_audio_path,
    const Qwen3TtsParams* params,
    qwen3_tts_stream_callback callback,
    void* user_data,
    int32_t chunk_frames);

/* Synthesize with streaming PCM output and pre-computed speaker embedding (skips encoder).
 * embedding: speaker embedding from qwen3_tts_extract_embedding_file().
 * embedding_size: must match the size returned by extract.
 * callback: invoked for each decoded PCM chunk.
 * user_data: passed through to callback.
 * chunk_frames: number of frames to accumulate before callback (0=default ~50 frames ≈ 1s).
 * Returns 0 on success, non-zero on failure or callback abort. */
int qwen3_tts_synthesize_streaming_with_embedding(
    Qwen3Tts* tts,
    const char* text,
    const float* embedding,
    int32_t embedding_size,
    const Qwen3TtsParams* params,
    qwen3_tts_stream_callback callback,
    void* user_data,
    int32_t chunk_frames);

/* Get last error message (or empty string) */
const char* qwen3_tts_get_error(const Qwen3Tts* tts);

#ifdef __cplusplus
}
#endif

#endif /* QWEN3TTS_C_API_H */
