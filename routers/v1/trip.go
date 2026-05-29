package routerV1

import (
	controllersV1 "github.com/ShiinaAiiko/nyanya-trip-route-track/server/controllers/v1"
	"github.com/ShiinaAiiko/nyanya-trip-route-track/server/services/middleware"
)

func (r *Routerv1) InitTrip() {
	tc := new(controllersV1.TripController)

	role := middleware.RoleMiddlewareOptions{
		BaseUrl: r.BaseUrl,
	}
	r.Group.POST(
		role.SetRole("/trip/cities/clear", &middleware.RoleOptionsType{
			CheckApp:           false,
			Authorize:          true,
			RequestEncryption:  false,
			ResponseEncryption: false,
			RequestDataType:    "protobuf",
			ResponseDataType:   "protobuf",
		}),
		tc.ClearTripCities)
	r.Group.POST(
		role.SetRole("/trip/roads/clear", &middleware.RoleOptionsType{
			CheckApp:           false,
			Authorize:          true,
			RequestEncryption:  false,
			ResponseEncryption: false,
			RequestDataType:    "protobuf",
			ResponseDataType:   "protobuf",
		}),
		tc.ClearTripRoads)
	r.Group.POST(
		role.SetRole("/trip/add", &middleware.RoleOptionsType{
			CheckApp:           false,
			Authorize:          true,
			RequestEncryption:  false,
			ResponseEncryption: false,
			RequestDataType:    "protobuf",
			ResponseDataType:   "protobuf",
		}),
		tc.AddTrip)

	r.Group.POST(
		role.SetRole("/trip/mark/add", &middleware.RoleOptionsType{
			CheckApp:           false,
			Authorize:          true,
			RequestEncryption:  false,
			ResponseEncryption: false,
			RequestDataType:    "protobuf",
			ResponseDataType:   "protobuf",
		}),
		tc.AddTripMark)

	r.Group.POST(
		role.SetRole("/trip/addTripToOnline", &middleware.RoleOptionsType{
			CheckApp:           false,
			Authorize:          true,
			RequestEncryption:  false,
			ResponseEncryption: false,
			RequestDataType:    "protobuf",
			ResponseDataType:   "protobuf",
		}),
		tc.AddTripToOnline)

	r.Group.POST(
		role.SetRole("/trip/position/update", &middleware.RoleOptionsType{
			CheckApp:           false,
			Authorize:          true,
			RequestEncryption:  false,
			ResponseEncryption: false,
			RequestDataType:    "protobuf",
			ResponseDataType:   "protobuf",
		}),
		tc.UpdateTripPosition)

	r.Group.POST(
		role.SetRole("/trip/finish", &middleware.RoleOptionsType{
			CheckApp:           false,
			Authorize:          true,
			RequestEncryption:  false,
			ResponseEncryption: false,
			RequestDataType:    "protobuf",
			ResponseDataType:   "protobuf",
		}),
		tc.FinishTrip)

	r.Group.POST(
		role.SetRole("/trip/correctedData", &middleware.RoleOptionsType{
			CheckApp:           false,
			Authorize:          true,
			RequestEncryption:  false,
			ResponseEncryption: false,
			RequestDataType:    "protobuf",
			ResponseDataType:   "protobuf",
		}),
		tc.CorrectedTripData)

	r.Group.POST(
		role.SetRole("/trip/update", &middleware.RoleOptionsType{
			CheckApp:           false,
			Authorize:          true,
			RequestEncryption:  false,
			ResponseEncryption: false,
			RequestDataType:    "protobuf",
			ResponseDataType:   "protobuf",
		}),
		tc.UpdateTrip)

	r.Group.POST(
		role.SetRole("/trip/list/update", &middleware.RoleOptionsType{
			CheckApp:           false,
			Authorize:          true,
			RequestEncryption:  false,
			ResponseEncryption: false,
			RequestDataType:    "protobuf",
			ResponseDataType:   "protobuf",
		}),
		tc.UpdateTrips)

	r.Group.POST(
		role.SetRole("/trip/delete", &middleware.RoleOptionsType{
			CheckApp:           false,
			Authorize:          true,
			RequestEncryption:  false,
			ResponseEncryption: false,
			RequestDataType:    "protobuf",
			ResponseDataType:   "protobuf",
		}),
		tc.DeleteTrip)

	r.Group.GET(
		role.SetRole("/trip/get", &middleware.RoleOptionsType{
			CheckApp:           false,
			Authorize:          false,
			RequestEncryption:  false,
			ResponseEncryption: false,
			RequestDataType:    "protobuf",
			ResponseDataType:   "protobuf",
		}),
		tc.GetTrip)

	r.Group.GET(
		role.SetRole("/trip/positions/get", &middleware.RoleOptionsType{
			CheckApp:           false,
			Authorize:          false,
			RequestEncryption:  false,
			ResponseEncryption: false,
			RequestDataType:    "protobuf",
			ResponseDataType:   "protobuf",
		}),
		tc.GetTripPositions)

	r.Group.GET(
		role.SetRole("/trip/history/positions/get", &middleware.RoleOptionsType{
			CheckApp:           false,
			Authorize:          false,
			RequestEncryption:  false,
			ResponseEncryption: false,
			RequestDataType:    "protobuf",
			ResponseDataType:   "json",
		}),
		tc.GetTripHistoryPositions)

	r.Group.GET(
		role.SetRole("/trip/list/get", &middleware.RoleOptionsType{
			CheckApp:           false,
			Authorize:          true,
			RequestEncryption:  false,
			ResponseEncryption: false,
			RequestDataType:    "protobuf",
			ResponseDataType:   "protobuf",
		}),
		tc.GetTrips)

	r.Group.GET(
		role.SetRole("/trip/statistics/get", &middleware.RoleOptionsType{
			CheckApp:           false,
			Authorize:          true,
			RequestEncryption:  false,
			ResponseEncryption: false,
			RequestDataType:    "protobuf",
			ResponseDataType:   "protobuf",
		}),
		tc.GetTripStatistics)

	// r.Group.GET(
	// 	role.SetRole("/trip/historicalStatistics/get", &middleware.RoleOptionsType{
	// 		CheckApp:           false,
	// 		Authorize:          true,
	// 		RequestEncryption:  false,
	// 		ResponseEncryption: false,
	// 		RequestDataType:    "protobuf",
	// 		ResponseDataType:   "protobuf",
	// 	}),
	// 	tc.GetHistoricalStatistics)

	r.Group.POST(
		role.SetRole("/trip/resume", &middleware.RoleOptionsType{
			CheckApp:           false,
			Authorize:          true,
			RequestEncryption:  false,
			ResponseEncryption: false,
			RequestDataType:    "protobuf",
			ResponseDataType:   "protobuf",
		}),
		tc.ResumeTrip)

	r.Group.POST(
		role.SetRole("/trip/addresses/update", &middleware.RoleOptionsType{
			CheckApp:           false,
			Authorize:          true,
			RequestEncryption:  false,
			ResponseEncryption: false,
			RequestDataType:    "protobuf",
			ResponseDataType:   "protobuf",
		}),
		tc.UpdateTripAddresses)

	r.Group.POST(
		role.SetRole("/trip/networkStatus/update", &middleware.RoleOptionsType{
			CheckApp:           false,
			Authorize:          true,
			RequestEncryption:  false,
			ResponseEncryption: false,
			RequestDataType:    "protobuf",
			ResponseDataType:   "protobuf",
		}),
		tc.UpdateTripNetworkStatus)

	r.Group.POST(
		role.SetRole("/trip/weather/update", &middleware.RoleOptionsType{
			CheckApp:           false,
			Authorize:          true,
			RequestEncryption:  false,
			ResponseEncryption: false,
			RequestDataType:    "protobuf",
			ResponseDataType:   "protobuf",
		}),
		tc.UpdateTripWeather)
}
