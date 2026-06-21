import * as THREE from './three.module.js';

// ============ 纹理生成器 ============
class TextureGenerator {
    static SIZE = 128;

    static PATTERNS = {
        solid: { name: '纯色', icon: 'fa-square' },
        noise: { name: '噪点', icon: 'fa-braille' },
        grass: { name: '草地', icon: 'fa-leaf' },
        wood: { name: '木纹', icon: 'fa-tree' },
        brick: { name: '砖墙', icon: 'fa-th-large' },
        stone: { name: '石纹', icon: 'fa-mountain' },
        grid: { name: '方格', icon: 'fa-table' },
        stripes: { name: '条纹', icon: 'fa-grip-lines' },
    };

    static _adjustColor(hex, amount) {
        const r = parseInt(hex.slice(1, 3), 16);
        const g = parseInt(hex.slice(3, 5), 16);
        const b = parseInt(hex.slice(5, 7), 16);
        const adj = (c) => Math.max(0, Math.min(255, Math.round(c + amount * 255)));
        return `rgb(${adj(r)},${adj(g)},${adj(b)})`;
    }

    static generate(baseColor, pattern) {
        const size = this.SIZE;
        const canvas = document.createElement('canvas');
        canvas.width = canvas.height = size;
        const ctx = canvas.getContext('2d');

        ctx.fillStyle = baseColor;
        ctx.fillRect(0, 0, size, size);

        switch (pattern) {
            case 'noise': this._drawNoise(ctx, size, baseColor); break;
            case 'grass': this._drawGrass(ctx, size, baseColor); break;
            case 'wood': this._drawWood(ctx, size, baseColor); break;
            case 'brick': this._drawBrick(ctx, size, baseColor); break;
            case 'stone': this._drawStone(ctx, size, baseColor); break;
            case 'grid': this._drawGrid(ctx, size, baseColor); break;
            case 'stripes': this._drawStripes(ctx, size, baseColor); break;
        }

        const tex = new THREE.CanvasTexture(canvas);
        tex.magFilter = THREE.LinearFilter;
        tex.minFilter = THREE.LinearMipmapLinearFilter;
        tex.generateMipmaps = true;
        tex.colorSpace = THREE.SRGBColorSpace;
        return { texture: tex, canvas, base64: canvas.toDataURL('image/png') };
    }

    static _drawNoise(ctx, size, baseColor) {
        for (let i = 0; i < size; i++) {
            for (let j = 0; j < size; j++) {
                const n = (Math.random() - 0.5) * 0.15;
                ctx.fillStyle = this._adjustColor(baseColor, n);
                ctx.fillRect(i, j, 1, 1);
            }
        }
    }

    static _drawGrass(ctx, size, baseColor) {
        this._drawNoise(ctx, size, baseColor);
        const h = Math.floor(size / 8);
        for (let i = 0; i < size; i++) {
            const grassH = 2 + Math.floor(Math.random() * h);
            ctx.fillStyle = 'rgba(100,180,80,0.5)';
            ctx.fillRect(i, 0, 1, grassH);
        }
    }

    static _drawWood(ctx, size, baseColor) {
        this._drawNoise(ctx, size, baseColor);
        const cx = size / 2, cy = size / 2;
        for (let r = 3; r < size / 2; r += 3 + Math.random() * 4) {
            ctx.strokeStyle = `rgba(${80 + Math.random() * 40},${40 + Math.random() * 20},${20 + Math.random() * 15},0.5)`;
            ctx.lineWidth = 1 + Math.random();
            ctx.beginPath();
            ctx.arc(cx + (Math.random() - 0.5) * 4, cy + (Math.random() - 0.5) * 4, r, 0, Math.PI * 2);
            ctx.stroke();
        }
    }

    static _drawBrick(ctx, size, baseColor) {
        this._drawNoise(ctx, size, baseColor);
        const brickH = size / 4;
        const brickW = size / 4;
        ctx.strokeStyle = 'rgba(40,15,5,0.5)';
        ctx.lineWidth = 1;
        for (let y = 0; y < size; y += brickH) {
            ctx.beginPath(); ctx.moveTo(0, y); ctx.lineTo(size, y); ctx.stroke();
            const xOff = (Math.floor(y / brickH) % 2 === 0) ? 0 : brickW;
            for (let x = xOff; x < size; x += brickW * 2) {
                ctx.beginPath(); ctx.moveTo(x, y); ctx.lineTo(x, y + brickH); ctx.stroke();
            }
        }
    }

    static _drawStone(ctx, size, baseColor) {
        this._drawNoise(ctx, size, baseColor);
        ctx.strokeStyle = 'rgba(50,50,50,0.5)';
        ctx.lineWidth = 1;
        for (let i = 0; i < 8; i++) {
            const x1 = Math.random() * size, y1 = Math.random() * size;
            ctx.beginPath();
            ctx.moveTo(x1, y1);
            ctx.lineTo(x1 + (Math.random() - 0.5) * size * 0.6, y1 + (Math.random() - 0.5) * size * 0.6);
            ctx.stroke();
        }
    }

    static _drawGrid(ctx, size, baseColor) {
        this._drawNoise(ctx, size, baseColor);
        const step = size / 4;
        ctx.strokeStyle = 'rgba(0,0,0,0.15)';
        ctx.lineWidth = 1;
        for (let i = step; i < size; i += step) {
            ctx.beginPath(); ctx.moveTo(i, 0); ctx.lineTo(i, size); ctx.stroke();
            ctx.beginPath(); ctx.moveTo(0, i); ctx.lineTo(size, i); ctx.stroke();
        }
    }

    static _drawStripes(ctx, size, baseColor) {
        const stripeW = size / 8;
        for (let y = 0; y < size; y += stripeW * 2) {
            ctx.fillStyle = this._adjustColor(baseColor, -0.15);
            ctx.fillRect(0, y, size, stripeW);
        }
    }
}

export { TextureGenerator };