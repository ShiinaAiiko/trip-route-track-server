package models

import (
	"errors"
	"time"

	conf "github.com/ShiinaAiiko/nyanya-trip-route-track/server/config"
	mongodb "github.com/ShiinaAiiko/nyanya-trip-route-track/server/db/mongo"
	"github.com/cherrai/nyanyago-utils/validation"
	"go.mongodb.org/mongo-driver/mongo"
)

type UserPosition struct {
	// authorId
	Id       string        `bson:"_id" json:"id,omitempty"`
	Position *TripPosition `bson:"position" json:"position,omitempty"`
	// 5 所有人 1 仅本人可看 -1 禁止共享
	PositionShare int64 `bson:"positionShare" json:"positionShare,omitempty"`
	// CreateTime Unix timestamp
	CreateTime int64 `bson:"createTime" json:"createTime,omitempty"`
	// LastUpdateTime Unix timestamp
	LastUpdateTime int64 `bson:"lastUpdateTime" json:"lastUpdateTime,omitempty"`
}

func (s *UserPosition) Default() error {
	unixTimeStamp := time.Now().Unix()

	if s.Position == nil {
		s.Position = new(TripPosition)
	}

	if s.PositionShare == 0 {
		s.PositionShare = -1
	}
	if s.CreateTime == 0 {
		s.CreateTime = unixTimeStamp
	}
	if s.LastUpdateTime == 0 {
		s.LastUpdateTime = -1
	}

	if err := s.Validate(); err != nil {
		return errors.New(s.GetCollectionName() + " Validate: " + err.Error())
	}
	return nil
}

func (s *UserPosition) GetCollectionName() string {
	return "UserPosition"
}

func (s *UserPosition) GetCollection() *mongo.Collection {
	return mongodb.GetCollection(conf.Config.Mongodb.Currentdb.Name, s.GetCollectionName())
}

func (s *UserPosition) Validate() error {
	return validation.ValidateStruct(
		s,
		validation.Parameter(&s.Id, validation.Required(), validation.Type("string")),
		validation.Parameter(&s.Position, validation.Required()),
		validation.Parameter(&s.PositionShare, validation.Enum([]int64{
			5, 1, -1,
		}), validation.Required()),
		validation.Parameter(&s.CreateTime, validation.Required()),
		validation.Parameter(&s.LastUpdateTime, validation.Required()),
	)
}
