package dbxV1

import (
	"context"
	"errors"
	"time"

	conf "github.com/ShiinaAiiko/nyanya-trip-route-track/server/config"
	"github.com/ShiinaAiiko/nyanya-trip-route-track/server/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type PositionDbx struct {
}

var userPositionProject = bson.M{
	"_id":            1,
	"position":       1,
	"positionShare":  1,
	"lastUpdateTime": 1,
}

// 更新位置直接存储在sso
func (t *PositionDbx) GetUserPosition(
	authorId string) (*models.UserPosition, error) {
	// vehicle := new(models.Vehicle)

	up := new(models.UserPosition)

	k := conf.Redisdb.GetKey("GetUserPosition")

	if err := conf.Redisdb.GetStruct(k.GetKey(authorId), up); err != nil {

		params := bson.M{
			"_id": authorId,
		}

		opts := options.FindOne().SetProjection(
			userPositionProject,
		)
		err := up.GetCollection().FindOne(context.TODO(), params, opts).Decode(up)
		if err != nil {
			return nil, err
		}

	}
	if err := conf.Redisdb.SetStruct(k.GetKey(authorId), up, k.GetExpiration()); err != nil {
		log.Info(err)
		return nil, err
	}
	return up, nil
}

func (t *PositionDbx) UpdateUserPosition(
	authorId string,
	position *models.TripPosition) error {
	// vehicle := new(models.Vehicle)
	up := new(models.UserPosition)

	setUp := bson.M{
		"position":       position,
		"lastUpdateTime": time.Now().Unix(),
	}

	updateResult, err := up.GetCollection().UpdateOne(context.TODO(),
		bson.M{
			"$and": []bson.M{
				{
					"_id": authorId,
				},
			},
		}, bson.M{
			"$set": setUp,
		}, options.Update().SetUpsert(false))

	if err != nil {
		log.Error(err)
		return err
	}

	k := conf.Redisdb.GetKey("GetUserPosition")

	if updateResult.MatchedCount == 0 {
		up.Id = authorId
		up.Position = position
		if err := up.Default(); err != nil {
			return err
		}
		_, err = up.GetCollection().InsertOne(context.TODO(), up)
		if err != nil {
			return err
		}
	} else {
		if updateResult.ModifiedCount == 0 {
			log.Error(updateResult)
			return errors.New("update fail")
		}

		if err := conf.Redisdb.GetStruct(k.GetKey(authorId), up); err == nil && up != nil {
			up.Position = position
		}
	}

	if err := conf.Redisdb.SetStruct(k.GetKey(authorId), up, k.GetExpiration()); err != nil {
		log.Info(err)
		return err
	}
	return nil
}

func (t *PositionDbx) UpdateUserPositionShare(
	authorId string,
	positionShare int64) error {
	// vehicle := new(models.Vehicle)
	up := new(models.UserPosition)

	setUp := bson.M{
		"positionShare":  positionShare,
		"lastUpdateTime": time.Now().Unix(),
	}

	updateResult, err := up.GetCollection().UpdateOne(context.TODO(),
		bson.M{
			"$and": []bson.M{
				{
					"_id": authorId,
				},
			},
		}, bson.M{
			"$set": setUp,
		}, options.Update().SetUpsert(false))

	if err != nil {
		log.Error(err)
		return err
	}

	k := conf.Redisdb.GetKey("GetUserPosition")

	if updateResult.MatchedCount == 0 {
		up.Id = authorId
		up.PositionShare = positionShare
		if err := up.Default(); err != nil {
			return err
		}
		_, err = up.GetCollection().InsertOne(context.TODO(), up)
		if err != nil {
			return err
		}
	} else {
		if updateResult.ModifiedCount == 0 {
			log.Error(updateResult)
			return errors.New("update fail")
		}

		if err := conf.Redisdb.GetStruct(k.GetKey(authorId), up); err == nil && up != nil {
			up.PositionShare = positionShare
		}
	}

	if err := conf.Redisdb.SetStruct(k.GetKey(authorId), up, k.GetExpiration()); err != nil {
		log.Info(err)
		return err
	}
	return nil
}

func (t *PositionDbx) GetUserPositionList(
	authorId string, maxDistance, startTime, endTime int64, lat1, lat2, lon1, lon2 float64) ([]*models.UserPosition, error) {
	up := new(models.UserPosition)
	var results []*models.UserPosition

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
						"_id": bson.M{
							"$nin": []string{authorId},
						},
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
			"$project": userPositionProject,
		},
	}

	aOptions := options.Aggregate()
	aOptions.SetAllowDiskUse(true)
	opts, err := up.GetCollection().Aggregate(context.TODO(), params, aOptions)
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
