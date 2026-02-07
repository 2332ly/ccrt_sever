package utils

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// AuthMiddleware 校验 JWT，并把 username 写入 ctx
func AuthMiddleware() gin.HandlerFunc {
	return func(ctx *gin.Context) {
		tokenStr := ParseBearerToken(ctx.GetHeader("Authorization"))
		if tokenStr == "" {
			RespondError(ctx, http.StatusUnauthorized, "UNAUTHORIZED", "Missing token")
			return
		}

		username, err := ValidateJWT(tokenStr)
		if err != nil {
			RespondError(ctx, http.StatusUnauthorized, "UNAUTHORIZED", "Invalid token")
			return
		}

		ctx.Set("username", username)
		ctx.Next()
	}
}
