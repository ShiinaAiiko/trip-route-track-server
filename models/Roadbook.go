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

type (
	RoadbookWaypointNavigationUrls struct {
		DomainUrl string `bson:"domainUrl" json:"domainUrl,omitempty"`
		ShortUrl  string `bson:"shortUrl" json:"shortUrl,omitempty"`
		Url       string `bson:"url" json:"url,omitempty"`
	}
	RoadbookWaypointNavigation struct {
		Distance   float64                         `bson:"distance" json:"distance,omitempty"`
		Duration   float64                         `bson:"duration" json:"duration,omitempty"`
		TravelMode string                          `bson:"travelMode" json:"travelMode,omitempty"`
		Urls       *RoadbookWaypointNavigationUrls `bson:"urls" json:"urls,omitempty"`
	}
	RoadbookWaypointCity struct {
		Country string `bson:"country" json:"country,omitempty"`
		State   string `bson:"state" json:"state,omitempty"`
		Region  string `bson:"region" json:"region,omitempty"`
		City    string `bson:"city" json:"city,omitempty"`
		Town    string `bson:"town" json:"town,omitempty"`
		Road    string `bson:"road" json:"road,omitempty"`
	}
	RoadbookWaypointCoords struct {
		Latitude  float64 `bson:"latitude" json:"latitude,omitempty"`
		Longitude float64 `bson:"longitude" json:"longitude,omitempty"`
	}

	RoadbookWaypointItem struct {
		Id string `bson:"id" json:"id,omitempty"`
		// Name       string                      `bson:"name" json:"name,omitempty"`
		Coords     *RoadbookWaypointCoords     `bson:"coords" json:"coords,omitempty"`
		City       *RoadbookWaypointCity       `bson:"city" json:"city,omitempty"`
		Address    string                      `bson:"address" json:"address,omitempty"`
		Icon       string                      `bson:"icon" json:"icon,omitempty"`
		Navigation *RoadbookWaypointNavigation `bson:"navigation" json:"navigation,omitempty"`

		LastNavigationTime int64 `bson:"lastNavigationTime" json:"lastNavigationTime,omitempty"`
	}

	RoadbookTimeLineItem struct {
		Id        string                  `bson:"id" json:"id,omitempty"`
		Title     string                  `bson:"title" json:"title,omitempty"`
		Desc      string                  `bson:"desc" json:"desc,omitempty"`
		Days      int32                   `bson:"days" json:"days,omitempty"`
		Waypoints []*RoadbookWaypointItem `bson:"waypoints" json:"waypoints,omitempty"`
	}
	RoadbookPermissions struct {
		// 为空则不支持分享
		// 传了则视为分享权限，可无视用户校验
		AllowShare bool `bson:"allowShare" json:"allowShare,omitempty"`
	}

	Roadbook struct {
		Id        string `bson:"_id" json:"id,omitempty"`
		Title     string `bson:"title" json:"title,omitempty"`
		Desc      string `bson:"desc" json:"desc,omitempty"`
		StartTime int64  `bson:"startTime" json:"startTime,omitempty"`

		Timelines []*RoadbookTimeLineItem `bson:"timelines" json:"timelines,omitempty"`

		Permissions *RoadbookPermissions `bson:"permissions" json:"permissions,omitempty"`

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
)

func (m *Roadbook) GetCollectionName() string {
	return "Roadbooks"
}

type RoadbookFsDB struct {
	RB         *fileStorageDB.Model[*Roadbook]
	RBList     *fileStorageDB.Model[[]*Roadbook]
	Expiration time.Duration
}

var roadbookFsDB *RoadbookFsDB

func (m *Roadbook) GetFsDB() *RoadbookFsDB {
	if roadbookFsDB != nil {
		return roadbookFsDB
	}

	rbDB, err := fileStorageDB.CreateModel[*Roadbook](conf.FsDB, m.GetCollectionName())
	if err != nil {
		log.Error(err)
		return nil
	}
	rbListDB, err := fileStorageDB.CreateModel[[]*Roadbook](conf.FsDB, m.GetCollectionName()+"List")
	if err != nil {
		log.Error(err)
		return nil
	}

	db := new(RoadbookFsDB)
	db.RB = rbDB
	db.RBList = rbListDB
	db.Expiration = 15 * 60 * time.Second

	roadbookFsDB = db
	return roadbookFsDB
}

func (m *Roadbook) CheckID(id string) bool {
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

func (m *Roadbook) GetShortId(digits int) string {
	str := nshortid.GetShortId(digits)

	if m.CheckID(str) {
		return str
	}

	return m.GetShortId(digits)
}

func (m *Roadbook) GetJMTLShortId(digits int) string {
	str := nshortid.GetShortId(digits)

	if m.CheckJMTLID(str) {
		return str
	}

	return m.GetJMTLShortId(digits)
}

func (m *Roadbook) CheckJMTLID(id string) bool {
	params := bson.M{
		"timelines.$.id": id,
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

func (m *Roadbook) Default() error {
	if m.Id == "" {
		m.Id = m.GetShortId(9)
	}

	if len(m.Timelines) == 0 {
		m.Timelines = []*RoadbookTimeLineItem{}
	}

	if m.Permissions == nil {
		m.Permissions = &RoadbookPermissions{
			AllowShare: false,
		}
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

func (m *Roadbook) GetCollection() *mongo.Collection {
	return mongodb.GetCollection(conf.Config.Mongodb.Currentdb.Name, m.GetCollectionName())
}

func (m *Roadbook) Validate() error {
	errStr := ""
	err := validation.ValidateStruct(
		m,
		validation.Parameter(&m.Id, validation.Type("string"), validation.Required()),
		validation.Parameter(&m.Title, validation.Required()),
		validation.Parameter(&m.Desc, validation.Required()),
		validation.Parameter(&m.AuthorId, validation.Required()),
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
