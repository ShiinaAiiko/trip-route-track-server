package controllersV1

import (
	conf "github.com/ShiinaAiiko/nyanya-trip-route-track/server/config"
	dbxv1 "github.com/ShiinaAiiko/nyanya-trip-route-track/server/dbx/v1"
	"github.com/ShiinaAiiko/nyanya-trip-route-track/server/models"
	"github.com/ShiinaAiiko/nyanya-trip-route-track/server/protos"
	"github.com/ShiinaAiiko/nyanya-trip-route-track/server/services/response"
	"github.com/cherrai/nyanyago-utils/narrays"
	"github.com/cherrai/nyanyago-utils/nint"
	"github.com/cherrai/nyanyago-utils/validation"
	sso "github.com/cherrai/saki-sso-go"
	"github.com/gin-gonic/gin"
	"github.com/jinzhu/copier"
)

// "github.com/cherrai/nyanyago-utils/validation"

var (
	positionDbx = dbxv1.PositionDbx{}
)

type PositionController struct {
}

func (fc *PositionController) UpdateUserPosition(c *gin.Context) {
	// 1、请求体
	var res response.ResponseProtobufType
	res.Code = 200

	// 2、获取参数
	data := new(protos.UpdateUserPosition_Request)
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
		validation.Parameter(&data.Position, validation.Required()),
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

	v := data.Position

	if err = positionDbx.UpdateUserPosition(
		userInfo.Uid, &models.TripPosition{
			Latitude:         v.Latitude,
			Longitude:        v.Longitude,
			Altitude:         v.Altitude,
			AltitudeAccuracy: v.AltitudeAccuracy,
			Accuracy:         v.Accuracy,
			Heading:          v.Heading,
			Speed:            v.Speed,
			Timestamp:        v.Timestamp,
		},
	); err != nil {
		res.Errors(err)
		res.Code = 10011
		res.Call(c)
		return
	}

	protoData := &protos.UpdateUserPosition_Response{}

	res.Data = protos.Encode(protoData)

	res.Call(c)
}

func (fc *PositionController) UpdateUserPositionShare(c *gin.Context) {
	// 1、请求体
	var res response.ResponseProtobufType
	res.Code = 200

	// 2、获取参数
	data := new(protos.UpdateUserPositionShare_Request)
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
		validation.Parameter(&data.PositionShare, validation.Enum([]int64{
			5, 1, -1,
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

	if err = positionDbx.UpdateUserPositionShare(
		userInfo.Uid, data.PositionShare,
	); err != nil {
		res.Errors(err)
		res.Code = 10011
		res.Call(c)
		return
	}

	protoData := &protos.UpdateUserPositionShare_Response{}

	res.Data = protos.Encode(protoData)

	res.Call(c)
}

func (fc *PositionController) GetUserPositionShare(c *gin.Context) {
	// 1、请求体
	var res response.ResponseProtobufType
	res.Code = 200

	// 2、获取参数
	data := new(protos.GetUserPositionShare_Request)
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
	userPosition, err := positionDbx.GetUserPosition(
		userInfo.Uid,
	)
	log.Info("userPosition", userPosition.PositionShare)
	if err != nil {
		res.Errors(err)
		res.Code = 10011
		res.Call(c)
		return
	}

	protoData := &protos.GetUserPositionShare_Response{
		PositionShare: userPosition.PositionShare,
	}

	res.Data = protos.Encode(protoData)

	res.Call(c)
}

func (fc *PositionController) GetUserPositionAndVehiclePosition(c *gin.Context) {
	// 1、请求体
	var res response.ResponseProtobufType
	res.Code = 200

	// 2、获取参数
	data := new(protos.GetUserPositionAndVehiclePosition_Request)
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
		validation.Parameter(&data.MaxDistance, validation.Required()),
		validation.Parameter(&data.LatitudeLimit, validation.Length(2, 2), validation.Required()),
		validation.Parameter(&data.LongitudeLimit, validation.Length(2, 2), validation.Required()),
		validation.Parameter(&data.TimeLimit, validation.Length(2, 2), validation.Required()),
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

	userPositionList, err := positionDbx.GetUserPositionList(
		userInfo.Uid,
		data.MaxDistance, data.TimeLimit[0], data.TimeLimit[1],
		data.LatitudeLimit[0], data.LatitudeLimit[1],
		data.LongitudeLimit[0], data.LongitudeLimit[1],
	)
	if err != nil {
		res.Errors(err)
		res.Code = 10006
		res.Call(c)
		return
	}

	// log.Info("userPositionList", userPositionList)

	list := []*protos.GetUserPositionAndVehiclePosition_Response_PositionItem{}

	userIds := []string{}

	for _, v := range userPositionList {
		if v.Id != userInfo.Uid {
			userIds = append(userIds, v.Id)
		}

	}

	vehiclePositionList, err := vehicleDbx.GetVehiclePositionList(
		data.MaxDistance, data.TimeLimit[0], data.TimeLimit[1],
		data.LatitudeLimit[0], data.LatitudeLimit[1],
		data.LongitudeLimit[0], data.LongitudeLimit[1],
	)
	if err != nil {
		res.Errors(err)
		res.Code = 10006
		res.Call(c)
		return
	}
	for _, v := range vehiclePositionList {
		if v.AuthorId != userInfo.Uid {
			userIds = append(userIds, v.AuthorId)
		}
	}

	users := []*sso.UserInfo{}

	if len(userIds) > 0 {
		users, err = conf.SSO.GetUsers(userIds)
		if err != nil {
			res.Errors(err)
			res.Code = 10006
			res.Call(c)
			return
		}
	}

	users = append(users, userInfo)

	for _, v := range userPositionList {
		tp := new(protos.TripPosition)

		// log.Error("userPositionList", vehiclePositionList, v.Id, v.PositionShare)

		if v.Id == userInfo.Uid && v.PositionShare == -1 {
			continue
		}
		if v.Id != userInfo.Uid && v.PositionShare != 5 {
			continue
		}

		copier.Copy(tp, v.Position)

		user := narrays.Filter(users, func(sv *sso.UserInfo, _ int) bool {
			return sv.Uid == v.Id
		})
		userInfo := new(protos.GetUserPositionAndVehiclePosition_Response_UserInfo)
		if len(user) == 1 {
			userInfo.Uid = user[0].Uid
			userInfo.Avatar = user[0].Avatar
			userInfo.Nickname = user[0].Nickname
		}

		list = append(list, &protos.GetUserPositionAndVehiclePosition_Response_PositionItem{
			Type:     "User",
			Position: tp,
			UserInfo: userInfo,
		})
	}

	for _, v := range vehiclePositionList {
		tp := new(protos.TripPosition)

		// log.Error("vehiclePositionList", vehiclePositionList, v.Id, v.PositionShare)

		if v.AuthorId == userInfo.Uid && v.PositionShare == -1 {
			continue
		}
		if v.AuthorId != userInfo.Uid && v.PositionShare != 5 {
			continue
		}

		copier.Copy(tp, v.Position)

		user := narrays.Filter(users, func(sv *sso.UserInfo, _ int) bool {
			return sv.Uid == v.AuthorId
		})
		userInfo := new(protos.GetUserPositionAndVehiclePosition_Response_UserInfo)
		if len(user) == 1 {
			userInfo.Uid = user[0].Uid
			userInfo.Avatar = user[0].Avatar
			userInfo.Nickname = user[0].Nickname
		}

		list = append(list, &protos.GetUserPositionAndVehiclePosition_Response_PositionItem{
			Type:     "Vehicle",
			Position: tp,
			UserInfo: userInfo,
			VehicleInfo: &protos.GetUserPositionAndVehiclePosition_Response_VehicleInfo{
				Id:       v.Id,
				Logo:     v.Logo,
				Name:     v.Name,
				CarModel: v.CarModel,
				Type:     v.Type,
			},
		})
	}

	res.Data = protos.Encode(&protos.GetUserPositionAndVehiclePosition_Response{
		List:  list,
		Total: nint.ToInt64(len(list)),
	})

	res.Call(c)
}
