package dbxV1

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sort"
	"time"

	conf "github.com/ShiinaAiiko/nyanya-trip-route-track/server/config"
	"github.com/ShiinaAiiko/nyanya-trip-route-track/server/models"
	"github.com/ShiinaAiiko/nyanya-trip-route-track/server/protos"
	"github.com/ShiinaAiiko/nyanya-trip-route-track/server/services/methods"
	"github.com/cherrai/nyanyago-utils/cipher"
	"github.com/cherrai/nyanyago-utils/narrays"
	"github.com/cherrai/nyanyago-utils/ncommon"
	"github.com/cherrai/nyanyago-utils/nlog"
	"github.com/cherrai/nyanyago-utils/nshortid"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
)

var (
	log     = nlog.New()
	tripDbx = TripDbx{}
)

type TripDbx struct {
}

func (t *TripDbx) GetShortId(digits int) string {
	str := nshortid.GetShortId(digits)

	shortUrl, err := t.GetTrip(str, "")
	if shortUrl == nil || err != nil {
		return str
	}
	return t.GetShortId(digits)
}

// func (t *TripDbx) GetShareKey(digits int) string {
// 	str := nshortid.GetShortId(digits)

// 	shareKey, err := t.GetTripByShareKey(str)
// 	if shareKey == "" || err != nil {
// 		return str
// 	}
// 	return t.GetShortId(digits)
// }

func (t *TripDbx) AddTrip(trip *models.Trip) (*models.Trip, error) {
	// 1、插入数据
	trip.Id = t.GetShortId(9)

	err := trip.Default()
	if err != nil {
		return nil, err
	}
	// log.Info("shortUrl", shortUrl)

	_, err = trip.GetCollection().InsertOne(context.TODO(), trip)
	if err != nil {
		return nil, err
	}
	// log.Info("shortUrl", shortUrl)

	writeGPSFile(trip, false)
	return trip, nil
}

func (t *TripDbx) AddTripMark(id, authorId string,
	mark *models.TripMark) error {
	trip := new(models.Trip)

	setUp := bson.M{}

	updateResult, err := trip.GetCollection().UpdateOne(context.TODO(),
		bson.M{
			"$and": []bson.M{
				{
					"_id":      id,
					"authorId": authorId,
					"status":   0,
				},
			},
		}, bson.M{
			"$push": bson.M{
				"marks": mark,
			},
			"$set": setUp,
		}, options.Update().SetUpsert(false))

	if err != nil {
		return err
	}
	if updateResult.ModifiedCount == 0 {
		return errors.New("update fail")
	}

	// 删除对应redis
	t.DeleteRedisData(authorId, id)
	return nil
}

func (t *TripDbx) UpdateTripCities(id, authorId string,
	cities []*models.TripCity) error {
	trip := new(models.Trip)

	setUp := bson.M{
		"cities":         cities,
		"lastUpdateTime": time.Now().Unix(),
	}

	updateResult, err := trip.GetCollection().UpdateOne(context.TODO(),
		bson.M{
			"$and": []bson.M{
				{
					"_id":      id,
					"authorId": authorId,
					// "status":   0,
				},
			},
		}, bson.M{
			"$set": setUp,
		}, options.Update().SetUpsert(false))

	if err != nil {
		return err
	}
	if updateResult.ModifiedCount == 0 {
		return errors.New("update fail")
	}

	// key := conf.Redisdb.GetKey("GetTrip")

	// err = conf.Redisdb.GetStruct(key.GetKey(id), trip)
	// if err == nil && trip != nil {
	// 	trip.Cities = cities
	// }
	// err = conf.Redisdb.SetStruct(key.GetKey(id), trip, key.GetExpiration())
	// if err != nil {
	// 	log.Info(err)
	// }

	t.DeleteRedisData(authorId, id)

	return nil
}

func (t *TripDbx) UpdateTripRoads(id, authorId string,
	roads []*models.TripRoad) error {
	trip := new(models.Trip)

	setUp := bson.M{
		"roads":          roads,
		"lastUpdateTime": time.Now().Unix(),
	}

	updateResult, err := trip.GetCollection().UpdateOne(context.TODO(),
		bson.M{
			"$and": []bson.M{
				{
					"_id":      id,
					"authorId": authorId,
					// "status":   0,
				},
			},
		}, bson.M{
			"$set": setUp,
		}, options.Update().SetUpsert(false))

	if err != nil {
		return err
	}
	if updateResult.ModifiedCount == 0 {
		return errors.New("update fail")
	}

	t.DeleteRedisData(authorId, id)

	return nil
}

func (t *TripDbx) UpdateTripPosition(authorId, id string, positions []*models.TripPosition, distance float64) error {
	trip, err := t.GetTrip(id, authorId)
	if err != nil {
		return err
	}
	if len(positions) == 0 {
		return errors.New("positions cannot be empty")
	}

	setUp := bson.M{}

	if distance > 0 {
		setUp["statistics.distance"] = distance
	}

	updateResult, err := trip.GetCollection().UpdateOne(context.TODO(),
		bson.M{
			"$and": []bson.M{
				{
					"_id":      id,
					"authorId": authorId,
					"status":   0,
				},
			},
		}, bson.M{
			// "$push": bson.M{
			// 	"positions": bson.M{
			// 		"$each": positions,
			// 	},
			// },
			"$set": setUp,
		}, options.Update().SetUpsert(false))

	if err != nil {
		return err
	}

	log.Info("updateResult", updateResult, distance)

	// if updateResult.ModifiedCount == 0 {
	// 	return errors.New("update fail")
	// }

	// 1、获取positions
	if _, err := readGPSFile(trip); err != nil {
		log.Error("readGPSFile", err)
		return err
	}

	// 2、更新positions

	// old
	// trip.Positions = append(trip.Positions, positions...)
	// log.Info(len(trip.Positions))

	// new

	timeAll := narrays.Map(trip.Positions, func(pos *models.TripPosition, i int) int64 {
		return pos.Timestamp
	})

	// log.Info("writeGPSFile1", trip.CreateTime, len(trip.Positions), trip.Id)
	for _, pos := range positions {
		// if pos.Timestamp == 1757549632000 {
		// 	log.Info("!narrays.Includes(timeAll, pos.Timestamp)", !narrays.Includes(timeAll, pos.Timestamp))
		// }
		if !narrays.Includes(timeAll, pos.Timestamp) {
			trip.Positions = append(trip.Positions, pos)
		}
	}
	// log.Info("writeGPSFile2", trip.CreateTime, len(trip.Positions), trip.Id)
	sort.Slice(trip.Positions, func(a, b int) bool {
		return trip.Positions[a].Timestamp < trip.Positions[b].Timestamp
	})
	// log.Info("writeGPSFile3", trip.CreateTime, len(trip.Positions), trip.Id)

	// for _, v := range trip.Positions {
	// 	if v.Timestamp == 1757549632000 {
	// 		log.Warn(v, "1757549632000", trip.CreateTime)
	// 	}

	// }

	// 如果是校验接口，则分别进行筛选，没有则push，有则跳过
	// 之后根据时间戳排序

	// 3、写入positions
	if err := writeGPSFile(trip, false); err != nil {
		log.Error("writeGPSFile", err)
		return err
	}

	// 删除对应redis
	t.DeleteRedisData(authorId, id)
	return nil
}

func (t *TripDbx) FinishTrip(authorId, id string,
	statistics *models.TripStatistics,
	permissions *models.TripPermissions, endTime int64) error {

	trip := new(models.Trip)

	updateResult, err := trip.GetCollection().UpdateOne(context.TODO(),
		bson.M{
			"$and": []bson.M{
				{
					"_id":      id,
					"authorId": authorId,
					"status":   0,
				},
			},
		}, bson.M{
			"$set": bson.M{
				"status":         1,
				"statistics":     statistics,
				"permissions":    permissions,
				"endTime":        endTime,
				"lastUpdateTime": time.Now().Unix(),
			},
		}, options.Update().SetUpsert(false))

	if err != nil {
		return err
	}
	if updateResult.ModifiedCount == 0 {
		return errors.New("update fail")
	}

	// 删除对应redis
	t.DeleteRedisData(authorId, id)
	return nil
}

func (t *TripDbx) CheckPositions(trip *models.Trip) error {
	if trip.Status == 1 {

		// existsTimestamp := []int64{}
		// newPositions := []*models.TripPosition{}

		// lat, lon, time := float64(0), float64(0), int64(0)

		// for i, v := range trip.Positions {
		// 	b := methods.GSS(v, trip.StartTime, trip.EndTime)

		// 	// 检测速度
		// 	if true || v.Speed > 30 {
		// 		distance := float64(0)
		// 		t := int64(0)
		// 		if i != 0 {
		// 			distance = methods.GetGeoDistance(lat, lon, v.Latitude, v.Longitude)
		// 			t = (v.Timestamp - time) / 1000
		// 		}
		// 		abs := int64(math.Abs((distance / float64(t)) - v.Speed))
		// 		if abs > 5 {
		// 			b = false
		// 		}
		// 	}
		// 	lat, lon, time = v.Latitude, v.Longitude, v.Timestamp

		// 	if !b ||
		// 		narrays.Includes(existsTimestamp, v.Timestamp) {
		// 		continue
		// 	}
		// 	existsTimestamp = append(existsTimestamp, v.Timestamp)
		// 	newPositions = append(newPositions, v)
		// }

		newPositions, existsTimestamp := t.FilterPositions(
			trip.Positions, trip.StartTime, trip.EndTime)

		log.Error(" newPositions  ", trip.Id, trip.StartTime,
			trip.EndTime,
			len(existsTimestamp), len(trip.Positions),
			len(newPositions))
		if err := t.UpdateTripAllPositions(trip.AuthorId, trip.Id, newPositions); err != nil {

			return err
		}
		// 每次在这之后都要重新获取一次统计
		t.GetTripStatistics(trip.Id, 0, true)
	}
	return nil
}

func (t *TripDbx) FilterPositions(positions []*models.TripPosition,
	startTime int64, endTime int64) (
	[]*models.TripPosition, []int64,
) {

	sort.Slice(positions, func(a, b int) bool {
		return positions[a].Timestamp < positions[b].Timestamp
	})
	existsTimestamp := []int64{}

	lat, lon, time := float64(0), float64(0), int64(0)

	return narrays.Filter(positions, func(v *models.TripPosition, i int) bool {
		if narrays.Includes(existsTimestamp, v.Timestamp) {
			return false
		}
		b := methods.GSS(v, startTime, endTime)
		// log.Info("b", b)

		// 检测速度
		// || i < 2 || (i >= 530 && i <= 565)
		// true ||
		// 20秒速一样才需要检测
		if v.Speed > 20 {
			distance := float64(0)
			t := int64(0)
			if i != 0 {
				distance = methods.GetGeoDistance(lat, lon, v.Latitude, v.Longitude)

				t = (v.Timestamp - time) / 1000

				// log.Info("Distance",
				// 	distance,
				// 	t,
				// 	nstrings.ToString((distance / float64(t))),
				// 	nstrings.ToString((distance/float64(t)*3.6))+"km/h",
				// )

			}
			abs := int64(math.Abs((distance / float64(t)) - v.Speed))

			if abs > 5 {
				log.Error(i, abs, (distance/float64(t))*3.6, v.Speed*3.6, distance, v.Accuracy)
				b = false
				// log.Error(v, v.Speed, abs, i)
			}
		}

		if b {
			lat, lon, time = v.Latitude, v.Longitude, v.Timestamp

			existsTimestamp = append(existsTimestamp, v.Timestamp)
		}

		return b
	}), existsTimestamp
}

func (t *TripDbx) FilterPositionsByPrivacyGeofence(
	positions []*models.TripPosition, pgp *models.PrivacyGeofencePoints,
) []*models.TripPosition {
	return positions
}

// deleteStatus -1 删除失败 0 不需要删除 1 删除成功
func (t *TripDbx) GetTripStatistics(
	id string,
	endTime int64,
	syncDataToDatabase bool,
) (*models.TripStatistics, int, error) {
	ts := new(models.TripStatistics)
	// log.Info("GetStatistics", id)

	trip, err := t.GetTripPositions(id, "")

	if endTime == 0 {
		endTime = trip.EndTime
	}

	ts.Distance = 0
	ts.MaxSpeed = 0
	ts.MaxAltitude = 0
	ts.MinAltitude = 0
	ts.ClimbAltitude = 0
	ts.DescendAltitude = 0
	ts.AverageSpeed = 0
	// log.Info("trip", trip, id, trip.StartTime, endTime)
	positions, _ := t.FilterPositions(trip.Positions, trip.StartTime, endTime)
	log.Info("positions", len(positions), len(trip.Positions))
	for i, v := range positions {
		ts.MaxSpeed = ncommon.IfElse(v.Speed > ts.MaxSpeed, v.Speed, ts.MaxSpeed)
		ts.MaxAltitude = ncommon.IfElse(v.Altitude > ts.MaxAltitude, v.Altitude, ts.MaxAltitude)
		ts.MinAltitude = ncommon.IfElse(v.Altitude < ts.MinAltitude, v.Altitude, ts.MinAltitude)

		if i == 0 {
			ts.MinAltitude = v.Altitude
			continue
		}
		lv := positions[i-1]

		// log.Info(v, lv)
		// log.Info(methods.GetGeoDistance(v.Latitude, v.Longitude, lv.Latitude, lv.Longitude))
		ts.Distance += methods.GetGeoDistance(v.Latitude, v.Longitude, lv.Latitude, lv.Longitude)

		if v.Altitude > lv.Altitude {
			ts.ClimbAltitude += v.Altitude - lv.Altitude
		}
		if v.Altitude < lv.Altitude {
			ts.DescendAltitude += lv.Altitude - v.Altitude
		}

	}

	ts.AverageSpeed = ts.Distance / (float64(endTime) - float64(trip.StartTime))

	ts.Distance = math.Round(ts.Distance*1000) / 1000
	ts.MaxSpeed = math.Round(ts.MaxSpeed*1000) / 1000
	ts.MaxAltitude = math.Round(ts.MaxAltitude*1000) / 1000
	ts.MinAltitude = math.Round(ts.MinAltitude*1000) / 1000
	ts.ClimbAltitude = math.Round(ts.ClimbAltitude*1000) / 1000
	ts.DescendAltitude = math.Round(ts.DescendAltitude*1000) / 1000
	ts.AverageSpeed = math.Round(ts.AverageSpeed*1000) / 1000

	// log.Info("distance", len(positions), ts.Distance)
	// log.Info("maxSpeed", ts.MaxSpeed)
	// log.Info("MaxAltitude", ts.MaxAltitude)
	// log.Info("MinAltitude", ts.MinAltitude)
	// log.Info("ClimbAltitude", ts.ClimbAltitude)
	// log.Info("DescendAltitude", ts.DescendAltitude)
	// log.Info("AverageSpeed", ts.AverageSpeed, ts.AverageSpeed*3.6)

	if syncDataToDatabase {
		if ts.Distance < 50 && trip.Status == 1 {
			if err = t.DeleteTrip(
				"", trip.Id,
			); err != nil {
				return ts, -1, err
			}
			return ts, 1, err
		}

		err = t.CorrectedTripData(
			"", trip.Id,
			ts,
		)
		if err != nil {
			return ts, 0, err
		}
	}
	return ts, 0, err
}

func (t *TripDbx) CheckEndTime(v *models.Trip) *models.Trip {
	if v.EndTime < v.StartTime {
		vTrip, err := t.GetTripPositions(v.Id, v.AuthorId)
		log.Info("vTrip", vTrip, err, v.Id, v.AuthorId, "")
		if vTrip == nil {
			return v
		}
		if err != nil && (vTrip.Positions == nil || len(vTrip.Positions) == 0) {
			log.Error(err)
			return v
		}
		// for _, sv := range vTrip.Positions {
		// 	log.Info("v.EndTime<v.StartTime", sv.Timestamp/1000, v.StartTime, sv.Timestamp/1000 < v.StartTime)

		// }

		log.Info("vTrip.Positions", vTrip)
		log.Info("vTrip.Positions", vTrip.Id, len(vTrip.Positions))
		if len(vTrip.Positions) <= 0 {
			return v
		}
		endTime := vTrip.Positions[len(vTrip.Positions)-1].Timestamp / 1000
		if endTime < v.StartTime {
			// log.Info("v.EndTime<v.StartTime", v.EndTime, v.StartTime)
			return v
		}
		updateResult, err := vTrip.GetCollection().UpdateOne(context.TODO(),
			bson.M{
				"$and": []bson.M{
					{
						"_id": v.Id,
					},
				},
			}, bson.M{
				"$set": bson.M{
					"endTime": endTime,
				},
			}, options.Update().SetUpsert(false))

		if err != nil {
			log.Error(err)
			return v
		}
		if updateResult.ModifiedCount == 0 {
			log.Error("updateResult.ModifiedCount", updateResult.ModifiedCount)
			return v
		}

		// 删除对应redis
		t.DeleteRedisData(v.AuthorId, v.Id)

		v.EndTime = endTime

	}
	return v
}

func (t *TripDbx) CorrectedTripData(authorId, id string,
	statistics *models.TripStatistics) error {

	trip := new(models.Trip)

	filter := bson.M{
		"_id": id,
	}
	if authorId != "" {
		filter["authorId"] = authorId
	}

	updateResult, err := trip.GetCollection().UpdateOne(context.TODO(),
		bson.M{
			"$and": []bson.M{
				filter,
			},
		}, bson.M{
			"$set": bson.M{
				"status":         1,
				"statistics":     statistics,
				"lastUpdateTime": time.Now().Unix(),
			},
		}, options.Update().SetUpsert(false))

	if err != nil {
		return err
	}
	if updateResult.ModifiedCount == 0 {
		return errors.New("update fail")
	}

	// 删除对应redis
	t.DeleteRedisData(authorId, id)
	return nil
}

func (t *TripDbx) UpdateTrip(authorId, id string, allowShare, name, typeStr, vehicleId string) error {
	trip := new(models.Trip)

	update := bson.M{
		"lastUpdateTime": time.Now().Unix(),
	}
	if allowShare != "" {
		update["permissions.allowShare"] = allowShare == "Allow"
	}
	if name != "" {
		update["name"] = name
	}
	if typeStr != "" {
		update["type"] = typeStr
	}
	if vehicleId != "" {
		update["vehicleId"] = vehicleId
		if vehicleId == "CancelVehicle" {
			update["vehicleId"] = ""
		}
	}

	// log.Info(authorId, id, update)

	updateResult, err := trip.GetCollection().UpdateOne(context.TODO(),
		bson.M{
			"$and": []bson.M{
				{
					"_id": id,
				},
				{
					"authorId": authorId,
					"status": bson.M{
						"$in": []int64{1, 0},
					},
				},
			},
		}, bson.M{
			"$set": update,
		}, options.Update().SetUpsert(false))

	if err != nil {
		return err
	}
	if updateResult.ModifiedCount == 0 {
		return errors.New("update fail")
	}

	// 删除对应redis
	t.DeleteRedisData(authorId, id)
	return nil
}

func (t *TripDbx) UpdateLastSegmentationTime(
	ctx context.Context,
	id, authorId string, lastSegmentationTime int64) error {
	trip := new(models.Trip)

	updateResult, err := trip.GetCollection().UpdateOne(ctx,
		bson.M{
			"$and": []bson.M{
				{
					"_id": id,
				},
				{
					"authorId": authorId,
				},
			},
		}, bson.M{
			"$set": bson.M{
				"lastSegmentationTime": time.Now().Unix(),
			},
		}, options.Update().SetUpsert(false))

	if err != nil {
		return err
	}
	if updateResult.ModifiedCount == 0 {
		return errors.New("update fail")
	}

	// 删除对应redis
	t.DeleteRedisData(authorId, id)
	return nil
}

func (t *TripDbx) UpdateTripAddresses(authorId, id string, addresses []*models.TripAddresses) error {
	trip := new(models.Trip)

	updateResult, err := trip.GetCollection().UpdateOne(context.TODO(),
		bson.M{
			"$and": []bson.M{
				{
					"_id": id,
				},
				{
					"authorId": authorId,
					"status": bson.M{
						"$in": []int64{1, 0},
					},
				},
			},
		}, bson.M{
			"$set": bson.M{
				"addresses":      addresses,
				"lastUpdateTime": time.Now().Unix(),
			},
		}, options.Update().SetUpsert(false))

	if err != nil {
		return err
	}
	if updateResult.ModifiedCount == 0 {
		return errors.New("update fail")
	}

	// 删除对应redis
	t.DeleteRedisData(authorId, id)
	return nil
}

func (t *TripDbx) UpdateTripNetworkStatus(authorId, id string, networkStatus []*models.TripNetworkStatus) error {
	trip := new(models.Trip)

	updateResult, err := trip.GetCollection().UpdateOne(context.TODO(),
		bson.M{
			"$and": []bson.M{
				{
					"_id": id,
				},
				{
					"authorId": authorId,
					"status": bson.M{
						"$in": []int64{1, 0},
					},
				},
			},
		}, bson.M{
			"$set": bson.M{
				"networkStatus":  networkStatus,
				"lastUpdateTime": time.Now().Unix(),
			},
		}, options.Update().SetUpsert(false))

	if err != nil {
		return err
	}
	if updateResult.ModifiedCount == 0 {
		return errors.New("update fail")
	}

	// 删除对应redis
	t.DeleteRedisData(authorId, id)
	return nil
}

func (t *TripDbx) UpdateTripWeather(id, authorId string,
	weather *models.TripWeather) error {
	trip := new(models.Trip)

	setUp := bson.M{}

	log.Info(id, authorId, weather)

	updateResult, err := trip.GetCollection().UpdateOne(context.TODO(),
		bson.M{
			"$and": []bson.M{
				{
					"_id":      id,
					"authorId": authorId,
					"status":   0,
				},
			},
		}, bson.M{
			"$push": bson.M{
				"weather": weather,
			},
			"$set": setUp,
		}, options.Update().SetUpsert(false))

	if err != nil {
		return err
	}
	if updateResult.ModifiedCount == 0 {
		return errors.New("update fail")
	}

	// 删除对应redis
	t.DeleteRedisData(authorId, id)
	return nil
}

func (t *TripDbx) ResumeTrip(id, authorId string) error {
	trip := new(models.Trip)

	updateResult, err := trip.GetCollection().UpdateOne(context.TODO(),
		bson.M{
			"$and": []bson.M{
				{
					"_id": id,
				},
				{
					"authorId": authorId,
					"status": bson.M{
						"$in": []int64{1, 0},
					},
				},
			},
		}, bson.M{
			"$set": bson.M{
				"status":         0,
				"endTime":        -1,
				"lastUpdateTime": time.Now().Unix(),
			},
		}, options.Update().SetUpsert(false))

	if err != nil {
		return err
	}
	if updateResult.ModifiedCount == 0 {
		return errors.New("update fail")
	}

	// 删除对应redis
	t.DeleteRedisData(authorId, id)
	return nil
}

func (t *TripDbx) UpdateTrips(authorId string, ids []string, vehicleId string) error {
	trip := new(models.Trip)

	update := bson.M{
		"lastUpdateTime": time.Now().Unix(),
	}
	if vehicleId != "" {
		update["vehicleId"] = vehicleId
		if vehicleId == "CancelVehicle" {
			update["vehicleId"] = ""
		}
	}

	updateResult, err := trip.GetCollection().UpdateMany(context.TODO(),
		bson.M{
			"$and": []bson.M{
				{
					"_id": bson.M{
						"$in": ids,
					},
				},
				{
					"authorId": authorId,
					"status": bson.M{
						"$in": []int64{1, 0},
					},
				},
			},
		}, bson.M{
			"$set": update,
		}, options.Update().SetUpsert(false))

	if err != nil {
		return err
	}

	log.Info(authorId, ids, vehicleId, updateResult)
	if updateResult.ModifiedCount == 0 {
		return errors.New("update fail")
	}

	// 删除对应redis
	for _, v := range ids {
		t.DeleteRedisData(authorId, v)
	}
	return nil
}

func (t *TripDbx) PermanentlyDeleteTrip(id string) error {
	trip := new(models.Trip)

	deleteResult, err := trip.GetCollection().DeleteOne(context.TODO(),
		bson.M{
			"$and": []bson.M{
				{
					"_id": id,
				},
			},
		})

	if err != nil {
		return err
	}
	if deleteResult.DeletedCount == 0 {
		return errors.New("delete fail")
	}
	// 删除对应redis
	t.DeleteRedisData("", id)
	return nil
}

func (t *TripDbx) DeleteTrip(authorId, id string) error {
	trip := new(models.Trip)

	updateResult, err := trip.GetCollection().UpdateMany(context.TODO(),
		bson.M{
			"$and": []bson.M{
				{
					"_id": id,
				},
				// {
				// 	"authorId": authorId,
				// 	// "status":   1,
				// },
			},
		}, bson.M{
			"$set": bson.M{
				"deleteTime": time.Now().Unix(),
				"status":     -1,
			},
		}, options.Update().SetUpsert(false))

	if err != nil {
		return err
	}
	if updateResult.ModifiedCount == 0 {
		return errors.New("delete fail")
	}
	// 删除对应redis
	t.DeleteRedisData(authorId, id)
	return nil
}

// 这个是重新设置positions，所以需要重新完成
// 存储到temp文件夹里
func (t *TripDbx) UpdateTripAllPositions(authorId, id string, positions []*models.TripPosition) error {
	// trip := new(models.Trip)
	trip, err := t.GetTrip(id, authorId)
	if err != nil {
		return err
	}

	log.Info("UpdateTripAllPositions", len(positions), trip.Status)
	updateResult, err := trip.GetCollection().UpdateOne(context.TODO(),
		bson.M{
			"$and": []bson.M{
				{
					"_id": id,
				},
			},
		}, bson.M{
			"$set": bson.M{
				// "positions": positions,
				// 已完成过的就不需要重新为0了
				"status":         0,
				"lastUpdateTime": time.Now().Unix(),
			},
		}, options.Update().SetUpsert(false))

	if err != nil {
		return err
	}
	if updateResult.ModifiedCount == 0 {
		return errors.New("delete fail")
	}

	trip.Positions = positions

	if err := writeGPSFile(trip, false); err != nil {
		return err
	}

	// 删除对应redis
	t.DeleteRedisData(authorId, id)
	return nil
}

var tripProject = bson.M{
	"_id":  1,
	"name": 1,
	"type": 1,
	// "positions.latitude":  1,
	// "positions.longitude": 1,
	// "positions.altitude":  1,
	// "positions.accuracy":  1,
	// "positions.speed":     1,
	// "positions.timestamp": 1,
	"vehicleId":            1,
	"marks":                1,
	"cities":               1,
	"roads":                1,
	"networkStatus":        1,
	"weather":              1,
	"statistics":           1,
	"permissions":          1,
	"addresses":            1,
	"authorId":             1,
	"status":               1,
	"createTime":           1,
	"startTime":            1,
	"endTime":              1,
	"deleteTime":           1,
	"lastSegmentationTime": 1,
}

var tripPositionsProject = bson.M{
	"_id":       1,
	"name":      1,
	"type":      1,
	"vehicleId": 1,
	// "positions.latitude":   1,
	// "positions.longitude":  1,
	// "positions.altitude":   1,
	// "positions.accuracy":   1,
	// "positions.altitudeAccuracy":   1,
	// "positions.speed":      1,
	// "positions.heading":      1,
	// "positions.timestamp":  1,
	"permissions": 1,
	"authorId":    1,
	"status":      1,
	"createTime":  1,
	"startTime":   1,
	"endTime":     1,
}

func (t *TripDbx) GetTrip(id string, authorId string) (*models.Trip, error) {
	trip := new(models.Trip)

	fsdb := trip.GetFsDB()
	// log.Info("GetTrip", id, authorId)

	val, err := fsdb.Trip.Get(id)

	if err == nil {
		tempTrip := val.Value()
		// log.Info("tempTrip", tempTrip)
		if authorId != "" && tempTrip.AuthorId == authorId {
			return tempTrip, err
		} else {
			trip = tempTrip
		}
	}

	// log.Info("trip", trip)

	if trip.Id == "" {
		params := bson.M{
			"_id": id,
		}

		if authorId != "" {
			params["authorId"] = authorId
		}

		opts := options.FindOne().SetProjection(
			tripProject,
		)
		err := trip.GetCollection().FindOne(context.TODO(), params, opts).Decode(trip)
		if err != nil {
			return nil, err
		}

		if err := fsdb.Trip.Set(id, trip, 7*24*time.Hour); err != nil {
			log.Error(err)
		}
	}

	return trip, nil
}

func (t *TripDbx) TempDownloadTripPositionsToLocal(pageNum, pageSize int64) {
	log.Info("DownloadTripPositionsToLocal")

	trip := new(models.Trip)
	var results []*models.Trip

	match := bson.M{
		"status": bson.M{
			"$in": []int64{1, 0},
		},
	}

	params := []bson.M{
		{
			"$match": bson.M{
				"$and": []bson.M{
					match,
				},
			},
		}, {
			"$sort": bson.M{
				"createTime": -1,
			},
		},
		{
			"$project": tripPositionsProject,
		},
		{
			"$skip": pageSize * (pageNum - 1),
		},
		{
			"$limit": pageSize,
		},
	}

	aOptions := options.Aggregate()
	aOptions.SetAllowDiskUse(true)

	opts, err := trip.GetCollection().Aggregate(context.TODO(), params,
		aOptions)
	if err != nil {
		log.Error(err)
		return
	}
	if err = opts.All(context.TODO(), &results); err != nil {
		log.Error(err)
		return
	}

	log.Info(len(results), pageSize, pageNum)

	for _, v := range results {
		writeGPSFile(v, false)
	}

	if len(results) == 5 {
		t.TempDownloadTripPositionsToLocal(pageNum+1, 5)
	}

	// nfile.CopyFilter(oldPath string, newPath string, f func(file fs.FileInfo, oldPath, newPath string) bool)
}

func (t *TripDbx) TempRemoveTripPositions(pageNum, pageSize int64) {
	log.Info("TempRemoveTripPositions")
	trip := new(models.Trip)
	var results []*models.Trip

	match := bson.M{
		"status": bson.M{
			"$in": []int64{1, 0, -1},
		},
	}

	params := []bson.M{
		{
			"$match": bson.M{
				"$and": []bson.M{
					match,
				},
			},
		}, {
			"$sort": bson.M{
				"createTime": -1,
			},
		},
		{
			"$project": tripPositionsProject,
		},
		{
			"$skip": pageSize * (pageNum - 1),
		},
		{
			"$limit": pageSize,
		},
	}

	aOptions := options.Aggregate()
	aOptions.SetAllowDiskUse(true)

	opts, err := trip.GetCollection().Aggregate(context.TODO(), params,
		aOptions)
	if err != nil {
		log.Error(err)
		return
	}
	if err = opts.All(context.TODO(), &results); err != nil {
		log.Error(err)
		return
	}

	log.Info(len(results), pageSize, pageNum)

	for _, v := range results {

		positions := []*models.TripPosition{}
		updateResult, err := trip.GetCollection().UpdateOne(context.TODO(),
			bson.M{
				"$and": []bson.M{
					{
						"_id": v.Id,
					},
				},
			}, bson.M{
				"$set": bson.M{
					"positions": positions,
				},
			}, options.Update().SetUpsert(false))

		log.Info("updateResult", v.Id, updateResult,
			updateResult.MatchedCount, updateResult.ModifiedCount,
			err)
	}

	if len(results) == 5 {
		t.TempRemoveTripPositions(pageNum+1, 5)
	}

	// nfile.CopyFilter(oldPath string, newPath string, f func(file fs.FileInfo, oldPath, newPath string) bool)
}

func (t *TripDbx) TempUpdateTripPositions(pageNum, pageSize int64) {
	log.Info("TempUpdateTripPositions", pageNum*pageSize)
	trip := new(models.Trip)
	var results []*models.Trip

	match := bson.M{
		// "_id": "MMxpBaSrt",
		"status": bson.M{
			"$in": []int64{1, 0},
		},
	}

	params := []bson.M{
		{
			"$match": bson.M{
				"$and": []bson.M{
					match,
				},
			},
		}, {
			"$sort": bson.M{
				"createTime": -1,
			},
		},
		// {
		// 	"$project": tripPositionsProject,
		// },
		{
			"$skip": pageSize * (pageNum - 1),
		},
		{
			"$limit": pageSize,
		},
	}

	aOptions := options.Aggregate()
	aOptions.SetAllowDiskUse(true)

	opts, err := trip.GetCollection().Aggregate(context.TODO(), params,
		aOptions)
	if err != nil {
		log.Error(err)
		return
	}
	if err = opts.All(context.TODO(), &results); err != nil {
		log.Error(err)
		return
	}

	log.Info(len(results), pageSize, pageNum)

	for _, v := range results {
		s, _, _ := t.GetTripStatistics(v.Id, 0, true)
		log.Info("MaxSpeed => ", s.MaxSpeed, s.MaxSpeed*3.6)

	}

	if len(results) == int(pageSize) {
		t.TempUpdateTripPositions(pageNum+1, pageSize)
	}

	// nfile.CopyFilter(oldPath string, newPath string, f func(file fs.FileInfo, oldPath, newPath string) bool)
}

// 搞一个temp文件。每次过滤后的放这里。根据以版本号的形式存储。
// （优先获取这里的，没有则拿old，old有问题就存temp。无限循环。
// （每次新版本号就删除之前的。
// （这样就可以避免每次都检测出问题后重复更新数据库了
// 万一发现代码逻辑问题，数据好回调

func writeGPSFile(trip *models.Trip, isOnlyTempFile bool) error {
	if trip == nil || trip.CreateTime == 0 {
		return nil
	}

	fsdb := trip.GetFsDB()

	if err := fsdb.TripPosition.Set(trip.Id, trip.Positions, 0); err != nil {
		log.Error(err)
	}
	// log.Error("writeGPSFile", len(trip.Positions))
	// log.Warn("writeGPSFile", len(trip.Positions))
	// log.Error(fsdb.TripPosition.Get(trip.Id))

	if !isOnlyTempFile {

		// 未来如果上传压力大，可以考虑用deb
		tempFolderPath := "./gps/" + time.Unix(trip.CreateTime, 0).Format("2006/01/02") + "/"

		csf := conf.SAaSS.CreateCloudServiceFile()
		if err := csf.UploadFile(trip.Positions, tempFolderPath, trip.Id, -1); err != nil {
			log.Error(err)
			return err
		}
		return nil
	}
	return nil

	// // tempFolderPath := "./static/gps/" + time.Unix(trip.CreateTime, 0).Format("2006/01/02")

	// tempFolderPath := "./static/gps/temp/" + conf.Config.Version + "/" + time.Unix(trip.CreateTime, 0).Format("2006/01/02")
	// if err := os.MkdirAll(tempFolderPath, os.ModePerm); err != nil {
	// 	return tempFolderPath, err
	// }

	// // log.Info("writeGPSFile5", trip.CreateTime, len(trip.Positions), trip.Id, tempFolderPath)
	// gpsFile, err := os.Create(tempFolderPath + "/" + trip.Id)
	// if err != nil {
	// 	log.Error(err)
	// 	return "", err
	// }
	// str, _ := json.Marshal(trip.Positions)

	// if _, err := gpsFile.Write([]byte(str)); err != nil {
	// 	return "", err
	// }

	// if !isOnlyTempFile {
	// 	tempFolderPath := "./gps/" + time.Unix(trip.CreateTime, 0).Format("2006/01/02") + "/"

	// 	csf := conf.SAaSS.CreateCloudServiceFile()
	// 	if err := csf.UploadFile(trip.Positions, tempFolderPath, trip.Id, -1); err != nil {
	// 		log.Error(err)
	// 		return "", err
	// 	}
	// 	// return tempFolderPath, nil
	// }

	// // if tempFile {

	// // } else {
	// // 	// os.Remove("./static/gps/temp/" + conf.Config.Version + "/" + time.Unix(trip.CreateTime, 0).Format("2006/01/02") + "/" + trip.Id)
	// // }

}

func readGPSFile(trip *models.Trip) (*models.Trip, error) {

	// return readGPSFileBySAaSS(trip)

	fsdb := trip.GetFsDB()

	if results, err := fsdb.TripPosition.Get(trip.Id); err == nil {
		trip.Positions = results.Value()
		if len(trip.Positions) > 0 {
			log.Info("fsdb 获取", len(trip.Positions))
			return trip, nil
		}
	}
	// log.Info(len(trip.Positions))
	// log.Info(fsdb.TripPosition.Get(trip.Id))

	tripPositions, err := readGPSFileBySAaSS(trip)
	if err != nil {
		log.Error(err)
		return nil, err
	}

	trip.Positions = tripPositions

	// return readTempGPSFile(trip, true)

	return trip, nil

}

func readGPSFileBySAaSS(trip *models.Trip) ([]*models.TripPosition, error) {
	tempTripPosition := new([]*models.TripPosition)

	// log.Warn("readGPSFileBySAaSS", tempTripPosition)
	if trip == nil || trip.CreateTime == 0 {
		return nil, nil
	}
	// trip.Positions = *tempTripPosition

	tempFolderPath := "./gps/" + time.Unix(trip.CreateTime, 0).Format("2006/01/02") + "/"

	csf := conf.SAaSS.CreateCloudServiceFile()

	file, err := csf.GetFile(tempFolderPath, trip.Id)
	if err != nil {
		log.Error(err)
		return nil, err
	}
	// log.Warn("readGPSFileBySAaSS cdFile", file)

	if err = file.JSON(tempTripPosition); err != nil {
		log.Error(file, err)
		return nil, err
	}

	// log.Warn("readGPSFileBySAaSS", len(*tempTripPosition))

	trip.Positions = *tempTripPosition

	// 下载了就缓存到本地
	writeGPSFile(trip, true)
	return *tempTripPosition, nil
}

// func readTempGPSFile(trip *models.Trip, tempFile bool) (*models.Trip, error) {
// 	tempTrip := new([]*models.TripPosition)

// 	// log.Info("v ...interface{}", trip.CreateTime)
// 	if trip == nil || trip.CreateTime == 0 {
// 		return nil, nil
// 	}
// 	// return readGPSFileBySAaSS(trip)
// 	if !tempFile {
// 		return readGPSFileBySAaSS(trip)
// 	}

// 	trip.Positions = *tempTrip

// 	tempFolderPath := "./static/gps/" + time.Unix(trip.CreateTime, 0).Format("2006/01/02")

// 	if tempFile {
// 		tempFolderPath = "./static/gps/temp/" + conf.Config.Version + "/" + time.Unix(trip.CreateTime, 0).Format("2006/01/02")
// 	}

// 	// log.Info(tempFolderPath + "/" + trip.Id)

// 	jsonFile, _ := os.Open(tempFolderPath + "/" + trip.Id)

// 	defer jsonFile.Close()
// 	decoder := json.NewDecoder(jsonFile)

// 	err := decoder.Decode(&tempTrip)
// 	// log.Error("tempTrip", tempTrip)
// 	if err != nil || tempTrip == nil || len(*tempTrip) == 0 {
// 		if tempFile {
// 			// 为false，则直接去saass拿，再没有就G
// 			return readGPSFileBySAaSS(trip)
// 			// return readTempGPSFile(trip, false)
// 		}
// 		log.Error(err, trip.Id)
// 		return nil, err
// 	}
// 	trip.Positions = *tempTrip
// 	return trip, nil
// }

func (t *TripDbx) GetTripPositions(id string, authorId string) (*models.Trip, error) {
	trip := new(models.Trip)

	// log.Info("redis1")
	// key := conf.Redisdb.GetKey("GetTripPositions")
	// err := conf.Redisdb.GetStruct(key.GetKey(id), trip)
	// // log.Info("trip.Permissions.ShareKey", trip.Permissions.ShareKey, shareKey)
	// // if shareKey != "" && trip != nil && trip.Permissions != nil && trip.Permissions.ShareKey == shareKey {
	// // 	return trip, nil
	// // }
	// // // log.Info(trip)
	// log.Info("redis2", trip)
	// // if authorId != "" && trip != nil && trip.AuthorId == authorId {
	// // 	return trip, nil
	// // }
	// if true || err != nil {

	getTrip, err := t.GetTrip(id, authorId)
	if err != nil || getTrip == nil {
		return nil, err
	}

	// params := bson.M{
	// 	"_id": id,
	// }
	// // if shareKey != "" {
	// // 	params["permissions.shareKey"] = shareKey
	// // }

	// if authorId != "" {
	// 	params["authorId"] = authorId
	// }

	// opts := options.FindOne().SetProjection(
	// 	tripPositionsProject,
	// )

	// if err := trip.GetCollection().FindOne(context.TODO(), params, opts).Decode(trip); err != nil {
	// 	return nil, err
	// }
	trip.Id = getTrip.Id
	trip.Name = getTrip.Name
	trip.Type = getTrip.Type
	trip.VehicleId = getTrip.VehicleId
	trip.Permissions = getTrip.Permissions
	trip.AuthorId = getTrip.AuthorId
	trip.Status = getTrip.Status
	trip.CreateTime = getTrip.CreateTime
	trip.StartTime = getTrip.StartTime
	trip.EndTime = getTrip.EndTime

	if _, err := readGPSFile(trip); err != nil {
		return nil, err
	}
	// log.Info("readGPSFile", len(trip.Positions), err)
	// }
	// err = conf.Redisdb.SetStruct(key.GetKey(id), trip, key.GetExpiration())
	// if err != nil {
	// 	log.Info(err)
	// }

	return trip, nil
}

func (t *TripDbx) GetTripById(id string) (*models.Trip, error) {
	trip := new(models.Trip)

	key := conf.Redisdb.GetKey("GetTrip")
	err := conf.Redisdb.GetStruct(key.GetKey(id), trip)
	if err != nil || true {
		params := bson.M{
			"_id": id,
		}
		opts := options.FindOne().SetProjection(
			tripProject,
		)
		err := trip.GetCollection().FindOne(context.TODO(), params, opts).Decode(trip)
		if err != nil {
			return nil, err
		}
	}
	err = conf.Redisdb.SetStruct(key.GetKey(id), trip, key.GetExpiration())
	if err != nil {
		log.Info(err)
	}

	return trip, nil
}

// func (t *TripDbx) GetTripByShareKey(shareKey string) (string, error) {
// 	trip := new(models.Trip)

// 	key := conf.Redisdb.GetKey("GetTripByShareKey")
// 	nv, err := conf.Redisdb.Get(key.GetKey(shareKey))
// 	if err != nil {
// 		return "", err
// 	}
// 	sk := nv.String()
// 	if sk != "" {
// 		return sk, nil

// 	}
// 	params := bson.M{
// 		"permissions.shareKey": shareKey,
// 	}

// 	opts := options.FindOne().SetProjection(
// 		bson.D{
// 			{"permissions", 1},
// 		},
// 	)
// 	err = trip.GetCollection().FindOne(
// 		context.TODO(), params, opts,
// 	).Decode(trip)

// 	if err != nil {
// 		return "", err
// 	}
// 	err = conf.Redisdb.Set(key.GetKey(shareKey), trip.Permissions.ShareKey, key.GetExpiration())
// 	if err != nil {
// 		log.Info(err)
// 	}

// 	return trip.Permissions.ShareKey, nil
// }

func (t *TripDbx) GetTripAllPositions(authorId, typeStr string,
	pageNum, pageSize int64, ids []string,
	startTime, endTime int64, vehicleLimit []string, fullField bool) ([]*models.Trip, error) {
	trip := new(models.Trip)
	var results []*models.Trip

	// key := conf.Redisdb.GetKey("GetAllTripPositions")

	// err := conf.Redisdb.GetStruct(key.GetKey(
	// 	authorId+typeStr+
	// 		nstrings.ToString(pageNum)+
	// 		nstrings.ToString(pageSize)+
	// 		strings.Join(ids, "-"),
	// ), results)
	// // log.Info("GetAllTripPositions redis", err)
	// if err != nil {
	match := bson.M{
		"authorId": authorId,
		"status": bson.M{
			"$in": []int64{1, 0},
		},
	}
	if startTime != 0 && endTime != 0 {
		match["createTime"] = bson.M{
			"$gte": startTime,
			"$lt":  endTime,
		}
	}
	if typeStr != "All" {
		match["type"] = typeStr
	}
	if len(vehicleLimit) > 0 {
		match["vehicleId"] = bson.M{
			"$in": vehicleLimit,
		}
	}
	if len(ids) > 0 {
		match["_id"] = bson.M{
			"$in": ids,
		}
		// log.Info(ids)
	}

	params := []bson.M{
		{
			"$match": bson.M{
				"$and": []bson.M{
					match,
				},
			},
		}, {
			"$sort": bson.M{
				"createTime": 1,
			},
		},
		{
			"$project": ncommon.IfElse(fullField, tripProject, tripPositionsProject),
		},
		{
			"$skip": pageSize * (pageNum - 1),
		},
		{
			"$limit": pageSize,
		},
	}

	aOptions := options.Aggregate()
	aOptions.SetAllowDiskUse(true)

	opts, err := trip.GetCollection().Aggregate(context.TODO(), params,
		aOptions)
	if err != nil {
		// log.Error(err)
		return nil, err
	}
	if err = opts.All(context.TODO(), &results); err != nil {
		// log.Error(err)
		return nil, err
	}

	for _, v := range results {
		if _, err := readGPSFile(v); err != nil {
			continue
		}
	}

	// }
	// err = conf.Redisdb.SetStruct(key.GetKey(
	// 	authorId+typeStr+
	// 		nstrings.ToString(pageNum)+
	// 		nstrings.ToString(pageSize)+
	// 		strings.Join(ids, "-"),
	// ),
	// 	results, key.GetExpiration())
	// if err != nil {
	// 	log.Info(err)
	// }

	return results, nil
}

func (t *TripDbx) FormatTripStatistics(trips []*models.Trip) *protos.TripHistoricalStatistics {
	ths := protos.TripHistoricalStatistics{}

	timeSec := int64(0)
	uselessData := []string{}
	distance := float64(0)
	days := []string{}
	maxDistance := &protos.TripHistoricalStatistics_NumItem{
		Num: 0,
		Id:  "",
	}
	maxSpeed := &protos.TripHistoricalStatistics_NumItem{
		Num: 0,
		Id:  "",
	}
	fastestAverageSpeed := &protos.TripHistoricalStatistics_NumItem{
		Num: 0,
		Id:  "",
	}
	maxAltitude := &protos.TripHistoricalStatistics_NumItem{
		Num: 0,
		Id:  "",
	}
	minAltitude := &protos.TripHistoricalStatistics_NumItem{
		Num: 9999999999,
		Id:  "",
	}
	maxClimbAltitude := &protos.TripHistoricalStatistics_NumItem{
		Num: 0,
		Id:  "",
	}
	maxDescendAltitude := &protos.TripHistoricalStatistics_NumItem{
		Num: 0,
		Id:  "",
	}
	maxTotalTripDuration := &protos.TripHistoricalStatistics_NumItem{
		Num: 0,
		Id:  "",
	}
	maxDrivingDuration := &protos.TripHistoricalStatistics_NumItem{
		Num: 0,
		Id:  "",
	}
	// trips := []*protos.Trip{}
	for _, v := range trips {

		// if v.Permissions.CustomTrip {
		// 	continue
		// }
		if maxDistance.Num <= v.Statistics.Distance {
			maxDistance.Num = v.Statistics.Distance
			maxDistance.Id = v.Id
		}
		if maxSpeed.Num <= v.Statistics.MaxSpeed {
			maxSpeed.Num = v.Statistics.MaxSpeed
			maxSpeed.Id = v.Id
		}
		if fastestAverageSpeed.Num <= v.Statistics.AverageSpeed {
			fastestAverageSpeed.Num = v.Statistics.AverageSpeed
			fastestAverageSpeed.Id = v.Id
		}
		if maxAltitude.Num <= v.Statistics.MaxAltitude {
			maxAltitude.Num = v.Statistics.MaxAltitude
			maxAltitude.Id = v.Id
		}
		if minAltitude.Num >= v.Statistics.MinAltitude {
			minAltitude.Num = v.Statistics.MinAltitude
			minAltitude.Id = v.Id
		}
		// log.Info("v.Statistics.ClimbAltitude", v.Id,
		// 	v.Status,
		// 	v.Statistics.ClimbAltitude, v.Statistics.DescendAltitude, v.Statistics.MinAltitude)
		if maxClimbAltitude.Num <= v.Statistics.ClimbAltitude && v.Statistics.ClimbAltitude != 0 {
			maxClimbAltitude.Num = v.Statistics.ClimbAltitude
			maxClimbAltitude.Id = v.Id
		}
		if maxDescendAltitude.Num <= v.Statistics.DescendAltitude && v.Statistics.DescendAltitude != 0 {
			maxDescendAltitude.Num = v.Statistics.DescendAltitude
			maxDescendAltitude.Id = v.Id
		}

		tempTime := v.EndTime - v.StartTime

		if maxTotalTripDuration.Num <= float64(tempTime) {
			maxTotalTripDuration.Num = float64(tempTime)
			maxTotalTripDuration.Id = v.Id
		}

		if v.Status == 0 && v.Statistics.Distance < 50 {
			uselessData = append(uselessData, v.Id)
		}
		if v.Status != 1 {
			continue
		}
		distance += v.Statistics.Distance
		timeSec += tempTime

		t := time.Unix(v.StartTime, 0)
		dateStr := t.Format("2006-01-02")

		if !narrays.Includes(days, dateStr) {
			days = append(days, dateStr)
		}

		// trips = append(trips, formartTrip(v))
	}

	// log.Info(days)

	ths.UselessData = uselessData
	ths.Count = int32(len(trips))
	ths.Time = timeSec
	ths.Distance = distance
	ths.Days = int32(len(days))
	ths.MaxDistance = maxDistance
	ths.MaxSpeed = maxSpeed
	ths.FastestAverageSpeed = fastestAverageSpeed
	ths.MaxAltitude = maxAltitude
	ths.MinAltitude = minAltitude
	ths.MaxClimbAltitude = maxClimbAltitude
	ths.MaxDescendAltitude = maxDescendAltitude
	ths.MaxTotalTripDuration = maxTotalTripDuration
	ths.MaxDrivingDuration = maxDrivingDuration

	return &ths
}

// old name GetTrips replace Old GetTripsBaseData
func (t *TripDbx) GetTripsBaseData(
	ids []string,
	authorId, typeStr string,
	pageNum, pageSize int64,
	createTimeLimit []int64,
	lastUpdateTimeLimit []int64,
	vehicleLimit []string,
	minDistance int64,
	maxDistance int64, cache bool) ([]*models.Trip, error) {
	trip := new(models.Trip)
	var results []*models.Trip
	k := authorId + cipher.MD5(
		fmt.Sprint(ids)+
			typeStr+
			fmt.Sprint(pageNum)+
			fmt.Sprint(pageSize)+
			fmt.Sprint(createTimeLimit)+
			fmt.Sprint(lastUpdateTimeLimit)+
			fmt.Sprint(vehicleLimit)+
			fmt.Sprint(minDistance)+
			fmt.Sprint(maxDistance),
	)
	fsdb := trip.GetFsDB()

	if cache {
		l1 := log.Time()
		val, err := fsdb.TripIds.Get(k)
		// log.Info(k)

		if err == nil {
			tempIds := val.Value()

			tempTrips := fsdb.Trip.MGet(tempIds)

			for _, v := range tempTrips {
				results = append(results, v.Val.Value())
			}

			if len(results) > 0 {
				sort.SliceStable(results, func(i, j int) bool {
					return results[i].CreateTime > results[j].CreateTime
				})
			}

			// log.Info("GetTripsBaseData tempTrips", tempIds, tempTrips)
		}
		l1.TimeEnd("fsdb.TripIds.Get(k)")

		log.Info("GetTripsBaseData", len(results), k, len(ids), err)
	}
	// log.Info("fsdb k", k, fmt.Sprint(ids)+
	// 	authorId+typeStr+
	// 	fmt.Sprint(pageNum)+
	// 	fmt.Sprint(pageSize)+
	// 	fmt.Sprint(createTimeLimit)+
	// 	fmt.Sprint(lastUpdateTimeLimit)+
	// 	fmt.Sprint(vehicleLimit)+
	// 	fmt.Sprint(minDistance)+
	// 	fmt.Sprint(maxDistance))

	// key := conf.Redisdb.GetKey("GetTrips")
	// err := conf.Redisdb.GetStruct(key.GetKey(
	// 	authorId+typeStr+
	// 		nstrings.ToString(pageNum)+
	// 		nstrings.ToString(pageSize)+
	// 		nstrings.ToString(startTime)+
	// 		nstrings.ToString(endTime),
	// ), results)
	l2 := log.Time()

	log.Info("---------即将获取数据库---------",
		len(results) == 0 || !cache)

	if len(results) == 0 || !cache {
		results = results[:0]

		match := bson.M{
			"authorId": authorId,
			"status": bson.M{
				"$in": []int64{1, 0},
			},
			"createTime": bson.M{
				"$gte": 0,
				"$lt":  time.Now().Unix(),
			},
			"lastUpdateTime": bson.M{
				"$gte": 0,
				"$lt":  time.Now().Unix(),
			},
			"statistics.distance": bson.M{
				"$gte": minDistance,
			},
		}
		if len(createTimeLimit) == 2 {
			match["createTime"] = bson.M{
				"$gte": createTimeLimit[0],
				"$lt":  createTimeLimit[1],
			}
		}
		if len(lastUpdateTimeLimit) == 2 &&
			lastUpdateTimeLimit[0] > 1540915201 &&
			lastUpdateTimeLimit[1] < 32503651200 {
			match["lastUpdateTime"] = bson.M{
				"$gte": lastUpdateTimeLimit[0],
				"$lt":  lastUpdateTimeLimit[1],
			}
		}
		if typeStr != "All" {
			match["type"] = typeStr
		}
		if len(ids) > 0 {
			match["_id"] = bson.M{
				"$in": ids,
			}
		}
		if len(vehicleLimit) > 0 {
			match["vehicleId"] = bson.M{
				"$in": vehicleLimit,
			}
		}

		if maxDistance < 500*1000 {
			match["statistics.distance"] = bson.M{
				"$gte": minDistance,
				"$lt":  maxDistance,
			}
		}
		params := []bson.M{
			{
				"$match": bson.M{
					"$and": []bson.M{
						match,
					},
				},
			}, {
				"$sort": bson.M{
					"status":     1,
					"createTime": -1,
					// "lastUpdateTime": -1,
				},
			},
		}

		if pageSize > 0 && pageSize < 10*10000 {
			params = append(params, bson.M{
				"$skip": pageSize * (pageNum - 1),
			}, bson.M{
				"$limit": pageSize,
			})
		}

		params = append(params, bson.M{
			"$project": tripProject,
		})

		// log.Info("params", params)

		cursor, err := trip.GetCollection().Aggregate(context.TODO(), params)
		if err != nil {
			return nil, err
		}
		defer cursor.Close(context.TODO())

		l2.TimeEnd("已拉取数据库，正在解析")
		l4 := log.Time()
		// count := 0
		// errCount := 0
		for cursor.Next(context.TODO()) {
			// count++
			var t models.Trip
			if err := cursor.Decode(&t); err != nil {
				// errCount++
				log.Error(err)
				continue
			}
			// log.Info(count, errCount, t.Id)
			results = append(results, &t)
		}
		l4.TimeEnd("解析数据耗时")

		// opts, err := trip.GetCollection().Aggregate(context.TODO(), params)
		// if err != nil {
		// 	// log.Error(err)
		// 	return nil, err
		// }
		// if err = opts.All(context.TODO(), &results); err != nil {
		// 	// log.Error(err)
		// 	return nil, err
		// }

		if cache {
			l3 := log.Time()
			tripIds := narrays.Map(results, func(v *models.Trip, i int) string {
				return v.Id
			})
			err = fsdb.TripIds.Set(k, tripIds, 7*23*time.Hour)
			if err != nil {
				log.Error(err)
			} else {

				for _, v := range results {
					if err := fsdb.Trip.Set(v.Id, v, 7*24*time.Hour); err != nil {
						log.Error(err)
					}
				}
			}
			l3.TimeEnd(" fsdb.TripIds.Set(k, narray")
		}
	}

	// log.Info(len(results))

	// log.Info("GetTripsBaseData Set", k, err, len(tripIds))

	// err = conf.Redisdb.SetStruct(key.GetKey(
	// 	authorId+typeStr+
	// 		nstrings.ToString(pageNum)+
	// 		nstrings.ToString(pageSize)+
	// 		nstrings.ToString(startTime)+
	// 		nstrings.ToString(endTime)), results, key.GetExpiration())
	// if err != nil {
	// 	log.Info(err)
	// }

	return results, nil
}

func (t *TripDbx) getAllTrips(
	ctx context.Context,
	ids []string,
	authorId, typeStr string,
	pageNum, pageSize int64,
	createTimeLimit []int64,
	lastUpdateTimeLimit []int64,
	vehicleLimit []string,
	minDistance int64,
	maxDistance int64,
	lastSegmentationTimeLimit []int64) ([]*models.Trip, error) {
	trip := new(models.Trip)
	var results []*models.Trip
	match := bson.M{
		"status": bson.M{
			"$in": []int64{1, 0},
		},
		"createTime": bson.M{
			"$gte": 0,
			"$lt":  time.Now().Unix(),
		},
		"lastUpdateTime": bson.M{
			"$gte": 0,
			"$lt":  time.Now().Unix(),
		},
		"statistics.distance": bson.M{
			"$gte": minDistance,
		},
	}

	if authorId != "" {
		match["authorId"] = authorId
	}

	if len(createTimeLimit) == 2 {
		match["createTime"] = bson.M{
			"$gte": createTimeLimit[0],
			"$lt":  createTimeLimit[1],
		}
	}
	if len(lastUpdateTimeLimit) == 2 {
		match["lastUpdateTime"] = bson.M{
			"$gte": lastUpdateTimeLimit[0],
			"$lt":  lastUpdateTimeLimit[1],
		}
	}

	if len(lastSegmentationTimeLimit) == 2 {
		if lastSegmentationTimeLimit[0] == -1 && lastSegmentationTimeLimit[1] == 0 {
			match["lastSegmentationTime"] = bson.M{
				"$in": bson.A{-1, nil},
			}
		} else {

			match["lastSegmentationTime"] = bson.M{
				"$gte": lastSegmentationTimeLimit[0],
				"$lt":  lastSegmentationTimeLimit[1],
			}
		}
	}

	log.Info(match["lastSegmentationTime"])

	if typeStr != "All" {
		match["type"] = typeStr
	}
	if len(ids) > 0 {
		match["_id"] = bson.M{
			"$in": ids,
		}
	}
	if len(vehicleLimit) > 0 {
		match["vehicleId"] = bson.M{
			"$in": vehicleLimit,
		}
	}

	if maxDistance < 500*1000 {
		match["statistics.distance"] = bson.M{
			"$gte": minDistance,
			"$lt":  maxDistance,
		}
	}
	params := []bson.M{
		{
			"$match": bson.M{
				"$and": []bson.M{
					match,
				},
			},
		}, {
			"$sort": bson.M{
				"status":     1,
				"createTime": -1,
				// "lastUpdateTime": -1,
			},
		},
		{
			"$skip": pageSize * (pageNum - 1),
		},
		{
			"$limit": pageSize,
		},
		{
			"$project": tripProject,
		},
	}

	opts, err := trip.GetCollection().Aggregate(ctx, params)
	if err != nil {
		// log.Error(err)
		return nil, err
	}
	if err = opts.All(ctx, &results); err != nil {
		// log.Error(err)
		return nil, err
	}

	return results, nil
}

func (t *TripDbx) GetHistoricalStatisticsData(authorId, typeStr, dataType string,
	startTime, endTime int64) (*models.Trip, error) {
	trip := new(models.Trip)
	var results []*models.Trip

	match := bson.M{
		"authorId": authorId,
		"statistics.distance": bson.M{
			// 至少300米的距离才能进入统计
			"$gte": 50,
		},
		"status": bson.M{
			"$in": []int64{1},
		},
	}

	if startTime != 0 && endTime != 0 {
		match["createTime"] = bson.M{
			"$gte": startTime,
			"$lt":  endTime,
		}
	}

	project := bson.M{
		"_id":      1,
		"authorId": 1,
	}
	sort := bson.M{}

	if typeStr != "All" {
		match["type"] = typeStr
	}

	if dataType == "maxSpeed" {
		project["statistics.maxSpeed"] = 1
		sort["statistics.maxSpeed"] = -1
	}
	if dataType == "maxDistance" {
		project["statistics.distance"] = 1
		sort["statistics.distance"] = -1
	}
	if dataType == "averageSpeed" {
		project["statistics.averageSpeed"] = 1
		sort["statistics.averageSpeed"] = -1
	}
	if dataType == "maxAltitude" {
		project["statistics.maxAltitude"] = 1
		sort["statistics.maxAltitude"] = -1
	}
	if dataType == "minAltitude" {
		project["statistics.minAltitude"] = 1
		sort["statistics.minAltitude"] = 1
	}
	if dataType == "climbAltitude" {
		project["statistics.climbAltitude"] = 1
		sort["statistics.climbAltitude"] = -1
		match["statistics.distance"] = bson.M{
			"$gte": 3000,
		}
	}
	if dataType == "descendAltitude" {
		project["statistics.descendAltitude"] = 1
		sort["statistics.descendAltitude"] = -1
		match["statistics.distance"] = bson.M{
			"$gte": 3000,
		}
	}

	params := []bson.M{
		{
			"$match": bson.M{
				"$and": []bson.M{
					match,
				},
			},
		}, {
			"$project": project,
		}, {
			"$sort": sort,
		}, {
			"$skip": 0,
		}, {
			"$limit": 1,
		},
	}

	opts, err := trip.GetCollection().Aggregate(context.TODO(), params)
	if err != nil {
		return nil, err
	}
	if err = opts.All(context.TODO(), &results); err != nil {
		return nil, err
	}

	if len(results) == 0 {
		return nil, nil
	}

	return results[0], nil
}

// func (t *TripDbx) GetTripsBaseData(
// 	authorId, typeStr string,
// 	// pageNum, pageSize int64,
// 	startTime, endTime int64,
// 	vehicleLimit []string,
// 	minDistance int64,
// 	maxDistance int64) ([]*models.Trip, error) {
// 	trip := new(models.Trip)
// 	var results []*models.Trip
// 	match := bson.M{
// 		"authorId": authorId,
// 		"status": bson.M{
// 			"$in": []int64{1, 0},
// 		},
// 		"createTime": bson.M{
// 			"$gte": startTime,
// 			"$lt":  endTime,
// 		},
// 		"statistics.distance": bson.M{
// 			"$gte": minDistance,
// 		},
// 	}
// 	if typeStr != "All" {
// 		match["type"] = typeStr
// 	}
// 	if len(vehicleLimit) > 0 {
// 		match["vehicleId"] = bson.M{
// 			"$in": vehicleLimit,
// 		}
// 	}
// 	if len(vehicleLimit) > 0 {
// 		match["vehicleId"] = bson.M{
// 			"$in": vehicleLimit,
// 		}
// 	}
// 	if maxDistance < 500*1000 {
// 		match["statistics.distance"] = bson.M{
// 			"$gte": minDistance,
// 			"$lt":  maxDistance,
// 		}
// 	}
// 	params := []bson.M{
// 		{
// 			"$match": bson.M{
// 				"$and": []bson.M{
// 					match,
// 				},
// 			},
// 		}, {
// 			"$sort": bson.M{
// 				"createTime": -1,
// 			},
// 		},
// 		{
// 			"$project": bson.M{
// 				"_id":         1,
// 				"name":        1,
// 				"type":        1,
// 				"status":      1,
// 				"vehicleId":   1,
// 				"cities":      1,
// 				"statistics":  1,
// 				"permissions": 1,
// 				"startTime":   1,
// 				"endTime":     1,
// 				"createTime":  1,
// 			},
// 		},
// 	}

// 	aOptions := options.Aggregate()
// 	aOptions.SetAllowDiskUse(true)
// 	opts, err := trip.GetCollection().Aggregate(context.TODO(), params, aOptions)
// 	if err != nil {
// 		// log.Error(err)
// 		return nil, err
// 	}
// 	if err = opts.All(context.TODO(), &results); err != nil {
// 		// log.Error(err)
// 		return nil, err
// 	}

// 	return results, nil
// }

func (t *TripDbx) DeleteRedisData(authorId, id string) error {
	log.Info("DeleteRedisData", authorId, id)

	trip := new(models.Trip)
	fsdb := trip.GetFsDB()

	if err := fsdb.Trip.Delete(id); err != nil {
		log.Error(err)
	}

	keys := fsdb.TripIds.Keys(authorId)
	// log.Error("fsdb.TripIds.Keys(authorId)", keys)
	for _, v := range keys {

		if err := fsdb.TripIds.Delete(v); err != nil {
			log.Error(err)
		}
	}

	key := conf.Redisdb.GetKey("GetTrip")
	err := conf.Redisdb.Delete(key.GetKey(id))
	if err != nil {
		return err
	}
	key = conf.Redisdb.GetKey("GetTripPositions")
	err = conf.Redisdb.Delete(key.GetKey(id))
	if err != nil {
		return err
	}
	return nil
}

func (t *TripDbx) FindCityByTime(cities []*models.TripCity, targetTime int64) ([]*models.City, error) {
	var cityId string
	var lastEntryTime int64 = -1

	for _, city := range cities {
		for _, entry := range city.EntryTimes {
			// 如果进入时间小于等于目标时间
			if entry.Timestamp <= targetTime {
				// 我们要找的是所有符合条件的进入时间里，最大的那一个
				// (即离 targetTime 最近的那次进入)
				if entry.Timestamp > lastEntryTime {
					lastEntryTime = entry.Timestamp
					cityId = city.CityId
				}
			}
		}
	}

	log.Info("cityId", cityId)
	tempCities, err := cityDbx.GetCities([]string{
		cityId,
	})
	if err != nil {
		return nil, err
	}

	return tempCities, nil
}

func (t *TripDbx) FindRoadByTime(roads []*models.TripRoad, targetTime int64) *models.TripRoad {
	var targetRoad *models.TripRoad
	var lastEntryTime int64 = -1

	for _, road := range roads {
		if road == nil {
			continue
		} // 健壮性检查

		for _, entry := range road.EntryTimes {
			if entry == nil {
				continue
			}

			// 核心逻辑：找到不晚于 targetTime 的最大(最近)一个时间点
			if entry.Timestamp <= targetTime {
				if entry.Timestamp > lastEntryTime {
					lastEntryTime = entry.Timestamp
					targetRoad = road
				}
			}
		}
	}

	return targetRoad
}

func (t *TripDbx) FindNetworkStatusByTime(list []*models.TripNetworkStatus, targetTime int64) *models.TripNetworkStatus {
	var lastStatus *models.TripNetworkStatus
	var lastEntryTime int64 = -1

	for _, item := range list {
		if item == nil {
			continue
		}

		if item.Timestamp <= targetTime {
			if item.Timestamp > lastEntryTime {
				lastEntryTime = item.Timestamp
				lastStatus = item
			}
		}
	}

	return lastStatus
}

func (t *TripDbx) FindWeatherByTime(list []*models.TripWeather, targetTime int64) *models.TripWeather {
	var lastVal *models.TripWeather
	var lastEntryTime int64 = -1

	for _, item := range list {
		if item == nil {
			continue
		}

		if item.Timestamp <= targetTime {
			if item.Timestamp > lastEntryTime {
				lastEntryTime = item.Timestamp
				lastVal = item
			}
		}
	}

	return lastVal
}

func (d *TripDbx) GetAllTripMemory() error {

	t := log.Time()
	defer t.TimeEnd("GetAllTripMemory")

	// aiDbx.IngestTripMemory("bDkFXZVgq")
	// aiDbx.IngestTripMemory("BwI3GUqrg")

	// aiDbx.IngestTripMemory("p3HXr8cwR")
	// 测试数据，记得到时候删了
	// aiDbx.IngestTripMemory("yNPLv432D")

	// userIds := []string{
	// 	"78L2tkleM",
	// 	"cXJoHSb9q",
	// 	"gM1b4llDU",
	// 	"GUUtjtg_S",
	// 	"w79TqUMqM",
	// }

	var count int64

	// maxConcurrent := 5
	// semaphore := make(chan struct{}, maxConcurrent)
	// var wg sync.WaitGroup

	// for _, userId := range userIds {
	// 	log.Warn("下一个用户", userId)

	// for i := range 1645 / 5 {

	// 	wg.Add(1)

	// 	pageNum := int64(i + 1)

	// 	go func(pageNum int64) { // 请根据你实际的 poi 类型替换 POI_TYPE
	// 		defer wg.Done()

	// 		// 获取信号量：如果通道满了，这里会阻塞，直到有其他协程退出
	// 		semaphore <- struct{}{}
	// 		defer func() { <-semaphore }() // 任务结束释放信号量

	// 		count += 5
	// 		t := log.Time()
	// 		trips, err := d.GetTripAllPositions(userId, "All",
	// 			pageNum, 5, []string{}, 0, 0, []string{}, false)

	// 		log.Info(len(trips), count)
	// 		t.TimeEnd("GetTripAllPositions page => " + nstrings.ToString(pageNum))

	// 		if err != nil {
	// 			log.Info(err)
	// 			return
	// 		}
	// 		if len(trips) == 0 {
	// 			return
	// 		}

	// 	}(pageNum)

	// }

	// break
	startPageNum := int64(1)
	// startPageNum = 323
	for i := range 100 * 10000 {
		// for i := range 2 {

		pageNum := (int64(i) + startPageNum)

		next, err := func() (bool, error) {

			ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
			defer cancel()

			count++

			ids := []string{
				// "bDkFXZVgq",
				// "BwI3GUqrg",
				// "hXO0ztKpZ",
				// "ykMRlTUd2",
			}

			trips, err := d.getAllTrips(
				ctx,
				ids,
				"", "All", pageNum, int64(1),
				[]int64{},
				[]int64{}, []string{},
				0,
				500*1000, []int64{
					-1, 0,
				})
			if err != nil {
				log.Info(err)
				return false, err
			}
			log.Info(len(trips))

			if len(trips) == 0 {
				return false, nil
			}
			trip := trips[0]
			// log.Info("pageNum", trip.Id, userId, trip.AuthorId, pageNum, count)

			// log.Info(len(trips), trip.Id, count)
			// time := time.Unix(trip.CreateTime, 0).Format("2006-01-02 15:04")

			// point, err := aiDbx.GetPointByTripIdAndAuthorId(ctx, trip.Id, trip.AuthorId)
			// if err != nil {
			// 	log.Info(err)
			// 	return false, err
			// }

			// if point != nil {
			// 	log.Error("已存在", len(trips), trip.Id, trip.AuthorId, pageNum, count, point.Id, point != nil, time)
			// 	return true, nil
			// }

			// log.Warn("开始生成切片！！！！", ctx, trip.Id, trip.AuthorId, pageNum, count, time)

			if err := tripMemoryDbx.IngestTripMemory(trip); err != nil {
				log.Error(err)
				return false, err
			}

			return true, nil
		}()

		if err != nil {
			log.Error("报错了！彻底结束", next, err, count, startPageNum+count)
			return err
		}
		if !next {
			log.Info("cancelIneer", next, count, startPageNum+count)
			break
		}

		// break
	}

	// break
	// }
	// wg.Wait()

	log.Error("正常彻底结束", startPageNum, count, startPageNum+count)

	return nil
}
