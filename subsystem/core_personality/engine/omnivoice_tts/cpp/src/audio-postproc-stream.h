#pragma once
// audio-postproc-stream.h: streaming version of audio-postproc.h.
//
// Three stateful stages chained as a pipeline: crossfader -> silence remover
// -> fade and pad. Each stage exposes push(samples, n, emit) and flush(emit)
// where emit is a callable bool(const float*, int) returning false to abort.
// Bit perfect against the buffered path on the audio shapes that OmniVoice
// produces in practice (chunks of about 15 s, far above twice the fade length
// of 0.1 s and the silence horizon of 0.5 s).
//
// Order in the pipeline matches _post_process_audio in omnivoice.py :
//   1. cross_fade_chunks (concat with fade out + silence + fade in)
//   2. remove_silence (mid silence drop, edge trim)
//   3. volume scale (ref_rms branch only; voice design skips here)
//   4. fade_and_pad (fade in + fade out + leading and trailing silence pad)

#include "audio-postproc.h"

#include <algorithm>
#include <cmath>
#include <cstdint>
#include <utility>
#include <vector>

// Stage 1: streaming cross fade. Holds the last fade_n samples of the most
// recent emission as `pending`. On the next push, applies fade out to the tail
// of pending, emits pending, emits silence_n zeros, applies fade in to the
// head of the new chunk, emits the new chunk minus its own tail. The retained
// tail becomes the new pending. Last chunk: flush emits pending verbatim, no
// trailing fade out, matching the Python behaviour of leaving the final
// chunk's tail intact.
struct crossfader_stream {
    int                fade_n;
    int                silence_n;
    bool               first_chunk;
    std::vector<float> pending;

    void init(int sr, double silence_dur) {
        int total_n = (int) (silence_dur * (double) sr);
        fade_n      = total_n / 3;
        silence_n   = fade_n;
        first_chunk = true;
        pending.clear();
    }

    template <class Emit> bool push(const float * samples, int n, Emit emit) {
        if (n <= 0) {
            return true;
        }

        if (first_chunk) {
            // Emit body, retain last fade_n as pending.
            int emit_n = std::max(0, n - fade_n);
            if (emit_n > 0) {
                if (!emit(samples, emit_n)) {
                    return false;
                }
            }
            pending.assign(samples + emit_n, samples + n);
            first_chunk = false;
            return true;
        }

        // Fade out tail of pending.
        int fout_n = std::min(fade_n, (int) pending.size());
        if (fout_n > 0) {
            int denom = std::max(fout_n - 1, 1);
            for (int j = 0; j < fout_n; j++) {
                float w = 1.0f - (float) j / (float) denom;
                pending[pending.size() - (size_t) fout_n + (size_t) j] *= w;
            }
        }

        // Emit pending (now fade out applied at its tail).
        if (!pending.empty()) {
            if (!emit(pending.data(), (int) pending.size())) {
                return false;
            }
        }

        // Emit silence gap.
        if (silence_n > 0) {
            std::vector<float> silence((size_t) silence_n, 0.0f);
            if (!emit(silence.data(), silence_n)) {
                return false;
            }
        }

        // Apply fade in to first fade_n of the new chunk in a copy.
        std::vector<float> chunk_copy(samples, samples + n);
        int                fin_n = std::min(fade_n, n);
        if (fin_n > 0) {
            int denom = std::max(fin_n - 1, 1);
            for (int j = 0; j < fin_n; j++) {
                float w = (float) j / (float) denom;
                chunk_copy[(size_t) j] *= w;
            }
        }

        // Emit body of the new chunk, retain last fade_n as new pending.
        int emit_n = std::max(0, n - fade_n);
        if (emit_n > 0) {
            if (!emit(chunk_copy.data(), emit_n)) {
                return false;
            }
        }
        pending.assign(chunk_copy.begin() + emit_n, chunk_copy.end());
        return true;
    }

    template <class Emit> bool flush(Emit emit) {
        if (!pending.empty()) {
            if (!emit(pending.data(), (int) pending.size())) {
                return false;
            }
            pending.clear();
        }
        return true;
    }
};

// Stage 2: streaming silence remover. Bit perfect against remove_silence in
// audio-postproc.h on chunks where mid silent groups are scoped within the
// look ahead horizon of min_sil_n samples. Internally accumulates pushed
// samples (in float and int16, in lockstep) and advances an emit cursor as
// scan windows close. Trail trim runs at flush by reversing the un emitted
// suffix and reusing postproc_detect_leading_silence.
//
// Latency: up to min_sil_n samples (500 ms at 24 kHz, mid_sil=500). Once a
// silent group closes (next non silent seek_step), the prefix up to its
// determined drop boundary emits in one shot.
struct silence_remover_stream {
    int    sr;
    int    mid_sil_ms;
    int    lead_sil_ms;
    int    trail_sil_ms;
    double thresh_lin;

    int seek_step;
    int min_sil_n;
    int keep_n;
    int chunk_n;

    std::vector<float>   buf_f;
    std::vector<int16_t> buf_s;
    size_t               emit_pos;
    bool                 lead_done;
    size_t               scan_pos;
    std::vector<size_t>  silent_grp;

    void init(int sr_, int mid_sil_ms_, int lead_sil_ms_, int trail_sil_ms_, double thresh_db) {
        sr           = sr_;
        mid_sil_ms   = mid_sil_ms_;
        lead_sil_ms  = lead_sil_ms_;
        trail_sil_ms = trail_sil_ms_;
        thresh_lin   = 32768.0 * std::pow(10.0, thresh_db / 20.0);
        seek_step    = sr / 100;
        min_sil_n    = sr * mid_sil_ms / 1000;
        keep_n       = min_sil_n;
        chunk_n      = sr / 100;
        buf_f.clear();
        buf_s.clear();
        emit_pos  = 0;
        lead_done = false;
        scan_pos  = 0;
        silent_grp.clear();
    }

    // Append n float samples to both buffers. Mirrors postproc_f32_to_s16 :
    // multiply by 32768.0 with truncation toward 0 and clamp to int16 range.
    void append(const float * samples, int n) {
        size_t old = buf_f.size();
        buf_f.insert(buf_f.end(), samples, samples + n);
        buf_s.resize(old + (size_t) n);
        for (size_t i = 0; i < (size_t) n; i++) {
            double v = (double) samples[i] * 32768.0;
            if (v > 32767.0) {
                v = 32767.0;
            }
            if (v < -32768.0) {
                v = -32768.0;
            }
            buf_s[old + i] = (int16_t) v;
        }
    }

    template <class Emit> bool push(const float * samples, int n, Emit emit) {
        if (n <= 0) {
            return true;
        }
        append(samples, n);

        // Lead trim phase. Scan chunks of chunk_n until the first non silent
        // is found; until then no emission happens.
        if (!lead_done) {
            int trim    = 0;
            int seg_len = (int) buf_s.size();
            while (trim < seg_len) {
                int    slice_end = std::min(trim + chunk_n, seg_len);
                int    sn        = slice_end - trim;
                double r         = postproc_slice_rms_s16(buf_s, (size_t) trim, (size_t) sn);
                if (r >= thresh_lin) {
                    break;
                }
                trim += chunk_n;
            }
            if (trim >= seg_len) {
                return true;
            }
            int lead_keep = sr * lead_sil_ms / 1000;
            int p         = std::max(0, trim - lead_keep);
            emit_pos      = (size_t) p;
            lead_done     = true;
            scan_pos      = emit_pos;
        }

        // Mid scan: advance scan_pos by seek_step while we have a full
        // min_sil_n window ahead. Track contiguous silent runs in silent_grp.
        while (scan_pos + (size_t) min_sil_n <= buf_s.size()) {
            double r         = postproc_slice_rms_s16(buf_s, scan_pos, (size_t) min_sil_n);
            bool   is_silent = (r <= thresh_lin);
            if (is_silent) {
                silent_grp.push_back(scan_pos);
            } else {
                if (!silent_grp.empty()) {
                    if (!close_silent_group(emit)) {
                        return false;
                    }
                }
            }
            scan_pos += (size_t) seek_step;
        }

        // Emit safe prefix: when no silent group is pending, every sample up
        // to scan_pos is decided. With a pending group, hold everything until
        // the group closes (its drop boundary depends on the group's end).
        if (silent_grp.empty()) {
            if (scan_pos > emit_pos) {
                if (!emit(&buf_f[emit_pos], (int) (scan_pos - emit_pos))) {
                    return false;
                }
                emit_pos = scan_pos;
            }
        }
        return true;
    }

    // Closes the active silent group [s, e] following the pydub pairwise
    // midpoint dedup rule: if the gap is at least 2*keep_n, drop the middle
    // and keep keep_n on each side; otherwise keep all samples but split the
    // overlap at (s + e) / 2 to avoid duplicating samples in the concat.
    template <class Emit> bool close_silent_group(Emit emit) {
        size_t s = silent_grp.front();
        size_t e = silent_grp.back() + (size_t) min_sil_n;
        if (e - s >= (size_t) (2 * keep_n)) {
            size_t emit_end = s + (size_t) keep_n;
            if (emit_end > emit_pos) {
                if (!emit(&buf_f[emit_pos], (int) (emit_end - emit_pos))) {
                    return false;
                }
            }
            emit_pos = e - (size_t) keep_n;
        } else {
            size_t mid = (s + e) / 2;
            if (mid > emit_pos) {
                if (!emit(&buf_f[emit_pos], (int) (mid - emit_pos))) {
                    return false;
                }
            }
            emit_pos = mid;
        }
        silent_grp.clear();
        return true;
    }

    // Trail trim: reverse the un emitted suffix and run the same leading
    // silence detector as remove_silence does. The trailing silence amount
    // beyond trail_sil_ms gets dropped, matching the buffered path verbatim.
    template <class Emit> bool flush(Emit emit) {
        // A still open silent group at flush is the trailing silence: the
        // pydub split_on_silence keeps margin keep_n past the last non silent,
        // then the trail trim reduces that margin to trail_sil_ms.
        size_t end_emit;
        if (!silent_grp.empty()) {
            size_t s = silent_grp.front();
            end_emit = s + (size_t) keep_n;
            if (end_emit > buf_s.size()) {
                end_emit = buf_s.size();
            }
            silent_grp.clear();
        } else {
            end_emit = buf_s.size();
        }

        if (emit_pos >= end_emit) {
            return true;
        }

        // Build a reversed int16 view of the un emitted suffix and let the
        // existing detector find the trailing silence length in samples.
        std::vector<int16_t> rev(buf_s.begin() + emit_pos, buf_s.begin() + end_emit);
        std::reverse(rev.begin(), rev.end());

        int trim_back  = postproc_detect_leading_silence(rev, thresh_lin, chunk_n);
        int trail_keep = sr * trail_sil_ms / 1000;
        int drop_trail = std::max(0, trim_back - trail_keep);

        size_t emit_n = end_emit - emit_pos;
        if (drop_trail >= (int) emit_n) {
            emit_pos = end_emit;
            return true;
        }
        emit_n -= (size_t) drop_trail;
        if (!emit(&buf_f[emit_pos], (int) emit_n)) {
            return false;
        }
        emit_pos = end_emit;
        return true;
    }
};

// Stage 3: streaming fade and pad. Emits pad_n zeros at start, holds the
// first fade_n samples to apply fade in, then streams the body while keeping
// the last fade_n samples as a tail buffer for the closing fade out. Flush
// applies fade out, emits the tail, then emits pad_n trailing zeros. Bit
// perfect against fade_and_pad_audio for inputs of at least 2*fade_n samples
// (always true on OmniVoice outputs, where audio is on the order of seconds
// and fade_n is 0.1 s).
struct fade_pad_stream {
    int sr;
    int fade_n;
    int pad_n;

    bool               started;
    bool               head_faded_in;
    std::vector<float> head_buf;
    std::vector<float> tail_buf;

    void init(int sr_, double fade_dur, double pad_dur) {
        sr            = sr_;
        fade_n        = (int) (fade_dur * (double) sr);
        pad_n         = (int) (pad_dur * (double) sr);
        started       = false;
        head_faded_in = false;
        head_buf.clear();
        tail_buf.clear();
    }

    template <class Emit> bool push(const float * samples, int n, Emit emit) {
        if (n <= 0) {
            return true;
        }

        // Leading pad: emit pad_n zeros once, on the first non empty push.
        if (!started) {
            if (pad_n > 0) {
                std::vector<float> z((size_t) pad_n, 0.0f);
                if (!emit(z.data(), pad_n)) {
                    return false;
                }
            }
            started = true;
        }

        const float * cursor    = samples;
        int           remaining = n;

        // Phase 1: collect first fade_n samples, apply fade in, emit.
        if (!head_faded_in) {
            int need = fade_n - (int) head_buf.size();
            int take = std::min(need, remaining);
            head_buf.insert(head_buf.end(), cursor, cursor + take);
            cursor += take;
            remaining -= take;

            if ((int) head_buf.size() == fade_n) {
                int denom = std::max(fade_n - 1, 1);
                for (int j = 0; j < fade_n; j++) {
                    head_buf[(size_t) j] *= (float) j / (float) denom;
                }
                if (!emit(head_buf.data(), fade_n)) {
                    return false;
                }
                head_buf.clear();
                head_faded_in = true;
            } else {
                return true;
            }
        }

        if (remaining <= 0) {
            return true;
        }

        // Phase 2: append to tail_buf, emit all but the last fade_n.
        tail_buf.insert(tail_buf.end(), cursor, cursor + remaining);
        int emit_n = (int) tail_buf.size() - fade_n;
        if (emit_n > 0) {
            if (!emit(tail_buf.data(), emit_n)) {
                return false;
            }
            tail_buf.erase(tail_buf.begin(), tail_buf.begin() + emit_n);
        }
        return true;
    }

    template <class Emit> bool flush(Emit emit) {
        if (!started) {
            return true;
        }

        if (!head_faded_in) {
            // Total audio shorter than fade_n. Apply degraded fade with k =
            // total / 2, matching fade_and_pad_audio for short inputs.
            int total = (int) head_buf.size();
            int k     = std::min(fade_n, total / 2);
            if (k > 0) {
                int denom = std::max(k - 1, 1);
                for (int j = 0; j < k; j++) {
                    head_buf[(size_t) j] *= (float) j / (float) denom;
                }
                for (int j = 0; j < k; j++) {
                    head_buf[head_buf.size() - (size_t) k + (size_t) j] *= 1.0f - (float) j / (float) denom;
                }
            }
            if (total > 0) {
                if (!emit(head_buf.data(), total)) {
                    return false;
                }
            }
            head_buf.clear();
        } else {
            // Tail buffer holds the last fade_n samples. Apply fade out and
            // emit. For inputs of at least 2*fade_n samples (always true on
            // OmniVoice outputs), this matches fade_and_pad_audio exactly.
            int k = (int) tail_buf.size();
            if (k > 0) {
                int denom = std::max(k - 1, 1);
                for (int j = 0; j < k; j++) {
                    tail_buf[(size_t) j] *= 1.0f - (float) j / (float) denom;
                }
                if (!emit(tail_buf.data(), k)) {
                    return false;
                }
            }
            tail_buf.clear();
        }

        // Trailing pad.
        if (pad_n > 0) {
            std::vector<float> z((size_t) pad_n, 0.0f);
            if (!emit(z.data(), pad_n)) {
                return false;
            }
        }
        return true;
    }
};
