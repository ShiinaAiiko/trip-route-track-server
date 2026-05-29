package routerV1

import (
	conf "github.com/ShiinaAiiko/nyanya-trip-route-track/server/config"
	controllersV1 "github.com/ShiinaAiiko/nyanya-trip-route-track/server/controllers/v1"
	"github.com/ShiinaAiiko/nyanya-trip-route-track/server/services/middleware"
)

func (r Routerv1) InitFile() {
	fc := new(controllersV1.FileController)

	role := middleware.RoleMiddlewareOptions{
		BaseUrl: r.BaseUrl,
	}
	r.Group.GET(
		role.SetRole(conf.ApiNames.File["GetUploadToken"], &middleware.RoleOptionsType{
			CheckApp:           false,
			Authorize:          true,
			RequestEncryption:  false,
			ResponseEncryption: false,
			RequestDataType:    "protobuf",
			ResponseDataType:   "protobuf",
		}),
		fc.GetUploadToken)

	r.Group.POST(
		role.SetRole(conf.ApiNames.File["GetAppToken"], &middleware.RoleOptionsType{
			CheckApp:           false,
			Authorize:          true,
			RequestEncryption:  false,
			ResponseEncryption: false,
			RequestDataType:    "protobuf",
			ResponseDataType:   "protobuf",
		}),
		fc.GetAppToken)
}
