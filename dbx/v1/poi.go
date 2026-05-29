package dbxV1

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	conf "github.com/ShiinaAiiko/nyanya-trip-route-track/server/config"
	"github.com/ShiinaAiiko/nyanya-trip-route-track/server/services/methods"
	"github.com/cherrai/nyanyago-utils/nstrings"
	"github.com/qdrant/go-client/qdrant"
	"github.com/qedus/osmpbf"
)

type POIDbx struct {
}

type POICategory struct {
	Namespace string `json:"namespace"`
	Kind      string `json:"kind"`
}

type POI struct {
	ID       int64             `json:"id"`
	Name     string            `json:"name"`
	Type     string            `json:"type"`     // Node 或 Way
	Category *POICategory      `json:"category"` // 核心标签
	Lat      float64           `json:"lat"`
	Lon      float64           `json:"lon"`
	Tags     map[string]string `json:"tags"`
}

type point struct {
	lat float32
	lon float32
}

var (
	nodeIndex       map[int64]point
	referencedNodes map[int64]struct{}
	poiDbx          = new(POIDbx)
)

func (d *POIDbx) InitTripPOI() error {
	t := log.Time()
	defer t.TimeEnd("InitTripPOI")

	log.Info("------ 开始高效三阶段 POI 初始化 ------")
	pbfPath := "~/ShiinaAiikoDevWorkspace/Private/Devtools/pro/Nominatim/poi/china-trip-poi.osm.pbf"

	// --- 阶段 1: 标记 ---
	log.Info("[阶段 1/3] 扫描 Way 引用关系...")
	referencedNodes = make(map[int64]struct{})
	wayWithNames := 0

	f1, _ := os.Open(pbfPath)
	d1 := osmpbf.NewDecoder(f1)
	d1.Start(runtime.GOMAXPROCS(0))
	for {
		if v, err := d1.Decode(); err == io.EOF {
			break
		} else if w, ok := v.(*osmpbf.Way); ok {
			if name, ok := w.Tags["name"]; ok && name != "" {
				wayWithNames++
				for _, id := range w.NodeIDs {
					referencedNodes[id] = struct{}{}
				}
			}
		}
	}
	f1.Close()
	log.Info("✅ 阶段 1 完成: 发现带名称的 Way %d 个，共引用 Node %d 个", wayWithNames, len(referencedNodes))

	// --- 阶段 2: 索引 ---
	log.Info("[阶段 2/3] 建立精简坐标索引...")
	nodeIndex = make(map[int64]point)

	f2, _ := os.Open(pbfPath)
	d2 := osmpbf.NewDecoder(f2)
	d2.Start(runtime.GOMAXPROCS(0))
	for {
		if v, err := d2.Decode(); err == io.EOF {
			break
		} else if n, ok := v.(*osmpbf.Node); ok {
			if _, needed := referencedNodes[n.ID]; needed {
				nodeIndex[n.ID] = point{lat: float32(n.Lat), lon: float32(n.Lon)}
			}
		}
	}
	f2.Close()

	// 关键：释放不再需要的 map 并强制 GC
	referencedNodes = nil
	runtime.GC()
	d.printMemUsage("坐标索引建立后")
	log.Info("✅ 阶段 2 完成: 索引已缓存 %d 个核心坐标", len(nodeIndex))

	// --- 阶段 3: 提取 ---
	geoFile, err := os.Create("./static/china_poi.geojson")
	if err != nil {
		log.Error("无法创建文件: %v", err)
	}
	defer geoFile.Close()

	// 写入 GeoJSON 的头部
	geoFile.WriteString(`{"type":"FeatureCollection","features":[` + "\n")
	first := true // 用于处理逗号分隔

	log.Info("[阶段 3/3] 执行最终 POI 提取...")
	nodePOI, wayPOI, wikiPOI := 0, 0, 0

	logIndex := 0

	// 定义一个 map 用于分类统计数量
	type CategoryStats struct {
		Count   int
		PoiList []*POI
	}

	categoryStats := make(map[string]*CategoryStats)

	f3, _ := os.Open(pbfPath)
	d3 := osmpbf.NewDecoder(f3)
	d3.Start(runtime.GOMAXPROCS(0))

	for {
		v, err := d3.Decode()
		if err == io.EOF {
			break
		}

		var poi *POI
		switch v := v.(type) {
		case *osmpbf.Node:
			poi = d.processNode(v)
		case *osmpbf.Way:
			poi = d.processWay(v)
		}

		// 如果成功提取了 POI，进行统计
		if poi != nil {

			// 第一组：严格剔除名单 (无论是否有Wiki，都不建议保留)
			// 包含：物理障碍、无名建筑、纯路网连接点、微观交通设施
			var DirectDeleteMap = map[string]bool{
				"other":            true,
				"bus_stop":         true,
				"subway_entrance":  true,
				"main":             true,
				"lift_gate":        true,
				"stop_position":    true,
				"ticket_validator": true,
				"entrance":         true,
				"gate":             true,
				"track":            true,
				"wall":             true,
				"fence":            true,
				"scrub":            true,
				"building":         true,
				"industrial":       true, // 列表中出现了两次，统一处理
				"residential":      true,
				"commercial":       true,
				"tertiary":         true,
				"unclassified":     true,
				"footway":          true,
				"pedestrian":       true,
				"water_point":      true,
				"construction":     true,
				"ditch":            true,
				"embankment":       true,
				"pitch":            true,
				"parking_entrance": true,
				"car":              true,
				"driver_training":  true,
				"driving_school":   true,
			}

			// 第二组：潜力剔除名单 (尝试看有没有 Wiki，没有再剔除)
			// 包含：自然景观、历史遗迹、行政机构、公共设施
			var WikiCheckDeleteMap = map[string]bool{
				"motorway_junction": true, // 有些著名的高速枢纽在Wiki有记录
				"toll_booth":        true,
				"grave":             true,
				"cemetery":          true,
				"tomb":              true,
				"townhall":          true,
				"government":        true,
				"farmyard":          true,
				"school":            true,
				"garden":            true,
				"city_wall":         true,
				"citywalls":         true,
				"wood":              true,
				"residential":       true,
				"commercial":        true,
				"park":              true,
				"university":        true,
				"viaduct":           true,
				"rail":              true,
				"motorway":          true,
				"theatre":           true,
				"square":            true,
			}

			var HighValueKeywords = []string{
				"小镇", "古镇", "景区", "风景区", "名胜", "国家公园", "度假区",
				"遗址", "博物馆", "纪念馆", "寺", "庙", "祠", "塔", "书院", "故居", "古村", "古寨",
				"观景台", "观景点", "瀑布", "峡谷", "草原", "湿地", "溶洞", "地质公园", "森林公园",
				"广场", "温泉", "公园",
				"大学", "校区", "学院", // 新增校园类
				"大桥", "特大桥", "立交", "跨海大桥", "大路口",
				"大剧院", "剧院", "艺术中心", "文化中心", "体育馆", "体育场", "会展中心", "足球场", "会展中心",
			}
			var filterUselessKeywords = []string{
				// "塘", "垂钓", "菜地", "停车场",
			}

			ShouldKeep := func(poi *POI) bool {
				name := poi.Name
				kind := poi.Category.Kind

				// if strings.Contains(poi.Name, "九寨沟") ||
				// 	strings.Contains(poi.Name, "万灵古镇") ||
				// 	strings.Contains(poi.Name, "中国夏布小镇") ||
				// 	strings.Contains(poi.Name, "西南大学") {
				// 	j, _ := json.MarshalIndent(poi, "", "  ")
				// 	fmt.Println(string(j))
				// }

				if name == "" {
					return false
				}

				// 逻辑 B：关键词“捞人” (防止误杀万灵古镇、九寨沟等)
				for _, kw := range HighValueKeywords {
					if strings.Contains(name, kw) {
						return true // 名字里有“古镇”，管你分类是不是住宅区，统统留下
					}
				}
				for _, kw := range filterUselessKeywords {
					if strings.Contains(name, kw) {
						return false
					}
				}

				// 逻辑 D：条件剔除名单 (如没有 Wiki 的 wood, school 等)
				if WikiCheckDeleteMap[kind] {
					wikiID := poi.Tags["wikidata"]

					return wikiID != ""
				}

				// 逻辑 C：黑名单剔除 (剩下的普通住宅区、墙、厕所、人行道)
				if DirectDeleteMap[kind] {
					return false
				}

				// 默认保留 (如加油站、酒店等没在黑名单里的功能点)
				return true
			}

			if !ShouldKeep(poi) {
				continue
			}

			if poi.Type == "Node" {
				nodePOI++

			}
			if poi.Type == "Way" {
				wayPOI++

			}

			if poi.Tags["wikidata"] != "" || poi.Tags["wikipedia"] != "" {
				wikiPOI++
			}

			// if poi.Category.Kind == "wood" && logIndex < 20 {
			// 	logIndex++

			// 	j, _ := json.MarshalIndent(poi, "", "  ")
			// 	fmt.Println(string(j))
			// }

			// 累加该分类的数量
			if categoryStats[poi.Category.Kind] == nil {
				categoryStats[poi.Category.Kind] = new(CategoryStats)
			}

			categoryStats[poi.Category.Kind].Count++

			categoryStats[poi.Category.Kind].PoiList = append(
				categoryStats[poi.Category.Kind].PoiList, poi)

			gjLon, gjLat := poi.Lon, poi.Lat

			// --- 流式写入 GeoJSON ---
			if !first {
				geoFile.WriteString(",\n")
			}

			// 构造一个简单的 Feature 对象并序列化
			feature := fmt.Sprintf(`{"type":"Feature","geometry":{"type":"Point","coordinates":[%f,%f]},"properties":{"name":%q,"category":%q,"type":%q}}`,
				gjLon, gjLat, poi.Name, poi.Category.Kind, poi.Type)

			geoFile.WriteString(feature)
			first = false

			// 你原有的重庆测试逻辑（可选保留）
			if poi.Category.Kind == "other" {
				// log.Info("发现目标: [%s] %s (类型: %s)", poi.Category, poi.Name, poi.Type)
			}
			const (
				ChongqingMinLon = 105.17
				ChongqingMaxLon = 110.19
				ChongqingMinLat = 28.10
				ChongqingMaxLat = 32.20
			)
			// isInChongqing := poi.Lon >= ChongqingMinLon && poi.Lon <= ChongqingMaxLon &&
			// 	poi.Lat >= ChongqingMinLat && poi.Lat <= ChongqingMaxLat

			// if poi.Category.Kind == "yes" {
			// 	j, _ := json.MarshalIndent(poi, "", "  ")
			// 	fmt.Println(string(j))

			// 	// fmt.Println(fmt.Sprintf("发现目标: [%s] %s (类型: %s) Names %s", poi.Category, poi.Name, poi.Type, poi.Tags))

			// }

			// TODO: 写入 Qdrant 的逻辑建议放在这里
		}
	}
	f3.Close()

	// 打印每个分类的详细个数
	// for category, cs := range categoryStats {
	// 	// 调用之前定义的 Mapping 关系，可以让打印更易读

	// 	cnName := d.getCategoryChineseName(category)
	// 	if category == "attraction" {

	// 		fmt.Printf("[%-16s / %-10s]: %d 个\n", category, cnName, cs.Count)

	// 		for _, poi := range cs.PoiList {
	// 			if logIndex < 20 {
	// 				j, _ := json.MarshalIndent(poi, "", "  ")
	// 				fmt.Println(string(j))
	// 				logIndex++

	// 			}

	// 		}

	// 	}
	// }

	// 4. 写入尾部
	geoFile.WriteString("\n]}")
	log.Info("\n✨ 大功告成！共导出 %d 个有效 POI 到 china_trip_poi.geojson\n")

	log.Info("--------------------------------------")

	// 1. 定义最大并发限制信号量
	maxConcurrent := runtime.NumCPU() * 6

	log.Info("并发数量", maxConcurrent)
	semaphore := make(chan struct{}, maxConcurrent)

	// 2. 用于等待所有协程结束
	var wg sync.WaitGroup

	// count := 0
	for _, cs := range categoryStats {

		// 调用之前定义的 Mapping 关系，可以让打印更易读
		if cs.Count <= 10 {
			continue
		}

		for _, poi := range cs.PoiList {

			// ShouldKeep := func(poi *POI, HighValueKeywords []string) bool {
			// 	name := poi.Name

			// 	for _, kw := range HighValueKeywords {
			// 		if strings.Contains(name, kw) {
			// 			return true // 名字里有“古镇”，管你分类是不是住宅区，统统留下
			// 		}
			// 	}

			// 	return false
			// }
			// if poi.Category.Kind == "water" {

			// 	if ShouldKeep(poi, []string{
			// 		"垂钓",
			// 	}) {
			// 		count++
			// 		fmt.Println(poi.Name, poi.Lat, poi.Lon, count, cs.Count)

			// 	}
			// }

			if logIndex < 0 {
				// if logIndex < 1 {
				// if logIndex < 10000000 {
				logIndex++

				// 增加等待计数
				wg.Add(1)

				// 这种写法确保了 logIndex 的值在进入协程时是正确的副本
				currentIndex := logIndex
				currentPoi := poi

				go func(idx int, p *POI) { // 请根据你实际的 poi 类型替换 POI_TYPE
					defer wg.Done()

					// 获取信号量：如果通道满了，这里会阻塞，直到有其他协程退出
					semaphore <- struct{}{}
					defer func() { <-semaphore }() // 任务结束释放信号量

					err := func() error {
						t := log.Time()
						// // 1. 创建一个带 10 秒超时的 ctx (针对单次写入)
						ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
						defer cancel() // 此时 defer 会在这个匿名函数结束时立即执行

						// j, _ := json.MarshalIndent(poi, "", "  ")
						// fmt.Println(string(j))

						idStr := fmt.Sprintf("%s:%d", poi.Type, poi.ID)
						pointId := aiDbx.GeneratePointID(idStr)

						// fmt.Println("pointId", idStr, pointId)

						// if pointId != "0000a406-b46e-5692-80a0-a4c68db759c9" {
						// 	return nil
						// }

						point, err := d.GetPointById(ctx, pointId)
						if err != nil {
							log.Error(err)
							return err
						}

						// 已经存在，无需在存
						if point != nil {

							log.Error("已存在 ->", idx, idStr)

							// // 已经存在，无需在存
							// oldPayload := point.Payload

							// // 1. 提取 wiki 结构并修改 status 为 0
							// if wikiValue, ok := oldPayload["wiki"]; ok {
							// 	// 确保它确实是个结构体
							// 	if wikiStruct := wikiValue.GetStructValue(); wikiStruct != nil {

							// 		// 2. 原地修改：只动 status，不动 fields 里的其他东西
							// 		// 这样 wikiStruct.Fields 里的其他 key (比如 url, text 等) 都会保留
							// 		wikiStruct.Fields["status"] = qdrant.NewValueInt(0)

							// 		// 3. 构造更新请求
							// 		updatePayload := map[string]*qdrant.Value{
							// 			"wiki": wikiValue, // 这里的 wikiValue 内部包含了完整的 Fields
							// 		}

							// 		// 4. 提交更新
							// 		err = d.UpdatePayloadById(ctx, pointId, updatePayload)
							// 		if err != nil {
							// 			log.Error(err)
							// 			return err
							// 		}

							// 		t.TimeEnd("更新成功 -> " + nstrings.ToString(idx) + "；" + idStr + "；" + pointId)
							// 	}
							// }

							return nil
						}
						vector, err := aiDbx.GetEmbedding(TextTypeDocument,
							poi.Name+" "+poi.Category.Namespace+" "+poi.Category.Kind)
						if err != nil {
							log.Error(err)
							return err
						}
						// log.Info("vector", logIndex, len(vector), vector[0])

						payload := d.ToQdrantPayload(poi)
						// j1, _ := json.MarshalIndent(payload, "", "  ")
						// fmt.Println("payload", string(j1))

						if err := d.SaveQdrant(ctx, pointId, vector, payload); err != nil {
							log.Error(err)
							return err
						}
						// log.Info("存储成功 ->", idx, idStr, pointId, point != nil)
						t.TimeEnd("存储成功 -> " + nstrings.ToString(idx) + "；" + idStr + "；" + pointId)

						return nil
					}()

					if err != nil {
						log.Error("Error -> "+nstrings.ToString(idx), err)

						// return
					}
				}(currentIndex, currentPoi)
			}

		}

	}

	// 3. 阻塞等待所有协程处理完毕
	wg.Wait()

	log.Info("✅ 阶段 3 完成！数据统计如下：")
	log.Info("总计节点 (Node): ", nodePOI)
	log.Info("总计路径 (Way): ", wayPOI)
	log.Info("合计 (Node + Way): ", nodePOI+wayPOI, logIndex)
	log.Info("有维基百科的POI数量: ", wikiPOI)

	log.Info("--------------------------------------")

	log.Info("--- 详细分类统计 ---")
	log.Info("所有数据处理完成")

	for category, cs := range categoryStats {

		// 调用之前定义的 Mapping 关系，可以让打印更易读

		cnName := d.getCategoryChineseName(category)

		if cs.Count <= 10 {
			continue
		}
		fmt.Printf("[%-16s / %-10s]: %d 个\n", category, cnName, cs.Count)

	}
	for category, cs := range categoryStats {

		cnName := d.getCategoryChineseName(category)

		if category == "water" {

			fmt.Printf("[%-16s / %-10s]: %d 个\n", category, cnName, cs.Count)

		}
	}
	log.Info("--------------------------------------")
	// 清理并完成
	nodeIndex = nil
	runtime.GC()
	log.Info("------ InitTripPOI 处理全部完成 ------")

	return nil
}

func (d *POIDbx) processNode(n *osmpbf.Node) *POI {
	name, ok := n.Tags["name"]
	if !ok || name == "" {
		return nil
	}
	return &POI{
		ID: n.ID, Name: name, Type: "Node",
		Category: d.getPrimaryTag(n.Tags),
		Lat:      n.Lat, Lon: n.Lon, Tags: n.Tags,
	}
}

func (d *POIDbx) processWay(w *osmpbf.Way) *POI {
	name, ok := w.Tags["name"]
	if !ok || name == "" {
		return nil
	}

	var latSum, lonSum float64
	count := 0
	for _, id := range w.NodeIDs {
		if p, ok := nodeIndex[id]; ok {
			latSum += float64(p.lat)
			lonSum += float64(p.lon)
			count++
		}
	}
	if count == 0 {
		return nil
	}

	cate := d.getPrimaryTag(w.Tags)

	if cate.Namespace == "other" && d.IsUsefulPOI(name, cate.Namespace) {
		return nil
	}

	return &POI{
		ID: w.ID, Name: name, Type: "Way",
		Category: cate,
		Lat:      latSum / float64(count),
		Lon:      lonSum / float64(count),
		Tags:     w.Tags,
	}
}

func (d *POIDbx) getPrimaryTag(tags map[string]string) *POICategory {
	coreKeys := []string{
		// 1. 景观与灵魂 (权重最高)
		"tourism", "natural", "historic", "waterway", "mountain_pass",

		// 2. 核心补给与功能
		"amenity", "highway", "barrier", "aeroway", "railway",

		// 3. 休闲与场所
		"leisure", "place", "shop",

		// 4. 建筑与土地用途 (这是你刚才那堆 other 的重灾区)
		"building", "landuse", "man_made", "office",

		// 5. 补充
		"entrance", "military", "craft",

		"public_transport",
		"religion",
	}
	for _, k := range coreKeys {
		if v, ok := tags[k]; ok {
			// return v
			// return fmt.Sprintf("%s:%s", k, v)
			if v == "yes" {
				continue
			}
			if v == "no" {
				continue
			}
			return &POICategory{
				Namespace: k,
				Kind:      v,
			}
		}
	}

	// for _, k := range []string{"amenity", "tourism", "natural", "highway", "shop"} {
	// 	if v, ok := tags[k]; ok {
	// 		return v
	// 	}
	// }
	return &POICategory{
		Namespace: "other",
		Kind:      "other",
	}
}

// 辅助函数：打印内存占用
func (d *POIDbx) printMemUsage(tag string) {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	log.Info("[%s] 内存占用: Alloc = %v MiB, TotalAlloc = %v MiB, Sys = %v MiB",
		tag, m.Alloc/1024/1024, m.TotalAlloc/1024/1024, m.Sys/1024/1024)
}

// 为了配合打印，建议补充这个简单的辅助映射函数
func (d *POIDbx) getCategoryChineseName(cat string) string {
	mapping := map[string]string{
		// --- [1. 自然景观：山川、地貌、植被] ---
		"peak":          "山峰/雪山",
		"saddle":        "山坳/垭口",
		"ridge":         "山脊",
		"valley":        "峡谷/山谷",
		"cliff":         "悬崖/断崖",
		"volcano":       "火山",
		"glacier":       "冰川",
		"bare_rock":     "裸岩/奇石",
		"cave_entrance": "溶洞/洞穴",
		"island":        "岛屿",
		"islet":         "小岛/礁石",
		"cape":          "海角/海岬",
		"wetland":       "湿地/沼泽",
		"grassland":     "草原/草地",
		"meadow":        "草甸/牧场",
		"wood":          "森林/树林",
		"forest":        "林区",
		"scrub":         "灌木丛",
		"grass":         "草坪/绿地",
		"sand":          "沙地/沙漠",
		"locality":      "地名点/自然村",

		// --- [2. 水利水文：湖泊、补给、水系] ---
		"water":      "湖泊/水库",
		"waterfall":  "瀑布",
		"spring":     "自然泉水",
		"hot_spring": "天然温泉",
		"beach":      "沙滩/海岸",
		"coastline":  "海岸线",
		"reservoir":  "水库/蓄水池",
		"dam":        "水坝",
		"lock_gate":  "船闸",
		"ditch":      "渠道/沟渠",
		"embankment": "堤坝/路堤",

		// --- [3. 旅游人文：打卡、古迹、设施] ---
		"attraction":          "特色景点",
		"viewpoint":           "观景点",
		"museum":              "博物馆",
		"zoo":                 "动物园",
		"theme_park":          "主题乐园",
		"artwork":             "艺术装置/雕塑",
		"park":                "公园/绿地",
		"garden":              "花园/园林",
		"picnic_site":         "野餐点",
		"information":         "游客中心/导览",
		"historic":            "历史遗迹",
		"heritage":            "文化遗产",
		"archaeological_site": "考古遗址",
		"ruins":               "废墟/遗址",
		"castle":              "古堡/遗迹",
		"fort":                "炮台/要塞",
		"monument":            "纪念性建筑",
		"memorial":            "纪念地/碑",
		"tomb":                "古墓/陵墓",
		"cemetery":            "公墓",
		"citywalls":           "古城墙",
		"city_wall":           "古城墙/城垣",
		"city_gate":           "古城门",
		"protected_building":  "保护建筑",
		"wayside_shrine":      "路边神龛",
		"place_of_worship":    "宗教场所",
		"temple":              "寺庙/道观",
		"religious":           "宗教设施",
		"tower":               "塔/高耸建筑",

		// --- [4. 自驾补给：能源、休息、安全] ---
		"fuel":             "加油站",
		"charging_station": "充电站",
		"services":         "高速服务区",
		"rest_area":        "公路休息区",
		"parking":          "停车场",
		"parking_entrance": "停车场入口",
		"drinking_water":   "饮用水点",
		"water_point":      "补给加水点",
		"public_bath":      "温泉/公共浴室",
		"shelter":          "避雨亭/凉亭",
		"police":           "警务/检查站",
		"border_control":   "边境检查站",

		// --- [5. 住宿营地：旅途落脚点] ---
		"camp_site":    "露营地",
		"caravan_site": "房车营地",
		"chalet":       "度假木屋",
		"hotel":        "酒店宾馆",
		"farmyard":     "农家院/农庄",

		// --- [6. 交通枢纽：水陆联运] ---
		"ferry_terminal":  "轮渡码头",
		"marina":          "游船码头",
		"bus_stop":        "公交站点",
		"subway_entrance": "地铁出入口",
		"funicular":       "缆车/索道站",

		// --- [7. 生活与功能：城镇、路径] ---
		"townhall":      "行政中心/村委会",
		"village":       "村庄",
		"hamlet":        "小村落",
		"residential":   "住宅区",
		"commercial":    "商业区",
		"industrial":    "工业区",
		"government":    "政府机构",
		"construction":  "施工区域",
		"school":        "学校",
		"square":        "广场",
		"track":         "机耕道/土路",
		"path":          "徒步小径",
		"footway":       "人行步道",
		"pedestrian":    "步行区",
		"tertiary":      "三级公路/乡道",
		"unclassified":  "未分级道路",
		"stop_position": "停车位置",
		"entrance":      "入口",
		"building":      "建筑实体",
		"fence":         "围栏/栅栏",
		"wall":          "墙体",
		"service":       "维护/后勤设施",
		"pitch":         "运动场",
		"fishing":       "垂钓点",

		// --- [8. 兜底项] ---
		"main":  "主入口/主建筑",
		"yes":   "通用地标",
		"other": "未归类点位",
	}

	if name, ok := mapping[cat]; ok {
		return name
	}
	return "其他"
}

// POIFilter 负责判断一个 POI 是否具有自驾路书价值
func (d *POIDbx) IsUsefulPOI(name string, category string) bool {
	// 如果不是 other 类型，默认保留（因为前面的 args 已经过滤过一遍）
	if category != "other" {
		return true
	}

	name = strings.ToLower(name)

	// 1. 彻底剔除：校园内部设施 & 城市细碎设施
	// 这些东西自驾车进不去，或者即便路过也完全没必要播报
	trashKeywords := []string{
		"校史", "楼", "宿舍", "教学", "实验", "公寓", "食堂", "课室",
		"摇篮", "校门", "雕塑", "走廊", "喷泉", "内部", "科室", "诊室",
		"人行", "斑马线", "垃圾站", "电线杆", "变压器", "宣传栏",
	}

	for _, kw := range trashKeywords {
		if strings.Contains(name, kw) {
			return false
		}
	}

	// 2. 深度剔除：一些无意义的纯数字或符号命名
	// 比如 "1号桩", "Node 123" 等
	if len(name) < 4 { // 名字太短的通常是噪音，除非是地名（地名在 place 逻辑里处理）
		// 这里可以根据实际情况微调，比如 2 个字的名字
	}

	// 3. 强制打捞：即便在 other 里，只要包含这些词，就是核心人文资产
	// 这些是自驾路书的“灵魂”
	treasureKeywords := []string{
		"墓", "碑", "塔", "遗址", "坟", "纪念", "古", "庙",
		"观沧海", "龙回头", "标志", "地理", "零公里",
	}

	for _, kw := range treasureKeywords {
		if strings.Contains(name, kw) {
			return true
		}
	}

	// 4. 默认策略：
	// 如果名字包含明显的地理位置（比如：接庄、邹城南），保留
	// 这部分通常由你之前的 place=village/town 逻辑覆盖
	return true
}

func (d *POIDbx) ToQdrantPayload(p *POI) map[string]*qdrant.Value {
	// 1. 初始化 Wiki 状态
	wikiID := p.Tags["wikidata"]
	wikiTitle := p.Tags["wikipedia"]
	wikiStatus := int64(0)
	// if wikiID != "" || wikiTitle != "" {
	// 	wikiStatus = 0
	// }

	// 2. 过滤并构建 osm.tags 的 MapValue
	cleanTagsMap := make(map[string]*qdrant.Value)
	for k, v := range p.Tags {
		if k != "wikidata" && k != "wikipedia" && k != "name" {
			cleanTagsMap[k] = qdrant.NewValueString(v)
		}
	}

	importance := int64(d.GetImportanceByKind(p.Category.Kind))

	// 3. 构建最终的 Payload Map
	return map[string]*qdrant.Value{
		"name":       qdrant.NewValueString(p.Name),
		"importance": qdrant.NewValueInt(importance), // 存入权重
		"address": qdrant.NewValueStruct(&qdrant.Struct{
			Fields: map[string]*qdrant.Value{
				"country":    qdrant.NewValueString(""), // 国家
				"state":      qdrant.NewValueString(""), // 省/州
				"region":     qdrant.NewValueString(""), // 地级市/地区
				"city":       qdrant.NewValueString(""), // 区/县/县级市
				"town":       qdrant.NewValueString(""), // 乡镇/街道
				"updated_at": qdrant.NewValueInt(0),
			},
		}),

		// 嵌套位置信息 (用于 Geo 索引)
		"location": qdrant.NewValueStruct(&qdrant.Struct{
			Fields: map[string]*qdrant.Value{
				"lat": qdrant.NewValueDouble(p.Lat),
				"lon": qdrant.NewValueDouble(p.Lon),
			},
		}),

		// 嵌套分类信息
		"category": qdrant.NewValueStruct(&qdrant.Struct{
			Fields: map[string]*qdrant.Value{
				"namespace": qdrant.NewValueString(p.Category.Namespace),
				"kind":      qdrant.NewValueString(p.Category.Kind),
			},
		}),

		// 嵌套 Wiki 增强信息
		"wiki": qdrant.NewValueStruct(&qdrant.Struct{
			Fields: map[string]*qdrant.Value{
				"status":  qdrant.NewValueInt(wikiStatus),
				"id":      qdrant.NewValueString(wikiID),
				"summary": qdrant.NewValueString(""),
				"title":   qdrant.NewValueString(wikiTitle),
				"cover": qdrant.NewValueStruct(&qdrant.Struct{
					Fields: map[string]*qdrant.Value{
						"url":    qdrant.NewValueString(""),
						"width":  qdrant.NewValueInt(0),
						"height": qdrant.NewValueInt(0),
					},
				}),
				"wiki_url":   qdrant.NewValueString(""),
				"updated_at": qdrant.NewValueInt(0),
			},
		}),

		// 嵌套 OSM 原始信息
		"osm": qdrant.NewValueStruct(&qdrant.Struct{
			Fields: map[string]*qdrant.Value{
				"id":   qdrant.NewValueInt(p.ID),
				"type": qdrant.NewValueString(p.Type),
				"tags": qdrant.NewValueStruct(&qdrant.Struct{
					Fields: cleanTagsMap,
				}),
			},
		}),
		"created_at": qdrant.NewValueInt(time.Now().Unix()),
	}
}

func (d *POIDbx) GetPointById(ctx context.Context, pointId string) (*qdrant.RetrievedPoint, error) {
	// 1. 调用 Get 接口获取数据
	res, err := conf.Qdrant.PointsClient.Get(ctx, &qdrant.GetPoints{
		CollectionName: conf.QdrantCollectionName.POI,
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

type POIPayload struct {
	// qdrantid uuid
	Id         string       `json:"id"`
	Name       string       `json:"name"`
	Importance int64        `json:"importance"`
	CreatedAt  int64        `json:"created_at"`
	Address    AddressInfo  `json:"address"`
	Location   LocationInfo `json:"location"`
	Category   CategoryInfo `json:"category"`
	Wiki       WikiInfo     `json:"wiki"`
	OSM        OSMInfo      `json:"osm"`

	// 搜索点位距离POI的距离，非距离搜索则为-1
	Distance  float64 `json:"distance"`
	Bearing   float64 `json:"bearing"`
	Direction string  `json:"direction"`
}

type AddressInfo struct {
	Country   string `json:"country"`
	State     string `json:"state"`
	Region    string `json:"region"`
	City      string `json:"city"`
	Town      string `json:"town"`
	UpdatedAt int64  `json:"updated_at"`
}

type LocationInfo struct {
	Lat float64 `json:"lat"`
	Lon float64 `json:"lon"`
}

type CategoryInfo struct {
	Namespace string `json:"namespace"`
	Kind      string `json:"kind"`
}

type WikiCover struct {
	URL    string `json:"url"`
	Width  int64  `json:"width"`
	Height int64  `json:"height"`
}

type WikiInfo struct {
	Status    int64      `json:"status"`
	ID        string     `json:"id"`
	Title     string     `json:"title"`
	Summary   string     `json:"summary"`
	WikiUrl   string     `json:"wiki_url"`
	Cover     *WikiCover `json:"cover"`
	UpdatedAt int64      `json:"updated_at"`
}

type OSMInfo struct {
	ID   int64                  `json:"id"`
	Type string                 `json:"type"`
	Tags map[string]interface{} `json:"tags"`
}

func (d *POIDbx) getValueInterface(v *qdrant.Value) interface{} {
	if v == nil {
		return nil
	}

	// 处理各种可能的类型
	switch x := v.Kind.(type) {
	case *qdrant.Value_StringValue:
		return x.StringValue
	case *qdrant.Value_IntegerValue:
		return x.IntegerValue
	case *qdrant.Value_DoubleValue:
		return x.DoubleValue
	case *qdrant.Value_BoolValue:
		return x.BoolValue
	case *qdrant.Value_StructValue:
		// 递归处理嵌套结构体
		res := make(map[string]interface{})
		for k, val := range x.StructValue.Fields {
			res[k] = d.getValueInterface(val)
		}
		return res
	case *qdrant.Value_ListValue:
		// 处理列表
		var res []interface{}
		for _, val := range x.ListValue.Values {
			res = append(res, d.getValueInterface(val))
		}
		return res
	case *qdrant.Value_NullValue:
		return nil
	default:
		return nil
	}
}
func (d *POIDbx) QdrantMapPayloadToStruct(id *qdrant.PointId, payload map[string]*qdrant.Value,
	lat *float64, lon *float64, heading *float64,
) (*POIPayload, error) {
	tempMap := make(map[string]interface{})

	for k, v := range payload {
		// qdrant.Value 内部是一个 OneOf 结构
		// 我们需要根据它的 Kind 来获取实际的 interface{}
		tempMap[k] = d.getValueInterface(v)
	}

	// 2. 利用 JSON 序列化和反序列化完成结构体映射
	// 这是处理嵌套 structpb 最简单且错误率最低的方法
	jsonData, err := json.Marshal(tempMap)
	if err != nil {
		return nil, fmt.Errorf("marshal payload failed: %w", err)
	}

	var poi POIPayload
	err = json.Unmarshal(jsonData, &poi)
	if err != nil {
		return nil, fmt.Errorf("unmarshal to struct failed: %w", err)
	}

	poi.Id = id.GetUuid()

	if err := poi.GetAddress(); err != nil {
		log.Error(err)
		// return nil, err
	}
	if err := poi.GetWikiSummary(); err != nil {
		log.Error(err)
		// return nil, err
	}

	poi.GetPOIRelativePosition(lat, lon, heading)

	return &poi, nil
}

func (poi *POIPayload) GetAddress() error {

	// log.Info("GetAddress", poi.Address.UpdatedAt, time.Now().Unix()-180*24*3600)
	if poi.Address.UpdatedAt > time.Now().Unix()-180*24*3600 &&
		poi.Address.Country != "" && poi.Address.City != "" {
		return nil
	}

	// 构造请求 URL
	apiURL := fmt.Sprintf(
		conf.Config.ToolsApiUrl+
			"/api/v1/geocode/regeo?latitude=%f&longitude=%f&platform=Amap",
		poi.Location.Lat, poi.Location.Lon)

	// log.Info("GetAddress", apiURL)
	// 创建带有超时的客户端（自驾数据处理建议 10s 超时）
	client := &http.Client{
		Timeout: 2 * time.Second,
	}

	resp, err := client.Get(apiURL)
	if err != nil {
		return fmt.Errorf("network error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GetAddress api error: status code %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	// log.Info("GetAddress body", string(body))

	type ReGeoResponse struct {
		Code int `json:"code"`
		Data *struct {
			// 基础逆地理字段
			FormattedAddress string `json:"address"` // 完整格式化地址
			Country          string `json:"country"` // 国家
			State            string `json:"state"`   // 省份/直辖市
			Region           string `json:"region"`  // 地级市/自治州
			City             string `json:"city"`    // 县/区
			Town             string `json:"town"`    // 乡镇/街道
			Road             string `json:"road"`    // 道路名称
			Cache            bool   `json:"cache"`   // 道路名称

			// 坐标回显
			LatLng *struct {
				Lat float64 `json:"lat"`
				Lng float64 `json:"lng"`
			} `json:"latlng"`
		} `json:"data"`
	}
	var result ReGeoResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return fmt.Errorf("json decode error: %v", err)
	}

	if result.Data == nil || result.Data.Country == "" || result.Data.FormattedAddress == "" {
		return fmt.Errorf("Failed to get address", apiURL)
	}
	poi.Address.Country = result.Data.Country
	poi.Address.State = result.Data.State
	poi.Address.Region = result.Data.Region
	poi.Address.City = result.Data.City
	poi.Address.Town = result.Data.Town
	poi.Address.UpdatedAt = time.Now().Unix()
	log.Info("GetAddress Cache", poi.Id, result.Data.Cache)

	// 4. 提交更新

	ctx, cancel := context.WithTimeout(context.Background(),
		10*time.Second)
	defer cancel()

	updatePayload := map[string]*qdrant.Value{
		"address": qdrant.NewValueStruct(&qdrant.Struct{
			Fields: map[string]*qdrant.Value{
				"country":    qdrant.NewValueString(poi.Address.Country),
				"state":      qdrant.NewValueString(poi.Address.State),
				"region":     qdrant.NewValueString(poi.Address.Region),
				"city":       qdrant.NewValueString(poi.Address.City),
				"town":       qdrant.NewValueString(poi.Address.Town),
				"updated_at": qdrant.NewValueInt(poi.Address.UpdatedAt),
			},
		}),
	}

	err = poiDbx.UpdatePayloadById(ctx, poi.Id, updatePayload)
	if err != nil {
		log.Error(err)
		return err
	}

	return nil
}

func (poi *POIPayload) GetWikiSummary() error {

	// testName := "九寨沟"
	// testName = ""

	name := nstrings.StringOr(poi.Wiki.Title, poi.Name)

	// log.Info("GetWikiSummary", poi.Id, name, poi.Name,

	// 	poi.Wiki.Title,
	// 	poi.Wiki.UpdatedAt, poi.Wiki.Status)

	// log.Info("GetWikiSummary",
	// 	poi.Name,
	// 	poi.Wiki.UpdatedAt > time.Now().Unix()-180*24*3600,
	// 	poi.Wiki.Status != 0)

	if poi.Wiki.UpdatedAt > time.Now().Unix()-180*24*3600 &&
		poi.Wiki.Status != 0 {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(),
		15*time.Second)
	defer cancel()

	wikiPage, err := methods.GetWikiSummary(ctx, name)
	if err != nil {
		return err
	}

	wikiStatus := -1

	if wikiPage != nil {
		// 1、判断距离，距离在5公里以内的，直接true
		if wikiStatus == -1 && len(wikiPage.Coordinates) > 0 {
			lat, lon := float64(0), float64(0)

			for _, coord := range wikiPage.Coordinates {
				lat, lon = coord.Lat, coord.Lon
			}

			distance := methods.GetGeoDistance(lat, lon, poi.Location.Lat, poi.Location.Lon)

			if distance < 5000 {
				wikiStatus = 1
			}
			log.Info("distance", wikiStatus, distance)
		}
		// 2、判断摘要有没有区县名字，有则为true

		if wikiStatus == -1 && wikiPage.Extract != "" {
			keywords := []string{
				poi.Address.Region,
				poi.Address.City,
			}

			for _, kw := range keywords {
				if kw != "" && strings.Contains(wikiPage.Extract, kw) {
					wikiStatus = 1
					log.Info("strings.Contains", kw, strings.Contains(wikiPage.Extract, kw))
					break
				}
			}

		}

		if wikiPage.Extract == "" {
			wikiStatus = -1
		}

	}

	// 其他条件都放弃？

	log.Info("wikiStatus", wikiStatus)
	log.Info("result.Query.Pages", wikiPage)
	log.Info("poi", poi.Name)

	// return fmt.Errorf("Failed to get address")

	// if result.Query.Pages == nil || result.Data.Country == "" || result.Data.City == "" {
	// 	return fmt.Errorf("Failed to get address")
	// }

	poi.Wiki.Status = int64(wikiStatus)
	poi.Address.UpdatedAt = time.Now().Unix()
	if wikiStatus == 1 {
		poi.Wiki.Cover = &WikiCover{
			URL:    wikiPage.Thumbnail.Source,
			Width:  wikiPage.Thumbnail.Width,
			Height: wikiPage.Thumbnail.Height,
		}
		poi.Wiki.ID = wikiPage.PageProps.WikiBaseItem
		poi.Wiki.Summary = wikiPage.Extract
		poi.Wiki.Title = wikiPage.Title
		poi.Wiki.WikiUrl = wikiPage.FullURL
	} else {
		poi.Wiki.Cover = &WikiCover{
			URL:    "",
			Width:  0,
			Height: 0,
		}
		poi.Wiki.ID = ""
		poi.Wiki.Summary = ""
		poi.Wiki.Title = ""
		poi.Wiki.WikiUrl = ""
	}

	// 4. 提交更新

	updatePayload := map[string]*qdrant.Value{
		"wiki": qdrant.NewValueStruct(&qdrant.Struct{
			Fields: map[string]*qdrant.Value{
				"status":  qdrant.NewValueInt(poi.Wiki.Status),
				"id":      qdrant.NewValueString(poi.Wiki.ID),
				"summary": qdrant.NewValueString(poi.Wiki.Summary),
				"title":   qdrant.NewValueString(poi.Wiki.Title),
				"cover": qdrant.NewValueStruct(&qdrant.Struct{
					Fields: map[string]*qdrant.Value{
						"url":    qdrant.NewValueString(poi.Wiki.Cover.URL),
						"width":  qdrant.NewValueInt(poi.Wiki.Cover.Width),
						"height": qdrant.NewValueInt(poi.Wiki.Cover.Height),
					},
				}),
				"wiki_url":   qdrant.NewValueString(poi.Wiki.WikiUrl),
				"updated_at": qdrant.NewValueInt(poi.Address.UpdatedAt),
			},
		}),
	}

	err = poiDbx.UpdatePayloadById(ctx, poi.Id, updatePayload)
	if err != nil {
		log.Error(err)
		return err
	}

	return nil
}

func (poi *POIPayload) GetPOIRelativePosition(lat1, lon1, heading *float64) {
	// 安全检查：如果位置坐标缺失，无法进行任何几何计算，直接返回
	if lat1 == nil || lon1 == nil {
		return
	}

	const R = 6371000.0 // 地球半径 (米)
	lat2, lon2 := poi.Location.Lat, poi.Location.Lon

	// 1. 距离计算 (Haversine 公式)
	p1 := *lat1 * math.Pi / 180
	p2 := lat2 * math.Pi / 180
	dPhi := (lat2 - *lat1) * math.Pi / 180
	dLambda := (lon2 - *lon1) * math.Pi / 180

	a := math.Sin(dPhi/2)*math.Sin(dPhi/2) +
		math.Cos(p1)*math.Cos(p2)*
			math.Sin(dLambda/2)*math.Sin(dLambda/2)
	c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))

	poi.Distance = R * c

	// 2. 方向计算逻辑：只有当 heading 不为 nil 时才执行
	if heading != nil {
		// 计算绝对方位角 (Absolute Bearing)
		y := math.Sin(dLambda) * math.Cos(p2)
		x := math.Cos(p1)*math.Sin(p2) -
			math.Sin(p1)*math.Cos(p2)*math.Cos(dLambda)

		absBearing := math.Mod(math.Atan2(y, x)*180/math.Pi+360, 360)

		// 计算相对角度 (Relative Bearing)
		relAngle := absBearing - *heading

		// 标准化到 [-180, 180]
		if relAngle > 180 {
			relAngle -= 360
		} else if relAngle < -180 {
			relAngle += 360
		}

		// 映射文案
		poi.Bearing = math.Round(relAngle)
		poi.Direction = getDirectionText(relAngle)
	}
}

// getDirectionText 将角度映射为专业领航文案
func getDirectionText(angle float64) string {
	// 采用 45 度分区逻辑
	switch {
	case angle >= -15 && angle <= 15:
		return "正前方"
	case angle > 15 && angle <= 60:
		return "右前方"
	case angle > 60 && angle <= 120:
		return "右方"
	case angle > 120 && angle <= 165:
		return "右后方"
	case angle > 165 || angle <= -165:
		return "正后方"
	case angle > -165 && angle <= -120:
		return "左后方"
	case angle > -120 && angle <= -60:
		return "左方"
	case angle > -60 && angle < -15:
		return "左前方"
	default:
		return "位置识别中"
	}
}

// POISearchParams 定义了 Agent 可以使用的所有聚合查询维度
type POISearchParams struct {
	// 地理位置
	Lat     *float64 `json:"lat"`     // 纬度
	Lon     *float64 `json:"lon"`     // 经度
	Heading *float64 `json:"heading"` // 经度
	Radius  float32  `json:"radius"`  // 半径（米）

	// 文本搜索
	Name string `json:"name"` // POI 名称（模糊匹配）

	// 精准过滤（Keyword 类型）
	Country string `json:"country"` // 行政区
	State   string `json:"state"`   // 行政区
	Region  string `json:"region"`  // 行政区
	City    string `json:"city"`    // 城市
	Town    string `json:"town"`    // 城市

	Categories []string `json:"categories"` // 分类 (category.kind)

	// 数值范围
	ImportanceMin *float64 `json:"importance_min"` // 最小重要度
	WikiStatus    *int64   `json:"wiki_status"`    // Wiki 状态

	// 控制参数
	Limit uint32 `json:"limit"` // 返回数量上限
}

type SearchPOIResult struct {
	Point *qdrant.RetrievedPoint
	POI   *POIPayload
}

// 要支持分页查询、要支持重要性排序
func (d *POIDbx) SearchPOI(ctx context.Context, params POISearchParams) ([]*SearchPOIResult, error) {
	var mustConditions []*qdrant.Condition

	// 1. 地理位置过滤
	if params.Lat != nil && params.Lon != nil {
		radius := float32(1000)
		if params.Radius != 0 {
			radius = params.Radius
		}

		mustConditions = append(mustConditions, &qdrant.Condition{
			ConditionOneOf: &qdrant.Condition_Field{
				Field: &qdrant.FieldCondition{
					Key: "location",
					GeoRadius: &qdrant.GeoRadius{
						Center: &qdrant.GeoPoint{Lat: *params.Lat, Lon: *params.Lon},
						Radius: radius,
					},
				},
			},
		})
	}

	// 2. 名称模糊匹配 (Match Text)
	if params.Name != "" {
		mustConditions = append(mustConditions, &qdrant.Condition{
			ConditionOneOf: &qdrant.Condition_Field{
				Field: &qdrant.FieldCondition{
					Key: "name",
					Match: &qdrant.Match{
						MatchValue: &qdrant.Match_Text{
							Text: params.Name,
						},
					},
				},
			},
		})
	}
	if params.Country != "" {
		mustConditions = append(mustConditions, &qdrant.Condition{
			ConditionOneOf: &qdrant.Condition_Field{
				Field: &qdrant.FieldCondition{
					Key: "address.country",
					Match: &qdrant.Match{
						MatchValue: &qdrant.Match_Keyword{
							Keyword: params.Country,
						},
					},
				},
			},
		})
	}
	if params.State != "" {
		mustConditions = append(mustConditions, &qdrant.Condition{
			ConditionOneOf: &qdrant.Condition_Field{
				Field: &qdrant.FieldCondition{
					Key: "address.state",
					Match: &qdrant.Match{
						MatchValue: &qdrant.Match_Keyword{
							Keyword: params.State,
						},
					},
				},
			},
		})
	}
	if params.Region != "" {
		mustConditions = append(mustConditions, &qdrant.Condition{
			ConditionOneOf: &qdrant.Condition_Field{
				Field: &qdrant.FieldCondition{
					Key: "address.region",
					Match: &qdrant.Match{
						MatchValue: &qdrant.Match_Keyword{
							Keyword: params.Region,
						},
					},
				},
			},
		})
	}

	// 3. 城市/区/分类精准匹配 (Match Keyword)
	if params.City != "" {
		mustConditions = append(mustConditions, &qdrant.Condition{
			ConditionOneOf: &qdrant.Condition_Field{
				Field: &qdrant.FieldCondition{
					Key: "address.city",
					Match: &qdrant.Match{
						MatchValue: &qdrant.Match_Keyword{
							Keyword: params.City,
						},
					},
				},
			},
		})
	}
	if params.Town != "" {
		mustConditions = append(mustConditions, &qdrant.Condition{
			ConditionOneOf: &qdrant.Condition_Field{
				Field: &qdrant.FieldCondition{
					Key: "address.town",
					Match: &qdrant.Match{
						MatchValue: &qdrant.Match_Keyword{
							Keyword: params.Town,
						},
					},
				},
			},
		})
	}
	if len(params.Categories) > 0 {
		mustConditions = append(mustConditions, &qdrant.Condition{
			ConditionOneOf: &qdrant.Condition_Field{
				Field: &qdrant.FieldCondition{
					Key: "category.kind",
					Match: &qdrant.Match{
						// 核心修正点：使用 Match_Keywords 而不是 Match_Any
						MatchValue: &qdrant.Match_Keywords{
							Keywords: &qdrant.RepeatedStrings{
								Strings: params.Categories, // 传入你的 kind 数组，如 ["peak", "water"]
							},
						},
					},
				},
			},
		})
	}

	// 4. 数值范围 (Range)
	if params.ImportanceMin != nil {
		mustConditions = append(mustConditions, &qdrant.Condition{
			ConditionOneOf: &qdrant.Condition_Field{
				Field: &qdrant.FieldCondition{
					Key: "importance",
					Range: &qdrant.Range{
						Gte: params.ImportanceMin,
					},
				},
			},
		})
	}

	// 5. 状态精准匹配 (Match Integer)
	if params.WikiStatus != nil {
		mustConditions = append(mustConditions, &qdrant.Condition{
			ConditionOneOf: &qdrant.Condition_Field{
				Field: &qdrant.FieldCondition{
					Key: "wiki.status",
					Match: &qdrant.Match{
						MatchValue: &qdrant.Match_Integer{
							Integer: *params.WikiStatus,
						},
					},
				},
			},
		})
	}

	// 6. 执行 Scroll 查询
	limit := uint32(20)
	if params.Limit > 0 {
		limit = params.Limit
	}

	res, err := conf.Qdrant.PointsClient.Scroll(ctx, &qdrant.ScrollPoints{
		CollectionName: conf.QdrantCollectionName.POI,
		Filter: &qdrant.Filter{
			Must: mustConditions,
		},
		Limit: &limit,
		OrderBy: &qdrant.OrderBy{
			Key:       "importance",                 // 排序的字段名
			Direction: qdrant.Direction_Desc.Enum(), // 倒序: Desc, 正序: Asc
		},
		WithPayload: qdrant.NewWithPayload(true),
	})

	if err != nil {
		return nil, fmt.Errorf("qdrant scroll error: %w", err)
	}

	results := []*SearchPOIResult{}
	for _, v := range res.GetResult() {

		poi, err := d.QdrantMapPayloadToStruct(v.Id, v.Payload,
			params.Lat, params.Lon, params.Heading)
		if err != nil {
			return nil, err
		}

		// 根据当前位置获取方向
		// distance := float64(0)

		// if params.Lat != nil && params.Lon != nil {
		// 	distance = methods.GetGeoDistance(poi.Location.Lat, poi.Location.Lon, *params.Lat, *params.Lon)
		// }

		score := poi.CalculateFinalScore()

		if poi.Importance != score {
			poi.Importance = score

			updatePayload := map[string]*qdrant.Value{
				"importance": qdrant.NewValueInt(score),
			}

			err = poiDbx.UpdatePayloadById(ctx, poi.Id, updatePayload)
			if err != nil {
				log.Error(err)
				return nil, err
			}
		}

		// poi.Distance = distance

		// log.Info(poi.Name, distance)

		results = append(results, &SearchPOIResult{
			Point: v,
			POI:   poi,
		})
	}

	// 排序优先级：有百科、importance、距离
	// log.Info("排序优先级")

	if len(results) > 0 {
		closestIdx := 0
		for k := 1; k < len(results); k++ {
			if results[k].POI.Distance < results[closestIdx].POI.Distance {
				closestIdx = k
			}
		}

		// 2. 将距离最近的元素交换到第一位 (Index 0)
		results[0], results[closestIdx] = results[closestIdx], results[0]

		// 3. 对剩下的元素 (从 Index 1 开始) 按照 Importance/评分 排序
		if len(results) > 2 {
			sort.Slice(results[1:], func(i, j int) bool {
				// 注意：这里的 i, j 是相对于 results[1:] 的索引
				// 如果 Importance 不同，权重大的排前面
				if results[i+1].POI.Importance != results[j+1].POI.Importance {
					return results[i+1].POI.Importance > results[j+1].POI.Importance
				}
				// 如果权重相同，则按距离排（或者你也可以按别的逻辑）
				return results[i+1].POI.Distance < results[j+1].POI.Distance
			})
		}
	}

	// for _, v1 := range results {
	// 	v := v1.POI
	// 	log.Info(v.Name, v.OSM.Type,
	// 		v.Importance, v.Distance,
	// 		v.Location.Lat, v.Location.Lon)
	// }

	return results, nil
}

func (d *POIDbx) UpdatePayloadById(ctx context.Context, pointId string, payload map[string]*qdrant.Value) error {
	waitTrue := true

	// 构造 SetPayload 请求
	req := &qdrant.SetPayloadPoints{
		CollectionName: conf.QdrantCollectionName.POI,
		Wait:           &waitTrue,
		// 设置要更新的 Payload
		Payload: payload,
		// 指定要更新的点 ID
		PointsSelector: &qdrant.PointsSelector{
			PointsSelectorOneOf: &qdrant.PointsSelector_Points{
				Points: &qdrant.PointsIdsList{
					Ids: []*qdrant.PointId{
						{
							PointIdOptions: &qdrant.PointId_Uuid{
								Uuid: pointId,
							},
						},
					},
				},
			},
		},
	}

	// 调用 SetPayload 接口（注意：这里不是 Upsert）
	_, err := conf.Qdrant.PointsClient.SetPayload(ctx, req)
	if err != nil {
		log.Error(pointId, payload, err)
		return fmt.Errorf("qdrant set payload error: %w", err)
	}

	return nil
}

func (d *POIDbx) SaveQdrant(ctx context.Context, pointId string, vector []float32, payload map[string]*qdrant.Value) error {
	// 1. 构造 Upsert 请求
	// CollectionName 必须与你之前 curl 创建的一致
	waitTrue := true

	req := &qdrant.UpsertPoints{
		CollectionName: conf.QdrantCollectionName.POI,
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

func (p *POIPayload) CalculateFinalScore() int64 {
	var score int64 = 0

	// --- A. 基础分类分 ---
	score += int64(poiDbx.GetImportanceByKind(p.Category.Kind))

	// --- B. Wiki 加成 ---
	if p.Wiki.Summary != "" {
		if p.Wiki.Cover != nil && p.Wiki.Cover.URL != "" {
			score += 2000 // 有文有图，顶级
		} else {
			score += 1500 // 有文无图，重要
		}
	}

	// --- C. OSM 实体加成 ---
	switch strings.ToLower(p.OSM.Type) {
	case "relation":
		score += 800
	case "way":
		score += 500
	case "node":
		score += 0
	}

	// --- D. 语义关键词修正 ---
	name := p.Name
	// 奖励项
	boosts := map[string]int64{
		"景区": 1000, "风景区": 1000, "风景名胜": 1000,
		"古镇": 800, "小镇": 800,
		"公园":  600,
		"广场":  500,
		"博物馆": 300,
	}
	for k, v := range boosts {
		if strings.Contains(name, k) {
			score += v
			break // 命中最大的即可
		}
	}

	// 惩罚项
	nerfs := []string{"校史馆", "校区", "办公", "宿舍",
		"内部", "锦鲤池", "荷花池", "鱼塘"}
	for _, k := range nerfs {
		if strings.Contains(name, k) {
			score -= 1200 // 惩罚力度加大，确保排在公共设施后面
			break
		}
	}

	return score
}

func (s *POIDbx) CalculateSearchRadius(alt float64) float32 {
	switch {
	case alt > 4000:
		return 10000 // 极高海拔：一眼望穿，拉大半径
	case alt > 3000:
		return 8000 // 高海拔：视野开阔
	case alt > 2000:
		return 5000 // 中海拔：起伏地带
	case alt > 1000:
		return 4000 // 低海拔：丘陵/河谷
	default:
		return 3000 // 基础半径
	}
}

func (d *POIDbx) GetImportanceByKind(kind string) int {
	switch kind {

	// --- [核心看点：顶级吸引力 (900 - 1000)] ---
	// 这些是用户出门的直接目的
	case "attraction", "viewpoint", "heritage", "national_park", "theme_park":
		return 1000
	case "museum", "gallery", "arts_centre":
		return 950
	case "castle", "fort", "city_gate", "archaeological_site":
		return 900

	// --- [自然地标：视觉与休憩 (800 - 899)] ---
	case "water", "island", "lake", "waterfall", "nature_reserve":
		return 850
	case "park", "leisure", "garden":
		return 800

	// --- [人文与宗教：辅助景观 (700 - 799)] ---
	case "square", "monument", "memorial":
		return 750
	case "place_of_worship", "temple", "church", "monastery":
		return 700

	// --- [功能补给：旅行刚需 (500 - 699)] ---
	case "hotel", "resort", "camp_site":
		return 650
	case "fuel", "charging_station", "rest_area":
		return 600
	case "hospital", "clinic":
		return 550

	// --- [交通设施：枢纽优先 (400 - 499)] ---
	case "airport", "ferry_terminal", "bus_station", "railway_station":
		return 450
	case "marina", "parking", "subway_entrance":
		return 400

	// --- [城市基础：除非搜索否则低权重 (100 - 399)] ---
	case "university", "school", "college":
		return 350
	case "townhall", "courthouse", "police":
		return 300
	case "bridge", "tower", "tunnel", "lighthouse":
		return 200

	// --- [底噪：商业与居住 (1 - 99)] ---
	case "commercial", "retail", "residential", "apartments", "house", "building":
		return 50

	default:
		return 10
	}
}
