package controllersV1

import (
	"sort"
	"strings"
	"time"

	dbxV1 "github.com/ShiinaAiiko/nyanya-trip-route-track/server/dbx/v1"
	"github.com/ShiinaAiiko/nyanya-trip-route-track/server/models"
	"github.com/ShiinaAiiko/nyanya-trip-route-track/server/protos"
	"github.com/ShiinaAiiko/nyanya-trip-route-track/server/services/middleware"
	"github.com/ShiinaAiiko/nyanya-trip-route-track/server/services/response"
	"github.com/cherrai/nyanyago-utils/cipher"
	"github.com/cherrai/nyanyago-utils/narrays"
	"github.com/cherrai/nyanyago-utils/nmath"
	"github.com/cherrai/nyanyago-utils/nstrings"
	"github.com/cherrai/nyanyago-utils/validation"
	sso "github.com/cherrai/saki-sso-go"
	"github.com/gin-gonic/gin"
	"github.com/jinzhu/copier"
)

// "github.com/cherrai/nyanyago-utils/validation"

var (
	cityDbx = dbxV1.CityDbx{}
)

type CityController struct {
}

// func (cl *CityController) GetFullName(cities []*models.City, city *protos.CityItem, lang string) string {

// 	if city.ParentCityId != "" {

// 	}

// 	return ""
// }

func (cl *CityController) UpdateCity(c *gin.Context) {
	// 1、请求体
	var res response.ResponseProtobufType
	res.Code = 200

	// 2、获取参数
	data := new(protos.UpdateCity_Request)
	var err error
	if err = protos.DecodeBase64(c.GetString("data"), data); err != nil {
		res.Error = err.Error()
		res.Code = 10002
		res.Call(c)
		return
	}

	// log.Info("data", data)

	// 3、验证参数
	if err = validation.ValidateStruct(
		data,
		validation.Parameter(&data.TripId, validation.Type("string"), validation.Required()),
		validation.Parameter(&data.City, validation.Required()),
		validation.Parameter(&data.EntryTime, validation.Type("int64"), validation.Required()),
	); err != nil {
		res.Errors(err)
		res.Code = 10002
		res.Call(c)
		return
	}
	if err = validation.ValidateStruct(
		data.City,
		validation.Parameter(&data.City.Address, validation.Type("string"), validation.Required()),
	); err != nil {
		res.Errors(err)
		res.Code = 10002
		res.Call(c)
		return
	}

	// log.Info("AddAndGetFullCity", data)

	city, err := cityDbx.AddAndGetFullCity(data.City)

	log.Info("AddAndGetFullCity", city, err)
	if err != nil {
		res.Errors(err)
		res.Code = 10001
		res.Call(c)
		return
	}

	// log.Info(city)

	if data.TripId != "" {
		trip, err := tripDbx.GetTripById(data.TripId)
		if err != nil {
			res.Errors(err)
			res.Code = 10006
			res.Call(c)
			return
		}

		// log.Info("trip.Cities", trip.Cities)

		latestCity := new(models.TripCity)
		latestTimestamp := int64(0)

		narrays.ForEach(trip.Cities, func(v *models.TripCity, i int) {
			narrays.ForEach(v.EntryTimes, func(sv *models.TripCityEntryTimeItem, index int) {
				// log.Info("sv.Timestamp > latestTimestamp", v.Id, len(v.EntryTimes), sv.Timestamp > latestTimestamp, sv.Timestamp, latestTimestamp)
				if sv.Timestamp > latestTimestamp {
					latestTimestamp = sv.Timestamp
					latestCity = v
				}
			})
		})

		// log.Info("latestCity", latestCity, latestCity.CityId != city.Id)

		if latestCity.CityId != city.Id {
			// log.Error("latestCity进入了", latestCity, latestCity.CityId, city.Id)

			isExits := false

			narrays.Some(trip.Cities, func(v *models.TripCity, i int) bool {
				if v.CityId == city.Id {
					isExits = true
					v.EntryTimes = append(v.EntryTimes, &models.TripCityEntryTimeItem{
						Timestamp: data.EntryTime,
					})
					return true
				}
				return false
			})

			if !isExits {
				trip.Cities = append(trip.Cities, &models.TripCity{
					CityId: city.Id,
					EntryTimes: []*models.TripCityEntryTimeItem{
						{
							Timestamp: data.EntryTime,
						},
					},
				})
			}

			// log.Info(
			// 	trip.Id, trip.AuthorId, trip.Cities)

			// trip.Cities
			for _, v := range trip.Cities {

				v.EntryTimes = narrays.StructDeduplication(v.EntryTimes, func(a *models.TripCityEntryTimeItem, b *models.TripCityEntryTimeItem) bool {
					return a.Timestamp == b.Timestamp
				})

			}

			if err := tripDbx.UpdateTripCities(
				trip.Id, trip.AuthorId, trip.Cities,
			); err != nil {
				res.Errors(err)
				res.Code = 10011
				res.Call(c)
				return
			}
		}

	}

	protoData := &protos.UpdateCity_Response{
		Id:        city.Id,
		EntryTime: data.EntryTime,
	}

	res.Data = protos.Encode(protoData)

	res.Call(c)
}

func (cl *CityController) GetCityDetails(c *gin.Context) {
	// 1、请求体
	var res response.ResponseProtobufType
	res.Code = 200

	// 2、获取参数
	data := new(protos.GetCityDetails_Request)
	var err error
	if err = protos.DecodeBase64(c.GetString("data"), data); err != nil {
		res.Error = err.Error()
		res.Code = 10002
		res.Call(c)
		return
	}

	// log.Info("data", data)

	// 3、验证参数
	if err = validation.ValidateStruct(
		data,
		validation.Parameter(&data.Ids, validation.Required()),
		validation.Parameter(&data.TripId, validation.Type("string")),
	); err != nil {
		res.Errors(err)
		res.Code = 10002
		res.Call(c)
		return
	}

	authorId := ""
	// 校验身份
	code := middleware.CheckAuthorize(c)
	// log.Info("code", data.Id, code)
	if code == 200 {
		userInfoAny, exists := c.Get("userInfo")
		if !exists {
			res.Errors(err)
			res.Code = 10004
			res.Call(c)
			return
		}
		authorId = userInfoAny.(*sso.UserInfo).Uid
	}

	if authorId == "" {
		if data.TripId != "" {
			trip, err := tripDbx.GetTrip(data.TripId, "")
			// log.Info("getTrip, err ", getTrip, err)
			if err != nil || trip == nil {
				res.Errors(err)
				res.Code = 10004
				res.Call(c)
				return
			}

			if !trip.Permissions.AllowShare {
				res.Errors(err)
				res.Code = 10004
				res.Call(c)
				return
			}

			authorId = trip.AuthorId
		}

		if authorId == "" {
			res.Errors(err)
			res.Code = 10004
			res.Call(c)
			return
		}
	}
	log.Error(data, authorId)

	cities, err := cityDbx.GetCities(data.Ids)
	if err != nil {
		res.Errors(err)
		res.Code = 10006
		res.Call(c)
		return
	}

	protoData := &protos.GetCityDetails_Response{
		Cities: []*protos.CityItem{},
	}

	ids := []string{}
	for _, city := range cities {
		if narrays.Includes(ids, city.Id) {
			continue
		}
		cityProto := new(protos.CityItem)
		copier.Copy(cityProto, city)

		// cityNameProto := new(protos.CityItem_CityName)
		// copier.Copy(cityNameProto, city.Name)
		// cityProto.Name = cityNameProto

		// log.Info(city.Name)

		// cl.GetFullName(cities, cityProto)

		ids = append(ids, cityProto.Id)
		protoData.Cities = append(protoData.Cities, cityProto)
	}

	protoData.Total = int32(len(protoData.Cities))

	res.Data = protos.Encode(protoData)

	res.Call(c)
}

func (cl *CityController) GetAllCitiesVisitedByUser(c *gin.Context) {
	// 1、请求体
	var res response.ResponseProtobufType
	res.Code = 200

	// 2、获取参数
	data := new(protos.GetAllCitiesVisitedByUser_Request)
	var err error
	if err = protos.DecodeBase64(c.GetString("data"), data); err != nil {
		res.Error = err.Error()
		res.Code = 10002
		res.Call(c)
		return
	}

	// log.Info("data", data)

	// 3、验证参数
	if err = validation.ValidateStruct(
		data,
		validation.Parameter(&data.TripIds),
		validation.Parameter(&data.JmId, validation.Type("string")),
		validation.Parameter(&data.TripId, validation.Type("string")),
		// validation.Parameter(&data.JmShareKey, validation.Type("string")),
	); err != nil {
		res.Errors(err)
		res.Code = 10002
		res.Call(c)
		return
	}

	// log.Info(data)

	authorId := ""
	// 校验身份
	code := middleware.CheckAuthorize(c)
	// log.Info("code", data.Id, code)
	if code == 200 {
		userInfoAny, exists := c.Get("userInfo")
		if !exists {
			res.Errors(err)
			res.Code = 10004
			res.Call(c)
			return
		}
		authorId = userInfoAny.(*sso.UserInfo).Uid
	}

	if authorId == "" {
		if data.JmId != "" {
			jm, err := jmDbx.GetJM(
				data.JmId, "",
			)
			if err != nil || jm == nil {
				res.Errors(err)
				res.Code = 10004
				res.Call(c)
				return
			}
			if !jm.Permissions.AllowShare {
				res.Errors(err)
				res.Code = 10004
				res.Call(c)
				return
			}

			authorId = jm.AuthorId
		}

		if data.TripId != "" {
			trip, err := tripDbx.GetTrip(data.TripId, "")
			// log.Info("getTrip, err ", getTrip, err)
			if err != nil || trip == nil {
				res.Errors(err)
				res.Code = 10004
				res.Call(c)
				return
			}

			if !trip.Permissions.AllowShare {
				res.Errors(err)
				res.Code = 10004
				res.Call(c)
				return
			}

			authorId = trip.AuthorId
		}

		if authorId == "" {
			res.Errors(err)
			res.Code = 10004
			res.Call(c)
			return
		}
	}

	// log.Info("data.JmShareKey", data.JmShareKey)
	// authorId := "78L2tkleM"

	protoData := &protos.GetAllCitiesVisitedByUser_Response{
		Cities: []*protos.CityItem{},
	}

	fsdb := new(models.City).GetFsDB()

	k := cipher.MD5(authorId + strings.Join(data.TripIds, ","))

	lt := log.Time()
	results, err := fsdb.CitiesProto.Get(k)
	// log.Info(err, len(data.TripIds), k, fsdb.CitiesProto.Keys(), narrays.Filter(fsdb.CitiesProto.Keys(), func(v string, i int) bool {
	// 	return v == k
	// }))

	// for _, v := range fsdb.CitiesProto.Keys() {
	// 	fsdb.CitiesProto.Delete(v)
	// }
	if err == nil {

		tempResult := results.Value()
		if len(tempResult) > 0 {
			protoData.Cities = tempResult
		}
	}

	lt.TimeEnd("第一步、" + nstrings.ToString(len(data.TripIds)) + " : " +
		nstrings.ToString(len(protoData.Cities)))

	// 当data.TripIds为空，则视为全部获取

	protoData.Cities = protoData.Cities[:0]
	if len(protoData.Cities) == 0 {

		lt2 := log.Time()
		cityIds, err := cityDbx.GetAllCitiesVisitedByUser(authorId, data.TripIds)
		if err != nil {
			res.Errors(err)
			res.Code = 10006
			res.Call(c)
			return
		}

		lt2.TimeEnd("2：" + nstrings.ToString(len(data.TripIds)))
		log.Info(len(cityIds))

		cityIdsMap := map[string]*dbxV1.UserVisitedCities{}
		ids := narrays.Map(cityIds, func(v *dbxV1.UserVisitedCities, index int) string {
			cityIdsMap[v.CityId] = v
			return v.CityId
		})

		lt3 := log.Time()

		cities, err := cityDbx.GetCities(ids)
		// log.Info("ids", len(ids), len(cities), err, cities[0].Id, cities[0].Name)
		if err != nil {
			res.Errors(err)
			res.Code = 10006
			res.Call(c)
			return
		}

		sort.SliceStable(cities, func(i, j int) bool {
			return cities[i].Level-cities[j].Level > 0
		})

		citiesMap := map[string]*protos.CityItem{}

		citiesProto := []*protos.CityItem{}

		log.Info("ids", len(citiesProto))
		for _, city := range cities {
			cityProto := new(protos.CityItem)
			copier.Copy(cityProto, city)

			// cityNameProto := new(protos.CityItem_CityName)
			// copier.Copy(cityNameProto, city.Name)
			// cityProto.Name = cityNameProto

			if cityIdsMap[city.Id] != nil {
				// 镇
				// log.Info(city.Name.ZhCN, cityIdsMap[city.Id].FirstEntryTime, cityIdsMap[city.Id].EntryCount)
				cityProto.FirstEntryTime = cityIdsMap[city.Id].FirstEntryTime
				cityProto.LastEntryTime = cityIdsMap[city.Id].LastEntryTime
				cityProto.EntryCount = cityIdsMap[city.Id].EntryCount
			}

			// log.Info(city.Name)

			// cl.GetFullName(cities, cityProto)
			citiesMap[city.Id] = cityProto
			citiesProto = append(citiesProto, cityProto)

		}

		parentCitiesMap := map[string]([]*protos.CityItem){}

		var getSubCities func(id *protos.CityItem, ids []*protos.CityItem) []*protos.CityItem

		getSubCities = func(city *protos.CityItem, parentCities []*protos.CityItem) []*protos.CityItem {
			// pCity := citiesMap[id]
			cities := []*protos.CityItem{}
			cities = append(cities, narrays.Filter(citiesProto, func(value *protos.CityItem, index int) bool {
				if value.ParentCityId == city.Id {
					newParentCities := append(parentCities, city)
					value.Cities = getSubCities(value, newParentCities)

					if cityIdsMap[value.Id] != nil {
						// value.Cities = newParentCities

						for _, v := range newParentCities {
							if v.FirstEntryTime == 0 {
								v.FirstEntryTime = 999999999999
							}
							v.FirstEntryTime = nmath.Min(v.FirstEntryTime, value.FirstEntryTime)
							v.LastEntryTime = nmath.Max(v.LastEntryTime, value.LastEntryTime)
							v.EntryCount += 1
						}

						parentCitiesMap[value.Id] = newParentCities

						// log.Info(value.Name.ZhCN, value.Level, value.FirstEntryTime, len(newParentCities))
					}

					return true
				}
				return false
			})...)
			return cities
		}

		// log.Info("ids", len(citiesProto), citiesProto[0])
		for _, city := range citiesProto {
			if city.ParentCityId == "" {
				// log.Info(city.ParentCityId == "", city.Name)
				city.Cities = getSubCities(city, []*protos.CityItem{})
				protoData.Cities = append(protoData.Cities, city)
				break
			}
		}

		if err = fsdb.CitiesProto.Set(k, protoData.Cities, 1*time.Hour); err != nil {
			log.Error(err)
		}
		lt3.TimeEnd("3" + k)
	}

	protoData.Total = int32(len(protoData.Cities))

	res.Data = protos.Encode(protoData)

	res.Call(c)
}

func (cl *CityController) GetCitiesByOpenAPI(c *gin.Context) {
	// 1、请求体
	var res response.ResponseProtobufType
	res.Code = 200

	// 2、获取参数
	data := new(protos.GetCitiesByOpenAPI_Request)
	var err error
	if err = protos.DecodeBase64(c.GetString("data"), data); err != nil {
		res.Error = err.Error()
		res.Code = 10002
		res.Call(c)
		return
	}

	// log.Info("data", data)

	// 3、验证参数
	if err = validation.ValidateStruct(
		data,
		// validation.Parameter(&data.Ids, validation.Required()),
	); err != nil {
		res.Errors(err)
		res.Code = 10002
		res.Call(c)
		return
	}

	// log.Info(data)

	// userInfoAny, exists := c.Get("userInfo")
	// if !exists {
	// 	res.Errors(err)
	// 	res.Code = 10004
	// 	res.Call(c)
	// 	return
	// }
	// authorId := userInfoAny.(*sso.UserInfo).Uid

	authorId := c.GetString("openUid")

	// log.Info("authorId", authorId)
	if authorId == "" {
		res.Errors(err)
		res.Code = 10004
		res.Call(c)
		return
	}

	cityIds, err := cityDbx.GetAllCitiesVisitedByUser(authorId, []string{})
	if err != nil {
		res.Errors(err)
		res.Code = 10006
		res.Call(c)
		return
	}

	cityIdsMap := map[string]*dbxV1.UserVisitedCities{}
	ids := narrays.Map(cityIds, func(v *dbxV1.UserVisitedCities, index int) string {
		cityIdsMap[v.CityId] = v
		return v.CityId
	})

	// log.Info("ids", len(ids))

	cities, err := cityDbx.GetCities(ids)
	if err != nil {
		res.Errors(err)
		res.Code = 10006
		res.Call(c)
		return
	}

	sort.SliceStable(cities, func(i, j int) bool {
		return cities[i].Level-cities[j].Level > 0
	})

	citiesProto := []*protos.CityItem{}

	for _, city := range cities {
		cityProto := new(protos.CityItem)
		copier.Copy(cityProto, city)

		// cityNameProto := new(protos.CityItem_CityName)
		// copier.Copy(cityNameProto, city.Name)
		// cityProto.Name = cityNameProto

		if cityIdsMap[city.Id] != nil {
			// 镇
			// log.Info(city.Name.ZhCN, cityIdsMap[city.Id].FirstEntryTime, cityIdsMap[city.Id].EntryCount)
			cityProto.FirstEntryTime = cityIdsMap[city.Id].FirstEntryTime
			cityProto.LastEntryTime = cityIdsMap[city.Id].LastEntryTime
			cityProto.EntryCount = cityIdsMap[city.Id].EntryCount
		}

		// log.Info(city.Name)

		// cl.GetFullName(cities, cityProto)
		citiesProto = append(citiesProto, cityProto)

	}

	protoData := &protos.GetCitiesByOpenAPI_Response{
		Cities: citiesProto,
	}
	protoData.Total = int32(len(citiesProto))

	res.JSON(c, res.ProtoToMap(protoData))
}
