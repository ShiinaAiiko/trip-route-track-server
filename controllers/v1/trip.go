package controllersV1

import (
	"errors"
	"fmt"
	"strconv"
	"time"

	dbxv1 "github.com/ShiinaAiiko/nyanya-trip-route-track/server/dbx/v1"
	"github.com/ShiinaAiiko/nyanya-trip-route-track/server/models"
	"github.com/ShiinaAiiko/nyanya-trip-route-track/server/protos"
	"github.com/ShiinaAiiko/nyanya-trip-route-track/server/services/methods"
	"github.com/ShiinaAiiko/nyanya-trip-route-track/server/services/middleware"
	"github.com/ShiinaAiiko/nyanya-trip-route-track/server/services/response"
	"github.com/cherrai/nyanyago-utils/narrays"
	"github.com/cherrai/nyanyago-utils/nint"
	"github.com/cherrai/nyanyago-utils/nlog"
	"github.com/cherrai/nyanyago-utils/nstrings"
	"github.com/cherrai/nyanyago-utils/ntimer"
	"github.com/cherrai/nyanyago-utils/validation"
	sso "github.com/cherrai/saki-sso-go"
	"github.com/jinzhu/copier"

	// "github.com/cherrai/nyanyago-utils/validation"
	"github.com/gin-gonic/gin"
)

var (
	log     = nlog.New()
	tripDbx = dbxv1.TripDbx{}
)

type TripController struct {
}

var vehiclesMap = map[string]*protos.VehicleItem{}

func formartTrip(v *models.Trip) *protos.Trip {
	postions := []*protos.TripPosition{}
	marks := []*protos.TripMark{}
	cities := []*protos.TripCity{}
	roads := []*protos.TripRoad{}
	addresses := []*protos.TripAddresses{}

	for _, v := range v.Addresses {
		addresses = append(addresses, (&protos.TripAddresses{
			Latitude:  v.Latitude,
			Longitude: v.Longitude,
			Altitude:  v.Altitude,
			City: &protos.TripAddressesCity{
				Country: v.City.Country,
				State:   v.City.State,
				Region:  v.City.Region,
				City:    v.City.City,
				Town:    v.City.Town,
				Road:    v.City.Road,
			},
			Address: &protos.TripAddressesAddress{
				FullName: v.Address.FullName,
				Type:     v.Address.Type,
				Name:     v.Address.Name,
			},
			EntryTime: v.EntryTime,
		}))
	}
	for _, v := range v.Positions {
		postions = append(postions, formartPosition(&protos.TripPosition{
			Latitude:         v.Latitude,
			Longitude:        v.Longitude,
			Altitude:         v.Altitude,
			AltitudeAccuracy: v.AltitudeAccuracy,
			Accuracy:         v.Accuracy,
			Heading:          v.Heading,
			Speed:            v.Speed,
			Timestamp:        v.Timestamp,
		}))
	}
	for _, v := range v.Marks {
		marks = append(marks, &protos.TripMark{
			Timestamp: v.Timestamp,
		})
	}

	for _, v := range v.Cities {
		entryTimes := []*protos.TripCity_EntryTimeItem{}
		for _, sv := range v.EntryTimes {
			entryTimes = append(entryTimes, &protos.TripCity_EntryTimeItem{
				Timestamp: sv.Timestamp,
			})
		}

		// log.Info("v.CityId", v.CityId, v)

		cities = append(cities, &protos.TripCity{
			CityId:     v.CityId,
			EntryTimes: entryTimes,
		})
	}

	for _, v := range v.Roads {
		entryTimes := []*protos.TripRoad_EntryTimeItem{}
		for _, sv := range v.EntryTimes {
			entryTimes = append(entryTimes, &protos.TripRoad_EntryTimeItem{
				Timestamp: sv.Timestamp,
			})
		}

		// log.Info("v.CityId", v.CityId, v)

		roadsList := []*protos.RoadInfo{}

		for _, road := range v.Roads {

			roadsList = append(roadsList, &protos.RoadInfo{
				Type: road.Type,
				Code: road.Code,
				Name: &protos.RoadName{
					ZhHans: road.Name.ZhHans,
					ZhHant: road.Name.ZhHant,
					En:     road.Name.En,
				},
				ShortCityName: road.ShortCityName,
			})
		}

		roads = append(roads, &protos.TripRoad{
			Roads:      roadsList,
			EntryTimes: entryTimes,
		})
	}

	// log.Info("roads", roads, v.Roads)

	// log.Info(len( v.Positions), len(postions))

	trip := &protos.Trip{
		Id:        v.Id,
		Name:      v.Name,
		Positions: postions,
		Addresses: addresses,
		Marks:     marks,
		Cities:    cities,
		Roads:     roads,
		Type:      v.Type,
		AuthorId:  v.AuthorId,
		// VehicleId: v.VehicleId,

		Status:         v.Status,
		CreateTime:     v.CreateTime,
		StartTime:      v.StartTime,
		EndTime:        v.EndTime,
		LastUpdateTime: v.LastUpdateTime,
	}

	// log.Error("v.Statistics ", v.Statistics)

	if v.Statistics != nil {
		trip.Statistics = &protos.TripStatistics{
			Distance:        v.Statistics.Distance,
			MaxSpeed:        v.Statistics.MaxSpeed,
			MaxAltitude:     v.Statistics.MaxAltitude,
			MinAltitude:     v.Statistics.MinAltitude,
			AverageSpeed:    v.Statistics.AverageSpeed,
			ClimbAltitude:   v.Statistics.ClimbAltitude,
			DescendAltitude: v.Statistics.DescendAltitude,
		}
	}

	if v.VehicleId != "" {
		vi := new(protos.VehicleItem)

		if vehiclesMap[v.VehicleId] != nil {
			vi = vehiclesMap[v.VehicleId]
		} else {
			if vehicle, err := vehicleDbx.GetVehicle(v.VehicleId, "", []int64{1}); vehicle != nil && err == nil {
				copier.Copy(vi, vehicle)
				vehiclesMap[v.VehicleId] = vi
				ntimer.SetTimeout(func() {
					delete(vehiclesMap, v.VehicleId)
				}, 5*1000)
			}
		}
		trip.Vehicle = vi
	}

	if v.Permissions != nil {
		trip.Permissions = &protos.TripPermissions{
			AllowShare: v.Permissions.AllowShare,
			CustomTrip: v.Permissions.CustomTrip,
		}
	}

	return trip
}

func formartPosition(v *protos.TripPosition) *protos.TripPosition {
	heading, err := strconv.ParseFloat(fmt.Sprintf("%.3f", v.Heading), 64)
	if err != nil {
		log.Error(err)
	} else {
		v.Heading = heading
	}
	speed, err := strconv.ParseFloat(fmt.Sprintf("%.3f", v.Speed), 64)
	if err != nil {
		log.Error(err)
	} else {
		v.Speed = speed
	}
	altitude, err := strconv.ParseFloat(fmt.Sprintf("%.3f", v.Altitude), 64)
	if err != nil {
		log.Error(err)
	} else {
		v.Altitude = altitude
	}
	accuracy, err := strconv.ParseFloat(fmt.Sprintf("%.2f", v.Accuracy), 64)
	if err != nil {
		log.Error(err)
	} else {
		v.Accuracy = accuracy
	}
	altitudeAccuracy, err := strconv.ParseFloat(fmt.Sprintf("%.2f", v.AltitudeAccuracy), 64)
	if err != nil {
		log.Error(err)
	} else {
		v.AltitudeAccuracy = altitudeAccuracy
	}

	return v
}

func (fc *TripController) AddTrip(c *gin.Context) {
	// 1、请求体
	var res response.ResponseProtobufType
	res.Code = 200

	// 2、获取参数
	data := new(protos.AddTrip_Request)
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
		validation.Parameter(&data.Type, validation.Required(), validation.Enum([]string{
			"Running",
			"Bike",
			"Drive",
			"Motorcycle",
			"Walking",
			"PowerWalking",
			"Train",
			"PublicTransport",
			"Plane"})),
		// validation.Parameter(&data.StartTime, validation.Required()),
	); err != nil {
		res.Errors(err)
		res.Code = 10002
		res.Call(c)
		return
	}

	if data.CustomTrip {
		if err = validation.ValidateStruct(
			data,
			validation.Parameter(&data.StartTime, validation.Required()),
			validation.Parameter(&data.EndTime, validation.Required()),
		); err != nil {
			res.Errors(err)
			res.Code = 10002
			res.Call(c)
			return
		}
	}

	userInfoAny, exists := c.Get("userInfo")
	if !exists {
		res.Errors(err)
		res.Code = 10004
		res.Call(c)
		return
	}
	userInfo := userInfoAny.(*sso.UserInfo)

	if data.VehicleId != "" {
		vehicle, err := vehicleDbx.GetVehicle(data.VehicleId, userInfo.Uid, []int64{1})
		if vehicle == nil || err != nil {
			res.Errors(err)
			res.Code = 10018
			res.Call(c)
			return
		}
	}

	// log.Info("userInfo", userInfo)

	trip := &models.Trip{
		Type: data.Type,
		// Postions: []*models.TripPostion{},
		// Statistics: &models.TripStatistics{
		// 	// TotalDistance: data.Statistics.TotalDistance,
		// 	// MaxSpeed:      data.Statistics.MaxSpeed,
		// 	// AverageSpeed:  data.Statistics.AverageSpeed,
		// 	// MaxAltitude:   data.Statistics.MaxAltitude,
		// },
		// Postions: []*models.TripPostion{},
		VehicleId: data.VehicleId,
		AuthorId:  userInfo.Uid,
		Status:    0,
		StartTime: time.Now().Unix(),
		// EndTime:   data.EndTime,
	}

	if data.CustomTrip {
		trip.Permissions = &models.TripPermissions{
			CustomTrip: data.CustomTrip,
		}
		trip.StartTime = data.StartTime
		trip.EndTime = data.EndTime
	}

	addTrip, err := tripDbx.AddTrip(trip)
	if err != nil {
		res.Errors(err)
		res.Code = 10016
		res.Call(c)
		return
	}
	// log.Info(addTrip)

	// authorId := c.MustGet("userInfo").(*sso.UserInfo).Uid

	protoData := &protos.AddTrip_Response{
		Trip: formartTrip(addTrip),
	}

	res.Data = protos.Encode(protoData)

	res.Call(c)
}

func (fc *TripController) AddTripMark(c *gin.Context) {
	// 1、请求体
	var res response.ResponseProtobufType
	res.Code = 200

	// 2、获取参数
	data := new(protos.AddTripMark_Request)
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
		validation.Parameter(&data.Id, validation.Type("string"), validation.Required()),
	); err != nil {
		res.Errors(err)
		res.Code = 10002
		res.Call(c)
		return
	}
	if err = validation.ValidateStruct(
		data.Mark,
		validation.Parameter(&data.Mark.Timestamp, validation.Type("int64"), validation.Required()),
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

	err = tripDbx.AddTripMark(
		data.Id, userInfo.Uid, &models.TripMark{
			Timestamp: data.Mark.Timestamp,
		},
	)
	if err != nil {
		res.Errors(err)
		res.Code = 10016
		res.Call(c)
		return
	}

	protoData := &protos.AddTripMark_Response{}

	res.Data = protos.Encode(protoData)

	res.Call(c)
}

func (fc *TripController) AddTripToOnline(c *gin.Context) {
	// 1、请求体
	var res response.ResponseProtobufType
	res.Code = 200

	// 2、获取参数
	data := new(protos.AddTripToOnline_Request)
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
		validation.Parameter(&data.Type, validation.Required(), validation.Enum([]string{
			"Running",
			"Bike",
			"Drive",
			"Motorcycle",
			"Walking",
			"PowerWalking",
			"Train",
			"PublicTransport",
			"Plane",
		})),
		validation.Parameter(&data.Positions, validation.Length(1, 100000000), validation.Required()),
		validation.Parameter(&data.StartTime, validation.Greater(int64(0)), validation.Required()),
		validation.Parameter(&data.EndTime, validation.Greater(int64(0)), validation.Required()),
		validation.Parameter(&data.CreateTime, validation.Greater(int64(0)), validation.Required()),
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

	postions := []*models.TripPosition{}
	marks := []*models.TripMark{}

	cities := []*models.TripCity{}
	roads := []*models.TripRoad{}
	networkStatus := narrays.Map(data.NetworkStatus, func(sv *protos.TripNetworkStatus, i int) *models.TripNetworkStatus {
		return &models.TripNetworkStatus{
			Status:    sv.Status,
			Timestamp: sv.Timestamp,
		}
	})
	weather := []*models.TripWeather{}

	for _, v := range data.Positions {
		postions = append(postions, &models.TripPosition{
			Latitude:         v.Latitude,
			Longitude:        v.Longitude,
			Altitude:         v.Altitude,
			AltitudeAccuracy: v.AltitudeAccuracy,
			Accuracy:         v.Accuracy,
			Heading:          v.Heading,
			Speed:            v.Speed,
			Timestamp:        v.Timestamp,
		})
	}
	for _, v := range data.Marks {
		marks = append(marks, &models.TripMark{
			Timestamp: v.Timestamp,
		})
	}
	for _, v := range data.Cities {
		entryTimes := []*models.TripCityEntryTimeItem{}
		for _, sv := range v.EntryTimes {
			entryTimes = append(entryTimes, &models.TripCityEntryTimeItem{
				Timestamp: sv.Timestamp,
			})
		}
		cities = append(cities, &models.TripCity{
			CityId:     v.CityId,
			EntryTimes: entryTimes,
		})
	}
	for _, v := range data.Roads {
		entryTimes := []*models.TripCityEntryTimeItem{}
		for _, sv := range v.EntryTimes {
			entryTimes = append(entryTimes, &models.TripCityEntryTimeItem{
				Timestamp: sv.Timestamp,
			})
		}
		roadsList := []*models.TripRoadInfo{}

		for _, road := range v.Roads {

			roadsList = append(roadsList, &models.TripRoadInfo{
				Type: road.Type,
				Code: road.Code,
				Name: &models.TypeRoadName{
					ZhHans: road.Name.ZhHans,
					ZhHant: road.Name.ZhHant,
					En:     road.Name.En,
				},
				ShortCityName: road.ShortCityName,
			})
		}

		roads = append(roads, &models.TripRoad{
			Roads:      roadsList,
			EntryTimes: entryTimes,
		})
	}
	for _, v := range data.Weather {
		weather = append(weather, &models.TripWeather{
			WeatherCode:         v.WeatherCode,
			Temperature:         v.Temperature,
			ApparentTemperature: v.ApparentTemperature,
			WindSpeed:           v.WindSpeed,
			WindDirection:       v.WindDirection,
			Humidity:            v.Humidity,
			Precipitation:       v.Precipitation,
			Timestamp:           v.Timestamp,
		})
	}
	// log.Info("userInfo", userInfo)

	trip := &models.Trip{
		Type: data.Type,
		// Positions:  postions,
		Marks:         marks,
		Cities:        cities,
		Roads:         roads,
		NetworkStatus: networkStatus,
		Weather:       weather,
		AuthorId:      userInfo.Uid,
		Status:        0,
		CreateTime:    data.CreateTime,
		StartTime:     data.StartTime,
		EndTime:       data.EndTime,
	}
	if data.CustomTrip {
		trip.Permissions = &models.TripPermissions{
			CustomTrip: data.CustomTrip,
		}
	}

	addTrip, err := tripDbx.AddTrip(trip)
	log.Info("addTrip", addTrip, err)
	if err != nil {
		res.Errors(err)
		res.Code = 10016
		res.Call(c)
		return
	}

	if err = tripDbx.UpdateTripPosition(
		userInfo.Uid, addTrip.Id, postions, 500,
	); err != nil {
		res.Errors(err)
		res.Code = 10011
		res.Call(c)
		return
	}

	s, deleteStatus, err := tripDbx.GetTripStatistics(addTrip.Id, addTrip.EndTime, true)

	log.Info("GetTripStatistics", s, deleteStatus, err)
	if err != nil {
		res.Errors(err)
		res.Code = 10011
		res.Call(c)
		return
	}
	if deleteStatus != 0 {
		if deleteStatus == -1 && err != nil {
			res.Errors(err)
			res.Code = 10017
			res.Call(c)
			return
		}
		res.Errors(err)
		res.Code = 10016
		res.Call(c)
		return
	}

	addTrip.Statistics = s

	protoData := &protos.AddTrip_Response{
		Trip: formartTrip(addTrip),
	}

	res.Data = protos.Encode(protoData)

	res.Call(c)
}

func (fc *TripController) UpdateTripPosition(c *gin.Context) {
	// 1、请求体
	var res response.ResponseProtobufType
	res.Code = 200

	// 2、获取参数
	data := new(protos.UpdateTripPosition_Request)
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
		validation.Parameter(&data.Id, validation.Required()),
		validation.Parameter(&data.Positions, validation.Length(1, 100000000), validation.Required()),
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

	postions := []*models.TripPosition{}

	for _, v := range data.Positions {
		postions = append(postions, &models.TripPosition{
			Latitude:         v.Latitude,
			Longitude:        v.Longitude,
			Altitude:         v.Altitude,
			AltitudeAccuracy: v.AltitudeAccuracy,
			Accuracy:         v.Accuracy,
			Heading:          v.Heading,
			Speed:            v.Speed,
			Timestamp:        v.Timestamp,
		})
	}

	err = tripDbx.UpdateTripPosition(
		userInfo.Uid, data.Id, postions, data.Distance,
	)
	if err != nil {
		res.Errors(err)
		res.Code = 10011
		res.Call(c)
		return
	}

	lastPosition := postions[len(postions)-1]

	if data.VehicleId != "" {
		if err = vehicleDbx.UpdateVehiclePosition(
			data.VehicleId, userInfo.Uid, postions[len(postions)-1],
		); err != nil {
			res.Errors(err)
			res.Code = 10011
			res.Call(c)
			return
		}
	}

	if err = positionDbx.UpdateUserPosition(
		userInfo.Uid, lastPosition,
	); err != nil {
		res.Errors(err)
		res.Code = 10011
		res.Call(c)
		return
	}

	protoData := &protos.UpdateTripPosition_Response{}

	res.Data = protos.Encode(protoData)

	res.Call(c)
}

func (fc *TripController) TestDataAndPermanentlyDeleteTrip(c *gin.Context, id string) error {
	getTrip, err := tripDbx.GetTripById(id)
	if err != nil || getTrip == nil {
		return errors.New("content does not exist")
	}
	// 少于10个坐标视为无效数据
	if getTrip.Status == 0 && (len(getTrip.Positions) < 10 || getTrip.Statistics.Distance < 50) {
		if err = tripDbx.PermanentlyDeleteTrip(id); err != nil {
			return err
		}
		return errors.New("deleted successfully")
	}
	return nil
}

func (fc *TripController) FinishTrip(c *gin.Context) {
	// 1、请求体
	var res response.ResponseProtobufType
	res.Code = 200

	// 2、获取参数
	data := new(protos.FinishTrip_Request)
	var err error
	if err = protos.DecodeBase64(c.GetString("data"), data); err != nil {
		res.Error = err.Error()
		res.Code = 10002
		res.Call(c)
		return
	}
	log.Info(data)

	// 3、验证参数
	if err = validation.ValidateStruct(
		data,
		validation.Parameter(&data.Id, validation.Required()),
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

	endTime := data.EndTime
	log.Info("endTime", endTime)

	tripPositions, err := tripDbx.GetTripPositions(data.Id, userInfo.Uid)
	log.Info("getTrip, err ", tripPositions, err)

	if endTime <= 0 {
		if err != nil || tripPositions == nil {
			res.Errors(err)
			res.Code = 10011
			res.Call(c)
			return
		}
		if len(tripPositions.Positions) <= 0 {
			if err = tripDbx.DeleteTrip(
				"", data.Id,
			); err != nil {
				res.Errors(err)
				res.Code = 10017
				res.Call(c)
				return
			}
			protoData := &protos.FinishTrip_Response{
				Deleted: true,
			}
			res.Code = 200
			res.Data = protos.Encode(protoData)

			res.Call(c)
			return
		}

		endTime = tripPositions.Positions[len(tripPositions.Positions)-1].Timestamp / 1000
	}
	log.Info("endTime", endTime)

	s, _, err := tripDbx.GetTripStatistics(data.Id, endTime, false)
	log.Info("GetTripStatistics", s.Distance, data.Id)
	if err != nil {
		res.Errors(err)
		res.Code = 10011
		res.Call(c)
		return
	}
	if s.Distance < 50 {
		if err = tripDbx.DeleteTrip(
			"", data.Id,
		); err != nil {
			res.Errors(err)
			res.Code = 10017
			res.Call(c)
			return
		}
		// if err := fc.TestDataAndPermanentlyDeleteTrip(c, data.Id); err != nil {
		// 	if err.Error() != "deleted successfully" {
		// 		res.Errors(err)
		// 		res.Code = 10017
		// 		res.Call(c)
		// 		return
		// 	}
		// }
		protoData := &protos.FinishTrip_Response{
			Deleted: true,
		}
		res.Code = 200
		res.Data = protos.Encode(protoData)

		res.Call(c)
		return
	}

	if err = tripDbx.FinishTrip(
		userInfo.Uid, data.Id,
		s,
		&models.TripPermissions{
			AllowShare: false,
		},
		endTime,
	); err != nil {
		res.Errors(err)
		res.Code = 10011
		res.Call(c)
		return
	}

	trip, err := tripDbx.GetTrip(data.Id, userInfo.Uid)

	if err != nil {
		res.Errors(err)
		res.Code = 10006
		res.Call(c)
		return
	}

	if err := tripMemoryDbx.IngestTripMemory(trip); err != nil {
		res.Errors(err)
		res.Code = 10020
		res.Call(c)
		return
	}

	protoData := &protos.FinishTrip_Response{
		PositionLength: int32(len(tripPositions.Positions)),
	}

	res.Data = protos.Encode(protoData)

	res.Call(c)
}

func (fc *TripController) CorrectedTripData(c *gin.Context) {
	// 1、请求体
	var res response.ResponseProtobufType
	res.Code = 200

	// 2、获取参数
	data := new(protos.CorrectedTripData_Request)
	var err error
	if err = protos.DecodeBase64(c.GetString("data"), data); err != nil {
		res.Error = err.Error()
		res.Code = 10002
		res.Call(c)
		return
	}
	log.Info(data)

	// 3、验证参数
	if err = validation.ValidateStruct(
		data,
		validation.Parameter(&data.Id, validation.Required()),
	); err != nil {
		res.Errors(err)
		res.Code = 10002
		res.Call(c)
		return
	}

	// userInfoAny, exists := c.Get("userInfo")
	// if !exists {
	// 	res.Errors(err)
	// 	res.Code = 10004
	// 	res.Call(c)
	// 	return
	// }
	// userInfo := userInfoAny.(*sso.UserInfo)

	_, _, err = tripDbx.GetTripStatistics(data.Id, 0, true)
	if err != nil {
		res.Errors(err)
		res.Code = 10011
		res.Call(c)
		return
	}

	protoData := &protos.CorrectedTripData_Response{}

	res.Data = protos.Encode(protoData)

	res.Call(c)
}

func (fc *TripController) UpdateTrip(c *gin.Context) {
	// 1、请求体
	var res response.ResponseProtobufType
	res.Code = 200

	// 2、获取参数
	data := new(protos.UpdateTrip_Request)
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
		validation.Parameter(&data.Id, validation.Required()),
	); err != nil {
		res.Errors(err)
		res.Code = 10002
		res.Call(c)
		return
	}

	if data.Type != "" {
		if err = validation.ValidateStruct(
			data,
			validation.Parameter(&data.Type, validation.Required(), validation.Enum([]string{"Running",
				"Bike",
				"Drive",
				"Motorcycle",
				"Walking",
				"PowerWalking",
				"Train",
				"PublicTransport",
				"Plane"})),
		); err != nil {
			res.Errors(err)
			res.Code = 10002
			res.Call(c)
			return
		}
	}
	if data.AllowShare != "" {
		if err = validation.ValidateStruct(
			data,
			validation.Parameter(&data.AllowShare, validation.Required(), validation.Enum([]string{"Allow", "NotAllow"})),
		); err != nil {
			res.Errors(err)
			res.Code = 10002
			res.Call(c)
			return
		}
	}

	userInfoAny, exists := c.Get("userInfo")
	if !exists {
		res.Errors(err)
		res.Code = 10004
		res.Call(c)
		return
	}
	userInfo := userInfoAny.(*sso.UserInfo)

	if data.VehicleId != "" && data.VehicleId != "CancelVehicle" {
		vehicle, err := vehicleDbx.GetVehicle(data.VehicleId, userInfo.Uid, []int64{1})
		if vehicle == nil || err != nil {
			res.Errors(err)
			res.Code = 10018
			res.Call(c)
			return
		}
	}

	// log.Info(userInfo.Uid, data.Id, sk, data.Name, data.Type)
	err = tripDbx.UpdateTrip(
		userInfo.Uid, data.Id, data.AllowShare, data.Name, data.Type, data.VehicleId,
	)
	if err != nil {
		res.Errors(err)
		res.Code = 10011
		res.Call(c)
		return
	}
	// if sk == "disable" {
	// 	sk = ""
	// }

	protoData := &protos.UpdateTrip_Response{
		// ShareKey: sk,
		Name: data.Name,
	}

	res.Data = protos.Encode(protoData)

	res.Call(c)
}

func (fc *TripController) UpdateTrips(c *gin.Context) {
	// 1、请求体
	var res response.ResponseProtobufType
	res.Code = 200

	// 2、获取参数
	data := new(protos.UpdateTrips_Request)
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
		validation.Parameter(&data.Ids, validation.Length(0, 101), validation.Required()),
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

	if data.VehicleId != "" && data.VehicleId != "CancelVehicle" {
		vehicle, err := vehicleDbx.GetVehicle(data.VehicleId, userInfo.Uid, []int64{1})
		if vehicle == nil || err != nil {
			res.Errors(err)
			res.Code = 10018
			res.Call(c)
			return
		}
	}

	// log.Info(userInfo.Uid, data.Id, sk, data.Name, data.Type)

	if err = tripDbx.UpdateTrips(
		userInfo.Uid, data.Ids, data.VehicleId,
	); err != nil {
		res.Errors(err)
		res.Code = 10011
		res.Call(c)
		return
	}

	protoData := &protos.UpdateTrips_Response{}

	res.Data = protos.Encode(protoData)

	res.Call(c)
}

func (fc *TripController) DeleteTrip(c *gin.Context) {
	// 1、请求体
	var res response.ResponseProtobufType
	res.Code = 200

	// 2、获取参数
	data := new(protos.DeleteTrip_Request)
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
		validation.Parameter(&data.Id, validation.Required()),
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

	// if err := tripDbx.DeleteTrip("", data.Id); err != nil {
	// 	res.Errors(err)
	// 	if err.Error() == "deleted successfully" {
	// 		res.Code = 200
	// 	} else {
	// 		res.Code = 10017
	// 	}
	// 	res.Call(c)
	// 	return
	// }

	if err = tripDbx.DeleteTrip(
		userInfo.Uid, data.Id,
	); err != nil {
		res.Errors(err)
		res.Code = 10017
		res.Call(c)
		return
	}

	protoData := &protos.DeleteTrip_Response{}

	res.Data = protos.Encode(protoData)

	res.Call(c)
}

func (fc *TripController) GetTrip(c *gin.Context) {
	// 1、请求体
	var res response.ResponseProtobufType
	res.Code = 200

	// 2、获取参数
	data := new(protos.GetTrip_Request)
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
		validation.Parameter(&data.Id, validation.Required()),
	); err != nil {
		res.Errors(err)
		res.Code = 10002
		res.Call(c)
		return
	}

	authorId := ""
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

	// log.Info(data.Id, authorId, data.ShareKey)
	trip, err := tripDbx.GetTrip(data.Id, authorId)
	log.Info("getTrip, err ", trip, err)
	if err != nil || trip == nil {
		res.Errors(err)
		res.Code = 10006
		res.Call(c)
		return
	}

	if authorId == "" && !trip.Permissions.AllowShare {
		res.Errors(err)
		res.Code = 10004
		res.Call(c)
		return
	}
	// log.Info(data.Id, authorId, data.ShareKey)

	pTrip := formartTrip(trip)
	// log.Info("pTrip", pTrip.Status)
	pTrip.Positions = []*protos.TripPosition{}

	protoData := &protos.GetTrip_Response{
		Trip: pTrip,
	}

	res.Data = protos.Encode(protoData)

	res.Call(c)
}

func (fc *TripController) GetTripPositions(c *gin.Context) {
	// 1、请求体
	var res response.ResponseProtobufType
	res.Code = 200

	// 2、获取参数
	data := new(protos.GetTripPositions_Request)
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
		validation.Parameter(&data.Id, validation.Required()),
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

	trip, err := tripDbx.GetTrip(data.Id, authorId)
	// log.Info("getTrip, err ", getTrip, err)
	if err != nil || trip == nil {
		res.Errors(err)
		res.Code = 10006
		res.Call(c)
		return
	}

	if authorId == "" && !trip.Permissions.AllowShare {
		res.Errors(err)
		res.Code = 10004
		res.Call(c)
		return
	}
	var pgp *models.PrivacyGeofencePoints
	// 非登录或者非作者登录且分享状态
	if (authorId == "" || authorId != trip.AuthorId) && trip.Permissions.AllowShare {

		pgp, err = getUserPGP(trip.AuthorId)

		if err != nil {
			res.Errors(err)
			res.Code = 10001
			res.Call(c)
			return
		}
	}

	authorId = trip.AuthorId

	// log.Info(data.Id, authorId, data.ShareKey)
	tripPositions, err := tripDbx.GetTripPositions(data.Id, authorId)
	// log.Info("tripPositions, err ", len(tripPositions.Positions), err)
	if err != nil || tripPositions == nil {
		res.Errors(err)
		res.Code = 10006
		res.Call(c)
		return
	}
	// log.Info(data.Id, authorId, data.ShareKey)

	positions := []string{}
	// mapKeys := map[string]int{}
	// keyIndex := int(0)
	// log.Error("pgp", len(tripPositions.Positions), pgp)

	if pgp != nil && len(pgp.Points) > 0 {
		// log.Warn("pgp", len(tripPositions.Positions))
		mp := narrays.Map(pgp.Points, func(
			v *models.PrivacyGeofencePointsItem, index int,
		) []*methods.Point {

			polygon := narrays.Map(v.Coords, func(
				pic *models.PrivacyGeofencePointsItemCoords, sIndex int,
			) *methods.Point {
				return &methods.Point{pic.Latitude, pic.Longitude}
			})

			return polygon
		})
		tripPositions.Positions = narrays.Filter(tripPositions.Positions, func(
			pos *models.TripPosition, index int,
		) bool {

			return !methods.IsPointInMultiPolygon(
				&methods.Point{pos.Latitude, pos.Longitude},
				mp,
			)
		})
		// log.Warn("pgp", len(tripPositions.Positions))

	}

	vPositions, existsTimestamp := tripDbx.FilterPositions(
		tripPositions.Positions,
		tripPositions.StartTime,
		tripPositions.EndTime)

	// log.Warn("pgp", len(vPositions), len(tripPositions.Positions))

	// log.Info("vPositions", vPositions, existsTimestamp,
	// 	len(tripPositions.Positions), tripPositions.CreateTime, tripPositions.EndTime)

	startLat := int64(0)
	startLon := int64(0)

	for i, v := range vPositions {
		// if end {
		// 	continue
		// }

		pv := new(protos.TripPosition)
		copier.Copy(pv, v)

		pv = formartPosition(pv)

		lat := nstrings.ToString(pv.Latitude)
		lon := nstrings.ToString(pv.Longitude)
		if i != 0 {
			lat = nstrings.ToString(startLat - nint.ToInt64(pv.Latitude*100000000))
			lon = nstrings.ToString(startLon - nint.ToInt64(pv.Longitude*100000000))

		}
		startLat = nint.ToInt64(pv.Latitude * 100000000)
		startLon = nint.ToInt64(pv.Longitude * 100000000)
		// lat = methods.GetGeoKey(&mapKeys, lat, &keyIndex)
		// lon = methods.GetGeoKey(&mapKeys, lon, &keyIndex)

		// log.Info("pv.Latitude", pv.Latitude)
		// log.Info("pv.Heading", pv.Heading)
		positions = append(positions,
			lat+"_"+
				lon+"_"+
				nstrings.ToString(pv.Altitude)+"_"+
				nstrings.ToString(pv.Speed)+"_"+
				nstrings.ToString((pv.Timestamp/1000)-tripPositions.StartTime)+"_"+
				nstrings.ToString(pv.Heading),
		)
		// nstrings.ToString("")+"_"+
		// // nstrings.ToString(pv.AltitudeAccuracy)+"_"+
		// nstrings.ToString("")+"_"+
		// // nstrings.ToString(pv.Accuracy)+"_"+
		// nstrings.ToString("")+"_"+
		// nstrings.ToString(pv.Heading)+"_"+
	}

	// log.Info("tripPositions.Status", tripPositions.Status)
	// log.Error("newPositions", len(existsTimestamp), len(vPositions), len(tripPositions.Positions))
	if len(existsTimestamp) != len(tripPositions.Positions) {
		if err = tripDbx.CheckPositions(tripPositions); err != nil {
			res.Errors(err)
			res.Code = 10001
			res.Call(c)
			return
		}
	}
	// log.Info("tripPositions.Status", tripPositions.Status)

	// keys := []string{}
	// for i := 0; i < keyIndex; i++ {
	// 	keys = append(keys, "")
	// }
	// for k, v := range mapKeys {
	// 	log.Info(k, v)
	// 	keys[v-1] = k
	// 	// keys = append(keys, k)
	// }

	tripPositionsProto := protos.TripPositions{
		StartTime: tripPositions.StartTime,
		Positions: positions,
		// Keys:      keys,
		Total:    nint.ToInt64(len(tripPositions.Positions)),
		Id:       tripPositions.Id,
		Type:     tripPositions.Type,
		AuthorId: tripPositions.AuthorId,
		Status:   tripPositions.Status,
		Permissions: &protos.TripPermissions{
			CustomTrip: tripPositions.Permissions.CustomTrip,
		},
	}

	protoData := &protos.GetTripPositions_Response{
		TripPositions: &tripPositionsProto,
	}

	res.Data = protos.Encode(protoData)

	res.Call(c)
}

func (fc *TripController) GetTripHistoryPositions(c *gin.Context) {
	// 1、请求体
	var res response.ResponseProtobufType
	res.Code = 200

	// 2、获取参数
	data := new(protos.GetTripHistoryPositions_Request)
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
		validation.Parameter(&data.Type, validation.Required(), validation.Enum([]string{
			"All", "Running",
			"Bike",
			"Drive",
			"Motorcycle",
			"Walking",
			"PowerWalking",
			"Train",
			"PublicTransport",
			"Plane"})),
		validation.Parameter(&data.PageNum, validation.GreaterEqual(int64(1)), validation.Required()),
		validation.Parameter(&data.PageSize, validation.NumRange(int64(1), int64(50)), validation.Required()),
		// validation.Parameter(&data.Ids, validation.Length(0, 100), validation.Required()),
		validation.Parameter(&data.TimeLimit, validation.Length(2, 2), validation.Required()),
		validation.Parameter(&data.FullData, validation.Type("bool"), validation.Required()),
		validation.Parameter(&data.JmId, validation.Type("string")),
	); err != nil {
		res.Errors(err)
		res.Code = 10002
		res.Call(c)
		return
	}

	authorId := ""

	log.Info(data.JmId, "jmid")

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

	var pgp *models.PrivacyGeofencePoints

	if data.JmId != "" {
		jm, err := jmDbx.GetJM(
			data.JmId, "",
		)
		// log.Info(jm, err)
		if err != nil || jm == nil {
			res.Errors(err)
			res.Code = 10006
			res.Call(c)
			return
		}

		log.Error(authorId == "", authorId != jm.AuthorId)
		if authorId != jm.AuthorId {
			pgp, err = getUserPGP(jm.AuthorId)

			if err != nil {
				res.Errors(err)
				res.Code = 10001
				res.Call(c)
				return
			}

		}

		authorId = jm.AuthorId
	}
	// log.Error("pgp", pgp)

	// log.Info(authorId)
	trips, err := tripDbx.GetTripAllPositions(authorId, data.Type, data.PageNum, data.PageSize, data.Ids, data.TimeLimit[0], data.TimeLimit[1], data.VehicleLimit, false)
	// log.Info("trips, err ", trips, err)
	if err != nil || len(trips) == 0 {
		res.Errors(err)
		res.Code = 10006
		res.Call(c)
		return
	}

	// if int64(len(trips) ) == data.PageSize {
	// 	go tripDbx.GetTripAllPositions(authorId, data.Type, data.PageNum+1, data.PageSize, data.Ids)
	// }
	// log.Info(data.Id, authorId, data.ShareKey)

	protoData := &protos.GetTripHistoryPositions_Response{}
	for _, v := range trips {

		// s, err := tripDbx.GetTripStatistics(v.Id, 0)
		// if err != nil {
		// 	res.Errors(err)
		// 	res.Code = 10011
		// 	res.Call(c)
		// 	return
		// }
		// log.Info(s.Distance)
		// err = tripDbx.CorrectedTripData(
		// 	authorId, v.Id,
		// 	s,
		// )
		// log.Error(err)

		tripPositions := new(protos.TripPositions)
		positions := []string{}
		// mapKeys := []string{}
		// mapKeys := map[string]int{}
		// alphabet := strings.Split("abcdefghijklmnopqrstuvwxyz", "")
		// keyIndex := int(0)

		if pgp != nil && len(pgp.Points) > 0 {
			// log.Warn("pgp", len(v.Positions))
			mp := narrays.Map(pgp.Points, func(
				v *models.PrivacyGeofencePointsItem, index int,
			) []*methods.Point {

				polygon := narrays.Map(v.Coords, func(
					pic *models.PrivacyGeofencePointsItemCoords, sIndex int,
				) *methods.Point {
					return &methods.Point{pic.Latitude, pic.Longitude}
				})

				return polygon
			})
			v.Positions = narrays.Filter(v.Positions, func(
				pos *models.TripPosition, index int,
			) bool {

				return !methods.IsPointInMultiPolygon(
					&methods.Point{pos.Latitude, pos.Longitude},
					mp,
				)
			})
			// log.Warn("pgp", len(v.Positions))

		}

		vPositions, existsTimestamp := tripDbx.FilterPositions(
			v.Positions, v.StartTime, v.EndTime)

		vPositions = tripDbx.FilterPositionsByPrivacyGeofence(vPositions, pgp)

		// var startPositions *models.TripPosition

		startLat := int64(0)
		startLon := int64(0)

		for si, sv := range vPositions {

			pv := new(protos.TripPosition)
			copier.Copy(pv, sv)

			pv = formartPosition(pv)
			lat := nstrings.ToString(pv.Latitude)
			lon := nstrings.ToString(pv.Longitude)

			if si != 0 {
				lat = nstrings.ToString(startLat - nint.ToInt64(pv.Latitude*100000000))
				lon = nstrings.ToString(startLon - nint.ToInt64(pv.Longitude*100000000))

			}
			startLat = nint.ToInt64(pv.Latitude * 100000000)
			startLon = nint.ToInt64(pv.Longitude * 100000000)
			// lat = methods.GetGeoKey(&mapKeys, lat, &keyIndex)
			// lon = methods.GetGeoKey(&mapKeys, lon, &keyIndex)

			// lats := strings.Split(lat, ".")
			// key := lats[0] + "." + lats[1][0:2]
			// if mapKeys[key] == "" {
			// 	mapKeys[key] = alphabet[alphabetIndex]
			// }

			if data.FullData {
				positions = append(positions,
					lat+"_"+
						lon+"_"+
						nstrings.ToString(pv.Altitude)+"_"+
						nstrings.ToString(pv.Speed)+"_"+
						nstrings.ToString((pv.Timestamp/1000)-tripPositions.StartTime)+"_"+
						nstrings.ToString(pv.Heading),
				)
			} else {
				positions = append(positions,
					lat+"_"+
						lon,
					// nstrings.ToString("")+"_"+
					// // nstrings.ToString(pv.Altitude)+"_"+
					// nstrings.ToString("")+"_"+
					// // nstrings.ToString(pv.AltitudeAccuracy)+"_"+
					// nstrings.ToString("")+"_"+
					// // nstrings.ToString(pv.Accuracy)+"_"+
					// nstrings.ToString("")+"_"+
					// // nstrings.ToString(pv.Heading)+"_"+
					// nstrings.ToString("")+"_"+
					// // nstrings.ToString(pv.Speed)+"_"+
					// nstrings.ToString("")+"_",
					// nstrings.ToString((pv.Timestamp/1000)-v.StartTime)
				)
			}

		}

		// keys := []string{}
		// for i := 0; i < keyIndex; i++ {
		// 	keys = append(keys, "")
		// }
		// for k, v := range mapKeys {
		// 	log.Info(k, v)
		// 	keys[v-1] = k
		// 	// keys = append(keys, k)
		// }

		tripPositions.StartTime = v.StartTime
		tripPositions.Id = v.Id
		tripPositions.Type = v.Type
		tripPositions.VehicleId = v.VehicleId
		tripPositions.Positions = positions
		tripPositions.AuthorId = v.AuthorId
		// tripPositions.Keys = keys
		tripPositions.Status = v.Status

		tripPositions.Permissions = &protos.TripPermissions{
			CustomTrip: v.Permissions.CustomTrip,
		}
		tripPositions.Total = nint.ToInt64(len(positions))

		protoData.List = append(protoData.List, tripPositions)

		if len(existsTimestamp) != len(v.Positions) {
			tripPositions.Status = 0
			log.Info(len(existsTimestamp), len(v.Positions),
				len(vPositions))
			if err = tripDbx.CheckPositions(v); err != nil {
				res.Errors(err)
				res.Code = 10001
				res.Call(c)
				return
			}
		}
	}

	protoData.Total = nint.ToInt64(len(protoData.List))

	log.Info(protoData.Total)

	res.JSON(c, res.ProtoToMap(protoData))
}

func (fc *TripController) GetTrips(c *gin.Context) {
	// 1、请求体
	var res response.ResponseProtobufType
	res.Code = 200

	// 2、获取参数
	data := new(protos.GetTrips_Request)
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
		validation.Parameter(&data.Type, validation.Required(), validation.Enum([]string{"All", "Running",
			"Bike",
			"Drive",
			"Motorcycle",
			"Walking",
			"PowerWalking",
			"Train",
			"PublicTransport",
			"Plane"})),
		// validation.Parameter(&data.TimeLimit, validation.Length(2, 2), validation.Required()),
		validation.Parameter(&data.PageNum, validation.GreaterEqual(int64(1)), validation.Required()),
		validation.Parameter(&data.PageSize, validation.NumRange(int64(1), int64(1000000)), validation.Required()),
		// validation.Parameter(&data.DistanceLimit, validation.Length(2, 2), validation.Required()),
		validation.Parameter(&data.TurnOffCache, validation.Type("bool")),
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

	filterTripIds := []string{}

	if len(data.JourneyMemoryLimit) > 0 {
		jmList, err := jmDbx.GetJMList(
			data.JourneyMemoryLimit,
			userInfo.Uid,
			1, 100000,
		)
		if err != nil {
			res.Errors(err)
			res.Code = 10006
			res.Call(c)
			return
		}

		for _, v := range jmList {
			for _, sv := range v.Timeline {
				filterTripIds = append(filterTripIds, sv.TripIds...)
			}
		}
	}

	log.Error("data.LastUpdateTimeLimit", data.LastUpdateTimeLimit)
	getTrips, err := tripDbx.GetTripsBaseData(
		filterTripIds,
		userInfo.Uid, data.Type,
		// 1, 100000,
		data.PageNum, data.PageSize,
		data.CreateTimeLimit,
		data.LastUpdateTimeLimit,
		data.VehicleLimit,

		data.DistanceLimit[0]*1000,
		data.DistanceLimit[1]*1000,
		!data.TurnOffCache,
	)
	if err != nil {
		res.Errors(err)
		res.Code = 10016
		res.Call(c)
		return
	}
	// log.Info("getTrips", getTrips)
	if len(getTrips) == 0 {
		res.Errors(err)
		res.Code = 10006
		res.Call(c)
		return

	}

	// // authorId := c.MustGet("userInfo").(*sso.UserInfo).Uid
	trips := []*protos.Trip{}
	for _, v := range getTrips {
		// log.Info("CheckEndTime", v)
		v = tripDbx.CheckEndTime(v)

		trips = append(trips, formartTrip(v))
	}
	protoData := &protos.GetTrips_Response{
		List:  trips,
		Total: nint.ToInt64(len(trips)),
	}

	res.Data = protos.Encode(protoData)

	res.Call(c)
}

func (fc *TripController) GetTripStatistics(c *gin.Context) {
	// 1、请求体
	var res response.ResponseProtobufType
	res.Code = 200

	// 2、获取参数
	data := new(protos.GetTripStatistics_Request)
	var err error
	if err = protos.DecodeBase64(c.GetString("data"), data); err != nil {
		res.Error = err.Error()
		res.Code = 10002
		res.Call(c)
		return
	}

	log.Info("data", data, data.DistanceLimit)

	// 3、验证参数
	if err = validation.ValidateStruct(
		data,
		validation.Parameter(&data.Type, validation.Required(), validation.Enum([]string{
			"All",
			"Running",
			"Bike",
			"Drive",
			"Motorcycle",
			"Walking",
			"PowerWalking",
			"Train",
			"PublicTransport",
			"Plane"})),
		validation.Parameter(&data.TimeLimit, validation.Length(2, 2), validation.Required()),
		validation.Parameter(&data.DistanceLimit, validation.Required()),
		// validation.Parameter(&data.PageNum, validation.GreaterEqual(int64(1)), validation.Required()),
		// validation.Parameter(&data.PageSize, validation.NumRange(int64(1), int64(101)), validation.Required()),
	); err != nil {
		res.Errors(err)
		res.Code = 10002
		res.Call(c)
		return
	}

	authorId := ""

	if c.GetBool("isOpenApi") {
		authorId = c.GetString("openUid")
	} else {
		userInfoAny, exists := c.Get("userInfo")
		if exists {
			userInfo := userInfoAny.(*sso.UserInfo)
			authorId = userInfo.Uid
		}
	}

	if authorId == "" {
		res.Errors(err)
		res.Code = 10004
		res.Call(c)
		return
	}

	filterTripIds := []string{}

	if len(data.JourneyMemoryLimit) > 0 {
		jmList, err := jmDbx.GetJMList(
			data.JourneyMemoryLimit,
			authorId,
			1, 100000,
		)
		if err != nil {
			res.Errors(err)
			res.Code = 10006
			res.Call(c)
			return
		}

		for _, v := range jmList {
			for _, sv := range v.Timeline {
				filterTripIds = append(filterTripIds, sv.TripIds...)
			}
		}
	}

	log.Info("data.DistanceLimit", data.DistanceLimit, authorId)
	l1 := log.Time()
	getTripsBaseData, err := tripDbx.GetTripsBaseData(
		filterTripIds,
		authorId, data.Type,
		1, 10*10000,
		// data.PageNum, data.PageSize,
		data.TimeLimit, []int64{}, data.VehicleLimit,
		data.DistanceLimit[0]*1000,
		data.DistanceLimit[1]*1000, true,
	)
	l1.TimeEnd("getTripsBaseData")
	log.Info("getTripsBaseData", len(getTripsBaseData))
	if err != nil {
		res.Errors(err)
		res.Code = 10016
		res.Call(c)
		return
	}
	// jmProto.Statistics = ts

	// timeSec := int64(0)
	// uselessData := []string{}
	// distance := float64(0)
	// days := []string{}
	// // trips := []*protos.Trip{}
	// for _, v := range getTripsBaseData {
	// 	if v.Status == 0 && v.Statistics.Distance < 50 {
	// 		uselessData = append(uselessData, v.Id)
	// 	}
	// 	if v.Status != 1 {
	// 		continue
	// 	}
	// 	distance += v.Statistics.Distance
	// 	timeSec += v.EndTime - v.StartTime

	// 	t := time.Unix(v.StartTime, 0)
	// 	dateStr := t.Format("2006-01-02")

	// 	if !narrays.Includes(days, dateStr) {
	// 		days = append(days, dateStr)
	// 	}

	// 	// trips = append(trips, formartTrip(v))
	// }

	l2 := log.Time()
	ts := tripDbx.FormatTripStatistics(getTripsBaseData)
	l2.TimeEnd("FormatTripStatistics")

	log.Error("getTripsBaseData", len(getTripsBaseData))

	protoData := &protos.GetTripStatistics_Response{
		Statistics: ts,
		// Count:       nint.ToInt32(len(getTripsBaseData)),
		// Distance:    distance,
		// UselessData: uselessData,
		// Time:        timeSec,
		// Days:        nint.ToInt32(len(days)),
		// List:        trips,
	}

	res.Data = protos.Encode(protoData)

	res.Call(c)
}

func (fc *TripController) GetBaseTripsByOpenAPI(c *gin.Context) {
	// 1、请求体
	var res response.ResponseProtobufType
	res.Code = 200

	// 2、获取参数
	data := new(protos.GetTrips_Request)
	var err error
	if err = protos.DecodeBase64(c.GetString("data"), data); err != nil {
		res.Error = err.Error()
		res.Code = 10002
		res.Call(c)
		return
	}

	log.Info("data", data, data.DistanceLimit)

	// 3、验证参数
	if err = validation.ValidateStruct(
		data,
	); err != nil {
		res.Errors(err)
		res.Code = 10002
		res.Call(c)
		return
	}

	authorId := c.GetString("openUid")

	// log.Info("authorId", authorId)
	if authorId == "" {
		res.Errors(err)
		res.Code = 10004
		res.Call(c)
		return
	}

	// log.Info("data.DistanceLimit", data.DistanceLimit, authorId)
	getTripsBaseData, err := tripDbx.GetTripsBaseData(
		[]string{},
		authorId, "All",
		1, 100000,
		[]int64{},
		[]int64{},
		data.VehicleLimit,
		0,
		500*1000, true,
	)
	// log.Info("getTripsBaseData", authorId, getTripsBaseData)
	if err != nil {
		res.Errors(err)
		res.Code = 10016
		res.Call(c)
		return
	}

	trips := []*protos.Trip{}
	for _, v := range getTripsBaseData {
		// log.Info("CheckEndTime", v)
		v = tripDbx.CheckEndTime(v)

		trips = append(trips, formartTrip(v))
	}
	protoData := &protos.GetTrips_Response{
		List:  trips,
		Total: nint.ToInt64(len(trips)),
	}

	res.JSON(c, res.ProtoToMap(protoData))
}

// func (fc *TripController) GetHistoricalStatistics(c *gin.Context) {
// 	// 1、请求体
// 	var res response.ResponseProtobufType
// 	res.Code = 200

// 	// 2、获取参数
// 	data := new(protos.GetHistoricalStatistics_Request)
// 	var err error
// 	if err = protos.DecodeBase64(c.GetString("data"), data); err != nil {
// 		res.Error = err.Error()
// 		res.Code = 10002
// 		res.Call(c)
// 		return
// 	}

// 	log.Info("data", data)

// 	// 3、验证参数
// 	if err = validation.ValidateStruct(
// 		data,
// 		validation.Parameter(&data.Type, validation.Required(), validation.Enum([]string{"All", "Running",
// 			"Bike",
// 			"Drive",
// 			"Motorcycle",
// 			"Walking",
// 			"PowerWalking",
// 			"Train",
// 			"PublicTransport",
// 			"Plane"})),
// 		validation.Parameter(&data.TimeLimit, validation.Length(2, 2), validation.Required()),
// 	); err != nil {
// 		res.Errors(err)
// 		res.Code = 10002
// 		res.Call(c)
// 		return
// 	}

// 	userInfoAny, exists := c.Get("userInfo")
// 	if !exists {
// 		res.Errors(err)
// 		res.Code = 10004
// 		res.Call(c)
// 		return
// 	}
// 	userInfo := userInfoAny.(*sso.UserInfo)

// 	protoData := &protos.GetHistoricalStatistics_Response{}
// 	for _, v := range []string{
// 		"maxDistance",
// 		"maxSpeed",
// 		"averageSpeed",
// 		"maxAltitude",
// 		"minAltitude",
// 		"climbAltitude",
// 		"descendAltitude",
// 	} {
// 		numItem := &protos.GetHistoricalStatistics_Response_NumItem{
// 			Num: 0,
// 			Id:  "",
// 		}

// 		vTrip, err := tripDbx.GetHistoricalStatisticsData(userInfo.Uid, data.Type, v, data.TimeLimit[0], data.TimeLimit[1])

// 		if vTrip != nil && err == nil {
// 			numItem.Id = vTrip.Id
// 			switch v {
// 			case "maxDistance":
// 				numItem.Num = vTrip.Statistics.Distance
// 				protoData.MaxDistance = numItem
// 			case "maxSpeed":
// 				numItem.Num = vTrip.Statistics.MaxSpeed
// 				protoData.MaxSpeed = numItem
// 			case "averageSpeed":
// 				numItem.Num = vTrip.Statistics.AverageSpeed
// 				protoData.FastestAverageSpeed = numItem
// 			case "maxAltitude":
// 				numItem.Num = vTrip.Statistics.MaxAltitude
// 				protoData.MaxAltitude = numItem
// 			case "minAltitude":
// 				numItem.Num = vTrip.Statistics.MinAltitude
// 				protoData.MinAltitude = numItem
// 			case "climbAltitude":
// 				numItem.Num = vTrip.Statistics.ClimbAltitude
// 				protoData.MaxClimbAltitude = numItem
// 			case "descendAltitude":
// 				numItem.Num = vTrip.Statistics.DescendAltitude
// 				protoData.MaxDescendAltitude = numItem

// 			}
// 		}
// 	}

// 	res.Data = protos.Encode(protoData)

// 	res.Call(c)
// }

func (fc *TripController) ResumeTrip(c *gin.Context) {
	// 1、请求体
	var res response.ResponseProtobufType
	res.Code = 200

	// 2、获取参数
	data := new(protos.ResumeTrip_Request)
	var err error
	if err = protos.DecodeBase64(c.GetString("data"), data); err != nil {
		res.Error = err.Error()
		res.Code = 10002
		res.Call(c)
		return
	}
	log.Info(data)

	// 3、验证参数
	if err = validation.ValidateStruct(
		data,
		validation.Parameter(&data.Id, validation.Required()),
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

	getTrip, err := tripDbx.GetTrip(data.Id, userInfo.Uid)
	// log.Info("getTrip, err ", getTrip.Status, err)
	if err != nil || getTrip == nil {
		res.Errors(err)
		res.Code = 10006
		res.Call(c)
		return
	}
	if getTrip.CreateTime+300*3600 < time.Now().Unix() {
		res.Errors(err)
		res.Code = 10019
		res.Call(c)
		return
	}

	err = tripDbx.ResumeTrip(data.Id, userInfo.Uid)
	if err != nil {
		res.Errors(err)
		res.Code = 10011
		res.Call(c)
		return
	}

	protoData := &protos.ResumeTrip_Response{}

	res.Data = protos.Encode(protoData)

	res.Call(c)
}

func (cl *TripController) ClearTripCities(c *gin.Context) {
	// 1、请求体
	var res response.ResponseProtobufType
	res.Code = 200

	// 2、获取参数
	data := new(protos.ClearTripCities_Request)
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
	authorId := userInfoAny.(*sso.UserInfo).Uid
	// log.Info("AddAndGetFullCity", data)

	if err := tripDbx.UpdateTripCities(
		data.TripId, authorId, []*models.TripCity{},
	); err != nil {
		res.Errors(err)
		res.Code = 10011
		res.Call(c)
		return
	}

	protoData := &protos.ClearTripCities_Response{}

	res.Data = protos.Encode(protoData)

	res.Call(c)
}

func (cl *TripController) ClearTripRoads(c *gin.Context) {
	// 1、请求体
	var res response.ResponseProtobufType
	res.Code = 200

	// 2、获取参数
	data := new(protos.ClearTripRoads_Request)
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
	authorId := userInfoAny.(*sso.UserInfo).Uid
	// log.Info("AddAndGetFullCity", data)

	if err := tripDbx.UpdateTripRoads(
		data.TripId, authorId, []*models.TripRoad{},
	); err != nil {
		res.Errors(err)
		res.Code = 10011
		res.Call(c)
		return
	}

	protoData := &protos.ClearTripRoads_Response{}

	res.Data = protos.Encode(protoData)

	res.Call(c)
}

func (fc *TripController) UpdateTripAddresses(c *gin.Context) {
	// 1、请求体
	var res response.ResponseProtobufType
	res.Code = 200

	// 2、获取参数
	data := new(protos.UpdateTripAddresses_Request)
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
		validation.Parameter(&data.Trips, validation.Length(1, 100000000), validation.Required()),
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

	// log.Info(userInfo.Uid, data.Id, sk, data.Name, data.Type)

	for _, v := range data.Trips {
		if err = tripDbx.UpdateTripAddresses(
			userInfo.Uid, v.Id, narrays.Map(v.Addresses, func(sv *protos.TripAddresses, i int) *models.TripAddresses {

				return &models.TripAddresses{
					Latitude:  sv.Latitude,
					Longitude: sv.Longitude,
					Altitude:  sv.Altitude,
					City: &models.TripAddressesCity{
						Country: sv.City.Country,
						State:   sv.City.State,
						Region:  sv.City.Region,
						City:    sv.City.City,
						Town:    sv.City.Town,
						Road:    sv.City.Road,
					},
					Address: &models.TripAddressesAddress{
						FullName: sv.Address.FullName,
						Type:     sv.Address.Type,
						Name:     sv.Address.Name,
					},
					EntryTime: sv.EntryTime,
				}
			}),
		); err != nil {
			log.Error(err)
			res.Errors(err)
			res.Code = 10011
			res.Call(c)
			return
		}
	}

	protoData := &protos.UpdateTripAddresses_Response{}

	res.Data = protos.Encode(protoData)

	res.Call(c)
}

func (fc *TripController) UpdateTripNetworkStatus(c *gin.Context) {
	// 1、请求体
	var res response.ResponseProtobufType
	res.Code = 200

	// 2、获取参数
	data := new(protos.UpdateTripNetworkStatus_Request)
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
		validation.Parameter(&data.Id, validation.Type("string"), validation.Required()),
		validation.Parameter(&data.NetworkStatus, validation.Length(1, 100000000), validation.Required()),
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

	// log.Info(userInfo.Uid, data.Id)

	if err = tripDbx.UpdateTripNetworkStatus(
		userInfo.Uid, data.Id, narrays.Map(data.NetworkStatus, func(sv *protos.TripNetworkStatus, i int) *models.TripNetworkStatus {
			return &models.TripNetworkStatus{
				Status:    sv.Status,
				Timestamp: sv.Timestamp,
			}
		}),
	); err != nil {
		log.Error(err)
		res.Errors(err)
		res.Code = 10011
		res.Call(c)
		return
	}

	protoData := &protos.UpdateTripNetworkStatus_Response{}

	res.Data = protos.Encode(protoData)

	res.Call(c)
}

func (fc *TripController) UpdateTripWeather(c *gin.Context) {
	// 1、请求体
	var res response.ResponseProtobufType
	res.Code = 200

	// 2、获取参数
	data := new(protos.UpdateTripWeather_Request)
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
		validation.Parameter(&data.Id, validation.Type("string"), validation.Required()),
	); err != nil {
		res.Errors(err)
		res.Code = 10002
		res.Call(c)
		return
	}
	if err = validation.ValidateStruct(
		data.Weather,
		validation.Parameter(&data.Weather.Timestamp, validation.Type("int64"), validation.Required()),
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

	// log.Info(userInfo.Uid, data.Id)

	if err = tripDbx.UpdateTripWeather(
		data.Id, userInfo.Uid, &models.TripWeather{
			WeatherCode:         data.Weather.WeatherCode,
			Temperature:         data.Weather.Temperature,
			ApparentTemperature: data.Weather.ApparentTemperature,
			WindSpeed:           data.Weather.WindSpeed,
			WindDirection:       data.Weather.WindDirection,
			Humidity:            data.Weather.Humidity,
			Precipitation:       data.Weather.Precipitation,
			Timestamp:           data.Weather.Timestamp,
		},
	); err != nil {
		log.Error(err)
		res.Errors(err)
		res.Code = 10011
		res.Call(c)
		return
	}

	protoData := &protos.UpdateTripWeather_Response{}

	res.Data = protos.Encode(protoData)

	res.Call(c)
}
