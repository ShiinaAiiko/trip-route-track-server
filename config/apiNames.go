package conf

type ApiNamesVal = map[string]string

type ApiNamesType struct {
	File                  ApiNamesVal
	Open                  ApiNamesVal
	JourneyMemory         ApiNamesVal
	JourneyMemoryTimeline ApiNamesVal
}

var ApiNames = ApiNamesType{
	File: ApiNamesVal{
		"GetUploadToken": "/file/getUploadToken",
		"GetAppToken":    "/file/appToken/get",
	},
	Open: ApiNamesVal{
		"GetBaseTripsByOpenAPI": "/open/trip/base/list/get",
		"GetCitiesByOpenAPI":    "/open/city/list/get",
	},
	JourneyMemoryTimeline: ApiNamesVal{
		"AddJMTimeline":     "/journeyMemory/timeline/add",
		"UpdateJMTimeline":  "/journeyMemory/timeline/update",
		"GetJMTimelineList": "/journeyMemory/timeline/list/get",
		"DeleteJMTimeline":  "/journeyMemory/timeline/delete",
	},
}
