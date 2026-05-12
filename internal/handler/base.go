package handler

import (
	"fmt"
	"html/template"
	"net/http"
	"strings"

	"prompt736/internal/models"
	"prompt736/internal/service"
	"prompt736/internal/types"
	"prompt736/internal/utils"

	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
)

// Handler HTTP处理器
type Handler struct {
	App *service.App
}

// NewHandler 创建新的处理器
func NewHandler(app *service.App) *Handler {
	return &Handler{App: app}
}

// setFlash 设置Flash消息
func (h *Handler) setFlash(c *gin.Context, key, value string) {
	session := sessions.Default(c)
	session.Set(key, value)
	session.Save()
}

// setFlashKey 设置国际化Flash消息
func (h *Handler) setFlashKey(c *gin.Context, flashKey, i18nKey string, args ...interface{}) {
	h.setFlash(c, flashKey, h.tr(c, i18nKey, args...))
}

// tr 获取国际化文本
func (h *Handler) tr(c *gin.Context, key string, args ...interface{}) string {
	siteConfig := c.MustGet("siteConfig").(models.SiteConfig)
	defaultLocale := h.App.I18n.Resolve(siteConfig.LocaleDefault, "zh")
	locale := c.GetString("locale")
	if locale == "" {
		locale = defaultLocale
	}
	return h.App.I18n.T(locale, key, args...)
}

// basePageData 获取基础页面数据
func (h *Handler) basePageData(c *gin.Context) types.PageData {
	siteConfig := c.MustGet("siteConfig").(models.SiteConfig)
	defaultLocale := h.App.I18n.Resolve(siteConfig.LocaleDefault, "zh")
	locale := c.GetString("locale")
	if locale == "" {
		locale = defaultLocale
	}

	data := types.PageData{
		SiteConfig:       siteConfig,
		Ads:              h.App.ParseAdSlots(siteConfig.AdsJSON, locale),
		Locale:           locale,
		RequestPath:      c.Request.URL.Path,
		RawQuery:         c.Request.URL.RawQuery,
		CSRFToken:        c.GetString("csrfToken"),
		PublishMinPoints: types.PublishMinPoints,
	}

	if user, exists := c.Get("user"); exists {
		data.User = user.(*models.User)
		data.CanPublish = utils.CanPublishResource(data.User, types.PublishMinPoints)
	}

	if flash, exists := c.Get("flash_success"); exists {
		data.FlashSuccess = flash.(string)
	}
	if flash, exists := c.Get("flash_error"); exists {
		data.FlashError = flash.(string)
	}

	return data
}

// getTemplateFuncs 获取模板函数
func getTemplateFuncs() template.FuncMap {
	return template.FuncMap{
		"truncate": func(a interface{}, b interface{}) string {
			var s string
			var n int

			switch v := a.(type) {
			case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
				n = utils.ToIntOrZero(v)
			case string:
				s = v
			}

			switch v := b.(type) {
			case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
				n = utils.ToIntOrZero(v)
			case string:
				s = v
			}

			if n <= 0 || len(s) <= n {
				return s
			}
			runes := []rune(s)
			if len(runes) <= n {
				return s
			}
			return string(runes[:n]) + "..."
		},
		"formatDate": func(v interface{}) string {
			return fmt.Sprintf("%v", v)
		},
		"safeHTML": func(s string) template.HTML {
			return utils.SafeArticleHTML(s)
		},
		"csrfField": func() template.HTML {
			return ""
		},
		"add": func(a, b interface{}) int {
			return utils.ToIntOrZero(a) + utils.ToIntOrZero(b)
		},
		"sub": func(a, b interface{}) int {
			return utils.ToIntOrZero(a) - utils.ToIntOrZero(b)
		},
		"iterate": func(n int) []int {
			result := make([]int, n)
			for i := range result {
				result[i] = i
			}
			return result
		},
		"upper": strings.ToUpper,
		"lower": strings.ToLower,
		"slice": func(s string, start, end int) string {
			if start > len(s) {
				return ""
			}
			if end > len(s) {
				end = len(s)
			}
			return s[start:end]
		},
		"eq": func(a, b interface{}) bool {
			return fmt.Sprintf("%v", a) == fmt.Sprintf("%v", b)
		},
		"gt": func(a, b interface{}) bool {
			return utils.ToIntOrZero(a) > utils.ToIntOrZero(b)
		},
		"lt": func(a, b interface{}) bool {
			return utils.ToIntOrZero(a) < utils.ToIntOrZero(b)
		},
		"T": func(key string, args ...interface{}) string {
			if len(args) > 0 {
				return fmt.Sprintf(key, args...)
			}
			return key
		},
		"langURL": func(lang string) string {
			return "/?lang=" + lang
		},
		"langActive": func(lang string) bool {
			return lang == "zh"
		},
		"resourceTypeClass": utils.CanonicalResourceType,
		"resourceTypeLabel": func(resourceType string) string {
			return resourceType
		},
		"sourceTypeLabel": func(sourceType string) string {
			return sourceType
		},
		"roleLabel": func(role string) string {
			return role
		},
		"statusLabel": func(status string) string {
			return status
		},
		"siteText": func(_ string, fallback string) string {
			return fallback
		},
		"ratingStars": utils.BuildRatingStars,
	}
}

// templateFuncs 带上下文的模板函数
func (h *Handler) templateFuncs(data types.PageData) template.FuncMap {
	funcs := getTemplateFuncs()

	funcs["T"] = func(key string, args ...interface{}) string {
		return h.App.I18n.T(data.Locale, key, args...)
	}
	funcs["langURL"] = func(lang string) string {
		return utils.BuildLangSwitchURL(data.RequestPath, data.RawQuery, lang)
	}
	funcs["langActive"] = func(lang string) bool {
		resolved := h.App.I18n.Resolve(lang, data.Locale)
		return resolved == data.Locale
	}
	funcs["resourceTypeLabel"] = func(resourceType string) string {
		key := "resource_type." + utils.CanonicalResourceType(resourceType)
		label := h.App.I18n.T(data.Locale, key)
		if label == key {
			return resourceType
		}
		return label
	}
	funcs["sourceTypeLabel"] = func(sourceType string) string {
		key := "source_type." + utils.CanonicalSourceType(sourceType)
		label := h.App.I18n.T(data.Locale, key)
		if label == key {
			return sourceType
		}
		return label
	}
	funcs["roleLabel"] = func(role string) string {
		key := "role." + strings.ToLower(role)
		label := h.App.I18n.T(data.Locale, key)
		if label == key {
			return role
		}
		return label
	}
	funcs["statusLabel"] = func(status string) string {
		key := "status." + strings.ToLower(status)
		label := h.App.I18n.T(data.Locale, key)
		if label == key {
			return status
		}
		return label
	}
	funcs["siteText"] = func(key, fallback string) string {
		value := h.App.I18n.T(data.Locale, key)
		if value == key || strings.TrimSpace(value) == "" {
			return fallback
		}
		return value
	}
	funcs["csrfField"] = func() template.HTML {
		if data.CSRFToken == "" {
			return ""
		}
		return template.HTML(fmt.Sprintf(
			`<input type="hidden" name="%s" value="%s">`,
			types.CSRFFormField,
			template.HTMLEscapeString(data.CSRFToken),
		))
	}

	return funcs
}

// renderPage 渲染前台页面
func (h *Handler) renderPage(c *gin.Context, templateFile string, data types.PageData) {
	tmpl := template.Must(template.New("").Funcs(h.templateFuncs(data)).ParseFiles(
		"templates/layouts/base.html",
		"templates/pages/"+templateFile,
	))

	c.Header("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.ExecuteTemplate(c.Writer, "base.html", data); err != nil {
		h.App.Log.WithError(err).Error("template render failed")
		c.String(http.StatusInternalServerError, h.tr(c, "flash.render_failed"))
	}
}

// renderAdminPage 渲染管理后台页面
func (h *Handler) renderAdminPage(c *gin.Context, templateFile string, data types.PageData) {
	tmpl := template.Must(template.New("").Funcs(h.templateFuncs(data)).ParseFiles(
		"templates/admin/base.html",
		"templates/admin/"+templateFile,
	))

	c.Header("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.ExecuteTemplate(c.Writer, "base.html", data); err != nil {
		h.App.Log.WithError(err).Error("admin template render failed")
		c.String(http.StatusInternalServerError, h.tr(c, "flash.render_failed"))
	}
}
