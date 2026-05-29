package middleware

import (
	"net/http"

	"github.com/ShiinaAiiko/nyanya-trip-route-track/server/protos"
	"github.com/ShiinaAiiko/nyanya-trip-route-track/server/services/response"
	"github.com/gin-gonic/gin"
)

func Response() gin.HandlerFunc {
	return func(c *gin.Context) {
		if _, isStaticServer := c.Get("isStaticServer"); isStaticServer {
			c.Next()
			return
		}
		if _, isWsServer := c.Get("WsServer"); isWsServer {
			c.Next()
			return
		}
		roles := new(RoleOptionsType)
		getRoles, isRoles := c.Get("roles")

		if isRoles {
			roles = getRoles.(*RoleOptionsType)
		}
		//  else {
		// 	// res := response.ResponseType{}
		// 	// res.Code = 10013
		// 	// c.JSON(http.StatusOK, res.GetResponse())
		// 	// return
		// }
		if isRoles && roles.isHttpServer {
			defer func() {
				roles := c.MustGet("roles").(*RoleOptionsType)
				// Log.Info("Response middleware", roles.ResponseEncryption)
				if roles.isHttpServer {

					var r *response.ResponseType
					getProtobufDataResponse, _ := c.Get("protobuf")
					if getProtobufDataResponse != nil {
						r = getProtobufDataResponse.(*response.ResponseType)

					} else {
						getBodyDataResponse, _ := c.Get("json")

						// log.Info("getBodyDataResponse", getBodyDataResponse)
						r = getBodyDataResponse.(*response.ResponseType)

					}

					if roles.ResponseEncryption {
						// if getBodyDataResponse == nil {
						// 	res.Code = 10001
						// 	c.JSON(http.StatusOK, res.Encryption(userAesKey, res))
						// } else {
						// 	// 当需要加密的时候
						// 	c.JSON(http.StatusOK, res.Encryption(userAesKey, getProtobufDataResponse))
						// }
					}

					switch roles.ResponseDataType {
					case "protobuf":
						res := &protos.ResponseType{
							Code:        r.Code,
							Data:        r.Data.(string),
							Msg:         r.Msg,
							CnMsg:       r.CnMsg,
							Error:       r.Error,
							RequestId:   r.RequestId,
							RequestTime: r.RequestTime,
							Platform:    r.Platform,
							Author:      r.Author,
						}

						protoData := protos.Encode(res)
						c.Writer.Header().Set("Content-Type", "application/x-protobuf")
						c.String(http.StatusOK,
							protoData)

					case "json":
						c.JSON(http.StatusOK, r)
					}
				}
			}()
			c.Next()
		} else {
			c.Next()
		}
	}
}
