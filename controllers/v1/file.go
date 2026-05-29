package controllersV1

import (
	"strings"
	"time"

	conf "github.com/ShiinaAiiko/nyanya-trip-route-track/server/config"
	"github.com/ShiinaAiiko/nyanya-trip-route-track/server/protos"
	"github.com/ShiinaAiiko/nyanya-trip-route-track/server/services/response"
	"github.com/cherrai/nyanyago-utils/cipher"
	"github.com/cherrai/nyanyago-utils/saass"
	"github.com/cherrai/nyanyago-utils/validation"
	sso "github.com/cherrai/saki-sso-go"
	"github.com/gin-gonic/gin"
)

var ()

type FileController struct {
}

func (fc *FileController) GetUploadToken(c *gin.Context) {
	// 1、请求体
	var res response.ResponseProtobufType
	res.Code = 200

	// 2、获取参数
	data := new(protos.GetUploadToken_Request)
	var err error
	if err = protos.DecodeBase64(c.GetString("data"), data); err != nil {
		res.Error = err.Error()
		res.Code = 10002
		res.Call(c)
		return
	}

	// 3、验证参数
	if err = validation.ValidateStruct(
		data,
		validation.Parameter(&data.FileInfo, validation.Required()),
	); err != nil {
		res.Errors(err)
		res.Code = 10002
		res.Call(c)
		return
	}

	if err = validation.ValidateStruct(
		data.FileInfo,
		validation.Parameter(&data.FileInfo.Name, validation.Type("string"), validation.Required()),
		validation.Parameter(&data.FileInfo.Size, validation.Type("int64"), validation.Required()),
		validation.Parameter(&data.FileInfo.Type, validation.Type("string"), validation.Required()),
		validation.Parameter(&data.FileInfo.Suffix, validation.Type("string"), validation.Required()),
		validation.Parameter(&data.FileInfo.LastModified, validation.Type("int64"), validation.Required()),
		validation.Parameter(&data.FileInfo.Hash, validation.Type("string"), validation.Required()),
	); err != nil {
		res.Errors(err)
		res.Code = 10002
		res.Call(c)
		return
	}

	authorId := c.MustGet("userInfo").(*sso.UserInfo).Uid

	// 4、获取Token
	chunkSize := int64(128 * 1024)

	if data.FileInfo.Size < 1024*1024 {
		chunkSize = 128 * 1024
	}
	if data.FileInfo.Size > 1024*1024 {
		chunkSize = 256 * 1024
	}
	if data.FileInfo.Size > 15*1024*1024 {
		chunkSize = 512 * 1024
	}

	fileName := strings.ToLower(cipher.MD5(
		data.FileInfo.Hash)) + data.FileInfo.Suffix

	if strings.Contains(data.FileInfo.Name, "backup_trip") {
		fileName = data.FileInfo.Name + data.FileInfo.Suffix
	}

	ut, err := conf.SAaSS.CreateChunkUploadToken(&saass.CreateUploadTokenOptions{
		FileInfo: &saass.FileInfo{
			Name:         data.FileInfo.Name,
			Size:         data.FileInfo.Size,
			Type:         data.FileInfo.Type,
			Suffix:       data.FileInfo.Suffix,
			LastModified: data.FileInfo.LastModified,
			Hash:         data.FileInfo.Hash,
		},
		// Path: "/trip/files/" + time.Now().Format("2006/01/02") + "/",
		// FileName: strings.ToLower(cipher.MD5(
		// 	data.FileInfo.Hash+nstrings.ToString(data.FileInfo.Size)+nstrings.ToString(time.Now().Unix()))) + data.FileInfo.Suffix,
		Path:             "/trip/files/",
		FileName:         fileName,
		ChunkSize:        chunkSize,
		VisitCount:       -1,
		ExpirationTime:   time.Now().AddDate(0, 0, 180).Unix(),
		AutoExtendPeriod: 60 * 60 * 24 * 180,
		// Type:           "File",
		FileConflict: "Replace",

		AllowShare: 1,
		RootPath:   conf.SAaSS.GenerateRootPath(authorId),
		UserId:     authorId,
		ShareUsers: []string{"AllUser"},

		OnProgress: func(progress saass.Progress) {
			// log.Info("progress", progress)
		},
		OnSuccess: func(urls saass.Urls) {
			// log.Info("urls", urls)
		},
		OnError: func(err error) {
			// log.Info("err", err)
		},
	})
	if err != nil {
		res.Errors(err)
		res.Code = 10019
		res.Call(c)
		return
	}
	urls := protos.Urls{
		DomainUrl: ut.Urls.DomainUrl,
		ShortUrl:  ut.Urls.ShortUrl,
		Url:       ut.Urls.Url,
	}
	log.Info("ChunkSize", conf.Config.Saass.AppId, ut)
	// log.Info("ChunkSize", ut.ChunkSize, chunkSize)
	protoData := &protos.GetUploadToken_Response{
		Urls:           &urls,
		ApiUrl:         ut.ApiUrl,
		Token:          ut.Token,
		ChunkSize:      ut.ChunkSize,
		UploadedOffset: ut.UploadedOffset,
	}

	res.Data = protos.Encode(protoData)

	res.Call(c)
}

func (fc *FileController) GetAppToken(c *gin.Context) {
	// 1、请求体
	var res response.ResponseProtobufType
	res.Code = 200

	log.Info("GetAppToken")
	// 2、获取参数
	data := new(protos.GetAppToken_Request)
	var err error
	if err = protos.DecodeBase64(c.GetString("data"), data); err != nil {
		res.Error = err.Error()
		res.Code = 10002
		res.Call(c)
		return
	}

	// 3、验证参数

	u, isExists := c.Get("userInfo")
	if !isExists {
		res.Code = 10002
		res.Call(c)
		return
	}
	userInfo := u.(*sso.UserInfo)
	authorId := userInfo.Uid

	// // 4、获取Token
	// chunkSize := int64(128 * 1024)

	// log.Info("ChunkSize", ut)
	// log.Info("ChunkSize", ut.ChunkSize, chunkSize)
	log.Info(conf.SAaSS.GenerateRootPath(authorId), authorId)
	token, err := conf.SAaSS.GetAppToken(conf.SAaSS.GenerateRootPath(authorId), authorId)
	log.Info(token, err)
	if err != nil {
		res.Error = err.Error()
		res.Code = 10018
		res.Call(c)
		return
	}
	protoData := &protos.GetAppToken_Response{
		BaseUrl:  conf.Config.Saass.BaseUrl,
		AppToken: token.Token,
		Deadline: token.Deadline,
	}

	res.Data = protos.Encode(protoData)

	res.Call(c)
}
