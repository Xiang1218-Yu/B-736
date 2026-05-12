package main

import (
	"html/template"
	"os"
	"time"

	"prompt736/internal/config"
	"prompt736/internal/database"
	"prompt736/internal/handler"
	"prompt736/internal/i18n"
	"prompt736/internal/models"
	"prompt736/internal/router"
	"prompt736/internal/seed"
	"prompt736/internal/service"
	"prompt736/internal/utils"

	"github.com/gin-gonic/gin"
	"github.com/patrickmn/go-cache"
	"github.com/sirupsen/logrus"
)

// main 应用程序入口
// 负责初始化所有核心组件并启动 HTTP 服务器
// 初始化流程：配置 -> 日志 -> JWT -> 数据库 -> 数据迁移 -> 种子数据
//             -> 缓存 -> 国际化 -> 应用实例 -> 处理器 -> 路由 -> 启动服务
func main() {
	cfg := config.Load()
	logger := logrus.New()
	logger.SetFormatter(&logrus.JSONFormatter{TimestampFormat: time.RFC3339})
	logger.SetOutput(os.Stdout)

	utils.InitJWT(cfg)

	db, err := database.Connect(cfg)
	if err != nil {
		logger.WithError(err).Fatal("database connection failed")
	}

	if err := db.AutoMigrate(&models.User{}, &models.Category{}, &models.Tag{}, &models.Resource{}, &models.Article{}, &models.Comment{}, &models.SiteConfig{}, &models.ShareLog{}, &models.ResourceTag{}); err != nil {
		logger.WithError(err).Fatal("auto migrate failed")
	}

	if err := seed.Ensure(db); err != nil {
		logger.WithError(err).Fatal("seed failed")
	}

	cacheStore := cache.New(2*time.Minute, 5*time.Minute)

	bundle := i18n.New("zh")
	if err := bundle.LoadDir("locales"); err != nil {
		logger.WithError(err).Fatal("load locales failed")
	}

	tmpl := template.New("")

	app := service.NewApp(db, cacheStore, cfg, logger, tmpl, bundle)

	h := handler.NewHandler(app)

	r := gin.New()

	router.SetupRouter(r, app, h, cfg, logger)

	if err := r.Run(":8080"); err != nil {
		logger.WithError(err).Fatal("server start failed")
	}
}
