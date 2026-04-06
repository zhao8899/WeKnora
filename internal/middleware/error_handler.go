package middleware

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	apperrors "github.com/Tencent/WeKnora/internal/errors"
	"github.com/Tencent/WeKnora/internal/types"
)

// ErrorHandler 是一个处理应用错误的中间件
func ErrorHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		// 处理请求
		c.Next()

		// 检查是否有错误
		if len(c.Errors) > 0 {
			// 获取最后一个错误
			err := c.Errors.Last().Err

			// Check for storage quota exceeded (returns 413 Payload Too Large)
			var quotaErr *types.StorageQuotaExceededError
			if errors.As(err, &quotaErr) {
				c.JSON(http.StatusRequestEntityTooLarge, gin.H{
					"success": false,
					"error": gin.H{
						"code":    "storage_quota_exceeded",
						"message": quotaErr.Error(),
					},
				})
				return
			}

			// 检查是否为应用错误
			if appErr, ok := apperrors.IsAppError(err); ok {
				// 返回应用错误
				c.JSON(appErr.HTTPCode, gin.H{
					"success": false,
					"error": gin.H{
						"code":    appErr.Code,
						"message": appErr.Message,
						"details": appErr.Details,
					},
				})
				return
			}

			// 处理其他类型的错误
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"error": gin.H{
					"code":    apperrors.ErrInternalServer,
					"message": "Internal server error",
				},
			})
		}
	}
}
