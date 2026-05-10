package app

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"html/template"
	"net/http"
	"net/url"
	"strings"
	"time"

	"prompt736/internal/models"

	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
	"github.com/microcosm-cc/bluemonday"
)

const (
	csrfSessionKey = "csrf_token"
	csrfFormField  = "_csrf"
)

var (
	errShareRewardLimited = errors.New("share reward already claimed")
	errAlreadyCheckedIn   = errors.New("already checked in today")
	articleHTMLPolicy     = newArticleHTMLPolicy()
)

// newArticleHTMLPolicy 创建文章 HTML 清理策略
func newArticleHTMLPolicy() *bluemonday.Policy {
	policy := bluemonday.UGCPolicy()
	policy.AllowAttrs("class").Globally()
	policy.AllowAttrs("target").OnElements("a")
	policy.AllowAttrs("rel").OnElements("a")
	policy.AllowAttrs("src", "alt", "title", "width", "height").OnElements("img")
	policy.AllowAttrs("loading").OnElements("img")
	policy.AllowURLSchemes("http", "https", "mailto")
	return policy
}

// SanitizeArticleContent 清理文章 HTML 内容
func SanitizeArticleContent(raw string) string {
	cleaned := articleHTMLPolicy.Sanitize(strings.TrimSpace(raw))
	return strings.TrimSpace(cleaned)
}

// SafeArticleHTML 将文章内容转换为安全的 HTML
func SafeArticleHTML(raw string) template.HTML {
	return template.HTML(SanitizeArticleContent(raw))
}

// SecurityHeadersMiddleware 安全头中间件
func (app *App) SecurityHeadersMiddleware() gin.HandlerFunc {
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

// ensureCSRFToken 确保 CSRF Token 存在
func (app *App) ensureCSRFToken(c *gin.Context) string {
	if token := c.GetString("csrfToken"); token != "" {
		return token
	}

	session := sessions.Default(c)
	if token, ok := session.Get(csrfSessionKey).(string); ok && token != "" {
		c.Set("csrfToken", token)
		return token
	}

	token := generateSecureToken(32)
	session.Set(csrfSessionKey, token)
	_ = session.Save()
	c.Set("csrfToken", token)
	return token
}

// CSRFMiddleware CSRF 验证中间件
func (app *App) CSRFMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if isSafeHTTPMethod(c.Request.Method) {
			c.Next()
			return
		}

		session := sessions.Default(c)
		sessionToken, _ := session.Get(csrfSessionKey).(string)
		if sessionToken == "" {
			sessionToken = app.ensureCSRFToken(c)
		}

		requestToken := strings.TrimSpace(c.PostForm(csrfFormField))
		if requestToken == "" {
			requestToken = strings.TrimSpace(c.GetHeader("X-CSRF-Token"))
		}

		if !secureStringEqual(sessionToken, requestToken) {
			app.setFlashKey(c, "flash_error", "flash.invalid_csrf")
			redirectBackOrDefault(c, "/")
			c.Abort()
			return
		}

		c.Next()
	}
}

// secureStringEqual 安全的字符串比较（防止时序攻击）
func secureStringEqual(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

// isSafeHTTPMethod 判断是否为安全的 HTTP 方法
func isSafeHTTPMethod(method string) bool {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodOptions, http.MethodTrace:
		return true
	default:
		return false
	}
}

// redirectBackOrDefault 重定向回来源页或默认页面
func redirectBackOrDefault(c *gin.Context, fallback string) {
	ref := strings.TrimSpace(c.Request.Referer())
	if ref == "" {
		c.Redirect(http.StatusFound, fallback)
		return
	}

	parsed, err := url.Parse(ref)
	if err != nil {
		c.Redirect(http.StatusFound, fallback)
		return
	}

	if parsed.Host != "" && !strings.EqualFold(parsed.Host, c.Request.Host) {
		c.Redirect(http.StatusFound, fallback)
		return
	}

	target := parsed.Path
	if target == "" || !strings.HasPrefix(target, "/") {
		target = fallback
	}
	if parsed.RawQuery != "" {
		target += "?" + parsed.RawQuery
	}

	c.Redirect(http.StatusFound, target)
}

// generateSecureToken 生成安全的随机 Token
func generateSecureToken(size int) string {
	if size <= 0 {
		size = 32
	}

	buf := make([]byte, size)
	if _, err := rand.Read(buf); err != nil {
		return "fallback-" + time.Now().String()
	}

	return base64.RawURLEncoding.EncodeToString(buf)
}

// CanPublishResource 判断用户是否可以发布资源
func CanPublishResource(user *models.User) bool {
	if user == nil {
		return false
	}
	if strings.ToLower(strings.TrimSpace(user.Status)) != "active" {
		return false
	}

	switch strings.ToLower(strings.TrimSpace(user.Role)) {
	case "admin", "editor":
		return true
	case "user":
		return user.Points >= PublishMinPoints
	default:
		return false
	}
}

// CommonMiddleware 通用中间件，注入公共数据
func (app *App) CommonMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		session := sessions.Default(c)

		var siteConfig models.SiteConfig
		if cachedConfig, err := app.getSiteConfig(); err == nil {
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
		app.ensureCSRFToken(c)

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

// AuthRequired 认证中间件
func (app *App) AuthRequired() gin.HandlerFunc {
	return func(c *gin.Context) {
		if _, exists := c.Get("user"); !exists {
			app.setFlashKey(c, "flash_error", "flash.login_required")
			c.Redirect(http.StatusFound, "/login")
			c.Abort()
			return
		}
		c.Next()
	}
}

// AdminRequired 管理员权限中间件
func (app *App) AdminRequired() gin.HandlerFunc {
	return func(c *gin.Context) {
		user := c.MustGet("user").(*models.User)
		if user.Role != "admin" {
			app.setFlashKey(c, "flash_error", "flash.no_permission")
			c.Redirect(http.StatusFound, "/")
			c.Abort()
			return
		}
		c.Next()
	}
}

// PublishAuthorized 发布权限中间件
func (app *App) PublishAuthorized() gin.HandlerFunc {
	return func(c *gin.Context) {
		user := c.MustGet("user").(*models.User)
		if !CanPublishResource(user) {
			app.setFlash(c, "flash_error", app.tr(c, "flash.publish_not_authorized", PublishMinPoints))
			c.Redirect(http.StatusFound, "/resources")
			c.Abort()
			return
		}
		c.Next()
	}
}

// setFlash 设置 Flash 消息
func (app *App) setFlash(c *gin.Context, key, value string) {
	session := sessions.Default(c)
	session.Set(key, value)
	session.Save()
}

// setFlashKey 使用国际化键设置 Flash 消息
func (app *App) setFlashKey(c *gin.Context, flashKey, i18nKey string, args ...interface{}) {
	app.setFlash(c, flashKey, app.tr(c, i18nKey, args...))
}

// tr 获取国际化文本
func (app *App) tr(c *gin.Context, key string, args ...interface{}) string {
	siteConfig := c.MustGet("siteConfig").(models.SiteConfig)
	defaultLocale := app.I18n.Resolve(siteConfig.LocaleDefault, "zh")
	locale := c.GetString("locale")
	if locale == "" {
		locale = defaultLocale
	}
	return app.I18n.T(locale, key, args...)
}

// dayRange 获取当天的时间范围
func dayRange(now time.Time) (time.Time, time.Time) {
	year, month, day := now.Date()
	location := now.Location()
	start := time.Date(year, month, day, 0, 0, 0, 0, location)
	return start, start.Add(24 * time.Hour)
}
