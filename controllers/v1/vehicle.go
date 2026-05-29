package controllersV1

import (
	dbxv1 "github.com/ShiinaAiiko/nyanya-trip-route-track/server/dbx/v1"
	"github.com/ShiinaAiiko/nyanya-trip-route-track/server/models"
	"github.com/ShiinaAiiko/nyanya-trip-route-track/server/protos"
	"github.com/ShiinaAiiko/nyanya-trip-route-track/server/services/response"
	"github.com/cherrai/nyanyago-utils/nint"
	"github.com/cherrai/nyanyago-utils/validation"
	sso "github.com/cherrai/saki-sso-go"
	"github.com/gin-gonic/gin"
	"github.com/jinzhu/copier"
)

// "github.com/cherrai/nyanyago-utils/validation"

var (
	vehicleDbx = dbxv1.VehicleDbx{}
)

type VehicleController struct {
}

func (fc *VehicleController) AddVehicle(c *gin.Context) {
	// 1、请求体
	var res response.ResponseProtobufType
	res.Code = 200

	// 2、获取参数
	data := new(protos.AddVehicle_Request)
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
		validation.Parameter(&data.Name, validation.Required(), validation.Type("string")),
		validation.Parameter(&data.Type, validation.Required(), validation.Enum([]string{
			"Bike",
			"Motorcycle",
			"Car",
			"Truck",
			"PublicTransport",
			"Airplane",
			"Other",
		})),
		// validation.Parameter(&data.Logo, validation.Required(), validation.Type("string")),
		// validation.Parameter(&data.LicensePlate, validation.Required(), validation.Type("string")),
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

	vehicle, err := vehicleDbx.AddVehicle(&models.Vehicle{
		Type:          data.Type,
		Name:          data.Name,
		Logo:          data.Logo,
		LicensePlate:  data.LicensePlate,
		CarModel:      data.CarModel,
		AuthorId:      userInfo.Uid,
		Status:        1,
		PositionShare: -1,
		// EndTime:   data.EndTime,
	})
	// log.Info(vehicle)
	if err != nil {
		res.Errors(err)
		res.Code = 10016
		res.Call(c)
		return
	}
	// log.Info(addTrip)

	// authorId := c.MustGet("userInfo").(*sso.UserInfo).Uid

	vi := new(protos.VehicleItem)
	copier.Copy(vi, vehicle)

	protoData := &protos.AddVehicle_Response{
		Vehicle: vi,
	}

	res.Data = protos.Encode(protoData)

	res.Call(c)
}

func (fc *VehicleController) GetVehicles(c *gin.Context) {
	// 1、请求体
	var res response.ResponseProtobufType
	res.Code = 200

	// 2、获取参数
	data := new(protos.GetVehicles_Request)
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
			"All",
			"Bike",
			"Motorcycle",
			"Car",
			"Truck",
			"PublicTransport",
			"Airplane",
			"Other",
		})),
		validation.Parameter(&data.PageNum, validation.GreaterEqual(int64(1)), validation.Required()),
		validation.Parameter(&data.PageSize, validation.NumRange(int64(1), int64(50)), validation.Required()),
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

	log.Info("userInfo", userInfo)

	vehicles, err := vehicleDbx.GetVehicles(userInfo.Uid, data.Type, data.PageNum, data.PageSize)
	log.Info(vehicles)
	if err != nil {
		res.Errors(err)
		res.Code = 10016
		res.Call(c)
		return
	}
	// log.Info(addTrip)

	// authorId := c.MustGet("userInfo").(*sso.UserInfo).Uid

	protoData := &protos.GetVehicles_Response{
		Total: nint.ToInt64(len(vehicles)),
	}

	for _, v := range vehicles {
		vi := new(protos.VehicleItem)
		copier.Copy(vi, v)
		protoData.List = append(protoData.List, vi)
	}

	res.Data = protos.Encode(protoData)

	res.Call(c)
}

func (fc *VehicleController) UpdateVehicle(c *gin.Context) {
	// 1、请求体
	var res response.ResponseProtobufType
	res.Code = 200

	// 2、获取参数
	data := new(protos.UpdateVehicle_Request)
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
		validation.Parameter(&data.Id, validation.Required(), validation.Type("string")),
		validation.Parameter(&data.Name, validation.Required(), validation.Type("string")),
		validation.Parameter(&data.Type, validation.Required(), validation.Enum([]string{
			"Bike",
			"Motorcycle",
			"Car",
			"Truck",
			"PublicTransport",
			"Airplane",
			"Other",
		})),

		validation.Parameter(&data.PositionShare, validation.Enum([]int64{
			5, 1, -1,
		}), validation.Required(), validation.Type("int64")),
		// validation.Parameter(&data.Logo, validation.Required(), validation.Type("string")),
		// validation.Parameter(&data.LicensePlate, validation.Required(), validation.Type("string")),
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

	log.Info("userInfo", userInfo)

	vehicle, err := vehicleDbx.GetVehicle(data.Id, userInfo.Uid, []int64{1})
	if err != nil {
		res.Errors(err)
		res.Code = 10006
		res.Call(c)
		return
	}

	log.Info(vehicle)
	if err = vehicleDbx.UpdateVehicle(
		data.Id, userInfo.Uid, data.Name, data.Logo, data.Type, data.LicensePlate, data.CarModel, data.PositionShare,
	); err != nil {
		res.Errors(err)
		res.Code = 10011
		res.Call(c)
		return
	}
	vehicle.Name = data.Name
	vehicle.Logo = data.Logo
	vehicle.Type = data.Type
	vehicle.LicensePlate = data.LicensePlate
	vehicle.CarModel = data.CarModel
	vehicle.PositionShare = data.PositionShare

	vi := new(protos.VehicleItem)
	copier.Copy(vi, vehicle)

	protoData := &protos.UpdateVehicle_Response{
		Vehicle: vi,
	}

	res.Data = protos.Encode(protoData)

	res.Call(c)
}

func (fc *VehicleController) DeleteVehicle(c *gin.Context) {
	// 1、请求体
	var res response.ResponseProtobufType
	res.Code = 200

	// 2、获取参数
	data := new(protos.DeleteVehicle_Request)
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
		validation.Parameter(&data.Id, validation.Required(), validation.Type("string")),
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

	// log.Info("userInfo", userInfo, data.Id)

	vehicle, err := vehicleDbx.GetVehicle(data.Id, userInfo.Uid, []int64{1})
	if err != nil {
		res.Errors(err)
		res.Code = 10006
		res.Call(c)
		return
	}

	if err = vehicleDbx.DeleteVehicle(
		data.Id, userInfo.Uid,
	); err != nil {
		res.Errors(err)
		res.Code = 10017
		res.Call(c)
		return
	}

	vi := new(protos.VehicleItem)
	copier.Copy(vi, vehicle)

	protoData := &protos.DeleteVehicle_Response{}

	res.Data = protos.Encode(protoData)

	res.Call(c)
}
