package methods

// // GPSPoint 定义 GPS 点结构
// type GPSPoint struct {
// 	Lat float64 `json:"lat"`
// 	Lng float64 `json:"lng"`
// }

// // GenerateRouteImageOptions 定义参数结构
// type GenerateRouteImageOptions struct {
// 	GPSPoints    [][]GPSPoint // 多组 GPS 点数组
// 	LineColor    color.RGBA   // 线条颜色，默认蓝色
// 	LineWidth    float64      // 线条粗细，默认 2
// 	BgImage      string       // 背景图，可以是 URL 或本地文件路径
// 	IsBgImageURL bool         // 指示 BgImage 是 URL (true) 还是文件路径 (false)
// 	PaddingX     float64      // 水平边界距离，默认 20
// 	PaddingY     float64      // 垂直边界距离，默认 20
// 	OutputPath   string       // 输出图片路径（包含文件名）
// 	Quality      int          // 输出质量（0-100），默认 100（无损）
// }

// // GenerateRouteImageResult 定义返回值结构
// type GenerateRouteImageResult struct {
// 	Base64    string // base64 图片字符串
// 	ImagePath string // 生成的图片文件路径
// }

// // GenerateRouteImage 生成行程路线缩略图（支持多组路径）
// func GenerateRouteImage(options GenerateRouteImageOptions) (GenerateRouteImageResult, error) {
// 	// 设置默认值
// 	if options.LineColor == (color.RGBA{}) {
// 		options.LineColor = color.RGBA{0, 123, 255, 255} // 默认蓝色 #007bff
// 	}
// 	if options.LineWidth == 0 {
// 		options.LineWidth = 2
// 	}
// 	if options.PaddingX == 0 {
// 		options.PaddingX = 20
// 	}
// 	if options.PaddingY == 0 {
// 		options.PaddingY = 20
// 	}
// 	if options.OutputPath == "" {
// 		options.OutputPath = "route-thumbnail.png"
// 	}
// 	if options.Quality <= 0 || options.Quality > 100 {
// 		options.Quality = 100
// 	}

// 	// 加载背景图并获取尺寸
// 	var width, height int
// 	var bgImg image.Image
// 	if options.BgImage != "" {
// 		var err error
// 		var reader io.Reader

// 		if options.IsBgImageURL {
// 			resp, err := http.Get(options.BgImage)
// 			if err != nil {
// 				log.Info("加载背景图 URL 失败:", err)
// 			} else {
// 				defer resp.Body.Close()
// 				reader = resp.Body
// 			}
// 		} else {
// 			file, err := os.Open(options.BgImage)
// 			if err != nil {
// 				log.Info("加载背景图文件失败:", err)
// 			} else {
// 				defer file.Close()
// 				reader = file
// 			}
// 		}

// 		if reader != nil {
// 			bgImg, _, err = image.Decode(reader)
// 			if err == nil {
// 				bounds := bgImg.Bounds()
// 				width = bounds.Dx()
// 				height = bounds.Dy()
// 			}
// 		}
// 	}

// 	if width == 0 || height == 0 {
// 		width = 300
// 		height = 200
// 	}

// 	// 创建画布
// 	dc := gg.NewContext(width, height)

// 	// 绘制背景
// 	if bgImg != nil {
// 		dc.DrawImage(bgImg, 0, 0)
// 	} else {
// 		dc.SetColor(color.RGBA{240, 240, 240, 255})
// 		dc.Clear()
// 	}

// 	// 计算所有点的经纬度范围
// 	var allLats, allLngs []float64
// 	for i, group := range options.GPSPoints {
// 		log.Info("路径组", i, "点数:", len(group))
// 		for j, p := range group {
// 			if j < 5 || j >= len(group)-5 { // 打印每组前5个和后5个点
// 				log.Info("路径组", i, "点", j, ":", p.Lat, p.Lng)
// 			}
// 			allLats = append(allLats, p.Lat)
// 			allLngs = append(allLngs, p.Lng)
// 		}
// 	}
// 	minLat, maxLat := minMax(allLats)
// 	minLng, maxLng := minMax(allLngs)
// 	log.Info("经纬度范围: Lat [", minLat, ",", maxLat, "], Lng [", minLng, ",", maxLng, "]")

// 	latRange := maxLat - minLat
// 	if latRange == 0 {
// 		latRange = 0.001
// 	}
// 	lngRange := maxLng - minLng
// 	if lngRange == 0 {
// 		lngRange = 0.001
// 	}

// 	// 转换每组 GPS 点为画布坐标
// 	pointGroups := make([][]struct{ x, y float64 }, len(options.GPSPoints))
// 	for i, group := range options.GPSPoints {
// 		points := make([]struct{ x, y float64 }, len(group))
// 		for j, point := range group {
// 			points[j].x = options.PaddingX + ((point.Lng-minLng)/lngRange)*(float64(width)-2*options.PaddingX)
// 			points[j].y = options.PaddingY + (float64(height) - 2*options.PaddingY) - ((point.Lat-minLat)/latRange)*(float64(height)-2*options.PaddingY)
// 		}
// 		pointGroups[i] = points
// 	}

// 	// 绘制多组路径（使用直线连接）
// 	dc.SetColor(options.LineColor)
// 	dc.SetLineWidth(options.LineWidth)

// 	for i, points := range pointGroups {
// 		if len(points) == 0 {
// 			continue
// 		}
// 		for j := 0; j < len(points); j++ {
// 			p := points[j]
// 			if j == 0 {
// 				dc.MoveTo(p.x, p.y)
// 			} else {
// 				dc.LineTo(p.x, p.y)
// 			}
// 		}
// 		dc.Stroke()
// 		log.Info("路径组", i, "绘制完成")
// 	}

// 	// 生成图片
// 	img := dc.Image()

// 	// 转换为 base64
// 	var buf bytes.Buffer
// 	encoder := &png.Encoder{CompressionLevel: png.CompressionLevel(-int(options.Quality / 34))}
// 	err := encoder.Encode(&buf, img)
// 	if err != nil {
// 		return GenerateRouteImageResult{}, err
// 	}
// 	base64Str := base64.StdEncoding.EncodeToString(buf.Bytes())

// 	// 保存到指定路径
// 	err = os.MkdirAll(filepath.Dir(options.OutputPath), 0755)
// 	if err != nil {
// 		return GenerateRouteImageResult{}, err
// 	}
// 	file, err := os.Create(options.OutputPath)
// 	if err != nil {
// 		return GenerateRouteImageResult{}, err
// 	}
// 	defer file.Close()

// 	err = encoder.Encode(file, img)
// 	if err != nil {
// 		return GenerateRouteImageResult{}, err
// 	}

// 	return GenerateRouteImageResult{
// 		Base64:    base64Str,
// 		ImagePath: options.OutputPath,
// 	}, nil
// }

// // minMax 辅助函数
// func minMax(arr []float64) (float64, float64) {
// 	if len(arr) == 0 {
// 		return 0, 0
// 	}
// 	min := arr[0]
// 	max := arr[0]
// 	for _, v := range arr {
// 		if v < min {
// 			min = v
// 		}
// 		if v > max {
// 			max = v
// 		}
// 	}
// 	return min, max
// }

// // 测试函数
// func TestGenerateRouteImage() {
// 	// 生成两组 GPS 点
// 	gpsPoints := make([][]GPSPoint, 2)

// 	// 第一组：北京到上海
// 	gpsPoints[0] = make([]GPSPoint, 500)
// 	lat := 39.9042 // 北京
// 	lng := 116.4074
// 	for i := 0; i < 500; i++ {
// 		lat -= 0.02 + rand.Float64()*0.02
// 		lng += 0.02 + rand.Float64()*0.02
// 		gpsPoints[0][i] = GPSPoint{Lat: lat, Lng: lng}
// 	}
// 	gpsPoints[0][0] = GPSPoint{Lat: 39.9042, Lng: 116.4074}   // 北京
// 	gpsPoints[0][499] = GPSPoint{Lat: 31.2304, Lng: 121.4737} // 上海

// 	// 第二组：上海到杭州
// 	gpsPoints[1] = make([]GPSPoint, 500)
// 	lat = 31.2304 // 上海
// 	lng = 121.4737
// 	for i := 0; i < 500; i++ {
// 		lat -= 0.01 + rand.Float64()*0.01
// 		lng -= 0.01 + rand.Float64()*0.01
// 		gpsPoints[1][i] = GPSPoint{Lat: lat, Lng: lng}
// 	}
// 	gpsPoints[1][0] = GPSPoint{Lat: 31.2304, Lng: 121.4737}   // 上海
// 	gpsPoints[1][499] = GPSPoint{Lat: 30.2741, Lng: 120.1551} // 杭州

// 	// 测试用本地文件路径
// 	bgFilePath := "./static/tripThumbnailBg.png"
// 	// 测试用 URL
// 	bgURL := "https://api.aiiko.club/public/images/upload/1/20250114/img_19fd1f07838688d21ab66d2f8cf98d9d.jpg"

// 	// 配置参数（用本地文件）
// 	optionsFile := GenerateRouteImageOptions{
// 		GPSPoints:    gpsPoints,
// 		LineColor:    color.RGBA{255, 0, 0, 255}, // 红色 #ff0000
// 		LineWidth:    3,
// 		BgImage:      bgFilePath,
// 		IsBgImageURL: false,
// 		PaddingX:     30, // 自定义水平边界距离
// 		PaddingY:     40, // 自定义垂直边界距离
// 		OutputPath:   "./static/route-from-file.png",
// 		Quality:      80, // 80% 质量
// 	}

// 	// 配置参数（用 URL）
// 	optionsURL := GenerateRouteImageOptions{
// 		GPSPoints:    gpsPoints,
// 		LineColor:    color.RGBA{0, 255, 0, 255}, // 绿色 #00ff00
// 		LineWidth:    5,
// 		BgImage:      bgURL,
// 		IsBgImageURL: true,
// 		PaddingX:     50, // 自定义水平边界距离
// 		PaddingY:     20, // 自定义垂直边界距离
// 		OutputPath:   "./static/route-from-url.png",
// 		Quality:      50, // 50% 质量
// 	}

// 	// 生成图片（本地文件）
// 	resultFile, err := GenerateRouteImage(optionsFile)
// 	if err != nil {
// 		log.Error("生成图片（文件）失败:", err)
// 		return
// 	}
// 	log.Info("Base64 (文件):", resultFile.Base64[:100]+"...")
// 	log.Info("图片保存到:", resultFile.ImagePath)

// 	// 生成图片（URL）
// 	resultURL, err := GenerateRouteImage(optionsURL)
// 	if err != nil {
// 		log.Error("生成图片（URL）失败:", err)
// 		return
// 	}
// 	log.Info("Base64 (URL):", resultURL.Base64[:100]+"...")
// 	log.Info("图片保存到:", resultURL.ImagePath)
// }
