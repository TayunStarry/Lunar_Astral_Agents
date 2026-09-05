/**
 * 天气查询插件
 * 支持多API提供者自动回退（和风天气 → Open-Meteo → wttr.in）
 */

// ===== 配置 =====

var DEFAULT_CONFIG_YAML = [
  "# 天气查询插件 - 配置文件",
  "# 支持多API提供者自动回退，不填Key也能用（自动使用免费API）",
  "# 此文件由插件首次运行时自动生成",
  "# 和风天气免费Key注册: https://dev.qweather.com",
  "",
  "plugin:",
  "  enabled: true",
  "",
  "api:",
  "  # API提供者: auto=自动回退, qweather=和风天气, openmeteo=Open-Meteo, wttr=wttr.in",
  "  provider: auto",
  "  # 和风天气认证方式: api_key=API密钥, jwt=JWT令牌（推荐，2027年起API Key受限）",
  "  qweather_auth: api_key",
  "  # 和风天气API Host（在控制台设置中查看，格式如 abc1234.qweatherapi.com）",
  "  qweather_host: \"\"",
  "  # 和风天气API Key（免费注册: https://dev.qweather.com），不填则自动跳过",
  "  qweather_key: \"\"",
  "  # 和风天气JWT认证 - 项目ID（Project ID）",
  "  qweather_project_id: \"\"",
  "  # 和风天气JWT认证 - 密钥ID（Key ID）",
  "  qweather_key_id: \"\"",
  "  # 和风天气JWT认证 - Ed25519私钥（PEM格式）",
  "  qweather_private_key: \"\"",
  "",
  "display:",
  "  # 默认城市，用户未指定城市时使用，示例: 北京",
  "  default_city: \"\"",
  "  # 是否显示空气质量指数（仅和风天气支持）",
  "  show_aqi: true",
  ""
].join("\n");

// 天气代码 → 中文描述（Open-Meteo WMO code）
var WMO_CODES = {
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

// ===== 配置管理 =====

function ensureConfigFile() {
  var config = {};
  var needsMigration = false;

  try {
    config = yara.config.getFile();
    if (config && Object.keys(config).length > 0) {
      // 仅当「缺少新版字段 api.qweather_host」时才视为旧版需迁移。
      // 注意不能用 `!qweather_host` 判断：qweather_host 允许为空字符串（合法默认），空串是 falsy
      // 会误判为"旧版"→ 每次加载都迁移 → setFile 触发 reload → 又迁移，形成 加载-迁移-重载 死循环。
      if (!config.api || config.api.qweather_host === undefined) {
        needsMigration = true;
      }
    }
  } catch (e) {}

  if (needsMigration) {
    yara.logger.info("天气查询: 检测到旧版配置，正在迁移...");
    var mergedConfig = {};
    try {
      yara.file.write("config.yaml.bak", yara.file.read("config.yaml"));
      yara.file.write("config.yaml", DEFAULT_CONFIG_YAML);
      mergedConfig = yara.config.getFile();
      if (config.api) {
        if (config.api.provider) mergedConfig.api.provider = config.api.provider;
        if (config.api.qweather_key) mergedConfig.api.qweather_key = config.api.qweather_key;
      }
      if (config.display) {
        if (config.display.default_city) mergedConfig.display.default_city = config.display.default_city;
        if (config.display.show_aqi !== undefined) mergedConfig.display.show_aqi = config.display.show_aqi;
      }
      yara.config.setFile(mergedConfig);
      yara.logger.info("天气查询: 配置迁移完成");
    } catch (e) {
      yara.logger.error("天气查询: 配置迁移失败: " + e.message);
    }
    return mergedConfig;
  }

  if (config && Object.keys(config).length > 0) {
    return config;
  }

  yara.logger.info("天气查询: 配置文件不存在，自动生成默认配置...");
  try {
    yara.file.write("config.yaml", DEFAULT_CONFIG_YAML);
    yara.logger.info("天气查询: 默认配置文件已生成");
  } catch (e) {
    yara.logger.error("天气查询: 无法生成配置文件: " + e.message);
  }

  try {
    return yara.config.getFile();
  } catch (e) {
    return {};
  }
}

function getConfig() {
  var config = ensureConfigFile();
  if (!config) return {};

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

// ===== API提供者：wttr.in（兜底，免费免Key） =====

function queryWttr(city) {
  yara.logger.info("天气查询: [wttr.in] 查询城市 -> " + city);

  var url = "https://wttr.in/" + yara.encoding.urlEncode(city) + "?format=j1";
  var resp = yara.http.get(url);

  if (resp.error) {
    return { error: "wttr.in 请求失败: " + resp.error };
  }
  if (resp.status !== 200) {
    return { error: "wttr.in 返回状态: " + resp.status };
  }

  try {
    var data = JSON.parse(resp.body);
    var current = data.current_condition[0];

    var result = {
      city: city,
      current: {
        temperature: parseFloat(current.temp_C) || 0,
        feels_like: parseFloat(current.FeelsLikeC) || parseFloat(current.temp_C) || 0,
        condition: current.weatherDesc[0].value || "未知",
        humidity: parseInt(current.humidity, 10) || 0,
        wind: current.winddir16Point + " " + current.windspeedKmph + "km/h",
        visibility: parseInt(current.visibility, 10) || 0
      },
      forecast: [],
      source: "wttr.in"
    };

    // 预报（最多2天）
    var weatherData = data.weather || [];
    for (var i = 0; i < Math.min(weatherData.length, 2); i++) {
      var day = weatherData[i];
      result.forecast.push({
        date: day.date || "",
        high: parseFloat(day.maxtempC) || 0,
        low: parseFloat(day.mintempC) || 0,
        condition: (day.hourly && day.hourly[4] && day.hourly[4].weatherDesc[0])
          ? day.hourly[4].weatherDesc[0].value : "未知"
      });
    }

    return result;
  } catch (e) {
    return { error: "wttr.in 数据解析失败: " + e.message };
  }
}

// ===== API提供者：Open-Meteo（免费免Key，需经纬度） =====

function queryOpenMeteo(city) {
  yara.logger.info("天气查询: [Open-Meteo] 查询城市 -> " + city);

  // 1. Geocoding: 城市名 → 经纬度
  var geoUrl = "https://geocoding-api.open-meteo.com/v1/search?name=" + yara.encoding.urlEncode(city) + "&count=1&language=zh";
  var geoResp = yara.http.get(geoUrl);

  if (geoResp.error) {
    return { error: "Open-Meteo 地理编码失败: " + geoResp.error };
  }
  if (geoResp.status !== 200) {
    return { error: "Open-Meteo 地理编码返回状态: " + geoResp.status };
  }

  try {
    var geoData = JSON.parse(geoResp.body);
    if (!geoData.results || geoData.results.length === 0) {
      return { error: "Open-Meteo 未找到城市: " + city };
    }
    var location = geoData.results[0];
    var lat = location.latitude;
    var lon = location.longitude;
    var resolvedName = location.name || city;
    var country = location.country || "";

    yara.logger.info("天气查询: [Open-Meteo] 地理编码 -> " + resolvedName + " (" + lat + ", " + lon + ")" + (country ? " " + country : ""));

    // 2. 天气查询
    var weatherUrl = "https://api.open-meteo.com/v1/forecast" +
      "?latitude=" + lat + "&longitude=" + lon +
      "&current=temperature_2m,apparent_temperature,weather_code,relative_humidity_2m,wind_speed_10m,wind_direction_10m" +
      "&daily=temperature_2m_max,temperature_2m_min,weather_code,precipitation_probability_max" +
      "&timezone=auto&forecast_days=3";

    var weatherResp = yara.http.get(weatherUrl);
    if (weatherResp.error) {
      return { error: "Open-Meteo 天气请求失败: " + weatherResp.error };
    }
    if (weatherResp.status !== 200) {
      return { error: "Open-Meteo 天气返回状态: " + weatherResp.status };
    }

    var weatherData = JSON.parse(weatherResp.body);

    // 风向角度转方向
    var windDir = weatherData.current.wind_direction_10m;
    var dirs = ["北", "东北", "东", "东南", "南", "西南", "西", "西北"];
    var dirIndex = Math.round((windDir % 360) / 45) % 8;
    var windDesc = dirs[dirIndex] + " " + Math.round(weatherData.current.wind_speed_10m) + "km/h";

    var result = {
      city: (country ? resolvedName + ", " + country : resolvedName),
      current: {
        temperature: Math.round(weatherData.current.temperature_2m),
        feels_like: Math.round(weatherData.current.apparent_temperature),
        condition: WMO_CODES[weatherData.current.weather_code] || ("天气代码" + weatherData.current.weather_code),
        humidity: weatherData.current.relative_humidity_2m,
        wind: windDesc
      },
      forecast: [],
      source: "Open-Meteo"
    };

    // 预报
    var daily = weatherData.daily;
    for (var i = 0; i < Math.min(daily.time.length, 2); i++) {
      result.forecast.push({
        date: daily.time[i],
        high: Math.round(daily.temperature_2m_max[i]),
        low: Math.round(daily.temperature_2m_min[i]),
        condition: WMO_CODES[daily.weather_code[i]] || "未知",
        precip_prob: daily.precipitation_probability_max ? daily.precipitation_probability_max[i] : null
      });
    }

    return result;
  } catch (e) {
    return { error: "Open-Meteo 数据解析失败: " + e.message };
  }
}

// ===== API提供者：和风天气（免费1000次/天，支持API Key和JWT认证） =====

function queryQWeather(city) {
  yara.logger.info("天气查询: [和风天气] 查询城市 -> " + city);

  var config = getConfig();
  var authType = config.qweatherAuth || "api_key";
  var apiHost = config.qweatherHost || "";
  var apiKey = config.qweatherKey || "";
  var projectID = config.qweatherProjectID || "";
  var keyID = config.qweatherKeyID || "";
  var privateKey = config.qweatherPrivateKey || "";

  if (!apiHost) {
    return { error: "和风天气 需要配置API Host（在控制台设置中查看）" };
  }

  var useJWT = authType === "jwt" && projectID && keyID && privateKey;
  var jwtToken = null;

  if (useJWT) {
    try {
      var now = Math.floor(Date.now() / 1000) - 30;
      var claims = {
        "sub": projectID,
        "iat": now,
        "exp": now + 900
      };
      jwtToken = yara.crypto.generateJWT(claims, privateKey, keyID);
      yara.logger.info("天气查询: [和风天气] JWT生成成功");
    } catch (e) {
      yara.logger.warn("天气查询: [和风天气] JWT生成失败: " + e.message);
      return { error: "和风天气 JWT生成失败: " + e.message };
    }
  }

  if (authType === "api_key" && !apiKey) {
    return { error: "和风天气(API Key模式) 需要配置API Key" };
  }

  var headers = {};
  if (useJWT && jwtToken) {
    headers["Authorization"] = "Bearer " + jwtToken;
  } else if (apiKey) {
    headers["X-QW-Api-Key"] = apiKey;
  }

  function makeQWeatherRequest(endpoint) {
    var url = "https://" + apiHost + "/" + endpoint;
    return yara.http.get(url, headers);
  }

  var cityResp = makeQWeatherRequest("geo/v2/city/lookup?location=" + yara.encoding.urlEncode(city));

  if (cityResp.error) {
    return { error: "和风天气 城市搜索失败: " + cityResp.error };
  }
  if (cityResp.status !== 200) {
    return { error: "和风天气 城市搜索返回状态: " + cityResp.status };
  }

  try {
    var cityData = JSON.parse(cityResp.body);
    if (cityData.code !== "200" || !cityData.location || cityData.location.length === 0) {
      return { error: "和风天气 未找到城市: " + city };
    }
    var loc = cityData.location[0];
    var locationId = loc.id;
    var resolvedName = loc.name || city;
    var adm1 = loc.adm1 || "";
    var country = loc.country || "";

    yara.logger.info("天气查询: [和风天气] 城市 -> " + resolvedName + " (ID: " + locationId + ")" + (adm1 ? " " + adm1 : ""));

    var nowResp = makeQWeatherRequest("v7/weather/now?location=" + locationId);
    if (nowResp.error) {
      return { error: "和风天气 实时天气请求失败: " + nowResp.error };
    }
    var nowData = JSON.parse(nowResp.body);
    if (nowData.code !== "200") {
      return { error: "和风天气 实时天气API错误: " + (nowData.code || "未知") + (nowData.message ? " - " + nowData.message : "") };
    }
    var now = nowData.now;

    var forecastUrl = "v7/weather/3d?location=" + locationId;
    var forecastResp = makeQWeatherRequest(forecastUrl);
    var forecastList = [];
    if (!forecastResp.error && forecastResp.status === 200) {
      try {
        var forecastData = JSON.parse(forecastResp.body);
        if (forecastData.code === "200" && forecastData.daily) {
          for (var i = 0; i < Math.min(forecastData.daily.length, 2); i++) {
            var d = forecastData.daily[i];
            forecastList.push({
              date: d.fxDate || "",
              high: parseInt(d.tempMax, 10) || 0,
              low: parseInt(d.tempMin, 10) || 0,
              condition: d.textDay || "未知",
              precip_prob: parseInt(d.pop, 10) || null
            });
          }
        }
      } catch (e) {
        yara.logger.warn("天气查询: [和风天气] 预报解析失败: " + e.message);
      }
    }

    var result = {
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
      source: "和风天气" + (useJWT ? " (JWT)" : " (API Key)")
    };

    if (getConfig().showAQI) {
      try {
        var airResp = makeQWeatherRequest("v7/air/now?location=" + locationId);
        if (!airResp.error && airResp.status === 200) {
          var airData = JSON.parse(airResp.body);
          if (airData.code === "200" && airData.now) {
            result.aqi = {
              value: parseInt(airData.now.aqi, 10) || 0,
              level: airData.now.level || "未知",
              category: airData.now.category || "未知",
              primary: airData.now.primary || "无"
            };
          }
        }
      } catch (e) {
        yara.logger.warn("天气查询: [和风天气] AQI 获取失败: " + e.message);
      }
    }

    return result;
  } catch (e) {
    return { error: "和风天气 数据解析失败: " + e.message };
  }
}

// ===== 主查询逻辑（自动回退） =====

function getWeather(city) {
  var config = getConfig();

  if (!config.enabled) {
    return { error: "天气查询插件未启用" };
  }

  if (!city || city.trim() === "") {
    if (config.defaultCity) {
      city = config.defaultCity;
      yara.logger.info("天气查询: 使用默认城市 -> " + city);
    } else {
      return { error: "请提供城市名称，例如: 北京、上海、深圳" };
    }
  }

  city = city.trim();

  var result;

  if (config.provider === "qweather") {
    result = queryQWeather(city);
    if (result && !result.error) return result;
    yara.logger.warn("天气查询: 和风天气失败，回退到 Open-Meteo -> " + (result ? result.error : "未知错误"));
    result = queryOpenMeteo(city);
    if (result && !result.error) return result;
    yara.logger.warn("天气查询: Open-Meteo 失败，回退到 wttr.in -> " + (result ? result.error : "未知错误"));
    result = queryWttr(city);
    if (result && !result.error) return result;
    return { error: "所有天气服务不可用，请稍后再试" };
  }

  if (config.provider === "openmeteo") {
    result = queryOpenMeteo(city);
    if (result && !result.error) return result;
    yara.logger.warn("天气查询: Open-Meteo 失败，回退到 wttr.in -> " + (result ? result.error : "未知错误"));
    result = queryWttr(city);
    if (result && !result.error) return result;
    return { error: "所有天气服务不可用，请稍后再试" };
  }

  if (config.provider === "wttr") {
    result = queryWttr(city);
    if (result && !result.error) return result;
    return { error: "wttr.in 服务不可用，请稍后再试" };
  }

  if (config.provider === "auto") {
    var hasHost = !!config.qweatherHost;
    var hasKeyCreds = config.qweatherAuth === "api_key" && !!config.qweatherKey;
    var hasJWTCreds = config.qweatherAuth === "jwt" && config.qweatherProjectID && config.qweatherKeyID && config.qweatherPrivateKey;
    var hasQWeatherCreds = hasHost && (hasKeyCreds || hasJWTCreds);

    if (hasQWeatherCreds) {
      result = queryQWeather(city);
      if (result && !result.error) return result;
      yara.logger.warn("天气查询: 和风天气失败，回退到 Open-Meteo -> " + (result ? result.error : "未知错误"));
    }
    result = queryOpenMeteo(city);
    if (result && !result.error) return result;
    yara.logger.warn("天气查询: Open-Meteo 失败，回退到 wttr.in -> " + (result ? result.error : "未知错误"));
    result = queryWttr(city);
    if (result && !result.error) return result;
    return { error: "所有天气服务不可用，请稍后再试" };
  }

  return { error: "无效的API提供者配置: " + config.provider };
}

// ===== 工具注册 =====

yara.tool.register("get_weather", {
  description: "查询指定城市的实时天气和未来2天预报。返回温度、天气状况、湿度、风力、体感温度等信息。支持中文城市名，如'北京'、'上海'、'深圳'。",
  parameters: [
    { name: "city", type: "string", description: "城市名称，中文或英文均可，示例: 北京、上海、shenzhen、Tokyo", required: true }
  ]
}, function(params, context) {
  try {
    yara.logger.info("天气查询: 工具被调用 -> city=" + params.city);

    var result = getWeather(params.city);

    if (result.error) {
      yara.logger.warn("天气查询: 查询失败 -> " + result.error);
      return { error: result.error };
    }

    yara.logger.info("天气查询: 成功 -> " + result.city + " (" + result.source + ") " + result.current.temperature + "°C " + result.current.condition);
    return result;

  } catch (e) {
    yara.logger.error("天气查询: 工具执行异常 -> " + e.message);
    return { error: "天气查询发生异常: " + e.message };
  }
});

// ===== 生命周期 =====

function onLoad() {
  yara.logger.info("天气查询插件已加载");

  var config = ensureConfigFile();
  var provider = (config.api && config.api.provider) || "auto";
  var authType = (config.api && config.api.qweather_auth) || "api_key";
  var hasHost = (config.api && config.api.qweather_host) ? true : false;
  var hasKey = (config.api && config.api.qweather_key) ? true : false;
  var hasJWT = (config.api && config.api.qweather_project_id && config.api.qweather_key_id && config.api.qweather_private_key) ? true : false;

  var credInfo = "无凭据";
  if (hasHost) {
    if (authType === "api_key" && hasKey) {
      credInfo = "API Key";
    } else if (authType === "jwt" && hasJWT) {
      credInfo = "JWT";
    }
  }

  yara.logger.info("天气查询: 提供者=" + provider + ", 认证方式=" + authType + ", 凭据=" + credInfo);
}

function onUnload() {
  yara.logger.info("天气查询插件已卸载");
}