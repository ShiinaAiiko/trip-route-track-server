package middleware

import (
	"github.com/ShiinaAiiko/nyanya-trip-route-track/server/protos"
	"github.com/ShiinaAiiko/nyanya-trip-route-track/server/services/response"
	sso "github.com/cherrai/saki-sso-go"
	"github.com/gin-gonic/gin"
	"github.com/jinzhu/copier"
)

func Params() gin.HandlerFunc {
	return func(c *gin.Context) {
		// log.Info("21231321")
		if _, isHttpServer := c.Get("isHttpServer"); !isHttpServer {
			c.Next()
			return
		}

		roles := new(RoleOptionsType)
		getRoles, isRoles := c.Get("roles")
		if isRoles {
			roles = getRoles.(*RoleOptionsType)
		}

		res := response.ResponseProtobufType{}
		res.Code = 10015

		// log.Info("roles", roles)

		if roles.RequestDataType == "protobuf" {
			data := ""
			switch c.Request.Method {
			case "GET":
				data = c.Query("data")

			case "POST":
				data = c.PostForm("data")
			default:
				break
			}
			// c.Set("data", data)
			// log.Info("data", data)

			dataProto := new(protos.RequestType)

			var err error
			if err = protos.DecodeBase64(data, dataProto); err != nil {
				res.Error = err.Error()
				res.Code = 10002
				res.Call(c)
				c.Abort()
				return
			}
			// log.Info(dataProto)
			c.Set("data", dataProto.Data)
			c.Set("token", dataProto.Token)
			c.Set("deviceId", dataProto.DeviceId)

			ua := new(sso.UserAgent)
			copier.Copy(ua, dataProto.UserAgent)
			c.Set("userAgent", ua)

			if dataProto.Open != nil {
				c.Set("openAppKey", dataProto.Open.AppKey)
				c.Set("openUserId", dataProto.Open.UserId)

			}

			c.Next()
			return
		}

		if roles.RequestDataType == "json" {
			appKey := ""
			userId := ""
			token := ""
			deviceId := ""
			switch c.Request.Method {
			case "GET":
				appKey = c.Query("appKey")
				userId = c.Query("userId")
				token = c.Query("token")
				deviceId = c.Query("deviceId")

			case "POST":
				appKey = c.PostForm("appKey")
				userId = c.PostForm("userId")
				token = c.PostForm("token")
				deviceId = c.PostForm("deviceId")
			default:
				break
			}

			c.Set("openAppKey", appKey)
			c.Set("openUserId", userId)
			c.Set("token", token)
			c.Set("deviceId", deviceId)

			c.Next()
			return
		}

		log.Info(roles.RequestDataType)

		c.Next()
	}
}
