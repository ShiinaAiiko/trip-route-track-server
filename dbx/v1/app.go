package dbxV1

import (
	"encoding/json"
	"strings"

	"github.com/ShiinaAiiko/nyanya-trip-route-track/server/protos"
)

type AppDbx struct {
}

var (
	appDbx = AppDbx{}
)

// AppFeatureBrief 发给 AI 的脱水版信息
type AppManifestItem struct {
	ID   string `json:"id"`
	Desc string `json:"desc"` // 告诉 AI 什么时候该选这个 ID
}

// AppFeatureDetail 后端补全给前端的 UI 渲染数据
type ManifestHydratorPathItem struct {
	Type  string `json:"type"` // 渲染类型: NAV_BUTTON, MARKDOWN_CARD, COMPONENT
	Title string `json:"title,omitempty"`
	Path  string `json:"path,omitempty"`
}

var (
	// 给 AI 看的索引
	AppManifest = []*AppManifestItem{
		{ID: "NAV_HOME", Desc: "主页/行程路线轨迹。查看当前地图、速度、海拔及里程数据，开启新行程。"},
		{ID: "NAV_TRIP_HISTORY", Desc: "行程历史列表。按日期、载具或记忆筛选过去的行驶记录。"},
		{ID: "NAV_TRACK_ROUTE", Desc: "足迹地图。已走过的行程所有轨迹路线均会被绘制在地图上。"},
		{ID: "NAV_JOURNEY_MEMORY", Desc: "旅途记忆。以多媒体形式回顾路上的笔记、照片和视频。"},
		{ID: "NAV_ROADBOOK", Desc: "自驾路书。创建专业行程、测算里程时长或参考车主精品路线。"},
		{ID: "NAV_ALTITUDE", Desc: "实时海拔。精准测量当前位置的经纬度、海拔高度。"},
		{ID: "NAV_CITY_FOOTPRINT", Desc: "城市足迹。查看走过的国家、省市统计及抵达时间线。"},
		{ID: "NAV_VEHICLE_MGMT", Desc: "我的载具。增删改查车辆信息，绑定行程。"},
		{ID: "NAV_PRIVACY_FENCE", Desc: "隐私围栏。设置地图隐藏区域，保护起止点隐私。"},
		{ID: "NAV_CUSTOM_ROUTE", Desc: "自定义行程。手动绘制因信号丢失而中断的行程轨迹。"},
		{ID: "NAV_SETTINGS", Desc: "系统设置。账号管理、语言切换、地图偏好及缓存清理。"},
		{ID: "NAV_CONTACT_DEV", Desc: "联系开发者。反馈建议、查看博客或获取技术支持。"},
	}

	// 后端用来补全的详情表
	ManifestHydrator = map[string]*protos.AIResponse_ActionItem{
		"NAV_HOME": {
			Title:   "行程路线轨迹",
			Content: "### 专业自驾数据监测\n全方位记录您的**GPS轨迹、瞬时速度、累计里程及海拔高度**。支持：\n- **多模式切换**：适配步行、骑行及驾驶动力算法。\n- **数据可视化**：实时仪表盘显示，确保每一公里都精准入库。",
			Paths: []*protos.AIResponse_ActionItem_ActionPathItem{
				{
					Type: "OPEN_LINK",
					Path: "/",
				},
			},
		},
		"NAV_TRIP_HISTORY": {
			Title:   "行程历史",
			Content: "### 深度复盘每一公里\n支持强大的**多维检索系统**：\n- **标签筛选**：按月份、载具或特定的旅途标签搜索。\n- **数据穿透**：直接进入单次行程的最高速度、最高海拔等核心统计页。",
			Paths: []*protos.AIResponse_ActionItem_ActionPathItem{
				{
					Type: "OPEN_MODAL",
					Path: "modal://track_history",
				},
			},
		},
		"NAV_TRACK_ROUTE": {
			Title:   "足迹地图",
			Content: "### 绘制属于你的生命版图\n这是您所有自驾行程的**可视化总结**。系统将自动汇总所有历史轨迹，在地图上以热力或线条形式展现。点击任意路段，即可瞬间回溯至那一天的驾驶详情。",
			Paths: []*protos.AIResponse_ActionItem_ActionPathItem{
				{
					Type: "OPEN_LINK",
					Path: "/trackRoute",
				},
			},
		},
		"NAV_JOURNEY_MEMORY": {
			Title:   "旅途记忆",
			Content: "### 让瞬间成为永恒\n不只是轨迹，更是**情感的容器**。支持以时间线记录：\n- **图文笔记**：定格美景与感悟。\n- **视频挂载**：还原路上的动态瞬间。\n- **地理位置锚点**：在地图上精准还原拍照位置。",
			Paths: []*protos.AIResponse_ActionItem_ActionPathItem{
				{
					Type: "OPEN_MODAL",
					Path: "modal://journeyMemories",
				},
			},
		},
		"NAV_ROADBOOK": {
			Title:   "自驾路书",
			Content: "### 掌控出发，心中有数\n- **智能规划**：自动计算每日最佳行驶时长与油耗建议。\n- **海量资源**：浏览资深车主分享的路线，获取**美食、住宿、避坑**第一手情报。\n- **工具集**：内置海拔分析与天气预警，拒绝盲目出发。",
			Paths: []*protos.AIResponse_ActionItem_ActionPathItem{
				{
					Type: "OPEN_LINK",
					Path: "/roadbook",
				},
			},
		},
		"NAV_ALTITUDE": {
			Title:   "实时海拔",
			Content: "### 高海拔地区必备\n基于 GPS 与气压传感器的**双重校准算法**，实时反馈您当前所在的：\n- **物理高度**：精确至米级。\n- **地理坐标**：经纬度精准定位。\n- **环境参数**：为高原驾驶提供关键安全参考。",
			Paths: []*protos.AIResponse_ActionItem_ActionPathItem{
				{
					Type: "OPEN_LINK",
					Path: "/altitude",
				},
			},
		},
		"NAV_CITY_FOOTPRINT": {
			Title:   "城市足迹",
			Content: "### 丈量你的世界宽度\n系统自动扫描历史轨迹，并统计您曾进入的**国家、省份、市、区县**。以时间轴形式展现您征服每一座行政区的先后顺序，生成您专属的**足迹荣誉墙**。",
			Paths: []*protos.AIResponse_ActionItem_ActionPathItem{
				{
					Type: "OPEN_MODAL",
					Path: "modal://city_footprint",
				},
			},
		},
		"NAV_VEHICLE_MGMT": {
			Title:   "我的载具",
			Content: "### 为每一辆车建立档案\n记录您的**载具详情**，并在开启行程时一键绑定。支持：\n- **能耗统计**：查看不同载具的行驶效率。\n- **载具切换**：支持摩托、轿车及重卡等多台载具管理。",
			Paths: []*protos.AIResponse_ActionItem_ActionPathItem{
				{
					Type: "OPEN_MODAL",
					Path: "modal://my_vehicle",
				},
			},
		},
		"NAV_PRIVACY_FENCE": {
			Title:   "隐私围栏",
			Content: "### 隐形保护，安心分享\n在地图上自由绘制**隐私区域**。当您分享轨迹给好友或社交平台时，围栏内的路段将被**物理截断**。有效防止家、公司或敏感驻车点的坐标外泄。",
			Paths: []*protos.AIResponse_ActionItem_ActionPathItem{
				{
					Type: "OPEN_MODAL",
					Path: "modal://privacy_fence",
				},
			},
		},
		"NAV_CUSTOM_ROUTE": {
			Title:   "自定义行程",
			Content: "### 完美主义者的补完计划\n自驾途中难免遇到隧道屏蔽或信号丢失。利用此工具，您可以根据记忆**在地图上补划**缺失路段，系统将自动平滑曲线，确保整个行程的逻辑连续性与美观度。",
			Paths: []*protos.AIResponse_ActionItem_ActionPathItem{
				{
					Type: "OPEN_MODAL",
					Path: "modal://custom_route",
				},
			},
		},
		"NAV_SETTINGS": {
			Title:   "系统设置",
			Content: "### 个性化领航配置\n- **账号登录**：多端数据云端无感同步。\n- **地图引擎**：切换 2D/3D 视角或离线包管理。\n- **多语偏好**：支持多国语切换及缓存深度清理，确保 App 运行流畅。",
			Paths: []*protos.AIResponse_ActionItem_ActionPathItem{
				{
					Type: "OPEN_MODAL",
					Path: "modal://settings",
				},
			},
		},
		"NAV_CONTACT_DEV": {
			Title:   "联系开发者",
			Content: "### 开发者寄语\n我是 **ShiinaAiiko**，一个热爱写代码、更热爱在路上的全栈开发者。如果您有新的功能点子或遇到了奇怪的 Bug，欢迎随时联系我。\n- **个人博客**: https://aiiko.club/ShiinaAiiko\n- **官方邮箱**: shiina@aiiko.club",

			Paths: []*protos.AIResponse_ActionItem_ActionPathItem{
				{
					Type:  "OPEN_LINK",
					Title: "发送邮件",
					Path:  "mailto:shiina@aiiko.club",
				},
				{
					Type:  "OPEN_LINK",
					Title: "打开博客",
					Path:  "https://aiiko.club/ShiinaAiiko",
				},
			},
		},
	}
)

func (d *AppDbx) GetAppManifestStr() string {
	j, _ := json.Marshal(AppManifest)
	return string(j)
}

func (d *AppDbx) SearchAppManifest(query string) []*AppManifestItem {
	var results []*AppManifestItem
	// 使用 map 去重，防止 ID 同时在两个源中被搜到导致重复返回
	matchedIDs := make(map[string]bool)
	query = strings.ToLower(strings.TrimSpace(query))

	isFullSearch := query == ""

	// 1. 扫描索引层 (AppManifest)
	for _, item := range AppManifest {
		if isFullSearch ||
			strings.Contains(strings.ToLower(item.ID), query) ||
			strings.Contains(strings.ToLower(item.Desc), query) {
			matchedIDs[item.ID] = true
		}
	}

	// 2. 扫描内容层 (ManifestHydrator)
	// 这一步很关键：万一用户搜的是“降水曲线”，Desc里没有，但Content(完整介绍)里有
	for id, detail := range ManifestHydrator {
		if !isFullSearch && (strings.Contains(strings.ToLower(detail.Title), query) ||
			strings.Contains(strings.ToLower(detail.Content), query)) {
			matchedIDs[id] = true
		}
	}

	// 3. 根据命中的 ID 组装结果
	// 注意：我们要返回的是 AppManifestItem，这样 AI 拿到的还是它能理解的“简本”
	for _, item := range AppManifest {
		if matchedIDs[item.ID] {
			v := &AppManifestItem{
				ID: item.ID,
			}
			if !isFullSearch && len(matchedIDs) <= 2 {
				if detail, ok := ManifestHydrator[item.ID]; ok {
					// 临时修改 Desc，把详细介绍带进去
					// 这样做 AI 拿到的还是 AppManifestItem 结构，但内容更丰富了
					v.Desc = detail.Title + "\n" + detail.Content
				}
			}
			results = append(results, v)
		}
	}

	// 4. 兜底逻辑：如果什么都没搜到，返回全量
	if len(results) == 0 && !isFullSearch {
		return AppManifest
	}

	return results
}
