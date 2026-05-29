package controllersV1

import (
	conf "github.com/ShiinaAiiko/nyanya-trip-route-track/server/config"
	"github.com/ShiinaAiiko/nyanya-trip-route-track/server/protos"
	"github.com/ShiinaAiiko/nyanya-trip-route-track/server/services/response"
	"github.com/cherrai/nyanyago-utils/validation"
	sso "github.com/cherrai/saki-sso-go"
	"github.com/gin-gonic/gin"
	"github.com/mitchellh/mapstructure"
)

// "github.com/cherrai/nyanyago-utils/validation"
var ()

type ConfigureController struct {
}

func (fc *ConfigureController) SyncConfigure(c *gin.Context) {
	// 1、请求体
	var res response.ResponseProtobufType
	res.Code = 200

	// 2、获取参数
	data := new(protos.SyncConfigure_Request)
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
		validation.Parameter(&data.Configure, validation.Required()),
	); err != nil {
		res.Errors(err)
		res.Code = 10002
		res.Call(c)
		return
	}

	userAgentAny, exists := c.Get("userAgent")
	if !exists {
		res.Errors(err)
		res.Code = 10004
		res.Call(c)
		return
	}
	userAgent := userAgentAny.(*sso.UserAgent)

	token := c.GetString("token")
	deviceId := c.GetString("deviceId")

	conf.SSO.AppData.Set(
		"configure",
		data.Configure,
		token, deviceId, userAgent)

	protoData := &protos.SyncConfigure_Response{}

	res.Data = protos.Encode(protoData)

	res.Call(c)
}

func (fc *ConfigureController) GetConfigure(c *gin.Context) {
	// 1、请求体
	var res response.ResponseProtobufType
	res.Code = 200

	// 2、获取参数
	data := new(protos.GetConfigure_Request)
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
	); err != nil {
		res.Errors(err)
		res.Code = 10002
		res.Call(c)
		return
	}

	userAgentAny, exists := c.Get("userAgent")
	if !exists {
		res.Errors(err)
		res.Code = 10004
		res.Call(c)
		return
	}
	userAgent := userAgentAny.(*sso.UserAgent)

	token := c.GetString("token")
	deviceId := c.GetString("deviceId")

	oldConfigureAny, err :=
		conf.SSO.AppData.Get(
			"configure",
			token, deviceId, userAgent)

		// 比较谁新谁久

	// conf.SSO.AppData.Get()

	oldConfigure := new(protos.Configure)

	if err == nil && oldConfigureAny != nil {
		mapstructure.Decode(oldConfigureAny, oldConfigure)
	}

	protoData := &protos.GetConfigure_Response{
		Configure: oldConfigure,
	}

	res.Data = protos.Encode(protoData)

	res.Call(c)
}
