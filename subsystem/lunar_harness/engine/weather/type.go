package weather

// type.go 天气查询的数据结构与数据源响应类型。
//
// 归一化后的 Data 是唯一对外结构；三家数据源（和风天气 / Open-Meteo / wttr.in）
// 的原始响应各自定义在本文件末尾，只供 provider_*.go 解析使用。

// Current 当前天气
type Current struct {
	Temperature int     `json:"temperature"`
	FeelsLike   int     `json:"feels_like"`
	Condition   string  `json:"condition"`
	Humidity    int     `json:"humidity"`
	Wind        string  `json:"wind"`
	Visibility  float64 `json:"visibility"`
}

// ForecastDay 单日预报
type ForecastDay struct {
	Date       string `json:"date"`
	High       int    `json:"high"`
	Low        int    `json:"low"`
	Condition  string `json:"condition"`
	PrecipProb *int   `json:"precip_prob,omitempty"`
}

// AirQuality 空气质量（仅和风天气支持）
type AirQuality struct {
	Value    int    `json:"value"`
	Level    string `json:"level"`
	Category string `json:"category"`
	Primary  string `json:"primary"`
}

// Data 一次天气查询的归一化结果
type Data struct {
	City     string        `json:"city"`
	Current  Current       `json:"current"`
	Forecast []ForecastDay `json:"forecast"`
	AQI      *AirQuality   `json:"aqi,omitempty"`
	Source   string        `json:"source"`
	Provider string        `json:"provider"`
	Strategy string        `json:"strategy"`
}

// ==== 和风天气响应 ====

type qWeatherGeo struct {
	Code     string `json:"code"`
	Location []struct {
		ID      string  `json:"id"`
		Name    string  `json:"name"`
		Adm1    string  `json:"adm1"`
		Country string  `json:"country"`
		Lat     float64 `json:"lat"`
		Lon     float64 `json:"lon"`
	} `json:"location"`
}

type qWeatherNow struct {
	Code string `json:"code"`
	Now  struct {
		Temp      string `json:"temp"`
		FeelsLike string `json:"feelsLike"`
		Text      string `json:"text"`
		Humidity  string `json:"humidity"`
		WindDir   string `json:"windDir"`
		WindSpeed string `json:"windSpeed"`
		Vis       string `json:"vis"`
	} `json:"now"`
}

type qWeatherDaily struct {
	Code  string `json:"code"`
	Daily []struct {
		FxDate  string `json:"fxDate"`
		TempMax string `json:"tempMax"`
		TempMin string `json:"tempMin"`
		TextDay string `json:"textDay"`
		Pop     string `json:"pop"`
	} `json:"daily"`
}

type qWeatherAir struct {
	Indexes []struct {
		AQI   float64 `json:"aqi"`
		Level string  `json:"level"`
		Cat   string  `json:"category"`
		Prim  struct {
			Name string `json:"name"`
		} `json:"primaryPollutant"`
	} `json:"indexes"`
}

// ==== Open-Meteo 响应 ====

type openMeteoGeo struct {
	Results []struct {
		Name      string  `json:"name"`
		Country   string  `json:"country"`
		Admin1    string  `json:"admin1"`
		Latitude  float64 `json:"latitude"`
		Longitude float64 `json:"longitude"`
	} `json:"results"`
}

type openMeteoForecast struct {
	Current struct {
		Temperature   float64 `json:"temperature_2m"`
		ApparentTemp  float64 `json:"apparent_temperature"`
		WeatherCode   int     `json:"weather_code"`
		Humidity      float64 `json:"relative_humidity_2m"`
		WindSpeed     float64 `json:"wind_speed_10m"`
		WindDirection float64 `json:"wind_direction_10m"`
		Visibility    float64 `json:"visibility"`
	} `json:"current"`
	Daily struct {
		Time        []string  `json:"time"`
		TempMax     []float64 `json:"temperature_2m_max"`
		TempMin     []float64 `json:"temperature_2m_min"`
		WeatherCode []int     `json:"weather_code"`
		PrecipProb  []float64 `json:"precipitation_probability_max"`
	} `json:"daily"`
}

// ==== wttr.in 响应 ====

type wttrDesc struct {
	Value string `json:"value"`
}

type wttrResponse struct {
	Current []struct {
		TempC       string     `json:"temp_C"`
		FeelsLikeC  string     `json:"FeelsLikeC"`
		WeatherDesc []wttrDesc `json:"weatherDesc"`
		Humidity    string     `json:"humidity"`
		WindDir     string     `json:"winddir16Point"`
		WindSpeed   string     `json:"windspeedKmph"`
		Visibility  string     `json:"visibility"`
	} `json:"current_condition"`
	NearestArea []struct {
		AreaName []wttrDesc `json:"areaName"`
	} `json:"nearest_area"`
	Weather []struct {
		Date    string `json:"date"`
		MaxTemp string `json:"maxtempC"`
		MinTemp string `json:"mintempC"`
		Hourly  []struct {
			WeatherDesc []wttrDesc `json:"weatherDesc"`
		} `json:"hourly"`
	} `json:"weather"`
}
