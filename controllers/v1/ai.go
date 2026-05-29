package controllersV1

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	conf "github.com/ShiinaAiiko/nyanya-trip-route-track/server/config"
	dbxV1 "github.com/ShiinaAiiko/nyanya-trip-route-track/server/dbx/v1"
	"github.com/ShiinaAiiko/nyanya-trip-route-track/server/protos"
	"github.com/ShiinaAiiko/nyanya-trip-route-track/server/services/methods"
	"github.com/ShiinaAiiko/nyanya-trip-route-track/server/services/response"
	"github.com/cherrai/nyanyago-utils/narrays"
	"github.com/cherrai/nyanyago-utils/ncommon"
	"github.com/cherrai/nyanyago-utils/nstrings"
	"github.com/cherrai/nyanyago-utils/validation"
	sso "github.com/cherrai/saki-sso-go"
	"github.com/gin-gonic/gin"
	"github.com/sashabaranov/go-openai"
)

// "github.com/cherrai/nyanyago-utils/validation"

var (
	aiDbx         = dbxV1.AIDbx{}
	poiDbx        = new(dbxV1.POIDbx)
	appDbx        = new(dbxV1.AppDbx)
	tripMemoryDbx = new(dbxV1.RgaTripMemoryDbx)
)

type AIController struct {
}

type TripStatistics struct {
	TotalDistanceKm float64 `json:"totalDist_km,omitempty"`
	MaxSpeedKmh     float64 `json:"maxSpeed_kmh,omitempty"`
	AvgSpeedKmh     float64 `json:"avgSpeed_kmh,omitempty"`
	MaxAltitudeM    float64 `json:"maxAlt_m,omitempty"`
	MinAltitudeM    float64 `json:"minAlt_m,omitempty"`
	AvgAltitudeM    float64 `json:"avgAlt_m,omitempty"`
	TotalClimbM     float64 `json:"climb_m,omitempty"`
	TotalDescendM   float64 `json:"descend_m,omitempty"`
}

// TripData 对应具体的行程实时数据 (语义化字段名)
type TripData struct {
	City      string  `json:"city,omitempty"`
	AltitudeM float64 `json:"alt_m,omitempty"`
	SpeedKmh  float64 `json:"speed_kmh,omitempty"`
	// Weather   string  `json:"weather"`
	// TempC     float64 `json:"temp_c"`
	RoadName string  `json:"road,omitempty"`
	Time     string  `json:"time,omitempty"`
	Lat      float64 `json:"lat,omitempty"`
	Lng      float64 `json:"lng,omitempty"`
}

// TripMemoryItem 历史行程记忆 (精简 Key 名)
// type TripMemoryItem struct {
// 	VisitTime string  `json:"time"`    // 上次经过时间
// 	Location  string  `json:"loc"`     // 地点名称
// 	Summary   string  `json:"summary"` // RGA行程摘要
// 	DistanceM float64 `json:"dist_m"`  // 距离当前坐标的距离
// }

// POIsItem 周边兴趣点 (语义化)
// type POIsItem struct {
// 	Name        string  `json:"name"`
// 	WikiSummary string  `json:"wiki,omitempty"` // 百科简介
// 	DistanceM   float64 `json:"dist_m"`         // 距离当前位置
// }

// WeatherItem 对应 ContextDataItem 里的 Data 序列化对象
// type WeatherItem struct {
// 	// 当前天气
// 	NowCond  string  `json:"now_cond"` // 现象：晴/大雨/浓雾/雪
// 	NowTempC float64 `json:"now_c"`    // 当前温度
// 	// NowVisM  float64 `json:"now_vis"`  // 当前能见度(米)，安全关键指标

// 	// 未来2小时趋势
// 	F1hCond  string  `json:"f1h_cond"` // 2小时后现象
// 	F1hTempC float64 `json:"f1h_c"`    // 2小时后温度
// 	// F2hVisM  float64 `json:"f2h_vis"`  // 2小时后能见度

// 	// 预警标签 (由后端逻辑直接给出，降低 AI 推理压力)
// 	// AlertTag string `json:"alert"` // 如：大风预警/道路结冰/无
// }

// // ContextDataItem 动态注入的数据项 (保持通用性)
// type ContextDataItem struct {
// 	Type string `json:"type"` // 如: "POIs", "Memory"
// 	Data any    `json:"data"` // 存储序列化后的 POIsItem 或 TripMemoryItem 数组
// }

// UserInputParams 根对象 (极致精简协议)
type UserInputParams struct {
	IsDriving       bool                     `json:"isDriving,omitempty"` // 核心约束判定：是否在驾驶
	CurrentTripData *TripData                `json:"arrival,omitempty"`   // 当前数据
	LastTripData    *TripData                `json:"departure,omitempty"` // 上一阶段数据，用于对比
	Stats           *TripStatistics          `json:"stats,omitempty"`     // 统计数据
	SystemTime      string                   `json:"sysTime,omitempty"`   // 系统时间
	ContextHint     []string                 `json:"hints,omitempty"`     // 上下文引导
	ContextData     []*dbxV1.ContextDataItem `json:"data,omitempty"`      // 动态注入的 POIs/Memory
	// UserMessage     string             `json:"userMsg,omitempty"`         // 用户主动输入
}

func (fc *AIController) AICoDriver(c *gin.Context) {

	// 1、请求体
	var res response.ResponseProtobufType
	res.Code = 200

	// 2、获取参数
	data := new(protos.AICoDriver_Request)
	var err error
	if err = protos.DecodeBase64(c.GetString("data"), data); err != nil {
		res.Error = err.Error()
		res.Code = 10002
		res.Call(c)
		return
	}

	log.Info(len(data.Message))
	// 3、验证参数
	if err = validation.ValidateStruct(
		data,
		validation.Parameter(&data.StartTrip),
		validation.Parameter(&data.TriggerReason, validation.Type("string")),
		validation.Parameter(&data.Message, validation.Length(1, 150), validation.Required()),
	); err != nil {
		res.Errors(err)
		res.Code = 10002
		res.Call(c)
		return
	}

	userInfoAny, exists := c.Get("userInfo")
	if !exists {
		res.Errors(err)
		res.Code = 10004
		res.Call(c)
		return
	}
	userInfo := userInfoAny.(*sso.UserInfo)
	// log.Info("userInfo", userInfo)
	dataJ, _ := json.MarshalIndent(data, "", "  ")
	log.Info(string(dataJ))

	if data.SessionId == "" {
		data.SessionId = aiDbx.GetSessionId()
	}

	// log.Info("data", data)

	isDev := false
	if isDev {
		fc.testSSE(c, &res)
		return
	}

	tokenUsage := &protos.AITokenUsage{}

	contextData := []*dbxV1.ContextDataItem{}

	type RoutingStrategy struct {
		BestMatch string // 匹配到的意图标签 (e.g., "STATS", "CHAT")
	}

	routingStrategy := RoutingStrategy{}

	// 1、意图语义路由：前置分流，精准降噪。
	if data.StartTrip && data.TriggerReason != "" {
		// AI_CO_DRIVER
		// [塞数据] 提前调用RGA 行程记忆
		routingStrategy.BestMatch = "AI_CO_DRIVER"
	} else {

		// 提取用户原始输入（假设存在 userInput 变量中）
		userInput := strings.TrimSpace(strings.ToLower(data.Message))

		// --- 第一路：关键词强匹配 (High Priority) ---
		// --- 1. 关键词拦截器 (Hard Match) ---
		if methods.ContainsKeywords(userInput, "闭嘴", "别说话", "静音") {
			routingStrategy.BestMatch = "SYSTEM_SILENCE"
		} else if methods.ContainsKeywords(userInput, "报警", "救命", "求救") {
			routingStrategy.BestMatch = "EMERGENCY"
		} else {

			// --- 第二路：向量语义评分 (Semantic Intelligence) ---
			// noiseWords := []string{"我", "你", "咱们", "咱", "你觉得", "我想问", "请问", "呢", "吗", "呀", "吧", "哈", "噢", "了"}
			// for _, word := range noiseWords {
			// 	userInput = strings.ReplaceAll(userInput, word, "")
			// }

			if conf.Config.Server.Mode == "debug" {
				userVec, err := aiDbx.GetEmbedding(
					dbxV1.TextTypeQuery, aiDbx.CleanseInput(userInput),
				)
				if err != nil {
					log.Error(err)
					res.Errors(err)
					res.Code = 10001
					res.Call(c)
					return
				}

				// --- 向量评分逻辑开始 ---
				// var maxScore float32 = -1.0
				// var bestMatchCategory = "INTENT_CHAT" // 默认闲聊
				// var baseExampleVec *dbxV1.IntentVectorsItem

				topResults, err := aiDbx.SearchIntent(c.Request.Context(), userVec)
				if err != nil {
					log.Error(err)
					res.Errors(err)
					res.Code = 10001
					res.Call(c)
					return
				}

				routingStrategy.BestMatch = aiDbx.ArbitrateIntent(userInput, topResults)
				log.Info("用户输入:", data.Message, "ArbitrateIntent判定结果:", routingStrategy.BestMatch, topResults)

			}
		}

	}

	spMap := struct {
		Role string
		// TriggerReason   string
		Constraints string
		// Input           string
		// SkillsBehaviors string
		ThoughtProcess       string
		OutputProtocol       string
		AppManifest          string
		InteractionGuideline string
	}{
		Role: `# Role: AI 领航员 Agent
资深自驾专家。回复准则：场景优先、逻辑闭环、表达极简。`,
		// 		TriggerReason: `
		// 行驶中，根据实时数据变化而触发`,
		Constraints: `
1. **数据驱动**：仅基于输入的 JSON 数据进行实时分析。
2. **精简交互**：行车中极简回复(60字以内)。
3. **深度洞察**：避免复读数据，解释背后的地理/人文/安全意义。`,

		// 		Input: `
		// {
		// 	"startTrip": boolean, // 是否已开始行程、以此判断是否在驾驶
		// 	"currentTripData": {
		// 		"city": "城市",
		// 		"altitude": "海拔(m)",
		// 		"speed": "时速(km/h)",
		// 		"weather": "当前天气",
		// 		"temp": "当前温度",
		// 		"road": "当前道路信息",
		// 		"time": "当前时间",
		// 		"lat": "当前维度",
		// 		"lng": "当前经度",
		// 	}, // 当前行程实时数据
		// 	"lastTripData": Object, // 上一段行程数据，类型和currentTripData一致
		// 	"tripStatistics": {
		// 		"distance": "累计行驶的总里程 (单位: km)",
		// 		"maxSpeed": "最高速度 (单位: km/h)",
		// 		"averageSpeed": "平均速度 (单位: km/h)",
		// 		"maxAltitude": "最高海拔点 (单位: m)",
		// 		"minAltitude": "最低海拔点 (单位: m)",
		// 		"averageAltitude": "平均海拔高度 (单位: m)",
		// 		"climbAltitude": "累计爬升高度 (单位: m)",
		// 		"descendAltitude": "累计下降高度 (单位: m)"
		// 	}, // 本次行程的全局统计数据，用于分析地形挑战与驾驶成就
		// 	"systemTime":"当前系统时间",
		// 	"contextHint":"上下文提示",
		// 	"contextData":[{
		// 		"type":"", // 数据类型：POIs / Statistics / Weather 等等
		// 		"data":"数据",
		// 		"fieldDesc":"data字段类型描述",
		// 	}], // 注入的相关数据,
		// 	"message":"用户主动输入的信息"
		// }`,

		// 		Input: `{
		//   "isDriving": "boolean", // 核心状态：若为 true 必须执行行车精简回复约束
		//   "currentTripData": {
		//     "city": "string",
		//     "alt_m": "number",      // 当前海拔(米)
		//     "speed_kmh": "number",  // 当前时速(km/h)
		//     "road": "string",       // 当前道路名
		//     "time": "string",       // 数据采集时间
		//     "lat": "number",
		//     "lng": "number"
		//   },
		//   "lastTripData": 同currentTripData
		//   "stats": {
		//     "totalDist_km": "number", // 累计行驶里程
		//     "maxSpeed_kmh": "number",
		//     "avgSpeed_kmh": "number",
		//     "maxAlt_m": "number",     // 历史最高海拔
		//     "minAlt_m": "number",
		//     "avgAlt_m": "number",
		//     "climb_m": "number",      // 累计爬升
		//     "descend_m": "number"     // 累计下降
		//   },
		//   "sysTime": "string", // 当前系统基准时间
		//   "hints": ["string"], // 上下文引导策略 (如: REVISIT_MOMENT, POI_DEEP_INSIGHT)
		//   "data": [
		//     {
		//       "type": "string", // "POIs" 或 "RGA-TripMemory"
		//       "data": "JSON_STRING" // 嵌套的 POIsItem 或 TripMemoryItem 数组
		//     }
		//   ],
		//   "userMsg": "string" // 用户主动输入的文字内容
		// }`,
		// 		SkillsBehaviors: `
		// - **安全/环境**：监测海拔变化速度预防高反；根据下坡/城市环境给出驾驶建议。
		// - **深度人文**：利用 RAG 提供坐标相关的地理、历史、美食推荐。
		// - **记忆**：若 tripMemory 不为空，必须将其作为核心谈资。严禁生硬复读，需以“重地重游”的视角。`,
		ThoughtProcess: `
仅记录业务逻辑干货（如：高海拔路段需预警），严禁分析步骤。简单任务必须为空。
`,
		OutputProtocol: `
## 你必须输出且仅输出一个 JSON 对象，结构如下：
{
  "code": 200, // 成功:200 | 失败:10001
  "status": { 
		"isRelevant": true, // 核心业务为true
		"isSafetyFenced": false, // 触发敏感话题为true
	},
	"actionId":"String", // AppFeatures Id，无则 ""
  "display": {
    "message": "Markdown String",
    "warning":"Markdown String" // 风险预警（如高反、长途疲劳、路况风险），可为空
  }
}

## JSON Constraints
1. **语法要求**："message" 和 "warning" 必须包含 Markdown 强调语法（如使用 "**" 加粗关键词。
2. **转义要求**：严禁物理换行，内容中 "\n" 和 "\"" 必须严格转义，确保 JSON 可解析。
3. **内容定义**："message" 为口语化总结；"warning" 为风险/环境预警（无则空）。
`,
		// 		OutputProtocol: `
		// ## 你必须输出且仅输出一个 JSON 对象，结构如下：
		// {
		//   "code": number, // 成功:200, 失败:10001
		//   "status": {
		// 		"isRelevant": boolean, // 核心业务为true，无关内容为false
		// 		"isSafetyFenced": boolean, // 严禁讨论的敏感话题为true，否则为false
		// 	},
		//   "display": {
		//     "message": string, // markdown内容，相关：自家博主风口语化总结；无关：一句话终结对话
		//     "warning": string  // markdown内容，风险预警（如高反、长途疲劳、路况风险），若无则空字符串
		//   }
		// }
		// ## JSON 稳定性约束：
		// 	 - 所有的 Markdown 内容，换行必须在 JSON 字符串中转义为 \n，严禁产生真实的物理换行。
		// 	 - 字符串内部的双引号 " 必须转义为 \"，以防破坏 JSON 结构。
		// 	 - 严禁使用原生 HTML 标签，仅允许标准的 Markdown 符号。

		// # Constraints
		// - 严禁输出 JSON 外的任何文字。
		// - 严禁在 JSON 结构外添加任何 Markdown 标签（如 json ）
		// `,

		InteractionGuideline: `
# 交互准则：引导式对话协议
1. **意图模糊判定**：
   当用户提出宽泛需求（如“推荐风景”、“想吃东西”、“想去哪玩”）时，禁止直接返回长列表。
   
2. **反问策略（二选一原则）**：
   - 必须先提供一个基于当前坐标的“保底选项”（即刻可达）。
   - 必须提出 1-2 个具体的维度进行反问，以缩小搜索范围。
   - 维度包括：【距离偏好：附近 vs 远方】、【路况偏好：铺装路 vs 轻越野】、【景观属性：高山、江河、古镇】。

3. **工具调用逻辑**：
   - 第一轮对话：不调用复杂工具，仅凭 Context 做初步引导。
   - 第二轮对话（用户明确意图后）：立即调用工具获取精准数据。

4. **回复模版示例**：
   “这附近最快能去的是[地点A]。如果你想看更硬核的风景，你是偏好去[维度1]还是[维度2]？我好为你精准匹配。”
`,
	}

	// TripStatistics 对应行程全局统计数据 (增加单位后缀)

	// 	if memoryCount == 0 {
	//     toneInstruction = "这是一个全新的地方，请保持好奇心，多介绍地理特征。"
	// } else if memoryCount <= 3 {
	//     toneInstruction = fmt.Sprintf("用户来过这里 %d 次，可以适当回顾上次的记录，语气要像老朋友叙旧。", memoryCount)
	// } else {
	//     // 超过 3 次，视为通勤或极度熟悉
	//     toneInstruction = "用户对这里非常熟悉。禁止使用'欢迎'、'重地重游'等词汇。请直接关注实时路况、天气或目的地信息，话术要精简、高效。"
	// }

	userMessage := data.Message
	// userMessage := ncommon.IfElse(data.StartTrip, "", data.Message)

	userInputParams := UserInputParams{
		IsDriving:  data.StartTrip,
		SystemTime: time.Now().Format("2006-01-02 15:04:05"),
		// UserMessage: ncommon.IfElse(data.StartTrip, "", data.Message),
	}

	// 2、预处理模块
	// 2.1 环境上下文提取：并发获取位置、车况、RGA等相关数据。

	// 统计数据
	if data.TriggerReason == "CHANGE_CITY" ||
		data.TriggerReason == "WEATHER_CHANGE" {
		userInputParams.CurrentTripData = &TripData{
			City:      data.CurrentTripData.City,
			AltitudeM: aiDbx.Round(data.CurrentTripData.Altitude, 1), // 海拔保留1位
			// Time: time.UnixMilli(data.CurrentTripData.Time).Format("2006-01-02 15:04:05"),
		}
		userInputParams.LastTripData = &TripData{
			City:      data.LastTripData.City,
			AltitudeM: aiDbx.Round(data.LastTripData.Altitude, 1), // 海拔保留1位
			// Time: time.UnixMilli(data.LastTripData.Time).Format("2006-01-02 15:04:05"),
		}

	} else if data.TriggerReason == "TEMPERATURE_DROP" {
		userInputParams.CurrentTripData = &TripData{
			City:      data.CurrentTripData.City,
			RoadName:  data.CurrentTripData.Road,
			AltitudeM: aiDbx.Round(data.CurrentTripData.Altitude, 1), // 海拔保留1位
			// Time: time.UnixMilli(data.CurrentTripData.Time).Format("2006-01-02 15:04:05"),
		}
		userInputParams.LastTripData = &TripData{
			City:      data.LastTripData.City,
			RoadName:  data.LastTripData.Road,
			AltitudeM: aiDbx.Round(data.LastTripData.Altitude, 1), // 海拔保留1位
			// Time: time.UnixMilli(data.LastTripData.Time).Format("2006-01-02 15:04:05"),
		}

	} else {
		if data.CurrentTripData != nil {

			// 赋值逻辑修改
			userInputParams.CurrentTripData = &TripData{
				City:      data.CurrentTripData.City,
				AltitudeM: aiDbx.Round(data.CurrentTripData.Altitude, 1),  // 海拔保留1位
				SpeedKmh:  aiDbx.Round(data.CurrentTripData.Speed*3.6, 1), // 速度保留1位
				// TempC:     aiDbx.Round(data.CurrentTripData.Temp, 1),             // 温度保留1位
				Lat: aiDbx.Round(data.CurrentTripData.Coords.Latitude, 6),  // 纬度保留6位
				Lng: aiDbx.Round(data.CurrentTripData.Coords.Longitude, 6), // 经度保留6位

				RoadName: data.CurrentTripData.Road,
				Time:     ncommon.IfElse(data.CurrentTripData.Time > 0, time.UnixMilli(data.CurrentTripData.Time).Format("2006-01-02 15:04:05"), ""),
			}

			if data.CurrentTripData.Statistics != nil {
				userInputParams.Stats = &TripStatistics{
					// 1. 里程：假设 methods.FormatDistance 返回的是米，我们转为 km 并保留 1 位小数
					// 如果原本就是 km，直接赋值即可
					TotalDistanceKm: aiDbx.Round(data.CurrentTripData.Statistics.Distance/1000, 1),

					// 2. 速度：转为 km/h 并保留 1 位小数
					MaxSpeedKmh: aiDbx.Round(data.CurrentTripData.Statistics.MaxSpeed*3.6, 1),
					AvgSpeedKmh: aiDbx.Round(data.CurrentTripData.Statistics.AverageSpeed*3.6, 1),

					// 3. 海拔相关：统一保留 1 位小数，直接传入数字
					MaxAltitudeM:  aiDbx.Round(data.CurrentTripData.Statistics.MaxAltitude, 1),
					MinAltitudeM:  aiDbx.Round(data.CurrentTripData.Statistics.MinAltitude, 1),
					AvgAltitudeM:  aiDbx.Round(data.CurrentTripData.Statistics.AverageAltitude, 1),
					TotalClimbM:   aiDbx.Round(data.CurrentTripData.Statistics.ClimbAltitude, 1),
					TotalDescendM: aiDbx.Round(data.CurrentTripData.Statistics.DescendAltitude, 1),
				}
			}
		}

		if data.LastTripData != nil {
			userInputParams.LastTripData = &TripData{
				City:      data.LastTripData.City,
				AltitudeM: aiDbx.Round(data.LastTripData.Altitude, 1),  // 海拔保留1位
				SpeedKmh:  aiDbx.Round(data.LastTripData.Speed*3.6, 1), // 速度保留1位
				// TempC:     aiDbx.Round( data.LastTripData.Temp, 1),             // 温度保留1位
				Lat:      aiDbx.Round(data.LastTripData.Coords.Latitude, 6),  // 纬度保留6位
				Lng:      aiDbx.Round(data.LastTripData.Coords.Longitude, 6), // 经度保留6位
				RoadName: data.LastTripData.Road,
				Time:     ncommon.IfElse(data.LastTripData.Time > 0, time.UnixMilli(data.LastTripData.Time).Format("2006-01-02 15:04:05"), ""),
			}
		}

	}

	// 获取城市百科
	if data.TriggerReason == "CHANGE_CITY" {

		cities := strings.Split(data.CurrentTripData.City, "·")
		narrays.Reverse(&cities)

		log.Info("cities", cities)

		wikiPage, err := methods.GetCityWikiSummary(c.Request.Context(),
			cities)
		if err != nil {
			res.Errors(err)
			res.Code = 10022
			res.Call(c)
			return
		}

		if wikiPage != nil && wikiPage.Extract != "" {
			cdt := dbxV1.ContextDataItem{
				Type: "WikiSummary",
				Data: wikiPage.Extract,
			}
			userInputParams.ContextData = append(userInputParams.ContextData, &cdt)
		}
	}

	// 如果是查询天气，直接走详细接口
	if routingStrategy.BestMatch == "AI_CO_DRIVER" {
		wi, err := methods.GetFullWeather(data.CurrentTripData.Coords.Latitude, data.CurrentTripData.Coords.Longitude)

		// j, _ := json.MarshalIndent(wi, "", "  ")
		// log.Info("GetFullWeather wi", string(j))

		if err != nil {
			res.Errors(err)
			res.Code = 10022
			res.Call(c)
			return
		}

		// 2. 获取下一个小时的数据
		weatherData := dbxV1.WeatherItem{
			NowCond:  wi.Weather,
			NowTempC: aiDbx.Round(wi.Temperature, 1),
		}
		nextStr := time.Now().Add(1 * time.Hour).Format("2006-01-02T15:00")

		for _, v := range wi.Hourly {
			if v.Time == nextStr {
				weatherData.F1hTempC = v.Temperature
				weatherData.F1hCond = v.Weather
			}
		}

		userInputParams.ContextData = append(userInputParams.ContextData, &dbxV1.ContextDataItem{
			Type: "Weather",
			Data: weatherData,
		})
	}

	// ||
	// 	routingStrategy.BestMatch == "INTENT_GEOGRAPHY"
	if routingStrategy.BestMatch == "AI_CO_DRIVER" {

		poisLen := 3
		radius := poiDbx.CalculateSearchRadius(data.CurrentTripData.Altitude)

		if data.TriggerReason == "CHANGE_CITY" {
			radius = 10 * 1000
			poisLen = 5
		}

		// POIs 获取
		poiPoints, err := poiDbx.SearchPOI(c.Request.Context(), dbxV1.POISearchParams{
			Lat:     &data.CurrentTripData.Coords.Latitude,
			Lon:     &data.CurrentTripData.Coords.Longitude,
			Heading: &data.CurrentTripData.Coords.Heading,
			Radius:  radius,
			Limit:   10,
		})
		if err != nil {
			res.Errors(err)
			res.Code = 10021
			res.Call(c)
			return
		}

		log.Info("poiPoints", len(poiPoints))

		var pois []*dbxV1.POIsItem
		if len(poiPoints) > 0 {
			for _, v := range poiPoints {
				if v.POI != nil && len(pois) < poisLen {
					pois = append(pois, &dbxV1.POIsItem{
						Name:        v.POI.Name,
						WikiSummary: v.POI.Wiki.Summary,
						DistanceM:   aiDbx.Round(v.POI.Distance, 1),
						Direction:   v.POI.Direction,
						BearingDeg:  v.POI.Bearing,
					})
				}
			}
		}

		if len(pois) > 0 {
			// 场景：探索新境
			userInputParams.ContextHint = append(userInputParams.ContextHint,
				"POI_DEEP_INSIGHT: 深度挖掘景观的地理成因，引导用户沉浸式观察。")

			cdt := dbxV1.ContextDataItem{
				Type: "POIs",
				Data: pois,
			}

			userInputParams.ContextData = append(userInputParams.ContextData,
				&cdt)

		}

		// RGA tripMemory的获取
		// if conf.Config.Server.Mode == "debug" {
		tripMemory, err := tripMemoryDbx.SearchTripMemory(c.Request.Context(), dbxV1.SearchTripMemoryOptions{
			Coords: &dbxV1.GeoCoords{
				Lat: data.CurrentTripData.Coords.Latitude,
				Lon: data.CurrentTripData.Coords.Longitude,
			},
			Filters: map[string]interface{}{
				"author_id": userInfo.Uid,
			},
			RadiusMeters: 5000,
			Limit:        10,
			// 注意：空间检索不需要设 Threshold，因为 Score 默认为 0
		})
		log.Info("tripMemory", len(tripMemory))

		if err != nil {
			res.Errors(err)
			res.Code = 10020
			res.Call(c)
			return
		}

		if len(tripMemory) == 0 {
			// 场景：探索新境
			userInputParams.ContextHint = append(userInputParams.ContextHint,
				"NEW_LOCATION_EXPLORATION: 当前为首次到访。侧重地理科普、人文背景及周边探索，语气保持好奇与专业。")
		} else if len(tripMemory) <= 3 {
			// 场景：故地重游
			userInputParams.ContextHint = append(userInputParams.ContextHint,
				fmt.Sprintf("REVISIT_MOMENT: 用户此前到访过 %d 次。重点提取 tripMemory 中的历史细节进行情感联结，语气如朋友叙旧，避免生硬播报。", len(tripMemory)))
		} else {
			// 场景：高频通勤
			userInputParams.ContextHint = append(userInputParams.ContextHint,
				"HIGH_FREQUENCY_COMMUTE: 此地为用户高频路段。严禁使用‘欢迎、重逢’等寒暄语。禁止长篇大论，仅提供实时路况、天气突变或驾驶风险预警，保持极简助手风格。")
		}

		var items []dbxV1.TripMemoryItem
		for i, point := range tripMemory {
			if i > 0 {
				break
			}
			p := point.Payload

			// log.Info(point)

			// 1. 获取嵌套的 location 对象
			locValue, ok := p["location"]
			if !ok || locValue.GetStructValue() == nil {
				continue // 如果没有 location 字段则跳过
			}

			fields := locValue.GetStructValue().GetFields()

			histLon := fields["lon"].GetDoubleValue()
			histLat := fields["lat"].GetDoubleValue()

			roadList := p["road_name"].GetListValue().GetValues()
			roadListNames := make([]string, 0, len(roadList))

			for _, item := range roadList {
				// GetStringValue() 如果类型不对会返回空字符串
				roadListNames = append(roadListNames, item.GetStringValue())
			}

			item := dbxV1.TripMemoryItem{
				// 假设你存入时字段名分别为 time, location, summary
				VisitTime:    time.Unix(int64(p["start_time"].GetIntegerValue()), 0).Format("2006-01-02 15:04"),
				Location:     p["location_name"].GetStringValue(),
				Summary:      p["summary"].GetStringValue(),
				DistanceM:    aiDbx.Round(methods.GetGeoDistance(histLat, histLon, data.CurrentTripData.Coords.Latitude, data.CurrentTripData.Coords.Longitude), 1),
				AltitudeM:    p["alt_end"].GetDoubleValue(),
				MaxSpeedKMH:  p["max_speed"].GetDoubleValue(),
				Weather:      p["weather"].GetStringValue(),
				TemperatureC: p["temperature"].GetDoubleValue(),
				Road:         strings.Join(roadListNames, "; "),
			}
			items = append(items, item)
		}

		cdt := dbxV1.ContextDataItem{
			Type: "RGA-TripMemory",
			// 		FieldDesc: `[
			//   {
			//     "lastVisitTime": "2023-06-15 18:32", // 上次经过时间
			//     "location": "四川省·甘孜藏族自治州·巴塘县·夏邛镇",
			//     "summary": "" // RGA行程记忆摘要,
			// 		"distance": "500m" // 距离当前坐标的距离 (单位: m)
			//   }
			// ], // 历史行程记忆检索结果（RAG 注入点），不为空则作为核心谈资`,
		}
		// jsonData, err := json.Marshal(items)
		// if err != nil {
		// 	log.Error(err)
		// 	cdt.Data = "[]"
		// } else {
		// 	cdt.Data = string(jsonData)

		cdt.Data = items
		// }

		userInputParams.ContextData = append(userInputParams.ContextData, &cdt)

		// 序列化为 JSON 字符串

		log.Info("tripMemory", len(tripMemory))
		// }
	}

	// 2,2 生成对应的Prompt，多源数据预装填：注入记忆、POI、天气。

	filterToolsFunc := []string{}

	switch routingStrategy.BestMatch {
	case "AI_CO_DRIVER":

		if data.TriggerReason != "" {
			spMap.InteractionGuideline = ""

			var TriggerInstructions = map[string]string{
				"FIRST_OPEN_DISTANCE": "航线启程：定位地理起点，核算全程逻辑，简述今日地貌演变趋势及核心地标。",
				"MILESTONE_DISTANCE":  fmt.Sprintf("里程节点：实测行驶%.2fkm。分析当前位置与起点的生境差异，量化空间位移带来的视觉变化，禁止使用恭喜及感性修饰。", data.CurrentTripData.Statistics.Distance/1000),
				"ALTITUDE_JUMP":       "气压与动力：监测到海拔剧烈起伏。评估含氧量对动力系统的衰减影响，提示植被垂直分布带从[低海拔]向[高海拔]的演替规律。",
				"CLIMB_ACHIEVEMENT":   "地形突破：攻克垭口。解析当前褶皱山系或冰川地貌的形成机制，拆解典型弯道的工程学布局。",
				"DESCEND_WARNING":     "势能预警：进入长下坡路段。提示利用发动机制动减解制动热衰退，预报坡底气压升高后的体感变化及微气候差异。",
				"TEMPERATURE_DROP":    "热力演变：气温骤降。解析冷锋或辐射降温对路面附着力的影响，评估凝冻或暗冰风险，提示优化座舱环境参数。",
				"SUDDEN_STOP":         "静止状态监测：识别当前停车坐标。若为景观区，解析周边地质构造或人文历史背景；若为非预定停车，启动安全冗余提示。",
				"CHANGE_ROAD":         "路径切变：切换行驶序列。对比新旧路网的铺装等级、设计时速及历史筑路背景，定位核心航向转折点。",
				"CHANGE_CITY":         "行政边界穿越：捕获跨行政区瞬间。对比两地文化图腾、建筑形制演变及方言地理分界线，量化行政区划间的海拔梯度。",
				"WEATHER_CHANGE":      "气象重构：深度解构当前气象场。评估湿度、能见度对驾驶视距的物理影响，分析云系演变对光影景观的潜在贡献。",
				"TIME_EVENT":          "光影相位：步入关键时间节点。分析色温变化对视觉疲劳的影响，提示在地特质体验，适配当前光照环境。",
				"REST_WELCOME_BACK":   "序列重启：数据链路重连。精准回溯休整前状态，实时同步停机期间的环境变量漂移，校准后续航程预期。",
				"DROWSY_DRIVING":      "生理冗余监测：分析血氧饱和度下降与驾驶时长间的关联。基于地理坐标检索最近的生境修复点（如观景台），提示物理休整，严禁说教。",
			}
			userInputParams.ContextHint = append(userInputParams.ContextHint,
				"SCENARIO: "+data.TriggerReason,
				TriggerInstructions[data.TriggerReason],
			)

		}

		switch data.TriggerReason {
		case "":
			log.Info("未开启驾驶状态、视为未在时普通调用")

			// 			spMap.TriggerReason = `
			// 用户处于非驾驶状态（静态或休息），适合深度交流、行程复盘或未来规划。`

			userInputParams.ContextHint = append(userInputParams.ContextHint,
				"用户处于非驾驶状态（静态或休息），适合深度交流、行程复盘或未来规划。`")

			userMessage = data.Message
			// spMap.Input = ""

			spMap.Constraints = `
1. **多维分析**：必须结合 历史统计数据 分析用户的驾驶习惯、地形挑战（爬升/海拔）或地理足迹。
2. **专业语气**：像拉力赛领航员，从容、专业。
3. **决策辅助**：利用 RAG 知识库，不仅给信息，更要给建议。`

			// 			spMap.SkillsBehaviors = `
			// - **深度复盘**：利用历史 RAG 数据和 Agent 工具链，分析本次行程的成就（如：累计爬升高度、平均时速的意义）。
			// - **人文百科**：提供详细的地理、历史背景知识，作为行程的延伸阅读。
			// - **规划专家**：根据当前位置和统计数据，推荐下一个适合休息点或值得绕路的美景。`

			filterToolsFunc = append(filterToolsFunc,
				dbxV1.GET_TRIP_STATISTICS,
				dbxV1.SEARCH_PLACE_INFO,
				dbxV1.GET_FULL_WEATHER,
				dbxV1.GET_APP_MANIFEST,
				dbxV1.SEARCH_NEARBY_POIS,
				dbxV1.QUERY_TRIP_MEMORY,
			)

		case "DROWSY_DRIVING":
			log.Info("驾驶状态、疲劳驾驶触发")

			//	spMap.TriggerReason = `
			//
			// 监测到驾驶员存在疲劳驾驶风险、已连续驾驶2小时，需立即介入进行安全干预。`

			spMap.Constraints = `
1. **生命至上**：强制执行极简回复（<40字），严禁任何地理人文废话。
2. **强介入感**：语气必须坚定、果断，以唤醒用户注意力为首要目标。
3. **安全导向**：display.warning 必须包含具体的安全指令（如：寻找服务区、开窗、降低温度）。
4. **拒绝闲聊**：所有回复必须指向“安全驾驶”这一核心目标。`

			// 			spMap.SkillsBehaviors = `
			// - **安全博弈**：结合当前天气或时速，指出疲劳驾驶在当前环境下的具体风险。
			// - **心理/生理干预**：通过简短有力的互动、提醒用户心跳/呼吸频率或强制建议最近的服务区导航。
			// - **环境监测**：根据 tripStatistics 中的平均时速变化，判断用户是否反应变慢并给出反馈。`

		case "CHANGE_CITY":
			log.Info("驾驶状态、更换城市触发")

			// spMap.TriggerReason = `
			// 驶入了新的城市，需要提供该城市的美食推荐、景区推荐、文化科普、及驾驶天气变化提醒。`

			spMap.Constraints = `
1. **地域边界感**：必须点出“离开[旧城]，进入[新城]”的瞬间感。
2. **实时快读**：行车中回复严格限制在 <60字，快速交代新城市的关键特征。
3. **差异对比**：对比新老城市的气温、海拔或人文差异，体现地理领航员的洞察力。`

			// 			spMap.SkillsBehaviors = `
			// - **地理名片**：用一句话概括新城市的“灵魂”标签（如：来到“日光城”拉萨）。
			// - **环境预警**：若海拔、温度骤变（如跨过关口进入高原），必须在 warning 中同步提醒预防高反或添加衣物。
			// - **人文连接**：利用 RAG 提示该城市独有的驾驶注意事项或地标。`

		default:
			log.Info("驾驶状态、其他类型的AI领航员")
		}

	case "INTENT_STATS":
		userInputParams.ContextHint = append(userInputParams.ContextHint,
			"用户正在查询历史行程统计数据。",
			"查询整体统计。调用 get_trip_statistics",
			"查询具体行程。调用 query_trip_memory")

		filterToolsFunc = append(filterToolsFunc,
			dbxV1.QUERY_TRIP_MEMORY,
			dbxV1.GET_TRIP_STATISTICS)

	case "INTENT_WEATHER":
		userInputParams.ContextHint = append(userInputParams.ContextHint,
			"若用户查询天气，请主动使用 get_full_weather 获取其背景")

		filterToolsFunc = append(filterToolsFunc, dbxV1.GET_FULL_WEATHER)

	case "INTENT_GEOGRAPHY":
		userInputParams.ContextHint = append(userInputParams.ContextHint,
			"用户对周边地理感兴趣。请分析[currentTripData.city]中的地点，结合当前海拔和位置提供百科式讲解。若有POIs，请重点介绍其特色。",
			"若用户询问的城市 / 地点 不是当前周边位置，请主动使用 search_place_info 获取其背景")

		filterToolsFunc = append(filterToolsFunc,
			dbxV1.SEARCH_NEARBY_POIS,
			dbxV1.QUERY_TRIP_MEMORY,
			dbxV1.SEARCH_PLACE_INFO)

	case "SYSTEM_SILENCE":
		userInputParams.ContextHint = append(userInputParams.ContextHint,
			"立即停止当前播报，进入静默模式。")
	// case "EMERGENCY":

	case "INTENT_NAVIGATION":
		userInputParams.ContextHint = append(userInputParams.ContextHint,
			"识别到功能操作或咨询，须从 AppFeatures 选取对应 ID 填入 'actionId' 字段",
			"当前用户涉及站内功能咨询。请优先根据 AppFeatures 引导用户，保持幽默风趣可爱。",
		)
		// 在此时注入站内文档
		spMap.AppManifest = appDbx.GetAppManifestStr()

	// case "EMERGENCY":

	default:
		userInputParams.ContextHint = append(userInputParams.ContextHint,
			"当前属于随性闲聊场景。请以资深领航员的身份随和应对，保持专业且有趣的驾驶伴侣人格。")

		filterToolsFunc = append(filterToolsFunc,
			dbxV1.GET_TRIP_STATISTICS,
			dbxV1.SEARCH_PLACE_INFO,
			dbxV1.GET_FULL_WEATHER,
			dbxV1.GET_APP_MANIFEST,
			dbxV1.SEARCH_NEARBY_POIS,
			dbxV1.QUERY_TRIP_MEMORY,
		)

	}

	// ` + ncommon.IfElse(spMap.Input != "", `
	// # Input Context Protocol (输入数据协议)
	// 你将实时接收以下JSON格式的数据流：
	// `+spMap.Input+`
	// `, "") + `

	// # Trigger Reason
	// ` + spMap.TriggerReason + `

	// # Skills & Behaviors (技能与行为)
	// ` + spMap.SkillsBehaviors + `

	systemPrompt := `
` + spMap.Role + `

# Constraints (核心约束)
` + spMap.Constraints + `
` + spMap.InteractionGuideline + `

# Safety Redline 
警告：严禁讨论政治、种族、领土、战争及敏感社会话题。
若用户提及，请以领航员身份礼貌拒答，并将 status.isSafetyFenced 设为 true

# Thought Process (Internal)
` + spMap.ThoughtProcess + `

# Output Protocol (Strict JSON Mode)
` + spMap.OutputProtocol + `
`

	if spMap.AppManifest != "" {
		systemPrompt += `
# App Manifest
` + spMap.AppManifest + `
`

	}

	contexts, err := aiDbx.GetAIMessages(data.SessionId)
	log.Info("contexts, err ",
		contexts, err)

	messages := append([]openai.ChatCompletionMessage{
		{Role: openai.ChatMessageRoleSystem, Content: systemPrompt},
	}, fc.ContextsToAIChatMessages(narrays.Filter(contexts, func(v *protos.ChatContextItem, i int) bool {
		return v.Id != data.MessageId
	}))...)

	jsonData, err := json.Marshal(userInputParams)
	if err != nil {
		log.Error(err)
		res.Errors(fmt.Errorf("json decode error: %v", err))
		res.Code = 10001
		res.Call(c)
		return
	}

	messages = append(messages, openai.ChatCompletionMessage{
		Role: openai.ChatMessageRoleUser, Content: string(jsonData),
	})

	contextData = append(contextData, userInputParams.ContextData...)

	if userMessage != "" {
		messages = append(messages, openai.ChatCompletionMessage{
			Role: openai.ChatMessageRoleUser, Content: userMessage,
		})
	}

	contextDataStr, _ := json.Marshal(userInputParams.ContextData)
	tokenUsage.ContextDataTokens = int32(methods.GetTokenCount(string(contextDataStr)))

	// if userPrompt {
	// }

	tools := aiDbx.GetAgentTripTools(filterToolsFunc)

	// res.Errors(err)
	// res.Code = 10004
	// res.Call(c)
	// return

	log.Info(messages)
	// log.Info("systemPrompt", systemPrompt)
	// log.Info("userPrompt", userPrompt)
	log.Info(tools)
	j, _ := json.MarshalIndent(userInputParams, "", "  ")
	log.Info(string(j))
	// log.Info(data.CurrentTripData != nil)
	// log.Info(userInputParams.CurrentTripData)

	log.Info("用户输入:", data.Message, "routingStrategy", routingStrategy.BestMatch)

	// res.Errors(err)
	// res.Code = 10004
	// res.Call(c)
	// return

	// 1. 设置响应头，这是 SSE 的核心要求
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("Transfer-Encoding", "chunked")
	c.Stream(func(w io.Writer) bool {

		// 获取底层的 http.Flusher
		flusher, ok := w.(http.Flusher)
		// log.Info(ok)
		if !ok {
			return false
		}

		c.SSEvent("message", protos.Encode(&protos.AIStreamResponse{
			Type:  "Session",
			Value: data.SessionId,
		}))
		flusher.Flush()

		ctx, cancel := context.WithTimeout(c.Request.Context(), 120*time.Second)
		defer cancel()

		// 设置最大循环次数，防止死循环
		curModel := ""
		var metaArr []*protos.AIResponse_Meta

		var shouldBreak bool

		maxIterations := conf.MaxIterations
		// maxIterations = 1

		var calcToken = func(loopCount int, retryCount int, usage *openai.Usage) {

			if usage != nil &&
				usage.PromptTokensDetails != nil &&
				usage.CompletionTokensDetails != nil {
				tuItem := &protos.AITokenUsage_TokenUsageItem{
					LoopCount:                 int32(loopCount + 1),
					RetryCount:                int32(retryCount + 1),
					PromptTokens:              int32(usage.PromptTokens),
					CompletionTokens:          int32(usage.CompletionTokens),
					TotalTokens:               int32(usage.TotalTokens),
					PromptCachedTokens:        int32(usage.PromptTokensDetails.CachedTokens),
					CompletionReasoningTokens: int32(usage.CompletionTokensDetails.ReasoningTokens),
				}

				tokenUsage.TokenUsageHistory = append(tokenUsage.TokenUsageHistory, tuItem)
				tokenUsage.TotalSessionTokens += tuItem.TotalTokens

				// log.Warn("tuItem", tuItem)
			}

			// log.Error("calcToken", loopCount, retryCount, usage.TotalTokens, tokenUsage)

			// log.Error("calcToken", loopCount, retryCount, tokenUsage.TotalSessionTokens)

			c.SSEvent("message", protos.Encode(&protos.AIStreamResponse{
				Type:  "tokenUsage",
				Value: protos.Encode(tokenUsage),
			}))
			flusher.Flush()

		}

		for i := range maxIterations {
			log.Info(" --- 第"+nstrings.ToString(i+1)+"步：发起请求让 AI 决定是否调用工具 ---", curModel)

			c.SSEvent("message", protos.Encode(&protos.AIStreamResponse{
				Type:  "nextLoop",
				Value: nstrings.ToString(i + 1),
			}))
			flusher.Flush()

			select {
			case <-c.Request.Context().Done():
				log.Warn("客户端连接已关闭，停止重试")
				return false
			default:
				// 继续执行

				shouldBreak, err = func() (bool, error) {

					var fullContent strings.Builder
					var fullReasoning strings.Builder
					var toolCalls []openai.ToolCall

					maxRetries := 2 // 内部网络抖动重试次数

					endRetry := false

					for retry := range maxRetries {

						select {
						case <-c.Request.Context().Done():
							log.Warn("客户端连接已关闭，停止重试")
							return true, nil // 彻底打破外部循环
						default:
							// 继续执行

							// --- 核心改进：如果是重试，构造续写 Messages ---
							currentMessages := messages
							if fullContent.Len() > 0 {
								// 构造续写上下文，让 AI 接着写 JSON
								currentMessages = append(currentMessages, openai.ChatCompletionMessage{
									Role:    openai.ChatMessageRoleAssistant,
									Content: fullContent.String(),
								})
								log.Warn(fmt.Sprintf("--- 检测到流中断，正在进行第 %d 次续写重试 ---", retry+1, fullContent.Len()))
							}

							stream, err := conf.OpenAIClient.CreateChatCompletionStream(ctx, openai.ChatCompletionRequest{
								Model: conf.OpenAIModel,
								// Model: "sa-groq-llama-3.3-70b",
								// Model:       "groq-llama-4-scout",
								Messages:    currentMessages,
								Tools:       tools,
								Stream:      true,
								MaxTokens:   4096, // 确保长路书不被腰斩
								Temperature: 0.5,  // 兼顾准确性与文采
								TopP:        0.8,  // 配合 Temperature 进一步稳定输出
								StreamOptions: &openai.StreamOptions{
									IncludeUsage: true, // 必须设为 true，否则 Usage 永远是 nil
								},
								ResponseFormat: &openai.ChatCompletionResponseFormat{
									Type: openai.ChatCompletionResponseFormatTypeJSONObject,
								},
							})

							if err != nil {
								c.SSEvent("error", protos.Encode(&protos.AIStreamResponse{
									Type:    "error",
									Content: "流式生成失败: " + err.Error(),
								}))
								flusher.Flush()
								return true, err
							}

							log.Info("开启流式返回给前端 ---", len(currentMessages), conf.OpenAIModel, fullContent.Len())

							// 正常开始读取
							var lastUsage *openai.Usage
							err = func() error {
								defer stream.Close()
								for {
									select {
									// --- 核心改动：每一帧数据回来前，先看用户还在不在 ---
									case <-c.Request.Context().Done():
										// 用户主动断开（切歌了、关屏幕了、信号完全断了导致连接重置）
										log.Warn("检测到客户端连接已关闭，立即终止 AI 生成流")
										return nil

									default:
										// 正常的非阻塞读取逻辑
										resp, streamErr := stream.Recv()

										if resp.Usage != nil {
											lastUsage = resp.Usage
										}

										if streamErr == io.EOF {
											calcToken(i, retry, lastUsage)
											return nil // 正常吐完所有 Token
										}

										if streamErr != nil {
											calcToken(i, retry, lastUsage)
											// 网络抖动、API 报错等，触发外层的重试逻辑
											return streamErr
										}

										if resp.Model != curModel {
											curModel = strings.Replace(resp.Model, "meta-llama/", "", 1)
											c.SSEvent("message", protos.Encode(&protos.AIStreamResponse{
												Type:  "model",
												Value: curModel,
											}))
											flusher.Flush()
										}

										// 1. 获取思考碎片 (Reasoning Content)
										reasoning := resp.Choices[0].Delta.ReasoningContent
										if reasoning != "" {
											c.SSEvent("message", protos.Encode(&protos.AIStreamResponse{
												Type:    "reasoning",
												Content: reasoning,
											}))

											fullReasoning.WriteString(reasoning)
											flusher.Flush()
										}

										// 2. 获取正式内容碎片 (Content)

										content := resp.Choices[0].Delta.Content
										// log.Info("Content: ", content)
										fullContent.WriteString(content)
										if content != "" {
											c.SSEvent("message", protos.Encode(&protos.AIStreamResponse{
												Type:    "message",
												Content: content,
											}))
											flusher.Flush()
										}

										deltaToolCalls := resp.Choices[0].Delta.ToolCalls

										// 2. 累积工具调用 (ToolCalls)
										if len(deltaToolCalls) > 0 {
											for _, tc := range deltaToolCalls {
												idx := *tc.Index

												// --- 动态扩容逻辑 ---
												// 如果当前的索引超出了切片长度，通过 append 扩容
												for len(toolCalls) <= idx {
													toolCalls = append(toolCalls, openai.ToolCall{
														Index:    new(int), // 初始化指针
														Function: openai.FunctionCall{},
													})
													*toolCalls[len(toolCalls)-1].Index = len(toolCalls) - 1
												}

												// --- 填充数据 ---
												target := &toolCalls[idx]

												if tc.ID != "" {
													target.ID = tc.ID
												}
												if tc.Type != "" {
													target.Type = tc.Type
												}
												if tc.Function.Name != "" {
													target.Function.Name += tc.Function.Name
												}
												if tc.Function.Arguments != "" {
													target.Function.Arguments += tc.Function.Arguments
												}
											}
										}
									}
								}
							}()

							if err == nil {
								// 如果内部读取完整结束（io.EOF），则打破重试循环
								endRetry = true
								break
							}

							log.Error("流读取中途崩溃，即将重试回填: ", err)
							// 如果到这里说明 err != nil，循环会继续，进入下一次 retry

						}

						if endRetry {
							break
						}
					}

					log.Info("A. 判定是否命中了工具", len(toolCalls) > 0, len(toolCalls))
					if len(toolCalls) > 0 {
						msg := openai.ChatCompletionMessage{
							Role:             openai.ChatMessageRoleAssistant,
							ReasoningContent: fullReasoning.String(),
							Content:          fullContent.String(),
							ToolCalls:        toolCalls,
						}
						messages = append(messages, msg)

						for _, toolCall := range msg.ToolCalls {

							select {
							case <-c.Request.Context().Done():
								res.Code = 10001
								res.Call(c)
								return true, nil

							default:

								meta := aiDbx.CallAgentTools(c, flusher, toolCall, userInfo.Uid)

								metaArr = append(metaArr, meta)

								messages = append(messages, openai.ChatCompletionMessage{
									Role: "tool", ToolCallID: toolCall.ID,
									Content: `{"status":"` + meta.Status +
										`","value":"` + meta.Value +
										`","error":"` + meta.Error + `"}`,
								})

							}
						}

						return false, nil
					}

					log.Info("--- 7. 清理并解析 JSON (关键步骤) ---")
					finalJSON := strings.Trim(fullContent.String(), "` \n")
					finalReasoning := strings.Trim(fullReasoning.String(), "` \n")
					finalJSON = strings.TrimPrefix(finalJSON, "json")
					finalJSON = methods.FastFixJSON(finalJSON)

					log.Info("finalReasoning", finalReasoning)
					log.Info("finalJSON", finalJSON)

					var res protos.AIResponse

					isTools, toolCall := aiDbx.ParseTextToToolCall(finalJSON)
					log.Info("finalJSON", isTools, toolCall)

					if isTools {

						meta := aiDbx.CallAgentTools(c, flusher, toolCall, userInfo.Uid)
						metaArr = append(metaArr, meta)
						messages = append(messages, openai.ChatCompletionMessage{
							Role: "tool", ToolCallID: toolCall.ID,
							Content: `{"status":"` + meta.Status + `","value":"` + meta.Value + `"}`,
						})
						res.Code = 200
						res.Display = &protos.AIResponse_Display{
							Message: "路书信息已同步更新。",
							Warning: "",
						}
					} else {
						if err := json.Unmarshal([]byte(finalJSON), &res); err != nil {
							// 如果解析失败，可能是 AI 输出带了前缀，这里可以做更强的正则清洗
							log.Error("\n解析 JSON 失败: %v", err)

							c.SSEvent("error", protos.Encode(&protos.AIStreamResponse{
								Type:    "error",
								Content: "解析 JSON 失败: " + err.Error(),
							}))
							flusher.Flush()
							return false, err
						}
					}

					res.SessionId = data.SessionId

					res.TokenUsage = tokenUsage
					res.Model = curModel
					res.Reasoning = &protos.AIResponse_Reasoning{
						Message: finalReasoning,
					}
					res.Meta = metaArr

					res.Action = dbxV1.ManifestHydrator[res.ActionId]

					aiDbx.SaveAIMessages(data.SessionId,
						data.MessageId,
						data.Message, res.Display.Message+"\n"+res.Display.Warning, 10)

					// log.Info(msg.ToolCalls)
					log.Info("成功解析！: ", res)

					c.SSEvent("message", protos.Encode(&protos.AIStreamResponse{
						Type:    "final",
						Content: protos.Encode(&res),
					}))
					flusher.Flush()

					c.SSEvent("done", "EOF")
					flusher.Flush()

					return true, nil // 返回 true 表示：任务完成，打破外部循环
				}()

				// 处理匿名函数返回的结果
				if err != nil {
					log.Error("交互报错: ", err)
					// 发送 SSE 错误消息...
					return false
				}
			}

			if shouldBreak {
				break
			}
		}
		return false
	})

	protoData := &protos.AICoDriver_Response{}

	res.Data = protos.Encode(protoData)

	res.Call(c)
}

func (fc *AIController) AiRoadbook(c *gin.Context) {

	// 1、请求体
	var res response.ResponseProtobufType
	res.Code = 200

	// 2、获取参数
	data := new(protos.AIRoadbook_Request)
	var err error
	if err = protos.DecodeBase64(c.GetString("data"), data); err != nil {
		res.Error = err.Error()
		res.Code = 10002
		res.Call(c)
		return
	}

	log.Info("data", data)

	// 3、验证参数
	if err = validation.ValidateStruct(
		data,
		validation.Parameter(&data.Id, validation.Type("string")),
		validation.Parameter(&data.Messages, validation.Length(1, 20), validation.Required()),
	); err != nil {
		res.Errors(err)
		res.Code = 10002
		res.Call(c)
		return
	}

	userInfoAny, exists := c.Get("userInfo")
	if !exists {
		res.Errors(err)
		res.Code = 10004
		res.Call(c)
		return
	}
	userInfo := userInfoAny.(*sso.UserInfo)
	log.Info("userInfo", userInfo)

	isDev := false
	if isDev {
		fc.testSSE(c, &res)
		return
	}

	// # Capabilities
	// - 如果用户提出修改需求，你必须先调用工具。
	// - 在工具执行完成后（即当前阶段），你需要总结操作结果并提供博主风格的文案。
	// - 如果用户没有提出修改等工具需求，则在 display_text 中回复用户感兴趣的内容。

	// 1. 设置响应头，这是 SSE 的核心要求
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("Transfer-Encoding", "chunked")
	c.Stream(func(w io.Writer) bool {

		// 获取底层的 http.Flusher
		flusher, ok := w.(http.Flusher)
		log.Info(ok)
		if !ok {
			return false
		}

		systemPrompt := `
# Role
你是资深的自驾路书 Agent。
` + ncommon.IfElse(data.Id != "", `当前操作的路书ID为`+data.Id+``, "") + `

# Decision Logic (Internal Filter)
1. 意图判定：路书/行程/景点/自驾技巧相关则深度分析；无关闲聊则极简回复(<50字)。
2. 思维代入：以资深领队视角进行心理复盘（如：分析重庆时，重点考量其对驾驶技术的挑战及补给价值）。

# Thought Process Protocol (按需思考模式)
思考内容 不是你的工作日志，严禁记录你的分析步骤。如果回复简单，请让 思考内容 保持为空
1. **逻辑空白期**：
   - 如果用户的问题 1 秒钟内就能得出结论（如：介绍城市、问好），思考内容 必须**完全保持空白**。
   - **禁止**出现任何数字列表（1., 2., 3.），**禁止**出现“分析、意图、起草”等词汇。
2. **强制负面约束**：
   - 只要你在思考里写了“第一步”、“起草内容”或“JSON格式”，我就认为你程序死机了。
   - 只有在面临“走国道还是走高速”这种**生死抉择**时，你才准写一句话。
3. **思考内容格式**：
   - 即使思考，也只允许写业务干货。
   - ❌ 错误：分析用户输入，意图是介绍重庆...
   - ✅ 正确：重庆 8D 导航易漂移，建议置入预警。

# Output Protocol (Strict JSON Mode)
1. 你必须输出且仅输出一个 JSON 对象，结构如下：
{
  "code": number, // 成功:200, 失败:10001
  "status": { 
		"isRelevant": boolean // 核心业务为true，无关内容为false
	},
  "display": {
    "message": string, // markdown内容，相关：自家博主风口语化总结；无关：一句话终结对话
    "warning": string  // markdown内容，风险预警（如高反、长途疲劳、路况风险），若无则空字符串
  }
}
2.JSON 稳定性约束：
	 - 所有的 Markdown 内容，换行必须在 JSON 字符串中转义为 \n，严禁产生真实的物理换行。
	 - 字符串内部的双引号 " 必须转义为 \"，以防破坏 JSON 结构。
	 - 严禁使用原生 HTML 标签，仅允许标准的 Markdown 符号。

# Workflow
1. 先查后改：涉及修改指令时，若信息不足必先 get，获取后立即 update，严禁反问。
2. 如果一次决策涉及多个修改动作（如：同时修改标题、描述、标签），必须在同一次响应中输出所有相关的工具调用指令。
3. 严禁将本可以合并的修改任务拆分为多次循环执行，以优化 Token 消耗和响应延迟。
4. 如果用户的指令包含重复性描述（如“改 10 次”），你只需确保最终结果符合用户预期，严禁发起多次完全相同的工具调用。
5. 在发起修改指令前，请对比当前状态。如果目标状态已达成，直接回复用户即可，无需调用工具。

# Constraints
- 严禁输出 JSON 外的任何文字。
- 严禁在 JSON 结构外添加任何 Markdown 标签（如 json ）。
`

		messages := []openai.ChatCompletionMessage{
			{Role: openai.ChatMessageRoleSystem, Content: systemPrompt},
			{Role: openai.ChatMessageRoleUser, Content: data.Messages[len(data.Messages)-1]},
		}

		tools := aiDbx.GetAgentTools()

		log.Info(tools)
		ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
		defer cancel()

		curModel := conf.OpenAIModel
		var metaArr []*protos.AIResponse_Meta

		for i := range conf.MaxIterations {

			log.Info(" --- 第一步：发起请求让 AI 决定是否调用工具 ---", "第"+nstrings.ToString(i+1)+"次循环", curModel)
			resp, err := conf.OpenAIClient.CreateChatCompletion(ctx, openai.ChatCompletionRequest{
				Model:       conf.OpenAIModel,
				Messages:    messages,
				Tools:       tools,
				MaxTokens:   4096, // 确保长路书不被腰斩
				Temperature: 0.7,  // 兼顾准确性与文采
				TopP:        0.8,  // 配合 Temperature 进一步稳定输出
			})
			if err != nil {
				log.Error(err)
				c.SSEvent("error", protos.Encode(&protos.AIStreamResponse{
					Type:    "error",
					Content: "流式生成失败: " + err.Error(),
				}))
				flusher.Flush()
				return false
			}

			log.Info("开启流式返回给前端 ---")
			msg := resp.Choices[0].Message
			log.Info("msg.ToolCalls", msg.ToolCalls)

			log.Info("--- 第二阶段：统一流式输出口 ---")

			curModel = strings.Replace(resp.Model, "meta-llama/", "", 1)
			c.SSEvent("message", protos.Encode(&protos.AIStreamResponse{
				Type:  "model",
				Value: curModel,
			}))
			flusher.Flush()

			// c.SSEvent("error", protos.Encode(&protos.AIStreamResponse{
			// 	Type:    "error",
			// 	Content: "流式生成失败: ",
			// }))
			// flusher.Flush()
			// return false

			log.Info("A. 判定是否命中了工具")
			if len(msg.ToolCalls) > 0 {
				messages = append(messages, msg)

				for _, toolCall := range msg.ToolCalls {

					select {
					case <-c.Request.Context().Done():
						res.Code = 10001
						res.Call(c)
						return false

					default:

						meta := aiDbx.CallAgentTools(c, flusher, toolCall, userInfo.Uid)

						metaArr = append(metaArr, meta)

						messages = append(messages, openai.ChatCompletionMessage{
							Role: "tool", ToolCallID: toolCall.ID,
							Content: `{"status":"` + meta.Status +
								`","value":"` + meta.Value +
								`","error":"` + meta.Error + `"}`,
						})

					}
				}
			} else {
				// 如果没命工具，直接把 AI 刚才生成的普通回复（如有）作为上下文，或者直接让它说话
				// 简单处理：直接让流式 API 接着对话上下文继续即可
				break
			}

		}

		log.Info("B. 最终流式生成：不管是“操作成功的感言”还是“普通对话”，都从这里流出")
		stream, err := conf.OpenAIClient.CreateChatCompletionStream(ctx, openai.ChatCompletionRequest{
			Model:       conf.OpenAIModel,
			Messages:    messages,
			Stream:      true,
			MaxTokens:   2048, // 确保长路书不被腰斩
			Temperature: 0.7,  // 兼顾准确性与文采
			TopP:        0.9,  // 配合 Temperature 进一步稳定输出
			// ResponseFormat: &openai.ChatCompletionResponseFormat{
			// 	Type: openai.ChatCompletionResponseFormatTypeJSONObject,
			// },
		})
		if err != nil {
			log.Error(err)
			c.SSEvent("error", protos.Encode(&protos.AIStreamResponse{
				Type:    "error",
				Content: "流式生成失败: " + err.Error(),
			}))
			flusher.Flush()
			return false
		}
		defer stream.Close()

		var fullContent strings.Builder
		var fullReasoning strings.Builder
		log.Info("\n>>> AI 思考完成，正在流式输出：")

	OuterLoop:
		for {
			select {
			case <-c.Request.Context().Done():
				res.Code = 10001
				res.Call(c)
				return false

			default:

				resp, err := stream.Recv()
				if err == io.EOF {
					// 【核心：发送结束信号】
					break OuterLoop
				}
				if err != nil {
					log.Error(err)
					c.SSEvent("error", protos.Encode(&protos.AIStreamResponse{
						Type:    "error",
						Content: "读取流中断: " + err.Error(),
					}))
					flusher.Flush()
					break OuterLoop
				}
				// 1. 获取思考碎片 (Reasoning Content)
				reasoning := resp.Choices[0].Delta.ReasoningContent
				if reasoning != "" {
					// log.Info("Thinking: ", reasoning)
					// 发送给前端，Type 标记为 "reasoning"

					c.SSEvent("message", protos.Encode(&protos.AIStreamResponse{
						Type:    "reasoning",
						Content: reasoning,
					}))

					fullReasoning.WriteString(reasoning)
					flusher.Flush()
					continue // 思考碎片和正式内容通常不会在同一个 resp 中，直接进入下一次循环
				}

				// 2. 获取正式内容碎片 (Content)

				content := resp.Choices[0].Delta.Content
				// log.Info("Content: ", content)
				fullContent.WriteString(content)
				if content != "" {

					c.SSEvent("message", protos.Encode(&protos.AIStreamResponse{
						Type:    "message",
						Content: content,
					}))
					flusher.Flush()
				}
				w.(http.Flusher).Flush() // 强行刷新
			}
		}

		log.Info("--- 7. 清理并解析 JSON (关键步骤) ---")
		finalJSON := strings.Trim(fullContent.String(), "` \n")
		finalReasoning := strings.Trim(fullReasoning.String(), "` \n")
		finalJSON = strings.TrimPrefix(finalJSON, "json")
		finalJSON = methods.FastFixJSON(finalJSON)

		log.Info("finalReasoning", finalReasoning)
		log.Info("finalJSON", finalJSON)

		var res protos.AIResponse

		isTools, toolCall := aiDbx.ParseTextToToolCall(finalJSON)
		log.Info("finalJSON", isTools, toolCall)

		if isTools {

			meta := aiDbx.CallAgentTools(c, flusher, toolCall, userInfo.Uid)
			metaArr = append(metaArr, meta)
			messages = append(messages, openai.ChatCompletionMessage{
				Role: "tool", ToolCallID: toolCall.ID,
				Content: `{"status":"` + meta.Status + `","value":"` + meta.Value + `"}`,
			})
			res.Code = 200
			res.Display = &protos.AIResponse_Display{
				Message: "路书信息已同步更新。",
				Warning: "",
			}
		} else {
			if err := json.Unmarshal([]byte(finalJSON), &res); err != nil {
				// 如果解析失败，可能是 AI 输出带了前缀，这里可以做更强的正则清洗
				log.Error("\n解析 JSON 失败: %v", err)

				c.SSEvent("error", protos.Encode(&protos.AIStreamResponse{
					Type:    "error",
					Content: "解析 JSON 失败: " + err.Error(),
				}))
				flusher.Flush()
				return false
			}
		}

		res.Model = curModel
		res.Reasoning = &protos.AIResponse_Reasoning{
			Message: finalReasoning,
		}
		res.Meta = metaArr

		// log.Info(msg.ToolCalls)
		log.Info("成功解析！: ", res)

		c.SSEvent("message", protos.Encode(&protos.AIStreamResponse{
			Type:    "final",
			Content: protos.Encode(&res),
		}))
		flusher.Flush()

		c.SSEvent("done", "EOF")
		flusher.Flush()

		return false
	})

	protoData := &protos.AIRoadbook_Response{}

	res.Data = protos.Encode(protoData)

	res.Call(c)
}

// AI 要支持历史对话上下文，2条内全量，2条仅保留问题，5条外不加
func (fc *AIController) ContextsToAIChatMessages(contexts []*protos.ChatContextItem) (msg []openai.ChatCompletionMessage) {
	count := len(contexts)
	if count == 0 {
		return msg
	}

	// 1. 确定处理范围：最多取最近 5 条
	start := 0
	if count > 5 {
		start = count - 5
	}

	// 截取最近的 5 条历史
	recentContexts := contexts[start:]
	newCount := len(recentContexts)

	for i, v := range recentContexts {
		// 这里的 i 是从 0 到 4 的索引（0 是 5 条里最老的一条）
		// 我们需要判断 i 相对于末尾的位置
		distanceFromLatest := newCount - 1 - i

		// 无论如何都要添加 User 问题
		msg = append(msg, openai.ChatCompletionMessage{
			Role:    openai.ChatMessageRoleUser,
			Content: v.Question,
		})

		// 只有距离末尾 2 条以内的（即索引最大的两条）才加 Assistant 回答
		if distanceFromLatest < 2 {
			msg = append(msg, openai.ChatCompletionMessage{
				Role:    openai.ChatMessageRoleAssistant,
				Content: v.Answer,
			})
		}
	}

	return msg
}

func (fc *AIController) testSSE(c *gin.Context, res *response.ResponseProtobufType) {
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("Transfer-Encoding", "chunked")

	c.Stream(func(w io.Writer) bool {
		// 获取底层的 http.Flusher
		flusher, ok := w.(http.Flusher)
		log.Info(ok)
		if !ok {
			return false
		}

		time.Sleep(1000 * time.Millisecond)

		c.SSEvent("message", protos.Encode(&protos.AIStreamResponse{
			Type:  "model",
			Value: conf.OpenAIModel,
		}))
		flusher.Flush()

		// time.Sleep(100 * time.Minute)

		metaArr := []*protos.AIResponse_Meta{}

		time.Sleep(500 * time.Millisecond)
		c.SSEvent("message", protos.Encode(&protos.AIStreamResponse{
			Type:   "meta",
			Action: "get_full_weather",
			Value: protos.Encode(&protos.AIResponse_Meta{
				Action:     "get_full_weather",
				Type:       "function",
				Status:     "calling",
				CreateTime: time.Now().Unix(),
			}),
			Content: "calling",
		}))
		flusher.Flush()
		time.Sleep(500 * time.Millisecond)

		c.SSEvent("message", protos.Encode(&protos.AIStreamResponse{
			Type:   "meta",
			Action: "get_full_weather",
			Value: protos.Encode(&protos.AIResponse_Meta{
				Action:     "get_full_weather",
				Type:       "function",
				Status:     "failed",
				CreateTime: time.Now().Unix(),
			}),
			Content: "failed",
		}))
		flusher.Flush()
		time.Sleep(500 * time.Millisecond)

		c.SSEvent("message", protos.Encode(&protos.AIStreamResponse{
			Type:   "meta",
			Action: "update_title",
			Value: protos.Encode(&protos.AIResponse_Meta{
				Action:     "update_title",
				Type:       "function",
				Status:     "success",
				CreateTime: time.Now().Unix(),
				EndTime:    time.Now().Unix(),
			}),
			Content: "success",
		}))
		flusher.Flush()
		time.Sleep(500 * time.Millisecond)

		metaArr = append(metaArr, &protos.AIResponse_Meta{
			Action:  "update_title",
			Type:    "function",
			Status:  "success",
			EndTime: time.Now().Unix(),
		})

		flusher.Flush()

		// select {
		// // 【核心添加：检查前端是否已断开】
		// case <-c.Request.Context().Done():
		// 	log.Warn("检测到用户主动停止请求，后端收工。")
		// 	// 这里直接返回 false 退出 c.Stream
		// 	return false

		// default:

		// }
		rawJSON := `让我分析一下这个请求：

1. **相关性判定**：用户请求介绍重庆，这属于地理位置/城市介绍的内容。虽然不完全直接与自驾路书相关，但重庆作为一个热门自驾目的地，与自驾旅游有一定关联性。我可以将其视为"边缘相关"，因为介绍重庆可以成为自驾路书的前置信息。

我将按照要求，以JSON格式回复，并提供Markdown格式的城市介绍内容，特别关注与自驾相关的信息。`

		speed := 10 * time.Millisecond

		// 模拟流式输出：遍历字符串并逐字符打印
		for i, char := range rawJSON {
			select {
			// 【核心添加：检查前端是否已断开】
			case <-c.Request.Context().Done():
				log.Warn("检测到用户主动停止请求，后端收工。")
				res.Code = 10001
				res.Call(c)
				// 这里直接返回 false 退出 c.Stream
				return false

			default:

				// if i > 100 {
				// 	c.SSEvent("error", protos.Encode(&protos.AIStreamResponse{
				// 		Type:    "error",
				// 		Content: "流式生成失败: ",
				// 	}))
				// 	flusher.Flush()

				// 	return false
				// }

				log.Info("char", i, string(char))

				c.SSEvent("message", protos.Encode(&protos.AIStreamResponse{
					Type:    "reasoning",
					Content: string(char),
				}))
				flusher.Flush()

				// 模拟延迟：可以根据需要调整速度
				// 20ms 是比较自然的流式体感
				time.Sleep(speed)
			}
			// 如果是在 Web Server 中，这里需要调用 flusher.Flush()
		}

		rawJSON2 := `{
    "code": 200,
    "status": {
        "isRelevant": true
    },
    "display": {
        "message": "### 重庆自驾指南\n\n**城市特色**：独特的桥都文化，众多跨江大桥",
        "warning": "重庆地形复杂，自驾新手需谨慎。"
    },
}`

		// 		rawJSON2 = `{
		//     "code": 200,
		//     "status": {
		//         "isRelevant": true,
		//         "location": "重庆",
		//         "category": "Self-driving Guide"
		//     },
		//     "display": {
		//         "message": "### 🚗 重庆立体纵横：8D 山城自驾实操指南\n\n**城市地标与视觉核心**：\n- **两江交汇**：长江与嘉陵江在朝天门交汇，清浊分明，是自驾穿梭两岸的核心景观。\n- **桥梁之都**：拥有超过 4500 座大桥，自驾在“菜园坝大桥”或“鹅公岩大桥”可体验云端穿行的错觉。\n- **魔幻高程**：你会发现自己在 1 楼开车，窗外可能是别人的屋顶，或是在 20 层楼高的立交桥上掉头。\n\n**🛠️ 自驾技术难点排查**：\n- **导航策略**：必须开启手机导航的**“立体楼层识别”**或**“高架识别”**功能。当提示“请靠右”时，务必注意是指向辅路还是下匝道。\n- **坡道起步**：老城区路段（如鹅岭、两路口）坡度极大且常遇堵车，非自动挡车型或无“坡道辅助”车型请谨慎进入。\n- **限行同步**：重庆对本地及外地车实行**“尾号限行”**。限行区域涵盖主要跨江大桥，违反将面临罚款及记分。\n\n**📍 深度自驾路线推荐**：\n- **【夜景流光线】**：南滨路（起点）→ 东水门大桥 → 洪崖洞外围（避开拥堵点）→ 嘉滨路。建议 19:30 以后行驶。\n- **【森林吸氧线】**：中心城区 → 歌乐山三百梯（急弯测试）→ 中梁山森林公园 → 磁器口后山。\n- **【工业怀旧线】**：九龙坡黄桷坪（涂鸦艺术）→ 李家沱大桥 → 巴南滨江路，人少景美。\n\n**💡 避坑锦囊**：\n- **避开洪崖洞核心区**：节假日周边交通会瘫痪，建议将车停在江北嘴，步行过大桥看洪崖洞全景。\n- **备好停车APP**：推荐下载“重庆停车”或“渝约停”，老城区（如渝中区）车位极度稀缺且路边禁止违停。",
		//         "warning": "⚠️ 特别警告：重庆导航易导致“鬼打墙”（即便使用最新版地图）。若错过出口，严禁倒车，请继续向前行驶，系统通常需要 3-5 公里才能规划出下一个折返点。"
		//     }
		// }`

		// 模拟流式输出：遍历字符串并逐字符打印
		for _, char := range rawJSON2 {

			select {
			// 【核心添加：检查前端是否已断开】
			case <-c.Request.Context().Done():
				log.Warn("检测到用户主动停止请求，后端收工。")
				res.Code = 10001
				res.Call(c)
				// 这里直接返回 false 退出 c.Stream
				return false

			default:
				c.SSEvent("message", protos.Encode(&protos.AIStreamResponse{
					Type:    "message",
					Content: string(char),
				}))
				flusher.Flush()

				// 模拟延迟：可以根据需要调整速度
				// 20ms 是比较自然的流式体感
				time.Sleep(speed)
			}
			// 如果是在 Web Server 中，这里需要调用 flusher.Flush()
		}

		var res protos.AIResponse

		res.Code = 200
		res.Status = &protos.AIResponse_Status{
			IsRelevant:     false,
			IsSafetyFenced: false,
		}
		res.Model = conf.OpenAIModel
		res.Reasoning = &protos.AIResponse_Reasoning{
			Message: rawJSON,
		}
		res.Display = &protos.AIResponse_Display{
			Message: "### 重庆自驾指南\n\n**城市特色**：独特的桥都文化，众多跨江大桥",
			Warning: "重庆地形复杂，自驾新手需谨慎。",
		}
		res.Meta = metaArr

		// log.Info(msg.ToolCalls)
		// log.Info("成功解析！: ", res)

		c.SSEvent("message", protos.Encode(&protos.AIStreamResponse{
			Type:    "final",
			Content: protos.Encode(&res),
		}))
		flusher.Flush()

		c.SSEvent("done", "EOF")
		flusher.Flush()

		return false
	})
	res.Code = 10001
	res.Call(c)
}
