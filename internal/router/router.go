package router

import (
	"net/http"
	"strings"

	"prompt736/internal/config"
	"prompt736/internal/handler"
	"prompt736/internal/models"
	"prompt736/internal/service"
	"prompt736/internal/types"
	"prompt736/internal/utils"

	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
)

// SetupRouter 设置路由
func SetupRouter(router *gin.Engine, app *service.App, h *handler.Handler, cfg config.Config, logger *logrus.Logger) {
	setupMiddleware(router, app, cfg, logger)
	setupRoutes(router, h)
}

// setupMiddleware 设置中间件
func setupMiddleware(router *gin.Engine, app *service.App, cfg config.Config, logger *logrus.Logger) {
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

	store := cookie.NewStore([]byte(cfg.JWTSecret))
	store.Options(sessions.Options{
		Path:     "/",
		MaxAge:   86400 * 7,
		HttpOnly: true,
		Secure:   strings.HasPrefix(strings.ToLower(cfg.BaseURL), "https://"),
		SameSite: http.SameSiteLaxMode,
	})
	router.Use(sessions.Sessions("session", store))
	router.Use(securityHeadersMiddleware(cfg))
	router.Use(commonMiddleware(app))
	router.Use(csrfMiddleware(app))
}

// securityHeadersMiddleware 安全头中间件
func securityHeadersMiddleware(cfg config.Config) gin.HandlerFunc {
	csp := strings.Join([]string{
		"default-src 'self'",
		"script-src 'self'",
		"style-src 'self' 'unsafe-inline' https://fonts.googleapis.com",
		"font-src 'self' https://fonts.gstatic.com data:",
		"img-src 'self' data: https:",
		"connect-src 'self'",
		"form-action 'self'",
		"base-uri 'self'",
		"frame-ancestors 'self'",
		"object-src 'none'",
	}, "; ")

	return func(c *gin.Context) {
		c.Header("X-Frame-Options", "SAMEORIGIN")
		c.Header("X-Content-Type-Options", "nosniff")
		c.Header("Referrer-Policy", "strict-origin-when-cross-origin")
		c.Header("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		c.Header("Content-Security-Policy", csp)
		if c.Request.TLS != nil || strings.EqualFold(c.GetHeader("X-Forwarded-Proto"), "https") {
			c.Header("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		}
		c.Next()
	}
}

// commonMiddleware 通用数据中间件
func commonMiddleware(app *service.App) gin.HandlerFunc {
	return func(c *gin.Context) {
		session := sessions.Default(c)

		var siteConfig models.SiteConfig
		if cachedConfig, err := app.GetSiteConfig(); err == nil {
			siteConfig = cachedConfig
		}
		c.Set("siteConfig", siteConfig)

		defaultLocale := app.I18n.Resolve(siteConfig.LocaleDefault, "zh")
		cookieLocale, _ := c.Cookie("lang")
		localeCandidate := strings.TrimSpace(c.Query("lang"))
		if localeCandidate == "" {
			localeCandidate = cookieLocale
		}
		locale := app.I18n.Resolve(localeCandidate, defaultLocale)
		c.Set("locale", locale)
		if cookieLocale != locale {
			c.SetCookie("lang", locale, 86400*30, "/", "", false, false)
		}
		ensureCSRFToken(c, app)

		if userID, ok := session.Get("userID").(uint); ok && userID > 0 {
			var user models.User
			if err := app.DB.First(&user, userID).Error; err == nil {
				c.Set("user", &user)
			}
		}

		if flash := session.Get("flash_success"); flash != nil {
			c.Set("flash_success", flash)
			session.Delete("flash_success")
			session.Save()
		}
		if flash := session.Get("flash_error"); flash != nil {
			c.Set("flash_error", flash)
			session.Delete("flash_error")
			session.Save()
		}

		c.Next()
	}
}

// ensureCSRFToken 确保CSRF令牌存在
func ensureCSRFToken(c *gin.Context, app *service.App) string {
	if token := c.GetString("csrfToken"); token != "" {
		return token
	}

	session := sessions.Default(c)
	if token, ok := session.Get(types.CSRFSessionKey).(string); ok && token != "" {
		c.Set("csrfToken", token)
		return token
	}

	token := utils.GenerateSecureToken(32)
	session.Set(types.CSRFSessionKey, token)
	_ = session.Save()
	c.Set("csrfToken", token)
	return token
}

// csrfMiddleware CSRF保护中间件
func csrfMiddleware(app *service.App) gin.HandlerFunc {
	return func(c *gin.Context) {
		if utils.IsSafeHTTPMethod(c.Request.Method) {
			c.Next()
			return
		}

		session := sessions.Default(c)
		sessionToken, _ := session.Get(types.CSRFSessionKey).(string)
		if sessionToken == "" {
			sessionToken = ensureCSRFToken(c, app)
		}

		requestToken := strings.TrimSpace(c.PostForm(types.CSRFFormField))
		if requestToken == "" {
			requestToken = strings.TrimSpace(c.GetHeader("X-CSRF-Token"))
		}

		if !utils.SecureStringEqual(sessionToken, requestToken) {
			setFlashKey(c, app, "flash_error", "flash.invalid_csrf")
			redirectBackOrDefault(c, "/")
			c.Abort()
			return
		}

		c.Next()
	}
}

// setFlashKey 设置Flash消息
func setFlashKey(c *gin.Context, app *service.App, flashKey, i18nKey string, args ...interface{}) {
	session := sessions.Default(c)
	locale := c.GetString("locale")
	if locale == "" {
		locale = "zh"
	}
	session.Set(flashKey, app.I18n.T(locale, i18nKey, args...))
	session.Save()
}

// redirectBackOrDefault 重定向回来源页面或默认页面
func redirectBackOrDefault(c *gin.Context, fallback string) {
	ref := strings.TrimSpace(c.Request.Referer())
	if ref == "" {
		c.Redirect(http.StatusFound, fallback)
		return
	}

	c.Redirect(http.StatusFound, ref)
}

// AuthRequired 认证中间件
func AuthRequired() gin.HandlerFunc {
	return func(c *gin.Context) {
		if _, exists := c.Get("user"); !exists {
			c.Redirect(http.StatusFound, "/login")
			c.Abort()
			return
		}
		c.Next()
	}
}

// AdminRequired 管理员权限中间件
func AdminRequired() gin.HandlerFunc {
	return func(c *gin.Context) {
		user, ok := c.Get("user")
		if !ok {
			c.Redirect(http.StatusFound, "/")
			c.Abort()
			return
		}
		u, ok := user.(*models.User)
		if !ok || u.Role != "admin" {
			c.Redirect(http.StatusFound, "/")
			c.Abort()
			return
		}
		c.Next()
	}
}

// PublishAuthorized 发布权限中间件
func PublishAuthorized() gin.HandlerFunc {
	return func(c *gin.Context) {
		user, ok := c.Get("user")
		if !ok {
			c.Redirect(http.StatusFound, "/resources")
			c.Abort()
			return
		}
		u, ok := user.(*models.User)
		if !ok || !utils.CanPublishResource(u, types.PublishMinPoints) {
			c.Redirect(http.StatusFound, "/resources")
			c.Abort()
			return
		}
		c.Next()
	}
}

// setupRoutes 设置路由
func setupRoutes(router *gin.Engine, h *handler.Handler) {
	router.Static("/static", "./static")
	router.Static("/uploads", h.App.Cfg.UploadDir)

	router.GET("/", h.HomePage)
	router.HEAD("/", h.HomePage)
	router.GET("/resources", h.ResourcesPage)
	router.HEAD("/resources", h.ResourcesPage)
	router.GET("/resources/:id", h.ResourceDetailPage)
	router.HEAD("/resources/:id", h.ResourceDetailPage)
	router.GET("/articles", h.ArticlesPage)
	router.HEAD("/articles", h.ArticlesPage)
	router.GET("/articles/:id", h.ArticleDetailPage)
	router.HEAD("/articles/:id", h.ArticleDetailPage)
	router.GET("/login", h.LoginPage)
	router.HEAD("/login", h.LoginPage)
	router.POST("/login", h.LoginHandler)
	router.GET("/register", h.RegisterPage)
	router.HEAD("/register", h.RegisterPage)
	router.POST("/register", h.RegisterHandler)
	router.GET("/logout", h.LogoutHandler)

	auth := router.Group("")
	auth.Use(AuthRequired())
	auth.GET("/profile", h.ProfilePage)
	auth.POST("/checkin", h.CheckinHandler)
	auth.POST("/resources/:id/share", h.ShareHandler)
	auth.POST("/resources/:id/comments", h.ResourceCommentHandler)
	auth.POST("/articles/:id/comments", h.ArticleCommentHandler)

	publish := auth.Group("/publish")
	publish.Use(PublishAuthorized())
	publish.GET("", h.PublishResourcePage)
	publish.GET("/template.csv", h.PublishTemplateCSV)
	publish.POST("/manual", h.PublishManualHandler)
	publish.POST("/import", h.PublishImportHandler)
	publish.POST("/crawler", h.PublishCrawlerHandler)

	admin := router.Group("/admin")
	admin.Use(AuthRequired(), AdminRequired())
	admin.GET("", h.AdminDashboard)
	admin.GET("/users", h.AdminUsersPage)
	admin.POST("/users/:id", h.AdminUpdateUser)
	admin.GET("/resources", h.AdminResourcesPage)
	admin.POST("/resources/:id/review", h.AdminReviewResource)
	admin.GET("/articles", h.AdminArticlesPage)
	admin.GET("/articles/new", h.AdminArticleFormPage)
	admin.POST("/articles", h.AdminCreateArticle)
	admin.GET("/articles/:id/edit", h.AdminArticleFormPage)
	admin.POST("/articles/:id", h.AdminUpdateArticle)
	admin.GET("/categories", h.AdminCategoriesPage)
	admin.POST("/categories", h.AdminCreateCategory)
	admin.POST("/categories/:id", h.AdminUpdateCategory)
	admin.POST("/categories/:id/delete", h.AdminDeleteCategory)
	admin.GET("/tags", h.AdminTagsPage)
	admin.POST("/tags", h.AdminCreateTag)
	admin.POST("/tags/:id", h.AdminUpdateTag)
	admin.POST("/tags/:id/delete", h.AdminDeleteTag)
	admin.GET("/settings", h.AdminSettingsPage)
	admin.POST("/settings", h.AdminUpdateSettings)
	admin.POST("/backup", h.AdminBackup)
	admin.POST("/restore", h.AdminRestore)
}
