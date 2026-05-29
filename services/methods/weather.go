package methods

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"time"

	conf "github.com/ShiinaAiiko/nyanya-trip-route-track/server/config"
)

// Language 语言类型定义
type Language string

const (
	ZH_CN Language = "zh_cn" // 简体中文
	ZH_HK Language = "zh_hk" // 繁体中文
	EN    Language = "en"    // 英文
)

// WeatherInfo 存储转换后的文本
type WeatherLabel struct {
	WeatherText string
	WindText    string
}

// GetWeatherAndWindLabel 获取多语言天气和风向描述
func GetWeatherAndWindLabel(code int, degrees float64, lang Language) WeatherLabel {
	return WeatherLabel{
		WeatherText: GetWeatherText(code, lang),
		WindText:    GetWindDirectionText(degrees, lang),
	}
}

// GetWeatherText 将 WMO Weather Code 转换为多语言文本
func GetWeatherText(code int, lang Language) string {
	weatherMap := map[int]map[Language]string{
		0:  {ZH_CN: "晴", ZH_HK: "晴", EN: "Clear"},
		1:  {ZH_CN: "晴", ZH_HK: "晴", EN: "Mainly Clear"},
		2:  {ZH_CN: "多云", ZH_HK: "多雲", EN: "Partly Cloudy"},
		3:  {ZH_CN: "阴", ZH_HK: "陰", EN: "Overcast"},
		45: {ZH_CN: "雾", ZH_HK: "霧", EN: "Fog"},
		48: {ZH_CN: "雾凇", ZH_HK: "霧凇", EN: "Depositing Rime Fog"},
		51: {ZH_CN: "小毛雨", ZH_HK: "小毛雨", EN: "Light Drizzle"},
		53: {ZH_CN: "中等毛雨", ZH_HK: "中等毛雨", EN: "Moderate Drizzle"},
		55: {ZH_CN: "密集毛雨", ZH_HK: "密集毛雨", EN: "Dense Drizzle"},
		56: {ZH_CN: "轻微冻毛雨", ZH_HK: "轻微凍毛雨", EN: "Light Freezing Drizzle"},
		57: {ZH_CN: "密集冻毛雨", ZH_HK: "密集凍毛雨", EN: "Dense Freezing Drizzle"},
		61: {ZH_CN: "小雨", ZH_HK: "小雨", EN: "Slight Rain"},
		63: {ZH_CN: "中雨", ZH_HK: "中雨", EN: "Moderate Rain"},
		65: {ZH_CN: "大雨", ZH_HK: "大雨", EN: "Heavy Rain"},
		66: {ZH_CN: "轻微冻雨", ZH_HK: "轻微凍雨", EN: "Light Freezing Rain"},
		67: {ZH_CN: "强冻雨", ZH_HK: "強凍雨", EN: "Heavy Freezing Rain"},
		71: {ZH_CN: "小雪", ZH_HK: "小雪", EN: "Slight Snow"},
		73: {ZH_CN: "中雪", ZH_HK: "中雪", EN: "Moderate Snow"},
		75: {ZH_CN: "大雪", ZH_HK: "大雪", EN: "Heavy Snow"},
		77: {ZH_CN: "霰", ZH_HK: "霰", EN: "Snow Grains"},
		80: {ZH_CN: "小阵雨", ZH_HK: "小陣雨", EN: "Slight Rain Showers"},
		81: {ZH_CN: "中阵雨", ZH_HK: "中陣雨", EN: "Moderate Rain Showers"},
		82: {ZH_CN: "大阵雨", ZH_HK: "大陣雨", EN: "Violent Rain Showers"},
		85: {ZH_CN: "小阵雪", ZH_HK: "小陣雪", EN: "Slight Snow Showers"},
		86: {ZH_CN: "大阵雪", ZH_HK: "大陣雪", EN: "Heavy Snow Showers"},
		95: {ZH_CN: "雷阵雨", ZH_HK: "雷陣雨", EN: "Thunderstorm"},
		96: {ZH_CN: "小冰雹", ZH_HK: "小冰雹", EN: "Thunderstorm with Slight Hail"},
		99: {ZH_CN: "大冰雹", ZH_HK: "大冰雹", EN: "Thunderstorm with Heavy Hail"},
	}

	if textMap, ok := weatherMap[code]; ok {
		return textMap[lang]
	}
	// 如果遇到未定义的 Code，返回原始 ID 以便调试
	return "Unknown"
}

// GetWindDirectionText 将角度 (0-360) 转换为多语言风向
func GetWindDirectionText(degrees float64, lang Language) string {
	// 确保角度在 0-360 之间
	deg := math.Mod(degrees, 360)
	if deg < 0 {
		deg += 360
	}

	// 8 方位定义
	windDirs := []map[Language]string{
		{ZH_CN: "北风", ZH_HK: "北風", EN: "North"},
		{ZH_CN: "东北风", ZH_HK: "東北風", EN: "Northeast"},
		{ZH_CN: "东风", ZH_HK: "東風", EN: "East"},
		{ZH_CN: "东南风", ZH_HK: "東南風", EN: "Southeast"},
		{ZH_CN: "南风", ZH_HK: "南風", EN: "South"},
		{ZH_CN: "西南风", ZH_HK: "西南風", EN: "Southwest"},
		{ZH_CN: "西风", ZH_HK: "西風", EN: "West"},
		{ZH_CN: "西北风", ZH_HK: "西北風", EN: "Northwest"},
	}

	// 计算索引：(角度 + 22.5度偏移) / 45度一个区间
	index := int((deg+22.5)/45.0) % 8
	return windDirs[index][lang]
}

// WeatherInfo 对应你前端 state.weatherInfo 的完整结构
type TodayWeatherInfo struct {
	Temperature         float64   `json:"temperature"`
	ApparentTemperature float64   `json:"apparentTemperature"`
	WindSpeed           float64   `json:"windSpeed"`
	WindDirection       string    `json:"windDirection"`
	WindDirectionNum    float64   `json:"windDirectionNum"`
	Humidity            float64   `json:"humidity"`
	Visibility          float64   `json:"visibility"`
	WeatherCode         int       `json:"weatherCode"`
	Weather             string    `json:"weather"`         // 翻译后的描述
	DaysTemperature     []float64 `json:"daysTemperature"` // [min, max]
	Precipitation       float64   `json:"precipitation"`

	FutureTemp float64 // 2小时后温度
	FutureCond string  // 2小时后天气描述
	FutureVis  float64 // 2小时后能见度
}

// GetFullWeather 完全还原前端的逻辑与转换
func GetTodayWeather(lat, lon float64) (*TodayWeatherInfo, error) {

	now := time.Now()
	currentHour := now.Hour()

	apiURL := fmt.Sprintf(
		"https://api.open-meteo.com/v1/forecast?latitude=%f&longitude=%f&current=temperature_2m,weather_code,relative_humidity_2m,wind_speed_10m,apparent_temperature,wind_direction_10m,visibility,precipitation&hourly=temperature_2m,weather_code&forecast_days=2",
		lat, lon,
	)
	if conf.Config.Server.Mode == "debug" {
		apiURL = fmt.Sprintf(
			conf.Config.ToolsApiUrl+
				// "http://192.168.204.130:23201"+
				`/api/v1/net/httpProxy?method=GET&url=%s`,
			url.QueryEscape(apiURL))
	}

	resp, err := http.Get(apiURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	// OpenMeteoRaw 对应 API 返回的原始 JSON 结构
	type OpenMeteoRaw struct {
		Current struct {
			Temperature2m       float64 `json:"temperature_2m"`
			ApparentTemperature float64 `json:"apparent_temperature"`
			WeatherCode         int     `json:"weather_code"`
			RelativeHumidity2m  float64 `json:"relative_humidity_2m"`
			WindSpeed10m        float64 `json:"wind_speed_10m"`
			WindDirection10m    float64 `json:"wind_direction_10m"`
			Visibility          float64 `json:"visibility"`
			Precipitation       float64 `json:"precipitation"`
		} `json:"current"`
		Hourly struct {
			Temperature2m []float64 `json:"temperature_2m"`
			WeatherCode   []int     `json:"weather_code"` // 新增
		} `json:"hourly"`
	}

	raw := new(OpenMeteoRaw)

	if conf.Config.Server.Mode == "debug" {

		type ProxyResponse struct {
			Code int `json:"code"`
			Data *OpenMeteoRaw
		}

		var proxyResult ProxyResponse
		if err := json.Unmarshal(body, &proxyResult); err != nil {
			return nil, fmt.Errorf("json decode error: %v", err)
		}

		if proxyResult.Code == 200 {
			raw = proxyResult.Data
		}
		log.Info("proxyResult", proxyResult)

	} else {

		if err := json.Unmarshal(body, raw); err != nil {
			return nil, fmt.Errorf("json decode error: %v", err)
		}

	}

	// 1. 基础字段映射 (还原前端默认值逻辑)
	wi := &TodayWeatherInfo{
		Temperature:         raw.Current.Temperature2m,
		ApparentTemperature: raw.Current.ApparentTemperature,
		// 前端逻辑: (speed / 3.6).toFixed(1)
		WindSpeed:        math.Round((raw.Current.WindSpeed10m/3.6)*10) / 10,
		WindDirectionNum: raw.Current.WindDirection10m,
		Humidity:         raw.Current.RelativeHumidity2m,
		Visibility:       raw.Current.Visibility,
		WeatherCode:      raw.Current.WeatherCode,
		Precipitation:    raw.Current.Precipitation,
	}

	// 2. 天气文本翻译 (还原前端 t('weather' + code) 逻辑)
	wi.Weather = GetWeatherText(wi.WeatherCode, ZH_CN)

	// 3. 风向文本转换 (还原前端 wd 判断逻辑)
	wi.WindDirection = GetWindDirectionText(wi.WindDirectionNum, ZH_CN)

	// 4. 温差逻辑 (还原前端 slice(12, 36) 或 (0, 24) 的逻辑)
	// 前端根据 UTC 小时判断取哪一段 hourly 数据
	var tempSlice []float64
	if currentHour >= 0 && currentHour < 12 {
		if len(raw.Hourly.Temperature2m) >= 24 {
			tempSlice = raw.Hourly.Temperature2m[0:24]
		}
	} else {
		if len(raw.Hourly.Temperature2m) >= 36 {
			tempSlice = raw.Hourly.Temperature2m[12:36]
		}
	}

	if len(tempSlice) > 0 {
		min, max := getMinMax(tempSlice)
		wi.DaysTemperature = []float64{min, max}
	} else {
		wi.DaysTemperature = []float64{0, 0}
	}

	targetHourIdx := currentHour + 1 // 定位到 1 小时后

	// 安全检查：确保索引不越界
	if len(raw.Hourly.Temperature2m) > targetHourIdx {
		wi.FutureTemp = raw.Hourly.Temperature2m[targetHourIdx]

		// 转换未来天气的文字描述
		futureCode := raw.Hourly.WeatherCode[targetHourIdx]
		wi.FutureCond = GetWeatherText(futureCode, ZH_CN)

	} else {
		// 兜底方案：如果没有未来数据，参考当前数据
		wi.FutureTemp = wi.Temperature
		wi.FutureCond = wi.Weather
		wi.FutureVis = wi.Visibility
	}

	return wi, nil
}

// 辅助：获取数组最值
func getMinMax(slice []float64) (float64, float64) {
	if len(slice) == 0 {
		return 0, 0
	}
	min, max := slice[0], slice[0]
	for _, v := range slice {
		if v < min {
			min = v
		}
		if v > max {
			max = v
		}
	}
	return min, max
}

// WeatherInfo 增强版：包含完整预报信息
type FullWeatherInfo struct {
	// --- 当前实况 ---
	Temperature         float64 `json:"temperature"`
	ApparentTemperature float64 `json:"apparentTemperature"`
	WindSpeed           float64 `json:"windSpeed"`
	WindDirection       string  `json:"windDirection"`
	WindDirectionNum    float64 `json:"windDirectionNum"`
	Humidity            float64 `json:"humidity"`
	Visibility          float64 `json:"visibility"`
	Pressure            float64 `json:"pressure"` // 新增：气压
	WeatherCode         int     `json:"weather_code"`
	Weather             string  `json:"weather"`
	Precipitation       float64 `json:"precipitation"` // 降水量

	// --- 24小时逐小时预报 ---
	Hourly []HourlyForecast `json:"hourly"`

	// --- 7天逐日预报 ---
	Daily []DailyForecast `json:"daily"`
}

type HourlyForecast struct {
	Time        string  `json:"time"`
	Temperature float64 `json:"temperature"`
	Weather     string  `json:"weather"`
	WeatherCode int     `json:"weather_code"`
}

type DailyForecast struct {
	Date        string  `json:"date"`
	MaxTemp     float64 `json:"max_temp"`
	MinTemp     float64 `json:"min_temp"`
	Weather     string  `json:"weather"`
	WeatherCode int     `json:"weather_code"`
}

func GetFullWeather(lat, lon float64) (*FullWeatherInfo, error) {
	// 构建增强版 URL
	// current 增加 pressure_msl
	// hourly 保持获取 (Open-Meteo 默认返回 7 天，我们后续切片取 24 小时)
	// daily 增加 7 天预报所需字段
	apiURL := fmt.Sprintf(
		"https://api.open-meteo.com/v1/forecast?latitude=%f&longitude=%f"+
			"&current=temperature_2m,weather_code,relative_humidity_2m,wind_speed_10m,apparent_temperature,wind_direction_10m,visibility,precipitation,pressure_msl"+
			"&hourly=temperature_2m,weather_code"+
			"&daily=weather_code,temperature_2m_max,temperature_2m_min&timezone=auto",
		lat, lon,
	)

	// 调试模式代理逻辑保持不变
	if conf.Config.Server.Mode == "debug" {
		apiURL = fmt.Sprintf("%s/api/v1/net/httpProxy?method=GET&url=%s",
			conf.Config.ToolsApiUrl, url.QueryEscape(apiURL))
	}

	resp, err := http.Get(apiURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	// OpenMeteoRaw 对应 API 增加 daily 字段
	type OpenMeteoRaw struct {
		Current struct {
			Temperature2m       float64 `json:"temperature_2m"`
			ApparentTemperature float64 `json:"apparent_temperature"`
			WeatherCode         int     `json:"weather_code"`
			RelativeHumidity2m  float64 `json:"relative_humidity_2m"`
			WindSpeed10m        float64 `json:"wind_speed_10m"`
			WindDirection10m    float64 `json:"wind_direction_10m"`
			Visibility          float64 `json:"visibility"`
			Precipitation       float64 `json:"precipitation"`
			PressureMsl         float64 `json:"pressure_msl"` // 气压
		} `json:"current"`
		Hourly struct {
			Time          []string  `json:"time"`
			Temperature2m []float64 `json:"temperature_2m"`
			WeatherCode   []int     `json:"weather_code"`
		} `json:"hourly"`
		Daily struct {
			Time             []string  `json:"time"`
			WeatherCode      []int     `json:"weather_code"`
			Temperature2mMax []float64 `json:"temperature_2m_max"`
			Temperature2mMin []float64 `json:"temperature_2m_min"`
		} `json:"daily"`
	}
	raw := new(OpenMeteoRaw)
	// JSON 解析逻辑 (兼容你的 ProxyResponse 结构)
	if conf.Config.Server.Mode == "debug" {
		type ProxyResponse struct {
			Code int           `json:"code"`
			Data *OpenMeteoRaw `json:"data"`
		}
		var proxyResult ProxyResponse
		if err := json.Unmarshal(body, &proxyResult); err != nil {
			return nil, err
		}
		raw = proxyResult.Data
	} else {
		if err := json.Unmarshal(body, raw); err != nil {
			return nil, err
		}
	}

	if raw == nil {
		return nil, fmt.Errorf("failed to get weather data")
	}

	// 1. 组装当前实况
	wi := &FullWeatherInfo{
		Temperature:         raw.Current.Temperature2m,
		ApparentTemperature: raw.Current.ApparentTemperature,
		WindSpeed:           math.Round((raw.Current.WindSpeed10m/3.6)*10) / 10,
		WindDirectionNum:    raw.Current.WindDirection10m,
		WindDirection:       GetWindDirectionText(raw.Current.WindDirection10m, ZH_CN),
		Humidity:            raw.Current.RelativeHumidity2m,
		Visibility:          raw.Current.Visibility,
		Pressure:            raw.Current.PressureMsl,
		WeatherCode:         raw.Current.WeatherCode,
		Weather:             GetWeatherText(raw.Current.WeatherCode, ZH_CN),
		Precipitation:       raw.Current.Precipitation,
	}

	// 2. 处理最近 24 小时预报
	// 找到当前时间对应的索引（或直接从当前小时开始取 24 个）
	nowLocal := time.Now()
	startIdx := nowLocal.Hour()
	for i := 0; i < 24; i++ {
		idx := startIdx + i
		if idx < len(raw.Hourly.Temperature2m) {
			wi.Hourly = append(wi.Hourly, HourlyForecast{
				Time:        raw.Hourly.Time[idx],
				Temperature: raw.Hourly.Temperature2m[idx],
				WeatherCode: raw.Hourly.WeatherCode[idx],
				Weather:     GetWeatherText(raw.Hourly.WeatherCode[idx], ZH_CN),
			})
		}
	}

	// 3. 处理最近 7 天预报
	for i := 0; i < len(raw.Daily.Time); i++ {
		wi.Daily = append(wi.Daily, DailyForecast{
			Date:        raw.Daily.Time[i],
			MaxTemp:     raw.Daily.Temperature2mMax[i],
			MinTemp:     raw.Daily.Temperature2mMin[i],
			WeatherCode: raw.Daily.WeatherCode[i],
			Weather:     GetWeatherText(raw.Daily.WeatherCode[i], ZH_CN),
		})
	}

	return wi, nil
}
