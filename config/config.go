package conf

import (
	"encoding/json"
	"os"
	"time"

	"github.com/ShiinaAiiko/nyanya-trip-route-track/server/services/typings"
	"github.com/cherrai/nyanyago-utils/goroutinepanic"
	"github.com/cherrai/nyanyago-utils/nfile"
	"github.com/cherrai/nyanyago-utils/nlog"
	"github.com/cherrai/nyanyago-utils/nshortid"
	"github.com/cherrai/nyanyago-utils/saass"
	sso "github.com/cherrai/saki-sso-go"
	"github.com/go-ego/gse"
	"github.com/go-resty/resty/v2"
	"github.com/sashabaranov/go-openai"
)

var (
	log          = nlog.New()
	Config       *typings.Config
	SSO          *sso.SakiSSO
	SAaSS        *saass.SAaSS
	Qdrant       *QdrantDB
	Seg          gse.Segmenter
	OpenAIClient *openai.Client
	OpenAIModel  = "glm-4.7-flash"

	QdrantCollectionName = struct {
		IntentTemplates string
		Trip            string
		POI             string
	}{
		Trip:            "trip_segments",
		IntentTemplates: "intent_templates",
		POI:             "poi",
	}

	// 设置最大循环次数，防止死循环
	MaxIterations = 3
	RestyClient   = resty.New()
	G             = goroutinepanic.G
	FileTokenSign = "saass_2022_6_4"
	// 文件到期后根据时间进行删除 未做
	// []string{"Image", "Video", "Audio", "Text", "File"}
	FileExpirationRemovalDeadline = 60 * 3600 * 24 * time.Second
	// 临时文件删除期限
	TempFileRemovalDeadline = 60 * 3600 * 24 * time.Second

	// ToolApiUrl = "https://tools.aiiko.club"

	// NominatimApiUrl = "http://192.168.204.139:17010"
	// NominatimApiUrl = "https://nominatim.aiiko.club"
)

func GetConfig(configPath string) {
	// ToolApiUrl = "http://192.168.204.139:23201"

	jsonFile, _ := os.Open(configPath)

	defer jsonFile.Close()
	decoder := json.NewDecoder(jsonFile)

	conf := new(typings.Config)
	//Decode从输入流读取下一个json编码值并保存在v指向的值里
	err := decoder.Decode(&conf)
	if err != nil {
		log.Error(err)
	}

	appListPath := "appList.json"

	appList, _ := os.Open(appListPath)

	defer appList.Close()
	appListDecoder := json.NewDecoder(appList)

	openAppList := new([]*typings.OpenApp)
	//Decode从输入流读取下一个json编码值并保存在v指向的值里
	err = appListDecoder.Decode(&openAppList)
	if err != nil {
		log.Error(err)
	}

	for _, v := range conf.OpenApp {
		isExist := false
		for _, sv := range *openAppList {
			if v.AppName == sv.AppName && sv.AppKey != "" {
				isExist = true
				v.AppKey = sv.AppKey
			}
		}

		if !isExist {
			v.AppKey = nshortid.GetShortId(24)
		}
		// log.Info("isExist", !isExist, v.AppKey, v)
	}
	// log.Info("conf", conf)
	// log.Info("conf", conf.OpenApp)
	// log.Info("openAppList", openAppList)

	nfile.CreateJsonFile(appListPath, conf.OpenApp, true)

	Config = conf
}

func CheckAppKey(appKey string) bool {
	isExist := false
	for _, v := range Config.OpenApp {
		if v.AppKey != "" && v.AppKey == appKey {
			isExist = true
		}
	}
	return isExist
}
