package models

import (
	"errors"
	"time"

	conf "github.com/ShiinaAiiko/nyanya-trip-route-track/server/config"
	mongodb "github.com/ShiinaAiiko/nyanya-trip-route-track/server/db/mongo"
	"github.com/cherrai/nyanyago-utils/validation"
	"go.mongodb.org/mongo-driver/mongo"
)

type Vehicle struct {
	// 使用短ID
	Id string `bson:"_id" json:"id,omitempty"`
	// 非必填
	Name string `bson:"name" json:"name,omitempty"`
	// Bike、Car、Truck、PublicTransport、Airplane、Other
	Type         string `bson:"type" json:"type,omitempty"`
	Logo         string `bson:"logo" json:"logo,omitempty"`
	LicensePlate string `bson:"licensePlate" licensePlate:"logo,omitempty"`
	AuthorId     string `bson:"authorId" json:"authorId,omitempty"`

	Position *TripPosition `bson:"position" json:"position,omitempty"`

	CarModel string `bson:"carModel" json:"carModel,omitempty"`
	// 5 所有人 1 仅本人可看 -1 禁止共享
	PositionShare int64 `bson:"positionShare" json:"positionShare,omitempty"`

	// 1 normal 0 ing -1 delete
	Status int64 `bson:"status" json:"status,omitempty"`
	// CreateTime Unix timestamp
	CreateTime int64 `bson:"createTime" json:"createTime,omitempty"`
	// LastUpdateTime Unix timestamp
	LastUpdateTime int64 `bson:"lastUpdateTime" json:"lastUpdateTime,omitempty"`
	// DelteTime Unix timestamp
	DelteTime int64 `bson:"delteTime" json:"delteTime,omitempty"`
}

func (s *Vehicle) Default() error {
	// 使用短ID
	// if s.Id == primitive.NilObjectID {
	// 	s.Id = primitive.NewObjectID()
	// }
	unixTimeStamp := time.Now().Unix()

	if s.Position == nil {
		s.Position = new(TripPosition)
	}

	if s.CreateTime == 0 {
		s.CreateTime = unixTimeStamp
	}
	if s.LastUpdateTime == 0 {
		s.LastUpdateTime = -1
	}
	if s.DelteTime == 0 {
		s.DelteTime = -1
	}

	if err := s.Validate(); err != nil {
		return errors.New(s.GetCollectionName() + " Validate: " + err.Error())
	}
	return nil
}

func (s *Vehicle) GetCollectionName() string {
	return "Vehicle"
}

func (s *Vehicle) GetCollection() *mongo.Collection {
	return mongodb.GetCollection(conf.Config.Mongodb.Currentdb.Name, s.GetCollectionName())
}

func (s *Vehicle) Validate() error {
	return validation.ValidateStruct(
		s,
		validation.Parameter(&s.Id, validation.Required(), validation.Type("string")),
		validation.Parameter(&s.Name, validation.Required(), validation.Type("string")),
		validation.Parameter(&s.Type, validation.Required(), validation.Enum([]string{
			"Bike",
			"Motorcycle",
			"Car",
			"Truck",
			"PublicTransport",
			"Airplane",
			"Other",
		})),
		validation.Parameter(&s.Logo, validation.Type("string")),
		validation.Parameter(&s.LicensePlate, validation.Type("string")),
		validation.Parameter(&s.CarModel, validation.Type("string")),
		validation.Parameter(&s.AuthorId, validation.Required()),
		validation.Parameter(&s.PositionShare, validation.Enum([]int64{
			5, 1, -1,
		}), validation.Required()),
		validation.Parameter(&s.Status, validation.Required(), validation.Enum([]int64{1, 0, -1})),
		validation.Parameter(&s.CreateTime, validation.Required()),
		validation.Parameter(&s.LastUpdateTime, validation.Required()),
		validation.Parameter(&s.DelteTime, validation.Required()),
	)
}
