package dbxV1

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/ShiinaAiiko/nyanya-trip-route-track/server/models"
	"github.com/cherrai/nyanyago-utils/cipher"
	"github.com/cherrai/nyanyago-utils/nstrings"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type RoadbookDbx struct {
	roadbook *models.Roadbook
}

var roadbookProject = bson.M{
	"_id":            1,
	"title":          1,
	"desc":           1,
	"startTime":      1,
	"timelines":      1,
	"permissions":    1,
	"authorId":       1,
	"status":         1,
	"createTime":     1,
	"lastUpdateTime": 1,
	"deleteTime":     1,
}

func (d *RoadbookDbx) AddRoadbook(rb *models.Roadbook) (*models.Roadbook, error) {
	// 1、插入数据
	err := rb.Default()
	if err != nil {
		return nil, err
	}

	_, err = rb.GetCollection().InsertOne(context.TODO(), rb)
	if err != nil {
		return nil, err
	}
	d.DeleteRedisData(rb.AuthorId, rb.Id)

	return rb, nil
}

func (d *RoadbookDbx) GetRoadbookList(
	authorId string, ids []string,
	pageNum, pageSize int32) ([]*models.Roadbook, error) {
	var results []*models.Roadbook

	fsdb := d.roadbook.GetFsDB()

	k :=
		authorId + cipher.MD5(fmt.Sprint(ids)+nstrings.ToString(pageNum)+nstrings.ToString(pageSize))

	val, err := fsdb.RBList.Get(k)

	if err == nil {
		tempVal := val.Value()
		// log.Info("tempVal", tempVal)
		if len(tempVal) > 0 {
			results = tempVal
		}
	}
	if len(results) == 0 {
		match := bson.M{
			"authorId": authorId,
			"status": bson.M{
				"$in": []int64{1, 0},
			},
		}

		if len(ids) > 0 {
			match["_id"] = bson.M{
				"$in": ids,
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
					"lastUpdateTime": -1,
				},
			},
			{
				"$skip": pageSize * (pageNum - 1),
			},
			{
				"$limit": pageSize,
			},
			{
				"$project": roadbookProject,
			},
		}

		opts, err := d.roadbook.GetCollection().Aggregate(context.TODO(), params)
		if err != nil {
			// log.Error(err)
			return nil, err
		}
		if err = opts.All(context.TODO(), &results); err != nil {
			// log.Error(err)
			return nil, err
		}

	}

	if err := fsdb.RBList.Set(k, results, 60*time.Minute); err != nil {
		log.Info(err)
	}

	return results, nil
}

func (d *RoadbookDbx) GetRoadbook(id, authorId string) (*models.Roadbook, error) {
	rb := new(models.Roadbook)

	// key := conf.Redisdb.GetKey("GetJM")
	// err := conf.Redisdb.GetStruct(key.GetKey(id), jm)

	// log.Error("GetCity", city)

	// log.Info("jm" , jm)

	fsdb := rb.GetFsDB()

	if val, err := fsdb.RB.Get(id); err == nil {
		tempVal := val.Value()

		if authorId == "" && tempVal.Permissions.AllowShare {
			rb = tempVal
		} else {
			rb = tempVal
		}
	}

	if rb.Id == "" {
		match := []bson.M{
			{
				"_id": id,
				"status": bson.M{
					"$in": []int64{1, 0},
				},
			},
		}
		if authorId != "" {
			match = append(match, bson.M{
				"authorId": authorId,
			})
		} else {
			match = append(match, bson.M{
				"permissions.allowShare": true,
			})
		}

		// log.Info("name != ", name != "")

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
				"$project": roadbookProject,
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

		var results []*models.Roadbook

		opts, err := rb.GetCollection().Aggregate(context.TODO(), params,
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
		rb = results[0]
	}

	if err := fsdb.RB.Set(id, rb, fsdb.Expiration); err != nil {
		log.Info(err)
	}
	// err = conf.Redisdb.SetStruct(key.GetKey(id), jm, key.GetExpiration())
	// if err != nil {
	// 	log.Info(err)
	// }

	return rb, nil
}

func (d *RoadbookDbx) AddRoadbookTimeline(id, authorId string, tli *models.RoadbookTimeLineItem) error {
	// 1、插入数据

	tli.Id = d.roadbook.GetJMTLShortId(9)

	updateResult, err := d.roadbook.GetCollection().UpdateOne(context.TODO(),
		bson.M{
			"$and": []bson.M{
				{
					"_id":      id,
					"authorId": authorId,
					"status":   1,
				},
			},
		}, bson.M{
			"$push": bson.M{
				"timelines": tli,
			},
			"$set": bson.M{
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
	d.DeleteRedisData(authorId, id)
	return nil

}

func (d *RoadbookDbx) UpdateRoadbook(id, authorId, title, desc string,
	startTime int64,
	timelines []*models.RoadbookTimeLineItem,
	permissions *models.RoadbookPermissions) error {
	// 1、插入数据

	log.Info(id, authorId, title, desc, startTime, timelines, permissions)

	updateResult, err := d.roadbook.GetCollection().UpdateOne(context.TODO(),
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
				"title":          title,
				"desc":           desc,
				"startTime":      startTime,
				"timelines":      timelines,
				"permissions":    permissions,
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
	d.DeleteRedisData(authorId, id)
	return nil
}

func (d *RoadbookDbx) DeleteRoadbook(id, authorId string,
) error {
	// 1、插入数据

	updateResult, err := d.roadbook.GetCollection().UpdateOne(context.TODO(),
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
				"status":     -1,
				"deleteTime": time.Now().Unix(),
			},
		}, options.Update().SetUpsert(false))

	if err != nil {
		return err
	}
	if updateResult.ModifiedCount == 0 {
		return errors.New("delete fail")
	}

	// 删除对应redis
	d.DeleteRedisData(authorId, id)
	return nil
}

func (d *RoadbookDbx) DeleteRedisData(authorId string, id string) error {
	var roadbook *models.Roadbook

	fsdb := roadbook.GetFsDB()

	if err := fsdb.RB.Delete(id); err != nil {
		log.Error(err)
	}
	if err := fsdb.RBList.Delete(id); err != nil {
		log.Error(err)
	}

	for _, v := range fsdb.RBList.Keys(authorId) {
		if err := fsdb.RBList.Delete(v); err != nil {
			log.Error(err)
			// return err
		}
	}

	return nil
}
