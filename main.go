package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	conf "github.com/ShiinaAiiko/nyanya-trip-route-track/server/config"
	controllersV1 "github.com/ShiinaAiiko/nyanya-trip-route-track/server/controllers/v1"
	mongodb "github.com/ShiinaAiiko/nyanya-trip-route-track/server/db/mongo"
	redisdb "github.com/ShiinaAiiko/nyanya-trip-route-track/server/db/redis"
	dbxv1 "github.com/ShiinaAiiko/nyanya-trip-route-track/server/dbx/v1"
	"github.com/ShiinaAiiko/nyanya-trip-route-track/server/models"
	"github.com/ShiinaAiiko/nyanya-trip-route-track/server/services/gin_service"
	"github.com/ShiinaAiiko/nyanya-trip-route-track/server/services/typings"
	"github.com/sashabaranov/go-openai"

	"github.com/cherrai/nyanyago-utils/nlog"
	"github.com/cherrai/nyanyago-utils/nredis"
	"github.com/cherrai/nyanyago-utils/ntimer"
	"github.com/cherrai/nyanyago-utils/saass"
	sso "github.com/cherrai/saki-sso-go"

	// sfu "github.com/pion/ion-sfu/pkg/sfu"

	"github.com/go-redis/redis/v8"
)

var (
	log           = nlog.New()
	tripDbx       = dbxv1.TripDbx{}
	tripMemoryDbx = dbxv1.RgaTripMemoryDbx{}
	cityDbx       = dbxv1.CityDbx{}
	appDbx        = dbxv1.AppDbx{}
	jmDbx         = dbxv1.JourneyMemoryDbx{}

	aiController = controllersV1.AIController{}
	aiDbx        = dbxv1.AIDbx{}
	poiDbx       = dbxv1.POIDbx{}
)

func minifyJSON(src string) string {
	var buffer bytes.Buffer
	if err := json.Compact(&buffer, []byte(src)); err != nil {
		return src // 如果解析失败，原样返回
	}
	return buffer.String()
}

func fastFixJSON(raw string) string {
	raw = strings.TrimSpace(raw)
	if !strings.HasPrefix(raw, "{") {
		return raw
	}

	var stack []rune
	inString := false
	isEscaped := false

	// 遍历一次，找出所有未闭合的符号
	for _, char := range raw {
		if isEscaped {
			isEscaped = false
			continue
		}
		if char == '\\' {
			isEscaped = true
			continue
		}
		if char == '"' {
			inString = !inString
			continue
		}

		if !inString {
			if char == '{' || char == '[' {
				stack = append(stack, char)
			} else if char == '}' || char == ']' {
				if len(stack) > 0 {
					// 这里简化处理，假设输出是合规的，只弹出对应的
					stack = stack[:len(stack)-1]
				}
			}
		}
	}

	// 处理断在字符串内部的情况
	if inString {
		raw += `"`
	}

	// 逆序闭合所有栈内符号
	for i := len(stack) - 1; i >= 0; i-- {
		// 补全前先处理可能多余的逗号
		raw = strings.TrimSuffix(raw, ",")
		if stack[i] == '{' {
			raw += "}"
		} else if stack[i] == '[' {
			raw += "]"
		}
	}

	return raw
}

// 文件到期后根据时间进行删除 未做
func main() {
	nlog.SetPrefixTemplate("[{{Timer}}] [{{Type}}] [{{Date}}] [{{File}}]@{{Name}}")
	nlog.SetName("TRIP")

	conf.G.Go(func() error {
		configPath := ""
		for k, v := range os.Args {
			switch v {
			case "--config":
				if os.Args[k+1] != "" {
					configPath = os.Args[k+1]
				}

			}
		}
		if configPath == "" {
			log.Error("Config file does not exist.")
			return nil
		}
		conf.GetConfig(configPath)

		// Connect to redis.
		redisdb.ConnectRedis(&redis.Options{
			Addr:     conf.Config.Redis.Addr,
			Password: conf.Config.Redis.Password, // no password set
			DB:       conf.Config.Redis.DB,       // use default DB
		})
		log.Info(conf.Config.Redis.Addr)
		conf.Redisdb = nredis.New(context.Background(), &redis.Options{
			Addr:     conf.Config.Redis.Addr,
			Password: conf.Config.Redis.Password, // no password set
			DB:       conf.Config.Redis.DB,       // use default DB
		}, conf.BaseKey, log)
		conf.Redisdb.CreateKeys(conf.RedisCacheKeys)

		conf.SSO = sso.New(&sso.SakiSsoOptions{
			AppId:  conf.Config.SSO.AppId,
			AppKey: conf.Config.SSO.AppKey,
			Host:   conf.Config.SSO.Host,
			Rdb:    conf.Redisdb,
			Log:    log,
		})
		mongodb.ConnectMongoDB(conf.Config.Mongodb.Currentdb.Uri, conf.Config.Mongodb.Currentdb.Name)

		conf.SAaSS = saass.New(&saass.Options{
			AppId:      conf.Config.Saass.AppId,
			AppKey:     conf.Config.Saass.AppKey,
			BaseUrl:    conf.Config.Saass.BaseUrl,
			ApiVersion: conf.Config.Saass.ApiVersion,
		})

		// if conf.Config.Server.Mode == "debug" {
		qdrant, err := conf.NewQdrantClient(conf.Config.Qdrant.GrpcUrl, conf.Config.Qdrant.ApiKey)
		if err != nil {
			log.Error(err)
			return err
		}
		conf.Qdrant = qdrant
		// }

		openAIConf := openai.DefaultConfig(conf.Config.LLM.ApiKey) // 这里传入任意字符串作为 fake key
		openAIConf.BaseURL = conf.Config.LLM.BaseURL

		openAIConf.HTTPClient = &http.Client{
			Timeout: 0 * time.Second, // 设置超时
		}

		conf.OpenAIClient = openai.NewClientWithConfig(openAIConf)
		conf.OpenAIModel = conf.Config.LLM.Model
		// conf.OpenAIModel = "gemini-2.5-flash"

		conf.InitFsDB()
		// 初始化索引
		models.InitModelIndex()

		ntimer.SetTimeout(func() {

			// log.Info("AppFeatures", len(appDbx.GetAppFeaturesStr()))
			// log.Info("FeatureHydrator", dbxv1.FeatureHydrator)

			// if conf.Config.Server.Mode == "debug" {
			// conf.FsDB.ClearAll()
			// } else {
			// }

			// 定期处理最新切片
			ntimer.SetRepeatTimeTimer(func() {
				if err := tripDbx.GetAllTripMemory(); err != nil {
					log.FullCallChain(err.Error(), "Error")
				}
			}, ntimer.RepeatTime{
				Hour: 6,
			}, "Day")

			ntimer.SetTimeout(func() {
				// aiDbx.TestPOIBearing()
				// aiDbx.TestCallAgentTools()
				// log.Error(tripMemoryDbx.BatchCleanupAndReplanEmbedding(context.Background()))

				// log.Info(aiDbx.GetAgentTripTools([]string{dbxv1.SEARCH_NEARBY_POIS}))

				conf.G.Go(func() error {
					// POI脚本
					// return poiDbx.InitTripPOI()

					// return nil
					// TRIP RGA脚本
					// return tripDbx.GetAllTripMemory()
					// for {
					// 	err := tripDbx.GetAllTripMemory()
					// 	if err != nil {
					// 		// 打印错误日志，方便排查原因
					// 		log.Error("读取行程记忆失败: %v, 30秒后尝试重启...", err)

					// 		// 必须加休眠，防止死循环导致 CPU 100%
					// 		time.Sleep(30 * time.Second)
					// 		continue
					// 	}

					// 	// 如果逻辑上 GetAllTripMemory 是阻塞运行的，当它正常结束时也可以选择重新开始
					// 	log.Error("函数执行完毕，准备下一轮运行...")
					// }

					return nil
				})

				conf.G.Error(func(err error) {
					log.FullCallChain(err.Error(), "Error")
				})
				_ = conf.G.Wait()

			}, 1000)

			cityDbx.InitCityDistricts()

			conf.Seg.LoadDict()
			aiDbx.InitIntentLibraryToQdrant()

			log.Info("Done.")
		}, 1000)

		ntimer.SetTimeout(func() {

			// log.Info(conf.Config.OpenAI)
			return

			// --- 1. 定义数据结构 (结构化输出) ---

			// type RoadBookItem struct {
			// 	Day      int     `json:"day"`
			// 	Lat      float64 `json:"lat"`
			// 	Lng      float64 `json:"lng"`
			// 	Location string  `json:"location"`
			// 	Tips     string  `json:"tips"`
			// }

			// --- 2. 定义工具 (Agent Tools) ---

			// 模拟 RAG 检索函数：从向量数据库查你的 1.4万公里经验
			// getRAGContext := func(location string) string {
			// 	// 实际开发中，这里会先将 location 转向量，然后在 MongoDB/Redis 中检索
			// 	if location == "折多山" {
			// 		return "【自驾经验】折多山号称康巴第一关，海拔4298米。下坡路段长，注意刹车过热。"
			// 	}
			// 	return "【自驾经验】沿途路况良好，建议保持油量在50%以上。"
			// }
			searchPrivateRoadbook := func(query string) string {
				// 实际生产中：1. 将 query 转 Embedding 2. 查 MongoDB 向量索引
				log.Info("[Log] 正在检索私有路书库: %s...\n", query)
				if strings.Contains(query, "折多山") {
					return "【私密笔记】: 折多山垭口海拔4298米，下坡建议挂 L 挡利用引擎制动。垭口风极大，务必穿冲锋衣。"
				}
				if strings.Contains(query, "雅安") {
					return "【私密笔记】: 雅安是雨城，G318 起点附近路面湿滑，注意行车安全。"
				}
				return "经验提示：保持油量，注意高反。"
			}
			// 模拟 天气 API 工具
			getRealtimeWeather := func(city string) string {
				return fmt.Sprintf("%s天气：晴转多云，气温 5°C - 15°C，适合翻山。", city)
			}

			// 配置 LiteLLM 作为 OpenAI 兼容的提供商

			ctx := context.Background()

			config := openai.DefaultConfig("sk-peC3rgDTZwbhQKoE") // 这里传入任意字符串作为 fake key
			config.BaseURL = "http://localhost:17004/v1"
			// config := openai.DefaultConfig("6c620d1b0c954ea8a7f00bb68e2b67be.376kVik9VPNAN0Bt") // 这里传入任意字符串作为 fake key
			// config.BaseURL = "https://api.z.ai/api/paas/v4"
			// config := openai.DefaultConfig("6c620d1b0c954ea8a7f00bb68e2b67be.376kVik9VPNAN0Bt") // 这里传入任意字符串作为 fake key
			// config.BaseURL = "https://open.bigmodel.cn/api/paas/v4"                             // LiteLLM Proxy 地址
			config.HTTPClient = &http.Client{
				Timeout: 180 * time.Second, // 设置超时
			}

			modle := "glm-4.5-air"
			modle = "glm-4.7-flash"

			client := openai.NewClientWithConfig(config)

			log.Info("--- 3. 构造 Tool 定义 ---")

			tools := []openai.Tool{
				{
					Type: openai.ToolTypeFunction,
					Function: &openai.FunctionDefinition{
						Name:        "search_roadbook",
						Description: "检索私有路书库获取专业经验",
						Parameters:  json.RawMessage(`{"type":"object","properties":{"query":{"type":"string"}},"required":["query"]}`),
					},
				},
				{
					Type: openai.ToolTypeFunction,
					Function: &openai.FunctionDefinition{
						Name:        "get_weather",
						Description: "获取实时天气预报",
						Parameters:  json.RawMessage(`{"type":"object","properties":{"loc":{"type":"string"}},"required":["loc"]}`),
					},
				},
			}

			log.Info("初始用户输入")
			lastResult := `{
  "status": {
    "code": 200
  },
  "display": {
    "message": "为您规划了一条横跨中国东西的壮美路线！途经巴蜀、黄土高原、河西走廊，最终抵达西域明珠乌鲁木齐。全程约 2800 公里，建议 5-6 天完成，沿途可领略大漠孤烟、长河落日的壮阔景色。",
    "warning": "⚠️ 风险预警：1. 本行程单日驾驶距离较长，建议轮换驾驶；2. 途经高原路段（如兰州至张掖），注意高反；3. 沙漠路段需备足饮用水和油料；4. 乌鲁木齐早晚温差大，注意保暖。"
  },
  "data": {
    "summary": {
      "days": 5
    },
    "tls": [
      {
        "id": "",
        "desc": "从巴蜀腹地出发，翻越秦岭，沿兰海高速一路向西，穿越黄土高原，抵达西北重镇兰州。",
        "days": 1.5,
        "act": "ADD",
        "wpts": [
          {
            "id": "",
            "name": "重庆",
            "addr": "重庆市",
            "act": "ADD"
          },
          {
            "id": "",
            "name": "广元",
            "addr": "四川省广元市",
            "act": "ADD"
          },
          {
            "id": "",
            "name": "西安",
            "addr": "陕西省西安市",
            "act": "ADD"
          },
          {
            "id": "",
            "name": "兰州",
            "addr": "甘肃省兰州市",
            "act": "ADD"
          }
        ]
      },
      {
        "id": "",
        "desc": "从兰州出发，沿连霍高速穿越河西走廊，途经丹霞地貌，抵达嘉峪关，感受丝路文明的厚重。",
        "days": 1,
        "act": "ADD",
        "wpts": [
          {
            "id": "",
            "name": "兰州",
            "addr": "甘肃省兰州市",
            "act": "ADD"
          },
          {
            "id": "",
            "name": "张掖",
            "addr": "甘肃省张掖市",
            "act": "ADD"
          },
          {
            "id": "",
            "name": "嘉峪关",
            "addr": "甘肃省嘉峪关市",
            "act": "ADD"
          }
        ]
      },
      {
        "id": "",
        "desc": "深入河西走廊西端，探访敦煌莫高窟，领略大漠风光，穿越茫茫戈壁，抵达哈密。",
        "days": 1,
        "act": "ADD",
        "wpts": [
          {
            "id": "",
            "name": "嘉峪关",
            "addr": "甘肃省嘉峪关市",
            "act": "ADD"
          },
          {
            "id": "",
            "name": "敦煌",
            "addr": "甘肃省酒泉市敦煌市",
            "act": "ADD"
          },
          {
            "id": "",
            "name": "哈密",
            "addr": "新疆维吾尔自治区哈密市",
            "act": "ADD"
          }
        ]
      },
      {
        "id": "",
        "desc": "从哈密出发，穿越吐鲁番盆地，翻越天山，抵达乌鲁木齐，完成西域之旅。",
        "days": 1.5,
        "act": "ADD",
        "wpts": [
          {
            "id": "",
            "name": "哈密",
            "addr": "新疆维吾尔自治区哈密市",
            "act": "ADD"
          },
          {
            "id": "",
            "name": "吐鲁番",
            "addr": "新疆维吾尔自治区吐鲁番市",
            "act": "ADD"
          },
          {
            "id": "",
            "name": "乌鲁木齐",
            "addr": "新疆维吾尔自治区乌鲁木齐市",
            "act": "ADD"
          }
        ]
      }
    ]
  }
}`
			lastResult = `
{
  "tls": [
    {
      "id": "PLcADnvdIQs",
      "desc": "重庆市内段：从北碚区出发，经荣昌区",
      "days": 1,
      "act": "ADD",
      "wpts": [
        {
          "id": "IwudTeWFc58",
          "name": "北碚区",
          "addr": "中国重庆市北碚区北温泉街道",
          "act": "ADD"
        },
        {
          "id": "qtnrT3YwV3r",
          "name": "荣昌区",
          "addr": "中国重庆市荣昌区昌州街道",
          "act": "ADD"
        }
      ]
    },
    {
      "id": "Ef6dhJacFRc",
      "desc": "进入四川：经成都，前往川西高原",
      "days": 1,
      "act": "ADD",
      "wpts": [
        {
          "id": "qqRpl3ZQOI8",
          "name": "荣昌区",
          "addr": "中国重庆市荣昌区昌州街道",
          "act": "ADD"
        },
        {
          "id": "huw2Ldl6hb6",
          "name": "成都市",
          "addr": "四川省成都市青羊区西御河街道",
          "act": "ADD"
        }
      ]
    },
    {
      "id": "sVZW5NC7r75",
      "desc": "成都至马尔康段",
      "days": 1,
      "act": "ADD",
      "wpts": [
        {
          "id": "W7ITYP20gZQ",
          "name": "成都市青羊区光华街道",
          "addr": "四川省成都市青羊区光华街道",
          "act": "ADD"
        },
        {
          "id": "EKspZCJhSYv",
          "name": "马尔康市",
          "addr": "四川省阿坝藏族羌族自治州马尔康市马尔康镇",
          "act": "ADD"
        }
      ]
    },
    {
      "id": "gQzkc4Ir5iy",
      "desc": "川西高原段：马尔康至金川至甘孜",
      "days": 3,
      "act": "ADD",
      "wpts": [
        {
          "id": "aJQH2SvN0k8",
          "name": "马尔康市马尔康镇",
          "addr": "四川省阿坝藏族羌族自治州马尔康市马尔康镇",
          "act": "ADD"
        },
        {
          "id": "gBFagE4VNpJ",
          "name": "金川县观音桥镇",
          "addr": "四川省阿坝藏族羌族自治州金川县观音桥镇",
          "act": "ADD"
        },
        {
          "id": "VAsF3VRyeTY",
          "name": "甘孜县",
          "addr": "四川省甘孜藏族自治州甘孜县甘孜镇",
          "act": "ADD"
        }
      ]
    },
    {
      "id": "C52tPUsK58M",
      "desc": "进入西藏：甘孜至昌都",
      "days": 1,
      "act": "ADD",
      "wpts": [
        {
          "id": "a3zBfih8DCO",
          "name": "甘孜县呷拉乡",
          "addr": "四川省甘孜藏族自治州甘孜县呷拉乡",
          "act": "ADD"
        },
        {
          "id": "sSfGh6npWOc",
          "name": "昌都市",
          "addr": "西藏自治区昌都市卡若区城关镇",
          "act": "ADD"
        }
      ]
    },
    {
      "id": "Gyf5QgVdUKI",
      "desc": "昌都至洛隆至边坝",
      "days": 1,
      "act": "ADD",
      "wpts": [
        {
          "id": "MsxlvbezfMW",
          "name": "昌都市卡若区城关镇",
          "addr": "西藏自治区昌都市卡若区城关镇",
          "act": "ADD"
        },
        {
          "id": "c38GLRpFN3U",
          "name": "洛隆县孜托镇",
          "addr": "西藏自治区昌都市洛隆县孜托镇",
          "act": "ADD"
        },
        {
          "id": "mgZ8H1EYAsS",
          "name": "边坝县边坝镇",
          "addr": "西藏自治区昌都市边坝县边坝镇",
          "act": "ADD"
        }
      ]
    },
    {
      "id": "Olxd6rWBHLQ",
      "desc": "边坝至嘉黎",
      "days": 1,
      "act": "ADD",
      "wpts": [
        {
          "id": "SPPZb1vfvb1",
          "name": "边坝县边坝镇",
          "addr": "西藏自治区昌都市边坝县边坝镇",
          "act": "ADD"
        },
        {
          "id": "Dcgu5HH1WFx",
          "name": "嘉黎县阿扎镇",
          "addr": "西藏自治区那曲市嘉黎县阿扎镇",
          "act": "ADD"
        }
      ]
    },
    {
      "id": "HvytuuEkLVX",
      "desc": "嘉黎至拉萨",
      "days": 2,
      "act": "ADD",
      "wpts": [
        {
          "id": "XWvSXYT2RB8",
          "name": "嘉黎县阿扎镇",
          "addr": "西藏自治区那曲市嘉黎县阿扎镇",
          "act": "ADD"
        },
        {
          "id": "vxJJmXZ1NjF",
          "name": "拉萨市",
          "addr": "西藏自治区拉萨市城关区吉崩岗街道",
          "act": "ADD"
        }
      ]
    },
    {
      "id": "qUm8oxgUxtO",
      "desc": "拉萨至浪卡子至康马",
      "days": 1,
      "act": "ADD",
      "wpts": [
        {
          "id": "j4Y6TMnnhHl",
          "name": "拉萨市城关区",
          "addr": "西藏自治区拉萨市城关区吉崩岗街道",
          "act": "ADD"
        },
        {
          "id": "Gwp0ApP8Cow",
          "name": "浪卡子县打隆镇",
          "addr": "西藏自治区山南市浪卡子县打隆镇",
          "act": "ADD"
        },
        {
          "id": "pkYKvk0AIJ1",
          "name": "康马县康马镇",
          "addr": "西藏自治区日喀则市康马县康马镇",
          "act": "ADD"
        }
      ]
    },
    {
      "id": "Au7HSxczv0m",
      "desc": "康马至岗巴至定日",
      "days": 1,
      "act": "ADD",
      "wpts": [
        {
          "id": "QEwnWJu4jgD",
          "name": "康马县康马镇",
          "addr": "西藏自治区日喀则市康马县康马镇",
          "act": "ADD"
        },
        {
          "id": "UhRiykB2nK3",
          "name": "岗巴县岗巴镇",
          "addr": "西藏自治区日喀则市岗巴县岗巴镇",
          "act": "ADD"
        },
        {
          "id": "JjLs0oDr7ml",
          "name": "定日县协格尔镇",
          "addr": "西藏自治区日喀则市定日县协格尔镇",
          "act": "ADD"
        }
      ]
    },
    {
      "id": "jbVWyW3wbCW",
      "desc": "定日至珠峰大本营往返",
      "days": 2,
      "act": "ADD",
      "wpts": [
        {
          "id": "Pxhc2IgOzGy",
          "name": "定日县协格尔镇",
          "addr": "西藏自治区日喀则市定日县协格尔镇",
          "act": "ADD"
        },
        {
          "id": "e4vvPNNLTR7",
          "name": "珠峰大本营",
          "addr": "西藏自治区日喀则市定日县扎西宗镇",
          "act": "ADD"
        },
        {
          "id": "DmqQJOWUPLQ",
          "name": "定日县扎果镇",
          "addr": "西藏自治区日喀则市定日县扎果镇",
          "act": "ADD"
        }
      ]
    },
    {
      "id": "tyO6Z41vbtO",
      "desc": "定日至聂拉木至萨嘎",
      "days": 2,
      "act": "ADD",
      "wpts": [
        {
          "id": "VeontPJ0jrS",
          "name": "定日县扎果镇",
          "addr": "西藏自治区日喀则市定日县扎果镇",
          "act": "ADD"
        },
        {
          "id": "pVYXI7U71dd",
          "name": "聂拉木县波绒乡",
          "addr": "西藏自治区日喀则市聂拉木县波绒乡",
          "act": "ADD"
        },
        {
          "id": "UYb1GwDOSxp",
          "name": "萨嘎县加加镇",
          "addr": "西藏自治区日喀则市萨嘎县加加镇",
          "act": "ADD"
        }
      ]
    },
    {
      "id": "PlTS6kSxAT7",
      "desc": "萨嘎至仲巴至普兰至札达至噶尔（阿里地区）",
      "days": 3,
      "act": "ADD",
      "wpts": [
        {
          "id": "W4DgynJZHow",
          "name": "萨嘎县加加镇",
          "addr": "西藏自治区日喀则市萨嘎县加加镇",
          "act": "ADD"
        },
        {
          "id": "DIVUt2c6pNk",
          "name": "仲巴县拉让乡",
          "addr": "西藏自治区日喀则市仲巴县拉让乡",
          "act": "ADD"
        },
        {
          "id": "fD6KFHF2Vef",
          "name": "普兰县霍尔镇",
          "addr": "西藏自治区阿里地区普兰县霍尔镇",
          "act": "ADD"
        },
        {
          "id": "hZ4quDt3mk5",
          "name": "札达县托林镇",
          "addr": "西藏自治区阿里地区札达县托林镇",
          "act": "ADD"
        },
        {
          "id": "UoyUSTcVXRD",
          "name": "阿里地区噶尔县",
          "addr": "西藏自治区阿里地区噶尔县狮泉河镇",
          "act": "ADD"
        }
      ]
    },
    {
      "id": "j0JQUXwBuzE",
      "desc": "阿里北线：狮泉河至日土至班公湖至红土达坂",
      "days": 3,
      "act": "ADD",
      "wpts": [
        {
          "id": "g5aDee8rdhn",
          "name": "噶尔县狮泉河镇",
          "addr": "西藏自治区阿里地区噶尔县狮泉河镇",
          "act": "ADD"
        },
        {
          "id": "vbvFskkWYbp",
          "name": "日土县日土镇",
          "addr": "西藏自治区阿里地区日土县日土镇",
          "act": "ADD"
        },
        {
          "id": "MXhIjmv2jxF",
          "name": "班公湖观景台",
          "addr": "西藏自治区阿里地区日土县日松乡",
          "act": "ADD"
        },
        {
          "id": "B77BpY4NAjW",
          "name": "红土达坂",
          "addr": "西藏自治区阿里地区日土县东汝乡",
          "act": "ADD"
        }
      ]
    },
    {
      "id": "",
      "desc": "新藏线段：经三十里营房、麻扎至叶城",
      "days": 3,
      "act": "ADD",
      "wpts": [
        {
          "id": "aihiFxDlSN7",
          "name": "日土县东汝乡",
          "addr": "西藏自治区阿里地区日土县东汝乡",
          "act": "ADD"
        },
        {
          "id": "tz2UkJYeEPt",
          "name": "三十里营房",
          "addr": "新疆维吾尔自治区和田地区皮山县昆岭镇",
          "act": "ADD"
        },
        {
          "id": "YAwQkDvUEXa",
          "name": "麻扎",
          "addr": "新疆维吾尔自治区喀什地区叶城县玉叶镇",
          "act": "ADD"
        },
        {
          "id": "TIraNxwseCS",
          "name": "叶城县",
          "addr": "新疆维吾尔自治区喀什地区叶城县金果镇",
          "act": "ADD"
        }
      ]
    },
    {
      "id": "xTFLflbvQNf",
      "desc": "叶城至喀什",
      "days": 2,
      "act": "ADD",
      "wpts": [
        {
          "id": "ADzHudwIhqP",
          "name": "叶城县依提木孔镇",
          "addr": "新疆维吾尔自治区喀什地区叶城县依提木孔镇",
          "act": "ADD"
        },
        {
          "id": "TUJYPnKM3Bf",
          "name": "喀什市",
          "addr": "新疆维吾尔自治区喀什地区喀什市亚瓦格街道",
          "act": "ADD"
        }
      ]
    },
    {
      "id": "jq8sabdF65G",
      "desc": "喀什周边：木吉火山口、塔县、红其拉甫",
      "days": 1,
      "act": "ADD",
      "wpts": [
        {
          "id": "w7ni7wPArJK",
          "name": "喀什市夏马勒巴格镇",
          "addr": "新疆维吾尔自治区喀什地区喀什市夏马勒巴格镇",
          "act": "ADD"
        },
        {
          "id": "mzMzSQYCmKZ",
          "name": "木吉湿地泥火山群",
          "addr": "新疆维吾尔自治区克孜勒苏柯尔克孜自治州阿克陶县木吉乡",
          "act": "ADD"
        },
        {
          "id": "ofWAoIn8YqK",
          "name": "塔合曼乡",
          "addr": "新疆维吾尔自治区喀什地区塔什库尔干塔吉克自治县塔合曼乡",
          "act": "ADD"
        },
        {
          "id": "BqATlPv355o",
          "name": "塔什库尔干塔吉克自治县",
          "addr": "新疆维吾尔自治区喀什地区塔什库尔干塔吉克自治县塔什库尔干镇",
          "act": "ADD"
        },
        {
          "id": "Qt3hxQ63gEE",
          "name": "红其拉甫国门",
          "addr": "新疆维吾尔自治区喀什地区塔什库尔干塔吉克自治县达布达尔乡",
          "act": "ADD"
        },
        {
          "id": "eAqUh8VCDBX",
          "name": "喀什古城",
          "addr": "新疆维吾尔自治区喀什地区喀什市亚瓦格街道",
          "act": "ADD"
        }
      ]
    },
    {
      "id": "hoQbdX4TGWo",
      "desc": "喀什至中国西极",
      "days": 1,
      "act": "ADD",
      "wpts": [
        {
          "id": "i5ehbZQprk5",
          "name": "喀什古城",
          "addr": "新疆维吾尔自治区喀什地区喀什市亚瓦格街道",
          "act": "ADD"
        },
        {
          "id": "xIMoGqWOjCp",
          "name": "中国西极纪念碑",
          "addr": "新疆维吾尔自治区克孜勒苏柯尔克孜自治州乌恰县吉根乡",
          "act": "ADD"
        }
      ]
    },
    {
      "id": "MUsug8rMeAo",
      "desc": "西极至阿图什",
      "days": 1,
      "act": "ADD",
      "wpts": [
        {
          "id": "bUUDH62hYpr",
          "name": "中国西极纪念碑",
          "addr": "新疆维吾尔自治区克孜勒苏柯尔克孜自治州乌恰县吉根乡",
          "act": "ADD"
        },
        {
          "id": "xzBMiuJOYVr",
          "name": "阿图什市",
          "addr": "新疆维吾尔自治区克孜勒苏柯尔克孜自治州阿图什市幸福街道",
          "act": "ADD"
        }
      ]
    },
    {
      "id": "sGZHwfwnk62",
      "desc": "南疆段：阿图什至阿合奇至乌什至阿克苏",
      "days": 2,
      "act": "ADD",
      "wpts": [
        {
          "id": "bMcUdGSXWo5",
          "name": "阿图什市",
          "addr": "新疆维吾尔自治区克孜勒苏柯尔克孜自治州阿图什市幸福街道",
          "act": "ADD"
        },
        {
          "id": "bql0pI3ead3",
          "name": "阿合奇县阿合奇镇",
          "addr": "新疆维吾尔自治区克孜勒苏柯尔克孜自治州阿合奇县阿合奇镇",
          "act": "ADD"
        },
        {
          "id": "OG81iiT8vUB",
          "name": "乌什县乌什镇",
          "addr": "新疆维吾尔自治区阿克苏地区乌什县乌什镇",
          "act": "ADD"
        },
        {
          "id": "WTyQPBlCHjL",
          "name": "阿克苏地区",
          "addr": "新疆维吾尔自治区阿克苏地区阿克苏市红桥街道",
          "act": "ADD"
        }
      ]
    },
    {
      "id": "vfXVekBRtqX",
      "desc": "阿克苏至库车",
      "days": 2,
      "act": "ADD",
      "wpts": [
        {
          "id": "lN61yIapZty",
          "name": "阿克苏市新城街道",
          "addr": "新疆维吾尔自治区阿克苏地区库车市新城街道",
          "act": "ADD"
        },
        {
          "id": "Hdn4oY8KEgQ",
          "name": "天山托木尔景区",
          "addr": "新疆维吾尔自治区阿克苏地区温宿县佳木镇",
          "act": "ADD"
        },
        {
          "id": "NSo8Y6izLVw",
          "name": "库车市",
          "addr": "新疆维吾尔自治区阿克苏地区库车市新城街道",
          "act": "ADD"
        }
      ]
    },
    {
      "id": "cHaDpNG0J4w",
      "desc": "库车至独山子（独库公路北段）",
      "days": 3,
      "act": "ADD",
      "wpts": [
        {
          "id": "reli4JtV7yL",
          "name": "库车市新城街道",
          "addr": "新疆维吾尔自治区阿克苏地区库车市新城街道",
          "act": "ADD"
        },
        {
          "id": "s5aAr42geIb",
          "name": "独山子区",
          "addr": "新疆维吾尔自治区克拉玛依市独山子区西宁路街道",
          "act": "ADD"
        }
      ]
    },
    {
      "id": "rTUuAKjEui2",
      "desc": "独山子至克拉玛依",
      "days": 2,
      "act": "ADD",
      "wpts": [
        {
          "id": "xzfuKa6KSQO",
          "name": "独山子区西宁路街道",
          "addr": "新疆维吾尔自治区克拉玛依市独山子区西宁路街道",
          "act": "ADD"
        },
        {
          "id": "OwysaTLfDTm",
          "name": "克拉玛依区昆仑路街道",
          "addr": "新疆维吾尔自治区克拉玛依市克拉玛依区昆仑路街道",
          "act": "ADD"
        }
      ]
    },
    {
      "id": "PqrMRmCRIXb",
      "desc": "北疆段：克拉玛依至和布克赛尔至白哈巴至喀纳斯",
      "days": 3,
      "act": "ADD",
      "wpts": [
        {
          "id": "uGoxcbRYxDJ",
          "name": "克拉玛依区昆仑路街道",
          "addr": "新疆维吾尔自治区克拉玛依市克拉玛依区昆仑路街道",
          "act": "ADD"
        },
        {
          "id": "Y8wCZHCfuWX",
          "name": "和布克赛尔镇",
          "addr": "新疆维吾尔自治区塔城地区和布克赛尔蒙古自治县和布克赛尔镇",
          "act": "ADD"
        },
        {
          "id": "sNafNBYXwWo",
          "name": "白哈巴风景区",
          "addr": "新疆维吾尔自治区阿勒泰地区哈巴河县铁热克提乡",
          "act": "ADD"
        },
        {
          "id": "JJI5RxyFsXw",
          "name": "喀纳斯景区",
          "addr": "新疆维吾尔自治区阿勒泰地区布尔津县禾木喀纳斯蒙古族乡",
          "act": "ADD"
        }
      ]
    },
    {
      "id": "sByR48EfrLZ",
      "desc": "喀纳斯至阿勒泰",
      "days": 2,
      "act": "ADD",
      "wpts": [
        {
          "id": "JIJ5CcO6Rzs",
          "name": "喀纳斯景区",
          "addr": "新疆维吾尔自治区阿勒泰地区布尔津县禾木喀纳斯蒙古族乡",
          "act": "ADD"
        },
        {
          "id": "XTl67QqSXml",
          "name": "阿勒泰市阿苇滩镇",
          "addr": "新疆维吾尔自治区阿勒泰地区阿勒泰市阿苇滩镇",
          "act": "ADD"
        }
      ]
    },
    {
      "id": "XvRylEehyu7",
      "desc": "阿勒泰至可可托海",
      "days": 2,
      "act": "ADD",
      "wpts": [
        {
          "id": "gTEpwPLnu1T",
          "name": "阿勒泰市团结路街道",
          "addr": "新疆维吾尔自治区阿勒泰地区阿勒泰市团结路街道",
          "act": "ADD"
        },
        {
          "id": "Zv6DkFA8Euz",
          "name": "可可托海景区",
          "addr": "新疆维吾尔自治区阿勒泰地区富蕴县可可托海镇",
          "act": "ADD"
        }
      ]
    },
    {
      "id": "c6SyUtf5GZ2",
      "desc": "可可托海至乌鲁木齐至天池",
      "days": 4,
      "act": "ADD",
      "wpts": [
        {
          "id": "MfZZe4UzUCu",
          "name": "可可托海景区",
          "addr": "新疆维吾尔自治区阿勒泰地区富蕴县吐尔洪乡",
          "act": "ADD"
        },
        {
          "id": "ehsKNIGquHh",
          "name": "乌鲁木齐市",
          "addr": "新疆维吾尔自治区乌鲁木齐市水磨沟区南湖南路街道",
          "act": "ADD"
        },
        {
          "id": "skjaErELoKO",
          "name": "天山天池景区",
          "addr": "新疆维吾尔自治区昌吉回族自治州阜康市九运街镇",
          "act": "ADD"
        }
      ]
    },
    {
      "id": "pA2XzcTwI3g",
      "desc": "乌鲁木齐至吐鲁番至哈密",
      "days": 3,
      "act": "ADD",
      "wpts": [
        {
          "id": "JfrAIuX03Mp",
          "name": "乌鲁木齐县水西沟镇",
          "addr": "新疆维吾尔自治区乌鲁木齐市乌鲁木齐县水西沟镇",
          "act": "ADD"
        },
        {
          "id": "NJHVXkQ6mbu",
          "name": "吐鲁番市葡萄镇",
          "addr": "新疆维吾尔自治区吐鲁番市高昌区葡萄镇",
          "act": "ADD"
        },
        {
          "id": "OFy2mBPvS0G",
          "name": "哈密市东河街道",
          "addr": "新疆维吾尔自治区哈密市伊州区东河街道",
          "act": "ADD"
        }
      ]
    },
    {
      "id": "tOr8zDznpq1",
      "desc": "哈密至敦煌",
      "days": 3,
      "act": "ADD",
      "wpts": [
        {
          "id": "A8iRF31lqIa",
          "name": "哈密市东河街道",
          "addr": "新疆维吾尔自治区哈密市伊州区东河街道",
          "act": "ADD"
        },
        {
          "id": "GAfJ5V3zZEv",
          "name": "敦煌市",
          "addr": "甘肃省酒泉市敦煌市沙州镇",
          "act": "ADD"
        },
        {
          "id": "AWmLOHEY5P5",
          "name": "莫高窟景区",
          "addr": "甘肃省酒泉市敦煌市莫高镇",
          "act": "ADD"
        },
        {
          "id": "u5bbNB23rxG",
          "name": "月牙泉景区",
          "addr": "甘肃省酒泉市敦煌市月牙泉镇",
          "act": "ADD"
        },
        {
          "id": "OfIK3vcl78R",
          "name": "敦煌古城",
          "addr": "甘肃省酒泉市敦煌市七里镇",
          "act": "ADD"
        }
      ]
    },
    {
      "id": "X2M2VcmsGwj",
      "desc": "敦煌至嘉峪关至酒泉",
      "days": 3,
      "act": "ADD",
      "wpts": [
        {
          "id": "u3yOAqrQICL",
          "name": "敦煌市月牙泉镇",
          "addr": "甘肃省酒泉市敦煌市月牙泉镇",
          "act": "ADD"
        },
        {
          "id": "Qb5g0uZADr5",
          "name": "嘉峪关市",
          "addr": "甘肃省嘉峪关市钢城街道",
          "act": "ADD"
        },
        {
          "id": "TXh4ZssUBZu",
          "name": "酒泉市",
          "addr": "甘肃省酒泉市肃州区新城街道",
          "act": "ADD"
        }
      ]
    },
    {
      "id": "YOR58Jlz3KD",
      "desc": "酒泉至张掖七彩丹霞",
      "days": 3,
      "act": "ADD",
      "wpts": [
        {
          "id": "xEPwj1pNAD4",
          "name": "酒泉市肃州区雄关路",
          "addr": "甘肃省酒泉市肃州区新城街道雄关路",
          "act": "ADD"
        },
        {
          "id": "B825KPb0nIA",
          "name": "临泽县沙河镇",
          "addr": "甘肃省张掖市临泽县沙河镇",
          "act": "ADD"
        },
        {
          "id": "HIVYSnmkhaH",
          "name": "张掖七彩丹霞景区",
          "addr": "甘肃省张掖市临泽县倪家营镇",
          "act": "ADD"
        },
        {
          "id": "Ax3RV6rXRhy",
          "name": "张掖市甘州区新墩镇",
          "addr": "甘肃省张掖市甘州区新墩镇",
          "act": "ADD"
        }
      ]
    },
    {
      "id": "GxoKWvLudn4",
      "desc": "张掖至兰州",
      "days": 3,
      "act": "ADD",
      "wpts": [
        {
          "id": "y4cXpdHZssK",
          "name": "张掖市甘州区乌江镇",
          "addr": "甘肃省张掖市甘州区乌江镇",
          "act": "ADD"
        },
        {
          "id": "CEAmI4UjvjQ",
          "name": "兰州市七里河区西园街道",
          "addr": "甘肃省兰州市七里河区西园街道",
          "act": "ADD"
        }
      ]
    },
    {
      "id": "PDD6XwNxMTU",
      "desc": "兰州至天水至西安",
      "days": 3,
      "act": "ADD",
      "wpts": [
        {
          "id": "PG0NYwqLkyP",
          "name": "兰州市城关区皋兰路街道",
          "addr": "甘肃省兰州市城关区皋兰路街道金昌南路",
          "act": "ADD"
        },
        {
          "id": "AUeUfEA4LGA",
          "name": "天水市秦州区玉泉镇",
          "addr": "甘肃省天水市秦州区玉泉镇",
          "act": "ADD"
        },
        {
          "id": "vhc6UelHw2T",
          "name": "西安市莲湖区北院门街道",
          "addr": "陕西省西安市莲湖区北院门街道",
          "act": "ADD"
        }
      ]
    },
    {
      "id": "WqKMcSJHnki",
      "desc": "西安市区景点：大唐不夜城、兵马俑",
      "days": 3,
      "act": "ADD",
      "wpts": [
        {
          "id": "LnwsiqAMos5",
          "name": "大唐不夜城",
          "addr": "陕西省西安市碑林区太乙路街道",
          "act": "ADD"
        },
        {
          "id": "iP0hmPfAzOC",
          "name": "兵马俑",
          "addr": "陕西省西安市临潼区代王街道",
          "act": "ADD"
        }
      ]
    },
    {
      "id": "JXbcD6DEFhI",
      "desc": "西安至安康至达州至重庆（返程）",
      "days": 3,
      "act": "ADD",
      "wpts": [
        {
          "id": "RNq8J1nro2Y",
          "name": "西安市雁塔区长延堡街道",
          "addr": "陕西省西安市雁塔区长延堡街道",
          "act": "ADD"
        },
        {
          "id": "jP2fHEZbYBo",
          "name": "安康市汉滨区江北街道",
          "addr": "陕西省安康市汉滨区江北街道",
          "act": "ADD"
        },
        {
          "id": "tBNrBEbHvAQ",
          "name": "重庆市城口县葛城街道",
          "addr": "重庆市城口县葛城街道",
          "act": "ADD"
        },
        {
          "id": "sSuO5liMvvZ",
          "name": "达州市通川区复兴镇",
          "addr": "四川省达州市通川区复兴镇",
          "act": "ADD"
        }
      ]
    },
    {
      "id": "KAtldIEynZe",
      "desc": "达州至重庆（终点）",
      "days": 2,
      "act": "ADD",
      "wpts": [
        {
          "id": "eXMMDTdaJUu",
          "name": "达州市通川区复兴镇",
          "addr": "四川省达州市通川区复兴镇",
          "act": "ADD"
        },
        {
          "id": "CYgrbkVneKd",
          "name": "重庆市北碚区",
          "addr": "重庆市北碚区北温泉街道",
          "act": "ADD"
        }
      ]
    }
  ]
}
`
			log.Info(len(lastResult), len(minifyJSON(lastResult)))

			systemPrompt := `
# Role
你是一个资深的自驾旅行领队和地理专家。你负责根据用户的需求生成或修改结构化的自驾路书。

# Goals
1. 识别用户意图（创建、修改、或无关闲聊）。
2. 生成符合特定 JSON Schema 的路书数据。
3. 确保地理逻辑合理（不走回头路、行程强度适中）。
4. 无关闲聊则tls返回空数组，status.code返10002
5. 单个tls的途径点不能过多，过多应该拆分成多个tls。

# Output Schema
必须严格按以下 JSON 格式输出，不得包含任何 Markdown 代码块外的文字：
{
  "status": {
		"code": 200, // 成功 200 / 失败 10001 / 无关闲聊 10002
	}, 
  "display": {
    "message": "回复用户的口语化文字，简述行程亮点",
    "warning": "风险预警（如高反、长途疲劳、路况风险），若无则忽略"
  },
  "data": {
    "summary": {
		   "days": 45 // 必须是正整数！计算出的 1.5 必须写成 2
		},
    "tls": [ // 时间线
      {
        "id": "已有保留，新建留空",
        "desc": "该阶段简短介绍",
        "days": 3, // 必须是正整数！计算出的 1.5 必须写成 2
        "act": "ADD/UPDATE/KEEP",
        "wpts": [ // 途径点
				  // 这里的途径点位合计距离不能过长，不能数量过多，不能毫无关系性（比如重庆到兰州，路过西安，那么应该分多个tls），超过请新开一个 tls 数组元素
          {
            "id": "已有保留，新建留空",
            "name": "地点名称",
            "addr": "省+市+区县+详细地名描述（用于 GPS 搜索）",
            "act": "ADD/UPDATE/KEEP"
          }
        ]
      }
    ]
  }
}

# Output Format (Priority 1)
- **必须输出压缩后的单行 JSON (Minified JSON)**。
- **严禁包含任何换行符 (\n)、缩进空格或多余的空格**。
- **禁止包含 Markdown 代码块符号**，直接输出 JSON 字符串。

# Output Schema (Minified Template)
{"status":{"code":200},"display":{"message":"str","warning":"str"},"data":{"summary":{"days":0},"tls":[{"id":"str","desc":"str","days":0,"act":"KEEP","wpts":[{"id":"str","name":"str","addr":"str","act":"KEEP"}]}]}}

# days参数估算规则 (Critical)
- **强制整数化**：所有 days 字段（包括 summary 和 tls 级）必须且只能是**正整数**（1, 2, 3...）。
- **取整逻辑**：严禁输出小数（如 0.5, 1.5）。若计算结果有余数，请使用**向上取整（Ceil）**原则。例如：计划行驶 600 公里，按单日 500km 上限计算，days 必须设为 2。
- **最小单位**：任何行程阶段的最小耗时不得低于 1 天。

偏好权重：
默认（高速优先）：平均车速按 80km/h 计算，每日有效驾驶时间 6-8 小时。

国道偏好：
若用户提到“走国道”、“看风景”，平均车速必须降至 40-50km/h，days 至少增加 1 - 2 倍。
计算逻辑：days = (两点间里程 / 对应时速) / 每日驾驶时间。请务必根据常识预估里程，不要给出不可达的计划。

# Constraints
- 相关性判断：若用户问题与路书、旅行、地理无关，请设置 code 为 10001，并在 message 中温柔拒绝。
- 地址准确性：addr 字段必须包含省市区县全称，严禁只写景点简称。
- 状态维护：若输入中包含 Id，修改该点位时必须保留原 Id 并设置 act 为 UPDATE；新增点位 Id 留空且 act 为 ADD。
- 语言风格：专业、热情、老练。`

			messages := []openai.ChatCompletionMessage{
				{
					Role:    openai.ChatMessageRoleSystem,
					Content: systemPrompt,
				},
				// 将之前几轮用户表达过的、至今仍然有效的“全局偏好”总结成一段话
				// 或者AI 自我总结（最优雅，耗费 1 次额外 API 调用）
				{
					Role:    openai.ChatMessageRoleUser,
					Content: "历史需求汇总：" + strings.Join([]string{
						// "帮我生成重庆到乌鲁木齐的路书",
						// "你推荐我去的景区，都在途径点添加一下，免得我忘了。",
						// "我还想去酒泉卫星发射中心，以及汉中停留玩玩",
						// "路过四川巴中的时候，可以多看看",
						// "出发的时候，在重庆多看看",
					}, " | "),
				},
				{
					Role:    openai.ChatMessageRoleAssistant,
					Content: minifyJSON(lastResult), // 把旧结果喂回去
				},
				// 当前
				{
					Role:    openai.ChatMessageRoleUser,
					Content: "帮我把每一个缺失desc的时间线都加上desc",
				},
				// {
				// 	Role:    openai.ChatMessageRoleUser,
				// 	Content: "SpaceX是什么",
				// },
			}

			log.Info(" --- 4. 第一步：发起请求让 AI 决定是否调用工具 ---")
			resp, err := client.CreateChatCompletion(ctx, openai.ChatCompletionRequest{
				Model:       modle,
				Messages:    messages,
				Tools:       tools,
				MaxTokens:   8192, // 确保长路书不被腰斩
				Temperature: 0.5,  // 兼顾准确性与文采
				TopP:        0.8,  // 配合 Temperature 进一步稳定输出
			})
			if err != nil {
				log.Error(err)
			}

			msg := resp.Choices[0].Message
			log.Info(msg.ToolCalls)
			if len(msg.ToolCalls) > 0 {
				messages = append(messages, msg) // 把 AI 的调用请求存入上下文

				for _, tc := range msg.ToolCalls {
					var toolResult string
					switch tc.Function.Name {
					case "search_roadbook":
						var args struct{ Query string }
						json.Unmarshal([]byte(tc.Function.Arguments), &args)
						toolResult = searchPrivateRoadbook(args.Query) // 执行 RAG
					case "get_weather":
						var args struct{ Loc string }
						json.Unmarshal([]byte(tc.Function.Arguments), &args)
						toolResult = getRealtimeWeather(args.Loc) // 执行 API
					}

					// 把工具执行结果反馈给 AI
					messages = append(messages, openai.ChatCompletionMessage{
						Role:       openai.ChatMessageRoleTool,
						Content:    toolResult,
						ToolCallID: tc.ID,
					})
				}
			}

			// --- 5. 第二步：流式输出最终结果 ---
			stream, err := client.CreateChatCompletionStream(ctx, openai.ChatCompletionRequest{
				Model:    modle,
				Messages: messages,
				Stream:   true,

				MaxTokens:   8192, // 确保长路书不被腰斩
				Temperature: 0.5,  // 兼顾准确性与文采
				TopP:        0.9,  // 配合 Temperature 进一步稳定输出
				ResponseFormat: &openai.ChatCompletionResponseFormat{
					Type: openai.ChatCompletionResponseFormatTypeJSONObject,
				},
			})
			if err != nil {
				log.Error(err)
			}
			defer stream.Close()

			log.Info("--- 最终路书生成中 ---")

			var fullContent strings.Builder
			log.Info("\n>>> AI 思考完成，正在生成路书数组：")

			for {
				response, err := stream.Recv()

				if errors.Is(err, io.EOF) {
					log.Error("err", err)
					break
				}
				content := response.Choices[0].Delta.Content
				log.Info(content)
				fullContent.WriteString(content)
			}

			// --- 7. 清理并解析 JSON (关键步骤) ---
			finalJSON := strings.Trim(fullContent.String(), "` \n")
			finalJSON = strings.TrimPrefix(finalJSON, "json")
			finalJSON = fastFixJSON(finalJSON)

			log.Info(finalJSON)

			var res typings.AIRoadbookResponse
			if err := json.Unmarshal([]byte(finalJSON), &res); err != nil {
				// 如果解析失败，可能是 AI 输出带了前缀，这里可以做更强的正则清洗
				log.Error("\n解析 JSON 失败: %v", err)
			} else {

				log.Info(msg.ToolCalls)
				log.Info("\n\n成功解析！: %s",
					res)
			}

			// // 选择模型（使用你在 LiteLLM 中配置的模型名）

			// log.Info("生成文本（非流式）")

			// req := openai.ChatCompletionRequest{
			// 	Model: modle, // 或 claude-3-5-sonnet、gemini-2.0-flash 等 LiteLLM 支持的模型名
			// 	Messages: []openai.ChatCompletionMessage{
			// 		{
			// 			Role:    "system",
			// 			Content: "分段落回复",
			// 		},
			// 		{
			// 			Role:    "user",
			// 			Content: "50字内介绍下你",
			// 		},
			// 	},
			// 	Temperature: 0.7,
			// 	MaxTokens:   10000,
			// }

			// resp, err := client.CreateChatCompletion(ctx, req)
			// if err != nil {
			// 	log.Error("调用失败: %v\n", err)
			// 	return
			// }

			// // 输出完整回复
			// log.Info("=== 非流式回复 ===")
			// log.Info(resp.Choices[0].Message.Content)
			// log.Info("\n=== 使用统计 ===\n")
			// log.Info("Prompt tokens: %d\n", resp.Usage.PromptTokens)
			// log.Info("Completion tokens: %d\n", resp.Usage.CompletionTokens)
			// log.Info("Total tokens: %d\n", resp.Usage.TotalTokens)

			// log.Info("流式生成（更适合路书这种长内容）")

			// req1 := openai.ChatCompletionRequest{
			// 	Model: modle,
			// 	Messages: []openai.ChatCompletionMessage{
			// 		{
			// 			Role:    openai.ChatMessageRoleSystem,
			// 			Content: "可爱的分段落回复",
			// 		},
			// 		{
			// 			Role:    openai.ChatMessageRoleUser,
			// 			Content: "200字内介绍下你",
			// 		},
			// 	},
			// 	Stream:      true, // 开启流式
			// 	Temperature: 0.8,
			// }

			// // 创建流式会话
			// stream, err := client.CreateChatCompletionStream(context.Background(), req1)
			// if err != nil {
			// 	log.Error("CreateChatCompletionStream error: %v", err)
			// }
			// defer stream.Close()

			// log.Info("=== 流式回复（实时输出）===")
			// log.Info("🤖: ")

			// results := ""

			// // 循环接收流式数据块
			// for {
			// 	response, err := stream.Recv()
			// 	if errors.Is(err, io.EOF) {
			// 		// 流结束
			// 		log.Info("\n\n=== 流式传输完成 ===")
			// 		log.Info(results)
			// 		return
			// 	}
			// 	if err != nil {
			// 		log.Error("Stream recv error: %v", err)
			// 	}

			// 	// 提取 delta 内容并实时打印
			// 	if len(response.Choices) > 0 {
			// 		delta := response.Choices[0].Delta.Content
			// 		if delta != "" {
			// 			log.Info(delta) // 实时输出每个 token
			// 			results += delta
			// 		}
			// 	}
			// }

			log.Info("Done.")
		}, 1200)

		// ntimer.SetTimeout(func() {
		// 	// 2代表精度，这种方式会有小数点后无效的0的情况
		// 	a := 9.286157608032227
		// 	log.Info(strconv.FormatFloat(a, 'f', 3, 64))
		// 	// 效果同上
		// 	log.Info(fmt.Sprintf("%.3f", a))
		// 	log.Info(strconv.ParseFloat(fmt.Sprintf("%.3f", a), 64))
		// 	// g可以去掉小数点后无效的0
		// 	log.Info(fmt.Sprintf("%g", a))
		// 	// 效果同上，可以去掉0，但是达不到保留指定位数的效果
		// 	log.Info(strconv.FormatFloat(a, 'g', -1, 64))

		// }, 1000)

		// ntimer.SetTimeout(func() {
		// 	// folder := "./data"
		// 	// files, _ := ioutil.ReadDir(folder)
		// 	// for _, file := range files {
		// 	// 	if !file.IsDir() {
		// 	// 		jsonFile, _ := os.Open(folder + "/" + file.Name())
		// 	// 		defer jsonFile.Close()
		// 	// 		decoder := json.NewDecoder(jsonFile)

		// 	// 		mt := new(models.Trip)
		// 	// 		//Decode从输入流读取下一个json编码值并保存在v指向的值里
		// 	// 		err := decoder.Decode(&mt)
		// 	// 		if err != nil {
		// 	// 			fmt.Println("Error:", err)
		// 	// 		}
		// 	// 		log.Info("mt", folder+"/"+file.Name(), mt.Id, len(mt.Positions))
		// 	// 		addTrip, err := tripDbx.AddTrip(mt)
		// 	// 		log.Info("	addTrip, err ", addTrip, err)
		// 	// 	}
		// 	// }

		// 	// AszmV32Jc
		// 	// ajSVMD0R5
		// 	// FJO6cjMBV

		// 	trips, err := tripDbx.GetTrips("78L2tkleM", "Drive", 1, 20, 1715089938, 1715489938)

		// 	log.Info(len(trips), err)
		// 	for _, v := range trips {
		// 		tripPositions, err := tripDbx.GetTripPositions(v.Id, "", "")
		// 		log.Info(len(tripPositions.Positions), err)

		// 		// for _, v := range trip.Positions {

		// 		// 	// log.Info("gss", methods.GSS(v, trip.StartTime, trip.EndTime))
		// 		// }

		// 		vPositions, existsTimestamp := tripDbx.FilterPositions(tripPositions.Positions, tripPositions.StartTime, tripPositions.EndTime)
		// 		log.Info("vPositions", len(vPositions), len(existsTimestamp))
		// 		if len(existsTimestamp) != len(tripPositions.Positions) && len(tripPositions.Positions) > 15000 {

		// 			// tripDbx.PermanentlyDeleteTrip(v.Id)
		// 			newTrip := &models.Trip{
		// 				Type:      v.Type,
		// 				Positions: vPositions,

		// 				Marks: v.Marks,
		// 				// Statistics: ts,
		// 				AuthorId:   v.AuthorId,
		// 				Status:     0,
		// 				CreateTime: v.CreateTime,
		// 				StartTime:  v.StartTime,
		// 				EndTime:    v.EndTime,
		// 			}
		// 			jsonStr, _ := json.Marshal(newTrip)
		// 			log.Info(len(jsonStr))
		// 			log.Info("更新数据，超长数据", len(vPositions), len(tripPositions.Positions))

		// 			err := os.WriteFile("./data/"+v.Id+".json", jsonStr, 0666)

		// 			log.Info("writeFile", err)
		// 			// ts, _, _ := tripDbx.GetTripStatistics(v.Id, v.EndTime, false)
		// 			// addTrip, err := tripDbx.AddTrip(&models.Trip{
		// 			// 	Type:      v.Type,
		// 			// 	Positions: vPositions,

		// 			// 	Marks: v.Marks,
		// 			// 	// Statistics: ts,
		// 			// 	AuthorId:   v.AuthorId,
		// 			// 	Status:     0,
		// 			// 	CreateTime: v.CreateTime,
		// 			// 	StartTime:  v.StartTime,
		// 			// 	EndTime:    v.EndTime,
		// 			// })

		// 			// log.Info("addTrip, err", addTrip, err)
		// 			// if err = tripDbx.CheckPositions(tripPositions); err != nil {

		// 			// 	log.Info("更新数据err", err)
		// 			// 	return
		// 			// }
		// 		}
		// 		log.Info("结束")
		// 	}

		// 	// s, err := tripDbx.GetTripStatistics("YONqRoKIt", 0)
		// 	// log.Info(s, err)

		// }, 1500)

		gin_service.Init()

		return nil
	})

	conf.G.Error(func(err error) {
		log.FullCallChain(err.Error(), "Error")
	})
	conf.G.Wait()
}
