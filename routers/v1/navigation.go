package routerV1

import (
	controllersV1 "github.com/ShiinaAiiko/nyanya-trip-route-track/server/controllers/v1"
	"github.com/ShiinaAiiko/nyanya-trip-route-track/server/services/middleware"
)

func (r *Routerv1) InitNavigation() {
	c := new(controllersV1.NavigationController)

	role := middleware.RoleMiddlewareOptions{
		BaseUrl: r.BaseUrl,
	}
	r.Group.GET(
		role.SetRole("/navigation/get", &middleware.RoleOptionsType{
			CheckApp:           false,
			Authorize:          true,
			RequestEncryption:  false,
			ResponseEncryption: false,
			RequestDataType:    "protobuf",
			ResponseDataType:   "protobuf",
		}),
		c.GetNavigationData)

}
