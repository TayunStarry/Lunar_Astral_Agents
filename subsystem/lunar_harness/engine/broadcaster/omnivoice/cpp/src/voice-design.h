#pragma once
// voice_design.h: speaker attribute validation and normalisation for the
// OmniVoice instruct string. Mirrors omnivoice/utils/voice_design.py and the
// _resolve_instruct logic in omnivoice/models/omnivoice.py.
//
// Six mutually exclusive categories :
//   gender   (male / female)
//   age      (child / teenager / young adult / middle-aged / elderly)
//   pitch    (very low / low / moderate / high / very high)
//   style    (whisper)
//   accent   (10 English-only labels)
//   dialect  (12 Chinese-only labels)
//
// Translation tables map every category item that exists in both English and
// Chinese. Accents are English-only and dialects are Chinese-only, both are
// passed through without translation but still subject to category conflict
// checks.

#include <algorithm>
#include <cctype>
#include <cstdint>
#include <set>
#include <string>
#include <unordered_map>
#include <vector>

struct VoiceDesign {
    std::unordered_map<std::string, std::string> en_to_zh;
    std::unordered_map<std::string, std::string> zh_to_en;
    std::set<std::string>                        all_valid;
    std::set<std::string>                        valid_en;
    std::set<std::string>                        valid_zh;
    std::vector<std::set<std::string>>           mutually_exclusive;
};

// Decode one UTF-8 sequence at p, write the codepoint to *cp, return the byte
// count (0 on malformed).
static int voice_design_utf8_decode(const char * p, size_t avail, uint32_t * cp) {
    if (avail == 0) {
        return 0;
    }
    uint8_t c = (uint8_t) p[0];
    if (c < 0x80) {
        *cp = c;
        return 1;
    }
    if ((c & 0xE0) == 0xC0 && avail >= 2) {
        *cp = ((uint32_t) (c & 0x1F) << 6) | ((uint32_t) (uint8_t) p[1] & 0x3F);
        return 2;
    }
    if ((c & 0xF0) == 0xE0 && avail >= 3) {
        *cp = ((uint32_t) (c & 0x0F) << 12) | ((uint32_t) ((uint8_t) p[1] & 0x3F) << 6) |
              ((uint32_t) (uint8_t) p[2] & 0x3F);
        return 3;
    }
    if ((c & 0xF8) == 0xF0 && avail >= 4) {
        *cp = ((uint32_t) (c & 0x07) << 18) | ((uint32_t) ((uint8_t) p[1] & 0x3F) << 12) |
              ((uint32_t) ((uint8_t) p[2] & 0x3F) << 6) | ((uint32_t) (uint8_t) p[3] & 0x3F);
        return 4;
    }
    return 0;
}

// True if the codepoint sits in the CJK Unified Ideographs main range, the
// same range _ZH_RE uses in voice_design.py.
static bool voice_design_is_cjk(uint32_t cp) {
    return cp >= 0x4E00 && cp <= 0x9FFF;
}

// True if the UTF-8 string contains at least one CJK ideograph.
static bool voice_design_has_cjk(const std::string & s) {
    size_t i = 0;
    while (i < s.size()) {
        uint32_t cp = 0;
        int      n  = voice_design_utf8_decode(s.data() + i, s.size() - i, &cp);
        if (n == 0) {
            i++;
            continue;
        }
        if (voice_design_is_cjk(cp)) {
            return true;
        }
        i += n;
    }
    return false;
}

// Longest common substring within a[ai:ai+am) and b[bi:bi+bm), recursive
// Ratcliff-Obershelp matching. Returns the total matched character count.
// Same algorithm as Python difflib.SequenceMatcher.get_matching_blocks().
static size_t voice_design_match(const std::string & a,
                                 size_t              ai,
                                 size_t              am,
                                 const std::string & b,
                                 size_t              bi,
                                 size_t              bm) {
    if (am == 0 || bm == 0) {
        return 0;
    }
    size_t              best_size = 0;
    size_t              best_a    = ai;
    size_t              best_b    = bi;
    std::vector<size_t> prev(bm + 1, 0);
    std::vector<size_t> curr(bm + 1, 0);
    for (size_t i = 0; i < am; i++) {
        for (size_t j = 0; j < bm; j++) {
            if (a[ai + i] == b[bi + j]) {
                curr[j + 1] = prev[j] + 1;
                if (curr[j + 1] > best_size) {
                    best_size = curr[j + 1];
                    best_a    = ai + i + 1 - best_size;
                    best_b    = bi + j + 1 - best_size;
                }
            } else {
                curr[j + 1] = 0;
            }
        }
        std::swap(prev, curr);
        std::fill(curr.begin(), curr.end(), 0);
    }
    if (best_size == 0) {
        return 0;
    }
    size_t left  = voice_design_match(a, ai, best_a - ai, b, bi, best_b - bi);
    size_t right = voice_design_match(a, best_a + best_size, ai + am - (best_a + best_size), b, best_b + best_size,
                                      bi + bm - (best_b + best_size));
    return best_size + left + right;
}

// SequenceMatcher.ratio(): 2 * matched / (len(a) + len(b)).
static float voice_design_ratio(const std::string & a, const std::string & b) {
    if (a.empty() && b.empty()) {
        return 1.0f;
    }
    if (a.empty() || b.empty()) {
        return 0.0f;
    }
    size_t m = voice_design_match(a, 0, a.size(), b, 0, b.size());
    return 2.0f * (float) m / (float) (a.size() + b.size());
}

// Build all category sets and translation tables. Call once at startup.
static void voice_design_init(VoiceDesign * vd) {
    *vd = {};

    struct Pair {
        const char * en;
        const char * zh;
    };

    static const Pair gender[] = {
        { "male",   "男" },
        { "female", "女" }
    };
    static const Pair age[] = {
        { "child",       "儿童" },
        { "teenager",    "少年" },
        { "young adult", "青年" },
        { "middle-aged", "中年" },
        { "elderly",     "老年" }
    };
    static const Pair pitch[] = {
        { "very low pitch",  "极低音调" },
        { "low pitch",       "低音调"   },
        { "moderate pitch",  "中音调"   },
        { "high pitch",      "高音调"   },
        { "very high pitch", "极高音调" }
    };
    static const Pair style[] = {
        { "whisper", "耳语" }
    };
    static const char * accents[]  = { "american accent", "british accent", "australian accent", "chinese accent",
                                       "canadian accent", "indian accent",  "korean accent",     "portuguese accent",
                                       "russian accent",  "japanese accent" };
    static const char * dialects[] = { "河南话", "陕西话",   "四川话", "贵州话", "云南话", "桂林话",
                                       "济南话", "石家庄话", "甘肃话", "宁夏话", "青岛话", "东北话" };

    auto add_pairs = [vd](const Pair * arr, size_t n) {
        std::set<std::string> cat;
        for (size_t i = 0; i < n; i++) {
            vd->en_to_zh[arr[i].en] = arr[i].zh;
            vd->zh_to_en[arr[i].zh] = arr[i].en;
            vd->all_valid.insert(arr[i].en);
            vd->all_valid.insert(arr[i].zh);
            vd->valid_en.insert(arr[i].en);
            vd->valid_zh.insert(arr[i].zh);
            cat.insert(arr[i].en);
            cat.insert(arr[i].zh);
        }
        vd->mutually_exclusive.push_back(cat);
    };
    auto add_flat = [vd](const char * const * arr, size_t n, bool is_zh) {
        std::set<std::string> cat;
        for (size_t i = 0; i < n; i++) {
            vd->all_valid.insert(arr[i]);
            if (is_zh) {
                vd->valid_zh.insert(arr[i]);
            } else {
                vd->valid_en.insert(arr[i]);
            }
            cat.insert(arr[i]);
        }
        vd->mutually_exclusive.push_back(cat);
    };

    add_pairs(gender, sizeof(gender) / sizeof(gender[0]));
    add_pairs(age, sizeof(age) / sizeof(age[0]));
    add_pairs(pitch, sizeof(pitch) / sizeof(pitch[0]));
    add_pairs(style, sizeof(style) / sizeof(style[0]));
    add_flat(accents, sizeof(accents) / sizeof(accents[0]), false);
    add_flat(dialects, sizeof(dialects) / sizeof(dialects[0]), true);
}

// Whitespace strip (ASCII space and tab only, matching Python str.strip on
// the typical inputs we see).
static std::string voice_design_strip(const std::string & s) {
    size_t a = 0;
    size_t b = s.size();
    while (a < b && (s[a] == ' ' || s[a] == '\t')) {
        a++;
    }
    while (b > a && (s[b - 1] == ' ' || s[b - 1] == '\t')) {
        b--;
    }
    return s.substr(a, b - a);
}

// ASCII tolower, leaves Chinese bytes untouched (Chinese has no case).
static std::string voice_design_lower(const std::string & s) {
    std::string out = s;
    for (char & c : out) {
        if (c >= 'A' && c <= 'Z') {
            c = (char) (c + 32);
        }
    }
    return out;
}

// True if s ends with the suffix bytes (byte-wise).
static bool voice_design_ends_with(const std::string & s, const char * suffix) {
    size_t sn = s.size();
    size_t xn = strlen(suffix);
    if (sn < xn) {
        return false;
    }
    return memcmp(s.data() + sn - xn, suffix, xn) == 0;
}

// Validate and normalise an instruct string. Returns true with *out set on
// success (joined with ", " for English or "，" for Chinese). Returns false
// with *err set to a multi-line error message on failure. Empty input
// returns true with empty *out.
//
// use_zh selects the target language when neither dialect nor accent forces
// the choice. Caller passes true if the synthesis text contains any CJK.
static bool voice_design_normalize(const VoiceDesign * vd,
                                   const std::string & instruct,
                                   bool                use_zh,
                                   std::string *       out,
                                   std::string *       err) {
    out->clear();
    err->clear();

    std::string instruct_str = voice_design_strip(instruct);
    if (instruct_str.empty()) {
        return true;
    }

    // Split on half-width ',' or full-width '，' (UTF-8 EF BC 8C). Surrounding
    // whitespace is stripped from each item, matching the Python regex
    // r"\s*[,，]\s*".
    std::vector<std::string> raw_items;
    {
        size_t start = 0;
        size_t i     = 0;
        auto   flush = [&](size_t end) {
            std::string item = voice_design_strip(instruct_str.substr(start, end - start));
            if (!item.empty()) {
                raw_items.push_back(item);
            }
        };
        while (i < instruct_str.size()) {
            bool   is_sep = false;
            size_t skip   = 0;
            if (instruct_str[i] == ',') {
                is_sep = true;
                skip   = 1;
            } else if (i + 2 < instruct_str.size() && (uint8_t) instruct_str[i] == 0xEF &&
                       (uint8_t) instruct_str[i + 1] == 0xBC && (uint8_t) instruct_str[i + 2] == 0x8C) {
                is_sep = true;
                skip   = 3;
            }
            if (is_sep) {
                flush(i);
                start = i + skip;
                i     = start;
            } else {
                i++;
            }
        }
        flush(instruct_str.size());
    }

    // Validate each item, collect did-you-mean suggestions for unknowns.
    std::vector<std::string>                                       normalised;
    std::vector<std::tuple<std::string, std::string, std::string>> unknown;
    for (const auto & raw : raw_items) {
        std::string n = voice_design_lower(voice_design_strip(raw));
        if (vd->all_valid.count(n) > 0) {
            normalised.push_back(n);
        } else {
            std::string best;
            float       best_ratio = 0.6f;
            for (const auto & v : vd->all_valid) {
                float r = voice_design_ratio(n, v);
                if (r >= best_ratio) {
                    best       = v;
                    best_ratio = r;
                }
            }
            unknown.emplace_back(raw, n, best);
        }
    }

    if (!unknown.empty()) {
        std::string msg = "Unsupported instruct items found in " + instruct_str + ":\n";
        for (const auto & u : unknown) {
            const std::string & raw = std::get<0>(u);
            const std::string & n   = std::get<1>(u);
            const std::string & sug = std::get<2>(u);
            msg += "  '" + raw + "' -> '" + n + "' (unsupported";
            if (!sug.empty()) {
                msg += "; did you mean '" + sug + "'?";
            }
            msg += ")\n";
        }
        msg += "\nValid English items: ";
        bool first = true;
        for (const auto & v : vd->valid_en) {
            if (!first) {
                msg += ", ";
            }
            msg += v;
            first = false;
        }
        msg += "\nValid Chinese items: ";
        first = true;
        for (const auto & v : vd->valid_zh) {
            if (!first) {
                msg += "，";
            }
            msg += v;
            first = false;
        }
        msg +=
            "\n\nTip: Use only English or only Chinese instructs. "
            "English instructs should use comma + space (e.g. "
            "'male, indian accent'),\nChinese instructs should use full-width "
            "comma (e.g. '男，河南话').";
        *err = msg;
        return false;
    }

    // Language consistency: dialect forces Chinese, accent forces English.
    bool has_dialect = false;
    bool has_accent  = false;
    for (const auto & n : normalised) {
        if (voice_design_ends_with(n, "话")) {
            has_dialect = true;
        }
        if (n.find(" accent") != std::string::npos) {
            has_accent = true;
        }
    }
    if (has_dialect && has_accent) {
        *err =
            "Cannot mix Chinese dialect and English accent in a single instruct. "
            "Dialects are for Chinese speech, accents for English speech.";
        return false;
    }
    if (has_dialect) {
        use_zh = true;
    } else if (has_accent) {
        use_zh = false;
    }

    // Translate to the unified language.
    if (use_zh) {
        for (auto & n : normalised) {
            auto it = vd->en_to_zh.find(n);
            if (it != vd->en_to_zh.end()) {
                n = it->second;
            }
        }
    } else {
        for (auto & n : normalised) {
            auto it = vd->zh_to_en.find(n);
            if (it != vd->zh_to_en.end()) {
                n = it->second;
            }
        }
    }

    // Category conflict check: at most one item per mutually-exclusive set.
    std::vector<std::vector<std::string>> conflicts;
    for (const auto & cat : vd->mutually_exclusive) {
        std::vector<std::string> hits;
        for (const auto & n : normalised) {
            if (cat.count(n) > 0) {
                hits.push_back(n);
            }
        }
        if (hits.size() > 1) {
            conflicts.push_back(hits);
        }
    }
    if (!conflicts.empty()) {
        std::string msg     = "Conflicting instruct items within the same category: ";
        bool        first_g = true;
        for (const auto & group : conflicts) {
            if (!first_g) {
                msg += "; ";
            }
            bool first_x = true;
            for (const auto & x : group) {
                if (!first_x) {
                    msg += " vs ";
                }
                msg += "'" + x + "'";
                first_x = false;
            }
            first_g = false;
        }
        msg +=
            ". Each category (gender, age, pitch, style, accent, dialect) "
            "allows at most one item.";
        *err = msg;
        return false;
    }

    // Pick the separator from the language of the result.
    bool any_zh = false;
    for (const auto & n : normalised) {
        if (voice_design_has_cjk(n)) {
            any_zh = true;
            break;
        }
    }
    const char * sep = any_zh ? "，" : ", ";

    std::string result;
    for (size_t k = 0; k < normalised.size(); k++) {
        if (k > 0) {
            result += sep;
        }
        result += normalised[k];
    }
    *out = result;
    return true;
}
