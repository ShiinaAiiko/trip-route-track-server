package controllersV1

import (
	conf "github.com/ShiinaAiiko/nyanya-trip-route-track/server/config"
	dbxV1 "github.com/ShiinaAiiko/nyanya-trip-route-track/server/dbx/v1"
	"github.com/ShiinaAiiko/nyanya-trip-route-track/server/models"
	"github.com/ShiinaAiiko/nyanya-trip-route-track/server/protos"
	"github.com/ShiinaAiiko/nyanya-trip-route-track/server/services/response"
	"github.com/cherrai/nyanyago-utils/narrays"
	"github.com/cherrai/nyanyago-utils/validation"
	sso "github.com/cherrai/saki-sso-go"
	"github.com/gin-gonic/gin"
	"github.com/mitchellh/mapstructure"
)

// "github.com/cherrai/nyanyago-utils/validation"
var (
	pgpDbx = dbxV1.PrivacyGeofencePointsDbx{}
)

type PrivacyGeofenceController struct {
}

func getUserPGP(userId string) (*models.PrivacyGeofencePoints, error) {
	var pgp *models.PrivacyGeofencePoints

	appData := conf.SSO.AppData.SetUserId(userId)

	userRes, err := conf.SSO.GetTempJwtToken(userId)
	// log.Info("		configureAny, err", userRes, err)
	if err != nil || userRes == nil {
		return nil, err
	}

	configureAny, err := appData.Get("configure", userRes.Token,
		userRes.DeviceId, userRes.UserAgent)
	// log.Info("		configureAny, err", configureAny, err)

	if err == nil && configureAny != nil {
		configure := new(protos.Configure)

		err = mapstructure.Decode(configureAny, configure)
		// log.Info("configure", configure, configure.General.EnableGeoFence)
		if err == nil && configure.General.EnableGeoFence {
			pgp, err = pgpDbx.GetPGP(userId)
			if err != nil || pgp == nil {
				return nil, err
			}

		}
	}

	return pgp, nil
}

func (fc *PrivacyGeofenceController) SetPrivacyGeofence(c *gin.Context) {
	// 1、请求体
	var res response.ResponseProtobufType
	res.Code = 200

	// 2、获取参数
	data := new(protos.SetPrivacyGeofence_Request)
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

	savePGP, err := pgpDbx.SetPGP(userInfo.Uid, narrays.Map(data.Points, func(v *protos.PrivacyGeofencePointsItem, index int) *models.PrivacyGeofencePointsItem {
		return &models.PrivacyGeofencePointsItem{
			Id: v.Id,
			Coords: narrays.Map(v.Coords, func(sv *protos.PrivacyGeofencePointsItem_Coords, index int) *models.PrivacyGeofencePointsItemCoords {

				return &models.PrivacyGeofencePointsItemCoords{
					Latitude:  sv.Latitude,
					Longitude: sv.Longitude,
				}
			}),
			CreateTime:     v.CreateTime,
			LastUpdateTime: v.LastUpdateTime,
		}
	}))
	log.Info(savePGP, err)
	if err != nil || savePGP == nil {
		res.Errors(err)
		res.Code = 10016
		res.Call(c)
		return
	}

	protoData := &protos.SetPrivacyGeofence_Response{}
	res.Data = protos.Encode(protoData)

	res.Call(c)
}

func (fc *PrivacyGeofenceController) GetPrivacyGeofence(c *gin.Context) {
	// 1、请求体
	var res response.ResponseProtobufType
	res.Code = 200

	// 2、获取参数
	data := new(protos.GetConfigure_Request)
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

	pgp, err := pgpDbx.GetPGP(userInfo.Uid)
	if err != nil || pgp == nil {
		res.Errors(err)
		res.Code = 10016
		res.Call(c)
		return
	}

	protoData := &protos.GetPrivacyGeofence_Response{
		Points: narrays.Map(pgp.Points, func(v *models.PrivacyGeofencePointsItem, index int) *protos.PrivacyGeofencePointsItem {
			return &protos.PrivacyGeofencePointsItem{
				Id: v.Id,
				Coords: narrays.Map(v.Coords, func(sv *models.PrivacyGeofencePointsItemCoords, index int) *protos.PrivacyGeofencePointsItem_Coords {

					return &protos.PrivacyGeofencePointsItem_Coords{
						Latitude:  sv.Latitude,
						Longitude: sv.Longitude,
					}
				}),
				CreateTime:     v.CreateTime,
				LastUpdateTime: v.LastUpdateTime,
			}
		}),
	}
	res.Data = protos.Encode(protoData)

	res.Call(c)
}
