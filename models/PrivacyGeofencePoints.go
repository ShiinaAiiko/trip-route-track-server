package models

import (
	"context"
	"errors"
	"time"

	conf "github.com/ShiinaAiiko/nyanya-trip-route-track/server/config"
	mongodb "github.com/ShiinaAiiko/nyanya-trip-route-track/server/db/mongo"

	"github.com/cherrai/nyanyago-utils/fileStorageDB"
	"github.com/cherrai/nyanyago-utils/nshortid"
	"github.com/cherrai/nyanyago-utils/validation"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

type PrivacyGeofencePointsItemCoords struct {
	Latitude  float64 `bson:"latitude" json:"latitude,omitempty"`
	Longitude float64 `bson:"longitude" json:"longitude,omitempty"`
}

type PrivacyGeofencePointsItem struct {
	Id     string      `bson:"id" json:"id,omitempty"`
	Coords []*PrivacyGeofencePointsItemCoords `bson:"coords" json:"coords,omitempty"`

	// CreateTime Unix timestamp
	CreateTime int64 `bson:"createTime" json:"createTime,omitempty"`
	// UpdateTime Unix timestamp
	LastUpdateTime int64 `bson:"lastUpdateTime" json:"lastUpdateTime,omitempty"`
}

type PrivacyGeofencePoints struct {
	Id     string                       `bson:"_id" json:"id,omitempty"`
	Points []*PrivacyGeofencePointsItem `bson:"points" json:"points,omitempty"`

	AuthorId string `bson:"authorId" json:"authorId,omitempty"`

	// 1 normal
	// -1 delete
	Status int `bson:"status" json:"status,omitempty"`
	// CreateTime Unix timestamp
	CreateTime int64 `bson:"createTime" json:"createTime,omitempty"`
	// UpdateTime Unix timestamp
	LastUpdateTime int64 `bson:"lastUpdateTime" json:"lastUpdateTime,omitempty"`

	// DeleteTime Unix timestamp
	DeleteTime int64 `bson:"deleteTime" json:"deleteTime,omitempty"`
}

func (m *PrivacyGeofencePoints) GetCollectionName() string {
	return "PrivacyGeofencePoints"
}

type PrivacyGeofencePointsFsDB struct {
	PGPoints   *fileStorageDB.Model[*PrivacyGeofencePoints]
	Expiration time.Duration
}

var pgFsDB *PrivacyGeofencePointsFsDB

func (m *PrivacyGeofencePoints) GetFsDB() *PrivacyGeofencePointsFsDB {
	if pgFsDB != nil {
		return pgFsDB
	}

	pgPoints, err := fileStorageDB.CreateModel[*PrivacyGeofencePoints](conf.FsDB, m.GetCollectionName()+"list")
	if err != nil {
		log.Error(err)
		return nil
	}

	db := new(PrivacyGeofencePointsFsDB)
	db.PGPoints = pgPoints
	db.Expiration = 24 * time.Hour

	pgFsDB = db
	return db
}

func (m *PrivacyGeofencePoints) CheckID(id string) bool {
	params := bson.M{
		"_id": id,
	}
	count, err := m.GetCollection().CountDocuments(context.TODO(), params)
	if err != nil || count > 0 {
		if err != nil {
			log.Error(err)
		}
		return false
	}

	return true
}

func (m *PrivacyGeofencePoints) GetShortId(digits int) string {
	str := nshortid.GetShortId(digits)

	if m.CheckID(str) {
		return str
	}

	return m.GetShortId(digits)
}

func (m *PrivacyGeofencePoints) Default() error {
	if m.Id == "" {
		m.Id = m.GetShortId(9)
	}

	if m.Status == 0 {
		m.Status = 1
	}
	unixTimeStamp := time.Now().Unix()
	if m.CreateTime == 0 {
		m.CreateTime = unixTimeStamp
	}
	if m.LastUpdateTime == 0 {
		m.LastUpdateTime = unixTimeStamp
	}
	if m.DeleteTime == 0 {
		m.DeleteTime = -1
	}
	if err := m.Validate(); err != nil {
		return errors.New(m.GetCollectionName() + " Validate: " + err.Error())
	}
	return nil
}

func (m *PrivacyGeofencePoints) GetCollection() *mongo.Collection {
	return mongodb.GetCollection(conf.Config.Mongodb.Currentdb.Name, m.GetCollectionName())
}

func (m *PrivacyGeofencePoints) Validate() error {
	errStr := ""
	err := validation.ValidateStruct(
		m,
		validation.Parameter(&m.Id, validation.Type("string"), validation.Required()),
		validation.Parameter(&m.Points, validation.Required()),
		validation.Parameter(&m.Status, validation.Enum([]int{1, -1})),
		validation.Parameter(&m.CreateTime, validation.Required()),
	)
	if err != nil {
		errStr += err.Error()
	}
	if errStr == "" {
		return nil
	}
	return errors.New(errStr)
}
