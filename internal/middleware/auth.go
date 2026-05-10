package middleware

import (
  "net/http"
  "strings"

  "prompt736/internal/utils"

  "github.com/gin-gonic/gin"
)

func AuthRequired() gin.HandlerFunc {
  return func(c *gin.Context) {
    token := c.GetHeader("Authorization")
    if token == "" {
      c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"message": "缺少登录凭证"})
      return
    }
    token = strings.TrimPrefix(token, "Bearer ")
    claims, err := utils.ParseToken(token)
    if err != nil {
      c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"message": "登录凭证无效"})
      return
    }
    c.Set("userID", claims.UserID)
    c.Set("role", claims.Role)
    c.Next()
  }
}

func AdminRequired() gin.HandlerFunc {
  return func(c *gin.Context) {
    role := c.GetString("role")
    if role != "admin" {
      c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"message": "权限不足"})
      return
    }
    c.Next()
  }
}

func EditorRequired() gin.HandlerFunc {
  return func(c *gin.Context) {
    role := c.GetString("role")
    if role != "admin" && role != "editor" {
      c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"message": "需要发布权限"})
      return
    }
    c.Next()
  }
}
