package middleware

import (
	"fmt"

	"github.com/Woolfer0097/GoodQueue/internal/pkg/domain"
	"github.com/gin-gonic/gin"
	"github.com/samber/oops"
)

func Recovery() gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			panicValue := recover()
			if panicValue == nil {
				return
			}
			_ = c.Error(oops.Code("panic").With("panic", fmt.Sprint(panicValue)).Wrap(domain.ErrInternal))
			c.Abort()
		}()
		c.Next()
	}
}
