import * as THREE from './three.module.js';

// ============ 图元定义 ============
const PRIMITIVES = {
    cube: {
        geo: (p) => new THREE.BoxGeometry(p.w || 1, p.h || 1, p.d || 1),
        icon: 'fa-cube', name: '立方体',
        params: [
            { key: 'w', label: '宽度', min: 0.1, max: 10, step: 0.1, default: 1 },
            { key: 'h', label: '高度', min: 0.1, max: 10, step: 0.1, default: 1 },
            { key: 'd', label: '深度', min: 0.1, max: 10, step: 0.1, default: 1 },
        ], multiFace: true
    },
    sphere: {
        geo: (p) => new THREE.SphereGeometry(p.r || 0.5, p.seg || 32, p.seg2 || 32),
        icon: 'fa-circle', name: '球体',
        params: [
            { key: 'r', label: '半径', min: 0.1, max: 10, step: 0.1, default: 0.5 },
            { key: 'seg', label: '分段', min: 4, max: 128, step: 1, default: 32 },
        ]
    },
    cylinder: {
        geo: (p) => new THREE.CylinderGeometry(p.rt || 0.5, p.rb || 0.5, p.h || 1, p.seg || 32),
        icon: 'fa-database', name: '圆柱体',
        params: [
            { key: 'rt', label: '顶半径', min: 0, max: 5, step: 0.1, default: 0.5 },
            { key: 'rb', label: '底半径', min: 0, max: 5, step: 0.1, default: 0.5 },
            { key: 'h', label: '高度', min: 0.1, max: 10, step: 0.1, default: 1 },
            { key: 'seg', label: '分段', min: 4, max: 128, step: 1, default: 32 },
        ], multiFace: ['侧面', '顶面', '底面']
    },
    cone: {
        geo: (p) => new THREE.ConeGeometry(p.r || 0.5, p.h || 1, p.seg || 32),
        icon: 'fa-traffic-cone', name: '圆锥体',
        params: [
            { key: 'r', label: '半径', min: 0.1, max: 5, step: 0.1, default: 0.5 },
            { key: 'h', label: '高度', min: 0.1, max: 10, step: 0.1, default: 1 },
            { key: 'seg', label: '分段', min: 4, max: 128, step: 1, default: 32 },
        ], multiFace: ['侧面', '底面']
    },
    plane: {
        geo: (p) => new THREE.PlaneGeometry(p.w || 1, p.h || 1),
        icon: 'fa-square', name: '平面',
        params: [
            { key: 'w', label: '宽度', min: 0.1, max: 20, step: 0.1, default: 1 },
            { key: 'h', label: '高度', min: 0.1, max: 20, step: 0.1, default: 1 },
        ]
    },
    torus: {
        geo: (p) => new THREE.TorusGeometry(p.r || 0.5, p.t || 0.2, p.rSeg || 16, p.tSeg || 32),
        icon: 'fa-donut', name: '圆环',
        params: [
            { key: 'r', label: '大半径', min: 0.1, max: 5, step: 0.1, default: 0.5 },
            { key: 't', label: '管半径', min: 0.02, max: 2, step: 0.02, default: 0.2 },
            { key: 'rSeg', label: '环分段', min: 4, max: 64, step: 1, default: 16 },
            { key: 'tSeg', label: '管分段', min: 4, max: 64, step: 1, default: 32 },
        ]
    },
    dodecahedron: {
        geo: (p) => new THREE.DodecahedronGeometry(p.r || 0.5, p.detail || 0),
        icon: 'fa-shapes', name: '十二面体',
        params: [
            { key: 'r', label: '半径', min: 0.1, max: 5, step: 0.1, default: 0.5 },
            { key: 'detail', label: '细节', min: 0, max: 3, step: 1, default: 0 },
        ]
    },
    octahedron: {
        geo: (p) => new THREE.OctahedronGeometry(p.r || 0.5, p.detail || 0),
        icon: 'fa-gem', name: '八面体',
        params: [
            { key: 'r', label: '半径', min: 0.1, max: 5, step: 0.1, default: 0.5 },
            { key: 'detail', label: '细节', min: 0, max: 3, step: 1, default: 0 },
        ]
    },
    tetrahedron: {
        geo: (p) => new THREE.TetrahedronGeometry(p.r || 0.5, p.detail || 0),
        icon: 'fa-play', name: '四面体',
        params: [
            { key: 'r', label: '半径', min: 0.1, max: 5, step: 0.1, default: 0.5 },
            { key: 'detail', label: '细节', min: 0, max: 3, step: 1, default: 0 },
        ]
    },
    torusKnot: {
        geo: (p) => new THREE.TorusKnotGeometry(p.r || 0.5, p.t || 0.15, p.tSeg || 64, p.rSeg || 8, p.p || 2, p.q || 3),
        icon: 'fa-infinity', name: '环结',
        params: [
            { key: 'r', label: '大半径', min: 0.1, max: 5, step: 0.1, default: 0.5 },
            { key: 't', label: '管半径', min: 0.02, max: 2, step: 0.02, default: 0.15 },
            { key: 'p', label: 'P 缠绕', min: 1, max: 10, step: 1, default: 2 },
            { key: 'q', label: 'Q 缠绕', min: 1, max: 10, step: 1, default: 3 },
        ]
    },
    ring: {
        geo: (p) => new THREE.RingGeometry(p.inner || 0.3, p.outer || 0.5, p.seg || 32),
        icon: 'fa-circle-notch', name: '圆环面',
        params: [
            { key: 'inner', label: '内半径', min: 0, max: 5, step: 0.05, default: 0.3 },
            { key: 'outer', label: '外半径', min: 0.1, max: 5, step: 0.05, default: 0.5 },
            { key: 'seg', label: '分段', min: 4, max: 128, step: 1, default: 32 },
        ]
    },
};

export { PRIMITIVES };