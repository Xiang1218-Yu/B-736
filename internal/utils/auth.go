package utils

import (
  "errors"
  "time"

  "prompt736/internal/config"

  "github.com/golang-jwt/jwt/v5"
  "golang.org/x/crypto/bcrypt"
)

var jwtSecret []byte

func InitJWT(cfg config.Config) {
  jwtSecret = []byte(cfg.JWTSecret)
}

type Claims struct {
  UserID uint   `json:"user_id"`
  Role   string `json:"role"`
  jwt.RegisteredClaims
}

func GenerateToken(userID uint, role string) (string, error) {
  if len(jwtSecret) == 0 {
    return "", errors.New("jwt secret not initialized")
  }
  claims := Claims{
    UserID: userID,
    Role:   role,
    RegisteredClaims: jwt.RegisteredClaims{
      ExpiresAt: jwt.NewNumericDate(time.Now().Add(72 * time.Hour)),
      IssuedAt:  jwt.NewNumericDate(time.Now()),
    },
  }
  token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
  return token.SignedString(jwtSecret)
}

func ParseToken(tokenString string) (*Claims, error) {
  if len(jwtSecret) == 0 {
    return nil, errors.New("jwt secret not initialized")
  }
  token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) {
    return jwtSecret, nil
  })
  if err != nil {
    return nil, err
  }
  if claims, ok := token.Claims.(*Claims); ok && token.Valid {
    return claims, nil
  }
  return nil, errors.New("invalid token")
}

func HashPassword(password string) (string, error) {
  bytes, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
  return string(bytes), err
}

func CheckPassword(hash, password string) bool {
  err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
  return err == nil
}
