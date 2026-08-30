// util.js — small shared helpers: number formatting, colors, canvas bootstrap.

export function fmt(x, nd = 3) {
  if (!isFinite(x)) return x > 0 ? '+∞' : (isNaN(x) ? 'NaN' : '−∞');
  if (Math.abs(x) >= 1e5 || (Math.abs(x) > 0 && Math.abs(x) < 1e-4)) return x.toExponential(2);
  return x.toFixed(nd);
}

export function esc(s) {
  return s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');
}

/** Size a canvas for devicePixelRatio; returns ctx. */
export function fitCanvas(cv) {
  const dpr = window.devicePixelRatio || 1;
  const r = cv.getBoundingClientRect();
  const w = Math.max(10, Math.round(r.width));
  // height priority: explicit inline style > height attr > live rect > 120
  let cssH = parseInt(cv.style.height, 10);
  if (!Number.isFinite(cssH) || cssH < 4) cssH = parseInt(cv.getAttribute('height') || '', 10);
  if (!Number.isFinite(cssH) || cssH < 4) cssH = Math.round(r.height);
  if (!Number.isFinite(cssH) || cssH < 4) cssH = 120;
  cv.width = Math.round(w * dpr);
  cv.height = Math.round(cssH * dpr);
  cv.style.height = `${cssH}px`;
  const ctx = cv.getContext('2d');
  ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
  return { ctx, w, h: cssH };
}

export const C = {
  cyan: '#35e0ff', cyanD: '#0e7d99', amber: '#ffb347', green: '#52ff7a',
  red: '#ff4757', magenta: '#ff5ad5', blue: '#4da3ff', yellow: '#ffe86e',
  dim: '#6d8294', faint: '#45596b', line: '#1b2a38', line2: '#24384c',
  panel: '#0d141c', txt: '#c8dce8', gridA: 'rgba(53,224,255,.05)',
};

export function drawGridBg(ctx, w, h, step = 26) {
  ctx.fillStyle = '#070d13';
  ctx.fillRect(0, 0, w, h);
  ctx.strokeStyle = 'rgba(36,60,82,.22)';
  ctx.lineWidth = 1;
  ctx.beginPath();
  for (let x = step; x < w; x += step) { ctx.moveTo(x + .5, 0); ctx.lineTo(x + .5, h); }
  for (let y = step; y < h; y += step) { ctx.moveTo(0, y + .5); ctx.lineTo(w, y + .5); }
  ctx.stroke();
}

/** invert viridis-ish: map t∈[0,1] to an industrial cyan→magenta ramp */
export function heatColor(t) {
  t = Math.min(1, Math.max(0, t));
  // piecewise: deep blue → cyan → yellow → magenta
  const stops = [
    [0.00, [16, 24, 39]],
    [0.25, [14, 90, 122]],
    [0.50, [53, 224, 255]],
    [0.75, [255, 232, 110]],
    [1.00, [255, 90, 213]],
  ];
  let i = 1;
  while (i < stops.length - 1 && stops[i][0] < t) i++;
  const [t0, c0] = stops[i - 1], [t1, c1] = stops[i];
  const u = (t - t0) / (t1 - t0 || 1);
  return `rgb(${Math.round(c0[0] + u * (c1[0] - c0[0]))},${Math.round(c0[1] + u * (c1[1] - c0[1]))},${Math.round(c0[2] + u * (c1[2] - c0[2]))})`;
}

/** token display text with visible control chars */
export function tokShow(s) {
  if (s === '') return '(空)';
  let t = s.replace(/\n/g, '⏎').replace(/\r/g, '\\r').replace(/\t/g, '⇥');
  if (t.startsWith(' ')) t = '␣' + t.slice(1);
  return t;
}
