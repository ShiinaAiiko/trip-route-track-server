package models

import (
	"context"
	"errors"
	"time"

	conf "github.com/ShiinaAiiko/nyanya-trip-route-track/server/config"
	mongodb "github.com/ShiinaAiiko/nyanya-trip-route-track/server/db/mongo"
	"github.com/ShiinaAiiko/nyanya-trip-route-track/server/protos"

	"github.com/cherrai/nyanyago-utils/fileStorageDB"
	"github.com/cherrai/nyanyago-utils/nshortid"
	"github.com/cherrai/nyanyago-utils/validation"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

type CityAddresses struct {
	Country string `bson:"country" json:"country,omitempty"`
	State   string `bson:"state" json:"state,omitempty"`
	Region  string `bson:"region" json:"region,omitempty"`
	City    string `bson:"city" json:"city,omitempty"`
	Town    string `bson:"town" json:"town,omitempty"`
	Address string `bson:"address" json:"address,omitempty"`
}

type CityCoords struct {
	Latitude  float64 `bson:"latitude" json:"latitude,omitempty"`
	Longitude float64 `bson:"longitude" json:"longitude,omitempty"`
}

var CityNameLanguages = []string{"zhCN", "en", "zhHans", "zhHant"}

type CityName struct {
	Ref       string `bson:"ref" json:"ref,omitempty"`
	ShortName string `bson:"shortName" json:"shortName,omitempty"`
	ZhCN      string `bson:"zhCN" json:"zhCN,omitempty"`
	En        string `bson:"en" json:"en,omitempty"`
	ZhHans    string `bson:"zhHans" json:"zhHans,omitempty"`
	ZhHant    string `bson:"zhHant" json:"zhHant,omitempty"`
}
type CityNames struct {
	Names      map[string]string `bson:"names" json:"names,omitempty"`
	CreateTime int64             `bson:"createTime" json:"createTime,omitempty"`
}

type City struct {
	Id string `bson:"_id" json:"id,omitempty"`
	// 城市名 多国语言
	Name  *CityName  `bson:"name" json:"name,omitempty"`
	Names *CityNames `bson:"names" json:"names,omitempty"`
	// 父级文件夹路径Id
	ParentCityId string `bson:"parentCityId" json:"parentCityId,omitempty"`

	Coords *CityCoords `bson:"coords" json:"coords,omitempty"`

	// 1 国家 2 省 3 市 4 区县 5 镇
	Level int `bson:"level" json:"level,omitempty"`

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

func (m *City) GetCollectionName() string {
	return "Cities"
}

type CityFsDB struct {
	City           *fileStorageDB.Model[*City]
	Cities         *fileStorageDB.Model[[]*City]
	CitiesProto    *fileStorageDB.Model[[]*protos.CityItem]
	CityNamesCache *fileStorageDB.Model[bool]
	Expiration     time.Duration
}

var cityFsDB *CityFsDB

func (m *City) GetFsDB() *CityFsDB {
	if cityFsDB != nil {
		return cityFsDB
	}

	cityDB, err := fileStorageDB.CreateModel[*City](conf.FsDB, m.GetCollectionName())
	if err != nil {
		log.Error(err)
		return nil
	}
	citiesDB, err := fileStorageDB.CreateModel[[]*City](conf.FsDB, m.GetCollectionName()+"list")
	if err != nil {
		log.Error(err)
		return nil
	}
	citiesProtoDB, err := fileStorageDB.CreateModel[[]*protos.CityItem](conf.FsDB, m.GetCollectionName()+"listproto")
	if err != nil {
		log.Error(err)
		return nil
	}
	cityNamesCacheDB, err := fileStorageDB.CreateModel[bool](conf.FsDB, m.GetCollectionName()+"listproto")
	if err != nil {
		log.Error(err)
		return nil
	}

	db := new(CityFsDB)
	db.City = cityDB
	db.Cities = citiesDB
	db.CitiesProto = citiesProtoDB
	db.CityNamesCache = cityNamesCacheDB
	db.Expiration = 24 * time.Hour

	cityFsDB = db
	return db
}

func (m *City) CheckID(id string) bool {
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

func (m *City) GetShortId(digits int) string {
	str := nshortid.GetShortId(digits)

	if m.CheckID(str) {
		return str
	}

	return m.GetShortId(digits)
}

func (m *City) Default() error {
	if m.Id == "" {
		m.Id = m.GetShortId(9)
	}

	if m.ParentCityId == "" || m.Level == 0 {
		m.Level = 1
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

func (m *City) GetCollection() *mongo.Collection {
	return mongodb.GetCollection(conf.Config.Mongodb.Currentdb.Name, m.GetCollectionName())
}

func (m *City) Validate() error {
	errStr := ""
	err := validation.ValidateStruct(
		m,
		validation.Parameter(&m.Id, validation.Type("string"), validation.Required()),
		validation.Parameter(&m.Name, validation.Required()),
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
