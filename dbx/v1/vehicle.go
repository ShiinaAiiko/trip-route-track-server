package dbxV1

import (
	"context"
	"errors"
	"time"

	conf "github.com/ShiinaAiiko/nyanya-trip-route-track/server/config"
	"github.com/ShiinaAiiko/nyanya-trip-route-track/server/models"
	"github.com/cherrai/nyanyago-utils/nshortid"
	"github.com/cherrai/nyanyago-utils/nstrings"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type VehicleDbx struct {
}

var vehicleProject = bson.M{
	"_id":            1,
	"name":           1,
	"type":           1,
	"logo":           1,
	"licensePlate":   1,
	"carModel":       1,
	"positionShare":  1,
	"status":         1,
	"tripCount":      1,
	"authorId":       1,
	"position":       1,
	"createTime":     1,
	"lastUpdateTime": 1,
}

func (t *VehicleDbx) GetShortId(digits int) string {
	str := nshortid.GetShortId(digits)

	v, err := t.GetVehicle(str, "", []int64{})
	if v == nil || err != nil {
		return str
	}
	return t.GetShortId(digits)
}

func (t *VehicleDbx) GetVehicle(id string, authorId string, status []int64) (*models.Vehicle, error) {
	vehicle := new(models.Vehicle)

	key := conf.Redisdb.GetKey("GetVehicle")
	err := conf.Redisdb.GetStruct(key.GetKey(id), vehicle)

	// log.Info("GetVehicle", id, vehicle, vehicle.Id)
	// log.Info("trip.Permissions.ShareKey", trip.Permissions.ShareKey, shareKey)

	if vehicle != nil && vehicle.Id != "" {
		return vehicle, nil
	}

	if authorId != "" && vehicle.AuthorId == authorId {
		return vehicle, nil
	}

	if err != nil {
		params := bson.M{
			"_id": id,
		}

		if authorId != "" {
			params["authorId"] = authorId
		}
		if len(status) > 0 {
			params["status"] = bson.M{
				"$in": status,
			}
		}

		opts := options.FindOne().SetProjection(
			vehicleProject,
		)
		err := vehicle.GetCollection().FindOne(context.TODO(), params, opts).Decode(vehicle)
		if err != nil {
			return nil, err
		}
	}
	err = conf.Redisdb.SetStruct(key.GetKey(id), vehicle, key.GetExpiration())
	if err != nil {
		log.Info(err)
	}

	return vehicle, nil
}

func (t *VehicleDbx) AddVehicle(vehicle *models.Vehicle) (*models.Vehicle, error) {
	// 1、插入数据
	vehicle.Id = t.GetShortId(9)

	err := vehicle.Default()
	if err != nil {
		return nil, err
	}

	_, err = vehicle.GetCollection().InsertOne(context.TODO(), vehicle)
	if err != nil {
		return nil, err
	}

	return vehicle, nil
}

func (t *VehicleDbx) GetVehicles(authorId, typeStr string, pageNum, pageSize int64) ([]*models.Vehicle, error) {
	vehicle := new(models.Vehicle)
	var results []*models.Vehicle

	key := conf.Redisdb.GetKey("GetVehicles")
	err := conf.Redisdb.GetStruct(key.GetKey(authorId+typeStr+nstrings.ToString(pageNum)+nstrings.ToString(pageSize)), results)

	if err != nil {
		match := bson.M{
			"authorId": authorId,
			"status": bson.M{
				"$in": []int64{1, 0},
			},
		}
		if typeStr != "All" {
			match["type"] = typeStr
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
				"$skip": pageSize * (pageNum - 1),
			},
			{
				"$limit": pageSize,
			},
			{
				"$project": vehicleProject,
			},
		}

		aOptions := options.Aggregate()
		aOptions.SetAllowDiskUse(true)
		opts, err := vehicle.GetCollection().Aggregate(context.TODO(), params, aOptions)
		if err != nil {
			// log.Error(err)
			return nil, err
		}
		if err = opts.All(context.TODO(), &results); err != nil {
			// log.Error(err)
			return nil, err
		}
	}
	err = conf.Redisdb.SetStruct(key.GetKey(authorId+typeStr+nstrings.ToString(pageNum)+nstrings.ToString(pageSize)), results, key.GetExpiration())
	if err != nil {
		log.Info(err)
	}

	return results, nil
}

func (t *VehicleDbx) UpdateVehicle(id, authorId, name, logo, typeStr, licensePlate, carModel string, positionShare int64) error {
	vehicle := new(models.Vehicle)

	setUp := bson.M{
		"logo":           logo,
		"type":           typeStr,
		"licensePlate":   licensePlate,
		"name":           name,
		"positionShare":  positionShare,
		"carModel":       carModel,
		"lastUpdateTime": time.Now().Unix(),
	}

	updateResult, err := vehicle.GetCollection().UpdateOne(context.TODO(),
		bson.M{
			"$and": []bson.M{
				{
					"_id":      id,
					"authorId": authorId,
					"status":   1,
				},
			},
		}, bson.M{
			"$set": setUp,
		}, options.Update().SetUpsert(false))

	t.DeleteRedisData(id)
	if err != nil {
		return err
	}
	if updateResult.ModifiedCount == 0 {
		return errors.New("update fail")
	}

	return nil
}

func (t *VehicleDbx) UpdateVehiclePosition(id, authorId string, position *models.TripPosition) error {
	vehicle := new(models.Vehicle)

	setUp := bson.M{
		"position":       position,
		"lastUpdateTime": time.Now().Unix(),
	}

	updateResult, err := vehicle.GetCollection().UpdateOne(context.TODO(),
		bson.M{
			"$and": []bson.M{
				{
					"_id":      id,
					"authorId": authorId,
					"status":   1,
				},
			},
		}, bson.M{
			"$set": setUp,
		}, options.Update().SetUpsert(false))

	t.DeleteRedisData(id)
	if err != nil {
		return err
	}
	if updateResult.ModifiedCount == 0 {
		return errors.New("update fail")
	}

	key := conf.Redisdb.GetKey("GetVehicle")
	if err = conf.Redisdb.GetStruct(key.GetKey(id), vehicle); err == nil {
		vehicle.Position = position

		if err = conf.Redisdb.SetStruct(key.GetKey(id), vehicle, key.GetExpiration()); err != nil {
			log.Info(err)
		}
	}

	return nil
}

func (t *VehicleDbx) DeleteVehicle(id, authorId string) error {
	vehicle := new(models.Vehicle)

	updateResult, err := vehicle.GetCollection().UpdateOne(context.TODO(),
		bson.M{
			"$and": []bson.M{
				{
					"_id":      id,
					"authorId": authorId,
					"status":   1,
				},
			},
		}, bson.M{
			"$set": bson.M{
				"status":    -1,
				"delteTime": time.Now().Unix(),
			},
		}, options.Update().SetUpsert(false))

	t.DeleteRedisData(id)
	if err != nil {
		return err
	}
	if updateResult.ModifiedCount == 0 {
		return errors.New("update fail")
	}

	return nil
}

func (t *VehicleDbx) DeleteRedisData(id string) error {
	key := conf.Redisdb.GetKey("GetVehicle")
	err := conf.Redisdb.Delete(key.GetKey(id))
	if err != nil {
		return err
	}
	return nil
}

func (t *VehicleDbx) GetVehiclePositionList(
	maxDistance, startTime, endTime int64, lat1, lat2, lon1, lon2 float64) ([]*models.Vehicle, error) {
	v := new(models.Vehicle)
	var results []*models.Vehicle

	// log.Info(lat1, lat2, lon1, lon2)

	minLat := lat1
	maxLat := lat2
	minLon := lon1
	maxLon := lon2
	if lat1 < lat2 {
		minLat, maxLat = lat1, lat2
	} else {
		minLat, maxLat = lat2, lat1
	}
	if lon1 < lon2 {
		minLon, maxLon = lon1, lon2
	} else {
		minLon, maxLon = lon2, lon1
	}

	params := []bson.M{
		{
			"$match": bson.M{
				"$and": []bson.M{
					{
						"positionShare": bson.M{
							"$gte": 0,
						},
						"lastUpdateTime": bson.M{
							"$gte": startTime,
							"$lt":  endTime,
						},
						"position.latitude": bson.M{
							"$gt": minLat,
							"$lt": maxLat,
						},
						"position.longitude": bson.M{
							"$gt": minLon,
							"$lt": maxLon,
						},
					},
				},
			},
		},
		{
			"$project": bson.M{
				"_id":           1,
				"position":      1,
				"type":          1,
				"authorId":      1,
				"positionShare": 1,
				"logo":          1,
				"name":          1,
				"carModel":      1,
			},
		},
	}

	aOptions := options.Aggregate()
	aOptions.SetAllowDiskUse(true)
	opts, err := v.GetCollection().Aggregate(context.TODO(), params, aOptions)
	if err != nil {
		// log.Error(err)
		return nil, err
	}
	if err = opts.All(context.TODO(), &results); err != nil {
		// log.Error(err)
		return nil, err
	}

	return results, nil
}
