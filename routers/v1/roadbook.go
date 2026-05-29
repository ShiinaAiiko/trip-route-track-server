package routerV1

import (
	controllersV1 "github.com/ShiinaAiiko/nyanya-trip-route-track/server/controllers/v1"
	"github.com/ShiinaAiiko/nyanya-trip-route-track/server/services/middleware"
)

func (r *Routerv1) InitRoadbook() {
	c := new(controllersV1.RoadbookController)

	role := middleware.RoleMiddlewareOptions{
		BaseUrl: r.BaseUrl,
	}
	r.Group.POST(
		role.SetRole("/roadbook/add", &middleware.RoleOptionsType{
			CheckApp:           false,
			Authorize:          true,
			RequestEncryption:  false,
			ResponseEncryption: false,
			RequestDataType:    "protobuf",
			ResponseDataType:   "protobuf",
		}),
		c.AddRoadbook)

	r.Group.GET(
		role.SetRole("/roadbook/list/get", &middleware.RoleOptionsType{
			CheckApp:  false,
			Authorize: true,
			// Authorize:          false,
			RequestEncryption:  false,
			ResponseEncryption: false,
			RequestDataType:    "protobuf",
			ResponseDataType:   "protobuf",
		}),
		c.GetRoadbookList)

	r.Group.POST(
		role.SetRole("/roadbook/update", &middleware.RoleOptionsType{
			CheckApp:           false,
			Authorize:          true,
			RequestEncryption:  false,
			ResponseEncryption: false,
			RequestDataType:    "protobuf",
			ResponseDataType:   "protobuf",
		}),
		c.UpdateRoadbook)

	r.Group.POST(
		role.SetRole("/roadbook/delete", &middleware.RoleOptionsType{
			CheckApp:           false,
			Authorize:          true,
			RequestEncryption:  false,
			ResponseEncryption: false,
			RequestDataType:    "protobuf",
			ResponseDataType:   "protobuf",
		}),
		c.DeleteRoadbook)

	r.Group.GET(
		role.SetRole("/roadbook/detail/get", &middleware.RoleOptionsType{
			CheckApp:           false,
			Authorize:          false,
			RequestEncryption:  false,
			ResponseEncryption: false,
			RequestDataType:    "protobuf",
			ResponseDataType:   "protobuf",
		}),
		c.GetRoadbookDetail)
}
