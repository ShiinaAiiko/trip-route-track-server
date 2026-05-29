package dbxV1

import (
	"context"
	"errors"
	"time"

	"github.com/ShiinaAiiko/nyanya-trip-route-track/server/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type PrivacyGeofencePointsDbx struct {
	pgp *models.PrivacyGeofencePoints
}

func (t *PrivacyGeofencePointsDbx) SetPGP(authorId string, points []*models.PrivacyGeofencePointsItem) (*models.PrivacyGeofencePoints, error) {

	pgp, err := t.GetPGP(authorId)

	// log.Info("SetPGP", pgp, err)
	if err != nil {
		return nil, err
	}

	if pgp == nil {
		// 没有的时候
		pgp = &models.PrivacyGeofencePoints{
			Points:   points,
			AuthorId: authorId,
		}
		// 1、插入数据
		err := pgp.Default()
		if err != nil {
			return nil, err
		}

		_, err = pgp.GetCollection().InsertOne(context.TODO(), pgp)
		if err != nil {
			return nil, err
		}
	} else {
		// 有的时候

		setUp := bson.M{
			"points":         points,
			"lastUpdateTime": time.Now().Unix(),
		}

		// log.Info("setUp", setUp)

		updateResult, err := pgp.GetCollection().UpdateOne(context.TODO(),
			bson.M{
				"$and": []bson.M{
					{
						"authorId": authorId,
					},
				},
			}, bson.M{
				"$set": setUp,
			}, options.Update().SetUpsert(false))

		if err != nil {
			return nil, err
		}
		// log.Info("updateResult", updateResult)
		if updateResult.ModifiedCount == 0 {
			return nil, errors.New("update fail")
		}
		pgp.Points = points

		// 删除对应redis
		if err := t.DeleteRedisData(authorId); err != nil {
			return nil, err
		}

	}

	return pgp, nil
}

func (t *PrivacyGeofencePointsDbx) GetPGP(authorId string) (*models.PrivacyGeofencePoints, error) {

	var pgp *models.PrivacyGeofencePoints

	fsdb := t.pgp.GetFsDB()

	results, err := fsdb.PGPoints.Get(authorId)

	if err == nil {
		tempResult := results.Value()
		if tempResult != nil {
			pgp = tempResult
		}
	}
	// log.Error("GetPGP", results, pgp)

	if pgp == nil {
		match := []bson.M{
			{
				"authorId": authorId,
			},
			{
				"status": 1,
			},
		}

		params := []bson.M{
			{
				"$match": bson.M{
					"$and": match,
				},
			}, {
				"$sort": bson.M{
					"createTime": -1,
				},
			},
			{
				"$skip": 0,
			},
			{
				"$limit": 1,
			},
		}

		aOptions := options.Aggregate()
		aOptions.SetAllowDiskUse(true)

		var results []*models.PrivacyGeofencePoints

		opts, err := t.pgp.GetCollection().Aggregate(context.TODO(), params,
			aOptions)
		if err != nil {
			log.Error(err)
			return nil, err
		}
		if err = opts.All(context.TODO(), &results); err != nil {
			log.Error(err)
			return nil, err
		}

		if len(results) == 0 {
			return nil, err
		}
		pgp = results[0]

		if err = fsdb.PGPoints.Set(authorId, pgp, fsdb.Expiration); err != nil {
			return nil, err
		}
	}

	return pgp, nil
}

func (t *PrivacyGeofencePointsDbx) DeleteRedisData(authorId string) error {

	fsdb := t.pgp.GetFsDB()

	if err := fsdb.PGPoints.Delete(authorId); err != nil {
		log.Error(err)
	}

	for _, v := range fsdb.PGPoints.Keys(authorId) {
		if err := fsdb.PGPoints.Delete(v); err != nil {
			log.Error(err)
		}
	}
	return nil
}
