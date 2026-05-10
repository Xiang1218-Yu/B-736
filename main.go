// Package main 是应用程序的入口
package main

import (
	"os"
	"time"

	"prompt736/internal/app"
	"prompt736/internal/config"
	"prompt736/internal/database"
	"prompt736/internal/i18n"
	"prompt736/internal/models"
	"prompt736/internal/router"
	"prompt736/internal/seed"
	"prompt736/internal/utils"

	"github.com/patrickmn/go-cache"
	"github.com/sirupsen/logrus"
)

func main() {
	// 加载配置
	cfg := config.Load()

	// 初始化日志
	logger := logrus.New()
	logger.SetFormatter(&logrus.JSONFormatter{TimestampFormat: time.RFC3339})
	logger.SetOutput(os.Stdout)

	// 初始化 JWT
	utils.InitJWT(cfg)

	// 连接数据库
	db, err := database.Connect(cfg)
	if err != nil {
		logger.WithError(err).Fatal("database connection failed")
	}

	// 数据库迁移
	if err := db.AutoMigrate(
		&models.User{},
		&models.Category{},
		&models.Tag{},
		&models.Resource{},
		&models.Article{},
		&models.Comment{},
		&models.SiteConfig{},
		&models.ShareLog{},
		&models.ResourceTag{},
	); err != nil {
		logger.WithError(err).Fatal("auto migrate failed")
	}

	// 初始化种子数据
	if err := seed.Ensure(db); err != nil {
		logger.WithError(err).Fatal("seed failed")
	}

	// 初始化缓存
	cacheStore := cache.New(2*time.Minute, 5*time.Minute)

	// 初始化国际化
	bundle := i18n.New("zh")
	if err := bundle.LoadDir("locales"); err != nil {
		logger.WithError(err).Fatal("load locales failed")
	}

	// 加载模板
	tmpl := app.LoadTemplates()

	// 初始化 App 实例
	application := &app.App{
		DB:        db,
		Cache:     cacheStore,
		Cfg:       cfg,
		Log:       logger,
		Templates: tmpl,
		I18n:      bundle,
	}

	// 设置路由
	r := router.SetupRouter(application, cfg, logger)

	// 启动服务器
	if err := r.Run(":8080"); err != nil {
		logger.WithError(err).Fatal("server start failed")
	}
}
