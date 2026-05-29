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
	JourneyMemoryMediaItem struct {
		// image / video / onlineVideo etc.
		Type   string `bson:"type" json:"type,omitempty"`
		Width  int    `bson:"width" json:"width,omitempty"`
		Height int    `bson:"height" json:"height,omitempty"`
		Url    string `bson:"url" json:"url,omitempty"`
	}
	JourneyMemoryTimeLineItem struct {
		Id      string                    `bson:"id" json:"id,omitempty"`
		Name    string                    `bson:"name" json:"name,omitempty"`
		Desc    string                    `bson:"desc" json:"desc,omitempty"`
		Media   []*JourneyMemoryMediaItem `bson:"media" json:"media,omitempty"`
		TripIds []string                  `bson:"tripIds" json:"tripIds,omitempty"`
		// 1 normal
		// -1 delete
		Status         int   `bson:"status" json:"status,omitempty"`
		CreateTime     int64 `bson:"createTime" json:"createTime,omitempty"`
		LastUpdateTime int64 `bson:"lastUpdateTime" json:"lastUpdateTime,omitempty"`
	}
	JourneyMemoryPermissions struct {
		// 为空则不支持分享
		// 传了则视为分享权限，可无视用户校验
		AllowShare bool `bson:"allowShare" json:"allowShare,omitempty"`
	}

	JourneyMemory struct {
		Id string `bson:"_id" json:"id,omitempty"`
		// 城市名 多国语言
		Name string `bson:"name" json:"name,omitempty"`
		// 城市名 多国语言
		Desc  string                    `bson:"desc" json:"desc,omitempty"`
		Media []*JourneyMemoryMediaItem `bson:"media" json:"media,omitempty"`

		Timeline []*JourneyMemoryTimeLineItem `bson:"timeline" json:"timeline,omitempty"`

		Permissions *JourneyMemoryPermissions `bson:"permissions" json:"permissions,omitempty"`

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

func (m *JourneyMemory) GetCollectionName() string {
	return "JourneyMemories"
}

type JMFsDB struct {
	Jm         *fileStorageDB.Model[*JourneyMemory]
	JmList     *fileStorageDB.Model[[]*JourneyMemory]
	JmTlList   *fileStorageDB.Model[[]*JourneyMemoryTimeLineItem]
	Expiration time.Duration
}

var jmFsDB *JMFsDB

func (m *JourneyMemory) GetFsDB() *JMFsDB {
	if jmFsDB != nil {
		return jmFsDB
	}

	jmDB, err := fileStorageDB.CreateModel[*JourneyMemory](conf.FsDB, m.GetCollectionName())
	if err != nil {
		log.Error(err)
		return nil
	}
	jmListDB, err := fileStorageDB.CreateModel[[]*JourneyMemory](conf.FsDB, m.GetCollectionName()+"List")
	if err != nil {
		log.Error(err)
		return nil
	}

	jmTlListDB, err := fileStorageDB.CreateModel[[]*JourneyMemoryTimeLineItem](conf.FsDB, m.GetCollectionName()+"TlList")
	if err != nil {
		log.Error(err)
		return nil
	}

	db := new(JMFsDB)
	db.Jm = jmDB
	db.JmList = jmListDB
	db.JmTlList = jmTlListDB
	db.Expiration = 15 * 60 * time.Second

	jmFsDB = db
	return jmFsDB
}

func (m *JourneyMemory) CheckID(id string) bool {
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

func (m *JourneyMemory) GetShortId(digits int) string {
	str := nshortid.GetShortId(digits)

	if m.CheckID(str) {
		return str
	}

	return m.GetShortId(digits)
}

func (m *JourneyMemory) GetJMTLShortId(digits int) string {
	str := nshortid.GetShortId(digits)

	if m.CheckJMTLID(str) {
		return str
	}

	return m.GetJMTLShortId(digits)
}

func (m *JourneyMemory) CheckJMTLID(id string) bool {
	params := bson.M{
		"timeline.$.id": id,
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

func (m *JourneyMemory) Default() error {
	if m.Id == "" {
		m.Id = m.GetShortId(9)
	}

	if len(m.Media) == 0 {
		m.Media = []*JourneyMemoryMediaItem{}
	}
	if len(m.Timeline) == 0 {
		m.Timeline = []*JourneyMemoryTimeLineItem{}
	}

	if m.Permissions == nil {
		m.Permissions = &JourneyMemoryPermissions{
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

func (m *JourneyMemory) GetCollection() *mongo.Collection {
	return mongodb.GetCollection(conf.Config.Mongodb.Currentdb.Name, m.GetCollectionName())
}

func (m *JourneyMemory) Validate() error {
	errStr := ""
	err := validation.ValidateStruct(
		m,
		validation.Parameter(&m.Id, validation.Type("string"), validation.Required()),
		validation.Parameter(&m.Name, validation.Required()),
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
