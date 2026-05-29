package routerV1

import (
	conf "github.com/ShiinaAiiko/nyanya-trip-route-track/server/config"
	controllersV1 "github.com/ShiinaAiiko/nyanya-trip-route-track/server/controllers/v1"
	"github.com/ShiinaAiiko/nyanya-trip-route-track/server/services/middleware"
)

func (r *Routerv1) InitOpen() {
	tc := new(controllersV1.TripController)
	cc := new(controllersV1.CityController)

	role := middleware.RoleMiddlewareOptions{
		BaseUrl: r.BaseUrl,
	}

	r.Group.GET(
		role.SetRole(conf.ApiNames.Open["GetBaseTripsByOpenAPI"], &middleware.RoleOptionsType{
			CheckApp:           true,
			Authorize:          false,
			RequestEncryption:  false,
			ResponseEncryption: false,
			RequestDataType:    "json",
			ResponseDataType:   "json",
		}),
		tc.GetBaseTripsByOpenAPI)

	r.Group.GET(
		role.SetRole(conf.ApiNames.Open["GetCitiesByOpenAPI"], &middleware.RoleOptionsType{
			CheckApp:           true,
			Authorize:          false,
			RequestEncryption:  false,
			ResponseEncryption: false,
			RequestDataType:    "json",
			ResponseDataType:   "json",
		}),
		cc.GetCitiesByOpenAPI)
}
