package dbxV1

import (
	"context"
	"errors"
	"fmt"
	"math"
	"math/rand"
	"strings"
	"time"

	conf "github.com/ShiinaAiiko/nyanya-trip-route-track/server/config"
	"github.com/ShiinaAiiko/nyanya-trip-route-track/server/models"
	"github.com/ShiinaAiiko/nyanya-trip-route-track/server/services/methods"
	"github.com/cherrai/nyanyago-utils/narrays"
	"github.com/cherrai/nyanyago-utils/ncommon"
	"github.com/cherrai/nyanyago-utils/nstrings"
	"github.com/paulmach/orb"
	"github.com/paulmach/orb/geo"
	"github.com/paulmach/orb/simplify"
	"github.com/qdrant/go-client/qdrant"
	"github.com/sashabaranov/go-openai"
	"golang.org/x/time/rate"
)

type RgaTripMemoryDbx struct {
}

var (
	tripMemoryDbx = new(RgaTripMemoryDbx)
)

func (d *RgaTripMemoryDbx) TesttTripMemory(tripId string) error {

	log.Info("开始 AI 核心功能全链路测试...")
	ctx := context.Background()

	// --- 测试 1: 纯语义检索 [语义理解测试] ---
	// log.Warn(">>> 测试 1: 纯语义检索 [聚合函数版]")
	// res1, err := d.SearchTripMemory(ctx, SearchOptions{
	// 	Text:      "那段路况比较差、海拔很高的地方在哪？",
	// 	Limit:     3,
	// 	Threshold: 0.62, // 设定你摸索出的分水岭门槛
	// })
	// d.printTestResult("语义检索", res1, err)

	// --- 测试 2: 纯地理空间检索 [位置感知测试] ---
	log.Warn(">>> 测试 2: 空间检索 [聚合函数版]")
	res2, err := d.SearchTripMemory(ctx, SearchTripMemoryOptions{
		Coords: &GeoCoords{
			Lat: 29.73504811,
			Lon: 98.70417381,
		},
		RadiusMeters: 5000,
		Limit:        3,
		// 注意：空间检索不需要设 Threshold，因为 Score 默认为 0
	})
	d.printTestResult("空间检索", res2, err)

	// --- 测试 3: 语义 + 属性过滤 [精准筛选测试] ---
	// log.Warn(">>> 测试 3: 混合检索 [聚合函数版]")
	// res3, err := d.SearchTripMemory(ctx, SearchOptions{
	// 	Text:      "风景非常壮观",
	// 	Filters:   map[string]interface{}{"is_high_alt": true},
	// 	Limit:     3,
	// 	Threshold: 0.62,
	// })
	// d.printTestResult("带属性过滤搜索", res3, err)

	// // --- 测试 4: 无关词干扰 [系统鲁棒性测试] ---
	// log.Warn(">>> 测试 4: 无关词拦截 [聚合函数版]")
	// res4, err := d.SearchTripMemory(ctx, SearchOptions{
	// 	Text:      "今天晚上想吃什么外卖",
	// 	Limit:     1,
	// 	Threshold: 0.62, // 关键：利用门槛直接拦截
	// })
	// d.printTestResult("无关内容搜索", res4, err)

	log.Info("所有 AI 功能测试跑通！")
	return nil
}

// 辅助打印，让你的控制台更整齐
func (d *RgaTripMemoryDbx) printTestResult(title string, results []*qdrant.ScoredPoint, err error) {
	if err != nil {
		log.Error("[%s] 失败: %v", title, err)
		return
	}
	log.Info("[%s] 命中数量: %d", title, len(results))
	for i, hit := range results {
		summary := hit.Payload["summary"].GetStringValue()
		location_name := hit.Payload["location_name"].GetStringValue()
		road_name := hit.Payload["road_name"].GetStringValue()
		log.Info(i+1, hit.Score, location_name, road_name)
		log.Info(summary)
	}
}

// 将 Trip数据 转换成 AI记忆
func (d *RgaTripMemoryDbx) IngestTripMemory(trip *models.Trip) error {

	if trip.LastSegmentationTime > 0 {
		log.Error("已生成完毕切片，不用再处理了", trip.Id)
		return nil
	}

	log.Warn("开始生成切片！！！！", trip.Id, trip.AuthorId,
		time.Unix(trip.CreateTime, 0).Format("2006-01-02 15:04"))

	// return nil

	t := log.Time()
	defer t.TimeEnd("IngestTripMemory => " + trip.Id)
	// d.TesttTripMemory(tripId)

	// return nil

	log.Info("行程Id", trip.Id)

	log.Info("1、获取行程数据")

	if _, err := readGPSFile(trip); err != nil {
		return err
	}

	log.Info("行程信息 => ", trip.Id, trip.Cities, len(trip.Positions))

	// geoStr, err := LineStringToGeoJSON(simplifyPositions)
	// if err != nil {
	// 	log.Error(err)
	// }

	// log.Info(geoStr)
	if len(trip.Positions) == 0 {
		return nil
	}

	log.Info("2. 抽稀 (Simplification)")

	simplifyPositions := d.Simplify(narrays.Map(trip.Positions,
		func(v *models.TripPosition, i int) *Point {
			return &Point{
				Lat: v.Latitude,
				Lng: v.Longitude,
			}
		}), 0.0002)
	log.Info("Trip资料 => ", trip.Statistics.Distance,
		len(trip.Positions), len(simplifyPositions))

	// if len(simplifyPositions) == 0 {
	// 	return nil
	// }

	// // 3. 切片与特征提取 (Segmentation & Feature Extraction)
	// // 根据特征点把行程切成若干个 5km 或 10km 的 Segment
	segments := d.CreateSegments(trip.Positions, simplifyPositions, 5000)

	log.Error(
		"切片数量 => ", len(segments), trip.Id)

	for i, seg := range segments {

		// if i < 30 || i > 30 {
		// 	continue
		// }

		// time.Sleep(2 * time.Second)

		// if i > 0 {
		// 	continue
		// }
		// if len(seg.RawData) == 0 {
		// 	continue
		// }

		err := func() error {
			// // 1. 创建一个带 10 秒超时的 ctx (针对单次写入)
			ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
			defer cancel() // 此时 defer 会在这个匿名函数结束时立即执行

			// 先通过ID检测数据库有没有，没有再进行下面的操作
			pointId := aiDbx.GeneratePointID(fmt.Sprintf("%s:seg:%d", trip.Id, i))

			// if pointId != "039e8fcc-de7e-5fa2-b1d8-655c417971db" {
			// 	return nil
			// }

			t := log.Time()
			defer t.TimeEnd(nstrings.ToString(i) + "的流程")

			// log.Info("pointId", i, pointId)

			t1 := log.Time()
			point, err := d.GetPointById(ctx, pointId)
			if err != nil {
				log.Error(err)
				return err
			}

			t1.TimeEnd("GetPointById " + pointId + " " + nstrings.ToString(point != nil))

			// // 已经存在，无需在存
			if point != nil {
				log.Error("已存在", i, pointId, point != nil)
				return nil
			}

			// 检测数据库是否有此向量数据

			// log.Info("segments", seg.Distance, len(seg.RawData), seg.RawData[0].Timestamp)

			// 4. 提取统计信息和时间

			segCtx, err := d.BuildSegmentContext(trip, seg)
			if err != nil {
				log.Error(err)
				return err
			}
			if segCtx == nil {
				return nil
			}
			// if sContext.IsSharpTurnArea {
			// 	log.Info("sContext 急弯", i, sContext, sContext.Distance, sContext.PointCount)

			// }

			// 5. 补全地理信息、城市、道路、天气、信号强度

			// weatherInfo := methods.GetWeatherAndWindLabel

			summary, err := d.GenerateSummary(ctx, segCtx)
			if err != nil {
				log.Error(trip.Id, i, err)
				return err
			}
			// log.Info("GenerateSummary", i)
			log.Info("summary => ", i, summary)

			// 暂时的
			// return nil

			// summary := &SegmentSummary{
			// 	Summary: strings.Replace(
			// 		point.Payload["summary"].GetStringValue(), "【驾驶记忆】", "", 1),
			// 	SummarySource: "gemini-2.5-flash",
			// }
			// 6. 生成语义总结

			// // 7. 向量化 (Embedding)
			// // 将 summary 变成 768 维向量
			vector, err := aiDbx.GetEmbedding(TextTypeDocument, summary.Summary)
			if err != nil {
				log.Error(err)
				return err
			}
			log.Info("GetEmbedding", i, len(vector), vector[0])

			// 8. 存储 (Loading)

			payload := d.ToFinalPayload(trip, summary, segCtx, seg.RawData)

			// j1, _ := json.MarshalIndent(payload, "", "  ")
			// log.Info("payload", i, string(j1))
			// return nil
			// // 存入 Qdrant
			if err := d.SaveTripMemory(ctx, pointId, vector, payload); err != nil {
				log.Error(err)
				return err
			}

			return nil
		}()

		if err != nil {
			log.Error(err)
			// log.FullCallChain(err.Error(), "Error")

			return err
		}

	}

	// 更新切片时间

	ctx, cancel := context.WithTimeout(
		context.Background(), 10*time.Second)
	defer cancel()

	if err := tripDbx.UpdateLastSegmentationTime(ctx,
		trip.Id, trip.AuthorId, time.Now().Unix()); err != nil {
		log.Error(err)
		return err
	}

	return nil
}

func (d *RgaTripMemoryDbx) Simplify(rawPoints []*Point, threshold float64) orb.LineString {
	// 1. 将你的原始点转为 orb 的线段结构
	ls := orb.LineString{}
	for _, p := range rawPoints {
		ls = append(ls, orb.Point{p.Lng, p.Lat})
	}

	// 2. 执行 Douglas-Peucker 抽稀
	// threshold (阈值) 的单位通常由你的坐标系决定
	// 如果是 WGS84（经纬度），0.0001 大约是 10 米
	// 对于 340km 的西藏行程，建议从 0.0005 (约 50m) 开始调试
	simplified := simplify.DouglasPeucker(threshold).Simplify(ls).(orb.LineString)

	return simplified
}

// Segment 现在包含了这段路所有的原始元数据（含速度、海拔等）
type Segment struct {
	RawData  []*models.TripPosition
	Distance float64 // 米
}

func (d *RgaTripMemoryDbx) CreateSegments(rawPositions []*models.TripPosition, simplifiedLS orb.LineString, targetDist float64) []Segment {
	var segments []Segment
	if len(simplifiedLS) < 2 || len(rawPositions) < 2 {
		return nil
	}

	// rawIdx 指向原始数据的进度
	rawIdx := 0
	var currentDist float64
	currentSeg := Segment{}

	// 遍历抽稀后的每一个“线段” (p1 -> p2)
	for i := 1; i < len(simplifiedLS); i++ {
		p1 := simplifiedLS[i-1]
		p2 := simplifiedLS[i]

		// 1. 计算这一小段抽稀路径的物理距离
		d := geo.Distance(p1, p2)
		currentDist += d

		// 2. 将原始数据中，处于 p1 和 p2 之间的所有点塞进当前 Segment
		// 我们通过坐标匹配来确定原始数据走到了哪
		for rawIdx < len(rawPositions) {
			rp := rawPositions[rawIdx]
			currentSeg.RawData = append(currentSeg.RawData, rp)

			// 匹配逻辑：如果当前原始点就是抽稀后的终点 p2
			// 说明这一小段“蒸馏”过程对应的原始点已经搜集完毕
			if rp.Longitude == p2.X() && rp.Latitude == p2.Y() {
				// rawIdx 停在这里，下一段 i 循环从这个点继续往后找
				break
			}
			rawIdx++
		}

		// 3. 检查是否达到切片阈值（如 5km），或者到了整趟旅程的终点
		if currentDist >= targetDist || i == len(simplifiedLS)-1 {
			currentSeg.Distance = currentDist
			segments = append(segments, currentSeg)

			// 重置切片容器，准备装下一个 5km
			currentSeg = Segment{}
			// 重点：下一段的起点应该是当前段的终点
			if rawIdx < len(rawPositions) {
				currentSeg.RawData = append(currentSeg.RawData, rawPositions[rawIdx])
			}
			currentDist = 0
		}
	}

	return segments
}

type SegmentContext struct {
	Lat float64
	Lon float64

	// --- 物理属性 (基础统计) ---
	Distance         float64
	AvgSpeed         float64
	MaxSpeed         float64
	MinSpeed         float64
	ElevationGain    float64
	ElevationAscent  float64
	ElevationDescent float64
	StartAlt         float64
	EndAlt           float64
	PointCount       int

	// --- 环境属性 (体感核心) ---
	Weather             string  // 建议传入：晴/雨/雪/雾/多云
	Temperature         float64 // 环境温度
	ApparentTemperature float64 // 体验温度
	WindSpeed           float64 // 风速
	WindDirection       string  // 风向
	Humidity            float64 // 湿度
	Precipitation       float64 // 降水量

	// 网络属性
	SignalQuality int    // 建议：0(无信号), 1(有信号)
	NetworkType   string // 建议：5G/4G/NoService (西藏自驾失联感很关键)

	// --- 空间属性 (地理语义) ---
	LocationName string        // 昌都市八宿县
	RoadName     []string      // G318
	POIs         []*POIPayload // 怒江72拐/东达山垭口

	// --- 时间属性 ---
	StartTime time.Time
	Duration  time.Duration
	TimeOfDay string // 清晨/正午/傍晚/深夜

	// --- 衍生属性 (给 AI 的“小纸条”) ---
	IsHighAltitude  bool   // 海拔 > 4000m 触发感性文案
	IsSteepClimb    bool   // 爬升 > 200m 触发“艰难翻越”文案
	IsSteepDescent  bool   // 下降 > 200m 触发“艰难翻越”文案
	IsSharpTurnArea bool   // 点密度极高 触发“压弯/险峻”文案
	DriveStyle      string // 根据 SpeedStability 转换成的文字：稳健/激进/受阻

}

func (s *RgaTripMemoryDbx) BuildSegmentContext(trip *models.Trip, seg Segment) (*SegmentContext, error) {
	if len(seg.RawData) == 0 {
		return nil, nil
	}

	var maxSpeed float64
	minSpeed := 999.0
	startPos := seg.RawData[0]
	endPos := seg.RawData[len(seg.RawData)-1]

	// 1. 基础遍历统计
	ascent, descent := float64(0), float64(0)
	for i, p := range seg.RawData {
		if p.Speed > maxSpeed {
			maxSpeed = p.Speed
		}
		if p.Speed < minSpeed {
			minSpeed = p.Speed
		}

		if i+1 < len(seg.RawData) {
			diff := seg.RawData[i].Altitude - seg.RawData[i+1].Altitude
			if diff > 0 {
				ascent += diff
			} else {
				descent += math.Abs(diff) // 下降也记为正数，方便后续逻辑判断
			}
		}
	}
	maxSpeed = maxSpeed * 3.6
	minSpeed = minSpeed * 3.6

	// log.Info("maxSpeed", len(seg.RawData), maxSpeed, startPos.Timestamp, endPos.Timestamp)

	elevationGain := endPos.Altitude - startPos.Altitude

	// 2. 时间处理
	startTime := time.UnixMilli(startPos.Timestamp)
	duration := time.UnixMilli(endPos.Timestamp).Sub(startTime)

	avgSpeed := (seg.Distance / 1000) / duration.Hours()
	// log.Info("startPos.Timestamp", startPos.Timestamp)
	// log.Info("duration", duration)

	// 3. 派生字段逻辑判断

	// [判断：高海拔] 超过 4000 米标记为高海拔
	isHighAltitude := startPos.Altitude > 3000 || endPos.Altitude > 3000

	// [判断：陡峭爬升] 5km 内爬升超过 200 米
	isSteepClimb := ascent > 200

	isSteepDescent := descent > 200

	// [判断：急弯区域]
	// 在 20 米抽稀精度下，如果 5km (5000m) 的 Segment 包含了超过 60 个点
	// 意味着平均每 80 米就有一个关键转向点，这在山路是非常密集的
	// isSharpTurnArea := len(seg.RawData) > 60

	var totalHeadingChange float64
	validHeadingCount := 0

	for i := 1; i < len(seg.RawData); i++ {
		p1 := seg.RawData[i-1]
		p2 := seg.RawData[i]

		// 过滤掉无效数据和低速干扰（静止时 Heading 容易乱跳）

		// j, _ := json.MarshalIndent(p1, "", "  ")

		// log.Info(string(j))

		if p1.Heading >= 0 && p2.Heading >= 0 && p2.Speed > 5 {
			diff := math.Abs(p2.Heading - p1.Heading)
			// 处理 0 度和 360 度跨度问题
			if diff > 180 {
				diff = 360 - diff
			}
			totalHeadingChange += diff
			validHeadingCount++
		}
	}
	isSharpTurnArea := totalHeadingChange > 1500
	log.Info("totalHeadingChange",
		totalHeadingChange, validHeadingCount,
		isSharpTurnArea)

	// 如果 5 公里内累计转角超过 1500 度（相当于转了 4 个多全圆），判定为急弯/盘山路

	// [判断：驾驶风格]
	// 简单逻辑：如果最高速和最低速差距巨大，且平均速不高，说明走走停停或频繁过弯
	driveStyle := "平稳巡航"
	speedDiff := maxSpeed - minSpeed
	if speedDiff > 40 {
		driveStyle = "激烈驾驶"
	} else if avgSpeed < 20 && isSharpTurnArea {
		driveStyle = "艰难蠕行"
	}

	var centerLat, centerLon float64
	count := float64(len(seg.RawData))
	var sumLat, sumLon float64
	for _, p := range seg.RawData {
		sumLat += p.Latitude
		sumLon += p.Longitude
	}
	centerLat = aiDbx.Round(sumLat/count, 6)
	centerLon = aiDbx.Round(sumLon/count, 6)

	segCtx := SegmentContext{
		Lat: centerLat,
		Lon: centerLon,
		// 物理统计
		Distance:         aiDbx.Round(seg.Distance, 2),
		AvgSpeed:         aiDbx.Round(avgSpeed, 2),
		MaxSpeed:         aiDbx.Round(maxSpeed, 2),
		MinSpeed:         aiDbx.Round(minSpeed, 2),
		ElevationGain:    aiDbx.Round(elevationGain, 2),
		ElevationAscent:  aiDbx.Round(ascent, 2),
		ElevationDescent: aiDbx.Round(descent, 2),
		StartAlt:         aiDbx.Round(startPos.Altitude, 2),
		EndAlt:           aiDbx.Round(endPos.Altitude, 2),
		PointCount:       len(seg.RawData),

		// 环境与语义
		StartTime: startTime,
		Duration:  duration,
		TimeOfDay: aiDbx.getTimeOfDay(startTime),

		// 派生标签
		IsHighAltitude:  isHighAltitude,
		IsSteepClimb:    isSteepClimb,
		IsSteepDescent:  isSteepDescent,
		IsSharpTurnArea: isSharpTurnArea,
		DriveStyle:      driveStyle,

		// 空间信息留空供外部注入
		LocationName: "",
		RoadName:     []string{},
	}

	centerPos := seg.RawData[len(seg.RawData)/2]
	timestamp := centerPos.Timestamp / 1000

	// 暂时的
	cities, err := tripDbx.FindCityByTime(trip.Cities, timestamp)
	if err != nil {
		log.Error(err)
		return nil, err
	}
	segCtx.LocationName = cityDbx.GetCityAddresses(cities, "zh-CN").Address

	road := tripDbx.FindRoadByTime(trip.Roads, timestamp)
	ns := tripDbx.FindNetworkStatusByTime(trip.NetworkStatus, timestamp)
	weather := tripDbx.FindWeatherByTime(trip.Weather, timestamp)

	// log.Info("weather", weather, trip.Weather)
	// log.Info("centerPos", centerPos)
	// log.Info("centerPos", timestamp,
	// 	cities,
	// 	road, ns, weather)

	// segCtx.Weather = "晴"
	// segCtx.Temperature = 26
	if road != nil {
		segCtx.RoadName = narrays.Filter(
			narrays.Map(road.Roads, func(val *models.TripRoadInfo, i int) string {

				return strings.TrimSpace(strings.Join([]string{
					val.Code, val.Name.ZhHans,
				}, " "))
			}), func(val string, i int) bool {
				return val != ""
			})

	}

	if ns != nil {
		segCtx.SignalQuality = ncommon.IfElse(ns.Status == 1, 1, -1)
	}

	if weather != nil {
		segCtx.Weather = methods.GetWeatherText(int(weather.WeatherCode), methods.ZH_CN)
		segCtx.Temperature = weather.Temperature
		segCtx.ApparentTemperature = weather.ApparentTemperature

		segCtx.WindSpeed = weather.WindSpeed
		segCtx.WindDirection = methods.GetWindDirectionText(float64(weather.WindDirection), methods.ZH_CN)
		segCtx.Humidity = weather.Humidity
		segCtx.Precipitation = weather.Precipitation

	}

	// segCtx.Weather = "多云"
	// segCtx.Temperature = 25.1
	// segCtx.WindSpeed = 2.6
	// segCtx.WindDirection = "东南风"
	// segCtx.Humidity = 53
	// segCtx.Precipitation = 0

	// log.Info("segCtx", segCtx)
	// log.Info(segCtx.StartTime.Format("2006-01-02 15:04"))
	// j1, _ := json.MarshalIndent(weather, "", "  ")
	// log.Info(string(j1))
	// j, _ := json.MarshalIndent(segCtx, "", "  ")
	// log.Info(string(j))

	// log.Info(trip.Cities)
	// log.Info(trip.NetworkStatus)
	// log.Info(trip.Weather)

	radius := poiDbx.CalculateSearchRadius(segCtx.EndAlt)

	// lat := float64(33.2681291)
	// lng := float64(103.9176684)
	params := POISearchParams{
		Lat: &segCtx.Lat,
		Lon: &segCtx.Lon,
		// Lat: &lat,
		// Lon: &lng,
		Radius: radius,
		Limit:  10,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()
	poiPoints, err := poiDbx.SearchPOI(ctx, params)
	if err != nil {
		return nil, err
	}

	if len(poiPoints) > 0 {

		for _, v := range poiPoints {
			// str, _ := json.Marshal(v.POI)
			// log.Info(string(str))

			log.Info(v.POI.Name, v.POI.OSM.Type,
				v.POI.Importance, v.POI.Distance,
				v.POI.Location.Lat, v.POI.Location.Lon)

			if v.POI != nil && len(segCtx.POIs) < 3 {
				segCtx.POIs = append(segCtx.POIs, v.POI)
			}
		}

		log.Info("poiPoints", segCtx.Lat, segCtx.Lon, radius, poiPoints)

	}

	return &segCtx, nil
}

type SegmentSummary struct {
	Summary       string
	SummarySource string
}

var (
	llmCount   = 0
	totalCount = 0
	limiter    = rate.NewLimiter(rate.Every(3*time.Second), 1)
)

func (s *RgaTripMemoryDbx) GenerateSummary(ctx context.Context, seg *SegmentContext) (*SegmentSummary, error) {
	t := log.Time()
	defer t.TimeEnd("GenerateSummary")

	summary := &SegmentSummary{
		Summary:       "",
		SummarySource: "",
	}

	// 检测走模板还是LLM

	// --- 1. 模板判定逻辑 ---
	// 如果不满足任何高光触发条件，则判定为“普通路段”，走模板流程
	isHighlight := seg.IsHighAltitude || seg.IsSteepClimb ||
		seg.IsSteepDescent ||
		seg.IsSharpTurnArea || seg.MaxSpeed > 130

	log.Info(isHighlight, seg.IsHighAltitude, seg.IsSteepClimb,
		seg.IsSteepDescent,
		seg.IsSharpTurnArea, seg.MaxSpeed > 130,
		seg.PointCount)

	totalCount++

	if !isHighlight {
		// for _, v := range seg.POIs {
		// 	// str, _ := json.Marshal(v.POI)
		// 	// log.Info(string(str))
		// 	log.Info(v.Name, v.OSM.Type,
		// 		v.Importance, v.Distance,
		// 		v.Location.Lat, v.Location.Lon)
		// }

		log.Error("执行模板生成逻辑, 获取次数", len(seg.POIs),
			llmCount, totalCount)
		// 执行模板生成逻辑
		// log.Info("buildTemplateContent")
		summary.Summary = s.buildTemplateContent(seg)
		summary.SummarySource = "template_v1"

		// 走模板则直接返回，不执行后续 LLM 代码

		log.Info(summary)

		// return nil, errors.New("提前结束，限流报错了")
		return summary, nil
	}

	// j, _ := json.MarshalIndent(seg, "", "  ")
	// log.Info(string(j))

	llmCount++
	log.Error("开始走LLM获取Summary, 获取次数", llmCount, totalCount)

	// lat := float64(29.423152)
	// lng := float64(105.593505)

	// 暂时的
	// return summary, nil
	// return summary, errors.New("提前结束，限流报错了")

	if err := limiter.Wait(ctx); err != nil {
		return nil, errors.New("提前结束，限流报错了")
	}

	// 走LLM
	// 1. 数据预处理与 Fallback 字符串准备
	loc := seg.LocationName
	if loc == "" {
		loc = "未知区域"
	}

	// 2. 动态语境分析 (内联逻辑：决定 AI 的写作侧重点)
	var focusPoints []string
	if seg.IsHighAltitude {
		focusPoints = append(focusPoints, "当前处于海拔4000米以上的生命禁区，强调稀薄氧气带来的压迫感和壮丽景色。")
	}
	if seg.IsSteepClimb {
		focusPoints = append(focusPoints, fmt.Sprintf("这段路有 %.f 米的剧烈爬升，侧重描写车辆引擎的嘶吼和翻山越岭的征服感。", seg.ElevationAscent))
	}
	if seg.IsSteepDescent {
		focusPoints = append(focusPoints, fmt.Sprintf("这段路有 %.f 米的剧烈下降，侧重描写车辆刹车的高温和电车动能回收的拉满。", seg.ElevationDescent))
	}

	if seg.IsSharpTurnArea {
		focusPoints = append(focusPoints, "这里弯道极度密集，描写方向盘在手中的频繁跳动和惊险的路面节奏。")
	}
	// if seg.NearbyPOI != "" {
	// 	focusPoints = append(focusPoints, fmt.Sprintf("路过著名地标「%s」，它是这段行程的视觉灵魂，必须提及。", seg.NearbyPOI))
	// }
	if seg.Temperature < 0 {
		focusPoints = append(focusPoints, "室外气温已降至冰点以下，强调车内外的冰火两重天。")
	}
	if seg.TimeOfDay == "深夜" || seg.TimeOfDay == "清晨" {
		focusPoints = append(focusPoints, "此时光影独特，强调孤寂的灯光或第一抹晨曦带来的孤独英雄感。")
	}

	narratives := []string{
		"侧重【地理质感】：像地理学家一样描述海拔起伏、地貌特征和环境的压迫感。",
		"侧重【驾驶快感】：老司机的直觉，强调均速与最高速带来的那种破风而行的速度感。",
		"侧重【时间流逝】：融入当时的光影（如清晨、深夜）与节奏，写出那种正在路上的动态感。",
	}
	// 如果没有特殊触发点，给一个基础基调
	dynamicFocus := narratives[rand.Intn(len(narratives))]
	if len(focusPoints) > 0 {
		dynamicFocus = strings.Join(focusPoints, "\n")
	}

	roadName := ""
	switch len(seg.RoadName) {
	case 0:
		roadName = "未知路段"
	case 1:
		roadName = seg.RoadName[0]
	default:
		roadName = strings.Join(seg.RoadName, " / ") + " 共线段"
	}

	// seg.NetworkType
	networkType := "未知"

	signalDesc := "未知"
	switch seg.SignalQuality {
	case 1:
		signalDesc = "信号正常"
	case -1:
		signalDesc = "信号失联"
	default:
		signalDesc = "未知"
	}

	// 1. 初始化基础天气和气温
	weatherDesc := "气象监测暂缺，只剩引擎声在山谷回荡。"
	if seg.Weather != "" && seg.Weather != "未知" {
		// 核心：天气 + 气温
		weatherDesc = fmt.Sprintf("%s，气温 %.1f°C", seg.Weather, seg.Temperature)

		// 2. 叠加体感逻辑 (Sensory)
		diff := seg.ApparentTemperature - seg.Temperature
		if diff <= -3 {
			weatherDesc += "，寒风刺骨，体感远低于实际气温"
		} else if diff >= 3 {
			weatherDesc += "，烈日灼人，体感异常燥热"
		}

		// 3. 叠加风力逻辑 (Atmosphere)
		if seg.WindSpeed > 10.8 {
			weatherDesc += fmt.Sprintf("，遭遇 %.1f m/s 强风横扫", seg.WindSpeed)
		} else if seg.WindSpeed > 5.5 {
			weatherDesc += "，阵风不断，车身偶有晃动"
		}

		// 4. 叠加降水/湿度逻辑 (Texture)
		if seg.Precipitation > 0 {
			weatherDesc += fmt.Sprintf("，伴有 %.1fmm 降水，路面湿滑", seg.Precipitation)
		} else if seg.Humidity > 85 {
			weatherDesc += "，空气粘稠潮湿"
		}
	}

	poiDesc := "无显著地标"
	wikiDesc := "暂无百科参考" // 初始化百科背景

	if len(seg.POIs) > 0 {
		var pStr []string
		var wStr []string
		for i, p := range seg.POIs {
			// 1. 构造地理锚点字符串 (保持你原有的逻辑)
			if i == 0 {
				pStr = append(pStr, fmt.Sprintf("%s(最近, 约%.0fm)", p.Name, p.Distance))
			} else if i < 3 {
				pStr = append(pStr, fmt.Sprintf("%s(约%.1fkm)", p.Name, p.Distance/1000))
			}

			// 2. 提取百科信息 (只取前2个，避免内容太杂乱)
			if i < 2 && p.Wiki.Summary != "" {
				// 简单截断：取前150个字符，防止百科太长喧宾夺主
				summary := []rune(p.Wiki.Summary)
				if len(summary) > 150 {
					wikiDesc = string(summary[:150]) + "..."
				} else {
					wikiDesc = string(summary)
				}
				wStr = append(wStr, fmt.Sprintf("【%s】: %s", p.Name, wikiDesc))
			}
		}
		poiDesc = strings.Join(pStr, " | ")
		if len(wStr) > 0 {
			wikiDesc = strings.Join(wStr, "\n")
		}
	}

	// 3. 构造请求消息
	messages := []openai.ChatCompletionMessage{
		{
			Role: openai.ChatMessageRoleSystem,
			Content: `你是一个硬核自驾游博主，文字风格参考《中国国家地理》的严谨与朋友圈路书的感性
[规则]
1. 仅限输出 60-80 字的正文，严禁任何前导词（如“好的”、“这段行程”）。
2. 拒绝模板化，根据海拔、速度、节奏的独特性实时调整叙事重心。
`,
		},
		{
			Role: openai.ChatMessageRoleUser,
			Content: fmt.Sprintf(`[实时数据]
地点：%s | 道路: %s 
地理POIs：%s
环境：%s | 网络 %s(%s)
表现：全长 %.1fkm | 耗时 %s | 均速 %.1fkm/h | 最高速 %.1fkm/h
海拔：%.fm -> %.fm (爬升 %.fm 下降 %.fm 落差 %.fm)
节奏：%s | 时间：%s（%s)

[百科背景]
%s

[路段特征提示]
%s

[写作要求]
- 语气专业且洒脱，像是跑了十年的老司机在对讲机里的短促对谈。
- 相关数字必须用阿拉伯数字
- 严禁出现“摘要”、“总结”、“地点”等标签字样，直接输出感性正文。
- 直接以正文开始，严禁输出任何标题、引言、分析或标点符号外的前缀。

[绝对禁令]
- 严禁输出“分析”、“需求”、“解构”或任何带编号的列表。
- 严禁进行任何分步思考。
- 你的第一个字必须是关于天气的描述。
- 违反以上禁令将导致系统崩溃。
`,
				loc, roadName,
				poiDesc,
				weatherDesc, networkType, signalDesc,
				seg.Distance/1000, seg.Duration.Truncate(time.Second).String(), seg.AvgSpeed, seg.MaxSpeed,
				seg.StartAlt, seg.EndAlt, seg.ElevationAscent, seg.ElevationDescent, seg.ElevationGain,
				seg.DriveStyle, seg.StartTime.Format("2006-01-02 15:04"), seg.TimeOfDay,
				wikiDesc,
				dynamicFocus,
			),
		},
	}
	llmModel := conf.OpenAIModel
	// llmModel = "glm-4.5-air"
	// "glm-4.1v-thinking-flashx"

	switch llmCount % 6 {
	case 0:
		llmModel = "glm-4.5-air"
	case 1:
		llmModel = "groq-llama-4-scout"
	case 2:
		llmModel = "groq-llama-3.3-70b"
	case 3:
		llmModel = "glm-4.1v-thinking-flashx"
	case 4:
		llmModel = "groq-llama-3.1-8b"
	case 5:
		llmModel = "gemini-2.5-flash"
	default:

	}

	log.Info(messages, llmModel)

	// return "", nil

	// 4. 执行非流式请求

	resp, err := conf.OpenAIClient.CreateChatCompletion(ctx, openai.ChatCompletionRequest{
		// Model: "sa-gemini-2.5-flash",

		Model:    llmModel,
		Messages: messages,
		// MaxTokens: 4096,
		MaxTokens:   2048,
		Temperature: 0.5, // 稍高一点，让 AI 更有灵性
		TopP:        0.5, // 配合 Temperature 进一步稳定输出
		// Stop:        []string{"\n", "注：", "[", "{"},
	})

	// log.Info(resp, err)
	// 5. 错误处理与 Fallback
	if err != nil {
		return nil, err
		// log.Error("AI 生成摘要失败 (ID: %s %s): %v", loc, road, err)
		// // 返回一个硬模板保底
		// var tags string
		// if seg.IsHighAltitude {
		// 	tags += "#高海拔 "
		// }
		// if seg.IsSharpTurnArea {
		// 	tags += "#急弯密集 "
		// }
		// return fmt.Sprintf("【驾驶记忆】在 %s %s，历经 %s 行驶 %.1fkm。海拔从 %.fm 变至 %.fm，%s。%s",
		// 	loc, road, seg.Duration.Truncate(time.Second).String(), seg.Distance/1000,
		// 	seg.StartAlt, seg.EndAlt, seg.DriveStyle, tags), nil
	}

	// 6. 清理并返回结果
	// log.Info(resp)

	//  模型:  gemini-2.5-flash 耗费Token:  1160 summary:  【驾驶记忆】
	// 傍晚G318，巴塘段4400米高空，稀薄氧气压迫着每一寸神经。方向盘在手中
	// 频繁跳动，不是速度，是这“生命禁区”的极致弯道。均速10公里，并非慢，
	// 而是对这壮丽又险峻的敬畏。夕阳余晖下，每一米下坠都是对极限的挑战。

	//  模型:  glm-4.7-flash 耗费Token:  1987 summary:  【驾驶记忆】四千米
	// 生命禁区，稀薄空气令人窒息。G318弯道密集如织，方向盘在手中剧烈跳动，
	// 我们艰难蠕行五公里，时速仅十公里。在这傍晚的绝美险途，用最慢的速度，
	// 丈量极致的压迫与壮丽。

	log.Info(resp.Choices[0].FinishReason)
	// log.Info(resp.Choices[0].Message)
	log.Info(resp.Choices[0].Message)
	summaryStr := strings.TrimSpace(resp.Choices[0].Message.Content)
	log.Info("summaryStr", summaryStr, len(summaryStr), resp.Model)
	if summaryStr == "" {
		return nil, errors.New("获取失败")
	}
	if len(summaryStr) > 500 {
		return nil, errors.New("字数太长")
	}

	badPrefixes := []string{"\n", "注：", "[", "{", "**"}
	for _, p := range badPrefixes {
		if strings.Contains(summaryStr, p) {

			return nil, errors.New("触发了错误关键词 -> " + p)
		}
	}

	summary.Summary = summaryStr
	summary.SummarySource = resp.Model

	log.Info("模型: ", resp.Model,
		"耗费Token: ", resp.Usage.TotalTokens, "summary: ", summary)
	return summary, nil
}

// buildTemplateContent 负责具体的模板拼接逻辑
func (s *RgaTripMemoryDbx) buildTemplateContent(seg *SegmentContext) string {
	// 1. 基础信息处理
	road := "未知路段"
	if len(seg.RoadName) > 0 {
		road = seg.RoadName[0]
	}

	// 2. 时间人性化处理：提取 "14:05" 这种格式
	timeStr := seg.StartTime.Format("15:04")
	// 还可以更感性一点，比如 "14点许"
	timeDesc := fmt.Sprintf("%d点许", seg.StartTime.Hour())

	// 2. 动态环境描述 (处理老数据无天气的情况)
	envDesc := ""
	if seg.Weather != "" {
		envDesc = fmt.Sprintf("，天气%s，气温%.1f℃", seg.Weather, seg.Temperature)
	}

	// log.Warn("seg.Weather", envDesc)

	// 3. 信号描述 (增加西藏自驾的“荒野感”)

	signalDesc := ""
	switch seg.SignalQuality {
	case 1:
		signalDesc = "网络连接正常"
	case -1:
		signalDesc = "身处信号荒原"
	default:
		signalDesc = "未知"
	}

	// 4. 海拔趋势描述
	altTrend := "平稳滑行"
	if seg.ElevationGain > 100 {
		altTrend = fmt.Sprintf("持续爬升(累计+%.0fm)", seg.ElevationAscent)
	} else if seg.ElevationGain < -100 {
		altTrend = fmt.Sprintf("一路下探(累计-%.0fm)", seg.ElevationDescent)
	}

	poiContext := ""
	if len(seg.POIs) > 0 {
		// 取排序后的第一个作为近点锚点
		p1 := seg.POIs[0]
		poiContext = fmt.Sprintf("，临近%s", p1.Name)

		// 如果有第二个高分 POI 且距离稍远，增加“眺望感”
		if len(seg.POIs) > 1 {
			p2 := seg.POIs[1]
			if p2.Distance > 1000 {
				poiContext += fmt.Sprintf("，远眺%s", p2.Name)
			} else {
				poiContext += fmt.Sprintf("并掠过%s", p2.Name)
			}
		}
	} else {
		// 无 POI 时的荒野修正
		if seg.EndAlt > 4500 {
			poiContext = "，四周唯有荒原与孤寂"
		}
	}

	// 5. 构思 5 套话术模板 (确保每个占位符都有对应的变量)

	// log.Info(seg)
	var summary string
	// 7. 5套话术，巧妙融入 POI 描述
	randIdx := rand.Intn(5)
	switch randIdx {
	case 0:
		// 风格：精确坐标感 (强调具体地标)
		summary = fmt.Sprintf("记录于%s，车辆正穿行在%s的%s%s。这段路%s，均速%.1fkm/h%s。当前海拔%.0fm。",
			timeStr, seg.LocationName, road, poiContext, altTrend, seg.AvgSpeed, envDesc, seg.EndAlt)
	case 1:
		// 风格：驾驶感 (融入地标作为背景)
		summary = fmt.Sprintf("正值%s，在%s%s，沿%s的驾驶风格显得颇为%s。海拔在%.0fm至%.0fm之间起伏%s。",
			seg.TimeOfDay, seg.LocationName, poiContext, road, seg.DriveStyle, seg.StartAlt, seg.EndAlt, envDesc)
	case 2:
		// 风格：旅途日记 (感性描述)
		summary = fmt.Sprintf("%s，车轮滚过%s%s。5公里的行程%s，均速维持在%.1fkm/h。海拔约%.0fm%s，%s。",
			timeDesc, road, poiContext, altTrend, seg.AvgSpeed, seg.EndAlt, envDesc, signalDesc)
	case 3:
		// 风格：地理播报 (极简，突出地标)
		summary = fmt.Sprintf("定位%s%s，沿%s向前推进。%s，当前高度%.0fm。%s%s。",
			seg.LocationName, poiContext, road, altTrend, seg.EndAlt, signalDesc, envDesc)
	default:
		// 风格：综合叙事 (氛围感)
		summary = fmt.Sprintf("车辆正经过%s%s。在%s的映衬下，以%.1fkm/h的速度%s。%s，实时海拔%.0fm。",
			seg.LocationName, poiContext, seg.TimeOfDay, seg.AvgSpeed, altTrend, signalDesc, seg.EndAlt)
	}

	return summary
}

func (d *RgaTripMemoryDbx) ToFinalPayload(
	trip *models.Trip,
	summary *SegmentSummary,
	ctx *SegmentContext,
	rawData []*models.TripPosition,
) map[string]*qdrant.Value { // 修改了返回类型

	// --- 1. 原有的地理中心计算逻辑 (保持不动) ---
	var roadValues []*qdrant.Value
	for _, road := range ctx.RoadName {
		roadValues = append(roadValues, &qdrant.Value{
			Kind: &qdrant.Value_StringValue{StringValue: road},
		})
	}

	// --- 2. 构造 gRPC 强类型 Payload ---
	return map[string]*qdrant.Value{
		"trip_id":        {Kind: &qdrant.Value_StringValue{StringValue: trip.Id}},
		"author_id":      {Kind: &qdrant.Value_StringValue{StringValue: trip.AuthorId}},
		"summary":        {Kind: &qdrant.Value_StringValue{StringValue: summary.Summary}},
		"summary_source": {Kind: &qdrant.Value_StringValue{StringValue: summary.SummarySource}},

		// 空间索引 (Qdrant Geo 索引要求的 Struct 结构)
		"location": {
			Kind: &qdrant.Value_StructValue{
				StructValue: &qdrant.Struct{
					Fields: map[string]*qdrant.Value{
						"lat": {Kind: &qdrant.Value_DoubleValue{DoubleValue: ctx.Lat}},
						"lon": {Kind: &qdrant.Value_DoubleValue{DoubleValue: ctx.Lon}},
					},
				},
			},
		},

		"location_name": {Kind: &qdrant.Value_StringValue{StringValue: ctx.LocationName}},
		"road_name": {
			Kind: &qdrant.Value_ListValue{
				ListValue: &qdrant.ListValue{
					Values: roadValues,
				},
			},
		},
		// "nearby_poi": {Kind: &qdrant.Value_StringValue{StringValue: ctx.NearbyPOI}},

		// 驾驶性能 (数值统一使用 Double 或 Integer)
		"distance":          {Kind: &qdrant.Value_DoubleValue{DoubleValue: ctx.Distance}},
		"avg_speed":         {Kind: &qdrant.Value_DoubleValue{DoubleValue: ctx.AvgSpeed}},
		"max_speed":         {Kind: &qdrant.Value_DoubleValue{DoubleValue: ctx.MaxSpeed}},
		"min_speed":         {Kind: &qdrant.Value_DoubleValue{DoubleValue: ctx.MinSpeed}},
		"elevation_gain":    {Kind: &qdrant.Value_DoubleValue{DoubleValue: ctx.ElevationGain}},
		"elevation_ascent":  {Kind: &qdrant.Value_DoubleValue{DoubleValue: ctx.ElevationAscent}},
		"elevation_descent": {Kind: &qdrant.Value_DoubleValue{DoubleValue: ctx.ElevationDescent}},
		"alt_start":         {Kind: &qdrant.Value_DoubleValue{DoubleValue: ctx.StartAlt}},
		"alt_end":           {Kind: &qdrant.Value_DoubleValue{DoubleValue: ctx.EndAlt}},

		// 环境感官 (补全了缺失的天气信息)
		"weather":              {Kind: &qdrant.Value_StringValue{StringValue: ctx.Weather}},
		"temperature":          {Kind: &qdrant.Value_DoubleValue{DoubleValue: ctx.Temperature}},
		"apparent_temperature": {Kind: &qdrant.Value_DoubleValue{DoubleValue: ctx.ApparentTemperature}},
		"wind_speed":           {Kind: &qdrant.Value_DoubleValue{DoubleValue: ctx.WindSpeed}},
		"wind_direction":       {Kind: &qdrant.Value_StringValue{StringValue: ctx.WindDirection}},
		"humidity":             {Kind: &qdrant.Value_DoubleValue{DoubleValue: ctx.Humidity}},
		"precipitation":        {Kind: &qdrant.Value_DoubleValue{DoubleValue: ctx.Precipitation}},

		// 网络属性
		"signal_quality": {Kind: &qdrant.Value_IntegerValue{IntegerValue: int64(ctx.SignalQuality)}},
		"network_type":   {Kind: &qdrant.Value_StringValue{StringValue: ctx.NetworkType}},
		// 时间维度
		"start_time":  {Kind: &qdrant.Value_IntegerValue{IntegerValue: ctx.StartTime.Unix()}},
		"duration":    {Kind: &qdrant.Value_DoubleValue{DoubleValue: ctx.Duration.Seconds()}},
		"time_of_day": {Kind: &qdrant.Value_StringValue{StringValue: ctx.TimeOfDay}},

		// AI 标签
		"is_high_alt":      {Kind: &qdrant.Value_BoolValue{BoolValue: ctx.IsHighAltitude}},
		"is_steep_climb":   {Kind: &qdrant.Value_BoolValue{BoolValue: ctx.IsSteepClimb}},
		"is_steep_descent": {Kind: &qdrant.Value_BoolValue{BoolValue: ctx.IsSteepDescent}},
		"is_sharp_turn":    {Kind: &qdrant.Value_BoolValue{BoolValue: ctx.IsSharpTurnArea}},
		"drive_style":      {Kind: &qdrant.Value_StringValue{StringValue: ctx.DriveStyle}},

		// 系统属性
		"point_count": {Kind: &qdrant.Value_IntegerValue{IntegerValue: int64(len(rawData))}},
		"created_at":  {Kind: &qdrant.Value_IntegerValue{IntegerValue: time.Now().Unix()}},
	}
}

func (d *RgaTripMemoryDbx) CheckPointExists(ctx context.Context, pointId string) (bool, error) {
	// 使用 Get 接口，这是按 ID 取回数据的最快路径
	res, err := conf.Qdrant.PointsClient.Get(ctx, &qdrant.GetPoints{
		CollectionName: conf.QdrantCollectionName.Trip,
		Ids: []*qdrant.PointId{
			{
				PointIdOptions: &qdrant.PointId_Uuid{Uuid: pointId},
			},
		},
		// 我们只需要知道是否存在，不需要取回 Payload 和 Vector，节省带宽
		WithPayload: &qdrant.WithPayloadSelector{SelectorOptions: &qdrant.WithPayloadSelector_Enable{Enable: false}},
		WithVectors: &qdrant.WithVectorsSelector{SelectorOptions: &qdrant.WithVectorsSelector_Enable{Enable: false}},
	})
	if err != nil {
		return false, fmt.Errorf("check point failed: %w", err)
	}

	// 如果返回的结果列表长度大于 0，说明数据已存在
	return len(res.GetResult()) > 0, nil
}

func (d *RgaTripMemoryDbx) GetPointById(ctx context.Context, pointId string) (*qdrant.RetrievedPoint, error) {
	// 1. 调用 Get 接口获取数据
	res, err := conf.Qdrant.PointsClient.Get(ctx, &qdrant.GetPoints{
		CollectionName: conf.QdrantCollectionName.Trip,
		Ids: []*qdrant.PointId{
			{
				PointIdOptions: &qdrant.PointId_Uuid{Uuid: pointId},
			},
		},
		// --- 关键修改：开启 Payload 提取 ---
		WithPayload: &qdrant.WithPayloadSelector{
			SelectorOptions: &qdrant.WithPayloadSelector_Enable{Enable: true},
		},
		// 向量通常很大且仅用于检索，如果你不需要重新计算距离，建议保持 false 节省带宽
		WithVectors: &qdrant.WithVectorsSelector{
			SelectorOptions: &qdrant.WithVectorsSelector_Enable{Enable: false},
		},
	})

	if err != nil {
		return nil, fmt.Errorf("fetch point failed: %w", err)
	}

	// 2. 检查结果是否存在
	points := res.GetResult()
	if len(points) == 0 {
		return nil, nil // 或者返回 fmt.Errorf("point not found")，取决于你的逻辑
	}

	// 3. 返回第一条匹配的数据
	return points[0], nil
}

// 辅助函数：将 uint32 转为指针
func uint32Ptr(v uint32) *uint32 {
	return &v
}

func (d *RgaTripMemoryDbx) GetPointByTripIdAndAuthorId(ctx context.Context, tripId string, authorId string) (*qdrant.RetrievedPoint, error) {
	// 1. 构造过滤条件：trip_id == xxx AND author_id == xxx
	filter := &qdrant.Filter{
		Must: []*qdrant.Condition{
			{
				ConditionOneOf: &qdrant.Condition_Field{
					Field: &qdrant.FieldCondition{
						Key: "trip_id",
						Match: &qdrant.Match{
							MatchValue: &qdrant.Match_Keyword{Keyword: tripId},
						},
					},
				},
			},
			{
				ConditionOneOf: &qdrant.Condition_Field{
					Field: &qdrant.FieldCondition{
						Key: "author_id",
						Match: &qdrant.Match{
							MatchValue: &qdrant.Match_Keyword{Keyword: authorId},
						},
					},
				},
			},
		},
	}

	// 2. 使用 Scroll 接口进行条件查询
	res, err := conf.Qdrant.PointsClient.Scroll(ctx, &qdrant.ScrollPoints{
		CollectionName: conf.QdrantCollectionName.Trip,
		Filter:         filter,
		Limit:          uint32Ptr(1), // 我们只需要找到一条
		WithPayload: &qdrant.WithPayloadSelector{
			SelectorOptions: &qdrant.WithPayloadSelector_Enable{Enable: true},
		},
		WithVectors: &qdrant.WithVectorsSelector{
			SelectorOptions: &qdrant.WithVectorsSelector_Enable{Enable: false},
		},
	})

	if err != nil {
		return nil, fmt.Errorf("scroll points failed: %w", err)
	}

	// 3. 检查结果
	points := res.GetResult()
	if len(points) == 0 {
		return nil, nil // 未找到匹配数据
	}

	return points[0], nil
}

// SaveToQdrant 执行最终的写入操作
// 参数即为你已经准备好的：id (UUID字符串), vector ([]float32), payload (map[string]*qdrant.Value)
func (d *RgaTripMemoryDbx) SaveTripMemory(ctx context.Context, pointId string, vector []float32, payload map[string]*qdrant.Value) error {
	// 1. 构造 Upsert 请求
	// CollectionName 必须与你之前 curl 创建的一致
	waitTrue := true

	req := &qdrant.UpsertPoints{
		CollectionName: conf.QdrantCollectionName.Trip,
		// 设为 true 确保写入后立即可以被检索，适合灌数场景
		Wait: &waitTrue,
		Points: []*qdrant.PointStruct{
			{
				Id: &qdrant.PointId{
					PointIdOptions: &qdrant.PointId_Uuid{
						Uuid: pointId,
					},
				},
				Vectors: &qdrant.Vectors{
					VectorsOptions: &qdrant.Vectors_Vector{
						Vector: &qdrant.Vector{
							Data: vector,
						},
					},
				},
				Payload: payload,
			},
		},
	}

	// 2. 调用 gRPC 客户端执行写入
	_, err := conf.Qdrant.PointsClient.Upsert(ctx, req)
	if err != nil {
		return fmt.Errorf("qdrant upsert error: %w", err)
	}

	return nil
}

type GeoCoords struct {
	Lat float64
	Lon float64
}
type SearchTripMemoryOptions struct {
	Text         string                 // 语义搜索词
	Coords       *GeoCoords             // 空间搜索坐标
	RadiusMeters float32                // 搜索半径 (m)
	Filters      map[string]interface{} // 硬性过滤条件 (如 is_high_alt)
	Limit        uint64
	Threshold    float32 // 语义阈值

	// 新增筛选参数
	StartDate   string // 格式 "2026-04-29"
	EndDate     string // 格式 "2026-04-30"
	MinAltitude int
	MaxAltitude int
	StartHour   *int // 0-23
	EndHour     *int // 0-23
}

func (d *RgaTripMemoryDbx) SearchTripMemory(ctx context.Context, opt SearchTripMemoryOptions) ([]*qdrant.ScoredPoint, error) {
	var queryVector []float32
	var err error

	// 1. 处理向量：如果有文本，转向量；如果没有，给占位向量
	if opt.Text != "" {
		queryVector, err = aiDbx.GetEmbedding(TextTypeQuery, opt.Text)
		if err != nil {
			return nil, fmt.Errorf("embedding failed: %w", err)
		}
	} else {
		queryVector = make([]float32, 768) // 空间检索必备占位符
	}

	// 2. 构造 Filter 条件集合
	var mustConditions []*qdrant.Condition

	// --- A. 日期范围过滤 (created_at) ---
	if opt.StartDate != "" || opt.EndDate != "" {
		rangeCond := &qdrant.Range{}
		if opt.StartDate != "" {
			t, _ := time.ParseInLocation("2006-01-02", opt.StartDate, time.Local)
			startTs := float64(t.Unix())
			rangeCond.Gte = &startTs
		}
		if opt.EndDate != "" {
			t, _ := time.ParseInLocation("2006-01-02", opt.EndDate, time.Local)
			// 结束日期取该日最后一秒
			endTs := float64(t.AddDate(0, 0, 1).Unix() - 1)
			rangeCond.Lte = &endTs
		}
		mustConditions = append(mustConditions, &qdrant.Condition{
			ConditionOneOf: &qdrant.Condition_Field{
				Field: &qdrant.FieldCondition{
					Key:   "created_at",
					Range: rangeCond,
				},
			},
		})
	}

	// --- B. 海拔范围过滤 (alt_end) ---
	if opt.MinAltitude != 0 || opt.MaxAltitude != 0 {
		rangeCond := &qdrant.Range{}
		if opt.MinAltitude != 0 {
			minAlt := float64(opt.MinAltitude)
			rangeCond.Gte = &minAlt
		}
		if opt.MaxAltitude != 0 {
			maxAlt := float64(opt.MaxAltitude)
			rangeCond.Lte = &maxAlt
		}
		mustConditions = append(mustConditions, &qdrant.Condition{
			ConditionOneOf: &qdrant.Condition_Field{
				Field: &qdrant.FieldCondition{
					Key:   "alt_end",
					Range: rangeCond,
				},
			},
		})
	}

	// --- C. 行驶小时时间筛选 (hour) ---
	if opt.StartHour != nil && opt.EndHour != nil {
		sH, eH := *opt.StartHour, *opt.EndHour

		if sH != eH {
			// 1. 获取该小时区间涵盖的所有语义标签
			periods := aiDbx.GetTimeOfDayFromHours(sH, eH)

			if len(periods) > 0 {
				// 2. 构造 Match_Keywords 条件 (只要满足其中一个标签即可)
				mustConditions = append(mustConditions, &qdrant.Condition{
					ConditionOneOf: &qdrant.Condition_Field{
						Field: &qdrant.FieldCondition{
							Key: "time_of_day",
							Match: &qdrant.Match{
								MatchValue: &qdrant.Match_Keywords{
									Keywords: &qdrant.RepeatedStrings{
										Strings: periods,
									},
								},
							},
						},
					},
				})
			}
		}
	}

	// --- D. 硬性属性过滤 (Bool) ---
	for k, v := range opt.Filters {
		if b, ok := v.(bool); ok {
			mustConditions = append(mustConditions, &qdrant.Condition{
				ConditionOneOf: &qdrant.Condition_Field{
					Field: &qdrant.FieldCondition{
						Key:   k,
						Match: &qdrant.Match{MatchValue: &qdrant.Match_Boolean{Boolean: b}},
					},
				},
			})
		}
	}

	// --- E. 地理空间过滤 ---
	if opt.Coords != nil && opt.Coords.Lat != 0 {
		mustConditions = append(mustConditions, &qdrant.Condition{
			ConditionOneOf: &qdrant.Condition_Field{
				Field: &qdrant.FieldCondition{
					Key: "location",
					GeoRadius: &qdrant.GeoRadius{
						Center: &qdrant.GeoPoint{Lat: opt.Coords.Lat, Lon: opt.Coords.Lon},
						Radius: opt.RadiusMeters,
					},
				},
			},
		})
	}

	// 3. 执行查询
	if opt.Text == "" && opt.Coords != nil && opt.Coords.Lat != 0 {
		limit := uint32(opt.Limit)
		// log.Info("纯地理/属性检索 -> 使用 Scroll")
		scrollParams := &qdrant.ScrollPoints{
			CollectionName: conf.QdrantCollectionName.Trip,
			Filter:         &qdrant.Filter{Must: mustConditions}, // 这里依然包含了你的距离半径过滤
			Limit:          &limit,
			WithPayload:    &qdrant.WithPayloadSelector{SelectorOptions: &qdrant.WithPayloadSelector_Enable{Enable: true}},
			OrderBy: &qdrant.OrderBy{
				Key:       "created_at",
				Direction: qdrant.Direction_Desc.Enum(),
			},
		}

		res, err := conf.Qdrant.PointsClient.Scroll(ctx, scrollParams)
		if err != nil {
			return nil, err
		}

		// 注意：Scroll 返回的是 []*RetrievedPoint，需要转换成 []*ScoredPoint 以适配你的函数签名
		var scoredPoints []*qdrant.ScoredPoint
		for _, p := range res.GetResult() {
			scoredPoints = append(scoredPoints, &qdrant.ScoredPoint{
				Id:      p.Id,
				Payload: p.Payload,
				Score:   0, // 纯空间检索，分数无意义
			})
		}
		return scoredPoints, nil
	}

	// 4. 执行向量查询 (Search)
	searchParams := &qdrant.SearchPoints{
		CollectionName: conf.QdrantCollectionName.Trip,
		Vector:         queryVector,
		Limit:          opt.Limit,
		WithPayload: &qdrant.WithPayloadSelector{
			SelectorOptions: &qdrant.WithPayloadSelector_Enable{
				Enable: true,
			},
		},
	}

	// 如果设置了 Filter，装载进去
	if len(mustConditions) > 0 {
		searchParams.Filter = &qdrant.Filter{Must: mustConditions}
	}

	log.Info("mustConditions", mustConditions)

	// 设置门槛
	if opt.Text != "" {
		if opt.Threshold > 0 {
			searchParams.ScoreThreshold = &opt.Threshold
		} else {
			searchParams.ScoreThreshold = ToPtr(float32(0.6))
		}
	} else {
		// 重点：当 Text 为空时，必须将阈值设为 nil
		// 否则全 0 向量永远过不了默认的 0.6 门槛
		searchParams.ScoreThreshold = nil
	}

	// log.Info("searchParams", searchParams)
	// log.Info("searchParams", opt, opt.Text)

	res, err := conf.Qdrant.PointsClient.Search(ctx, searchParams)
	if err != nil {
		return nil, fmt.Errorf("qdrant search failed: %w", err)
	}

	return res.GetResult(), nil
}

func (d *RgaTripMemoryDbx) BatchCleanupAndReplanEmbedding(ctx context.Context) error {
	var offset *qdrant.PointId
	limit := uint32(50) // 涉及 AI 接口调用，建议将并发/批量大小调小，避免触发速率限制
	totalProcessed := 0
	count := 0

	for {
		// 1. 滚动查询 (不再需要 WithVectors，因为我们要重新生成)
		scrollReq := &qdrant.ScrollPoints{
			CollectionName: conf.QdrantCollectionName.Trip,
			Limit:          &limit,
			Offset:         offset,
			WithPayload:    &qdrant.WithPayloadSelector{SelectorOptions: &qdrant.WithPayloadSelector_Enable{Enable: true}},
		}

		resp, err := conf.Qdrant.PointsClient.Scroll(ctx, scrollReq)
		if err != nil {
			return fmt.Errorf("scroll error: %w", err)
		}

		if len(resp.Result) == 0 {
			break
		}

		var pointsToUpdate []*qdrant.PointStruct
		target := "。。"
		// target := "，身处信号荒原。"
		// target := "。身处信号荒原，"
		// target := "身处信号荒原"

		for _, point := range resp.Result {
			summaryVal, ok := point.Payload["summary"]
			if !ok {
				continue
			}

			summaryStr := summaryVal.GetStringValue()
			if strings.Contains(summaryStr, target) {
				// targets := []string{
				// 	"，身处信号荒原。",
				// 	"。身处信号荒原，",
				// 	"身处信号荒原",
				// }
				newSummary := summaryStr
				// for _, t := range targets {
				// 	// 根据你的逻辑：前两个换成句号，最后一个换成空
				// 	newSummary = strings.ReplaceAll(newSummary, t, "。")
				// }
				newSummary = strings.ReplaceAll(newSummary, "。。", "。")
				newSummary = strings.ReplaceAll(newSummary, "，。", "。")
				newSummary = strings.ReplaceAll(newSummary, "。，", "。")

				newSummary = strings.TrimSpace(newSummary)
				// 2. 清洗文本
				// newSummary := strings.ReplaceAll(summaryStr, target, "。")
				count++
				log.Info(count, summaryStr)
				log.Info(count, newSummary)
				if newSummary == "" {
					log.Error("结束")
					return nil
				}
				// 3. 核心：重新生成向量
				newVector, err := aiDbx.GetEmbedding(TextTypeDocument, newSummary)
				if err != nil {
					log.Error("点位 %s 重新生成 Embedding 失败: %v", point.Id.GetUuid(), err)
					continue
				}

				// 更新 Payload 中的 summary
				point.Payload["summary"] = &qdrant.Value{
					Kind: &qdrant.Value_StringValue{StringValue: newSummary},
				}

				// 构造更新点位
				pointsToUpdate = append(pointsToUpdate, &qdrant.PointStruct{
					Id: point.Id,
					Vectors: &qdrant.Vectors{
						VectorsOptions: &qdrant.Vectors_Vector{
							Vector: &qdrant.Vector{
								Data: newVector, // 使用新生成的向量
							},
						},
					},
					Payload: point.Payload,
				})
			}
		}

		// 4. 批量写回
		if len(pointsToUpdate) > 0 {
			waitTrue := true
			_, err = conf.Qdrant.PointsClient.Upsert(ctx, &qdrant.UpsertPoints{
				CollectionName: conf.QdrantCollectionName.Trip,
				Wait:           &waitTrue,
				Points:         pointsToUpdate,
			})
			if err != nil {
				return fmt.Errorf("batch upsert error: %w", err)
			}
			totalProcessed += len(pointsToUpdate)
			fmt.Printf("已更新并重新向量化 %d 条切片...\n", totalProcessed)
		}

		if resp.NextPageOffset == nil {
			break
		}
		offset = resp.NextPageOffset
	}

	return nil
}
