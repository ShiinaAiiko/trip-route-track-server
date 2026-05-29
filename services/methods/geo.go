package methods

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strings"

	conf "github.com/ShiinaAiiko/nyanya-trip-route-track/server/config"
	"github.com/ShiinaAiiko/nyanya-trip-route-track/server/models"
	"github.com/cherrai/nyanyago-utils/nstrings"
)

// Return unit is meters
func GetGeoDistance(
	lat1 float64, lng1 float64,
	lat2 float64, lng2 float64) float64 {
	const PI float64 = 3.141592653589793
	radlat1 := PI * lat1 / 180
	radlat2 := PI * lat2 / 180

	theta := lng1 - lng2
	radtheta := PI * theta / 180

	dist := math.Sin(radlat1)*math.Sin(radlat2) + math.Cos(radlat1)*math.Cos(radlat2)*math.Cos(radtheta)

	if dist > 1 {
		dist = 1
	}

	dist = math.Acos(dist)
	dist = dist * 180 / PI
	dist = dist * 60 * 1.1515
	dist = dist * 1.609344
	return dist * 1000
}

func GSS(v *models.TripPosition, startTime, endTime int64) bool {
	// log.Info("gss", v.Timestamp/1000, startTime, v.Timestamp/1000, endTime)
	// log.Info(v)
	// log.Info(v.Speed != -1 && v.Speed >= 0 &&
	// 	v.Altitude != -1 && v.Altitude >= 0 &&
	// 	v.Accuracy != -1 && v.Accuracy <= 20 && v.Timestamp/1000 >= startTime && v.Timestamp/1000 <= endTime)
	return v.Speed != -1 && v.Speed >= 0 &&
		v.Altitude != -1 && v.Altitude >= 0 &&
		v.Accuracy != -1 && v.Accuracy <= 20 && v.Timestamp/1000 >= startTime && v.Timestamp/1000 <= endTime
}

func GetGeoKey(mapKeys *(map[string]int), latlon string, keyIndex *int) string {
	latlons := strings.Split(latlon, ".")
	k := latlons[0] + "." + latlons[1][0:2]

	if (*mapKeys)[k] == 0 {
		*keyIndex++
		(*mapKeys)[k] = *keyIndex
	}
	return nstrings.ToString((*mapKeys)[k]) + "." + latlons[1][2:len(latlons[1])-1]
}

// Point 表示一个二维点
type Point [2]float64

// IsPointInMultiPolygon 判断点是否在任意一个多边形内
func IsPointInMultiPolygon(point *Point, polygons [][]*Point) bool {
	for _, poly := range polygons {
		if IsPointInPolygon(point, poly) {
			return true
		}
	}
	return false
}

// IsPointInPolygon 使用射线法判断点是否在单个多边形内
func IsPointInPolygon(point *Point, polygon []*Point) bool {
	x, y := point[0], point[1]
	inside := false

	n := len(polygon)
	if n < 3 {
		return false // 至少需要3个点才能形成多边形
	}

	for i, j := 0, n-1; i < n; j, i = i, i+1 {
		xi, yi := (polygon)[i][0], (polygon)[i][1]
		xj, yj := (polygon)[j][0], (polygon)[j][1]

		// 检查点是否在多边形的边上
		if onSegment(xi, yi, xj, yj, x, y) {
			return true
		}

		// 射线法核心判断
		intersect := (yi > y) != (yj > y) &&
			x < (xj-xi)*(y-yi)/(yj-yi)+xi
		if intersect {
			inside = !inside
		}
	}

	return inside
}

// onSegment 判断点是否在线段上
func onSegment(xi, yi, xj, yj, x, y float64) bool {
	if x <= max(xi, xj) && x >= min(xi, xj) &&
		y <= max(yi, yj) && y >= min(yi, yj) {
		area := (xj-xi)*(y-yi) - (yj-yi)*(x-xi)
		return area == 0
	}
	return false
}

func max(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

func min(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

func FormatDistance(distance float64) string {
	if distance < 1000 {
		// 不足 1000m，输出为整数米
		// 使用 %.0f 会进行四舍五入，如果需要直接截断可以 int(distance)
		return fmt.Sprintf("%.0fm", distance)
	}

	// 超过 1000m，转换为 km，保留 2 位小数
	km := distance / 1000
	return fmt.Sprintf("%.2fkm", km)
}

type GeoResponse struct {
	Author      string   `json:"author"`
	CnMsg       string   `json:"cnMsg"`
	Code        int      `json:"code"`
	Data        *GeoData `json:"data"`
	Msg         string   `json:"msg"`
	Platform    string   `json:"platform"`
	RequestTime int64    `json:"requestTime"`
}

type GeoData struct {
	Address   string  `json:"address"`
	City      string  `json:"city"`
	Country   string  `json:"country"`
	Latitude  float64 `json:"latitude"`
	Level     string  `json:"level"`
	Longitude float64 `json:"longitude"`
	Region    string  `json:"region"`
	Road      string  `json:"road"`
	State     string  `json:"state"`
	Town      string  `json:"town"`
}

// GetGeocode 获取指定地址的地理编码信息
func Geo(address string) (*GeoData, error) {
	// 1. 构建 URL 并进行转义
	baseURL := conf.Config.ToolsApiUrl + "/api/v1/geocode/geo"
	params := url.Values{}
	params.Add("address", address)
	params.Add("platform", "Amap")

	fullURL := fmt.Sprintf("%s?%s", baseURL, params.Encode())

	log.Info("Geo fullURL", fullURL)

	// 2. 发起 GET 请求
	resp, err := http.Get(fullURL)
	if err != nil {
		return nil, fmt.Errorf("network request failed: %v", err)
	}
	defer resp.Body.Close()

	// 3. 读取响应体
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read body failed: %v", err)
	}

	// 4. 解析 JSON
	var result GeoResponse
	err = json.Unmarshal(body, &result)
	if err != nil {
		log.Error("Geo body", string(body))
		return nil, fmt.Errorf("json unmarshal failed: %v", err)
	}

	return result.Data, nil
}
