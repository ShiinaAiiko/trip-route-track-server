package dbxV1

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	conf "github.com/ShiinaAiiko/nyanya-trip-route-track/server/config"
	"github.com/ShiinaAiiko/nyanya-trip-route-track/server/models"
	"github.com/ShiinaAiiko/nyanya-trip-route-track/server/protos"
	"github.com/ShiinaAiiko/nyanya-trip-route-track/server/services/response"
	"github.com/cherrai/nyanyago-utils/narrays"
	"github.com/cherrai/nyanyago-utils/nint"
	"github.com/cherrai/nyanyago-utils/nstrings"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
)

var (
	cityDbx = CityDbx{}
)

type CityDbx struct {
	city *models.City
}

var cityProject = bson.M{
	"_id":            1,
	"name":           1,
	"names":          1,
	"parentCityId":   1,
	"coords":         1,
	"level":          1,
	"status":         1,
	"createTime":     1,
	"lastUpdateTime": 1,
}

func (t *CityDbx) AddAndGetFullCity(ci *protos.UpdateCity_Request_City) (city *models.City, err error) {

	cities := []string{
		ci.Country,
		ci.State,
		ci.Region,
		ci.City,
		ci.Town,
	}
	// log.Info("city", city, ci, cities)

	pId := ""

	for cIndex := 0; cIndex < 5; cIndex++ {
		cName := cities[cIndex]
		if cName == "" {
			continue
		}

		cCities := narrays.Filter(cities, func(sv string, si int) bool {
			return si <= cIndex
		})

		// log.Error(cCities, cName, strings.Join(cCities, ""), pId, cIndex+1)
		city, err = t.AddAndGetCity(cName, strings.Join(cCities, ""), pId, cIndex+1)
		// log.Info("GetCity", cCities, strings.Join(cCities, ""), cName, city, err)
		if err != nil {
			return nil, err
		}
		pId = city.Id
	}

	return city, nil
}

func (t *CityDbx) GetCityCoords(fullName string) (*models.CityCoords, error) {

	coords := new(models.CityCoords)

	resp, err := conf.RestyClient.R().SetQueryParams(map[string]string{}).
		Get(
			conf.Config.ToolsApiUrl + "/api/v1/geocode/geo?address=" +
				fullName + "&platform=Amap",
			// "https://tools.aiiko.club/api/v1/geocode/geo?address=" +
			// 	fullName + "&platform=Amap",
		)
	if err != nil {
		log.Error(err)
		return nil, err
	}

	res := new(response.ResponseType)

	// log.Info("resp.Body()", resp.String())
	if err = json.Unmarshal(resp.Body(), res); err != nil {
		return nil, err
	}
	// log.Info("resp.Body()", res)

	if res.Code != 200 {
		return nil, errors.New(res.Msg)
	}

	data := res.Data.(map[string]any)

	lat, err := strconv.ParseFloat(nstrings.StringOr(nstrings.ToString(data["latitude"]), "0"), 64)
	if err == nil {
		coords.Latitude = lat
	}
	lng, err := strconv.ParseFloat(nstrings.StringOr(nstrings.ToString(data["longitude"]), "0"), 64)
	if err == nil {
		coords.Longitude = lng
	}

	return coords, nil
}

func (t *CityDbx) AddAndGetCity(name string, fullName string, parentCityId string, level int) (*models.City, error) {
	city, err := t.GetCity("", parentCityId, name, fullName)
	// log.Warn("AddAndGetCity", city, err)
	if err != nil {
		return nil, err
	}
	if city == nil {
		coords, err := t.GetCityCoords(fullName)
		if err != nil {
			return nil, err
		}
		city = &models.City{
			Name: &models.CityName{
				ZhCN: name,
			},
			Coords:       coords,
			ParentCityId: parentCityId,
			Level:        level,
			Status:       1,
		}
		if err = city.Default(); err != nil {
			return nil, err
		}
		log.Info("AddCity", city)

		city, err = t.AddCity(city)
		if err != nil {
			return nil, err
		}

	}
	if city.Coords == nil || (city.Coords.Latitude == 0 && city.Coords.Longitude == 0) {
		coords, err := t.GetCityCoords(fullName)
		if err != nil {
			return nil, err
		}
		log.Error(coords)

		if err := t.UpdateCity(city.Id, fullName, coords, nil); err != nil {
			log.Error(err)
			return city, err
		}
		city.Coords = coords
	}
	log.Info("city.Coords", city.Coords)

	return city, nil
}
func (t *CityDbx) AddCity(city *models.City) (*models.City, error) {
	// 1、插入数据
	err := city.Default()
	if err != nil {
		return nil, err
	}

	_, err = city.GetCollection().InsertOne(context.TODO(), city)
	if err != nil {
		return nil, err
	}

	return city, nil
}

func (t *CityDbx) UpdateCity(
	id, fullName string, coords *models.CityCoords,
	names *models.CityNames) error {
	city := new(models.City)

	setUp := bson.M{
		"lastUpdateTime": time.Now().Unix(),
	}

	if coords != nil {
		setUp["coords"] = coords
	}

	if names != nil {
		setUp["names"] = names
	}

	// log.Info("setUp", setUp)

	updateResult, err := city.GetCollection().UpdateOne(context.TODO(),
		bson.M{
			"$and": []bson.M{
				{
					"_id": id,
				},
			},
		}, bson.M{
			"$set": setUp,
		}, options.Update().SetUpsert(false))

	if err != nil {
		return err
	}
	// log.Info("updateResult", updateResult)
	if updateResult.ModifiedCount == 0 {
		return errors.New("update fail")
	}

	// 删除对应redis
	_ = t.DeleteRedisData(id, fullName)
	return nil
}

func (t *CityDbx) GetCity(id, parentCityId, name string, fullName string) (*models.City, error) {
	city := &models.City{}

	key := conf.Redisdb.GetKey("GetCity")
	err := conf.Redisdb.GetStruct(key.GetKey(id+fullName), city)

	// log.Error("GetCity", city)
	if err != nil || city.Id == "" {
		match := []bson.M{}
		if id != "" {
			match = append(match, bson.M{
				"_id": id,
			})
		}
		if name != "" {
			nameMatch := []bson.M{}

			for _, v := range models.CityNameLanguages {
				nameMatch = append(nameMatch, bson.M{
					"name." + v: name,
				})
			}
			match = append(match, bson.M{
				"$or": nameMatch,
			})
		}
		if parentCityId != "" {
			match = append(match, bson.M{
				"parentCityId": parentCityId,
			})
		}

		// log.Info("name != ", name != "")

		params := []bson.M{
			{
				"$match": bson.M{
					"$and": match,
				},
			}, {
				"$sort": bson.M{
					"createTime": -1,
				},
			},
			{
				"$project": cityProject,
			},
			{
				"$skip": 0,
			},
			{
				"$limit": 1,
			},
		}

		aOptions := options.Aggregate()
		aOptions.SetAllowDiskUse(true)

		log.Info("match", match, id, name, fullName)

		var results []*models.City

		opts, err := city.GetCollection().Aggregate(context.TODO(), params,
			aOptions)
		if err != nil {
			log.Error(err)
			return nil, err
		}
		if err = opts.All(context.TODO(), &results); err != nil {
			log.Error(err)
			return nil, err
		}

		if len(results) == 0 {
			return nil, err
		}
		city = results[0]
	}

	if err := conf.Redisdb.SetStruct(key.GetKey(id+fullName), city, key.GetExpiration()); err != nil {
		log.Info(err)
		return nil, err
	}

	return city, nil
}

func (t *CityDbx) GetFullCityForCities(id string, cities []*models.City) (results []*models.City) {

	for _, v := range cities {
		// log.Info(v.Id, v.Name, id)
		if v.Id == id {
			results = append(results, v)
			if v.ParentCityId != "" {
				results = append(results, t.GetFullCityForCities(v.ParentCityId, cities)...)

				// log.Info("results", results)
			}
			return
		}
	}

	return
}
func (t *CityDbx) GetFullCityForCitiesProto(id string, cities []*protos.CityItem) (citiesProto []*protos.CityItem) {

	for _, v := range cities {
		if v.Id == id {
			citiesProto = append(citiesProto, v)
			if v.ParentCityId != "" {
				citiesProto = append(citiesProto, t.GetFullCityForCitiesProto(v.ParentCityId, cities)...)
			}
			return
		}
	}

	return
}

// func (t *CityDbx) GetCities(ids []string) (cities []*models.City, err error) {

// 	for _, id := range ids {
// 		pCities, err := t.GetFullCities(id)
// 		if err != nil {
// 			return nil, nil
// 		}
// 		cities = append(cities, pCities...)

// 	}

//		return cities, nil
//	}

func (t *CityDbx) getCities(ids []string, lastResultIds []string) ([]*models.City, error) {
	city := new(models.City)

	ids = narrays.StructDeduplication(
		narrays.Filter(ids,
			func(value string, index int) bool {
				return !narrays.Includes(lastResultIds, value)
			}), func(a string, b string) bool {
			return a == b
		})
	// log.Info("ids", ids)

	if len(ids) == 0 {
		return nil, nil
	}
	// log.Info(len(ids))

	var err error
	var results []*models.City

	// key := conf.Redisdb.GetKey("GetCities")

	// mKey := key.CreateMKey(ids...)

	// conf.Redisdb.Delete(key.GetKey("sD3Lsp8SG"))

	// cmds, emptyKeyIndices, err := conf.Redisdb.MGet(mKey.GetMKey())

	emptyKeys := ids

	// emptyKeys := mKey.GetEmptyKeys(emptyKeyIndices...)

	// log.Info("cmds", len(cmds), len(ids), ids, err)
	// log.Info("emptyKeys", len(emptyKeyIndices), emptyKeyIndices, emptyKeys)

	// for _, v := range cmds {
	// 	city := new(models.City)
	// 	v.Struct(city)
	// 	results = append(results, city)
	// }

	if err != nil || len(emptyKeys) != 0 {

		// 1. 定义结果容器和批次大小
		const batchSize = 500
		totalKeys := len(emptyKeys)

		log.Info("开始分批获取城市数据，总计 ID 数:", totalKeys)

		for i := 0; i < totalKeys; i += batchSize {
			// 计算当前批次的边界
			end := i + batchSize
			if end > totalKeys {
				end = totalKeys
			}

			// 2. 截取当前批次的 ID 列表
			currentBatchKeys := emptyKeys[i:end]

			// 3. 构建当前批次的查询参数
			params := []bson.M{
				{
					"$match": bson.M{
						"_id": bson.M{
							"$in": currentBatchKeys,
						},
					},
				},
				{
					"$project": cityProject,
				},
			}

			aOptions := options.Aggregate()
			aOptions.SetAllowDiskUse(true)
			// 显式设置 BatchSize，配合手动分批，让单次往返更高效
			aOptions.SetBatchSize(int32(batchSize))

			// 4. 执行聚合查询
			var batchResults []*models.City

			l := log.Time()
			cursor, err := city.GetCollection().Aggregate(context.TODO(), params, aOptions)
			if err != nil {
				log.Error("批次查询失败:", err)
				return nil, err
			}
			l.TimeEnd(fmt.Sprintf("Aggregate [Batch %d-%d]", i, end))

			// 5. 解析当前批次数据
			l2 := log.Time()
			if err = cursor.All(context.TODO(), &batchResults); err != nil {
				log.Error("解析批次数据失败:", err)
				cursor.Close(context.TODO())
				return nil, err
			}
			l2.TimeEnd(fmt.Sprintf("opts.All [Batch %d-%d]", i, end))
			cursor.Close(context.TODO())

			// 6. 汇总结果
			results = append(results, batchResults...)
		}

		// 将汇总结果赋值回原变量
		// results = allResults

		// results = append(results, newResults...)

		if len(results) == 0 {
			return nil, nil
		}

	}

	// redis
	// values := map[string]any{}
	// for _, v := range results {
	// 	values[key.GetKey(v.Id)] = v
	// }

	// // log.Info("results", len(results), len(values))

	// err = conf.Redisdb.MSetStruct(values, key.GetExpiration())
	// if err != nil {
	// 	log.Info(err)
	// }
	// log.Error("results", len(results), len(values))

	parentIds := narrays.Filter(narrays.Map(results, func(v *models.City, i int) string {
		return v.ParentCityId
	}),
		func(v string, i int) bool {
			return v != ""
		})

	// log.Info("parentIds", parentIds)

	if len(parentIds) == 0 {
		return results, nil
	}
	parentResult, err := t.getCities(parentIds, append(lastResultIds, ids...))
	if err != nil {
		log.Error(err)
		return nil, err
	}
	results = append(results, parentResult...)

	return results, nil
}

// 缺redis
func (t *CityDbx) GetCities(ids []string) ([]*models.City, error) {
	var cities []*models.City

	fsdb := t.city.GetFsDB()

	// k := cipher.MD5(strings.Join(ids, ","))

	// results, err := fsdb.Cities.Get(k)
	// if err == nil {

	// 	tempResult := results.Value()
	// 	if len(tempResult) > 0 {
	// 		cities = tempResult
	// 	}
	// }

	results := fsdb.City.MGet(ids)

	tempIds := []string{}

	// log.Info(results)
	for _, val := range results {
		// log.Info(val.Key, val.Err)
		if val.Err == nil {
			tempVal := val.Val.Value()

			// log.Info(val.Key)
			// fsdb.City.Delete(val.Key)

			cities = append(cities, tempVal)
			if tempVal.ParentCityId != "" && !narrays.Includes(tempIds, tempVal.ParentCityId) {
				tempIds = append(tempIds, tempVal.ParentCityId)
			}
			continue
		}
		tempIds = append(tempIds, val.Key)
	}

	// cities = cities[:0]
	// tempIds = ids

	log.Info("GetCities", len(ids), len(cities), len(tempIds))
	if len(cities) == 0 || len(tempIds) > 0 {
		tempCities, err := t.getCities(tempIds, []string{})

		log.Error("tempCities", len(tempCities), len(tempIds))
		if err != nil {
			return nil, err
		}

		// return tempCities, nil

		tempCities, err = t.CitiesI18n(tempCities)

		if err != nil {
			return nil, err
		}

		// cities = tempCities

		for _, v := range tempCities {
			err = fsdb.City.Set(v.Id, v, 15*7*24*time.Hour)

			if err != nil {
				log.Error(err)
			}
		}

		cities = append(cities, tempCities...)

		// err = fsdb.Cities.Set(k, cities, 15*time.Minute)

		// if err != nil {
		// 	log.Error(err)
		// }

	}
	// log.Info(len(ids), len(cities), len(ids))

	return cities, nil
}

func (t *CityDbx) DeleteRedisData(id string, fullName string) error {

	key := conf.Redisdb.GetKey("GetCity")

	if err := conf.Redisdb.Delete(key.GetKey(id + fullName)); err != nil {
		log.Error(err)
	}

	if err := conf.Redisdb.Delete(key.GetKey(id + "")); err != nil {
		log.Error(err)
	}

	if err := conf.Redisdb.Delete(key.GetKey("" + fullName)); err != nil {
		log.Error(err)
	}

	return nil
}

func (t *CityDbx) InitTripPositionCity(tripId string) error {
	trip, err := tripDbx.GetTripById(tripId)
	if err != nil {
		return err
	}
	tripPositions, err := tripDbx.GetTripPositions(trip.Id, trip.AuthorId)
	if err != nil {
		return err
	}

	nextPosTime := int64(0)
	count := 1
	for _, v := range tripPositions.Positions {

		if v.Timestamp > nextPosTime {
			log.Info(count, v.Latitude, v.Timestamp)
			nextPosTime = v.Timestamp + 20*1000
			count++
		}
	}
	// log.Info("tripPositions", len(tripPositions.Positions))

	return nil
}

type UserVisitedCities struct {
	CityId         string
	FirstEntryTime int64
	LastEntryTime  int64
	EntryCount     int32
}

// 缺redis
func (t *CityDbx) GetAllCitiesVisitedByUser(authorId string, tripIds []string) (cities []*UserVisitedCities, err error) {
	trip := new(models.Trip)

	match := bson.M{
		"authorId": authorId,
		"status":   1,
		"cities": bson.M{
			"$exists": true,
			"$not": bson.M{
				"$size": 0,
			},
		},
	}
	if len(tripIds) > 0 {
		match["_id"] = bson.M{
			"$in": tripIds,
		}
	}

	params := []bson.M{
		{
			"$match": match,
		}, {
			"$project": bson.M{
				"cities": 1,
			},
		},
		{
			"$unwind": "$cities",
		},
		{
			"$unwind": "$cities.entryTimes",
		}, {
			"$group": bson.M{
				"_id":            "$cities.cityId",
				"firstEntryTime": bson.M{"$min": "$cities.entryTimes"},
				"lastEntryTime":  bson.M{"$max": "$cities.entryTimes"},
				"entryCount":     bson.M{"$sum": 1},
			},
		},
		{
			"$project": bson.M{
				"_id":            0,
				"cityId":         "$_id",
				"firstEntryTime": "$firstEntryTime.timestamp",
				"lastEntryTime":  "$lastEntryTime.timestamp",
				"entryCount":     1,
			},
		},
	}

	var results []map[string]any
	opts, err := trip.GetCollection().Aggregate(context.TODO(), params)
	if err != nil {
		// log.Error(err)
		return nil, err
	}
	if err = opts.All(context.TODO(), &results); err != nil {
		// log.Error(err)
		return nil, err
	}

	// log.Info("results", results)

	// type CityIdType struct {
	// 	CityId    string
	// 	EntryTime int64
	// }

	// cityIds := []*CityIdType{}

	for _, v := range results {
		// log.Info(v)

		// if nstrings.ToString(v["cityId"]) == "" {
		// 	log.Info(v)
		// }

		cities = append(cities, &UserVisitedCities{
			CityId:         nstrings.ToString(v["cityId"]),
			FirstEntryTime: nint.ToInt64(v["firstEntryTime"]),
			LastEntryTime:  nint.ToInt64(v["firstEntryTime"]),
			EntryCount:     v["entryCount"].(int32),
		})

		// log.Info(v["entryCount"], nint.ToInt32())
		// for _, sv := range v.Cities {

		// 	cityIds = append(cityIds, &CityIdType{
		// 		CityId: sv.CityId,
		// 		EntryTime: nmath.Min(narrays.Map(sv.EntryTimes, func(v *models.TripCityEntryTimeItem, index int) int64 {
		// 			return v.Timestamp
		// 		})...),
		// 	})
		// }
		// cities = append(cities, v.Cities)
		// log.Info("cityIds", cityIds)

		// cityIdsMap := map[string]int64{}

		// for _, v := range cityIds {

		// 	if cityIdsMap[v.CityId] == 0 || cityIdsMap[v.CityId] > v.EntryTime {
		// 		cityIdsMap[v.CityId] = v.EntryTime
		// 	}
		// }
		// log.Info("cityIdsMap", cityIdsMap)

	}

	sort.SliceStable(cities, func(i, j int) bool {
		return cities[i].FirstEntryTime-cities[j].FirstEntryTime > 0
	})

	return cities, nil
}

func (t *CityDbx) InitCityes() (cities *map[string]int64, err error) {
	trip := new(models.Trip)

	var results []*models.Trip

	params := []bson.M{
		{
			"$match": bson.M{
				"$and": []bson.M{
					{
						// "_id": "T1HAZSw5G",
						"_id": bson.M{
							"$in": []string{"HjFiAGJwf"},
							// "$in": []string{"GfhyeHASJ", "smYOp1MrX", "l8LRZP67g", "tG0Bo3t3t"},
						},
						// "createTime": bson.M{
						// 	"$lte": 1710486182,
						// 	"$gte": 0,
						// },
						"cities": bson.M{
							"$exists": true,
							"$not": bson.M{
								"$size": 0,
							},
						},
					},
				},
			},
		},
		{
			"$project": bson.M{
				"_id":    1,
				"cities": 1,
			},
		},
	}

	opts, err := trip.GetCollection().Aggregate(context.TODO(), params)
	if err != nil {
		// log.Error(err)
		return nil, err
	}
	if err = opts.All(context.TODO(), &results); err != nil {
		// log.Error(err)
		return nil, err
	}

	log.Info("results", results)

	type CityIdType struct {
		Id        string
		EntryTime int64
	}

	for _, v := range results {
		for _, sv := range v.Cities {

			log.Info(sv.CityId)

			updateResult, err := trip.GetCollection().UpdateOne(context.TODO(),
				bson.M{
					"$and": []bson.M{
						{
							"_id": v.Id,
							// "cities._id": sv.Id,
						},
					},
				}, bson.M{
					"$set": bson.M{
						"cities": []*models.TripCity{},
					},
				}, options.Update().SetUpsert(false))

			log.Info(updateResult, err)

		}
		// cities = append(cities, v.Cities)
	}

	return nil, nil
}

func (t *CityDbx) InitAddCityesForTrip() (cities *map[string]int64, err error) {
	trip := new(models.Trip)

	var results []*models.Trip

	params := []bson.M{
		{
			"$match": bson.M{
				"$and": []bson.M{
					{
						"createTime": bson.M{
							"$lte": 1736230041,
							"$gte": 1735193241,
						},
					},
				},
			},
		},
		{
			"$sort": bson.M{
				"createTime": -1,
			},
		},
		{
			"$skip": 0,
		},
		{
			"$limit": 1,
		},
		{
			"$project": bson.M{
				"_id":        1,
				"createTime": 1,
				"cities":     1,
			},
		},
	}

	opts, err := trip.GetCollection().Aggregate(context.TODO(), params)
	if err != nil {
		// log.Error(err)
		return nil, err
	}
	if err = opts.All(context.TODO(), &results); err != nil {
		// log.Error(err)
		return nil, err
	}

	log.Info("results", results)

	for _, v := range results {
		t := time.Unix(v.CreateTime, 0)
		log.Info(v.Id, t.Format("2006-01-02 15:04:05"), len(v.Cities))

	}

	return nil, nil
}

func (t *CityDbx) SetSubCityLevel(cityId string, level int) (cities *map[string]int64, err error) {
	city := new(models.City)

	log.Info("cityId", cityId, level)

	params := []bson.M{
		{
			"$match": bson.M{
				"parentCityId": cityId,
			},
		},
	}

	aOptions := options.Aggregate()
	aOptions.SetAllowDiskUse(true)

	var results []*models.City

	opts, err := city.GetCollection().Aggregate(context.TODO(), params,
		aOptions)
	if err != nil {
		log.Error(err)
		return nil, err
	}
	if err = opts.All(context.TODO(), &results); err != nil {
		log.Error(err)
		return nil, err
	}

	log.Info("results", results)
	for _, v := range results {

		log.Info(v.Name.ZhCN)

		updateResult, err := city.GetCollection().UpdateOne(context.TODO(),
			bson.M{
				"$and": []bson.M{
					{
						"_id": v.Id,
					},
				},
			}, bson.M{
				"$set": bson.M{
					"level": level,
				},
			}, options.Update().SetUpsert(false))

		log.Info(updateResult, err)

		t.SetSubCityLevel(v.Id, level+1)

	}
	// trip := new(models.Trip)

	// var results []*models.Trip

	// params := []bson.M{
	// 	{
	// 		"$match": bson.M{
	// 			"$and": []bson.M{
	// 				bson.M{
	// 					// "_id": "T1HAZSw5G",
	// 					"cities": bson.M{
	// 						"$exists": true,
	// 						"$not": bson.M{
	// 							"$size": 0,
	// 						},
	// 					},
	// 				},
	// 			},
	// 		},
	// 	},
	// 	{
	// 		"$project": bson.M{
	// 			"_id":    1,
	// 			"cities": 1,
	// 		},
	// 	},
	// }

	// opts, err := trip.GetCollection().Aggregate(context.TODO(), params)
	// if err != nil {
	// 	// log.Error(err)
	// 	return nil, err
	// }
	// if err = opts.All(context.TODO(), &results); err != nil {
	// 	// log.Error(err)
	// 	return nil, err
	// }

	// log.Info("results", results)

	// type CityIdType struct {
	// 	Id        string
	// 	EntryTime int64
	// }

	// for _, v := range results {
	// 	for _, sv := range v.Cities {

	// 		log.Info(sv.Id, sv.CityId)

	// 		updateResult, err := trip.GetCollection().UpdateOne(context.TODO(),
	// 			bson.M{
	// 				"$and": []bson.M{
	// 					{
	// 						"_id":        v.Id,
	// 						"cities._id": sv.Id,
	// 					},
	// 				},
	// 			}, bson.M{
	// 				"$set": bson.M{
	// 					"cities.$.cityId": sv.Id,
	// 					"cities.$._id":    "",
	// 				},
	// 			}, options.Update().SetUpsert(false))

	// 		log.Info(updateResult, err)

	// 	}
	// 	// cities = append(cities, v.Cities)
	// }

	return nil, nil
}

type OsmInfo struct {
	OsmType string
	OsmId   int64
	Names   map[string]string
	// Latlng  *models.CityCoords
}

func (t *CityDbx) GetOsmInfo(fullName string) (*OsmInfo, error) {

	result := new(OsmInfo)
	key := conf.Redisdb.GetKey("GetOsmInfo")
	err := conf.Redisdb.GetStruct(key.GetKey(fullName), result)

	// log.Info(err, result, fullName)
	if err != nil {
		resp, err := conf.RestyClient.R().SetQueryParams(map[string]string{
			"q":      fullName,
			"format": "jsonv2",
		}).
			Get(
				conf.Config.NominatimApiUrl + "/search",
				// "https://tools.aiiko.club/api/v1/geocode/geo?address=" +
				// 	fullName + "&platform=Amap",
			)
		if err != nil {
			log.Error(err)
			return nil, err
		}

		res := [](map[string]any){}

		if err = json.Unmarshal(resp.Body(), &res); err != nil {
			log.Error("json.Unmarshal => ", err, string(resp.Body()))
			return nil, err
		}
		// log.Info("resp.Body()", resp.String())
		// log.Info("resp.Body()", res[0], len(res) == 0,
		// 	resp.Request.URL)

		if len(res) == 0 {
			return nil, err
		}
		// log.Info(res[0]["addresstype"])

		// j, _ := json.MarshalIndent(res[0], "", "  ")
		// log.Info(string(j))

		result.OsmType = nstrings.ToString(res[0]["osm_type"])
		result.OsmId = nint.ToInt64(res[0]["osm_id"])

		resp, err = conf.RestyClient.R().SetQueryParams(map[string]string{
			"osmtype": strings.ToUpper(result.OsmType[0:1]),
			"osmid":   nstrings.ToString(result.OsmId),
		}).
			Get(
				conf.Config.NominatimApiUrl + "/details",
				// "https://tools.aiiko.club/api/v1/geocode/geo?address=" +
				// 	fullName + "&platform=Amap",
			)
		if err != nil {
			log.Error(err)
			return nil, err
		}

		res2 := map[string]any{}

		// log.Info("resp.Body()", resp.String())
		if err = json.Unmarshal(resp.Body(), &res2); err != nil {
			return nil, err
		}
		// log.Info("resp.Body()", res)

		// log.Info(res2["names"], fullName, result.OsmType, strings.ToUpper(result.OsmType[0:1]))

		// log.Info(res[0]["osm_id"], nint.ToInt64(res[0]["osm_id"]))

		result.Names = map[string]string{}
		for k, v := range res2["names"].(map[string]any) {
			result.Names[k] = nstrings.ToString(v)
		}

		// result.OsmId = nint.ToInt64(res[0]["osm_id"])
	}
	err = conf.Redisdb.SetStruct(key.GetKey(fullName), result, key.GetExpiration())
	if err != nil {
		log.Info(err)
	}

	return result, nil
}

func (t *CityDbx) getFullName(id string, cities []*models.City) string {
	fullCities := t.GetFullCityForCities(id, cities)
	results := narrays.Map(fullCities, func(v *models.City, index int) string {
		return nstrings.StringOr(v.Name.ZhCN, v.Name.En)
	})
	narrays.Reverse(&results)
	return strings.Join(results, ",")
}

type I18nInfo struct {
	CityId  string
	Name    *models.CityName
	OsmInfo *OsmInfo
}

func (t *CityDbx) FortmatNames(city *models.City, osmInfo *OsmInfo) *I18nInfo {
	result := new(I18nInfo)

	result.CityId = city.Id
	result.OsmInfo = osmInfo
	result.Name = &models.CityName{
		Ref: nstrings.StringOr(
			osmInfo.Names["ref"]),
		ShortName: nstrings.StringOr(
			osmInfo.Names["short_name"],
			osmInfo.Names["ref"]),
		ZhCN: nstrings.StringOr(
			osmInfo.Names["name:zh"],
			osmInfo.Names["name:zh-Hans"],
			osmInfo.Names["name"]),
		En: nstrings.StringOr(
			osmInfo.Names["name:en"],
			osmInfo.Names["name"]),
		ZhHans: nstrings.StringOr(
			osmInfo.Names["name:zh"],
			osmInfo.Names["name:zh-Hans"],
			osmInfo.Names["name"]),
		ZhHant: nstrings.StringOr(
			osmInfo.Names["name:zh-Hant"],
			osmInfo.Names["name:zh-Hans"],
			osmInfo.Names["name:zh"],
			osmInfo.Names["name"]),
	}

	return result
}

func (t *CityDbx) CityI18n(city *models.City, cities []*models.City) (*I18nInfo, error) {
	result := new(I18nInfo)

	timestamp := time.Now().Unix()

	if city.Names == nil {
		city.Names = new(models.CityNames)
	}

	// log.Info("CityI18n", city.Id,
	// 	city.Names.CreateTime >= timestamp, len(city.Names.Names))

	if city.Names.CreateTime >= timestamp && len(city.Names.Names) > 0 {
		result = t.FortmatNames(city, &OsmInfo{
			Names: city.Names.Names,
		})
		return result, nil
	}

	fsdb := city.GetFsDB()

	isCacheFetched := false

	if b, err := fsdb.CityNamesCache.Get(city.Id); err == nil {
		isCacheFetched = b.Value()
	}

	// log.Warn("isCacheFetched", isCacheFetched)
	if !isCacheFetched {
		fn := t.getFullName(city.Id, cities)
		// log.Info(fn, city.Id, cities)

		if fn == "中国" {
			fn = "中华人民共和国"
		}

		osmInfo, err := t.GetOsmInfo(fn)
		if err != nil {
			return nil, err
		}
		log.Info("osmInfo", osmInfo, fn)

		if osmInfo == nil {

			osmInfo, err = t.GetOsmInfo(
				nstrings.StringOr(city.Name.ZhCN, city.Name.En))
			if err != nil {
				return nil, err
			}
			err := fsdb.CityNamesCache.Set(city.Id, true, 0)
			if err != nil {
				log.Error(err)
			}
			if osmInfo == nil {
				return result, nil
			}

		}

		result = t.FortmatNames(city, osmInfo)

		if err := t.UpdateCity(city.Id, fn, nil, &models.CityNames{
			Names:      osmInfo.Names,
			CreateTime: timestamp + 3600*24*30*6,
		}); err != nil {
			log.Error(err)
			return result, err
		}
		// log.Info(city.Id, fn, osmInfo, osmInfo.Names, err)

	}

	return result, nil
}

func (t *CityDbx) CitiesI18n(cities []*models.City) ([]*models.City, error) {
	// 1、插入数据
	// log.Info("i18n", len(cities))

	// results := []*I18nInfo{}

	for _, v := range cities {

		// log.Info(v, len(cities))
		result, err := t.CityI18n(v, cities)
		if err != nil {
			return nil, err
		}

		v.Name = result.Name

		// results = append(results, result)
	}

	return cities, nil
}

func (t *CityDbx) FortmatI18nNames(city *models.City, lang string) string {

	cityName := &models.CityName{
		Ref: nstrings.StringOr(
			city.Names.Names["ref"]),
		ShortName: nstrings.StringOr(
			city.Names.Names["short_name"],
			city.Names.Names["ref"]),
		ZhCN: nstrings.StringOr(
			city.Names.Names["name:zh"],
			city.Names.Names["name:zh-Hans"],
			city.Names.Names["name"]),
		En: nstrings.StringOr(
			city.Names.Names["name:en"],
			city.Names.Names["name"]),
		ZhHans: nstrings.StringOr(
			city.Names.Names["name:zh"],
			city.Names.Names["name:zh-Hans"],
			city.Names.Names["name"]),
		ZhHant: nstrings.StringOr(
			city.Names.Names["name:zh-Hant"],
			city.Names.Names["name:zh-Hans"],
			city.Names.Names["name:zh"],
			city.Names.Names["name"]),
	}

	name := cityName.ZhCN
	switch lang {
	case "zh-CN":
		name = cityName.ZhCN
	case "zh-TW":
		name = cityName.ZhCN
	case "en-US":
		name = cityName.En

	}

	return name
}

func (t *CityDbx) GetCityAddresses(cities []*models.City, lang string) *models.CityAddresses {
	// result := new(I18nInfo)

	ca := new(models.CityAddresses)

	for _, v := range cities {
		switch v.Level {
		case 1:
			ca.Country = t.FortmatI18nNames(v, lang)
		case 2:
			ca.State = t.FortmatI18nNames(v, lang)
		case 3:
			ca.Region = t.FortmatI18nNames(v, lang)
		case 4:
			ca.City = t.FortmatI18nNames(v, lang)
		case 5:
			ca.Town = t.FortmatI18nNames(v, lang)
		}
	}

	ca.Address = strings.Join(narrays.Filter([]string{
		ca.State,
		ca.Region,
		ca.City,
		ca.Town,
	}, func(val string, i int) bool {

		return val != ""
	}), "·")

	return ca
}

func (t *CityDbx) InitCityDistricts() {

	log.Info("InitCityDistricts")

	// resp, err := conf.RestyClient.R().SetQueryParams(map[string]string{}).
	// 	Get(
	// 		conf.Config.NominatimApiUrl + "/reverse?format=jsonv2&lat=" +
	// 			// "https://nominatim.aiiko.club/reverse?format=jsonv2&lat=" +
	// 			nstrings.ToString(lat) + "&lon=" +
	// 			nstrings.ToString(lng) + "&zoom=" +
	// 			nstrings.ToString(zoom) + "&addressdetails=1&accept-language=zh-CN",
	// 	)
	// if err != nil {
	// 	return nil, err
	// }

	// https://tools.aiiko.club/api/v1/geocode/cityDistricts?country=China
}

// func (t *CityDbx) GetUserAllCities(authorId string) (cities *map[string]int64, err error) {
// 	trip := new(models.Trip)

// 	var results []*models.Trip

// 	params := []bson.M{
// 		{
// 			"$match": bson.M{
// 				"$and": []bson.M{
// 					{
// 						"authorId": authorId,
// 						"status": bson.M{
// 							"$in": []int64{1, 0},
// 						},
// 						"cities": bson.M{
// 							"$exists": true,
// 							"$not": bson.M{
// 								"$size": 0,
// 							},
// 						},
// 					},
// 				},
// 			},
// 		}, {
// 			"$sort": bson.M{
// 				"status":     1,
// 				"createTime": -1,
// 			},
// 		},
// 		// {
// 		// 	"$skip": pageSize * (pageNum - 1),
// 		// },
// 		// {
// 		// 	"$limit": pageSize,
// 		// },
// 		{
// 			"$project": bson.M{
// 				"_id":    1,
// 				"cities": 1,
// 			},
// 		},
// 	}

// 	opts, err := trip.GetCollection().Aggregate(context.TODO(), params)
// 	if err != nil {
// 		// log.Error(err)
// 		return nil, err
// 	}
// 	if err = opts.All(context.TODO(), &results); err != nil {
// 		// log.Error(err)
// 		return nil, err
// 	}

// 	log.Info("results", results)

// 	type CityIdType struct {
// 		CityId    string
// 		EntryTime int64
// 	}

// 	cityIds := []*CityIdType{}

// 	for _, v := range results {
// 		for _, sv := range v.Cities {

// 			cityIds = append(cityIds, &CityIdType{
// 				CityId: sv.CityId,
// 				EntryTime: nmath.Min(narrays.Map(sv.EntryTimes, func(v *models.TripCityEntryTimeItem, index int) int64 {
// 					return v.Timestamp
// 				})...),
// 			})
// 		}
// 		// cities = append(cities, v.Cities)
// 	}
// 	log.Info("cityIds", cityIds)

// 	cityIdsMap := map[string]int64{}

// 	for _, v := range cityIds {

// 		if cityIdsMap[v.CityId] == 0 || cityIdsMap[v.CityId] > v.EntryTime {
// 			cityIdsMap[v.CityId] = v.EntryTime
// 		}
// 	}
// 	log.Info("cityIdsMap", cityIdsMap)

// 	return &cityIdsMap, nil
// }
