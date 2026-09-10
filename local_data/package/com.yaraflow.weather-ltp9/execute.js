/**
 * LTP9 天气查询插件（com.yaraflow.weather-ltp9 / execute.js）
 * 全部同步：不使用 async/await/Promise/fetch。网络走引擎同步 API engine.http.get/post；
 * JWT 走 engine.crypto.signJWT；事件订阅与导出函数都返回普通对象（非 Promise）。
 */

// ===== 配置 =====

const WMO_CODES = {
  0: "晴天", 1: "大部晴朗", 2: "多云", 3: "阴天",
  45: "雾", 48: "冻雾",
  51: "小毛毛雨", 53: "毛毛雨", 55: "大毛毛雨",
  56: "小冻毛毛雨", 57: "大冻毛毛雨",
  61: "小雨", 63: "中雨", 65: "大雨",
  66: "小冻雨", 67: "大冻雨",
  71: "小雪", 73: "中雪", 75: "大雪",
  77: "雪粒",
  80: "小阵雨", 81: "阵雨", 82: "大阵雨",
  85: "小阵雪", 86: "大阵雪",
  95: "雷暴", 96: "小冰雹雷暴", 99: "大冰雹雷暴"
};

function getConfig() {
  let config = {};
  try { config = engine.config.getFile() || {}; } catch (e) { config = {}; }
  return {
    enabled: (config.plugin && config.plugin.enabled !== undefined) ? config.plugin.enabled : true,
    provider: (config.api && config.api.provider) || "auto",
    qweatherAuth: (config.api && config.api.qweather_auth) || "api_key",
    qweatherHost: (config.api && config.api.qweather_host) || "",
    qweatherKey: (config.api && config.api.qweather_key) || "",
    qweatherProjectID: (config.api && config.api.qweather_project_id) || "",
    qweatherKeyID: (config.api && config.api.qweather_key_id) || "",
    qweatherPrivateKey: (config.api && config.api.qweather_private_key) || "",
    defaultCity: (config.display && config.display.default_city) || "",
    showAQI: (config.display && config.display.show_aqi !== undefined) ? config.display.show_aqi : true
  };
}

// ===== 同步 HTTP + JSON =====

function httpGetJson(url, headers) {
  const r = engine.http.get(url, headers || {});
  if (r.error) return { error: r.error, status: 0, data: null };
  let data = null;
  try { data = JSON.parse(r.body); } catch (e) { data = null; }
  return { status: r.status, data: data, body: r.body, error: (r.status >= 200 && r.status < 300) ? "" : "HTTP " + r.status };
}

// ===== 同步各提供者 =====

function queryWttr(city) {
  console.info("[天气] [wttr.in] 查询城市 -> " + city);
  const url = "https://wttr.in/" + encodeURIComponent(city) + "?format=j1";
  const r = httpGetJson(url);
  if (r.error) return { error: "wttr.in 请求失败: " + r.error };
  if (!r.data || !r.data.current_condition) return { error: "wttr.in 返回异常: HTTP " + r.status };
  const current = r.data.current_condition[0];
  const result = {
    city: city,
    current: {
      temperature: parseFloat(current.temp_C) || 0,
      feels_like: parseFloat(current.FeelsLikeC) || parseFloat(current.temp_C) || 0,
      condition: (current.weatherDesc && current.weatherDesc[0] && current.weatherDesc[0].value) || "未知",
      humidity: parseInt(current.humidity, 10) || 0,
      wind: current.winddir16Point + " " + current.windspeedKmph + "km/h",
      visibility: parseInt(current.visibility, 10) || 0
    },
    forecast: [],
    source: "wttr.in"
  };
  const days = r.data.weather || [];
  for (let i = 0; i < Math.min(days.length, 2); i++) {
    const day = days[i];
    const h = (day.hourly && day.hourly[4]) || null;
    result.forecast.push({
      date: day.date || "",
      high: parseFloat(day.maxtempC) || 0,
      low: parseFloat(day.mintempC) || 0,
      condition: (h && h.weatherDesc && h.weatherDesc[0] && h.weatherDesc[0].value) || "未知"
    });
  }
  return result;
}

function queryOpenMeteo(city) {
  console.info("[天气] [Open-Meteo] 查询城市 -> " + city);
  const geo = httpGetJson("https://geocoding-api.open-meteo.com/v1/search?name=" + encodeURIComponent(city) + "&count=1&language=zh");
  if (geo.error || !geo.data || !geo.data.results || geo.data.results.length === 0) {
    return { error: "Open-Meteo 未找到城市: " + city };
  }
  const location = geo.data.results[0];
  const lat = location.latitude;
  const lon = location.longitude;
  const resolvedName = location.name || city;
  const country = location.country || "";
  console.info("[天气] [Open-Meteo] 地理编码 -> " + resolvedName + " (" + lat + ", " + lon + ")");

  const w = httpGetJson("https://api.open-meteo.com/v1/forecast" +
    "?latitude=" + lat + "&longitude=" + lon +
    "&current=temperature_2m,apparent_temperature,weather_code,relative_humidity_2m,wind_speed_10m,wind_direction_10m" +
    "&daily=temperature_2m_max,temperature_2m_min,weather_code,precipitation_probability_max" +
    "&timezone=auto&forecast_days=3");
  if (w.error || !w.data || !w.data.current) return { error: "Open-Meteo 天气请求失败: HTTP " + w.status };

  const cur = w.data.current;
  const dirs = ["北", "东北", "东", "东南", "南", "西南", "西", "西北"];
  const dirIndex = Math.round(((cur.wind_direction_10m || 0) % 360) / 45) % 8;
  const result = {
    city: (country ? resolvedName + ", " + country : resolvedName),
    current: {
      temperature: Math.round(cur.temperature_2m),
      feels_like: Math.round(cur.apparent_temperature),
      condition: WMO_CODES[cur.weather_code] || ("天气代码" + cur.weather_code),
      humidity: cur.relative_humidity_2m,
      wind: dirs[dirIndex] + " " + Math.round(cur.wind_speed_10m) + "km/h"
    },
    forecast: [],
    source: "Open-Meteo"
  };
  const daily = w.data.daily;
  for (let i = 0; i < Math.min(daily.time.length, 2); i++) {
    result.forecast.push({
      date: daily.time[i],
      high: Math.round(daily.temperature_2m_max[i]),
      low: Math.round(daily.temperature_2m_min[i]),
      condition: WMO_CODES[daily.weather_code[i]] || "未知",
      precip_prob: daily.precipitation_probability_max ? daily.precipitation_probability_max[i] : null
    });
  }
  return result;
}

function queryQWeather(city) {
  const config = getConfig();
  const apiHost = config.qweatherHost || "";
  if (!apiHost) return { error: "和风天气 需要配置 API Host" };
  const useJWT = config.qweatherAuth === "jwt" && config.qweatherProjectID && config.qweatherPrivateKey;
  if (!useJWT && !config.qweatherKey) return { error: "和风天气 需要配置 API Key 或 JWT 私钥" };

  const headers = {};
  if (useJWT) {
    const now = Math.floor(engine.time.now()) - 30;
    const token = engine.crypto.signJWT({ sub: config.qweatherProjectID, iat: now, exp: now + 900 }, config.qweatherPrivateKey, "EdDSA", config.qweatherKeyID);
    headers["Authorization"] = "Bearer " + token;
  } else {
    headers["X-QW-Api-Key"] = config.qweatherKey;
  }

  function req(endpoint) {
    return httpGetJson("https://" + apiHost + "/" + endpoint, headers);
  }

  const cityResp = req("geo/v2/city/lookup?location=" + encodeURIComponent(city));
  if (cityResp.error) return { error: "和风天气 城市搜索失败: " + cityResp.error };
  if (!cityResp.data || cityResp.data.code !== "200" || !cityResp.data.location || cityResp.data.location.length === 0) {
    return { error: "和风天气 未找到城市: " + city };
  }
  const loc = cityResp.data.location[0];
  const locationId = loc.id;
  const resolvedName = loc.name || city;
  const adm1 = loc.adm1 || "";
  const country = loc.country || "";

  const nowResp = req("v7/weather/now?location=" + locationId);
  if (nowResp.error || !nowResp.data || nowResp.data.code !== "200") {
    return { error: "和风天气 实时天气请求失败: HTTP " + nowResp.status };
  }
  const now = nowResp.data.now;
  const forecastList = [];
  const fc = req("v7/weather/3d?location=" + locationId);
  if (fc.data && fc.data.code === "200" && fc.data.daily) {
    for (let i = 0; i < Math.min(fc.data.daily.length, 2); i++) {
      const d = fc.data.daily[i];
      forecastList.push({
        date: d.fxDate || "",
        high: parseInt(d.tempMax, 10) || 0,
        low: parseInt(d.tempMin, 10) || 0,
        condition: d.textDay || "未知",
        precip_prob: parseInt(d.pop, 10) || null
      });
    }
  }

  const result = {
    city: (adm1 ? resolvedName + ", " + adm1 : resolvedName) + (country && country !== "中国" ? ", " + country : ""),
    current: {
      temperature: parseInt(now.temp, 10) || 0,
      feels_like: parseInt(now.feelsLike, 10) || 0,
      condition: now.text || "未知",
      humidity: parseInt(now.humidity, 10) || 0,
      wind: (now.windDir || "") + " " + (now.windSpeed || "") + "km/h",
      visibility: parseInt(now.vis, 10) || 0
    },
    forecast: forecastList,
    source: "和风天气 (API Key)"
  };

  if (getConfig().showAQI) {
    const air = req("v7/air/now?location=" + locationId);
    if (air.data && air.data.code === "200" && air.data.now) {
      const a = air.data.now;
      result.aqi = { value: parseInt(a.aqi, 10) || 0, level: a.level || "未知", category: a.category || "未知", primary: a.primary || "无" };
    }
  }
  return result;
}

function tryProvider(fn, city) {
  try { return fn(city); } catch (e) { return { error: (e && e.message) ? e.message : String(e) }; }
}

function getWeather(city) {
  const config = getConfig();
  if (!config.enabled) return { error: "天气查询插件未启用" };
  let c = city ? String(city).trim() : "";
  if (!c) {
    if (config.defaultCity) { c = config.defaultCity; console.info("[天气] 使用默认城市 -> " + c); }
    else return { error: "请提供城市名称，例如: 北京、上海、深圳" };
  }

  const provider = config.provider;
  const hasHost = !!config.qweatherHost;
  const hasKey = !!config.qweatherKey;
  const hasJWT = config.qweatherAuth === "jwt" && !!config.qweatherProjectID && !!config.qweatherPrivateKey;
  const hasQCreds = hasHost && (hasKey || hasJWT);

  if (provider === "qweather") {
    let r = tryProvider(queryQWeather, c);
    if (r && !r.error) return r;
    console.warn("[天气] 和风天气失败，回退到 Open-Meteo -> " + (r ? r.error : "未知"));
    r = tryProvider(queryOpenMeteo, c);
    if (r && !r.error) return r;
    console.warn("[天气] Open-Meteo 失败，回退到 wttr.in");
    r = tryProvider(queryWttr, c);
    if (r && !r.error) return r;
    return { error: "所有天气服务不可用，请稍后再试" };
  }

  if (provider === "openmeteo") {
    let r = tryProvider(queryOpenMeteo, c);
    if (r && !r.error) return r;
    console.warn("[天气] Open-Meteo 失败，回退到 wttr.in");
    r = tryProvider(queryWttr, c);
    if (r && !r.error) return r;
    return { error: "所有天气服务不可用，请稍后再试" };
  }

  if (provider === "wttr") {
    const r = tryProvider(queryWttr, c);
    if (r && !r.error) return r;
    return { error: "wttr.in 服务不可用，请稍后再试" };
  }

  if (provider === "auto") {
    if (hasQCreds) {
      let r = tryProvider(queryQWeather, c);
      if (r && !r.error) return r;
      console.warn("[天气] 和风天气失败，回退到 Open-Meteo -> " + (r ? r.error : "未知"));
    }
    let r = tryProvider(queryOpenMeteo, c);
    if (r && !r.error) return r;
    console.warn("[天气] Open-Meteo 失败，回退到 wttr.in -> " + (r ? r.error : "未知"));
    r = tryProvider(queryWttr, c);
    if (r && !r.error) return r;
    return { error: "所有天气服务不可用，请稍后再试" };
  }

  return { error: "无效的API提供者配置: " + provider };
}

// ===== 对外入口（全部同步，返回普通对象，不返回 Promise） =====

function queryWeather(rawCity) {
  const result = getWeather(rawCity);
  if (result.error) {
    console.warn("[天气] 查询失败 -> " + result.error);
    return { ok: false, error: result.error };
  }
  console.info("[天气] 成功 -> " + result.city + " (" + result.source + ") " + result.current.temperature + "°C " + result.current.condition);
  return { ok: true, data: result };
}

// 事件订阅：同步返回结果（引擎同步阻塞拿到后回传）
engine.event.subscribe("weather.query", function (ev) {
  const p = ev && ev.payload ? ev.payload : {};
  const result = queryWeather(p.city);
  engine.signal.all({ topic: "weather.query", result: result });
  return result;
});

// 导出函数：同步（非 async）
engine.export("queryWeather", function (city) {
  return queryWeather(city);
});

// 探针：用于确认引擎执行的是本（同步）版本
engine.export("ping", function () {
  return { pong: true, src: "com.yaraflow.weather-ltp9", sync: true, version: 3 };
});

// ===== 生命周期 =====

function onLoad() {
  const cfg = getConfig();
  console.info("[天气] LTP9 天气插件已加载（同步），provider=" + cfg.provider + ", 默认城市=" + (cfg.defaultCity || "无"));
}

function onUnload() {
  console.info("[天气] LTP9 天气插件已卸载");
}