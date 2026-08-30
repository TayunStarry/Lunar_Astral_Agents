// tok.js — Byte-level BPE tokenizer (Qwen2/minimind family), pure JS.
// Replicates the fast-tokenizer pretokenization behaviour empirically validated
// against tools/tok_cases.json (62 adversarial cases).

// ---------- byte <-> unicode mapping (GPT-2 style, all 256 bytes printable) ----------
const BYTE_ENCODER = (() => {
  const bs = [];
  for (let b = 33; b < 127; b++) bs.push(b);
  for (let b = 161; b < 173; b++) bs.push(b);
  for (let b = 174; b < 256; b++) bs.push(b);
  const cs = bs.slice();
  let n = 0;
  for (let b = 0; b < 256; b++) {
    if (!bs.includes(b)) { bs.push(b); cs.push(256 + n); n++; }
  }
  const enc = new Array(256);
  for (let i = 0; i < 256; i++) enc[bs[i]] = String.fromCharCode(cs[i]);
  return enc;
})();
const BYTE_DECODER = (() => {
  const m = new Array(65536);
  for (let b = 0; b < 256; b++) m[BYTE_ENCODER[b].charCodeAt(0)] = b;
  return m;
})();

const encoder_ = new TextEncoder();
const decoder_ = new TextDecoder('utf-8', { fatal: false });

function isCJKPunctClass(cp) { /* unused helper placeholder */ return false; }

// char classes used by the pretokenizer
function isLetterCp(cp) {
  // Close enough approximation of \p{L}: relies on native RegExp property classes.
  return LETTER_RE.test(String.fromCharCode(cp));
}
function isDigitCp(cp) {
  return DIGIT_RE.test(String.fromCharCode(cp));
}
const LETTER_RE = /^\p{L}$/u;
const DIGIT_RE = /^\p{N}$/u;
const WS_RE = /^\s$/u;

export class BPETokenizer {
  constructor(tokenizerJson) {
    const tj = typeof tokenizerJson === 'string'
      ? JSON.parse(tokenizerJson) : tokenizerJson;
    this.vocabSize = Object.keys(tj.model.vocab).length;
    this.tokToId = new Map();        // bpe token string -> id
    for (const [t, id] of Object.entries(tj.model.vocab)) this.tokToId.set(t, id);
    this.idToTok = new Array(this.vocabSize);
    for (const [t, id] of this.tokToId) this.idToTok[id] = t;

    this.ranks = new Map();          // "a\u0000b" -> rank
    tj.model.merges.forEach((m, i) => {
      const parts = Array.isArray(m) ? m : m.split(' ');
      this.ranks.set(parts[0] + '\u0000' + parts[1], i);
    });

    this.special = [];               // [{id, text}] sorted longest-first
    this.specialById = new Map();
    for (const at of tj.added_tokens || []) {
      this.special.push({ id: at.id, text: at.content });
      this.specialById.set(at.id, at.content);
    }
    this.special.sort((a, b) => b.text.length - a.text.length);
    // special content -> id for encoding convenience
    this.specialTextId = new Map(this.special.map(s => [s.text, s.id]));
  }

  static async load(url) {
    const res = await fetch(url);
    if (!res.ok) throw new Error(`HTTP ${res.status} fetching ${url}`);
    return new BPETokenizer(await res.json());
  }

  /**
   * Split text into pretokens, faithfully replicating the Rust ByteLevel regex
   *   's|'t|'re|'ve|'m|'ll|'d | ?\p{L}+ | ?\p{N}+ | ?[^\s\p{L}\p{N}]+ |\s+(?!\S)|\s+
   * (case-sensitive contractions, optional single literal space, backtracking
   *  \s+(?!\S) which strips the final whitespace of an interior run).
   */
  pretokenize(text) {
    const arr = Array.from(text);
    const segs = [];
    const cplen = arr.length;
    const isLet = c => LETTER_RE.test(c);
    const isDig = c => DIGIT_RE.test(c);
    const isWs = c => WS_RE.test(c);
    const isPunct = c => !isWs(c) && !isLet(c) && !isDig(c);
    let p = 0;
    while (p < cplen) {
      const start = p;
      let consumed = false;
      // 1. contractions (lowercase only, straight after a literal apostrophe)
      if (arr[p] === "'" && p + 1 < cplen && !isWs(arr[p + 1])) {
        const three = arr.slice(p + 1, p + 4).join('');
        for (const suf of ['s', 't', 're', 've', 'm', 'll', 'd']) {
          if (three.startsWith(suf)) { p += suf.length + 1; consumed = true; break; }
        }
      }
      // 2./3./4.: optional single literal ' ' + class run
      if (!consumed) {
        for (const cls of [isLet, isDig, isPunct]) {
          let q = p;
          if (arr[q] === ' ') q++;
          if (q < cplen && cls(arr[q])) {
            while (q < cplen && cls(arr[q])) q++;
            p = q; consumed = true; break;
          }
        }
      }
      // 5. \s+(?!\S) — longest ws prefix whose successor char is whitespace or EOS
      if (!consumed) {
        let r = p;
        while (r < cplen && isWs(arr[r])) r++;
        if (r > p) {
          let L = r - p;
          if (r === cplen) {                  // touches end-of-input: take all
            p = r; segs.push([start, p]); continue;
          }
          while (L >= 1 && !isWs(arr[p + L])) L--;   // backtrack until ws boundary
          if (L >= 1) { p += L; segs.push([start, p]); continue; }
        }
      }
      // 6. \s+ fallback
      if (!consumed) {
        let r = p;
        while (r < cplen && isWs(arr[r])) r++;
        p = Math.max(r, p + 1);
      }
      segs.push([start, Math.max(p, start + 1)]);
    }
    return segs.map(([a, b]) => arr.slice(a, b).join(''));
  }

  /** GPT-2 style BPE over one pretoken (already byte-mapped). */
  bpe(word) {
    if (word.length <= 1) return [word];
    let syms = Array.from(word);
    for (;;) {
      let bestRank = Infinity, bestI = -1;
      for (let i = 0; i < syms.length - 1; i++) {
        const r = this.ranks.get(syms[i] + '\u0000' + syms[i + 1]);
        if (r !== undefined && r < bestRank) { bestRank = r; bestI = i; }
      }
      if (bestI < 0) break;
      const merged = syms[bestI] + syms[bestI + 1];
      syms.splice(bestI, 2, merged);
    }
    return syms;
  }

  encode(text) {
    // Special tokens are matched literally, longest first.
    const outIds = [];
    let i = 0;
    outer: while (i < text.length) {
      for (const sp of this.special) {
        if (sp.text.length && text.startsWith(sp.text, i)) {
          outIds.push(sp.id);
          i += sp.text.length;
          continue outer;
        }
      }
      // find run until next possible special
      let j = i + 1;
      inner: while (j < text.length) {
        for (const sp of this.special) {
          if (sp.text.length && text.startsWith(sp.text, j)) break inner;
        }
        j++;
      }
      this._encodeSegment(text.slice(i, j), outIds);
      i = j;
    }
    return outIds;
  }

  _encodeSegment(seg, outIds) {
    if (!seg) return;
    // Byte-level BPE is applied independently per pretoken.
    const enc = new TextEncoder();
    for (const piece of this.pretokenize(seg)) {
      const bytes = encoder_.encode(piece);
      let mapped = '';
      for (let i = 0; i < bytes.length; i++) mapped += BYTE_ENCODER[bytes[i]];
      for (const sym of this.bpe(mapped)) {
        const id = this.tokToId.get(sym);
        if (id !== undefined) outIds.push(id);
        else {
          // fallback: encode byte-by-byte
          for (const ch of sym) {
            const id2 = this.tokToId.get(ch);
            if (id2 !== undefined) outIds.push(id2);
          }
        }
      }
    }
  }

  /** Decode ids -> text (byte-level round trip; specials render literally). */
  decode(ids, skipSpecial = false) {
    const buf = [];
    for (const id of ids) {
      const spText = this.specialById.get(id);
      if (spText !== undefined) {
        if (!skipSpecial) buf.push(spText);
        continue;
      }
      const tok = this.idToTok[id];
      if (tok === undefined) continue;
      for (let k = 0; k < tok.length; k++) {
        const b = BYTE_DECODER[tok.charCodeAt(k)];
        if (b !== undefined) buf.push(b);
      }
    }
    return decoder_.decode(new Uint8Array(buf));
  }

  pieceRepr(id) {
    const t = this.decode([id]);
    return t === '' ? '' : t;
  }
}
