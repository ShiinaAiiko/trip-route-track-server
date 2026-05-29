package middleware

import (
	"errors"

	conf "github.com/ShiinaAiiko/nyanya-trip-route-track/server/config"
	"github.com/ShiinaAiiko/nyanya-trip-route-track/server/services/response"
	"github.com/cherrai/nyanyago-utils/cipher"
	"github.com/gin-gonic/gin"
)

func Open() gin.HandlerFunc {
	return func(c *gin.Context) {
		// if !c.GetBool("isOpenApi") {
		// 	c.Next()
		// 	return
		// }
		roles := new(RoleOptionsType)
		getRoles, isRoles := c.Get("roles")
		// log.Info("21231321", getRoles)
		if isRoles {
			roles = getRoles.(*RoleOptionsType)
		}
		if !roles.CheckApp {
			c.Next()
			return
		}
		// log.Info("21231321", getRoles)
		c.Set("isOpenApi", true)

		res := response.ResponseProtobufType{}
		res.Code = 10014

		openAppKey := c.GetString("openAppKey")
		openUserId := c.GetString("openUserId")

		// 校验AppKey是否存在
		if !conf.CheckAppKey(openAppKey) {
			res.Code = 10014
			res.Call(c)
			c.Abort()
			return
		}

		// 解密UID
		aes := cipher.AES{
			Key: openAppKey,
		}
		deUid, deUidErr := aes.DecryptWithString(openUserId, "")
		if deUidErr != nil {
			res.Errors(errors.New("UID decryption failed"))
			res.Code = 10002
			res.Call(c)
			c.Abort()
			return
		}
		deUidStr := deUid.Trim(`"`)

		c.Set("openUid", deUidStr)

		// log.Info("deUid", deUid.HexEncodeToString())
		// log.Info("deUid", deUid.Trim(`"`))

		// log.Info("ccccccccccccccccccccccc", openData, openData == nil)
		// log.Info("ccccccccccccccccccccccc", openData.AppKey)
		// log.Info("ccccccccccccccccccccccc", openData.Uid)
		// log.Info("ccccccccccccccccccccccc", deUidStr)

		c.Next()
	}
}
