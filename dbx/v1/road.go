package dbxV1

import (
	"github.com/ShiinaAiiko/nyanya-trip-route-track/server/models"
)

type RoadDbx struct {
	road *models.TripRoad
}

func (r *RoadDbx) GetRoadId(roadInfo []*models.TripRoadInfo) string {
	id := ""

	for _, v := range roadInfo {
		id += v.Code + v.Type + v.ShortCityName + v.Name.En + v.Name.ZhHans + v.Name.ZhHant
	}
	return id

}
