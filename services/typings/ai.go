package typings

type StreamResponse struct {
	Type    string      `json:"type"`    // "meta" (指令执行) 或 "text" (AI文案)
	Action  string      `json:"action"`  // 比如 "update_title"
	Value   interface{} `json:"value"`   // 设置的新值
	Content string      `json:"content"` // AI 吐出的字符碎片
}

type AIResponse[T any] struct {
	Status struct {
		// 200 成功 / 10001 失败
		Code int `json:"code"`
	} `json:"status"`

	Meta struct {
		Model  string `json:"model"`
		Action string `json:"action"`
		Value  string `json:"value"`
		Status string `json:"status"`
	} `json:"meta"`
	Display struct {
		Message string `json:"message"`
		Warning string `json:"warning,omitempty"`
	} `json:"display"`
	Reasoning struct {
		Message string `json:"message"`
	} `json:"reasoning"`

	Data T `json:"data"`
}

type WaypointItem struct {
	Id      string `json:"id,omitempty"` // 已有保留，新建留空
	Name    string `json:"name"`
	Address string `json:"addr"`          // 缩减 Key 名省 Token
	Action  string `json:"act,omitempty"` // ADD/UPDATE/KEEP
}

type TimelineItem struct {
	Id        string         `json:"id,omitempty"`
	Desc      string         `json:"desc,omitempty"`
	Days      int32          `json:"days"`
	Waypoints []WaypointItem `json:"wpts,omitempty"` // 缩减 Key 名
	Action    string         `json:"act,omitempty"`
}

type AIRoadbookResponse = AIResponse[struct {
	Summary struct {
		Days int32 `json:"days"` // 总天数
	} `json:"summary"`
	Timelines []TimelineItem `json:"tls"` // 缩减 Key 名
}]
