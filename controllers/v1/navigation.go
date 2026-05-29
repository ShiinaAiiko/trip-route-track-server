package controllersV1

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"sort"
	"strings"
	"time"

	conf "github.com/ShiinaAiiko/nyanya-trip-route-track/server/config"
	"github.com/ShiinaAiiko/nyanya-trip-route-track/server/protos"
	"github.com/ShiinaAiiko/nyanya-trip-route-track/server/services/response"
	"github.com/cherrai/nyanyago-utils/cipher"
	"github.com/cherrai/nyanyago-utils/narrays"
	"github.com/cherrai/nyanyago-utils/nfile"
	"github.com/cherrai/nyanyago-utils/nstrings"
	"github.com/cherrai/nyanyago-utils/saass"
	"github.com/cherrai/nyanyago-utils/validation"
	sso "github.com/cherrai/saki-sso-go"
	"github.com/gin-gonic/gin"
)

// "github.com/cherrai/nyanyago-utils/validation"

var ()

type NavigationController struct {
}

// OpenRouteService 响应顶级结构
type ORSResponse struct {
	Type     string       `json:"type"`     //  通常是 "FeatureCollection"
	Features []ORSFeature `json:"features"` // 路线特征数组
	BBox     []float64    `json:"bbox"`     // 边界框 [minLng, minLat, maxLng, maxLat]
	Metadata ORSMetadata  `json:"metadata"` // 元数据
}

// Feature 结构
type ORSFeature struct {
	Type       string        `json:"type"`       // 通常是 "Feature"
	Properties ORSProperties `json:"properties"` // 路线属性
	Geometry   ORSGeometry   `json:"geometry"`   // 几何信息
	BBox       []float64     `json:"bbox"`       // 这条路线的边界框
}

// 属性结构
type ORSProperties struct {
	Segments  []ORSSegment `json:"segments"`   // 路段数组（每个途经点之间为一个segment）
	Summary   ORSSummary   `json:"summary"`    // 总结信息
	WayPoints []int        `json:"way_points"` // 途经点索引 [0, 最后一个点索引]
}

// 总结信息
type ORSSummary struct {
	Distance float64 `json:"distance"` // 总距离（米）
	Duration float64 `json:"duration"` // 总耗时（秒）
}

// 路段结构
type ORSSegment struct {
	Distance float64   `json:"distance"` // 这段路的距离（米）
	Duration float64   `json:"duration"` // 这段路的耗时（秒）
	Steps    []ORSStep `json:"steps"`    // 步骤数组（当 instructions=true 时才有）
}

// 步骤结构（道路名称在这里）
type ORSStep struct {
	Distance    float64 `json:"distance"`    // 这一步的距离
	Duration    float64 `json:"duration"`    // 这一步的耗时
	Instruction string  `json:"instruction"` // 文字说明，如 "Head northeast on G50"
	Name        string  `json:"name"`        // 道路名称，如 "G50沪渝高速"
	Type        int     `json:"type"`        // 转向类型（1=右转，0=左转，10=到达）
	WayPoints   []int   `json:"way_points"`  // 这一步的坐标索引范围 [start, end]
}

// 几何信息
type ORSGeometry struct {
	Coordinates [][]float64 `json:"coordinates"` // 坐标点数组 [经度, 纬度, 可选海拔]
	Type        string      `json:"type"`        // 通常是 "LineString"
}

// 元数据
type ORSMetadata struct {
	Attribution string    `json:"attribution"`
	Service     string    `json:"service"`
	Timestamp   int64     `json:"timestamp"`
	Query       ORSQuery  `json:"query"`
	Engine      ORSEngine `json:"engine"`
}

// 查询参数
type ORSQuery struct {
	Coordinates [][]float64 `json:"coordinates"`
	Profile     string      `json:"profile"`
	Format      string      `json:"format"`
}

// 引擎信息
type ORSEngine struct {
	Version   string `json:"version"`
	BuildDate string `json:"build_date"`
	GraphDate string `json:"graph_date"`
	OSMDate   string `json:"osm_date"`
}

// 简化版响应（只提取需要的数据）
type SimpleRouteInfo struct {
	Distance      float64         // 总距离（米）
	Duration      float64         // 总耗时（秒）
	Segments      []SimpleSegment // 分段信息
	Polyline      [][]float64     // 路线坐标
	RoadNames     []string        // 经过的主要道路
	BBox          []float64       // 边界框
	RequestCoords [][]float64     // 请求的坐标
}

type SimpleSegment struct {
	Distance float64 // 这段路的距离
	Duration float64 // 这段路的耗时
	RoadName string  // 这段路的主要道路名称
}

func (fc *NavigationController) GetNavigationData(c *gin.Context) {
	// 1、请求体
	var res response.ResponseProtobufType
	res.Code = 200

	// 2、获取参数
	data := new(protos.GetNavigationData_Request)
	var err error
	if err = protos.DecodeBase64(c.GetString("data"), data); err != nil {
		res.Error = err.Error()
		res.Code = 10002
		res.Call(c)
		return
	}

	// 3、验证参数
	if err = validation.ValidateStruct(
		data,
		validation.Parameter(&data.Waypoints, validation.Length(1, 20), validation.Required()),
		validation.Parameter(&data.TravelOptions, validation.Enum([]string{
			"driving-car",
			"driving-hgv",
			"cycling-regular",
			"foot-walking",
			"cycling-electric",
		}), validation.Required()),
		validation.Parameter(&data.Preference, validation.Enum([]string{
			"fastest",
			"shortest",
			"recommended",
		}), validation.Required()),
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

	// conf.Config.OpenRouteService.ApiKey

	apiKey := conf.Config.OpenRouteService.ApiKey

	coordinates := narrays.Map(data.Waypoints, func(v *protos.GetNavigationData_Request_Coords, i int) []float64 {
		return []float64{v.Longitude, v.Latitude}
	})

	sort.SliceStable(data.Waypoints, func(i, j int) bool {
		return data.Waypoints[i].Latitude < data.Waypoints[j].Latitude
	})

	fileName := cipher.MD5(strings.Join(narrays.Map(data.Waypoints, func(v *protos.GetNavigationData_Request_Coords, i int) string {
		return nstrings.ToString(v.Longitude) + "," + nstrings.ToString(v.Latitude)
	}), ","))

	// 构建参数
	params := map[string]interface{}{
		"coordinates":  coordinates,
		"instructions": false,
		"elevation":    false,
		"geometry":     true,
		"preference":   data.Preference,
		"options": map[string]interface{}{
			"avoid_features": getSupportedAvoidFeatures(data.TravelOptions, data.RouteOptions),
		},
		"radiuses": []int{5000, 5000},
	}

	// log.Info("params", data.TravelOptions, data.RouteOptions, getSupportedAvoidFeatures(data.TravelOptions, data.RouteOptions))

	jsonData, _ := json.Marshal(params)

	// 创建请求
	url := "https://api.openrouteservice.org/v2/directions/" + data.TravelOptions + "/geojson"
	req, _ := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	req.Header.Set("Authorization", apiKey)
	req.Header.Set("Content-Type", "application/json")

	// 发送
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Println("请求失败:", err)
		return
	}
	defer resp.Body.Close()

	body, _ := ioutil.ReadAll(resp.Body)

	// fmt.Println("状态码:", resp.StatusCode)
	// fmt.Println("响应头:", resp.Header)
	// // fmt.Println("响应体:", string(body))
	// fmt.Println("响应体:", len(string(body)))

	// bodyStr := string(body)

	var respMap map[string]any

	if err = json.Unmarshal(body, &respMap); err != nil {
		log.Error(err)
		res.Errors(err)
		res.Code = 10001
		res.Call(c)
		return
	}

	// log.Info("respMap", respMap["error"])

	if respMap["error"] != nil {
		log.Error(respMap["error"])
		res.Errors(errors.New(nstrings.ToString(respMap["error"])))
		res.Code = 10001
		res.Call(c)
		return
	}

	summary := respMap["features"].([]interface{})[0].(map[string]interface{})["properties"].(map[string]interface{})["summary"].(map[string]interface{})

	distance := summary["distance"].(float64)
	duration := summary["duration"].(float64)

	reader := bytes.NewReader(body)
	size := reader.Size()
	hash := nfile.MuseGetHashByBytes(body)

	ut, err := conf.SAaSS.CreateChunkUploadToken(&saass.CreateUploadTokenOptions{
		FileInfo: &saass.FileInfo{
			Name:         fileName,
			Size:         size,
			Type:         "application/json",
			Suffix:       ".json",
			LastModified: time.Now().UnixMilli(),
			Hash:         hash,
		},
		// Path: "/trip/files/" + time.Now().Format("2006/01/02") + "/",
		// FileName: strings.ToLower(cipher.MD5(
		// 	data.FileInfo.Hash+nstrings.ToString(data.FileInfo.Size)+nstrings.ToString(time.Now().Unix()))) + data.FileInfo.Suffix,
		Path:             "/trip/navigation/files/",
		FileName:         fileName,
		ChunkSize:        256 * 1024,
		VisitCount:       -1,
		ExpirationTime:   time.Now().AddDate(0, 0, 180).Unix(),
		AutoExtendPeriod: 60 * 60 * 24 * 180,
		// Type:           "File",
		FileConflict: "Replace",

		AllowShare: 1,
		RootPath:   conf.SAaSS.GenerateRootPath(userInfo.Uid),
		UserId:     userInfo.Uid,
		ShareUsers: []string{"AllUser"},

		OnProgress: func(progress saass.Progress) {
			// log.Info("progress", progress)
		},
		OnSuccess: func(urls saass.Urls) {
			// log.Info("urls", urls)
		},
		OnError: func(err error) {
			// log.Info("err", err)
		},
	})
	if err != nil {
		res.Errors(err)
		res.Code = 10001
		res.Call(c)
		return
	}

	urls, err := ut.ChunkUpload(body)
	// log.Info(urls, err, "uploadUserId", len(body))
	if err != nil {
		res.Errors(err)
		res.Code = 10001
		res.Call(c)
		return
	}

	protoData := &protos.GetNavigationData_Response{
		NavigationData: &protos.RoadbookWaypointNavigation{
			Distance:   distance,
			Duration:   duration,
			TravelMode: data.TravelOptions,
			Urls: &protos.Urls{
				DomainUrl: urls.DomainUrl,
				ShortUrl:  urls.ShortUrl,
				Url:       urls.Url,
			},
		},
	}

	res.Data = protos.Encode(protoData)
	log.Info("data.Waypoints", data, protoData)

	res.Call(c)
}

func getSupportedAvoidFeatures(profile string, requestedAvoids []string) []string {
	// 定义各profile支持的avoid_features
	supportedMap := map[string]map[string]bool{
		"driving-car": {
			"highways": true,
			"tollways": true,
			"ferries":  true,
			"fords":    true,
			"tunnels":  true,
		},
		"driving-hgv": {
			"highways": true,
			"tollways": true,
			"ferries":  true,
			"fords":    true,
			"tunnels":  true,
		},
		"cycling-regular": {
			"ferries": true,
			"fords":   true,
			"tunnels": true,
			// 不支持 highways, tollways
		},
		"cycling-mountain": {
			"ferries": true,
			"fords":   true,
			"tunnels": true,
		},
		"cycling-road": {
			"ferries": true,
			"fords":   true,
			"tunnels": true,
		},
		"cycling-electric": {
			"ferries": true,
			"fords":   true,
			"tunnels": true,
			// 不支持 highways, tollways
		},
		"foot-walking": {
			"steps":   true,
			"ferries": true,
			"fords":   true,
			// 不支持 highways, tunnels 等
		},
		"foot-hiking": {
			"steps":   true,
			"ferries": true,
			"fords":   true,
		},
		"wheelchair": {
			"steps":   true,
			"ferries": true,
			"fords":   true,
		},
	}

	// 获取该profile支持的avoid_features
	supported, exists := supportedMap[profile]
	if !exists {
		// 如果不支持的profile，返回空数组
		return []string{}
	}

	// 筛选出requested中受支持的项
	result := make([]string, 0)
	for _, avoid := range requestedAvoids {
		if supported[avoid] {
			result = append(result, avoid)
		}
	}

	return result
}
