package app

import (
	"html/template"
	"net/http"
	"path/filepath"

	"github.com/gin-gonic/gin"
)

// LoadTemplates 加载所有模板文件
func LoadTemplates() *template.Template {
	tmpl := template.New("").Funcs(getTemplateFuncs())

	// 加载所有模板
	patterns := []string{
		"templates/layouts/*.html",
		"templates/pages/*.html",
		"templates/admin/*.html",
	}

	for _, pattern := range patterns {
		files, _ := filepath.Glob(pattern)
		for _, file := range files {
			tmpl = template.Must(tmpl.ParseFiles(file))
		}
	}

	return tmpl
}

// templateFuncs 获取带上下文数据的模板函数
func (app *App) templateFuncs(data PageData) template.FuncMap {
	funcs := getTemplateFuncs()

	funcs["T"] = func(key string, args ...interface{}) string {
		return app.I18n.T(data.Locale, key, args...)
	}
	funcs["langURL"] = func(lang string) string {
		return BuildLangSwitchURL(data.RequestPath, data.RawQuery, lang)
	}
	funcs["langActive"] = func(lang string) bool {
		resolved := app.I18n.Resolve(lang, data.Locale)
		return resolved == data.Locale
	}
	funcs["resourceTypeLabel"] = func(resourceType string) string {
		key := "resource_type." + CanonicalResourceType(resourceType)
		label := app.I18n.T(data.Locale, key)
		if label == key {
			return resourceType
		}
		return label
	}
	funcs["sourceTypeLabel"] = func(sourceType string) string {
		key := "source_type." + CanonicalSourceType(sourceType)
		label := app.I18n.T(data.Locale, key)
		if label == key {
			return sourceType
		}
		return label
	}
	funcs["roleLabel"] = func(role string) string {
		key := "role." + strings.ToLower(role)
		label := app.I18n.T(data.Locale, key)
		if label == key {
			return role
		}
		return label
	}
	funcs["statusLabel"] = func(status string) string {
		key := "status." + strings.ToLower(status)
		label := app.I18n.T(data.Locale, key)
		if label == key {
			return status
		}
		return label
	}
	funcs["siteText"] = func(key, fallback string) string {
		value := app.I18n.T(data.Locale, key)
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
			csrfFormField,
			template.HTMLEscapeString(data.CSRFToken),
		))
	}

	return funcs
}

// basePageData 获取基础页面数据
func (app *App) basePageData(c *gin.Context) PageData {
	siteConfig := c.MustGet("siteConfig").(models.SiteConfig)
	defaultLocale := app.I18n.Resolve(siteConfig.LocaleDefault, "zh")
	locale := c.GetString("locale")
	if locale == "" {
		locale = defaultLocale
	}

	data := PageData{
		SiteConfig:       siteConfig,
		Ads:              app.ParseAdSlots(siteConfig.AdsJSON, locale),
		Locale:           locale,
		RequestPath:      c.Request.URL.Path,
		RawQuery:         c.Request.URL.RawQuery,
		CSRFToken:        c.GetString("csrfToken"),
		PublishMinPoints: PublishMinPoints,
	}

	if user, exists := c.Get("user"); exists {
		data.User = user.(*models.User)
		data.CanPublish = CanPublishResource(data.User)
	}

	if flash, exists := c.Get("flash_success"); exists {
		data.FlashSuccess = flash.(string)
	}
	if flash, exists := c.Get("flash_error"); exists {
		data.FlashError = flash.(string)
	}

	return data
}

// renderPage 渲染前台页面
func (app *App) renderPage(c *gin.Context, templateFile string, data PageData) {
	tmpl := template.Must(template.New("").Funcs(app.templateFuncs(data)).ParseFiles(
		"templates/layouts/base.html",
		"templates/pages/"+templateFile,
	))

	c.Header("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.ExecuteTemplate(c.Writer, "base.html", data); err != nil {
		app.Log.WithError(err).Error("template render failed")
		c.String(http.StatusInternalServerError, app.tr(c, "flash.render_failed"))
	}
}

// renderAdminPage 渲染管理后台页面
func (app *App) renderAdminPage(c *gin.Context, templateFile string, data PageData) {
	tmpl := template.Must(template.New("").Funcs(app.templateFuncs(data)).ParseFiles(
		"templates/admin/base.html",
		"templates/admin/"+templateFile,
	))

	c.Header("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.ExecuteTemplate(c.Writer, "base.html", data); err != nil {
		app.Log.WithError(err).Error("admin template render failed")
		c.String(http.StatusInternalServerError, app.tr(c, "flash.render_failed"))
	}
}
