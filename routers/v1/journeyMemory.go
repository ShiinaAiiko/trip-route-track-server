package routerV1

import (
	conf "github.com/ShiinaAiiko/nyanya-trip-route-track/server/config"
	controllersV1 "github.com/ShiinaAiiko/nyanya-trip-route-track/server/controllers/v1"
	"github.com/ShiinaAiiko/nyanya-trip-route-track/server/services/middleware"
)

func (r *Routerv1) InitJourneyMemory() {
	c := new(controllersV1.JourneyMemoryController)

	role := middleware.RoleMiddlewareOptions{
		BaseUrl: r.BaseUrl,
	}
	r.Group.POST(
		role.SetRole("/journeyMemory/add", &middleware.RoleOptionsType{
			CheckApp:           false,
			Authorize:          true,
			RequestEncryption:  false,
			ResponseEncryption: false,
			RequestDataType:    "protobuf",
			ResponseDataType:   "protobuf",
		}),
		c.AddJM)

	r.Group.POST(
		role.SetRole("/journeyMemory/update", &middleware.RoleOptionsType{
			CheckApp:           false,
			Authorize:          true,
			RequestEncryption:  false,
			ResponseEncryption: false,
			RequestDataType:    "protobuf",
			ResponseDataType:   "protobuf",
		}),
		c.UpdateJM)

	r.Group.GET(
		role.SetRole("/journeyMemory/detail/get", &middleware.RoleOptionsType{
			CheckApp:           false,
			Authorize:          false,
			RequestEncryption:  false,
			ResponseEncryption: false,
			RequestDataType:    "protobuf",
			ResponseDataType:   "protobuf",
		}),
		c.GetJMDetail)

	r.Group.GET(
		role.SetRole("/journeyMemory/list/get", &middleware.RoleOptionsType{
			CheckApp:  false,
			Authorize: true,
			// Authorize:          false,
			RequestEncryption:  false,
			ResponseEncryption: false,
			RequestDataType:    "protobuf",
			ResponseDataType:   "protobuf",
		}),
		c.GetJMList)

	r.Group.POST(
		role.SetRole("/journeyMemory/delete", &middleware.RoleOptionsType{
			CheckApp:           false,
			Authorize:          true,
			RequestEncryption:  false,
			ResponseEncryption: false,
			RequestDataType:    "protobuf",
			ResponseDataType:   "protobuf",
		}),
		c.DeleteJM)

	r.Group.POST(
		role.SetRole(conf.ApiNames.JourneyMemoryTimeline["AddJMTimeline"], &middleware.RoleOptionsType{
			CheckApp:           false,
			Authorize:          true,
			RequestEncryption:  false,
			ResponseEncryption: false,
			RequestDataType:    "protobuf",
			ResponseDataType:   "protobuf",
		}),
		c.AddJMTimeline)

	r.Group.POST(
		role.SetRole(conf.ApiNames.JourneyMemoryTimeline["UpdateJMTimeline"], &middleware.RoleOptionsType{
			CheckApp:           false,
			Authorize:          true,
			RequestEncryption:  false,
			ResponseEncryption: false,
			RequestDataType:    "protobuf",
			ResponseDataType:   "protobuf",
		}),
		c.UpdateJMTimeline)

	r.Group.GET(
		role.SetRole(conf.ApiNames.JourneyMemoryTimeline["GetJMTimelineList"], &middleware.RoleOptionsType{
			CheckApp:           false,
			Authorize:          false,
			RequestEncryption:  false,
			ResponseEncryption: false,
			RequestDataType:    "protobuf",
			ResponseDataType:   "protobuf",
		}),
		c.GetJMTimelineList)

	r.Group.POST(
		role.SetRole(conf.ApiNames.JourneyMemoryTimeline["DeleteJMTimeline"], &middleware.RoleOptionsType{
			CheckApp:           false,
			Authorize:          true,
			RequestEncryption:  false,
			ResponseEncryption: false,
			RequestDataType:    "protobuf",
			ResponseDataType:   "protobuf",
		}),
		c.DeleteJMTimeline)

}
