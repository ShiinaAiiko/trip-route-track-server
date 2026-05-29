package dbxV1

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/ShiinaAiiko/nyanya-trip-route-track/server/models"
	"github.com/cherrai/nyanyago-utils/cipher"
	"github.com/cherrai/nyanyago-utils/nstrings"
	"github.com/mitchellh/mapstructure"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type JourneyMemoryDbx struct {
	jm *models.JourneyMemory
}

var jmProject = bson.M{
	"_id":   1,
	"name":  1,
	"desc":  1,
	"media": 1,
	// "timeline.tripIds": 1,
	// "timeline.id":      1,
	// "timeline": bson.M{
	// 	"$filter": bson.M{
	// 		"input": "$timeline",
	// 		"as":    "tl",
	// 		"cond": bson.M{
	// 			"$eq": bson.A{
	// 				"$$tl.status", 1,
	// 			},
	// 		},
	// 	},
	// },
	"timeline": bson.M{
		"$map": bson.M{
			"input": bson.M{
				"$filter": bson.M{
					"input": "$timeline",
					"as":    "tl",
					"cond": bson.M{
						"$eq": bson.A{
							"$$tl.status", 1,
						},
					},
				},
			},
			"as": "filteredTL",
			"in": bson.M{
				"id":      "$$filteredTL.id",
				"media":   "$$filteredTL.media",
				"tripIds": "$$filteredTL.tripIds",
			},
		},
	},
	"permissions":    1,
	"authorId":       1,
	"status":         1,
	"createTime":     1,
	"lastUpdateTime": 1,
	"deleteTime":     1,
}

func (d *JourneyMemoryDbx) AddJM(jm *models.JourneyMemory) (*models.JourneyMemory, error) {
	// 1、插入数据
	err := jm.Default()
	if err != nil {
		return nil, err
	}

	_, err = jm.GetCollection().InsertOne(context.TODO(), jm)
	if err != nil {
		return nil, err
	}
	_ = d.DeleteRedisData(jm.AuthorId, jm.Id)

	return jm, nil
}

func (d *JourneyMemoryDbx) UpdateJM(id, authorId, name, desc, allowShare string, media []*models.JourneyMemoryMediaItem) error {
	setUp := bson.M{
		"lastUpdateTime": time.Now().Unix(),
	}

	if name != "" {
		setUp["name"] = name
	}

	if desc != "" {
		setUp["desc"] = desc
	}

	// 设置分享的时候，media为空，所以分开
	if allowShare != "" {
		setUp["permissions.allowShare"] = allowShare == "Allow"
	} else {
		// if len(media) > 0 {
		setUp["media"] = media
		// }
	}

	updateResult, err := d.jm.GetCollection().UpdateOne(context.TODO(),
		bson.M{
			"$and": []bson.M{
				{
					"_id":      id,
					"authorId": authorId,
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

	_ = d.DeleteRedisData(authorId, id)

	return nil
}

func (d *JourneyMemoryDbx) DeleteJM(id, authorId string) error {
	updateResult, err := d.jm.GetCollection().UpdateOne(context.TODO(),
		bson.M{
			"$and": []bson.M{
				{
					"_id":      id,
					"authorId": authorId,
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

	_ = d.DeleteRedisData(authorId, id)

	return nil
}

func (d *JourneyMemoryDbx) GetJM(id, authorId string) (*models.JourneyMemory, error) {
	jm := new(models.JourneyMemory)

	// key := conf.Redisdb.GetKey("GetJM")
	// err := conf.Redisdb.GetStruct(key.GetKey(id), jm)

	// log.Error("GetCity", city)

	// log.Info("jm", jm)

	fsdb := jm.GetFsDB()

	if val, err := fsdb.Jm.Get(id); err == nil {
		tempVal := val.Value()

		if authorId == "" && tempVal.Permissions.AllowShare {
			jm = tempVal
		} else {
			jm = tempVal
		}
	}

	if jm.Id == "" {
		match := []bson.M{
			{
				"_id": id,
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
				"$project": jmProject,
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

		var results []*models.JourneyMemory

		opts, err := jm.GetCollection().Aggregate(context.TODO(), params,
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
		jm = results[0]
	}

	if err := fsdb.Jm.Set(id, jm, fsdb.Expiration); err != nil {
		log.Info(err)
	}
	// err = conf.Redisdb.SetStruct(key.GetKey(id), jm, key.GetExpiration())
	// if err != nil {
	// 	log.Info(err)
	// }

	return jm, nil
}

func (d *JourneyMemoryDbx) GetJMList(ids []string,
	authorId string,
	pageNum, pageSize int32) ([]*models.JourneyMemory, error) {
	var results []*models.JourneyMemory

	fsdb := d.jm.GetFsDB()

	k :=
		authorId + cipher.MD5(fmt.Sprint(ids)+nstrings.ToString(pageNum)+nstrings.ToString(pageSize))

	val, err := fsdb.JmList.Get(k)

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
				"$project": jmProject,
			},
		}

		opts, err := d.jm.GetCollection().Aggregate(context.TODO(), params)
		if err != nil {
			// log.Error(err)
			return nil, err
		}
		if err = opts.All(context.TODO(), &results); err != nil {
			// log.Error(err)
			return nil, err
		}

	}

	if err := fsdb.JmList.Set(k, results, 60*time.Minute); err != nil {
		log.Info(err)
	}

	return results, nil
}

func (d *JourneyMemoryDbx) DeleteRedisData(authorId string, id string) error {
	var jm *models.JourneyMemory

	fsdb := jm.GetFsDB()

	if err := fsdb.Jm.Delete(id); err != nil {
		log.Error(err)
		// return err
	}
	if err := fsdb.JmTlList.Delete(id); err != nil {
		log.Error(err)
		// return err
	}

	// log.Info(fsdb.JmTlList.Keys())
	for _, v := range fsdb.JmTlList.Keys("") {
		// log.Info(strings.Contains(v, id))
		if strings.Contains(v, id) {
			if err := fsdb.JmTlList.Delete(v); err != nil {
				log.Error(err)
				// return err
			}
		}
	}
	// log.Info(fsdb.JmTlList.Keys())
	for _, v := range fsdb.JmList.Keys(authorId) {
		if err := fsdb.JmList.Delete(v); err != nil {
			log.Error(err)
			// return err
		}
	}

	// key := conf.Redisdb.GetKey("GetJM")

	// if err := conf.Redisdb.Delete(key.GetKey(id)); err != nil {
	// 	return err
	// }

	return nil
}

func (d *JourneyMemoryDbx) AddJMTimeline(id, authorId string, jmt *models.JourneyMemoryTimeLineItem) error {
	// 1、插入数据

	if len(jmt.Media) == 0 {
		jmt.Media = []*models.JourneyMemoryMediaItem{}
	}

	if len(jmt.TripIds) == 0 {
		jmt.TripIds = []string{}
	}
	jmt.Id = d.jm.GetJMTLShortId(9)
	jmt.Status = 1
	jmt.CreateTime = time.Now().Unix()

	updateResult, err := d.jm.GetCollection().UpdateOne(context.TODO(),
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
				"timeline": jmt,
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
	_ = d.DeleteRedisData(authorId, id)
	return nil
}

func (d *JourneyMemoryDbx) UpdateJMTimeline(
	id, authorId, timelineId, name, desc string,
	media []*models.JourneyMemoryMediaItem, tripIds []string) error {

	// 1、插入数据

	setUp := bson.M{
		"lastUpdateTime": time.Now().Unix(),
	}
	if name != "" {
		setUp["timeline.$.name"] = name
	}
	if desc != "" {
		setUp["timeline.$.desc"] = desc
	}
	// log.Info(media, tripIds)
	// if len(media) != 0 {
	setUp["timeline.$.media"] = media
	// }
	// if len(tripIds) != 0 {
	setUp["timeline.$.tripIds"] = tripIds
	// }
	if len(setUp) > 1 {
		setUp["timeline.$.lastUpdateTime"] = time.Now().Unix()
	}

	updateResult, err := d.jm.GetCollection().UpdateOne(context.TODO(),
		bson.M{
			"$and": []bson.M{
				{
					"_id":         id,
					"authorId":    authorId,
					"status":      1,
					"timeline.id": timelineId,
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

	// 删除对应redis
	_ = d.DeleteRedisData(authorId, id)
	return nil
}

func (d *JourneyMemoryDbx) GetJMTimelineList(
	id, authorId string, pageNum, pageSize int32) ([]*models.JourneyMemoryTimeLineItem, error) {

	tlResults := []*models.JourneyMemoryTimeLineItem{}

	fsdb := d.jm.GetFsDB()

	k := id + authorId + nstrings.ToString(pageNum) + nstrings.ToString(pageSize)

	val, err := fsdb.JmTlList.Get(k)
	// log.Info(val, err)
	if err == nil {
		tempVal := val.Value()
		// log.Info("tempVal", tempVal)
		if len(tempVal) > 0 {
			tlResults = tempVal
		}
	}

	log.Info(len(tlResults), id, authorId, pageNum, pageSize, pageSize*(pageNum-1))
	if len(tlResults) == 0 {

		match := []bson.M{
			{
				"_id": id,
			}, {
				"status": bson.M{
					"$in": []int{1},
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
		params := []bson.M{
			{
				"$match": bson.M{
					"$and": match,
				},
			},
			{
				"$unwind": "$timeline",
			},
			// {
			// 	"$project": jmTimelineProject,
			// },
			{
				"$sort": bson.M{
					"createTime": -1,
				},
			},
			{
				"$match": bson.M{
					"$and": []bson.M{
						{
							"timeline.status": bson.M{
								"$in": []int32{1},
							},
						},
					},
				},
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

		var results []map[string]any

		opts, err := d.jm.GetCollection().Aggregate(context.TODO(), params,
			aOptions)
		if err != nil {
			log.Error(err)
			return nil, err
		}
		if err = opts.All(context.TODO(), &results); err != nil {
			log.Error(err)
			return nil, err
		}

		for _, v := range results {

			tl := new(models.JourneyMemoryTimeLineItem)

			err := mapstructure.Decode(v["timeline"], tl)
			if err != nil {
				return nil, err
			}

			tlResults = append(tlResults, tl)
		}

		if len(results) == 0 {
			return nil, err
		}

	}

	if err := fsdb.JmTlList.Set(k, tlResults, fsdb.Expiration); err != nil {
		log.Info(err)
	}

	return tlResults, nil
}

func (d *JourneyMemoryDbx) DeleteJMTimeline(
	id, authorId, timelineId string) error {

	// 1、插入数据

	setUp := bson.M{
		"timeline.$.status":         -1,
		"timeline.$.lastUpdateTime": time.Now().Unix(),
	}

	updateResult, err := d.jm.GetCollection().UpdateOne(context.TODO(),
		bson.M{
			"$and": []bson.M{
				{
					"_id":         id,
					"authorId":    authorId,
					"status":      1,
					"timeline.id": timelineId,
				},
			},
		}, bson.M{
			"$set": setUp,
		}, options.Update().SetUpsert(false))

	if err != nil {
		return err
	}
	if updateResult.ModifiedCount == 0 {
		return errors.New("delete fail")
	}

	// 删除对应redis
	_ = d.DeleteRedisData(authorId, id)
	return nil
}
