package router

import (
	"net/http"
	"strings"

	"prompt736/internal/app"
	"prompt736/internal/handler"
	"prompt736/internal/middleware"
	"prompt736/internal/service"

	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
)

// Setup 初始化 Gin 引擎，注册所有路由和中间件，返回可运行的引擎实例
func Setup(a *app.App, svc *service.Service, h *handler.Handler, ah *handler.AdminHandler, logger *logrus.Logger) *gin.Engine {
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(gin.LoggerWithFormatter(func(param gin.LogFormatterParams) string {
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
	store := cookie.NewStore([]byte(a.Cfg.JWTSecret))
	store.Options(sessions.Options{
		Path:     "/",
		MaxAge:   86400 * 7,
		HttpOnly: true,
		Secure:   strings.HasPrefix(strings.ToLower(a.Cfg.BaseURL), "https://"),
		SameSite: http.SameSiteLaxMode,
	})
	r.Use(sessions.Sessions("session", store))

	// 安全头中间件
	r.Use(middleware.SecurityHeadersMiddleware())

	// 静态文件
	r.Static("/static", "./static")
	r.Static("/uploads", a.Cfg.UploadDir)

	// 公共数据注入 + CSRF 中间件
	r.Use(middleware.CommonMiddleware(a, svc))
	r.Use(middleware.CSRFMiddleware(a))

	// ---- 公开路由 ----
	r.GET("/", h.HomePage)
	r.HEAD("/", h.HomePage)
	r.GET("/resources", h.ResourcesPage)
	r.HEAD("/resources", h.ResourcesPage)
	r.GET("/resources/:id", h.ResourceDetailPage)
	r.HEAD("/resources/:id", h.ResourceDetailPage)
	r.GET("/articles", h.ArticlesPage)
	r.HEAD("/articles", h.ArticlesPage)
	r.GET("/articles/:id", h.ArticleDetailPage)
	r.HEAD("/articles/:id", h.ArticleDetailPage)
	r.GET("/login", h.LoginPage)
	r.HEAD("/login", h.LoginPage)
	r.POST("/login", h.LoginHandler)
	r.GET("/register", h.RegisterPage)
	r.HEAD("/register", h.RegisterPage)
	r.POST("/register", h.RegisterHandler)
	r.GET("/logout", h.LogoutHandler)

	// ---- 需要登录的路由 ----
	auth := r.Group("")
	auth.Use(middleware.AuthRequired(a))
	auth.GET("/profile", h.ProfilePage)
	auth.POST("/checkin", h.CheckinHandler)
	auth.POST("/resources/:id/share", h.ShareHandler)
	auth.POST("/resources/:id/comments", h.ResourceCommentHandler)
	auth.POST("/articles/:id/comments", h.ArticleCommentHandler)

	// ---- 需要发布权限的路由 ----
	publish := auth.Group("/publish")
	publish.Use(middleware.PublishAuthorized(a, svc))
	publish.GET("", h.PublishResourcePage)
	publish.GET("/template.csv", h.PublishTemplateCSV)
	publish.POST("/manual", h.PublishManualHandler)
	publish.POST("/import", h.PublishImportHandler)
	publish.POST("/crawler", h.PublishCrawlerHandler)

	// ---- 管理后台路由 ----
	admin := r.Group("/admin")
	admin.Use(middleware.AuthRequired(a), middleware.AdminRequired(a))
	admin.GET("", ah.AdminDashboard)
	admin.GET("/users", ah.AdminUsersPage)
	admin.POST("/users/:id", ah.AdminUpdateUser)
	admin.GET("/resources", ah.AdminResourcesPage)
	admin.POST("/resources/:id/review", ah.AdminReviewResource)
	admin.GET("/articles", ah.AdminArticlesPage)
	admin.GET("/articles/new", ah.AdminArticleFormPage)
	admin.POST("/articles", ah.AdminCreateArticle)
	admin.GET("/articles/:id/edit", ah.AdminArticleFormPage)
	admin.POST("/articles/:id", ah.AdminUpdateArticle)
	admin.GET("/categories", ah.AdminCategoriesPage)
	admin.POST("/categories", ah.AdminCreateCategory)
	admin.POST("/categories/:id", ah.AdminUpdateCategory)
	admin.POST("/categories/:id/delete", ah.AdminDeleteCategory)
	admin.GET("/tags", ah.AdminTagsPage)
	admin.POST("/tags", ah.AdminCreateTag)
	admin.POST("/tags/:id", ah.AdminUpdateTag)
	admin.POST("/tags/:id/delete", ah.AdminDeleteTag)
	admin.GET("/settings", ah.AdminSettingsPage)
	admin.POST("/settings", ah.AdminUpdateSettings)
	admin.POST("/backup", ah.AdminBackup)
	admin.POST("/restore", ah.AdminRestore)

	return r
}
