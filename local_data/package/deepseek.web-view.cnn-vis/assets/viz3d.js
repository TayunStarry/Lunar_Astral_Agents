/* =========================================================================
 * viz3d.js — 轻量正交投影 3D 渲染器 (零依赖, CAD 风格)
 *
 * 把网络每一层的特征图画成"带纹理的四边形面板", 沿网络流向(X轴)排布,
 * 支持鼠标拖拽旋转 / 滚轮缩放 / 点击选中(回调给 2D 详情面板)。
 * 正交投影下任意平面四边形都是平行四边形 -> 可用 canvas 仿射变换
 * 完美贴图, 无需 WebGL。
 * ========================================================================= */
(function (root, factory) {
  if (typeof module !== 'undefined' && module.exports) module.exports = factory();
  else root.Viz3D = factory();
})(typeof self !== 'undefined' ? self : this, function () {
  'use strict';

  const DEG = Math.PI / 180;
  const STAGE_GAP_COLORS = ['#58a6ff', '#ffb000', '#3fb950', '#ff7b72', '#bc8cff'];

  class Viz3D {
    constructor(canvas, opts = {}) {
      this.cv = canvas;
      this.ctx = canvas.getContext('2d');
      this.yaw = opts.yaw ?? -38;          // 绕Y轴角度
      this.pitch = opts.pitch ?? 24;       // 俯视角
      this.zoom = opts.zoom ?? 1.25;
      this.autoOrbit = opts.autoOrbit ?? true;
      this.groups = [];                    // [{title,x,color,items:[{tex,cw,ch,col,row,y,z,scale,id,gid,idx}]}]
      this.onSelect = null;
      this.hover = null;
      this.selected = null;                // {gid,idx}
      this._dirty = true;
      this._raf = null;

      canvas.addEventListener('pointerdown', e => this._down(e));
      canvas.addEventListener('pointermove', e => this._move(e));
      canvas.addEventListener('pointerup', e => this._up(e));
      canvas.addEventListener('pointerleave', () => { this.hover = null; this._dirty = true; });
      canvas.addEventListener('wheel', e => {
        e.preventDefault();
        this.zoom = Math.min(3.2, Math.max(0.45, this.zoom * (e.deltaY < 0 ? 1.09 : 0.92)));
        this._dirty = true;
      }, { passive: false });

      this._loop = this._loop.bind(this);
      requestAnimationFrame(this._loop);
    }

    /* spec: [{id,title,color,x,items:[{tex,cw,ch}],cols,itemScale}] */
    setScene(specs) {
      const groups = [];
      for (const sp of specs) {
        const g = { id: sp.id, title: sp.title, color: sp.color || '#8b949e',
                    x: sp.x, items: [] };
        const cols = sp.cols || Math.ceil(Math.sqrt(sp.items.length));
        const rows = Math.ceil(sp.items.length / cols);
        let maxY = -1e9, minY = 1e9, maxZ = -1e9;
        sp.items.forEach((it, idx) => {
          const col = idx % cols, row = (idx / cols) | 0;
          const s = (sp.itemScale || 1);
          const w = it.cw * s, h = it.ch * s;
          const z = (col - (cols - 1) / 2) * (w + 2.2);
          const y = ((rows - 1) / 2 - row) * (h + 2.2);
          g.items.push({ tex: it.tex, cw: it.cw, ch: it.ch,
                         y, z, hw: w / 2, hh: h / 2, gid: sp.id, idx });
          if (y + h > maxY) maxY = y + h;
          if (y - h < minY) minY = y - h;
          if (Math.abs(z) > maxZ) maxZ = Math.abs(z);
        });
        g.labelY = minY - 6;
        groups.push(g);
      }
      this.groups = groups;
      this._layout();
      this._dirty = true;
    }

    setSelected(gid, idx) {
      this.selected = (gid && idx != null) ? { gid, idx } : null;
      this._dirty = true;
    }

    resetView() {
      this.yaw = -38; this.pitch = 24; this.zoom = 1; this._dirty = true;
    }

    /* ---------------- 布局与投影 ---------------- */

    _layout() {
      // 计算整体包围盒用于居中
      let x0 = 1e9, x1 = -1e9, r = 0;
      for (const g of this.groups) {
        x0 = Math.min(x0, g.x); x1 = Math.max(x1, g.x);
        for (const it of g.items) {
          r = Math.max(r, Math.abs(it.z) + it.hw, Math.abs(it.y) + it.hh);
        }
      }
      this.worldW = (x1 - x0) || 1;
      this.xMid = (x0 + x1) / 2;
      this.radius = Math.max(r, this.worldW * 0.42);
    }

    _projRaw(x, y, z) {
      const t = this.yaw * DEG, p = this.pitch * DEG;
      const ct = Math.cos(t), st = Math.sin(t), cp = Math.cos(p), sp = Math.sin(p);
      const x1 = x * ct - z * st;
      const z1 = x * st + z * ct;
      const y2 = y * cp - z1 * sp;
      return { sx: x1, sy: -y2, depth: z1 * cp + y * sp };
    }

    _proj(x, y, z, W, H) {
      const S = this.zoom * Math.min(W / (this.worldW + 90), H / (this.radius * 2.6));
      const q = this._projRaw(x - this.xMid, y, z);
      return { x: W / 2 + q.sx * S, y: H / 2 + 18 + q.sy * S, d: q.depth };
    }

    /* ---------------- 交互 ---------------- */

    _down(e) {
      this.autoOrbit = false;
      this._drag = { x: e.clientX, y: e.clientY, yaw: this.yaw, pitch: this.pitch, moved: false };
      this.cv.setPointerCapture(e.pointerId);
    }

    _pick(px, py) {
      // 命中检测: 对每个面板做逆仿射
      for (const g of this.groups) {
        for (const it of g.items) {
          if (!it._corners) continue;
          const [P00, P10, P01] = it._corners;
          const vx1 = P10.x - P00.x, vy1 = P10.y - P00.y;
          const vx2 = P01.x - P00.x, vy2 = P01.y - P00.y;
          const det = vx1 * vy2 - vx2 * vy1;
          if (Math.abs(det) < 1e-6) continue;
          const dx = px - P00.x, dy = py - P00.y;
          const u = (dx * vy2 - dy * vx2) / det;
          const v = (vx1 * dy - vy1 * dx) / det;
          if (u >= 0 && v >= 0 && u <= 1 && v <= 1) return { g, it };
        }
      }
      return null;
    }

    _move(e) {
      const rect = this.cv.getBoundingClientRect();
      const px = e.clientX - rect.left, py = e.clientY - rect.top;
      if (this._drag) {
        const dx = e.clientX - this._drag.x, dy = e.clientY - this._drag.y;
        if (Math.abs(dx) + Math.abs(dy) > 3) this._drag.moved = true;
        this.yaw = Math.max(-85, Math.min(30, this._drag.yaw + dx * 0.4));
        this.pitch = Math.max(4, Math.min(55, this._drag.pitch + dy * 0.3));
        this._dirty = true;
      } else {
        const hit = this._pick(px, py);
        const id = hit ? hit.it.gid + ':' + hit.it.idx : null;
        if (id !== this.hover) { this.hover = id; this._dirty = true; }
        this.cv.style.cursor = hit ? 'pointer' : 'grab';
      }
    }

    _up(e) {
      if (this._drag && !this._drag.moved) {
        const rect = this.cv.getBoundingClientRect();
        const hit = this._pick(e.clientX - rect.left, e.clientY - rect.top);
        if (hit) {
          this.selected = { gid: hit.it.gid, idx: hit.it.idx };
          if (this.onSelect) this.onSelect(hit.it.gid, hit.it.idx);
          this._dirty = true;
        }
      }
      this._drag = null;
    }

    /* ---------------- 渲染 ---------------- */

    _loop() {
      if (this.autoOrbit) {
        this.yaw = -38 + Math.sin(Date.now() / 2600) * 16;
        this._dirty = true;
      }
      if (this._dirty) { this._render(); this._dirty = false; }
      requestAnimationFrame(this._loop);
    }

    invalidate() { this._dirty = true; }

    _render() {
      const cv = this.cv, ctx = this.ctx;
      const W = cv.clientWidth, H = cv.clientHeight;
      const dpr = window.devicePixelRatio || 1;
      if (cv.width !== W * dpr || cv.height !== H * dpr) {
        cv.width = W * dpr; cv.height = H * dpr;
      }
      ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
      ctx.clearRect(0, 0, W, H);

      this._drawGrid(ctx, W, H);

      // 收集面板并按深度排序 (画家算法)
      const quads = [];
      for (const g of this.groups) {
        for (const it of g.items) {
          const P00 = this._proj(g.x, it.y + it.hh, it.z - it.hw, W, H);
          const P10 = this._proj(g.x, it.y + it.hh, it.z + it.hw, W, H);
          const P11 = this._proj(g.x, it.y - it.hh, it.z + it.hw, W, H);
          const P01 = this._proj(g.x, it.y - it.hh, it.z - it.hw, W, H);
          it._corners = [P00, P10, P01];
          quads.push({ g, it, P00, P10, P11, P01,
                       depth: (P00.d + P11.d) / 2 });
        }
      }
      quads.sort((a, b) => a.depth - b.depth);

      ctx.imageSmoothingEnabled = false;
      for (const q of quads) this._drawQuad(ctx, q);

      // 组标签
      ctx.font = '600 11px ui-monospace,Menlo,Consolas,monospace';
      ctx.textAlign = 'center';
      for (const g of this.groups) {
        const p = this._proj(g.x, g.labelY, 0, W, H);
        ctx.fillStyle = g.color;
        ctx.fillText(g.title, p.x, p.y);
      }
      ctx.textAlign = 'left';
    }

    _drawGrid(ctx, W, H) {
      const S = 12;
      const yFloor = -this.radius * 1.05;
      ctx.lineWidth = 1;
      ctx.strokeStyle = 'rgba(120,140,160,.08)';
      ctx.beginPath();
      // 两族网格线: 平行流向 / 垂直流向
      for (let i = -6; i <= 6; i++) {
        const z = i * S;
        const a = this._proj(this.xMid - this.worldW / 2 - 20, yFloor, z, W, H);
        const b = this._proj(this.xMid + this.worldW / 2 + 20, yFloor, z, W, H);
        ctx.moveTo(a.x, a.y); ctx.lineTo(b.x, b.y);
      }
      for (let gx = -20; gx <= this.worldW + 20; gx += S * 2) {
        const x = this.xMid - this.worldW / 2 - 20 + gx;
        const a = this._proj(x, yFloor, -S * 6, W, H);
        const b = this._proj(x, yFloor, S * 6, W, H);
        ctx.moveTo(a.x, a.y); ctx.lineTo(b.x, b.y);
      }
      ctx.stroke();
    }

    _drawQuad(ctx, q) {
      const { P00, P10, P11, P01, it, g } = q;
      const sel = this.selected && this.selected.gid === it.gid && this.selected.idx === it.idx;
      const hov = this.hover === it.gid + ':' + it.idx;
      const cw = it.cw, chh = it.ch;
      const m11x = (P10.x - P00.x) / cw, m11y = (P10.y - P00.y) / cw;
      const m21x = (P01.x - P00.x) / chh, m21y = (P01.y - P00.y) / chh;
      ctx.save();
      ctx.transform(m11x, m11y, m21x, m21y, P00.x, P00.y);
      ctx.drawImage(it.tex, 0, 0);
      ctx.restore();
      // 边框 (屏幕空间画, 线宽恒定)
      ctx.strokeStyle = sel ? '#ffffff' : hov ? '#ffb000'
        : (it.idx === 0 ? g.color : 'rgba(140,155,170,.35)');
      ctx.lineWidth = sel ? 2 : hov ? 1.5 : 1;
      ctx.beginPath();
      ctx.moveTo(P00.x, P00.y); ctx.lineTo(P10.x, P10.y);
      ctx.lineTo(P11.x, P11.y); ctx.lineTo(P01.x, P01.y);
      ctx.closePath(); ctx.stroke();
    }
  }

  return { Viz3D, STAGE_GAP_COLORS };
});
