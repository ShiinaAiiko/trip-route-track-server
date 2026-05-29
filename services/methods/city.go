package methods

// import (
// 	"encoding/json"
// 	"errors"

// 	conf "github.com/ShiinaAiiko/nyanya-trip-route-track/server/config"
// 	"github.com/cherrai/nyanyago-utils/nfile"
// )

// func GetCityBoundaries(version string) {

// 	log.Info("GetCityBoundaries", version)

// 	basePath := "./static/cityBoundaries/" + version

// 	log.Info("basePath", basePath, nfile.IsExists(basePath))

// 	if !nfile.IsExists(basePath) {
// 		if err := nfile.Mkdir(basePath, 0755); err != nil {
// 			log.Error(err)
// 			return
// 		}
// 	}

// 	districts, err := GetAllAmapCityDistricts("中国", basePath)
// 	if err != nil {
// 		log.Error(err)
// 		return
// 	}

// 	log.Info("districts", districts)

// 	if err := getCityBoundaries(districts, basePath); err != nil {
// 		log.Error(err)
// 		return
// 	}

// 	// https://restapi.amap.com/v3/config/district?keywords=%E4%B8%AD%E5%9B%BD&subdistrict=3&key=fb7fdf3663af7a532b8bdcd1fc3e6776
// }

// func getCityBoundaries(districts *CityDistricts, basePath string) error {

// 	log.Info(districts.Name)

// 	for _, v := range districts.Districts {

// 		if err := getCityBoundaries(v, basePath); err != nil {
// 			return err
// 		}

// 	}

// 	return nil
// }

// type CityDistricts struct {
// 	Adcode    string
// 	Name      string
// 	Center    string
// 	Level     string
// 	Districts []*CityDistricts
// }

// type AmapRes struct {
// 	Status    string
// 	Info      string
// 	Count     string
// 	Districts []*CityDistricts
// }

// func GetAllAmapCityDistricts(country string, basePath string) (*CityDistricts, error) {

// 	jsonPath := basePath + "/cityDirectory.json"

// 	cityDistricts := []*CityDistricts{}

// 	if err := nfile.ReadJsonFile(jsonPath, &cityDistricts); err != nil {
// 		return nil, err
// 	}

// 	log.Info("len(cityDistricts)", len(cityDistricts))
// 	if len(cityDistricts) > 0 {
// 		return cityDistricts[0], nil
// 	}

// 	resp, err := conf.RestyClient.R().SetQueryParams(map[string]string{}).
// 		Get(
// 			"https://restapi.amap.com/v3/config/district?keywords=" + country +
// 				"&subdistrict=3&key=fb7fdf3663af7a532b8bdcd1fc3e6776",
// 		)
// 	if err != nil {
// 		log.Error(err)
// 		return nil, err
// 	}
// 	amapRes := new(AmapRes)
// 	// log.Info("resp.Body()", resp.String())
// 	if err = json.Unmarshal(resp.Body(), amapRes); err != nil {
// 		return nil, err
// 	}
// 	// log.Info("resp.Body()", res)

// 	if amapRes.Status != "1" {
// 		return nil, errors.New(amapRes.Info)
// 	}
// 	log.Info(amapRes.Districts, len(amapRes.Districts))

// 	if err = nfile.CreateJsonFile(jsonPath, amapRes.Districts, true); err != nil {
// 		return nil, err
// 	}

// 	return amapRes.Districts[0], nil
// }
