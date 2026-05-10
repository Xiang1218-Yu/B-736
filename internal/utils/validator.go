package utils

import (
  "net/http"

  "github.com/gin-gonic/gin"
  "github.com/go-playground/validator/v10"
)

var validate = validator.New()

func BindAndValidate(c *gin.Context, payload interface{}) bool {
  if err := c.ShouldBindJSON(payload); err != nil {
    c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"message": "请求格式错误"})
    return false
  }
  if err := validate.Struct(payload); err != nil {
    c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"message": "参数校验失败", "detail": err.Error()})
    return false
  }
  return true
}
