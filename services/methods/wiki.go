package methods

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	conf "github.com/ShiinaAiiko/nyanya-trip-route-track/server/config"
	"golang.org/x/time/rate"
)

var (
	wikiCount = 0
	// wikiLimiter = rate.NewLimiter(rate.Every(5*time.Second), 1)
	wikiLimiter = rate.NewLimiter(rate.Every(3*time.Second), 5)
)

type WikiPage struct {
	PageID int    `json:"pageid"`
	Title  string `json:"title"`
	// summary内容
	Extract string `json:"extract"`
	FullURL string `json:"fullurl"`
	// 如果不存在，该字段会被返回
	Missing   string `json:"missing"`
	Thumbnail struct {
		Source string `json:"source"`
		Width  int64  `json:"width"`
		Height int64  `json:"height"`
	} `json:"thumbnail"`
	Coordinates []struct {
		Lat float64 `json:"lat"`
		Lon float64 `json:"lon"`
	} `json:"coordinates"`
	PageProps struct {
		WikiBaseItem string `json:"wikibase_item"` // QID
	} `json:"pageprops"`
}

func GetWikiSummary(ctx context.Context, keywords string) (*WikiPage, error) {

	// // 构造请求 URL
	wikiUrl := fmt.Sprintf(
		"https://zh.wikipedia.org/w/api.php?action=query&prop=extracts|pageimages|pageprops|info|coordinates&exintro&explaintext&pithumbsize=1000&inprop=url&titles=%s&format=json&redirects=1&converttitles=1",
		url.QueryEscape(keywords))

	apiURL := wikiUrl
	wikiCount++
	log.Info("GetWikiSummary URL", wikiCount, apiURL)

	if err := wikiLimiter.Wait(ctx); err != nil {
		return nil, errors.New("WikiLimiter => 提前结束，限流报错了")
	}

	headers := map[string]string{
		"User-Agent": "TripPOI/1.0 (contact: shiina@aiiko.club)",
	}

	if conf.Config.Server.Mode == "debug" {

		headersStr, _ := json.Marshal(headers)

		apiURL = fmt.Sprintf(
			conf.Config.ToolsApiUrl+
				// "http://192.168.204.130:23201"+
				`/api/v1/net/httpProxy?headers=%s&method=GET&url=%s`,
			url.QueryEscape(string(headersStr)), url.QueryEscape(wikiUrl))

	}

	// log.Info("GetWikiSummary", conf.Config.Server.Mode, apiURL)

	// return nil
	// // 创建带有超时的客户端（自驾数据处理建议 10s 超时）

	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create request error: %v", err)
	}

	if conf.Config.Server.Mode != "debug" {
		req.Header.Set(
			"User-Agent", headers["User-Agent"],
		)
	}

	client := &http.Client{
		Timeout: 10 * time.Second,
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("network error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GetWikiSummary api error: status code %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	// log.Info("GetAddress body", string(body))

	type WikiResponse struct {
		Query struct {
			Pages map[string]*WikiPage `json:"pages"`
		} `json:"query"`
	}

	result := new(WikiResponse)

	if conf.Config.Server.Mode == "debug" {

		type ProxyResponse struct {
			Code int `json:"code"`
			Data *WikiResponse
		}

		var proxyResult ProxyResponse
		if err := json.Unmarshal(body, &proxyResult); err != nil {
			return nil, fmt.Errorf("json decode error: %v", err)
		}

		if proxyResult.Code == 200 {
			result = proxyResult.Data
		}
		log.Info("proxyResult", proxyResult)

	} else {

		if err := json.Unmarshal(body, result); err != nil {
			return nil, fmt.Errorf("json decode error: %v", err)
		}

	}

	var wikiPage *WikiPage

	if result != nil || len(result.Query.Pages) == 0 {
		for _, v := range result.Query.Pages {
			if v != nil {
				wikiPage = v
				break
			}
		}
	}

	return wikiPage, nil

}

func GetCityWikiSummary(ctx context.Context, cities []string) (*WikiPage, error) {

	var wikiPage *WikiPage
	var err error

	// 2. 循环尝试每一个非空的查询词
	for i, q := range cities {
		if q == "" {
			continue
		}
		topCity := cities[i]

		wikiPage, err = GetWikiSummary(ctx, q)
		if err != nil {
			return nil, err
		}

		// 如果查到了，直接跳出循环
		if wikiPage != nil && wikiPage.Extract != "" {
			if topCity != "" && strings.Contains(wikiPage.Extract, topCity) {
				break
			}
		}
	}

	log.Info("wikiPage", wikiPage)

	if wikiPage != nil && wikiPage.Extract != "" {

		return wikiPage, nil
	}

	return nil, nil
}
