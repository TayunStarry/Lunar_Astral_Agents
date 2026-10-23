/**
 * 实时资讯 · 配置与查询服务层
 * 负责读取 config.json、按策略调度 weather-providers.js 的数据源，
 * 并归一化 60s.7se.cn 的新闻数据。对外暴露 fetchWeather / fetchNews / queryWeather。
 */

// ===== 配置 =====

/** 本模块配置加载 Promise（config.json → config.example.json → 默认值） */
let swConfigPromise = null;

/** 默认配置：与 com.ltp9-flash.weather 的 config.yaml 字段保持一致 */
function swDefaultConfig() {
    return {
        enabled: true,
        provider: 'auto',
        qweatherAuth: 'api_key',
        qweatherHost: '',
        qweatherKey: '',
        qweatherProjectID: '',
        qweatherKeyID: '',
        qweatherPrivateKey: '',
        defaultCity: '',
        showAQI: true,
        newsEnabled: true
    };
}

/** 把 config.json 的嵌套结构归一化为扁平配置 */
function normalizeSWConfig(raw) {
    const cfg = swDefaultConfig();
    if (!raw || typeof raw !== 'object') return cfg;
    const api = raw.api || {};
    const display = raw.display || {};
    const plugin = raw.plugin || {};
    const news = raw.news || {};

    if (plugin.enabled !== undefined) cfg.enabled = !!plugin.enabled;
    if (api.provider) cfg.provider = String(api.provider).trim().toLowerCase();
    if (api.qweather_auth) cfg.qweatherAuth = String(api.qweather_auth).trim().toLowerCase();
    cfg.qweatherHost = String(api.qweather_host || '').trim();
    cfg.qweatherKey = String(api.qweather_key || '').trim();
    cfg.qweatherProjectID = String(api.qweather_project_id || '').trim();
    cfg.qweatherKeyID = String(api.qweather_key_id || '').trim();
    cfg.qweatherPrivateKey = String(api.qweather_private_key || '').trim();
    cfg.defaultCity = String(display.default_city || '').trim();
    if (display.show_aqi !== undefined) cfg.showAQI = !!display.show_aqi;
    if (news.enabled !== undefined) cfg.newsEnabled = !!news.enabled;
    return cfg;
}

/**
 * 读取本模块配置：优先 config.json（本地私有，已 gitignore），
 * 其次 config.example.json（占位符，可保证免费源仍可用），最后默认值。
 * @returns {Promise<Object>}
 */
async function loadSWConfig() {
    if (swConfigPromise) return swConfigPromise;
    swConfigPromise = (async () => {
        for (const name of ['config.json', 'config.example.json']) {
            try {
                const resp = await fetch(name + '?t=' + Date.now(), { cache: 'no-store' });
                if (!resp.ok) continue;
                return normalizeSWConfig(await resp.json());
            } catch (e) {
                // 尝试下一个配置文件
            }
        }
        console.warn('[实时资讯] 未读取到配置文件，使用默认配置（和风天气不可用，免费源可用）');
        return swDefaultConfig();
    })();
    return swConfigPromise;
}

// ===== 天气调度 =====

/**
 * 按策略查询天气：auto / qweather / openmeteo / wttr
 * @param {string} city
 * @param {string} [provider] 为空时使用 config.api.provider
 * @returns {Promise<{ok:boolean, data?:Object, error?:string}>}
 */
async function queryWeather(city, provider) {
    const cfg = await loadSWConfig();
    if (!cfg.enabled) return { ok: false, error: '实时资讯模块未启用（config.json: plugin.enabled）' };

    const targetCity = String(city || '').trim() || cfg.defaultCity;
    if (!targetCity) return { ok: false, error: '请提供城市名称，例如：北京、上海、深圳' };

    const strategy = String(provider || cfg.provider || 'auto').trim().toLowerCase();
    const errors = [];

    async function run(name, fn) {
        try {
            const data = await fn(targetCity);
            data.provider = name;
            // strategy 记录实际命中的数据源；显式请求与回退结果不同时保留 requested
            data.strategy = name;
            if (strategy !== name && strategy !== 'auto') data.requested = strategy;
            return { ok: true, data: data };
        } catch (e) {
            const message = (e && e.message) ? e.message : String(e);
            errors.push(PROVIDER_LABELS[name] + '：' + message);
            console.warn('[实时资讯] ' + name + ' 查询失败:', e);
            return null;
        }
    }

    if (strategy === 'qweather') {
        if (qweatherAvailable(cfg)) {
            const result = await run('qweather', c => queryQWeather(c, cfg));
            if (result) return result;
        } else {
            errors.push('和风天气：' + qweatherUnavailableReason(cfg));
        }
        const openMeteo = await run('openmeteo', queryOpenMeteo);
        if (openMeteo) return openMeteo;
        const wttr = await run('wttr', queryWttr);
        if (wttr) return wttr;
    } else if (strategy === 'openmeteo') {
        const openMeteo = await run('openmeteo', queryOpenMeteo);
        if (openMeteo) return openMeteo;
        const wttr = await run('wttr', queryWttr);
        if (wttr) return wttr;
    } else if (strategy === 'wttr') {
        const wttr = await run('wttr', queryWttr);
        if (wttr) return wttr;
    } else if (strategy === 'auto') {
        if (qweatherAvailable(cfg)) {
            const result = await run('qweather', c => queryQWeather(c, cfg));
            if (result) return result;
        }
        const openMeteo = await run('openmeteo', queryOpenMeteo);
        if (openMeteo) return openMeteo;
        const wttr = await run('wttr', queryWttr);
        if (wttr) return wttr;
    } else {
        return { ok: false, error: '无效的天气数据源策略：' + strategy };
    }

    return { ok: false, error: '所有天气服务暂不可用：' + errors.join('；') };
}

/**
 * 查询指定城市天气（手动查询入口，默认 auto）
 * @param {string} city
 * @param {string} [provider]
 * @returns {Promise<Object>} 结构化天气数据（失败时抛出异常）
 */
async function fetchWeather(city, provider) {
    const result = await queryWeather(city, provider || 'auto');
    if (!result.ok) throw new Error(result.error);
    return result.data;
}

// ===== 新闻数据源 =====

/**
 * 获取当日新闻摘要（60s.7se.cn）
 * 兼容接口新旧结构：
 *   新：{ time, title, data: string[], info_text, url }
 *   旧：{ data: { date, news: string[] } }
 * @returns {Promise<Object>} { date, title, news, url, source }
 */
async function fetchNews() {
    const cfg = await loadSWConfig();
    if (!cfg.newsEnabled) throw new Error('新闻查询已在 config.json 中关闭（news.enabled=false）');

    const data = await fetchJSON('https://60s.7se.cn/v2/60s');
    const pick = value => Array.isArray(value)
        ? value.map(item => String(item || '').trim()).filter(Boolean)
        : null;

    if (data && data.status !== undefined && String(data.status) !== '200') {
        throw new Error('新闻服务返回异常：' + (data.msg || data.status));
    }

    // 新结构：data 直接是新闻字符串数组
    const fresh = pick(data && data.data);
    if (fresh && fresh.length > 0) {
        return {
            date: data.time || data.info_text || '',
            title: data.title || '今日要闻',
            news: fresh,
            url: data.url || '',
            source: '60s.7se.cn'
        };
    }

    // 旧结构：data.data.news
    const legacy = data && data.data && pick(data.data.news);
    if (legacy && legacy.length > 0) {
        return {
            date: data.data.date || data.time || '',
            title: data.title || '今日要闻',
            news: legacy,
            url: data.data.url || data.url || '',
            source: '60s.7se.cn'
        };
    }

    // 兜底：直接数组 / { news: [] }
    const direct = pick(data) || pick(data && data.news);
    if (direct && direct.length > 0) {
        return {
            date: (data && data.time) || '',
            title: (data && data.title) || '今日要闻',
            news: direct,
            url: (data && data.url) || '',
            source: '60s.7se.cn'
        };
    }

    throw new Error('新闻数据解析失败（接口结构可能已变更）');
}

// ===== 文本总结（回填给模型） =====

function weatherLabel(data) {
    return PROVIDER_LABELS[data.provider] || data.source || data.provider || '天气服务';
}

/** 把结构化天气数据转成给模型看的紧凑文本 */
function generateWeatherText(data) {
    const c = data.current || {};
    const lines = [];
    const fallback = data.requested ? '，已从 ' + data.requested + ' 回退' : '';
    lines.push('【' + data.city + '】当前天气（' + weatherLabel(data) + ' · 策略 ' + (data.strategy || 'auto') + fallback + '）：');
    lines.push('- 气温：' + swInt(c.temperature) + '°C（体感 ' + swInt(c.feels_like) + '°C）');
    lines.push('- 天气状况：' + (c.condition || '未知'));
    lines.push('- 湿度：' + swInt(c.humidity) + '%');
    lines.push('- 风力：' + (c.wind || '-'));
    if (c.visibility) lines.push('- 能见度：' + c.visibility + 'km');
    if (data.aqi) {
        lines.push('- 空气质量：' + (data.aqi.category || data.aqi.level || '未知') +
            '（AQI ' + swInt(data.aqi.value) +
            (data.aqi.primary ? '，主要污染物 ' + data.aqi.primary : '') + '）');
    }
    (data.forecast || []).forEach(f => {
        let line = '- 预报 ' + f.date + '：' + (f.condition || '未知') +
            '，' + swInt(f.low) + '°C ~ ' + swInt(f.high) + '°C';
        if (f.precip_prob !== null && f.precip_prob !== undefined) {
            line += '，降水概率 ' + swInt(f.precip_prob) + '%';
        }
        lines.push(line);
    });
    return lines.join('\n');
}

/** 把新闻数据转成给模型看的紧凑文本 */
function generateNewsText(data) {
    const lines = ['【' + (data.date || '今日') + '】' + (data.title || '当日要闻') +
        '（共 ' + data.news.length + ' 条）：'];
    data.news.forEach((item, index) => lines.push((index + 1) + '. ' + item));
    if (data.source) lines.push('来源：' + data.source);
    return lines.join('\n');
}
