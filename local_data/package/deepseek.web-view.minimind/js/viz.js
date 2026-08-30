// viz.js — canvas visual components. All stateless helpers, UI keeps the state.

import { fitCanvas, drawGridBg, heatColor, C, fmt, tokShow } from './util.js';

/* ================= Pipeline flow diagram ================= */

export const FLOW_NODES = [
  { id: 'tok',    label: 'TOKENIZE\n分词 BPE',          col: 0, row: 0 },
  { id: 'embed',  label: 'EMBEDDING\n查表→768维',        col: 1, row: 0 },
  { id: 'ln1',    label: 'RMSNorm\n(输入层范数)',         col: 2, row: 0 },
  { id: 'qkv',    label: 'Q·K·V 投影\n(8头+4KV组)',      col: 3, row: 0 },
  { id: 'rope',   label: 'RoPE 旋转\n位置编码',           col: 4, row: 0 },
  { id: 'att',    label: '缩放点积注意力\nΣsoftmax(qk/√d)v', col: 5, row: 0 },
  { id: 'ores',   label: 'O投影+残差\nx += attn(x)',      col: 6, row: 0 },
  { id: 'ln2',    label: 'RMSNorm #2',                  col: 6, row: 1 },
  { id: 'ffn',    label: 'SwiGLU FFN\ngate⊙up→down',     col: 5, row: 1 },
  { id: 'res2',   label: '残差相加\nx += ffn(ln2)',       col: 4, row: 1 },
  { id: 'loop',   label: '×8 层循环\nL0 → L7',            col: 3, row: 1 },
  { id: 'fnorm',  label: 'Final Norm',                   col: 2, row: 1 },
  { id: 'head',   label: 'LM HEAD\n(embedding.T)',       col: 1, row: 1 },
  { id: 'dist',   label: '下一个 Token\n概率分布',          col: 0, row: 1 },
];
export const FLOW_TARGET = {
  tok: null, embed: 'scatter-e', ln1: 'calc', qkv: 'scatter-q',
  rope: 'calc', att: 'heatmap', ores: 'mlp', ln2: 'mlp', ffn: 'mlp',
  res2: null, loop: 'heatmap', fnorm: 'calc', head: 'cand', dist: 'cand',
};

export function drawFlow(cv, ui) {
  const { ctx, w, h } = fitCanvas(cv);
  drawGridBg(ctx, w, h);
  const cols = 7, rows = 2;
  const bw = Math.min(118, (w - 30) / cols - 10), bh = 62;
  const ox = (w - (cols * bw + (cols - 1) * 14)) / 2 + 4;
  const oy = h / 2 - bh - 9;
  const pos = {};
  for (const n of FLOW_NODES) {
    pos[n.id] = {
      x: ox + n.col * (bw + 14),
      y: oy + n.row * (bh + 22),
      w: bw, h: bh,
    };
  }
  // connections
  ctx.strokeStyle = 'rgba(53,224,255,.35)';
  ctx.lineWidth = 1.2;
  const chainTop = ['tok','embed','ln1','qkv','rope','att','ores'];
  const chainBot = ['ores','ln2','ffn','res2','loop','fnorm','head','dist'];
  ctx.setLineDash([]);
  for (let i = 0; i < chainTop.length - 1; i++) {
    const a = pos[chainTop[i]], b = pos[chainTop[i + 1]];
    arrow(ctx, a.x + a.w, a.y + a.h / 2, b.x, b.y + b.h / 2);
  }
  // downward connector ores→ln2
  arrow(ctx, pos.ores.x + pos.ores.w / 2, pos.ores.y + pos.ores.h,
              pos.ln2.x + pos.ln2.w / 2, pos.ln2.y);
  for (let i = 0; i < chainBot.length - 1; i++) {
    if (chainBot[i] === 'ores') continue;
    const a = pos[chainBot[i]], b = pos[chainBot[i + 1]];
    arrow(ctx, a.x - 0, a.y + a.h / 2, b.x + b.w, b.y + b.h / 2, true); // right-to-left
  }
  // layer loop arc from res2 back up to ln1 area
  ctx.save();
  ctx.strokeStyle = 'rgba(255,179,71,.55)';
  ctx.setLineDash([4, 3]);
  const l0 = pos.loop, t1 = pos.ln1;
  ctx.beginPath();
  ctx.moveTo(l0.x + l0.w / 2 - 20, l0.y);
  ctx.bezierCurveTo(l0.x + l0.w/2 - 60, l0.y - 40, t1.x + t1.w / 2 + 46, t1.y - 30, t1.x + t1.w / 2 + 24, t1.y + 12);
  ctx.stroke();
  ctx.restore();

  // active pulse for current generation stage
  const act = ui.flowActive;
  let pulseT = performance.now() % 1400 / 1400;

  for (const n of FLOW_NODES) {
    const p = pos[n.id];
    const isHot = act && act.nodes?.includes(n.id);
    ctx.fillStyle = isHot ? 'rgba(53,224,255,.16)' : '#0b141d';
    ctx.strokeStyle = isHot ? C.cyan : (act?.nodes?.length ? 'rgba(53,224,255,.18)' : C.line2);
    ctx.lineWidth = isHot ? 1.6 : 1;
    roundRect(ctx, p.x, p.y, p.w, p.h, 2);
    ctx.fill(); ctx.stroke();
    if (isHot) {
      ctx.strokeStyle = `rgba(53,224,255,${0.5 - 0.45*pulseT})`;
      ctx.lineWidth = 3;
      roundRect(ctx, p.x - 2, p.y - 2, p.w + 4, p.h + 4, 3);
      ctx.stroke();
    }
    ctx.fillStyle = isHot ? C.cyan : C.dim;
    ctx.font = '10px "JetBrains Mono", monospace';
    ctx.textAlign = 'center';
    const lines = n.label.split('\n');
    lines.forEach((s, i) => ctx.fillText(s, p.x + p.w / 2, p.y + 21 + i * 15));
    ctx.fillStyle = C.faint;
    ctx.font = '8px monospace';
    ctx.fillText(n.id.toUpperCase(), p.x + p.w / 2, p.y + p.h - 5);

    // hover hit-testing data stash
    p.node = n.id;
  }
  cv._flowPos = pos;

  function arrow(ctx, x1, y1, x2, y2, rtl = false) {
    ctx.beginPath();
    ctx.moveTo(x1, y1); ctx.lineTo(x2, y2);
    ctx.stroke();
    const dirX = rtl ? 1 : -1;
    ctx.beginPath();
    ctx.moveTo(x2, y2);
    ctx.lineTo(x2 + dirX * 5, y2 - 3.2);
    ctx.lineTo(x2 + dirX * 5, y2 + 3.2);
    ctx.closePath();
    ctx.fillStyle = 'rgba(53,224,255,.5)';
    ctx.fill();
  }
}

function roundRect(ctx, x, y, w, h, r) {
  ctx.beginPath();
  ctx.moveTo(x + r, y);
  ctx.arcTo(x + w, y, x + w, y + h, r);
  ctx.arcTo(x + w, y + h, x, y + h, r);
  ctx.arcTo(x, y + h, x, y, r);
  ctx.arcTo(x, y, x + w, y, r);
  ctx.closePath();
}

export function flowHitTest(cv, mx, my) {
  const pos = cv._flowPos;
  if (!pos) return null;
  for (const k of Object.keys(pos)) {
    const p = pos[k];
    if (mx >= p.x && mx <= p.x + p.w && my >= p.y && my <= p.y + p.h) return p.node;
  }
  return null;
}

/* ================= Dot-product dimension bars ================= */
// Inputs: q (hd), k (hd) arrays; highlight the pair (i,j)
export function drawDotBars(cv, q, k, opts = {}) {
  const { ctx, w, h } = fitCanvas(cv);
  drawGridBg(ctx, w, h);
  const hd = q.length;
  const prod = new Float32Array(hd);
  let dotSum = 0;
  for (let d = 0; d < hd; d++) { prod[d] = q[d] * k[d]; dotSum += prod[d]; }
  const mAbs = Math.max(1e-9, ...Array.from(prod).map(Math.abs));
  const padL = 30, padB = 26, topPad = 10;
  const bw = (w - padL - 8) / hd;
  const zeroY = topPad + (h - topPad - padB) / 2;
  const amp = (h - topPad - padB) / 2 - 2;
  ctx.font = '9px "JetBrains Mono",monospace';
  ctx.textAlign = 'right';
  ctx.fillStyle = C.faint;
  ctx.fillText('+' , padL - 4, topPad + 8);
  ctx.fillText('−' , padL - 4, h - padB - 4);
  ctx.strokeStyle = C.line;
  ctx.beginPath(); ctx.moveTo(padL, zeroY + .5); ctx.lineTo(w - 8, zeroY + .5); ctx.stroke();
  for (let d = 0; d < hd; d++) {
    const v = prod[d] / mAbs;
    const bx = padL + d * bw;
    const bh2 = Math.abs(v) * amp;
    const hl = opts.highlightDim === d;
    ctx.fillStyle = v >= 0
      ? (hl ? C.yellow : 'rgba(82,255,122,.72)')
      : (hl ? C.red : 'rgba(255,90,150,.66)');
    if (hl) { ctx.shadowColor = v>=0?C.yellow:C.red; ctx.shadowBlur=6; }
    ctx.fillRect(bx + .5, v >= 0 ? zeroY - bh2 : zeroY, Math.max(1, bw - 1.5), Math.max(bh2, .5));
    ctx.shadowBlur = 0;
  }
  // running total readout
  ctx.textAlign = 'right';
  ctx.fillStyle = C.txt;
  ctx.font = '11px "JetBrains Mono",monospace';
  ctx.fillText(`q·k = ${fmt(dotSum, 2)}`, w - 10, 16);
  if (opts.running != null) {
    ctx.fillStyle = C.green;
    ctx.fillText(`累计 ${opts.running} dims = ${fmt(opts.runningVal, 2)}`, w - 10, 30);
  }
  ctx.fillStyle = C.faint;
  ctx.font = '9px monospace';
  ctx.textAlign = 'left';
  ctx.fillText('dim → 0…95（96 维）', padL, h - 10);
  return dotSum;
}

/* ================= Softmax row distribution ================= */
export function drawSoftRow(cv, weights, hiIdx, labels, metaTxt = '') {
  const { ctx, w, h } = fitCanvas(cv);
  drawGridBg(ctx, w, h);
  const n = weights.length;
  const padL = 8, padR = 56, padT = 12, padB = 20;
  const maxW = Math.max(1e-9, ...weights);
  const bw = (w - padL - padR) / n;
  for (let i = 0; i < n; i++) {
    const v = weights[i] / maxW;
    const bh2 = v * (h - padT - padB);
    const x = padL + i * bw;
    const isHi = i === hiIdx;
    ctx.fillStyle = isHi ? C.amber : `rgba(53,224,255,${0.28 + 0.55 * v})`;
    if (isHi) { ctx.shadowColor = C.amber; ctx.shadowBlur = 8; }
    ctx.fillRect(x + 1, h - padB - bh2, Math.max(1.2, bw - 2), bh2);
    ctx.shadowBlur = 0;
  }
  ctx.fillStyle = C.faint; ctx.font='9px monospace'; ctx.textAlign='left';
  ctx.fillText('key token 索引 j →', padL + 2, h - 7);
  ctx.textAlign='right';
  ctx.fillStyle = C.txt; ctx.font='11px "JetBrains Mono",monospace';
  ctx.fillText(`P(key ${hiIdx}) = ${(weights[hiIdx]*100).toFixed(2)}%`, w-8, 14);
  if (metaTxt) { ctx.fillStyle=C.dim; ctx.font='9px monospace'; ctx.fillText(metaTxt, w-8, 26); }
}

/* ================= Weighted-sum V stack ================= */
// rows: contributions list [{txt, weight, vecScaledMaxAbs}], outVec final
export function drawSumViz(cv, vs, outHead, outFullBars, selIdx, metaTxt='') {
  const { ctx, w, h } = fitCanvas(cv);
  drawGridBg(ctx, w, h);
  const midY = Math.floor(h * 0.42);
  const segH = Math.min(13, (midY - 12) / vs.length);
  // contribution rows, width proportional to |w|
  const maxWv = Math.max(...vs.map(v => Math.abs(v.weight)), 1e-9);
  vs.forEach((vv, idx) => {
    const y = 8 + idx * (segH + 2);
    const frac = Math.abs(vv.weight) / maxWv;
    const wid = frac * (w - 210);
    const isSel = idx === selIdx;
    ctx.fillStyle = isSel ? C.amber : vv.color || 'rgba(77,163,255,.75)';
    if (isSel){ctx.shadowColor=C.amber;ctx.shadowBlur=7;}
    ctx.fillRect(64, y, Math.max(1,wid), segH);
    ctx.shadowBlur=0;
    ctx.strokeStyle = C.line2;
    ctx.strokeRect(64, y, Math.max(1,wid), segH);
    ctx.fillStyle = C.dim; ctx.font='9.5px "JetBrains Mono",monospace'; ctx.textAlign='left';
    ctx.fillText(`w${idx===selIdx?'★':''}=${vv.weight.toFixed(3)} · v[${vv.j}]`, 2, y + segH - 2.5);
    ctx.fillStyle = isSel?C.amber:C.faint;
    ctx.fillText(vv.txt.slice(0, 12), 66 + wid + 4, y + segH - 2.5);
  });
  // divider
  ctx.strokeStyle=C.line;ctx.beginPath();ctx.moveTo(8,midY+6);ctx.lineTo(w-8,midY+6);ctx.stroke();
  // output vector bars (outHead): centered bipolar bars per dim
  const botY = midY + 14;
  const barAreaH = h - botY - 6;
  const mAbs = Math.max(1e-9, ...Array.from(outFullBars).map(Math.abs));
  const bdw = (w - 16) / outFullBars.length;
  const zy = botY + barAreaH / 2;
  ctx.strokeStyle = C.line;
  ctx.beginPath(); ctx.moveTo(8, zy+.5); ctx.lineTo(w-8, zy+.5); ctx.stroke();
  for (let d = 0; d < outFullBars.length; d++) {
    const v = outFullBars[d]/mAbs;
    const bh2 = Math.abs(v)*barAreaH/2;
    ctx.fillStyle = v>=0?'rgba(82,255,122,.85)':'rgba(255,90,150,.8)';
    ctx.fillRect(8+d*bdw+0.5, v>=0?zy-bh2:zy, Math.max(1,bdw-1.4), Math.max(.6,bh2));
  }
  ctx.fillStyle=C.dim;ctx.font='9px monospace';ctx.textAlign='left';
  ctx.fillText('o_i = Σ_t w_t·v_t （96 维输出）', 10, botY - 2);
  if (metaTxt){ctx.textAlign='right';ctx.fillStyle=C.green;ctx.font='10px mono';ctx.fillText(metaTxt, w-8, botY-2);}
}

/* ================= Attention heatmap ================= */
export function drawHeatmap(cv, session, layer, head, tokens, allHeads, onClickCell, onHoverCell) {
  const { ctx, w, h } = fitCanvas(cv);
  drawGridBg(ctx, w, h);
  const pool = session.capPool;
  const T = tokens.length;
  if (!pool || T === 0) { placeholder(ctx, w, h, '等待预填数据'); return; }
  const headsToList = allHeads ? [0,1,2,3,4,5,6,7] : [head];
  const cell = Math.min((h - 26) / T, (w - 130) / (allHeads ? T : T));
  const cellS = Math.max(3, Math.floor(cell));
  const gridW = cellS * T, gridH = cellS * T;
  const ox = 120 + (w - 130 - gridW) / 2, oy = 6;
  ctx.font = '9px "JetBrains Mono",monospace';

  // column headers (keys)
  for (let j = 0; j < T; j += Math.ceil(T / Math.max(4, Math.floor(gridW/34)))) {
    ctx.save();
    ctx.translate(ox + j*cellS + cellS/2, oy - 3);
    ctx.rotate(-Math.PI/4);
    ctx.fillStyle = C.dim; ctx.textAlign='left';
    ctx.fillText(`${j}`, 0, 0);
    ctx.restore();
  }
  for (let hj = 0; hj < headsToList.length; hj++) {
    const hh = headsToList[hj];
    if (allHeads) ctx.globalAlpha = 1 / headsToList.length;
    const lay = session.capPool.layers[layer];
    for (let i = 0; i < T; i++) {
      const row = lay.attnRows[i]?.[hh];
      if (!row) continue;
      for (let j = 0; j < row.length; j++) {
        const v = row[j];
        const mx = ox + j*cellS, my = oy + i*cellS;
        ctx.fillStyle = heatColor(Math.pow(v, 0.42));
        ctx.fillRect(mx, my, cellS - (cellS>7?1:0), cellS - (cellS>7?1:0));
      }
      // masked upper triangle hatch (beyond row length)
      for (let j = row.length; j < T; j++) {
        const mx = ox + j*cellS, my = oy + i*cellS;
        ctx.fillStyle = 'rgba(10,16,23,.5)';
        ctx.fillRect(mx,my,cellS-(cellS>7?1:0),cellS-(cellS>7?1:0));
        if (cellS > 10 && (i+j)%4===0){
          ctx.strokeStyle='rgba(70,90,110,.25)';ctx.lineWidth=.6;
          ctx.beginPath();ctx.moveTo(mx,my+cellS);ctx.lineTo(mx+cellS,my);ctx.stroke();
        }
      }
    }
    ctx.globalAlpha = 1;
  }

  // axis chips: query labels left
  ctx.textAlign = 'right';
  for (let i = 0; i < T; i++) {
    if (i % Math.ceil(T / Math.max(6, Math.floor(gridH/26))) !== 0 && i !== T-1) continue;
    ctx.fillStyle = C.dim;
    ctx.fillText(`${i}·${tokens[i].short}`, ox - 6, oy + i*cellS + cellS/2 + 3);
  }
  // legend
  const lgY = oy + gridH + 14;
  gradBar(ctx, ox, lgY, 160, 8);
  ctx.fillStyle = C.dim; ctx.font='9px monospace'; ctx.textAlign='left';
  ctx.fillText('权重 0 → 最大', ox + 166, lgY + 8);

  cv._hmGeom = { ox, oy, cellS, T };

  cv.onclick = (ev) => {
    const r = cv.getBoundingClientRect();
    const mx = ev.clientX - r.left, my = ev.clientY - r.top;
    if (cv._hmGeom) {
      const {ox:g_ox, oy:g_oy, cellS:cs, T:tN} = cv._hmGeom;
      const j = Math.floor((mx-g_ox)/cs), i = Math.floor((my-g_oy)/cs);
      if (i>=0 && i<tN && j>=0 && j<tN && j<=i) onClickCell(i,j);
    }
  };
  cv.onmousemove = (ev) => {
    const r = cv.getBoundingClientRect();
    const mx = ev.clientX - r.left, my = ev.clientY - r.top;
    if (!cv._hmGeom) return;
    const {ox:g_ox, oy:g_oy, cellS:cs, T:tN} = cv._hmGeom;
    const j = Math.floor((mx-g_ox)/cs), i = Math.floor((my-g_oy)/cs);
    if (i>=0 && i<tN && j>=0 && j<=i && onHoverCell) {
      const row = session.capPool.layers[layer].attnRows[i]?.[head];
      onHoverCell({i, j, wgt: row ? row[j] : null});
    } else if (onHoverCell) onHoverCell(null);
  };
  cv.onmouseleave = () => onHoverCell && onHoverCell(null);
}

function gradBar(ctx,x,y,w,h){
  for(let i=0;i<w;i++){ ctx.fillStyle=heatColor(Math.pow(i/(w-1),0.42)); ctx.fillRect(x+i,y,1,h);}
}

function placeholder(ctx,w,h,msg){
  ctx.fillStyle=C.faint;ctx.font='12px monospace';ctx.textAlign='center';
  ctx.fillText(msg, w/2, h/2);
}

/* ================= QKV scatter ================= */
export function drawScatter(cv, pts, axes, hoverIdx) {
  const { ctx, w, h } = fitCanvas(cv);
  drawGridBg(ctx, w, h, 30);
  if (!pts || !pts.length) { placeholder(ctx,w,h,'无数据 —— 先运行预填'); return; }
  const pad = 34;
  let minX = Infinity,maxX=-Infinity,minY=Infinity,maxY=-Infinity;
  for (const p of pts) {
    minX=Math.min(minX,p.x);maxX=Math.max(maxX,p.x);
    minY=Math.min(minY,p.y);maxY=Math.max(maxY,p.y);
  }
  if (!isFinite(minX)) return;
  if (maxX-minX<1e-6){minX-=1;maxX+=1;}
  if (maxY-minY<1e-6){minY-=1;maxY+=1;}
  const spanX=maxX-minX,spanY=maxY-minY;
  const map=(p)=>[
    pad+(p.x-minX)/spanX*(w-pad-12),
    h-pad-(p.y-minY)/spanY*(h-pad-14),
  ];
  // axes crosshair at 0
  if (minX<0&&maxX>0){const[,yy]=map({x:0,y:minY});ctx.strokeStyle='rgba(109,130,148,.35)';ctx.beginPath();ctx.moveTo(pad,yy+.5);ctx.lineTo(w-12,yy+.5);ctx.stroke();}
  if (minY<0&&maxY>0){const[xx]=map({x:minX,y:0});ctx.strokeStyle='rgba(109,130,148,.35)';ctx.beginPath();ctx.moveTo(xx+.5,pad-10);ctx.lineTo(xx+.5,h-pad);ctx.stroke();}
  // points & stems
  const marks={Q:(x,y,s)=>{ctx.fillStyle=s||'rgba(53,224,255,.9)';ctx.fillRect(x-3,y-3,6,6);},
               K:(x,y)=>{ctx.beginPath();ctx.moveTo(x,y-4);ctx.lineTo(x+4,y);ctx.lineTo(x,y+4);ctx.lineTo(x-4,y);ctx.closePath();ctx.fillStyle='rgba(255,179,71,.92)';ctx.fill();},
               V:(x,y)=>{ctx.beginPath();ctx.arc(x,y,3.4,0,7);ctx.fillStyle='rgba(82,255,122,.9)';ctx.fill();}};
  for (let i=0;i<pts.length;i++){
    const p=pts[i];const[x,y]=map(p);
    if(hoverIdx===i){
      ctx.strokeStyle='rgba(255,232,110,.9)';
      ctx.setLineDash([2,2]);
      ctx.beginPath();ctx.moveTo(pad,y);ctx.lineTo(x,y);ctx.lineTo(x,h-pad);ctx.stroke();
      ctx.setLineDash([]);
    }
    (marks[p.type]||marks.Q)(x,y,i===hoverIdx?C.yellow:null);
    if(p.type==='Q'&&hoverIdx===i){ctx.strokeStyle='rgba(255,232,110,.65)';ctx.strokeRect(x-5.5,y-5.5,11,11);}
  }
  // legend
  ctx.font='10px monospace';ctx.textAlign='left';
  let lx=pad+2;
  const present=[...new Set(pts.map(p=>p.type))];
  for(const t of present){
    marks[t](lx+5,h-13,null);
    ctx.fillStyle=C.dim;ctx.fillText(t,lx+14,h-9);
    lx+=34;
  }
  ctx.fillStyle=C.faint;ctx.textAlign='right';
  ctx.fillText(`X: dim${axes.dxLabel}  Y: dim${axes.dyLabel}${axes.pca?' · PCA':''}`,w-10,h-9);
  ctx.fillText(`[${fmt(minX,2)}, ${fmt(maxX,2)}]`, w-10, 14);
  cv._scMap=map;
  cv._scPts=pts;
}

/* ================= probability curve (sorted full vocab) ================= */
export function drawProbCurve(cv, top, postFull, sampledId) {
  const { ctx, w, h } = fitCanvas(cv);
  drawGridBg(ctx, w, h);
  if (!postFull) return;
  const sorted = Array.from(postFull).sort((a,b)=>b-a).slice(0,300);
  const maxP = Math.max(sorted[0], 1e-9);
  ctx.strokeStyle='rgba(53,224,255,.85)';ctx.lineWidth=1.4;ctx.beginPath();
  for(let i=0;i<sorted.length;i++){
    const x=10+i/300*(w-20);
    const y=h-10-Math.log10(1+sorted[i]/maxP*999)/3*(h-20);
    i?ctx.lineTo(x,y):ctx.moveTo(x,y);
  }
  ctx.stroke();
  ctx.lineTo(w-10,h-10);ctx.lineTo(10,h-10);ctx.closePath();
  const g=ctx.createLinearGradient(0,0,0,h);g.addColorStop(0,'rgba(53,224,255,.30)');g.addColorStop(1,'rgba(53,224,255,.02)');
  ctx.fillStyle=g;ctx.fill();
  ctx.fillStyle=C.faint;ctx.font='8.5px monospace';ctx.textAlign='left';
  ctx.fillText('log-scale · 排序后的全词表后验分布(Top300)',12,12);
}
