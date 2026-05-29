package models

import (
	"context"
	"errors"
	"time"

	conf "github.com/ShiinaAiiko/nyanya-trip-route-track/server/config"
	mongodb "github.com/ShiinaAiiko/nyanya-trip-route-track/server/db/mongo"
	"github.com/cherrai/nyanyago-utils/fileStorageDB"
	"github.com/cherrai/nyanyago-utils/validation"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type TripPosition struct {
	Latitude  float64 `bson:"latitude" json:"latitude,omitempty"`
	Longitude float64 `bson:"longitude" json:"longitude,omitempty"`
	// -1 则是数据不存在
	Altitude float64 `bson:"altitude" json:"altitude,omitempty"`
	// -1 则是数据不存在
	AltitudeAccuracy float64 `bson:"altitudeAccuracy" json:"altitudeAccuracy,omitempty"`
	// -1 则是数据不存在
	Accuracy float64 `bson:"accuracy" json:"accuracy,omitempty"`
	// -1 则是数据不存在
	Heading float64 `bson:"heading" json:"heading,omitempty"`
	// -1 则是数据不存在
	Speed     float64 `bson:"speed" json:"speed,omitempty"`
	Timestamp int64   `bson:"timestamp" json:"timestamp,omitempty"`
}

type TripStatistics struct {
	Distance        float64 `bson:"distance" json:"distance,omitempty"`
	MaxSpeed        float64 `bson:"maxSpeed" json:"maxSpeed,omitempty"`
	AverageSpeed    float64 `bson:"averageSpeed" json:"averageSpeed,omitempty"`
	MaxAltitude     float64 `bson:"maxAltitude" json:"maxAltitude,omitempty"`
	MinAltitude     float64 `bson:"minAltitude" json:"minAltitude,omitempty"`
	ClimbAltitude   float64 `bson:"climbAltitude" json:"climbAltitude,omitempty"`
	DescendAltitude float64 `bson:"descendAltitude" json:"descendAltitude,omitempty"`
}

type TripPermissions struct {
	// 为空则不支持分享
	// 传了则视为分享权限，可无视用户校验
	// ShareKey string `bson:"shareKey" json:"shareKey,omitempty"`
	AllowShare bool `bson:"allowShare" json:"allowShare,omitempty"`

	CustomTrip bool `bson:"customTrip" json:"customTrip,omitempty"`
	// Share bool `bson:"share" json:"share,omitempty"`
}

type TripMark struct {
	Timestamp int64 `bson:"timestamp" json:"timestamp,omitempty"`
}

type TripCityEntryTimeItem struct {
	Timestamp int64 `bson:"timestamp" json:"timestamp,omitempty"`
}
type TripCity struct {
	// Id         string                   `bson:"_id" json:"id,omitempty"`
	CityId     string                   `bson:"cityId" json:"cityId,omitempty"`
	EntryTimes []*TripCityEntryTimeItem `bson:"entryTimes" json:"entryTimes,omitempty"`
}
type TypeRoadName struct {
	En     string `bson:"en" json:"en,omitempty"`
	ZhHans string `bson:"zhHans" json:"zhHans,omitempty"`
	ZhHant string `bson:"zhHant" json:"zhHant,omitempty"`
}

type TripRoadInfo struct {
	// "motorway" | "trunk" | "primary" | "secondary" | "tertiary" | "unclassified"
	Type string        `bson:"type" json:"type,omitempty"`
	Code string        `bson:"code" json:"code,omitempty"`
	Name *TypeRoadName `bson:"name" json:"name,omitempty"`
	// Names         map[string]string `bson:"names" json:"names,omitempty"`
	ShortCityName string `bson:"shortCityName" json:"shortCityName,omitempty"`
}

type TripRoad struct {
	// Id   string       `bson:"_id" json:"id,omitempty"`
	Roads []*TripRoadInfo `bson:"roads" json:"roads,omitempty"`

	EntryTimes []*TripCityEntryTimeItem `bson:"entryTimes" json:"entryTimes,omitempty"`
}

type TripAddressesCity struct {
	Country string `bson:"country" json:"country,omitempty"`
	State   string `bson:"state" json:"state,omitempty"`
	Region  string `bson:"region" json:"region,omitempty"`
	City    string `bson:"city" json:"city,omitempty"`
	Town    string `bson:"town" json:"town,omitempty"`
	Road    string `bson:"road" json:"road,omitempty"`
}
type TripAddressesAddress struct {
	FullName string `bson:"fullName" json:"fullName,omitempty"`
	Type     string `bson:"type" json:"type,omitempty"`
	Name     string `bson:"name" json:"name,omitempty"`
}

type TripAddresses struct {
	Latitude  float64               `bson:"latitude" json:"latitude,omitempty"`
	Longitude float64               `bson:"longitude" json:"longitude,omitempty"`
	Altitude  float64               `bson:"altitude" json:"altitude,omitempty"`
	City      *TripAddressesCity    `bson:"city" json:"city,omitempty"`
	Address   *TripAddressesAddress `bson:"address" json:"address,omitempty"`

	EntryTime int64 `bson:"entryTime" json:"entryTime,omitempty"`
}

type TripNetworkStatus struct {
	//  1 online; 2 offline'
	Status    int32 `bson:"status" json:"status,omitempty"`
	Timestamp int64 `bson:"timestamp" json:"timestamp,omitempty"`
}

type TripWeather struct {
	// WeatherCode WMO 原始天气代码（如 0, 1, 61 等）
	WeatherCode int32 `json:"weatherCode" bson:"weatherCode"`
	// Temperature 实际气温 (°C)
	Temperature float64 `json:"temperature" bson:"temperature"`
	// ApparentTemperature 体感温度 (°C)
	ApparentTemperature float64 `json:"apparentTemperature" bson:"apparentTemperature"`
	// WindSpeed 风速 (km/h)
	WindSpeed float64 `json:"windSpeed" bson:"windSpeed"`
	// WindDirection 风向（°）
	WindDirection int32 `json:"wind_direction" bson:"wind_direction"`
	// Humidity 相对湿度 (%)
	Humidity float64 `json:"humidity" bson:"humidity"`
	// Precipitation 降水量 (mm)
	Precipitation float64 `json:"precipitation" bson:"precipitation"`
	// Timestamp 采样时间戳（秒）
	Timestamp int64 `json:"timestamp" bson:"timestamp"`
}

type Trip struct {
	// 使用短ID
	Id string `bson:"_id" json:"id,omitempty"`
	// 非必填
	Name string `bson:"name" json:"name,omitempty"`
	// Running、Bike、Drive、Motorcycle、Walking、PowerWalking
	Type          string               `bson:"type" json:"type,omitempty"`
	Positions     []*TripPosition      `bson:"positions" json:"positions,omitempty"`
	Addresses     []*TripAddresses     `bson:"addresses" json:"addresses,omitempty"`
	NetworkStatus []*TripNetworkStatus `bson:"networkStatus" json:"networkStatus,omitempty"`
	Marks         []*TripMark          `bson:"marks" json:"marks,omitempty"`
	Cities        []*TripCity          `bson:"cities" json:"cities,omitempty"`
	Roads         []*TripRoad          `bson:"roads" json:"roads,omitempty"`
	Weather       []*TripWeather       `bson:"weather" json:"weather,omitempty"`
	Statistics    *TripStatistics      `bson:"statistics" json:"statistics,omitempty"`
	Permissions   *TripPermissions     `bson:"permissions" json:"permissions,omitempty"`
	AuthorId      string               `bson:"authorId" json:"authorId,omitempty"`
	VehicleId     string               `bson:"vehicleId" json:"vehicleId,omitempty"`

	// 1 normal 0 ing -1 delete
	Status int64 `bson:"status" json:"status,omitempty"`
	// CreateTime Unix timestamp
	CreateTime int64 `bson:"createTime" json:"createTime,omitempty"`
	// LastUpdateTime Unix timestamp
	LastUpdateTime int64 `bson:"lastUpdateTime" json:"lastUpdateTime,omitempty"`
	// StartTime Unix timestamp
	StartTime int64 `bson:"startTime" json:"startTime,omitempty"`
	// EndTime Unix timestamp
	EndTime int64 `bson:"endTime" json:"endTime,omitempty"`
	// DeleteTime Unix timestamp
	DeleteTime int64 `bson:"deleteTime" json:"deleteTime,omitempty"`
	// LastSegmentationTime Unix timestamp
	LastSegmentationTime int64 `bson:"lastSegmentationTime" json:"lastSegmentationTime,omitempty"`
}

func (s *Trip) Default() error {
	// 使用短ID
	// if s.Id == primitive.NilObjectID {
	// 	s.Id = primitive.NewObjectID()
	// }
	unixTimeStamp := time.Now().Unix()

	if s.Positions == nil {
		s.Positions = []*TripPosition{}
	}
	if s.Marks == nil {
		s.Marks = []*TripMark{}
	}
	if s.Cities == nil {
		s.Cities = []*TripCity{}
	}
	if s.Addresses == nil {
		s.Addresses = []*TripAddresses{}
	}
	if s.NetworkStatus == nil {
		s.NetworkStatus = []*TripNetworkStatus{}
	}
	if s.Roads == nil {
		s.Roads = []*TripRoad{}
	}
	if s.Weather == nil {
		s.Weather = []*TripWeather{}
	}
	if s.Statistics == nil {
		s.Statistics = &TripStatistics{}
	}
	if s.Permissions == nil {
		s.Permissions = &TripPermissions{}
	}
	if s.CreateTime == 0 {
		s.CreateTime = unixTimeStamp
	}
	if s.LastUpdateTime == 0 {
		s.LastUpdateTime = unixTimeStamp
	}
	if s.EndTime == 0 {
		s.EndTime = -1
	}
	if s.DeleteTime == 0 {
		s.DeleteTime = -1
	}
	if s.LastSegmentationTime == 0 {
		s.LastSegmentationTime = -1
	}

	if err := s.Validate(); err != nil {
		return errors.New(s.GetCollectionName() + " Validate: " + err.Error())
	}
	return nil
}

func (s *Trip) GetCollectionName() string {
	return "Trip"
}

func (s *Trip) CreateIndex() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// 定义你最核心的复合索引：ESR原则 (Equal, Sort, Range)
	// authorId (等值) -> status (等值) -> createTime (排序)
	indexModels := []mongo.IndexModel{
		{
			Keys: bson.D{
				{Key: "authorId", Value: 1},
				{Key: "status", Value: 1},
				{Key: "createTime", Value: -1},
			},
			Options: options.Index().SetName("idx_author_status_time"),
		},
		{
			Keys: bson.D{
				{Key: "authorId", Value: 1},
				{Key: "type", Value: 1},
				{Key: "createTime", Value: -1},
			},
			Options: options.Index().SetName("idx_author_type_time"),
		},
		// 如果你经常按车辆过滤，建议也给 vehicleId 加一个索引
		{
			Keys: bson.D{
				{Key: "authorId", Value: 1},
				{Key: "vehicleId", Value: 1},
				{Key: "createTime", Value: -1},
			},
			Options: options.Index().SetName("idx_author_vehicle_time"),
		},
		// 针对 lastUpdateTime 的范围查询
		{
			Keys:    bson.D{{Key: "lastUpdateTime", Value: -1}},
			Options: options.Index().SetName("idx_last_update"),
		},
	}

	// 执行创建
	names, err := s.GetCollection().Indexes().CreateMany(ctx, indexModels)
	if err != nil {
		log.Error("自动化索引创建失败 (可能已存在): %v", err)
		return
	}

	log.Info("索引自动化检查完成: %v", names)
}

type TripFsDB struct {
	Trip         *fileStorageDB.Model[*Trip]
	TripPosition *fileStorageDB.Model[[]*TripPosition]
	TripIds      *fileStorageDB.Model[[]string]
	Expiration   time.Duration
}

var tripFsDB *TripFsDB

func (s *Trip) GetFsDB() *TripFsDB {
	if tripFsDB != nil {
		return tripFsDB
	}

	tripDB, err := fileStorageDB.CreateModel[*Trip](conf.FsDB, s.GetCollectionName())
	if err != nil {
		log.Error(err)
		return nil
	}
	tripPositionDB, err := fileStorageDB.CreateModel[[]*TripPosition](
		conf.FsDB, s.GetCollectionName()+"TripPosition")
	if err != nil {
		log.Error(err)
		return nil
	}
	tripIdsDB, err := fileStorageDB.CreateModel[[]string](conf.FsDB, s.GetCollectionName()+"list")
	if err != nil {
		log.Error(err)
		return nil
	}
	db := new(TripFsDB)
	db.Trip = tripDB
	db.TripPosition = tripPositionDB
	db.TripIds = tripIdsDB
	db.Expiration = 15 * time.Minute

	tripFsDB = db
	return db
}

func (s *Trip) GetCollection() *mongo.Collection {
	return mongodb.GetCollection(conf.Config.Mongodb.Currentdb.Name, s.GetCollectionName())
}

func (s *Trip) Validate() error {
	return validation.ValidateStruct(
		s,
		validation.Parameter(&s.Id, validation.Required(), validation.Type("string")),
		validation.Parameter(&s.Type, validation.Required(), validation.Enum([]string{
			"Running",
			"Bike",
			"Drive",
			"Motorcycle",
			"Walking",
			"PowerWalking",
			"Train",
			"PublicTransport",
			"Plane"})),
		validation.Parameter(&s.AuthorId, validation.Required()),
		validation.Parameter(&s.Status, validation.Enum([]int64{1, 0, -1})),
		validation.Parameter(&s.CreateTime, validation.Required()),
		validation.Parameter(&s.LastUpdateTime, validation.Required()),
		validation.Parameter(&s.StartTime, validation.Required()),
		validation.Parameter(&s.DeleteTime, validation.Required()),
	)
}
