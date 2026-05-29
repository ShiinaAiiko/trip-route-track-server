package routerV1

import (
	controllersV1 "github.com/ShiinaAiiko/nyanya-trip-route-track/server/controllers/v1"
	"github.com/ShiinaAiiko/nyanya-trip-route-track/server/services/middleware"
)

func (r *Routerv1) InitCity() {
	c := new(controllersV1.CityController)

	role := middleware.RoleMiddlewareOptions{
		BaseUrl: r.BaseUrl,
	}

	r.Group.POST(
		role.SetRole("/city/update", &middleware.RoleOptionsType{
			CheckApp:           false,
			Authorize:          true,
			RequestEncryption:  false,
			ResponseEncryption: false,
			RequestDataType:    "protobuf",
			ResponseDataType:   "protobuf",
		}),
		c.UpdateCity)

	r.Group.GET(
		role.SetRole("/city/details/list/get", &middleware.RoleOptionsType{
			CheckApp:  false,
			Authorize: false,
			// Authorize:          false,
			RequestEncryption:  false,
			ResponseEncryption: false,
			RequestDataType:    "protobuf",
			ResponseDataType:   "protobuf",
		}),
		c.GetCityDetails)

	r.Group.GET(
		role.SetRole("/city/user/list/get", &middleware.RoleOptionsType{
			CheckApp:  false,
			Authorize: false,
			// Authorize:          false,
			RequestEncryption:  false,
			ResponseEncryption: false,
			RequestDataType:    "protobuf",
			ResponseDataType:   "protobuf",
		}),
		c.GetAllCitiesVisitedByUser)

	// r.Group.GET(
	// 	role.SetRole("/city/list/byJmShareKey/get", &middleware.RoleOptionsType{
	// 		CheckApp:  false,
	// 		Authorize: false,
	// 		// Authorize:          false,
	// 		RequestEncryption:  false,
	// 		ResponseEncryption: false,
	// 		RequestDataType:    "protobuf",
	// 		ResponseDataType:   "protobuf",
	// 	}),
	// 	c.GetAllCitiesVisitedByJMShareKey)

}
