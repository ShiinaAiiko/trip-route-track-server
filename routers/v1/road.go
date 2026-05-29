package routerV1

import (
	controllersV1 "github.com/ShiinaAiiko/nyanya-trip-route-track/server/controllers/v1"
	"github.com/ShiinaAiiko/nyanya-trip-route-track/server/services/middleware"
)

func (r *Routerv1) InitRoad() {
	c := new(controllersV1.RoadController)

	role := middleware.RoleMiddlewareOptions{
		BaseUrl: r.BaseUrl,
	}

	r.Group.POST(
		role.SetRole("/road/update", &middleware.RoleOptionsType{
			CheckApp:           false,
			Authorize:          true,
			RequestEncryption:  false,
			ResponseEncryption: false,
			RequestDataType:    "protobuf",
			ResponseDataType:   "protobuf",
		}),
		c.UpdateRoad)

}
