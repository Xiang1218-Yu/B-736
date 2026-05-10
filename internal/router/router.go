// Package router 提供路由注册功能
package router

import (
	"net/http"
	"strings"

	"prompt736/internal/app"
	"prompt736/internal/config"

	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
)

// SetupRouter 配置并返回 Gin 路由引擎
func SetupRouter(app *app.App, cfg config.Config, logger *logrus.Logger) *gin.Engine {
	router := gin.New()
	router.Use(gin.Recovery())
	router.Use(gin.LoggerWithFormatter(func(param gin.LogFormatterParams) string {
		logger.WithFields(logrus.Fields{
			"status":  param.StatusCode,
			"method":  param.Method,
			"path":    param.Path,
			"latency": param.Latency.String(),
			"ip":      param.ClientIP,
		}).Info("http_request")
		return ""
	}))

	// Session 中间件
	store := cookie.NewStore([]byte(cfg.JWTSecret))
	store.Options(sessions.Options{
		Path:     "/",
		MaxAge:   86400 * 7,
		HttpOnly: true,
		Secure:   strings.HasPrefix(strings.ToLower(cfg.BaseURL), "https://"),
		SameSite: http.SameSiteLaxMode,
	})
	router.Use(sessions.Sessions("session", store))
	router.Use(app.SecurityHeadersMiddleware())

	// 静态文件
	router.Static("/static", "./static")
	router.Static("/uploads", cfg.UploadDir)

	// 通用中间件
	router.Use(app.CommonMiddleware())
	router.Use(app.CSRFMiddleware())

	// 公共页面路由
	registerPublicRoutes(router, app)

	// 需要认证的路由
	registerAuthRoutes(router, app)

	// 管理后台路由
	registerAdminRoutes(router, app)

	return router
}

// registerPublicRoutes 注册公共页面路由
func registerPublicRoutes(router *gin.Engine, app *app.App) {
	// 首页
	router.GET("/", app.HomePage)
	router.HEAD("/", app.HomePage)

	// 资源相关
	router.GET("/resources", app.ResourcesPage)
	router.HEAD("/resources", app.ResourcesPage)
	router.GET("/resources/:id", app.ResourceDetailPage)
	router.HEAD("/resources/:id", app.ResourceDetailPage)

	// 文章相关
	router.GET("/articles", app.ArticlesPage)
	router.HEAD("/articles", app.ArticlesPage)
	router.GET("/articles/:id", app.ArticleDetailPage)
	router.HEAD("/articles/:id", app.ArticleDetailPage)

	// 认证相关
	router.GET("/login", app.LoginPage)
	router.HEAD("/login", app.LoginPage)
	router.POST("/login", app.LoginHandler)
	router.GET("/register", app.RegisterPage)
	router.HEAD("/register", app.RegisterPage)
	router.POST("/register", app.RegisterHandler)
	router.GET("/logout", app.LogoutHandler)
}

// registerAuthRoutes 注册需要认证的路由
func registerAuthRoutes(router *gin.Engine, app *app.App) {
	auth := router.Group("")
	auth.Use(app.AuthRequired())
	{
		// 个人中心
		auth.GET("/profile", app.ProfilePage)
		auth.POST("/checkin", app.CheckinHandler)

		// 资源操作
		auth.POST("/resources/:id/share", app.ShareHandler)
		auth.POST("/resources/:id/comments", app.ResourceCommentHandler)

		// 文章操作
		auth.POST("/articles/:id/comments", app.ArticleCommentHandler)

		// 资源发布
		publish := auth.Group("/publish")
		publish.Use(app.PublishAuthorized())
		{
			publish.GET("", app.PublishResourcePage)
			publish.GET("/template.csv", app.PublishTemplateCSV)
			publish.POST("/manual", app.PublishManualHandler)
			publish.POST("/import", app.PublishImportHandler)
			publish.POST("/crawler", app.PublishCrawlerHandler)
		}
	}
}

// registerAdminRoutes 注册管理后台路由
func registerAdminRoutes(router *gin.Engine, app *app.App) {
	admin := router.Group("/admin")
	admin.Use(app.AuthRequired(), app.AdminRequired())
	{
		// 仪表盘
		admin.GET("", app.AdminDashboard)

		// 用户管理
		admin.GET("/users", app.AdminUsersPage)
		admin.POST("/users/:id", app.AdminUpdateUser)

		// 资源管理
		admin.GET("/resources", app.AdminResourcesPage)
		admin.POST("/resources/:id/review", app.AdminReviewResource)

		// 文章管理
		admin.GET("/articles", app.AdminArticlesPage)
		admin.GET("/articles/new", app.AdminArticleFormPage)
		admin.POST("/articles", app.AdminCreateArticle)
		admin.GET("/articles/:id/edit", app.AdminArticleFormPage)
		admin.POST("/articles/:id", app.AdminUpdateArticle)

		// 分类管理
		admin.GET("/categories", app.AdminCategoriesPage)
		admin.POST("/categories", app.AdminCreateCategory)
		admin.POST("/categories/:id", app.AdminUpdateCategory)
		admin.POST("/categories/:id/delete", app.AdminDeleteCategory)

		// 标签管理
		admin.GET("/tags", app.AdminTagsPage)
		admin.POST("/tags", app.AdminCreateTag)
		admin.POST("/tags/:id", app.AdminUpdateTag)
		admin.POST("/tags/:id/delete", app.AdminDeleteTag)

		// 设置
		admin.GET("/settings", app.AdminSettingsPage)
		admin.POST("/settings", app.AdminUpdateSettings)

		// 备份
		admin.POST("/backup", app.AdminBackup)
		admin.POST("/restore", app.AdminRestore)
	}
}
