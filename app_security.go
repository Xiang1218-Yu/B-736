package main

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
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
	csrfSessionKey     = "csrf_token"
	csrfFormField      = "_csrf"
	checkinRewardPoint = 10
	shareRewardPoint   = 5
	publishMinPoints   = 100
)

var (
	errShareRewardLimited = errors.New("share reward already claimed")
	errAlreadyCheckedIn   = errors.New("already checked in today")
	articleHTMLPolicy     = newArticleHTMLPolicy()
)

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

func sanitizeArticleContent(raw string) string {
	cleaned := articleHTMLPolicy.Sanitize(strings.TrimSpace(raw))
	return strings.TrimSpace(cleaned)
}

func safeArticleHTML(raw string) template.HTML {
	return template.HTML(sanitizeArticleContent(raw))
}

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

func secureStringEqual(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

func isSafeHTTPMethod(method string) bool {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodOptions, http.MethodTrace:
		return true
	default:
		return false
	}
}

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

func canPublishResource(user *models.User) bool {
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
		return user.Points >= publishMinPoints
	default:
		return false
	}
}

func dayRange(now time.Time) (time.Time, time.Time) {
	year, month, day := now.Date()
	location := now.Location()
	start := time.Date(year, month, day, 0, 0, 0, 0, location)
	return start, start.Add(24 * time.Hour)
}
