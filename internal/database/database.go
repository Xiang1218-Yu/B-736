package database

import (
  "fmt"
  "time"

  "prompt736/internal/config"

  "gorm.io/driver/mysql"
  "gorm.io/gorm"
  "gorm.io/gorm/logger"
)

func Connect(cfg config.Config) (*gorm.DB, error) {
  dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=utf8mb4&parseTime=True&loc=Local&collation=utf8mb4_unicode_ci", cfg.DBUser, cfg.DBPassword, cfg.DBHost, cfg.DBPort, cfg.DBName)
  db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{
    Logger: logger.Default.LogMode(logger.Warn),
  })
  if err != nil {
    return nil, err
  }

  sqlDB, err := db.DB()
  if err != nil {
    return nil, err
  }
  sqlDB.SetMaxOpenConns(50)
  sqlDB.SetMaxIdleConns(10)
  sqlDB.SetConnMaxLifetime(30 * time.Minute)

  return db, nil
}
