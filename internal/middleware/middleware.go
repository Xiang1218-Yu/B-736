package middleware

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"prompt736/internal/app"
	"prompt736/internal/models"
	"prompt736/internal/service"

	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
)

// ---- 安全头中间件 ----

// SecurityHeadersMiddleware 为响应添加安全相关 HTTP 头
func SecurityHeadersMiddleware() gin.HandlerFunc {
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

// ---- 公共数据注入中间件 ----

// CommonMiddleware 注入站点配置、locale、CSRF token、用户信息和 flash 消息
func CommonMiddleware(a *app.App, svc *service.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		session := sessions.Default(c)

		// 加载站点配置
		var siteConfig models.SiteConfig
		if cachedConfig, err := svc.GetSiteConfig(); err == nil {
			siteConfig = cachedConfig
		}
		c.Set("siteConfig", siteConfig)

		// 确定语言 locale
		defaultLocale := a.I18n.Resolve(siteConfig.LocaleDefault, "zh")
		cookieLocale, _ := c.Cookie("lang")
		localeCandidate := strings.TrimSpace(c.Query("lang"))
		if localeCandidate == "" {
			localeCandidate = cookieLocale
		}
		locale := a.I18n.Resolve(localeCandidate, defaultLocale)
		c.Set("locale", locale)
		if cookieLocale != locale {
			c.SetCookie("lang", locale, 86400*30, "/", "", false, false)
		}

		// 确保 CSRF token
		ensureCSRFToken(c)

		// 加载已登录用户
		if userID, ok := session.Get("userID").(uint); ok && userID > 0 {
			var user models.User
			if err := a.DB.First(&user, userID).Error; err == nil {
				c.Set("user", &user)
			}
		}

		// Flash 消息
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

// ---- CSRF 中间件 ----

// CSRFMiddleware 验证非安全方法的 CSRF token
func CSRFMiddleware(a *app.App) gin.HandlerFunc {
	return func(c *gin.Context) {
		if isSafeHTTPMethod(c.Request.Method) {
			c.Next()
			return
		}

		session := sessions.Default(c)
		sessionToken, _ := session.Get(service.CSRFSessionKey()).(string)
		if sessionToken == "" {
			sessionToken = ensureCSRFToken(c)
		}

		requestToken := strings.TrimSpace(c.PostForm(service.CSRFFormField))
		if requestToken == "" {
			requestToken = strings.TrimSpace(c.GetHeader("X-CSRF-Token"))
		}

		if !secureStringEqual(sessionToken, requestToken) {
			session := sessions.Default(c)
			session.Set("flash_error", a.I18n.T(c.GetString("locale"), "flash.invalid_csrf"))
			session.Save()
			redirectBackOrDefault(c, "/")
			c.Abort()
			return
		}

		c.Next()
	}
}

// ---- 认证中间件 ----

// AuthRequired 要求用户已登录
func AuthRequired(a *app.App) gin.HandlerFunc {
	return func(c *gin.Context) {
		if _, exists := c.Get("user"); !exists {
			session := sessions.Default(c)
			locale := c.GetString("locale")
			session.Set("flash_error", a.I18n.T(locale, "flash.login_required"))
			session.Save()
			c.Redirect(http.StatusFound, "/login")
			c.Abort()
			return
		}
		c.Next()
	}
}

// AdminRequired 要求用户是管理员
func AdminRequired(a *app.App) gin.HandlerFunc {
	return func(c *gin.Context) {
		user := c.MustGet("user").(*models.User)
		if user.Role != "admin" {
			session := sessions.Default(c)
			locale := c.GetString("locale")
			session.Set("flash_error", a.I18n.T(locale, "flash.no_permission"))
			session.Save()
			c.Redirect(http.StatusFound, "/")
			c.Abort()
			return
		}
		c.Next()
	}
}

// PublishAuthorized 要求用户有发布权限
func PublishAuthorized(a *app.App, svc *service.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		user := c.MustGet("user").(*models.User)
		if !service.CanPublishResource(user) {
			session := sessions.Default(c)
			locale := c.GetString("locale")
			session.Set("flash_error", a.I18n.T(locale, "flash.publish_not_authorized", service.PublishMinPoints))
			session.Save()
			c.Redirect(http.StatusFound, "/resources")
			c.Abort()
			return
		}
		c.Next()
	}
}

// ---- 内部辅助函数 ----

// ensureCSRFToken 确保 session 中存在 CSRF token，并写入 context
func ensureCSRFToken(c *gin.Context) string {
	if token := c.GetString("csrfToken"); token != "" {
		return token
	}

	session := sessions.Default(c)
	if token, ok := session.Get(service.CSRFSessionKey()).(string); ok && token != "" {
		c.Set("csrfToken", token)
		return token
	}

	token := generateSecureToken(32)
	session.Set(service.CSRFSessionKey(), token)
	_ = session.Save()
	c.Set("csrfToken", token)
	return token
}

// secureStringEqual 使用常量时间比较防止时序攻击
func secureStringEqual(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

// isSafeHTTPMethod 判断 HTTP 方法是否为安全方法（不需要 CSRF 校验）
func isSafeHTTPMethod(method string) bool {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodOptions, http.MethodTrace:
		return true
	default:
		return false
	}
}

// redirectBackOrDefault 优先重定向到来源页，否则使用 fallback
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

// generateSecureToken 生成加密安全的随机 token
func generateSecureToken(size int) string {
	if size <= 0 {
		size = 32
	}

	buf := make([]byte, size)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Sprintf("fallback-%d", time.Now().UnixNano())
	}

	return base64.RawURLEncoding.EncodeToString(buf)
}
