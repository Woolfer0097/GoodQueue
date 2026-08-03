package identity

import (
	"strings"

	"github.com/Woolfer0097/GoodQueue/internal/pkg/domain"
	"github.com/gin-gonic/gin"
)

const HeaderName = "X-User-ID"
const contextKey = "goodqueue.external-user-id"

func Optional() gin.HandlerFunc {
	return func(c *gin.Context) {
		rawUserID := c.GetHeader(HeaderName)
		if rawUserID != "" {
			userID, err := domain.ParseExternalUserID(rawUserID)
			if err == nil {
				c.Set(contextKey, userID)
			}
		}
		c.Next()
	}
}

func FromContext(c *gin.Context) (domain.ExternalUserID, bool) {
	value, exists := c.Get(contextKey)
	if !exists {
		return "", false
	}
	userID, valid := value.(domain.ExternalUserID)
	return userID, valid
}

func RawHeader(c *gin.Context) string {
	return strings.TrimSpace(c.GetHeader(HeaderName))
}
