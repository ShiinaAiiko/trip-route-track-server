package controllersV1

import (
	dbxV1 "github.com/ShiinaAiiko/nyanya-trip-route-track/server/dbx/v1"
	"github.com/ShiinaAiiko/nyanya-trip-route-track/server/models"
	"github.com/ShiinaAiiko/nyanya-trip-route-track/server/protos"
	"github.com/ShiinaAiiko/nyanya-trip-route-track/server/services/response"
	"github.com/cherrai/nyanyago-utils/narrays"
	"github.com/cherrai/nyanyago-utils/validation"
	"github.com/gin-gonic/gin"
)

// "github.com/cherrai/nyanyago-utils/validation"

var (
	roadDbx = dbxV1.RoadDbx{}
)

type RoadController struct {
}

// func (cl *CityController) GetFullName(cities []*models.City, city *protos.CityItem, lang string) string {

// 	if city.ParentCityId != "" {

// 	}

// 	return ""
// }

func (cl *RoadController) UpdateRoad(c *gin.Context) {
	// 1、请求体
	var res response.ResponseProtobufType
	res.Code = 200

	// 2、获取参数
	data := new(protos.UpdateRoad_Request)
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
		validation.Parameter(&data.Roads, validation.Required()),
		validation.Parameter(&data.EntryTime, validation.Type("int64"), validation.Required()),
	); err != nil {
		res.Errors(err)
		res.Code = 10002
		res.Call(c)
		return
	}

	if len(data.Roads) == 0 {
		res.Errors(err)
		res.Code = 10002
		res.Call(c)
		return
	}

	for _, road := range data.Roads {
		if err = validation.ValidateStruct(
			road,
			validation.Parameter(&road.Type, validation.Type("string"), validation.Required()),
			validation.Parameter(&road.Name, validation.Required()),
			validation.Parameter(&road.ShortCityName, validation.Type("string"), validation.Required()),
		); err != nil {
			res.Errors(err)
			res.Code = 10002
			res.Call(c)
			return
		}

		if road.Code == "" &&
			road.Name.En == "" &&
			road.Name.ZhHans == "" &&
			road.Name.ZhHant == "" {
			res.Errors(err)
			res.Code = 10002
			res.Call(c)
			return
		}
	}

	trip, err := tripDbx.GetTripById(data.TripId)
	if err != nil {
		res.Errors(err)
		res.Code = 10006
		res.Call(c)
		return
	}

	// log.Info("trip.Cities", trip.Cities)

	roads := []*models.TripRoadInfo{}

	for _, road := range data.Roads {
		roadInfo := &models.TripRoadInfo{
			Type: road.Type,
			Code: road.Code,
			Name: &models.TypeRoadName{
				En:     road.Name.En,
				ZhHans: road.Name.ZhHans,
				ZhHant: road.Name.ZhHant,
			},
			ShortCityName: road.ShortCityName,
		}

		roads = append(roads, roadInfo)

	}

	latestRoad := new(models.TripRoad)
	latestTimestamp := int64(0)

	narrays.ForEach(trip.Roads, func(v *models.TripRoad, i int) {
		narrays.ForEach(v.EntryTimes, func(sv *models.TripCityEntryTimeItem, index int) {
			// log.Info("sv.Timestamp > latestTimestamp", v.Id, len(v.EntryTimes), sv.Timestamp > latestTimestamp, sv.Timestamp, latestTimestamp)
			if sv.Timestamp > latestTimestamp {
				latestTimestamp = sv.Timestamp
				latestRoad = v
			}
		})
	})

	// log.Info("latestCity", latestCity, latestCity.CityId != city.Id)

	if roadDbx.GetRoadId(roads) != roadDbx.GetRoadId(latestRoad.Roads) {
		// log.Error("latestCity进入了", latestCity, latestCity.CityId, city.Id)

		isExits := false

		narrays.Some(trip.Roads, func(v *models.TripRoad, i int) bool {
			if roadDbx.GetRoadId(v.Roads) == roadDbx.GetRoadId(roads) {
				isExits = true
				v.EntryTimes = append(v.EntryTimes, &models.TripCityEntryTimeItem{
					Timestamp: data.EntryTime,
				})
				return true
			}
			return false
		})

		if !isExits {
			trip.Roads = append(trip.Roads, &models.TripRoad{
				Roads: roads,
				EntryTimes: []*models.TripCityEntryTimeItem{
					{
						Timestamp: data.EntryTime,
					},
				},
			})
		}

		for _, v := range trip.Roads {

			v.EntryTimes = narrays.StructDeduplication(v.EntryTimes, func(a *models.TripCityEntryTimeItem, b *models.TripCityEntryTimeItem) bool {
				return a.Timestamp == b.Timestamp
			})

		}

		if err := tripDbx.UpdateTripRoads(
			trip.Id, trip.AuthorId, trip.Roads,
		); err != nil {
			res.Errors(err)
			res.Code = 10011
			res.Call(c)
			return
		}
	}

	protoData := &protos.UpdateRoad_Response{
		EntryTime: data.EntryTime,
	}

	res.Data = protos.Encode(protoData)

	res.Call(c)
}
