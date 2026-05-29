package dbxV1

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	conf "github.com/ShiinaAiiko/nyanya-trip-route-track/server/config"
	"github.com/ShiinaAiiko/nyanya-trip-route-track/server/models"
	"github.com/ShiinaAiiko/nyanya-trip-route-track/server/protos"
	"github.com/ShiinaAiiko/nyanya-trip-route-track/server/services/methods"
	"github.com/cherrai/nyanyago-utils/narrays"
	"github.com/cherrai/nyanyago-utils/ncommon"
	"github.com/cherrai/nyanyago-utils/nshortid"
	"github.com/cherrai/nyanyago-utils/nstrings"
	"github.com/cherrai/nyanyago-utils/validation"
	"github.com/gin-gonic/gin"
	"github.com/gofrs/uuid"
	"github.com/qdrant/go-client/qdrant"
	"github.com/sashabaranov/go-openai"
)

type AIDbx struct {
}

var (
	aiDbx       = AIDbx{}
	roadbookDbx = RoadbookDbx{}
)

type Point struct{ Lat, Lng float64 }

// func LineStringToGeoJSON(ls orb.LineString) (string, error) {
// 	// 1. 封装为 Feature（地理要素）
// 	feature := geojson.NewFeature(ls)

// 	// 2. 注入元数据（可选，方便在地图工具中识别）
// 	feature.Properties["generator"] = "AI-Road-Book-Simplifier"
// 	feature.Properties["point_count"] = len(ls)

// 	// 3. 序列化为带缩进的 JSON（方便人类阅读和调试）
// 	// 如果追求极致性能，可以使用 json.Marshal(feature)
// 	bytes, err := json.Marshal(feature)
// 	if err != nil {
// 		return "", err
// 	}

// 	return string(bytes), nil

// }

func (s *AIDbx) Round(val float64, precision int) float64 {
	p := math.Pow10(precision)
	return math.Round(val*p) / p
}

// 辅助函数：将时间转化为语义
func (s *AIDbx) getTimeOfDay(t time.Time) string {
	hour := t.Hour()
	switch {
	case hour >= 5 && hour < 9:
		return "清晨"
	case hour >= 9 && hour < 12:
		return "上午"
	case hour >= 12 && hour < 14:
		return "正午"
	case hour >= 14 && hour < 18:
		return "下午"
	case hour >= 18 && hour < 20:
		return "傍晚"
	default:
		return "深夜"
	}
}
func (s *AIDbx) GetTimeOfDayFromHours(startH, endH int) []string {
	periodMap := make(map[string]bool)

	// 处理跨零点逻辑：如果是 22 到 5，则遍历 22, 23, 0, 1, 2, 3, 4
	curr := startH
	for {
		// 调用你现有的函数，获取该小时对应的标签
		// 注意：getTimeOfDay 需要 time.Time，我们构造一个模拟时间
		mockTime := time.Date(2026, 1, 1, curr, 0, 0, 0, time.Local)
		period := aiDbx.getTimeOfDay(mockTime) // 使用你提供的函数
		periodMap[period] = true

		if curr == endH {
			break
		}
		curr = (curr + 1) % 24
	}

	// 转为切片
	var periods []string
	for p := range periodMap {
		periods = append(periods, p)
	}
	return periods
}

type EmbedReq struct {
	Model string   `json:"model"`
	Input []string `json:"input"`
}

type EmbedRes struct {
	Embeddings [][]float32 `json:"embeddings"`
}

type TextType string

const (
	// 检索
	TextTypeQuery TextType = "query"
	// 存储
	TextTypeDocument TextType = "document"
)

// GetEmbedding 专注处理单条路书文案
func (d *AIDbx) GetEmbedding(textType TextType, text string) ([]float32, error) {
	// Nomic 存储规范：必须带上 search_document: 前缀
	data := EmbedReq{
		Model: "nomic-embed-text",
		Input: []string{ncommon.IfElse(textType == TextTypeDocument, "search_document: ", "search_query: ") + text},
	}

	payload, _ := json.Marshal(data)

	// count++
	// baseUrl := ncommon.IfElse(count%2 == 0, "https://llm.aiiko.club", conf.Config.LLM.BaseURL)
	baseUrl := conf.Config.LLM.BaseURL

	// log.Info("baseUrl", baseUrl)

	// 2. 修改为 LiteLLM 标准路径 (注意是 /v1/embeddings)
	url := baseUrl + "/v1/embeddings"

	client := &http.Client{Timeout: 30 * time.Second}

	// 3. LiteLLM 需要鉴权，必须改用 http.NewRequest
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(payload))
	if err != nil {
		return nil, err
	}

	// 注入你的 master_key
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+conf.Config.LLM.ApiKey)

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("litellm error: %d", resp.StatusCode)
	}

	// 4. 适配逻辑：内部解析 OpenAI 格式，然后填入你的 EmbedRes
	var openAIRes struct {
		Data []struct {
			Embedding []float32 `json:"embedding"`
		} `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&openAIRes); err != nil {
		return nil, err
	}

	if len(openAIRes.Data) == 0 {
		return nil, fmt.Errorf("no embeddings returned from litellm")
	}

	// 5. 保持输出格式不变，封装进你的 EmbedRes
	res := EmbedRes{
		Embeddings: [][]float32{openAIRes.Data[0].Embedding},
	}

	return res.Embeddings[0], nil
}

var SANamespace = uuid.Must(uuid.FromString("695fb37f-bd3d-40cb-8069-fcde614ca166"))

func (d *AIDbx) GeneratePointID(kw string) string {
	// 构造唯一的 Name：例如 "2026-tibet:seg:0"
	name := kw

	// 使用 NewV5 生成确定性的 UUID
	// 只要 Namespace 和 name 一样，结果永远一样
	u := uuid.NewV5(SANamespace, name)

	return u.String()
}

// IntentTemplate 定义意图模板
type IntentTemplate struct {
	Category     string
	Description  string
	Examples     []string
	Keywords     []string // 用于 Qdrant 的 Payload 过滤或后置校验
	IsSaveQdrant bool
}

type IntentVectorsItem struct {
	Example string
	Vector  []float32
}

// IntentVector 存储计算后的向量特征
type IntentVector struct {
	Category string
	Vectors  []*IntentVectorsItem // 一个类别对应多个范句向量
}

var templates = []IntentTemplate{
	{
		Category:    "INTENT_STATS", // 【车辆/行程数据】
		Description: "查询行驶里程、速度、海拔、能耗等车辆统计数据",
		Keywords:    []string{"公里", "多远", "速", "海拔", "高度", "爬升", "能耗", "油", "电", "路程", "行程"},
		Examples: []string{
			"查询累计行驶里程", "今天跑了多远", "现在的平均时速", "查看实时海拔高度",
			"累计爬升了多少米", "当前的能耗曲线", "剩余续航里程", "查看胎压和温度",
			"统计驾驶数据", "今天的行程摘要", "电池百分比", "油耗是多少",
			"开了多久了", "现在速度多少", "路程统计",
			"查询一下我跑了多远",
			"你对我历史行程记录来个总的评价呢？",
		},
		IsSaveQdrant: true,
	},
	{
		Category:    "INTENT_WEATHER", // 【天气/气象查询】
		Description: "查询当前、未来天气情况，以及空气质量、穿衣指数、气象预警等信息",
		Keywords:    []string{"天", "气", "雨", "雪", "晴", "风", "温", "度", "空气", "预报", "伞", "冷", "热"},
		Examples: []string{
			"今天天气怎么样", "明天会下雨吗", "现在的气温是多少", "这附近空气质量好吗",
			"还要下多久的雨", "查看未来三天的天气预报", "今天需要带伞吗", "现在的风力等级",
			"紫外线强度高吗", "几点开始下雪", "这地方今天多少度", "有没有大风预警",
			"查询目的地的天气情况", "现在适合户外活动吗", "路面会有积雪吗",
		},
		IsSaveQdrant: true, // 建议保存，方便大模型在后续对话中记忆当前气象背景
	},
	{
		Category:    "INTENT_GEOGRAPHY", // 【地理/环境百科】
		Description: "询问城市、地理位置、地名、山川河流、周边景点",
		Keywords:    []string{"山", "县", "河", "景点", "服务区", "历史", "文化", "哪里", "位置"},
		Examples: []string{
			"这是什么山", "前面是哪个县城", "周围有什么好玩的", "最近的服务区在哪里",
			"介绍一下这条河", "这一带的历史人文", "这地方有什么特产", "当前地理位置坐标",
			"离国道还有多远", "周边景点推荐", "查看实时地图", "现在的气温和天气",
			"这附近有什么名胜古迹", "这一片是什么地形",
		},
		IsSaveQdrant: true,
	},
	{
		Category:    "INTENT_NAVIGATION", // 【站内导览/功能跳转】
		Description: "用户询问如何使用 App、查找特定页面、或者请求跳转到某个站内功能模块",
		Keywords:    []string{"怎么", "哪里", "功能", "页面", "设置", "轨迹", "历史", "记录", "主页", "打开", "去"},
		Examples: []string{
			"怎么看历史轨迹", "如何开始记录行程", "我的运动记录在哪里", "跳转到设置页面",
			"你能做什么", "有哪些功能", "怎么开启导航", "去主页", "我想看以前跑过的路",
			"如何修改个人信息", "怎么看海拔曲线", "站内搜索在哪", "帮我打开历史列表",
			"我想了解这个 App 的用法", "功能介绍一下", "联系开发者",
			"了解所有功能",
			"打开旅途记忆",
			"打开设置",
			"打开历史行程轨迹",
		},
		IsSaveQdrant: false, // 导航意图通常是即时指令，不需要长期记忆，除非你想分析用户最常用的功能
	},
	// {
	// 	Category:    "INTENT_KNOWLEDGE", // 【通用/人物/百科】
	// 	Description: "询问人物百科、历史常识、科普知识。",
	// 	Keywords:    []string{"是谁", "什么是", "介绍下", "百科", "谁是", "科普", "简历"},
	// 	Examples: []string{
	// 		"埃隆马斯克是谁", "介绍下雷军", "什么是量子力学", "英雄联盟游戏介绍",
	// 		"科普下高原反应", "如何修理汽车爆胎", "西藏的民俗习惯", "谁是世界首富",
	// 		"介绍下LOL", "马化腾的简历", "这款车的设计理念", "什么是自动驾驶",
	// 		"介绍一下这个名人", "帮我搜搜这个事",
	// 	},
	// 	IsSaveQdrant: false,
	// },
	{
		Category:    "INTENT_CHAT", // 【日常/问候/指令】
		Description: "闲聊、打招呼、指令（如闭嘴、安静）。",
		Keywords:    []string{"你好", "聊天", "闭嘴", "大声", "笑话", "累"},
		Examples: []string{
			"你好呀", "讲个笑话吧", "我累了休息下", "陪我聊聊天", "你是谁",
			"你真厉害", "今天天气不错", "闭嘴停止说话", "音量调大一点", "换个话题",
			"哈喽哈喽", "没事逗逗你", "你会做什么",
		},
		IsSaveQdrant: false,
	},
}

var globalIntentVectors []*IntentVector
var project = "Trip"

func (d *AIDbx) GetIntentLibrary() []*IntentVector {
	t := log.Time()
	defer t.TimeEnd("GetIntentLibrary")

	if len(globalIntentVectors) > 0 {
		return globalIntentVectors
	}

	count := 0
	for _, t := range templates {
		iv := IntentVector{Category: t.Category}
		for _, ex := range t.Examples {
			// 调用你的 GetEmbedding 函数
			v, err := d.GetEmbedding(TextTypeDocument, ex)
			if err != nil {
				log.Error(err)
				return nil
			}
			count++
			log.Info(count)

			iv.Vectors = append(iv.Vectors, &IntentVectorsItem{
				Example: ex,
				Vector:  v,
			})
		}
		globalIntentVectors = append(globalIntentVectors, &iv)
	}

	return globalIntentVectors
}

func (d *AIDbx) InitIntentLibraryToQdrant() error {
	t := log.Time()
	defer t.TimeEnd("InitIntentLibraryToQdrant")

	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()

	// if err := d.DeleteEmptySourceIntents(ctx); err != nil {
	// 	log.Error(err)
	// 	return err
	// }

	maxConcurrent := 10
	semaphore := make(chan struct{}, maxConcurrent)
	var wg sync.WaitGroup

	for _, t := range templates {
		// iv := IntentVector{Category: t.Category}
		for _, ex := range t.Examples {

			wg.Add(1)
			go func() {

				defer wg.Done()
				semaphore <- struct{}{}
				defer func() { <-semaphore }()

				// 2. 调用 gRPC 客户端执行写入
				err := d.SaveIntentTemplates(ctx, project, ex, t.Category, "")
				if err != nil {
					log.Error(err)
					return
				}

				// log.Info("保存成功", project, ex, t.Category)

			}()
		}
		// globalIntentVectors = append(globalIntentVectors, &iv)
	}

	wg.Wait()

	return nil
}

func (d *AIDbx) DeleteEmptySourceIntents(ctx context.Context) error {
	// 构建删除请求
	// 构建删除请求
	req := &qdrant.DeletePoints{
		CollectionName: conf.QdrantCollectionName.IntentTemplates,
		Points: &qdrant.PointsSelector{
			PointsSelectorOneOf: &qdrant.PointsSelector_Filter{
				Filter: &qdrant.Filter{
					// 使用 Should 表达 (source == "") OR (source is null)
					Should: []*qdrant.Condition{
						{
							// 条件 1：字段为空字符串
							ConditionOneOf: &qdrant.Condition_Field{
								Field: &qdrant.FieldCondition{
									Key: "source",
									Match: &qdrant.Match{
										MatchValue: &qdrant.Match_Text{
											Text: "",
										},
									},
								},
							},
						},
						{
							// 条件 2：字段不存在 (Is Empty)
							ConditionOneOf: &qdrant.Condition_IsEmpty{
								IsEmpty: &qdrant.IsEmptyCondition{
									Key: "source",
								},
							},
						},
					},
				},
			},
		},
	}

	_, err := conf.Qdrant.PointsClient.Delete(ctx, req)
	if err != nil {
		log.Error("Delete points with empty source failed: %v", err)
		return err
	}

	log.Info("Successfully deleted intent templates with empty source.")
	return nil
}

func (d *AIDbx) SaveIntentTemplates(ctx context.Context, project, example, category, source string) error {

	vector, err := d.GetEmbedding(TextTypeDocument, example)
	if err != nil {
		log.Error(err)
		return err
	}

	pointId := d.GeneratePointID(
		fmt.Sprintf("%s::%s::%s", project, example, category))

	res, err := conf.Qdrant.PointsClient.Get(ctx, &qdrant.GetPoints{
		CollectionName: conf.QdrantCollectionName.IntentTemplates,
		Ids: []*qdrant.PointId{
			{
				PointIdOptions: &qdrant.PointId_Uuid{Uuid: pointId},
			},
		},
		// 我们只需要知道是否存在，不需要取回 Payload 和 Vector，节省带宽
		WithPayload: &qdrant.WithPayloadSelector{SelectorOptions: &qdrant.WithPayloadSelector_Enable{Enable: false}},
		WithVectors: &qdrant.WithVectorsSelector{SelectorOptions: &qdrant.WithVectorsSelector_Enable{Enable: false}},
	})
	if err != nil {
		log.Info(fmt.Errorf("check point failed: %w", err))
		return err
	}

	if len(res.GetResult()) > 0 {
		return nil
	}

	waitTrue := true
	req := &qdrant.UpsertPoints{
		CollectionName: conf.QdrantCollectionName.IntentTemplates,
		// 设为 true 确保写入后立即可以被检索，适合灌数场景
		Wait: &waitTrue,
		Points: []*qdrant.PointStruct{
			{
				Id: &qdrant.PointId{
					PointIdOptions: &qdrant.PointId_Uuid{
						Uuid: pointId,
					},
				},
				Vectors: &qdrant.Vectors{
					VectorsOptions: &qdrant.Vectors_Vector{
						Vector: &qdrant.Vector{
							Data: vector,
						},
					},
				},
				Payload: map[string]*qdrant.Value{
					"project": {Kind: &qdrant.Value_StringValue{
						StringValue: project,
					}},
					"category": {Kind: &qdrant.Value_StringValue{
						StringValue: category,
					}},
					"example": {Kind: &qdrant.Value_StringValue{
						StringValue: example,
					}},
					"source": {Kind: &qdrant.Value_StringValue{
						StringValue: source,
					}},
				},
			},
		},
	}

	_, err = conf.Qdrant.PointsClient.Upsert(ctx, req)
	if err != nil {
		log.Error(err)
		return err
	}

	return nil
}

// func (d *AIDbx) ArbitrateIntent(userInput string, results []QueryResult) string {
// 	if len(results) == 0 {
// 		return "INTENT_CHAT"
// 	}

// 	if results[0].Example == userInput {
// 		return results[0].Category
// 	}

// 	if len(results) < 2 {
// 		return "INTENT_CHAT"
// 	}

// 	top1 := results[0]
// 	top2 := results[1]
// 	top3 := results[2]

// 	// --- 1. 同类合并逻辑 (解决你遇到的“你好”问题) ---
// 	// 如果前两名（或前三名）分类完全一致，直接返回该分类。
// 	// 无论分数是否全等，只要它们达成了共识，就不需要 LLM 仲裁。
// 	if top1.Category == top2.Category {
// 		// 只有在分数极低的情况下才怀疑
// 		if top1.Score > 0.85 {
// 			return top1.Category
// 		}
// 	}

// 	// --- 2. 分数全等且分类冲突判定 ---
// 	// 只有当分数全等，但【分类不同】时，才认为是真正的检索失效
// 	if top1.Score == top2.Score && top1.Category != top2.Category {
// 		log.Warn("Qdrant 检索异常：分类冲突且分数全等，进入 LLM 仲裁")
// 		return d.CallLLMArbitrator(userInput)
// 	}

// 	// --- 3. 梯度差判定 ---
// 	gap12 := top1.Score - top2.Score
// 	gap23 := top2.Score - top3.Score

// 	// A. 绝对领先
// 	log.Info(top1.Score > 0.95 && gap12 > 0.04)
// 	if top1.Score > 0.95 && gap12 > 0.04 {
// 		return top1.Category
// 	}

// 	// B. 孤立误报检测 (马斯克防护)
// 	// 如果 Top2 和 Top3 分类相同，且与 Top1 不同，且它们之间咬得很紧
// 	// 说明 Top1 可能是撞词误报，走 LLM 裁决
// 	if top2.Category == top3.Category && top1.Category != top2.Category && gap23 < 0.005 {
// 		return d.CallLLMArbitrator(userInput)
// 	}

// 	// C. 模糊区判定
// 	// 分差太小且分类不同，说明模型在纠结
// 	if gap12 < 0.02 {
// 		log.Info("意图纠缠 [Gap:%.4f]，正在唤醒 LLM...", gap12)
// 		return d.CallLLMArbitrator(userInput)
// 	}

//		// 最终兜底
//		if top1.Score < 0.95 {
//			return d.CallLLMArbitrator(userInput)
//		}
//		return top1.Category
//	}
func (d *AIDbx) ArbitrateIntent(userInput string, results []QueryResult) string {
	count := len(results)
	if count == 0 {
		return "INTENT_CHAT"
	}

	// 1. 完全匹配直通车
	if results[0].Example == userInput {
		return results[0].Category
	}

	// 2. 数量不足判定：如果只有一条结果且分数不高，直接交给 LLM 或兜底
	if count < 2 {
		if results[0].Score > 0.95 {
			return results[0].Category
		}
		return d.CallLLMArbitrator(userInput)
	}

	// 准备前三名数据，注意越界保护
	top1 := results[0]
	top2 := results[1]
	var top3 *QueryResult
	if count >= 3 {
		top3 = &results[2]
	}

	// 计算分差（梯度）
	gap12 := top1.Score - top2.Score

	// --- 策略 A: 同类共识 (High Confidence Consensus) ---
	// 如果前两名分类一致且分数及格，直接采纳，不唤醒 LLM
	if top1.Category == top2.Category {
		if top1.Score > 0.85 {
			return top1.Category
		}
	}

	// --- 策略 B: 极端冲突判定 ---
	// 分数完全一样但分类不同，或者分差极其微小（纠缠区）
	if gap12 < 0.001 && top1.Category != top2.Category {
		log.Warn("意图严重纠缠，唤醒 LLM 仲裁", "gap", gap12)
		return d.CallLLMArbitrator(userInput)
	}

	// --- 策略 C: 孤立误报检测 (防止撞词) ---
	// 如果 Top1 孤立无援，而 Top2/Top3 结盟反击，说明 Top1 可能是偏僻词撞词
	if top3 != nil {
		gap23 := top2.Score - top3.Score
		if top1.Category != top2.Category && top2.Category == top3.Category {
			// 如果 2 和 3 咬得极紧，说明它们更有可能是真实意图
			if gap23 < 0.005 && gap12 < 0.05 {
				return d.CallLLMArbitrator(userInput)
			}
		}
	}

	// --- 策略 D: 绝对领先判定 ---
	// 只有分差足够大，且得分够高，才敢说不需要 LLM
	if top1.Score > 0.96 && gap12 > 0.05 {
		return top1.Category
	}

	// --- 策略 E: 模糊区兜底 ---
	// 剩下的所有情况（如：分数都在 0.8-0.9 之间浮动，或者分差太小）
	// 说明检索结果不够权威，统一走 LLM
	log.Info("进入模糊区判定", "top1Score", top1.Score, "gap12", gap12)
	return d.CallLLMArbitrator(userInput)
}

func (d *AIDbx) CallLLMArbitrator(input string) string {

	log.Info("CallLLMArbitrator", input)

	val, err := conf.GlobalFsDB.Get("CallLLMArbitrator:" + input)

	log.Error("cate", val, err)
	if err == nil {
		cateAny := val.Value()
		cate := cateAny.(string)
		log.Error("CallLLMArbitrator fsdb cate", cate, cate != "")

		if cate != "" {
			return cate
		}
	}

	// return "INTENT_CHAT"
	userInput := strings.TrimSpace(strings.ToLower(input))

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// 调用你的 LLM 接口
	var categoryDescriptions []string
	validMap := make(map[string]*IntentTemplate)
	for i, t := range templates {
		// 格式化为：1. INTENT_NAME: 描述内容
		desc := fmt.Sprintf("%d. %s: %s", i+1, t.Category, t.Description)
		categoryDescriptions = append(categoryDescriptions, desc)
		validMap[t.Category] = &t
	}

	// 将数组组合成字符串，用换行分隔
	intentGuide := strings.Join(categoryDescriptions, "\n")

	// --- 构建最终的 System Content ---
	systemContent := fmt.Sprintf(`你是一个自驾助手的意图路由专家。请分析用户的输入，将其归类为以下几种之一：

%s

请直接返回分类代码（如 INTENT_STATS），不要任何解释。
禁止讨论政治、种族、人权等敏感话题。
`, intentGuide)

	messages := []openai.ChatCompletionMessage{
		{
			Role:    openai.ChatMessageRoleSystem,
			Content: systemContent,
		},
		{
			Role:    openai.ChatMessageRoleUser,
			Content: userInput,
		},
	}

	resp, err := conf.OpenAIClient.CreateChatCompletion(ctx, openai.ChatCompletionRequest{
		// Model:       conf.OpenAIModel,
		Model: "groq-llama-3.1-8b",
		// Model: "glm-4.5-flash",
		// Model:       "glm-4-flash",
		Messages:    messages,
		MaxTokens:   500,
		Temperature: 0, // 稍高一点，让 AI 更有灵性
		// Stop:        []string{"\n", "分析", "步骤"},
	})

	// 5. 错误处理与 Fallback
	if err != nil {
		log.Error(err)
		return "INTENT_CHAT"
	}

	log.Info(messages)
	// log.Info(resp.Choices[0].FinishReason)
	// log.Info(resp.Choices[0].Message.Content)
	cate := strings.TrimSpace(resp.Choices[0].Message.Content)
	// log.Info(resp.Model)

	log.Error("CallLLMArbitrator", resp.Model, resp.Usage.TotalTokens, cate)

	if validMap[cate] == nil {
		log.Warn("LLM 返回了非预期的分类 [%s]，已强制降级为 INTENT_CHAT", cate)
		return "INTENT_CHAT"
	}

	// 将统计和geo的查询结果,存储进qdrant

	if validMap[cate].IsSaveQdrant {
		err := d.SaveIntentTemplates(ctx, project, userInput, cate, "LLM")
		if err != nil {
			log.Error(err)
		}
	}

	if err := conf.GlobalFsDB.Set("CallLLMArbitrator:"+input, cate, 7*24*time.Hour); err != nil {
		log.Error(err)
	}
	return cate
}

type QueryResult struct {
	Category string
	Example  string
	Score    float32
}

// SearchIntent 通过向量匹配获取最接近的前 3 条意图
func (d *AIDbx) SearchIntent(ctx context.Context, userVector []float32) ([]QueryResult, error) {
	// 1. 构造查询请求
	project := "Trip"
	searchReq := &qdrant.SearchPoints{
		CollectionName: conf.QdrantCollectionName.IntentTemplates,
		Vector:         userVector,
		Limit:          3, // 获取前 3 名进行投票仲裁
		// 关键：增加项目过滤，确保多租户安全
		Filter: &qdrant.Filter{
			Must: []*qdrant.Condition{
				{
					ConditionOneOf: &qdrant.Condition_Field{
						Field: &qdrant.FieldCondition{
							Key: "project",
							Match: &qdrant.Match{
								MatchValue: &qdrant.Match_Keyword{
									Keyword: project,
								},
							},
						},
					},
				},
			},
		},
		// 必须开启 Payload 返回，否则拿不到 Category
		WithPayload: &qdrant.WithPayloadSelector{
			SelectorOptions: &qdrant.WithPayloadSelector_Enable{
				Enable: true,
			},
		},
	}

	// 2. 执行 gRPC 查询
	res, err := conf.Qdrant.PointsClient.Search(ctx, searchReq)
	if err != nil {
		return nil, fmt.Errorf("qdrant search failed: %v", err)
	}

	// 3. 解析结果
	var finalResults []QueryResult
	for _, hit := range res.GetResult() {
		payload := hit.GetPayload()
		finalResults = append(finalResults, QueryResult{
			Category: payload["category"].GetStringValue(),
			Example:  payload["example"].GetStringValue(),
			Score:    hit.GetScore(),
		})
	}

	return finalResults, nil
}

func (d *AIDbx) CleanseInput(userInput string) string {
	// 2. 使用 Segment 方法 (接收 []byte)
	// 这个方法返回的是 []gse.SegmentRes 结构体切片
	segments := conf.Seg.Segment([]byte(userInput))

	// log.Info("--- 意图去噪处理 ---")

	var cleanWords []string

	for _, s := range segments {
		// 获取词语文本
		text := s.Token().Text()
		// 获取词性 (Pos)
		pos := s.Token().Pos()

		// log.Info("词: %s \t 词性: %s\n", text, pos)

		// 3. 核心过滤逻辑：
		// r: 代词 (我, 咱们)
		// y: 语气词 (呀, 呢, 吗)
		// x: 标点/非语素
		// u: 助词 (了, 的)
		if pos == "r" || pos == "y" || pos == "x" || pos == "u" || pos == "w" {
			continue
		}

		cleanWords = append(cleanWords, text)
	}

	// 4. 最终生成的提纯文本
	finalQuery := ""
	for _, w := range cleanWords {
		finalQuery += w
	}

	// log.Info("\n原始输入: %s\n", userInput)
	log.Info("送入Qdrant的文本: ", finalQuery)

	return nstrings.StringOr(finalQuery, userInput)
}

func (d *AIDbx) GetSessionId() string {
	return nshortid.GetShortId(18)
}
func (d *AIDbx) SaveAIMessages(sessionId, messageId, question, answer string, limit int) error {
	if sessionId == "" || question == "" || answer == "" {
		return nil
	}
	contexts, err := d.GetAIMessages(sessionId)
	if err != nil && err.Error() != "value does not exist" {
		return err
	}

	contexts = append(narrays.Filter(contexts, func(v *protos.ChatContextItem, i int) bool {
		return v.Id != messageId
	}), &protos.ChatContextItem{
		Id:       messageId,
		Question: question,
		Answer:   answer,
	})

	if len(contexts) > limit {
		// 这种写法会直接截掉开头的元素
		contexts = contexts[len(contexts)-limit:]
	}

	return conf.AISessionFsDB.Set(sessionId, contexts, 7*24*time.Hour)

}
func (d *AIDbx) GetAIMessages(sessionId string) ([]*protos.ChatContextItem, error) {
	val, err := conf.AISessionFsDB.Get(sessionId)
	if err != nil {
		return nil, err
	}
	return val.Value(), nil
}

type TripMemoryItem struct {
	VisitTime    string  `json:"time,omitempty"`
	Location     string  `json:"loc,omitempty"`
	Summary      string  `json:"summary,omitempty"`
	DistanceM    float64 `json:"dist_m,omitempty"`
	AltitudeM    float64 `json:"altitude_m,omitempty"`
	MaxSpeedKMH  float64 `json:"max_speed_kmh,omitempty"`
	Weather      string  `json:"weather,omitempty"`
	TemperatureC float64 `json:"temp_c,omitempty"`
	Road         string  `json:"road,omitempty"`
}

type POIsItem struct {
	Name        string  `json:"name"`
	Address     string  `json:"address,omitempty"`
	WikiSummary string  `json:"wiki,omitempty"`
	DistanceM   float64 `json:"dist_m,omitempty"`
	Direction   string  `json:"direction,omitempty"`
	BearingDeg  float64 `json:"bearing_deg"`
}

type WeatherItem struct {
	// 当前天气
	NowCond  string  `json:"now_cond"` // 现象：晴/大雨/浓雾/雪
	NowTempC float64 `json:"now_c"`    // 当前温度
	// NowVisM  float64 `json:"now_vis"`  // 当前能见度(米)，安全关键指标

	// 未来2小时趋势
	F1hCond  string  `json:"f1h_cond"` // 2小时后现象
	F1hTempC float64 `json:"f1h_c"`    // 2小时后温度
	// F2hVisM  float64 `json:"f2h_vis"`  // 2小时后能见度

	// 预警标签 (由后端逻辑直接给出，降低 AI 推理压力)
	// AlertTag string `json:"alert"` // 如：大风预警/道路结冰/无
}

type ContextDataItem struct {
	Type string `json:"type"` // 如: "POIs", "Memory"
	Data any    `json:"data"` // 存储序列化后的 POIsItem 或 TripMemoryItem 数组
}

// 对应 Go 后端的映射逻辑
var POICategoryToKindMap = map[string][]string{
	"自然景观": {
		"peak", "water", "glacier", "island", "islet",
		"lake", "spring", "waterfall", "wetland", "wood", "bare_rock",
	},
	"户外探索": {
		"saddle", "valley", "ridge", "cliff", "cave_entrance",
		"path", "track", "volcano", "hot_spring",
	},
	"自驾配套": {
		"fuel", "charging_station", "parking", "services",
		"rest_area", "drinking_water", "emergency", "repair",
	},
	"人文古迹": {
		"place_of_worship", "memorial", "ruins", "monument",
		"archaeological_site", "citywalls", "city_gate", "castle",
		"tomb", "fort", "heritage", "temple",
	},
	"旅游景点": {
		"park", "attraction", "museum", "viewpoint", "theme_park",
		"zoo", "beach", "artwork", "exhibition_centre", "theatre", "square",
	},
	"城镇设施": {
		"residential", "commercial", "university", "police",
		"townhall", "courthouse", "school", "college",
		"hospital", "bank", "community_centre", "kindergarten",
	},
	"餐饮住宿": {
		"camp_site", "hotel", "guest_house", "chalet",
		"resort", "restaurant", "pub", "caravan_site", "hostel",
	},
	"道路交通": {
		"trunk", "primary", "secondary", "motorway",
		"trunk_link", "motorway_link", "motorway_junction",
		"bridge", "ferry_terminal", "subway", "stop", "bus_station",
	},
}

var (
	GET_TRIP_STATISTICS = "get_trip_statistics"
	SEARCH_PLACE_INFO   = "search_place_info"
	GET_FULL_WEATHER    = "get_full_weather"
	GET_APP_MANIFEST    = "get_app_manifest"
	SEARCH_NEARBY_POIS  = "search_nearby_pois"
	QUERY_TRIP_MEMORY   = "query_trip_memory"
)

func (fc *AIDbx) GetAgentTripTools(filterFunc []string) (tools []openai.Tool) {
	// id := methods.GetAIToolsParameterStr(
	// 	"id", "string", "行程的唯一识别ID")
	tripType := `"tripType": {
  "type": "string",
  "enum": [
    "All",
    "Running",
    "Bike",
    "Drive",
    "Motorcycle",
    "Walking",
    "PowerWalking",
    "Train",
    "PublicTransport",
    "Plane"
  ],
  "description": "出行类别。必须从列表中选择：All(全部), Running(跑步), Bike(骑行), Drive(驾驶), Motorcycle(摩托车), Walking(步行), PowerWalking(健走), Train(火车), PublicTransport(公交), Plane(飞机)。"
}`
	startTime := methods.GetAIToolsParameterStr(
		"startTime", "string", "查询起始日期，格式必须为 YYYY-MM-DD (例如: 2022-04-11), 不限制留空")
	endTime := methods.GetAIToolsParameterStr(
		"endTime", "string", "查询结束日期，格式必须为 YYYY-MM-DD (例如: 2026-04-19), 不限制留空")
	maxDistance := methods.GetAIToolsParameterStr(
		"maxDistance", "integer", "单次行程筛选的距离上限 (单位米), 不限制则设为0")
	minDistance := methods.GetAIToolsParameterStr(
		"minDistance", "integer", "单次行程筛选的距离下限 (单位米), 不限制则设为0")

	poiCategoryEnum := make([]string, 0, len(POICategoryToKindMap))
	for k := range POICategoryToKindMap {
		poiCategoryEnum = append(poiCategoryEnum, fmt.Sprintf("\"%s\"", k)) // 加上双引号符合 JSON 规范
	}

	tools = []openai.Tool{
		{
			Type: openai.ToolTypeFunction,
			Function: &openai.FunctionDefinition{
				Name:        GET_TRIP_STATISTICS,
				Description: "获取用户行程的历史统计数据（如里程、时长等）。支持按运动类型、时间范围及单次行程的距离区间进行筛选。",
				Parameters: json.RawMessage(`{
                "type": "object",
                "properties": {
                    ` + tripType + `,
                    ` + startTime + `,
                    ` + endTime + `,
                    ` + maxDistance + `,
                    ` + minDistance + `
                },
                "required": ["tripType"]
            }`),
			},
		},
		{
			Type: openai.ToolTypeFunction,
			Function: &openai.FunctionDefinition{
				Name:        SEARCH_PLACE_INFO,
				Description: "综合获取目标地点的深度百科、实时天气及周边关键POIs。当用户询问不在当前视野内或非本地地点时使用。",
				Parameters: json.RawMessage(`{
                "type": "object",
                "properties": { 
								    "query": {
											"type": "string",
											"description": "具体的地点名称，如 '洪崖洞'、'荣昌区' 或 '缙云山'"
										 },
										"wiki_level": {
											"type": "string",
											"enum": ["summary", "deep"],
											"description": "Wiki内容深度：summary提供简述，deep提供地理成因及历史深度背景"
										},
										"include_weather": {
												"type": "boolean",
												"description": "是否包含实时天气及未来1小时预报，默认true"
										},
										"include_pois": {
												"type": "boolean",
												"description": "是否包含周边核心自驾配套POI，默认true"
										}
                },
                "required": ["query"]
            }`),
			},
		},
		{
			Type: openai.ToolTypeFunction,
			Function: &openai.FunctionDefinition{
				Name:        GET_FULL_WEATHER,
				Description: "获取目标地点的专业气象报告。包含小时级降水预报、风速、湿度、7天长期趋势、极端天气预警等。适用于用户计划行程、关心天气细节或恶劣天气驾驶决策时。",
				Parameters: json.RawMessage(`{
            "type": "object",
						"properties": { 
                "query": {
                    "type": "string",
                    "description": "地点名称，如 '重庆市荣昌区'、'洪崖洞'。当用户明确提到地名时使用。"
                },
                "latitude": {
                    "type": "number",
                    "description": "纬度。当用户询问 '当前位置'、'这里' 或提供具体坐标时使用。"
                },
                "longitude": {
                    "type": "number",
                    "description": "经度。同纬度配合使用。"
                }
            },
            "required": []
        }`),
			},
		},
		{
			Type: openai.ToolTypeFunction,
			Function: &openai.FunctionDefinition{
				Name:        GET_APP_MANIFEST,
				Description: "检索 App 内各项功能的官方详细说明文档、操作指南及 UI 组件元数据。当用户询问‘有什么功能’、‘怎么用’或需要调用特定模块（如气象、路书、设置）时使用。",
				Parameters: json.RawMessage(`{
						"type": "object",
            "properties": { 
								"query": {
                    "type": "string",
                    "description": "用户的搜索关键词或原始需求，例如 '天气'、'路书怎么用'。留空则表示获取全量清单。"
                 }
            },
            "required": []
        }`),
			},
		},
		{
			Type: openai.ToolTypeFunction,
			Function: &openai.FunctionDefinition{
				Name:        SEARCH_NEARBY_POIS,
				Description: "根据类别（如景区、江河、山峰、垭口、加油站、服务区等）搜索当前位置周边的兴趣点。返回列表包含名称、距离、介绍、地址。",
				Parameters: json.RawMessage(`{
            "type": "object",
            "properties": {
                "name": {
                    "type": "string",
                    "description": "POI 名称（模糊匹配）"
                },
                "category": {
                    "type": "string",
										"enum": [` + strings.Join(poiCategoryEnum, ", ") + `],
                    "description": "搜索类别，如：'自然风景'、'人文古迹'、'火锅'、'停车场'、'加油站'"
                },
                "address": {
                    "type": "string",
                    "description": "搜索目标位置完整地址"
                },
                "latitude": {
                    "type": "number",
                    "description": "纬度。当用户询问 '当前位置'、'这里' 或提供具体坐标时使用。"
                },
                "longitude": {
                    "type": "number",
                    "description": "经度。同纬度配合使用。"
                },
                "radius_m": {
                    "type": "integer",
                    "description": "搜索半径（米），默认10000，最大50000"
                },
                "sort_by": {
                    "type": "string",
                    "enum": ["default ","distance", "importance"],
                    "description": "排序逻辑：default 默认排序， distance按距离，importance按旅游相关度"
                },
                "limit": {
                    "type": "integer",
                    "description": "默认10，最多不能超过20"
                }
            },
            "required": []
        }`),
			},
		},
		{
			Type: openai.ToolTypeFunction,
			Function: &openai.FunctionDefinition{
				Name:        QUERY_TRIP_MEMORY,
				Description: "检索用户过去行程中的记忆切片（RGA-TripMemory）。用于回答关于历史行程的海拔、感受、路线回溯等问题。",
				Parameters: json.RawMessage(`{
            "type": "object",
            "properties": {
                "query": {
                    "type": "string",
                    "description": "语义向量搜索关键词，如 '高海拔'、'看雪'、'上次进藏'、'那个好吃的路边摊'"
                },
                "start_date": {
                    "type": "string",
                    "description": "（可选）开始日期筛选，格式 YYYY-MM-DD"
                },
                "end_date": {
                    "type": "string",
                    "description": "（可选）结束日期筛选，格式 YYYY-MM-DD"
                },
                "min_altitude": {
                    "type": "number",
                    "description": "（可选）最低海拔筛选，如 3000"
                },
                "max_altitude": {
                    "type": "number",
                    "description": "（可选）最高海拔筛选，如 3000"
                },
								"start_hour": {
										"type": "integer",
										"minimum": 0,
										"maximum": 23,
										"description": "起始小时(0-23)。请将用户的语义描述转换为数字：'清晨'约5-8, '中午'约12-14, '傍晚'约18-20, '深夜'约22-4。"
								},
								"end_hour": {
										"type": "integer",
										"minimum": 0,
										"maximum": 23,
										"description": "结束小时(0-23)。若与start_hour相同则代表全天。支持跨零点查询（如23到2）。"
								},
                "address": {
                    "type": "string",
                    "description": "搜索目标位置完整地址"
                },
                "latitude": {
                    "type": "number",
                    "description": "纬度。当用户询问 '当前位置'、'这里' 或提供具体坐标时使用。"
                },
                "longitude": {
                    "type": "number",
                    "description": "经度。同纬度配合使用。"
                },
                "radius_m": {
                    "type": "integer",
                    "description": "搜索半径（米），默认5000，最大50000"
                }
            },
            "required": []
        }`),
			},
		},
	}

	return narrays.Filter(tools, func(value openai.Tool, index int) bool {
		return narrays.Includes(filterFunc, value.Function.Name)
	})

}

func (fc *AIDbx) GetAgentTools() (tools []openai.Tool) {

	id := methods.GetAIToolsParameterStr(
		"id", "string", "路书的唯一识别ID")
	title := methods.GetAIToolsParameterStr(
		"title", "string", "新的路书标题内容 (30字以内)")
	desc := methods.GetAIToolsParameterStr(
		"desc", "string", "路书描述介绍 (150字以内)")
	startTime := methods.GetAIToolsParameterStr(
		"startTime", "string", "计划的出发时间，(仅需日期，如：2026-04-11)")

	tools = []openai.Tool{
		{
			Type: openai.ToolTypeFunction,
			Function: &openai.FunctionDefinition{
				Name:        "get_roadbook_detail",
				Description: "获取路书的完整详情，包括标题、描述、出发时间和途径点列表",
				Parameters: json.RawMessage(`{
                "type": "object",
                "properties": {
                    ` + id + `
                },
                "required": ["id"]
            }`),
			},
		},
		{
			Type: openai.ToolTypeFunction,
			Function: &openai.FunctionDefinition{
				Name:        "update_roadbook_detail",
				Description: "修改指定路书的标题、介绍、出发时间、时间线（时间线需要全量更新）",
				Parameters: json.RawMessage(`{
                "type": "object",
                "properties": {
                    ` + id + `,
                    ` + title + `,
                    ` + desc + `,
                    ` + startTime + `,
										"timelines": {
											"type": "array",
											"description": "路书的行程时间线数组（每个时间线还包含了本时间线的途径点数组）",
											"items": {
												"type": "object",
												"properties": {
														"id": { "type": "string", "description": "如果没有旧 ID，请务必不要填写此字段或传空字符串" },
														"title": { "type": "string", "description": "此时间线的标题 (最多30字)" },
														"desc": { "type": "string", "description": "此时间线的介绍 (最多60字)" },
														"days": { "type": "integer", "description": "此时间线的持续时间，指在这个时间线区间内，停留持续多少天 (正整数，单位天)" },
														"waypoints": {
																"type": "array",
																"description": "此时间线的途径点列表",
																"items": {
																		"type": "object",
																		"properties": {
														            "id": { "type": "string", "description": "如果没有旧 ID，请务必不要填写此字段或传空字符串" },
																				"name": { "type": "string", "description": "途径点名称 (如 九寨沟景区、重庆市政府、南滨路等)" },
																				"address": { "type": "string", "description": "途径点的完整地址 （包含省市区县街道镇、用于查询经纬度信息）" }
																		},
																		"required": ["name", "address"]
																}
														},
														"required": ["desc", "days", "waypoints"]
												}
											}
			              }
                },
                "required": ["id"]
            }`),
			},
		},
		{
			Type: openai.ToolTypeFunction,
			Function: &openai.FunctionDefinition{
				Name:        "create_roadbook",
				Description: "创建一个新的自驾路书，这是唯一创建函数 (如用户未指定标题、介绍、开始时间，你可以帮忙填写)",
				Parameters: json.RawMessage(`{
                "type": "object",
                "properties": {
                    ` + id + `,
                    ` + title + `,
                    ` + desc + `,
                    ` + startTime + `
                },
                "required": ["title","desc","startTime"]
            }`),
			},
		},
		// {
		// 	Type: openai.ToolTypeFunction,
		// 	Function: &openai.FunctionDefinition{
		// 		Name:        "get_roadbook_timelines",
		// 		Description: `获取指定路书的完整时间线和途径点列表。`,
		// 		Parameters: json.RawMessage(`{
		//             "type": "object",
		//             "properties": {
		//                 ` + id + `
		//             },
		//             "required": ["id"]
		//         }`),
		// 	},
		// },
		// 		{
		// 			Type: openai.ToolTypeFunction,
		// 			Function: &openai.FunctionDefinition{
		// 				Name: "update_roadbook_timelines",
		// 				Description: `这是增删查改时间线的唯一接口。
		// 全量同步指定路书的完整时间线和途径点列表。
		// 无论是增加、删除、修改还是调整顺序，
		// 请根据最新对话结果重新组织完整的时间线和途径点列表发送过来。
		// 必须包含该路书当前所有的内容，除非用户明确要求删除某个部分。
		// 禁止为了节省长度而截断输出。`,
		// 				Parameters: json.RawMessage(`{
		//                 "type": "object",
		//                 "properties": {
		//                     ` + id + `,
		// 										"timelines": {
		// 											"type": "array",
		// 											"description": "路书的行程时间线数组",
		// 											"items": {
		// 													"type": "object",
		// 													"properties": {
		// 															"id": { "type": "string", "description": "如果没有旧 ID，请务必不要填写此字段或传空字符串" },
		// 															"title": { "type": "string", "description": "此时间线的标题 (最多30字)" },
		// 															"desc": { "type": "string", "description": "此时间线的介绍 (最多60字)" },
		// 															"days": { "type": "integer", "description": "此时间线的持续时间，指在这个时间线区间内，停留持续多少天 (正整数，单位天)" },
		// 															"waypoints": {
		// 																	"type": "array",
		// 																	"description": "此时间线的途径点列表",
		// 																	"items": {
		// 																			"type": "object",
		// 																			"properties": {
		// 															            "id": { "type": "string", "description": "如果没有旧 ID，请务必不要填写此字段或传空字符串" },
		// 																					"name": { "type": "string", "description": "途径点名称 (如 九寨沟景区、重庆市政府、南滨路等)" },
		// 																					"address": { "type": "string", "description": "途径点的完整地址 （包含省市区县街道镇、用于查询经纬度信息）" }
		// 																			},
		// 																			"required": ["name", "address"]
		// 																	}
		// 															},
		// 															"required": ["desc", "days", "waypoints"]
		// 													}
		// 											}
		// 			              }
		//                 },
		//                 "required": ["id","timelines"]
		//             }`),
		// 			},
		// 		},
	}

	return tools

}

func (fc *AIDbx) CallAgentTools(c *gin.Context, flusher http.Flusher, toolCall openai.ToolCall, authorId string) *protos.AIResponse_Meta {

	// log.Info("callAgentTools", toolCall.Function.Name, toolCall.Function.Arguments)
	meta := &protos.AIResponse_Meta{
		Action:     toolCall.Function.Name,
		Type:       "function",
		Status:     "calling",
		CreateTime: time.Now().Unix(),
	}

	callFunc := func() *protos.AIResponse_Meta {
		meta.EndTime = time.Now().Unix()

		c.SSEvent("message", protos.Encode(&protos.AIStreamResponse{
			Type:    "meta",
			Action:  toolCall.Function.Name,
			Value:   protos.Encode(meta),
			Content: meta.Status,
		}))

		flusher.Flush()

		return meta
	}

	meta.Status = "calling"
	callFunc()

	fc.CallAgentToolsFunc(c.Request.Context(),
		toolCall, authorId, meta)

	return callFunc()

}

func (fc *AIDbx) TestPOIBearing() {
	log.Info("TestPOIBearing")

	poiPoints, err := poiDbx.SearchPOI(context.Background(), POISearchParams{
		Lat:    ToPtr(29.417632),
		Lon:    ToPtr(105.594949),
		Radius: 5000,
		Limit:  10,
	})
	if err != nil {
		log.Error(err)
		return
	}

	log.Info("poiPoints", poiPoints)
}

func (fc *AIDbx) TestCallAgentTools() {

	ctx, cancel := context.WithTimeout(context.Background(),
		180*time.Second)
	defer cancel()

	// toolCall := openai.ToolCall{
	// 	ID:   "1",
	// 	Type: openai.ToolTypeFunction,
	// 	Function: openai.FunctionCall{
	// 		Name: SEARCH_PLACE_INFO,
	// 		Arguments: `{
	//               "query": "洪崖洞",
	//               "wiki_level": "deep",
	//               "include_weather":true,
	//               "include_pois":true
	//             }`,
	// 	},
	// }
	// toolCall := openai.ToolCall{
	// 	ID:   "1",
	// 	Type: openai.ToolTypeFunction,
	// 	Function: openai.FunctionCall{
	// 		Name: GET_FULL_WEATHER,
	// 		Arguments: `{
	//               "query": "洪崖洞"
	//             }`,
	// 	},
	// }

	toolCalls := []openai.ToolCall{
		// {
		// 	ID:   "1",
		// 	Type: openai.ToolTypeFunction,
		// 	Function: openai.FunctionCall{
		// 		Name: SEARCH_NEARBY_POIS,
		// 		Arguments: `{
		//             "category": "旅游景点",
		// 						"address":"重庆市北碚区",
		// 						"sort_by":"distance",
		// 						"limit":5
		//           }`,
		// 	},
		// },
		{
			ID:   "1",
			Type: openai.ToolTypeFunction,
			Function: openai.FunctionCall{
				Name: QUERY_TRIP_MEMORY,
				Arguments: `{
                "query": "青海之旅",
								"min_altitude":4000,
								"max_altitude":6000,
								"start_hour":21,
								"end_hour":23
              }`,
			},
		},
	}

	for _, toolCall := range toolCalls {
		meta := &protos.AIResponse_Meta{
			Action:     toolCall.Function.Name,
			Type:       "function",
			Status:     "calling",
			CreateTime: time.Now().Unix(),
		}

		// 调用
		fc.CallAgentToolsFunc(ctx,
			toolCall,
			"78L2tkleM", meta)

		j, _ := json.MarshalIndent(meta, "", "  ")
		log.Info("TestCallAgentTools meta", toolCall.Function.Name,
			string(j))
	}
}

func (fc *AIDbx) CallAgentToolsFunc(ctx context.Context,
	toolCall openai.ToolCall,
	authorId string,
	meta *protos.AIResponse_Meta,
) {
	var err error

	log.Info("callAgentTools", toolCall.Function.Name, toolCall.Function.Arguments)

	switch toolCall.Function.Name {

	case QUERY_TRIP_MEMORY:
		type ReqParams struct {
			Query string `json:"query"`

			StartDate   string `json:"start_date"`
			EndDate     string `json:"end_date"`
			MinAltitude int    `json:"min_altitude"`
			MaxAltitude int    `json:"max_altitude"`
			StartHour   int    `json:"start_hour"`
			EndHour     int    `json:"end_hour"`

			Address   string  `json:"address"`
			Latitude  float64 `json:"latitude"`
			Longitude float64 `json:"longitude"`
			RadiusM   int     `json:"radius_m"`
		}

		var args ReqParams

		log.Info("QUERY_TRIP_MEMORY Arguments", toolCall.Function.Arguments)
		if err = json.Unmarshal([]byte(toolCall.Function.Arguments), &args); err != nil {
			meta.Status = "failed"
			meta.Error = err.Error()

			return
		}

		j, _ := json.MarshalIndent(args, "", "  ")
		log.Info("args", string(j))

		poiCategoryEnum := []string{""}
		for k := range POICategoryToKindMap {
			poiCategoryEnum = append(poiCategoryEnum, k) // 加上双引号符合 JSON 规范
		}

		if err = validation.ValidateStruct(
			&args,
			// validation.Parameter(&args.Category, validation.Enum(poiCategoryEnum)),
		); err != nil {
			meta.Status = "failed"
			meta.Error = err.Error()

			return
		}

		params := SearchTripMemoryOptions{
			Text: args.Query,
			Filters: map[string]interface{}{
				"author_id": authorId,
			},
			RadiusMeters: ncommon.IfElse(args.RadiusM > 0,
				float32(args.RadiusM), 5*1000),
			Limit: 10,
			// 注意：空间检索不需要设 Threshold，因为 Score 默认为 0

			StartDate:   args.StartDate,
			EndDate:     args.EndDate,
			MinAltitude: args.MinAltitude,
			MaxAltitude: args.MaxAltitude,
			StartHour:   ToPtr(args.StartHour),
			EndHour:     ToPtr(args.EndHour),
		}

		if args.Latitude != 0 || args.Longitude != 0 {
			params.Coords = &GeoCoords{
				Lat: args.Latitude,
				Lon: args.Longitude,
			}
		}

		if args.Address != "" {
			geoData, err := methods.Geo(args.Address)

			if err != nil {
				meta.Status = "failed"
				meta.Error = err.Error()
				return
			}

			log.Info("geoData", geoData)
			params.Coords = &GeoCoords{
				Lat: geoData.Latitude,
				Lon: geoData.Longitude,
			}
		}

		j, _ = json.MarshalIndent(params, "", "  ")
		log.Info("params", string(j))

		limit := 3

		tripMemory, err := tripMemoryDbx.SearchTripMemory(ctx, params)
		log.Info("tripMemory", len(tripMemory))

		if err != nil {
			meta.Status = "failed"
			meta.Error = err.Error()
			return
		}

		var items []TripMemoryItem
		for i, point := range tripMemory {
			if i >= limit {
				break
			}
			p := point.Payload

			// log.Info(point)

			// 1. 获取嵌套的 location 对象
			locValue, ok := p["location"]
			if !ok || locValue.GetStructValue() == nil {
				continue // 如果没有 location 字段则跳过
			}

			fields := locValue.GetStructValue().GetFields()

			histLon := fields["lon"].GetDoubleValue()
			histLat := fields["lat"].GetDoubleValue()
			distance := float64(0)

			if params.Coords != nil {
				distance = methods.GetGeoDistance(histLat, histLon,
					params.Coords.Lat, params.Coords.Lon)
			}

			roadList := p["road_name"].GetListValue().GetValues()
			roadListNames := make([]string, 0, len(roadList))

			for _, item := range roadList {
				// GetStringValue() 如果类型不对会返回空字符串
				roadListNames = append(roadListNames, item.GetStringValue())
			}
			item := TripMemoryItem{
				// 假设你存入时字段名分别为 time, location, summary

				VisitTime:    time.Unix(int64(p["start_time"].GetIntegerValue()), 0).Format("2006-01-02 15:04"),
				Location:     p["location_name"].GetStringValue(),
				Summary:      p["summary"].GetStringValue(),
				DistanceM:    distance,
				AltitudeM:    p["alt_end"].GetDoubleValue(),
				MaxSpeedKMH:  p["max_speed"].GetDoubleValue(),
				Weather:      p["weather"].GetStringValue(),
				TemperatureC: p["temperature"].GetDoubleValue(),
				Road:         strings.Join(roadListNames, "; "),
			}
			items = append(items, item)
		}

		if len(items) > 0 {
			cdt := ContextDataItem{
				Type: "RGA-TripMemory",
				Data: items,
			}
			results, _ := json.Marshal(&cdt)

			if conf.Config.Server.Mode == "debug" {
				results, _ = json.MarshalIndent(&cdt, "", "  ")
			}
			log.Info("results", string(results))

			meta.Value = string(results)
		} else {
			meta.Value = "未搜索到"
		}

		meta.Status = "success"

		return

	case SEARCH_NEARBY_POIS:
		type ReqParams struct {
			Name      string  `json:"name"`
			Category  string  `json:"category"`
			Address   string  `json:"address"`
			Latitude  float64 `json:"latitude"`
			Longitude float64 `json:"longitude"`
			RadiusM   int     `json:"radius_m"`
			SortBy    string  `json:"sort_by"`
			Limit     int     `json:"limit"`
		}

		var args ReqParams

		log.Info("SEARCH_NEARBY_POIS Arguments", toolCall.Function.Arguments)
		if err = json.Unmarshal([]byte(toolCall.Function.Arguments), &args); err != nil {
			meta.Status = "failed"
			meta.Error = err.Error()

			return
		}

		j, _ := json.MarshalIndent(args, "", "  ")
		log.Info("args", string(j))

		poiCategoryEnum := []string{""}
		for k := range POICategoryToKindMap {
			poiCategoryEnum = append(poiCategoryEnum, k) // 加上双引号符合 JSON 规范
		}

		if err = validation.ValidateStruct(
			&args,
			validation.Parameter(&args.Category, validation.Enum(poiCategoryEnum)),
		); err != nil {
			meta.Status = "failed"
			meta.Error = err.Error()

			return
		}

		// meta.Status = "success"
		// meta.Error = err.Error()

		if args.Limit == 0 || args.Limit > 20 {
			args.Limit = 10
		}

		params := POISearchParams{
			Name:       args.Name,
			Categories: POICategoryToKindMap[args.Category],
			Radius: ncommon.IfElse(args.RadiusM > 0,
				float32(args.RadiusM), 10*1000),
			Limit: uint32(args.Limit),
		}

		if args.Latitude != 0 || args.Longitude != 0 {
			params.Lat = &args.Latitude
			params.Lon = &args.Longitude
		}

		if args.Address != "" {
			geoData, err := methods.Geo(args.Address)

			if err != nil {
				meta.Status = "failed"
				meta.Error = err.Error()
				return
			}

			log.Info("geoData", geoData)
			params.Lat = &geoData.Latitude
			params.Lon = &geoData.Longitude
		}

		// POIs 获取
		// lat := float64(29.56494256724482)
		// lng := float64(106.57476939325748)
		poiPoints, err := poiDbx.SearchPOI(ctx, params)
		if err != nil {
			log.Error(err)
			meta.Status = "failed"
			meta.Error = err.Error()
			return
		}

		log.Info("poiPoints", len(poiPoints))

		if args.SortBy == "distance" {
			sort.Slice(poiPoints, func(i, j int) bool {
				// 处理无效距离（比如 -1 表示无法计算距离的情况）
				if poiPoints[i].POI.Distance < 0 {
					return false
				}
				if poiPoints[j].POI.Distance < 0 {
					return true
				}

				// 从近到远排序
				return poiPoints[i].POI.Distance < poiPoints[j].POI.Distance
			})
		}

		var pois []*POIsItem
		if len(poiPoints) > 0 {
			for _, v := range poiPoints {
				address := strings.Join(narrays.Filter([]string{
					v.POI.Address.State,
					v.POI.Address.Region,
					v.POI.Address.City,
					v.POI.Address.Town,
				}, func(v string, i int) bool {
					return v != ""
				}), "")
				if v.POI != nil && address != "" && len(pois) < args.Limit {
					pois = append(pois, &POIsItem{
						Name:        v.POI.Name,
						Address:     address,
						WikiSummary: v.POI.Wiki.Summary,
						DistanceM:   aiDbx.Round(v.POI.Distance, 1),
						Direction:   v.POI.Direction,
						BearingDeg:  v.POI.Bearing,
					})
				}
			}
		}

		if len(pois) > 0 {
			cdt := ContextDataItem{
				Type: "POIs",
				Data: pois,
			}
			results, _ := json.Marshal(&cdt)

			if conf.Config.Server.Mode == "debug" {
				results, _ = json.MarshalIndent(&cdt, "", "  ")
			}
			log.Info("results", string(results))

			meta.Value = string(results)
		} else {
			meta.Value = "未搜索到"
		}

		meta.Status = "success"

		return

	case GET_APP_MANIFEST:
		type ReqParams struct {
			Query string `json:"query"`
		}

		var args ReqParams

		log.Info("Arguments", toolCall.Function.Arguments)
		if err = json.Unmarshal([]byte(toolCall.Function.Arguments), &args); err != nil {
			meta.Status = "failed"
			meta.Error = err.Error()

			return
		}

		log.Info("args", args)

		if err = validation.ValidateStruct(
			&args,
		); err != nil {
			meta.Status = "failed"
			meta.Error = err.Error()
			return
		}

		appManifest := appDbx.SearchAppManifest(args.Query)

		results, _ := json.Marshal(&ContextDataItem{
			Type: "AppManifest",
			Data: appManifest,
		})
		log.Info("results", string(results))

		meta.Status = "success"
		meta.Value = string(results)

		return

	case GET_TRIP_STATISTICS:
		type ReqParams struct {
			TripType    string `json:"tripType"`
			StartTime   string `json:"startTime"`
			EndTime     string `json:"endTime"`
			MaxDistance int    `json:"maxDistance"`
			MinDistance int    `json:"minDistance"`
		}

		var args ReqParams

		log.Info("Arguments", toolCall.Function.Arguments)
		if err = json.Unmarshal([]byte(toolCall.Function.Arguments), &args); err != nil {
			meta.Status = "failed"
			meta.Error = err.Error()

			return
		}

		log.Info("args", args, args.MinDistance, args.MaxDistance)

		if err = validation.ValidateStruct(
			&args,
			validation.Parameter(&args.TripType, validation.Required(), validation.Enum([]string{
				"All",
				"Running",
				"Bike",
				"Drive",
				"Motorcycle",
				"Walking",
				"PowerWalking",
				"Train",
				"PublicTransport",
				"Plane"})),
		); err != nil {
			meta.Status = "failed"
			meta.Error = err.Error()

			return
		}

		timeLimit := []int64{}

		if args.StartTime != "" && args.EndTime != "" {
			st, _ := time.Parse("2006-01-02", args.StartTime)
			et, _ := time.Parse("2006-01-02", args.EndTime)
			timeLimit = []int64{
				st.Unix(),
				et.Unix(),
			}
		}
		getTripsBaseData, err := tripDbx.GetTripsBaseData(
			[]string{},
			authorId, args.TripType,
			1, 100000,
			timeLimit,
			[]int64{},
			[]string{},
			int64(args.MinDistance),
			ncommon.IfElse(args.MaxDistance > 0, int64(args.MaxDistance), 500*1000), true,
		)
		if err != nil {
			meta.Status = "failed"
			meta.Error = err.Error()
			return
		}

		ts := tripDbx.FormatTripStatistics(getTripsBaseData)

		type TripStatsOutput struct {
			Summary struct {
				TripTotalCount int     `json:"trip_total_count"`
				TotalDistance  float64 `json:"total_distance_km"`
				TotalDuration  float64 `json:"total_duration_hours"`
				ActiveDays     int     `json:"active_days"`
			} `json:"summary"`

			Achievements struct {
				MaxDistance          float64 `json:"max_distance_km"`        // 单次最远
				MaxSpeed             float64 `json:"max_speed_kmh"`          // 最高时速
				FastestAverageSpeed  float64 `json:"fastest_avg_speed"`      // 最快平均时速
				MaxAltitude          float64 `json:"max_altitude_m"`         // 最高海拔
				MinAltitude          float64 `json:"min_altitude_m"`         // 最低海拔
				MaxClimbAltitude     float64 `json:"max_climb_m"`            // 单次最大爬升
				MaxDescendAltitude   float64 `json:"max_descend_m"`          // 单次最大下降
				MaxTotalTripDuration float64 `json:"max_total_duration_h"`   // 最长旅途耗时(含休息)
				MaxDrivingDuration   float64 `json:"max_driving_duration_h"` // 最长纯驾驶耗时
			} `json:"achievements"`
		}

		output := TripStatsOutput{}

		// --- Summary ---
		output.Summary.TripTotalCount = int(ts.Count)
		output.Summary.ActiveDays = int(ts.Days)

		var toTwo = func(f float64) float64 {
			return math.Round(f*100) / 100
		}

		output.Summary.TotalDistance = toTwo(ts.Distance / 1000)
		output.Summary.TotalDuration = toTwo(float64(ts.Time) / 3600)

		// --- Achievements (极限成就补全) ---
		if ts.MaxDistance != nil {
			output.Achievements.MaxDistance = toTwo(ts.MaxDistance.Num / 1000)
		}
		if ts.MaxSpeed != nil {
			output.Achievements.MaxSpeed = toTwo(ts.MaxSpeed.Num)
		}
		if ts.FastestAverageSpeed != nil {
			output.Achievements.FastestAverageSpeed = toTwo(ts.FastestAverageSpeed.Num)
		}
		if ts.MaxAltitude != nil {
			output.Achievements.MaxAltitude = toTwo(ts.MaxAltitude.Num)
		}
		if ts.MinAltitude != nil {
			output.Achievements.MinAltitude = toTwo(ts.MinAltitude.Num)
		}
		if ts.MaxClimbAltitude != nil {
			output.Achievements.MaxClimbAltitude = toTwo(ts.MaxClimbAltitude.Num)
		}
		if ts.MaxDescendAltitude != nil {
			output.Achievements.MaxDescendAltitude = toTwo(ts.MaxDescendAltitude.Num)
		}
		if ts.MaxTotalTripDuration != nil {
			// 转换秒为小时
			output.Achievements.MaxTotalTripDuration = toTwo(ts.MaxTotalTripDuration.Num / 3600)
		}
		if ts.MaxDrivingDuration != nil {
			// 转换秒为小时
			output.Achievements.MaxDrivingDuration = toTwo(ts.MaxDrivingDuration.Num / 3600)
		}

		results, _ := json.Marshal(output)
		log.Info("results", string(results))

		meta.Status = "success"
		meta.Value = string(results)

		return

	case SEARCH_PLACE_INFO:
		type ReqParams struct {
			Query          string `json:"query"`
			WikiLevel      string `json:"wiki_level"`
			IncludeWeather bool   `json:"include_weather"`
			IncludePois    bool   `json:"include_pois"`
		}

		var args ReqParams

		log.Info("SEARCH_PLACE_INFO Arguments", toolCall.Function.Arguments)
		if err = json.Unmarshal([]byte(toolCall.Function.Arguments), &args); err != nil {
			meta.Status = "failed"
			meta.Error = err.Error()

			return
		}

		j, _ := json.MarshalIndent(args, "", "  ")
		log.Info("args", string(j))

		if err = validation.ValidateStruct(
			&args,
			validation.Parameter(&args.Query, validation.Required()),
		); err != nil {
			meta.Status = "failed"
			meta.Error = err.Error()

			return
		}

		contextData := []*ContextDataItem{}

		// meta.Status = "success"
		// meta.Error = err.Error()

		geoData, err := methods.Geo(args.Query)

		if err != nil {
			meta.Status = "failed"
			meta.Error = err.Error()

			return
		}

		log.Info("geoData", geoData)

		if args.WikiLevel != "" {

			// 1. 定义按优先级排序的查询备选项
			queries := []string{args.Query, geoData.Town, geoData.City, geoData.Region, geoData.State}

			wikiPage, err := methods.GetCityWikiSummary(ctx, queries)
			if err != nil {
				meta.Status = "failed"
				meta.Error = err.Error()
				return
			}

			if wikiPage != nil && wikiPage.Extract != "" {
				cdt := ContextDataItem{
					Type: "WikiSummary",
					Data: wikiPage.Extract,
				}
				contextData = append(contextData, &cdt)
			}

		}

		if args.IncludePois {
			radius := float32(10 * 1000)
			poisLen := 5

			// POIs 获取
			// lat := float64(29.56494256724482)
			// lng := float64(106.57476939325748)
			poiPoints, err := poiDbx.SearchPOI(ctx, POISearchParams{
				Lat: &geoData.Latitude,
				Lon: &geoData.Longitude,
				// Lat:    &lat,
				// Lon:    &lng,
				Radius: radius,
				Limit:  20,
			})
			if err != nil {
				log.Error(err)
				meta.Status = "failed"
				meta.Error = err.Error()

				return
			}

			log.Info("poiPoints", len(poiPoints))

			var pois []*POIsItem
			if len(poiPoints) > 0 {
				for _, v := range poiPoints {
					if v.POI != nil && len(pois) < poisLen {
						pois = append(pois, &POIsItem{
							Name:        v.POI.Name,
							WikiSummary: v.POI.Wiki.Summary,
							DistanceM:   aiDbx.Round(v.POI.Distance, 1),
							Direction:   v.POI.Direction,
							BearingDeg:  v.POI.Bearing,
						})
					}
				}
			}

			if len(pois) > 0 {
				cdt := ContextDataItem{
					Type: "POIs",
					Data: pois,
				}

				contextData = append(contextData, &cdt)

			}
		}

		if args.IncludeWeather {

			wi, err := methods.GetFullWeather(geoData.Latitude, geoData.Longitude)

			log.Info("GetTodayWeather", wi)
			if err != nil {
				log.Error(err)
				// meta.Status = "failed"
				// meta.Error = err.Error()
				// return
			}

			if wi != nil {
				// 2. 获取下一个小时的数据
				weatherData := WeatherItem{
					NowCond:  wi.Weather,
					NowTempC: aiDbx.Round(wi.Temperature, 1),
				}
				nextStr := time.Now().Add(1 * time.Hour).Format("2006-01-02T15:00")

				for _, v := range wi.Hourly {
					if v.Time == nextStr {
						weatherData.F1hTempC = v.Temperature
						weatherData.F1hCond = v.Weather
					}
				}

				contextData = append(contextData, &ContextDataItem{
					Type: "Weather",
					Data: weatherData,
				})
			}

		}

		results, _ := json.Marshal(contextData)

		if conf.Config.Server.Mode == "debug" {
			results, _ = json.MarshalIndent(contextData, "", "  ")
		}

		log.Info("results", string(results))

		meta.Status = "success"
		meta.Value = string(results)

		return

	case GET_FULL_WEATHER:
		type ReqParams struct {
			Query     string  `json:"query"`
			Latitude  float64 `json:"latitude"`
			Longitude float64 `json:"longitude"`
		}

		var args ReqParams

		log.Info("GET_FULL_WEATHER Arguments", toolCall.Function.Arguments)
		if err = json.Unmarshal([]byte(toolCall.Function.Arguments), &args); err != nil {
			meta.Status = "failed"
			meta.Error = err.Error()

			return
		}

		j, _ := json.MarshalIndent(args, "", "  ")
		log.Info("args", string(j))

		if err = validation.ValidateStruct(
			&args,
			// validation.Parameter(&args.Query, validation.Required()),
		); err != nil {
			meta.Status = "failed"
			meta.Error = err.Error()
			return
		}

		// meta.Status = "success"
		// meta.Error = err.Error()

		lat, lng := args.Latitude, args.Longitude

		if lat == 0 && lng == 0 && args.Query != "" {
			geoData, err := methods.Geo(args.Query)

			if err != nil {
				meta.Status = "failed"
				meta.Error = err.Error()

				return
			}

			log.Info("geoData", geoData)

			lat, lng = geoData.Latitude, geoData.Longitude
		}

		if lat == 0 && lng == 0 {
			meta.Status = "failed"
			return
		}

		wi, err := methods.GetFullWeather(lat, lng)

		// log.Info("weatherInfo", wi)
		if err != nil || wi == nil {
			meta.Status = "failed"
			meta.Error = err.Error()
			return
		}

		// WeatherSummary 给 LLM 消费的语义化结构
		type WeatherSummary struct {
			Current struct {
				Weather       string  `json:"weather"`
				TempC         float64 `json:"temperature_c"`
				FeelsLikeC    float64 `json:"feels_like_celsius"`
				HumidityPct   float64 `json:"humidity_percent"`
				PressureHpa   float64 `json:"pressure_hpa"`
				Wind          string  `json:"wind_status"` // 格式如: "东北风 (3.2 m/s)"
				VisibilityM   float64 `json:"visibility_meters"`
				Precipitation float64 `json:"precipitation_mm"`
			} `json:"current"`

			Hourly []string `json:"hourly_forecast_24h"` // 简化为字符串数组节省 Token
			Daily  []string `json:"daily_trend_7d"`
		}

		// 构造语义化视图
		summary := WeatherSummary{}

		// 1. 实况部分
		summary.Current.Weather = wi.Weather
		summary.Current.TempC = wi.Temperature
		summary.Current.FeelsLikeC = wi.ApparentTemperature
		summary.Current.HumidityPct = wi.Humidity
		summary.Current.PressureHpa = wi.Pressure
		summary.Current.Wind = fmt.Sprintf("%s (%.1f m/s)", wi.WindDirection, wi.WindSpeed)
		summary.Current.VisibilityM = wi.Visibility
		summary.Current.Precipitation = wi.Precipitation

		// 2. 24小时趋势 (每 6 小时采样，转为语义化短句)
		for i, h := range wi.Hourly {
			if i%3 == 0 {
				item := fmt.Sprintf("%s: %.1f°C, %s", h.Time[11:16], h.Temperature, h.Weather)
				summary.Hourly = append(summary.Hourly, item)
			}
		}

		// 3. 7天趋势 (转为语义化短句)
		for _, d := range wi.Daily {
			item := fmt.Sprintf("%s: %s, %.1f~%.1f°C", d.Date[5:], d.Weather, d.MinTemp, d.MaxTemp)
			summary.Daily = append(summary.Daily, item)
		}

		// 4. 序列化为 JSON 字符串
		// jsonData, _ := json.Marshal(summary)

		if conf.Config.Server.Mode == "debug" {
			j, _ = json.MarshalIndent(summary, "", "  ")
			log.Info("jsonData", string(j))
		}

		// 注入到 ContextData

		results, _ := json.Marshal(&ContextDataItem{
			Type: "FullWeather",
			Data: summary,
		})

		// if conf.Config.Server.Mode == "debug" {
		// 	results, _ = json.MarshalIndent(userInputParams, "", "  ")
		// }

		// log.Info("results", string(results))

		meta.Status = "success"
		meta.Value = string(results)

		return

	case "get_roadbook_detail":
		var args struct {
			Id string `json:"id"`
		}

		if err = json.Unmarshal([]byte(toolCall.Function.Arguments), &args); err != nil {
			meta.Status = "failed"
			meta.Error = err.Error()
			return
		}

		if err = validation.ValidateStruct(
			&args,
			validation.Parameter(&args.Id, validation.Type("string"), validation.Required()),
		); err != nil {
			meta.Status = "failed"
			meta.Error = err.Error()

			return
		}

		rb, err := roadbookDbx.GetRoadbook(
			args.Id, authorId,
		)
		log.Info(rb, err)
		if err != nil || rb == nil {
			meta.Status = "failed"
			if err != nil {
				meta.Error = err.Error()
			}

			return
		}

		// meta.Status = "success"
		// meta.Value = fc.getRoadbookDistilledStr(rb)

		var timelineStrings []string
		currentStartTime := time.Unix(rb.StartTime, 0)
		for i, tl := range rb.Timelines {
			// 计算当前这段行程的起止日期
			// 比如：StartTime 是 10-07，Days 是 3，那么这段就是 10-07 到 10-09
			duration := time.Duration(tl.Days) * 24 * time.Hour
			endTime := currentStartTime.Add(duration - time.Second) // 减 1 秒为了显示当天的结束

			var wpStrings []string
			for _, wp := range tl.Waypoints {

				location := wp.City.City
				if wp.City.Road != "" {
					location = wp.City.Road
				} else if wp.City.Town != "" {
					location = wp.City.Town
				}
				navInfo := ""
				if wp.Navigation != nil && wp.Navigation.Distance > 0 {
					navInfo = fmt.Sprintf(" (距下点 %.1fkm, 耗时 %.1fmin)",
						wp.Navigation.Distance/1000, wp.Navigation.Duration/60)
				}

				wpStrings = append(wpStrings, `{
					"id": "`+wp.Id+`",
					"name": "`+wp.Address+`",
					"address": "`+strings.Join([]string{
					wp.City.Country,
					wp.City.State,
					wp.City.Region,
					wp.City.City,
					wp.City.Town,
				}, "")+`",
					"overview":"`+fmt.Sprintf("    - %s%s", location, navInfo)+`"
				}`)
			}
			// 格式化当前阶段的描述

			timelineStrings = append(timelineStrings, `{
				"id": "`+tl.Id+`",
				"title": "`+tl.Title+`",
				"desc": "`+tl.Desc+`",
				"days": `+nstrings.ToString(tl.Days)+`,
				"overview":"`+fmt.Sprintf("第%d个时间线 [%s 至 %s] (共 %d 天):\n  标题: %s\n  描述: %s\n  途径点列表:\n%s",
				i+1,
				currentStartTime.Format("2006-01-02"), // MM-DD 格式
				endTime.Format("2006-01-02"),
				tl.Days,
				tl.Title,
				tl.Desc,
				strings.Join(wpStrings, "\n"))+`",
				"waypoints": [
					`+strings.Join(wpStrings, ",")+`
				],
			}`)

			// 重要：更新游标，供下一个 Timeline 使用
			currentStartTime = currentStartTime.Add(duration)
		}

		// 2. 组装最终给 AI 的 JSON (或者是语义化的字符串)
		// 提示：这里直接用 fmt.Sprintf 或者 json.Marshal 更好，手动拼接字符串容易出错

		results := `{
	"id": "` + rb.Id + `",
	"title": "` + rb.Title + `",
	"desc": "` + rb.Desc + `",
	"startTime": "` + time.Unix(rb.StartTime, 0).Format("2006-01-02") + `",
	"timelines": [
		` + strings.Join(timelineStrings, ",") + `
	]
}
	  `

		log.Info("results", results)

		meta.Status = "success"
		meta.Value = results

		return

		// log.Info("更新上下文，准备让 AI 解释刚才的操作", args)

	case "update_roadbook", "update_roadbook_detail", "update_roadbook_timeline":

		type SyncRoadbookReq struct {
			Id        string `json:"id"` // 路书 ID
			Title     string `json:"title"`
			StartTime string `json:"startTime"`
			Desc      string `json:"desc"`
			Timelines []struct {
				Id        string `json:"id"`    // 时间线 ID
				Title     string `json:"title"` // 时间线标题
				Desc      string `json:"desc"`  // 时间线描述
				Days      int    `json:"days"`  // 持续天数
				Waypoints []struct {
					Id      string `json:"id"`      // AI 传回来的旧 ID
					Name    string `json:"name"`    // 途径点名称
					Address string `json:"address"` // 详细地址
				} `json:"waypoints"` // 嵌套途径点
			} `json:"timelines"` // 全量时间线数组
		}
		var args SyncRoadbookReq

		if err = json.Unmarshal([]byte(toolCall.Function.Arguments), &args); err != nil {
			meta.Status = "failed"
			meta.Error = err.Error()

			return
		}

		log.Info("update_roadbook_detail args", args)

		if err = validation.ValidateStruct(
			&args,
			validation.Parameter(&args.Id, validation.Type("string"), validation.Required()),
		); err != nil {
			meta.Status = "failed"
			meta.Error = err.Error()

			return
		}

		startTimeDate, _ := time.Parse("2006-01-02", args.StartTime)

		startTime := startTimeDate.Unix()

		rb, err := roadbookDbx.GetRoadbook(
			args.Id, authorId,
		)
		if err != nil || rb == nil {
			meta.Status = "failed"
			if err != nil {
				meta.Error = err.Error()
			}

			return
		}

		if !(args.Title != "" || startTime > 0 || args.Desc != "") {
			meta.Status = "failed"
			meta.Error = "No argument provided, nothing has changed."
			return
		}

		if args.Title == "" {
			args.Title = rb.Title
		}
		if args.Desc == "" {
			args.Desc = rb.Desc
		}
		if startTime <= 0 {
			startTime = rb.StartTime
		}

		if err = roadbookDbx.UpdateRoadbook(
			args.Id, authorId,
			args.Title,
			args.Desc,
			startTime,
			rb.Timelines,
			rb.Permissions,
		); err != nil {
			meta.Status = "failed"
			meta.Error = err.Error()

			return
		}

		meta.Status = "success"

		return

	case "create_roadbook":
		var args struct {
			Id        string `json:"id"`
			Title     string `json:"title"`
			StartTime string `json:"startTime"`
			Desc      string `json:"desc"`
		}

		if err = json.Unmarshal([]byte(toolCall.Function.Arguments), &args); err != nil {
			meta.Status = "failed"
			meta.Error = err.Error()

			return
		}

		if err = validation.ValidateStruct(
			&args,
			validation.Parameter(&args.Title, validation.Type("string"), validation.Required()),
			validation.Parameter(&args.StartTime, validation.Type("string"), validation.Required()),
			validation.Parameter(&args.Desc, validation.Type("string"), validation.Required()),
		); err != nil {
			meta.Status = "failed"
			meta.Value = err.Error()

			return
		}

		startTimeDate, _ := time.Parse("2006-01-02", args.StartTime)
		startTime := startTimeDate.Unix()

		rb, err := roadbookDbx.AddRoadbook(&models.Roadbook{
			Title:     args.Title,
			Desc:      args.Desc,
			StartTime: startTime,
			AuthorId:  authorId,
		})
		log.Info(rb, err)
		if err != nil {
			meta.Status = "failed"
			meta.Value = err.Error()

			return
		}

		meta.Status = "success"
		meta.Value = fc.getRoadbookDistilledStr(rb)

		return

	case "get_roadbook_timelines":
		var args struct {
			Id string `json:"id"`
		}

		if err = json.Unmarshal([]byte(toolCall.Function.Arguments), &args); err != nil {
			meta.Status = "failed"
			meta.Error = err.Error()

			return
		}

		if err = validation.ValidateStruct(
			&args,
			validation.Parameter(&args.Id, validation.Type("string"), validation.Required()),
		); err != nil {
			meta.Status = "failed"
			meta.Error = err.Error()

			return
		}

		rb, err := roadbookDbx.GetRoadbook(
			args.Id, authorId,
		)
		log.Info(rb, err)
		if err != nil || rb == nil {
			meta.Status = "failed"
			if err != nil {
				meta.Error = err.Error()
			}

			return
		}

		var timelineStrings []string
		currentStartTime := time.Unix(rb.StartTime, 0)
		for _, tl := range rb.Timelines {
			// 计算当前这段行程的起止日期
			// 比如：StartTime 是 10-07，Days 是 3，那么这段就是 10-07 到 10-09
			duration := time.Duration(tl.Days) * 24 * time.Hour

			var wpStrings []string
			for _, wp := range tl.Waypoints {
				wpStrings = append(wpStrings, `{
					"id": "`+wp.Id+`",
					"name": "`+wp.Address+`",
					"address": "`+strings.Join([]string{
					wp.City.Country,
					wp.City.State,
					wp.City.Region,
					wp.City.City,
					wp.City.Town,
				}, "")+`",
				}`)
			}
			// 格式化当前阶段的描述

			timelineStrings = append(timelineStrings, `{
				"id": "`+tl.Id+`",
				"title": "`+tl.Title+`",
				"desc": "`+tl.Desc+`",
				"days": `+nstrings.ToString(tl.Days)+`,
				"waypoints": [
					`+strings.Join(wpStrings, ",")+`
				],
			}`)

			// 重要：更新游标，供下一个 Timeline 使用
			currentStartTime = currentStartTime.Add(duration)
		}

		// 2. 组装最终给 AI 的 JSON (或者是语义化的字符串)
		// 提示：这里直接用 fmt.Sprintf 或者 json.Marshal 更好，手动拼接字符串容易出错

		results := `{
	"id": "` + rb.Id + `",
	"title": "` + rb.Title + `",
	"desc": "` + rb.Desc + `",
	"startTime": "` + time.Unix(rb.StartTime, 0).Format("2006-01-02") + `",
	"timelines": [
		` + strings.Join(timelineStrings, ",") + `
	]
}
	  `

		log.Info("results", results)

		meta.Status = "success"
		meta.Value = results

		return

	case "update_roadbook_timelines":
		type SyncRoadbookReq struct {
			Id        string `json:"id"` // 路书 ID
			Timelines []struct {
				Id        string `json:"id"`    // 时间线 ID
				Title     string `json:"title"` // 时间线标题
				Desc      string `json:"desc"`  // 时间线描述
				Days      int    `json:"days"`  // 持续天数
				Waypoints []struct {
					Id      string `json:"id"`      // AI 传回来的旧 ID
					Name    string `json:"name"`    // 途径点名称
					Address string `json:"address"` // 详细地址
				} `json:"waypoints"` // 嵌套途径点
			} `json:"timelines"` // 全量时间线数组
		}

		var args SyncRoadbookReq

		if err = json.Unmarshal([]byte(toolCall.Function.Arguments), &args); err != nil {
			meta.Status = "failed"
			meta.Error = err.Error()

			return
		}

		if err = validation.ValidateStruct(
			&args,
			validation.Parameter(&args.Id, validation.Type("string"), validation.Required()),
		); err != nil {
			meta.Status = "failed"
			meta.Error = err.Error()

			return
		}

		// if err = roadbookDbx.UpdateRoadbook(
		// 	args.Id, authorId,
		// 	args.Title,
		// 	args.Desc,
		// 	startTime,
		// 	rb.Timelines,
		// 	rb.Permissions,
		// ); err != nil {
		// 	meta.Status = "failed"
		// 	meta.Error = err.Error()

		// 	callFunc()
		// 	return meta
		// }

		log.Info("update_roadbook_timelines args", args)

		meta.Status = "success"

		return

	}

	log.Info("更新上下文，准备让 AI 解释刚才的操作")
	meta.Status = "failed"
	meta.Error = "No argument provided, nothing has changed."

}

func (fc *AIDbx) ParseTextToToolCall(content string) (bool, openai.ToolCall) {
	var tc openai.ToolCall

	// 1. 兼容型正则：匹配工具名 (get_roadbook_content)
	nameRe := regexp.MustCompile(`(?s)<tool_call>([\w_]+)`)
	nameMatches := nameRe.FindStringSubmatch(content)
	if len(nameMatches) < 2 {
		return false, tc
	}
	funcName := nameMatches[1]

	// 2. 提取所有的 Key-Value 对
	// 匹配模式：<arg_key>xxx</arg_key>\s*<arg_value>yyy</arg_value>
	kvRe := regexp.MustCompile(`(?s)<arg_key>(.*?)</arg_key>\s*<arg_value>(.*?)</arg_value>`)
	kvMatches := kvRe.FindAllStringSubmatch(content, -1)

	// 3. 将提取到的 KV 对组装成 JSON 字符串
	argsMap := make(map[string]interface{})
	for _, match := range kvMatches {
		if len(match) == 3 {
			key := strings.TrimSpace(match[1])
			value := strings.TrimSpace(match[2])
			argsMap[key] = value
		}
	}

	// 4. 如果没有解析出任何参数，尝试找一下有没有纯 JSON (兜底逻辑)
	if len(argsMap) == 0 {
		jsonRe := regexp.MustCompile(`(?s)\{.*\}`)
		jsonStr := jsonRe.FindString(content)
		if jsonStr != "" && json.Valid([]byte(jsonStr)) {
			// 直接使用纯 JSON 逻辑
		}
	}

	// 5. 序列化回 JSON 字符串，以适配 openai.ToolCall 结构
	argsBytes, _ := json.Marshal(argsMap)
	argsStr := string(argsBytes)

	if funcName != "" && len(argsMap) > 0 {
		tc = openai.ToolCall{
			ID:   "call_" + funcName,
			Type: openai.ToolTypeFunction,
			Function: openai.FunctionCall{
				Name:      funcName,
				Arguments: argsStr,
			},
		}
		return true, tc
	}

	return false, tc
}
func (fc *AIDbx) getRoadbookDistilledStr(rb *models.Roadbook) string {

	currentStartTime := time.Unix(rb.StartTime, 0)
	var timelineStrings []string
	for i, tl := range rb.Timelines {
		// 计算当前这段行程的起止日期
		// 比如：StartTime 是 10-07，Days 是 3，那么这段就是 10-07 到 10-09
		duration := time.Duration(tl.Days) * 24 * time.Hour
		endTime := currentStartTime.Add(duration - time.Second) // 减 1 秒为了显示当天的结束

		var wpStrings []string
		for _, wp := range tl.Waypoints {
			location := wp.City.City
			if wp.City.Road != "" {
				location = wp.City.Road
			} else if wp.City.Town != "" {
				location = wp.City.Town
			}

			navInfo := ""
			if wp.Navigation != nil && wp.Navigation.Distance > 0 {
				navInfo = fmt.Sprintf(" (距上点 %.1fkm, 耗时 %.1fmin)",
					wp.Navigation.Distance/1000, wp.Navigation.Duration/60)
			}
			wpStrings = append(wpStrings, fmt.Sprintf("    - %s%s", location, navInfo))
		}

		// 格式化当前阶段的描述
		tlItem := fmt.Sprintf("第%d个时间线 [%s 至 %s] (共 %d 天):\n  标题: %s\n  描述: %s\n  途径点列表:\n%s",
			i+1,
			currentStartTime.Format("2006-01-02"), // MM-DD 格式
			endTime.Format("2006-01-02"),
			tl.Days,
			tl.Title,
			tl.Desc,
			strings.Join(wpStrings, "\n"))

		timelineStrings = append(timelineStrings, tlItem)

		// 重要：更新游标，供下一个 Timeline 使用
		currentStartTime = currentStartTime.Add(duration)
	}

	// 2. 组装最终给 AI 的 JSON (或者是语义化的字符串)
	// 提示：这里直接用 fmt.Sprintf 或者 json.Marshal 更好，手动拼接字符串容易出错
	result := fmt.Sprintf(`{
		"roadbookId": "%s",
		"summary": {
			"title": "%s",
			"desc": "%s",
			"startTime": "%s"
		},
		"timelines": %s
	}`,
		rb.Id,
		rb.Title,
		rb.Desc,
		time.Unix(rb.StartTime, 0).Format("2006-01-02"),
		`"`+strings.Join(timelineStrings, "\n\n")+`"`, // 这里把详情作为一段文字传给 AI，比纯嵌套 JSON 更省 Token
	)

	return result

}

func ToPtr[T any](f T) *T { return &f }
