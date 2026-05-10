package main

import (
	"os"
	"time"

	"prompt736/internal/app"
	"prompt736/internal/config"
	"prompt736/internal/database"
	"prompt736/internal/handler"
	"prompt736/internal/i18n"
	"prompt736/internal/models"
	"prompt736/internal/router"
	"prompt736/internal/seed"
	"prompt736/internal/service"
	tmplPkg "prompt736/internal/template"
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

	// 自动迁移
	if err := db.AutoMigrate(&models.User{}, &models.Category{}, &models.Tag{}, &models.Resource{}, &models.Article{}, &models.Comment{}, &models.SiteConfig{}, &models.ShareLog{}, &models.ResourceTag{}); err != nil {
		logger.WithError(err).Fatal("auto migrate failed")
	}

	// 种子数据
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
	tmpl := tmplPkg.Load()

	// 创建应用核心实例
	application := app.New(db, cacheStore, cfg, logger, tmpl, bundle)

	// 创建服务层
	svc := service.New(application)

	// 创建处理器
	h := handler.New(application, svc)
	ah := handler.NewAdminHandler(application, svc, h)

	// 初始化路由
	engine := router.Setup(application, svc, h, ah, logger)

	// 启动服务
	if err := engine.Run(":8080"); err != nil {
		logger.WithError(err).Fatal("server start failed")
	}
}
