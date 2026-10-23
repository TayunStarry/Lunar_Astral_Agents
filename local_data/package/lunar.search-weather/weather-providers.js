/**
 * 实时资讯 · 天气数据源层
 * 负责 wttr.in / Open-Meteo / 和风天气 三种提供者的取数与归一化。
 * 不读取配置、不渲染界面，供 weather-service.js 与智能体工具层调用。
 */

// ===== 通用工具 =====

/** 安全取整（空值/非法值回退 0） */
function swInt(value) {
    const n = parseFloat(value);
    return Number.isFinite(n) ? Math.round(n) : 0;
}

/** 带超时的 JSON 请求（浏览器 fetch，20 秒超时） */
async function fetchJSON(url, options) {
    const controller = new AbortController();
    const timer = setTimeout(() => controller.abort(), 20000);
    try {
        const resp = await fetch(url, Object.assign({ signal: controller.signal, cache: 'no-store' }, options || {}));
        const text = await resp.text();
        let data = null;
        try {
            data = JSON.parse(text);
        } catch (e) {
            throw new Error('返回内容不是 JSON（HTTP ' + resp.status + '）');
        }
        if (!resp.ok) throw new Error('HTTP ' + resp.status);
        return data;
    } finally {
        clearTimeout(timer);
    }
}

/** 和风天气是否具备可用的 API Key 凭据 */
function qweatherAvailable(cfg) {
    return !!(cfg.qweatherHost && cfg.qweatherKey);
}

/** 和风天气不可用原因 */
function qweatherUnavailableReason(cfg) {
    if (!cfg.qweatherHost) return '未配置 API Host';
    if (!cfg.qweatherKey) {
        return cfg.qweatherAuth === 'jwt'
            ? '浏览器版暂不支持 Ed25519 JWT，请改用 API Key'
            : '未配置 API Key';
    }
    return '';
}

// ===== 天气数据源 =====

const WMO_CODES = {
    0: '晴天', 1: '大部晴朗', 2: '多云', 3: '阴天',
    45: '雾', 48: '冻雾',
    51: '小毛毛雨', 53: '毛毛雨', 55: '大毛毛雨',
    56: '小冻毛毛雨', 57: '大冻毛毛雨',
    61: '小雨', 63: '中雨', 65: '大雨',
    66: '小冻雨', 67: '大冻雨',
    71: '小雪', 73: '中雪', 75: '大雪',
    77: '雪粒',
    80: '小阵雨', 81: '阵雨', 82: '大阵雨',
    85: '小阵雪', 86: '大阵雪',
    95: '雷暴', 96: '小冰雹雷暴', 99: '大冰雹雷暴'
};

const WIND_DIRS = ['北', '东北', '东', '东南', '南', '西南', '西', '西北'];

const PROVIDER_LABELS = {
    qweather: '和风天气',
    openmeteo: 'Open-Meteo',
    wttr: 'wttr.in'
};

/** wttr.in：当前天气 + 未来 2 天预报 */
async function queryWttr(city) {
    const url = 'https://wttr.in/' + encodeURIComponent(city) + '?format=j1';
    const data = await fetchJSON(url);
    const current = data && data.current_condition && data.current_condition[0];
    if (!current) throw new Error('wttr.in 天气数据解析失败');

    const area = data.nearest_area && data.nearest_area[0];
    const areaName = area && area.areaName && area.areaName[0] && area.areaName[0].value;
    // 中文输入优先保留中文城市名；areaName 多为英文
    const displayCity = /[\u4e00-\u9fff]/.test(city) ? city : (areaName || city);

    const result = {
        city: displayCity,
        current: {
            temperature: swInt(current.temp_C),
            feels_like: swInt(current.FeelsLikeC) || swInt(current.temp_C),
            condition: (current.weatherDesc && current.weatherDesc[0] && current.weatherDesc[0].value) || '未知',
            humidity: swInt(current.humidity),
            wind: ((current.winddir16Point || '') + ' ' + (current.windspeedKmph || '') + 'km/h').trim(),
            visibility: swInt(current.visibility)
        },
        forecast: [],
        source: 'wttr.in',
        provider: 'wttr'
    };

    const days = (data.weather || []).slice(0, 2);
    days.forEach(day => {
        const hourly = (day.hourly && day.hourly[4]) || null;
        result.forecast.push({
            date: day.date || '',
            high: swInt(day.maxtempC),
            low: swInt(day.mintempC),
            condition: (hourly && hourly.weatherDesc && hourly.weatherDesc[0] && hourly.weatherDesc[0].value) || '未知',
            precip_prob: null
        });
    });
    return result;
}

/** Open-Meteo：地理编码 + 当前天气 + 3 天预报 */
async function queryOpenMeteo(city) {
    const geo = await fetchJSON('https://geocoding-api.open-meteo.com/v1/search?name=' +
        encodeURIComponent(city) + '&count=1&language=zh&format=json');
    const location = geo && geo.results && geo.results[0];
    if (!location) throw new Error('Open-Meteo 未找到城市：' + city);

    const weather = await fetchJSON('https://api.open-meteo.com/v1/forecast' +
        '?latitude=' + encodeURIComponent(location.latitude) +
        '&longitude=' + encodeURIComponent(location.longitude) +
        '&current=temperature_2m,apparent_temperature,weather_code,relative_humidity_2m,wind_speed_10m,wind_direction_10m,visibility' +
        '&daily=temperature_2m_max,temperature_2m_min,weather_code,precipitation_probability_max' +
        '&timezone=auto&forecast_days=3');
    if (!weather || !weather.current) throw new Error('Open-Meteo 天气请求失败');

    const cur = weather.current;
    const dirIndex = Math.round(((cur.wind_direction_10m || 0) % 360) / 45) % 8;
    const resolvedName = location.name || city;
    const country = location.country || '';
    const adm1 = location.admin1 || '';

    const result = {
        city: [resolvedName, adm1, country && country !== '中国' ? country : '']
            .filter(Boolean).join(', '),
        current: {
            temperature: swInt(cur.temperature_2m),
            feels_like: swInt(cur.apparent_temperature),
            condition: WMO_CODES[cur.weather_code] || ('天气代码 ' + cur.weather_code),
            humidity: swInt(cur.relative_humidity_2m),
            wind: WIND_DIRS[dirIndex] + ' ' + swInt(cur.wind_speed_10m) + 'km/h',
            visibility: cur.visibility ? Math.round(cur.visibility / 100) / 10 : 0
        },
        forecast: [],
        source: 'Open-Meteo',
        provider: 'openmeteo'
    };

    const daily = weather.daily;
    if (daily && Array.isArray(daily.time)) {
        for (let i = 0; i < Math.min(daily.time.length, 3); i++) {
            result.forecast.push({
                date: daily.time[i] || '',
                high: swInt(daily.temperature_2m_max && daily.temperature_2m_max[i]),
                low: swInt(daily.temperature_2m_min && daily.temperature_2m_min[i]),
                condition: WMO_CODES[daily.weather_code && daily.weather_code[i]] || '未知',
                precip_prob: daily.precipitation_probability_max
                    ? swInt(daily.precipitation_probability_max[i])
                    : null
            });
        }
    }
    return result;
}

/** 和风天气：API Key + 城市搜索 + 实时 + 3 天预报 + 空气质量 */
async function queryQWeather(city, cfg) {
    if (!cfg.qweatherHost) throw new Error('和风天气未配置 API Host');
    if (!cfg.qweatherKey) throw new Error(qweatherUnavailableReason(cfg));

    const base = 'https://' + cfg.qweatherHost.replace(/^https?:\/\//, '').replace(/\/+$/, '');
    const headers = { 'X-QW-Api-Key': cfg.qweatherKey };

    const geo = await fetchJSON(base + '/geo/v2/city/lookup?location=' +
        encodeURIComponent(city) + '&lang=zh', { headers });
    const loc = geo && geo.location && geo.location[0];
    if (!loc) throw new Error('和风天气未找到城市：' + city);
    if (String(geo.code) !== '200') throw new Error('和风天气城市搜索失败：code=' + geo.code);

    const locationID = loc.id;
    const nowResp = await fetchJSON(base + '/v7/weather/now?location=' +
        encodeURIComponent(locationID), { headers });
    if (String(nowResp.code) !== '200' || !nowResp.now) {
        throw new Error('和风天气实时天气请求失败：code=' + (nowResp && nowResp.code));
    }
    const now = nowResp.now;

    const result = {
        city: [loc.name || city, loc.adm1 || '', loc.country && loc.country !== '中国' ? loc.country : '']
            .filter(Boolean).join(', '),
        current: {
            temperature: swInt(now.temp),
            feels_like: swInt(now.feelsLike),
            condition: now.text || '未知',
            humidity: swInt(now.humidity),
            wind: ((now.windDir || '') + ' ' + (now.windSpeed || '') + 'km/h').trim(),
            visibility: swInt(now.vis)
        },
        forecast: [],
        source: '和风天气 (API Key)',
        provider: 'qweather'
    };

    try {
        const fc = await fetchJSON(base + '/v7/weather/3d?location=' +
            encodeURIComponent(locationID), { headers });
        if (String(fc.code) === '200' && Array.isArray(fc.daily)) {
            fc.daily.slice(0, 3).forEach(day => {
                result.forecast.push({
                    date: day.fxDate || '',
                    high: swInt(day.tempMax),
                    low: swInt(day.tempMin),
                    condition: day.textDay || '未知',
                    precip_prob: day.pop !== undefined ? swInt(day.pop) : null
                });
            });
        }
    } catch (e) {
        console.warn('[实时资讯] 和风天气预报获取失败:', e);
    }

    if (cfg.showAQI && loc.lat && loc.lon) {
        try {
            const air = await fetchJSON(base + '/airquality/v1/current/' +
                encodeURIComponent(loc.lat) + '/' + encodeURIComponent(loc.lon) + '?lang=zh', { headers });
            const index = air && Array.isArray(air.indexes) ? air.indexes[0] : null;
            if (index) {
                result.aqi = {
                    value: swInt(index.aqi),
                    level: index.level || '',
                    category: index.category || '',
                    primary: (index.primaryPollutant && index.primaryPollutant.name) || ''
                };
            }
        } catch (e) {
            console.warn('[实时资讯] 和风空气质量获取失败（可能未开通该接口）:', e);
        }
    }
    return result;
}
