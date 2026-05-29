package controllersV1

import (
	dbxV1 "github.com/ShiinaAiiko/nyanya-trip-route-track/server/dbx/v1"
	"github.com/ShiinaAiiko/nyanya-trip-route-track/server/models"
	"github.com/ShiinaAiiko/nyanya-trip-route-track/server/protos"
	"github.com/ShiinaAiiko/nyanya-trip-route-track/server/services/middleware"
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
	roadbookDbx = dbxV1.RoadbookDbx{}
)

type RoadbookController struct {
}

func (fc *RoadbookController) AddRoadbook(c *gin.Context) {
	// 1、请求体
	var res response.ResponseProtobufType
	res.Code = 200

	// 2、获取参数
	data := new(protos.AddRoadbook_Request)
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
		validation.Parameter(&data.Title, validation.Type("string"), validation.Required()),
		validation.Parameter(&data.Desc, validation.Type("string")),
		validation.Parameter(&data.StartTime, validation.Type("int64"), validation.Required()),
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
	authorId := userInfo.Uid

	// log.Info("userInfo", userInfo)

	rb, err := roadbookDbx.AddRoadbook(&models.Roadbook{
		Title:     data.Title,
		Desc:      data.Desc,
		StartTime: data.StartTime,
		AuthorId:  authorId,
	})
	log.Info("AddRoadbook", rb, err)
	if err != nil {
		res.Errors(err)
		res.Code = 10016
		res.Call(c)
		return
	}

	// authorId := c.MustGet("userInfo").(*sso.UserInfo).Uid

	rbProto := new(protos.RoadbookItem)

	if err := copier.Copy(rbProto, rb); err != nil {
		res.Errors(err)
		res.Code = 10016
		res.Call(c)
		return
	}

	protoData := &protos.AddRoadbook_Response{
		Roadbook: rbProto,
	}

	res.Data = protos.Encode(protoData)

	res.Call(c)
}

func (fc *RoadbookController) GetRoadbookList(c *gin.Context) {
	// 1、请求体
	var res response.ResponseProtobufType
	res.Code = 200

	// 2、获取参数
	data := new(protos.GetRoadbookList_Request)
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
		validation.Parameter(&data.Ids, validation.Type("[]string")),
		validation.Parameter(&data.PageNum, validation.GreaterEqual(int32(1)), validation.Required()),
		validation.Parameter(&data.PageSize, validation.NumRange(int32(1), int32(50)), validation.Required()),
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

	rbList, err := roadbookDbx.GetRoadbookList(
		userInfo.Uid,
		data.Ids,
		data.PageNum, data.PageSize)
	// log.Info("GetRoadbookList", rb, err)
	if err != nil {
		res.Errors(err)
		res.Code = 10016
		res.Call(c)
		return
	}
	if len(rbList) == 0 {
		res.Errors(err)
		res.Code = 10006
		res.Call(c)
		return

	}

	// authorId := c.MustGet("userInfo").(*sso.UserInfo).Uid

	rbListProto := []*protos.RoadbookItem{}

	for _, v := range rbList {
		rbProto := new(protos.RoadbookItem)
		if err := copier.Copy(rbProto, v); err != nil {
			res.Errors(err)
			res.Code = 10016
			res.Call(c)
			return
		}
		rbListProto = append(rbListProto, rbProto)

	}

	protoData := &protos.GetRoadbookList_Response{
		List:  rbListProto,
		Total: nint.ToInt32(len(rbListProto)),
	}

	res.Data = protos.Encode(protoData)

	res.Call(c)
}

func (fc *RoadbookController) UpdateRoadbook(c *gin.Context) {
	// 1、请求体
	var res response.ResponseProtobufType
	res.Code = 200

	// 2、获取参数
	data := new(protos.UpdateRoadbook_Request)
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
		validation.Parameter(&data.Title, validation.Type("string")),
		validation.Parameter(&data.Desc, validation.Type("string")),
		validation.Parameter(&data.StartTime, validation.GreaterEqual(int64(1)), validation.Required()),
		validation.Parameter(&data.Timelines),
		validation.Parameter(&data.Permissions),
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
	authorId := userInfo.Uid

	// log.Info("userInfo", userInfo)

	if err = roadbookDbx.UpdateRoadbook(
		data.Id,
		authorId,
		data.Title,
		data.Desc,
		data.StartTime,

		narrays.Map(data.Timelines, func(v *protos.RoadbookTimelineItem, index int) *models.RoadbookTimeLineItem {
			return &models.RoadbookTimeLineItem{
				Id:    v.Id,
				Title: v.Title,
				Desc:  v.Desc,
				Days:  v.Days,
				Waypoints: narrays.Map(v.Waypoints, func(sv *protos.RoadbookWaypointItem, index int) *models.RoadbookWaypointItem {
					waypoints := &models.RoadbookWaypointItem{
						Id: sv.Id,
						Coords: &models.RoadbookWaypointCoords{
							Latitude:  sv.Coords.Latitude,
							Longitude: sv.Coords.Longitude,
						},
						City: &models.RoadbookWaypointCity{
							Country: sv.City.Country,
							State:   sv.City.State,
							Region:  sv.City.Region,
							City:    sv.City.City,
							Town:    sv.City.Town,
							Road:    sv.City.Road,
						},
						Address:            sv.Address,
						Icon:               sv.Icon,
						LastNavigationTime: nint.Int64Or(sv.LastNavigationTime, -1),
					}

					if sv.Navigation != nil {
						waypoints.Navigation = &models.RoadbookWaypointNavigation{
							Distance:   sv.Navigation.Distance,
							Duration:   sv.Navigation.Duration,
							TravelMode: sv.Navigation.TravelMode,
							Urls: &models.RoadbookWaypointNavigationUrls{
								DomainUrl: sv.Navigation.Urls.DomainUrl,
								ShortUrl:  sv.Navigation.Urls.ShortUrl,
								Url:       sv.Navigation.Urls.Url,
							},
						}
					} else {
						waypoints.Navigation = &models.RoadbookWaypointNavigation{
							Distance:   0,
							Duration:   0,
							TravelMode: "",
							Urls: &models.RoadbookWaypointNavigationUrls{
								DomainUrl: "",
								ShortUrl:  "",
								Url:       "",
							},
						}

					}
					return waypoints
				}),
			}
		}),

		&models.RoadbookPermissions{
			AllowShare: data.Permissions.AllowShare,
		},
	); err != nil {
		res.Errors(err)
		res.Code = 10011
		res.Call(c)
		return
	}

	protoData := &protos.UpdateRoadbook_Response{}

	res.Data = protos.Encode(protoData)

	res.Call(c)
}

func (fc *RoadbookController) DeleteRoadbook(c *gin.Context) {
	// 1、请求体
	var res response.ResponseProtobufType
	res.Code = 200

	// 2、获取参数
	data := new(protos.DeleteRoadbook_Request)
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

	userInfoAny, exists := c.Get("userInfo")
	if !exists {
		res.Errors(err)
		res.Code = 10004
		res.Call(c)
		return
	}
	userInfo := userInfoAny.(*sso.UserInfo)
	authorId := userInfo.Uid

	// log.Info("userInfo", userInfo)

	if err = roadbookDbx.DeleteRoadbook(data.Id, authorId); err != nil {
		res.Errors(err)
		res.Code = 10017
		res.Call(c)
		return
	}

	protoData := &protos.DeleteRoadbook_Response{}

	res.Data = protos.Encode(protoData)

	res.Call(c)
}

func (fc *RoadbookController) GetRoadbookDetail(c *gin.Context) {
	// 1、请求体
	var res response.ResponseProtobufType
	res.Code = 200

	// 2、获取参数
	data := new(protos.GetRoadbookDetail_Request)
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
		validation.Parameter(&data.Id, validation.Type("string"), validation.Required()),
		// validation.Parameter(&data.ShareKey, validation.Type("string")),
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

	rb, err := roadbookDbx.GetRoadbook(
		data.Id, authorId,
	)
	log.Info("GetRoadbook", rb, err)
	if err != nil || rb == nil {
		res.Errors(err)
		res.Code = 10006
		res.Call(c)
		return
	}

	if authorId == "" {
		authorId = rb.AuthorId
	}

	rbProto := new(protos.RoadbookItem)

	if err := copier.Copy(rbProto, rb); err != nil {
		res.Errors(err)
		res.Code = 10016
		res.Call(c)
		return
	}

	protoData := &protos.GetRoadbookDetail_Response{
		Roadbook: rbProto,
	}

	res.Data = protos.Encode(protoData)

	res.Call(c)
}
