package main

import (
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"prompt736/internal/config"
	"prompt736/internal/database"
	"prompt736/internal/i18n"
	"prompt736/internal/models"
	"prompt736/internal/seed"
	"prompt736/internal/utils"

	"github.com/PuerkitoBio/goquery"
	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"github.com/patrickmn/go-cache"
	"github.com/sirupsen/logrus"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type App struct {
	DB        *gorm.DB
	Cache     *cache.Cache
	Cfg       config.Config
	Log       *logrus.Logger
	Templates *template.Template
	I18n      *i18n.Bundle
}

type PageData struct {
	Title        string
	ActiveNav    string
	User         *models.User
	SiteConfig   models.SiteConfig
	Ads          map[string]AdSlot
	FlashSuccess string
	FlashError   string
	Data         interface{}
	Locale       string
	RequestPath  string
	RawQuery     string
	CSRFToken    string

	// Home page
	Stats          map[string]int64
	HotResources   []models.Resource
	LatestArticles []models.Article
	Categories     []models.Category

	// List pages
	Resources   []models.Resource
	Articles    []models.Article
	Tags        []models.Tag
	Query       string
	CategoryID  uint
	TagID       uint
	Type        string
	QueryParams string
	Page        int
	PageSize    int
	Total       int64
	TotalPages  int

	// Detail pages
	Resource *models.Resource
	Article  *models.Article
	Comments []models.Comment

	// Profile
	CanCheckin       bool
	CanPublish       bool
	PublishMinPoints int

	// Admin
	Users            []models.User
	Status           string
	PendingResources []models.Resource
	Backups          []BackupInfo
	Config           *models.SiteConfig
}

type BackupInfo struct {
	Name string
	Time string
}

type BackupSnapshot struct {
	Version      int                  `json:"version"`
	GeneratedAt  time.Time            `json:"generated_at"`
	Users        []models.User        `json:"users"`
	Categories   []models.Category    `json:"categories"`
	Tags         []models.Tag         `json:"tags"`
	Resources    []models.Resource    `json:"resources"`
	Articles     []models.Article     `json:"articles"`
	Comments     []models.Comment     `json:"comments"`
	SiteConfigs  []models.SiteConfig  `json:"site_configs"`
	ShareLogs    []models.ShareLog    `json:"share_logs"`
	ResourceTags []models.ResourceTag `json:"resource_tags"`
}

type AdSlot struct {
	Code     string
	Title    string
	Desc     string
	Link     string
	ImageURL string
	CTA      string
	Contact  string
	Enabled  bool
}

type adConfig struct {
	Slots []adSlotConfig `json:"slots"`
}

type adSlotConfig struct {
	Code     string `json:"code"`
	Title    string `json:"title"`
	Desc     string `json:"desc"`
	Link     string `json:"link"`
	ImageURL string `json:"image_url"`
	CTA      string `json:"cta"`
	Contact  string `json:"contact"`
	Enabled  *bool  `json:"enabled"`
}

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

	// Load templates
	tmpl := loadTemplates()

	app := &App{
		DB:        db,
		Cache:     cacheStore,
		Cfg:       cfg,
		Log:       logger,
		Templates: tmpl,
		I18n:      bundle,
	}

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

	// Session middleware
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

	// Static files
	router.Static("/static", "./static")
	router.Static("/uploads", cfg.UploadDir)

	// Middleware to inject common data
	router.Use(app.CommonMiddleware())
	router.Use(app.CSRFMiddleware())

	// Public pages
	router.GET("/", app.HomePage)
	router.HEAD("/", app.HomePage)
	router.GET("/resources", app.ResourcesPage)
	router.HEAD("/resources", app.ResourcesPage)
	router.GET("/resources/:id", app.ResourceDetailPage)
	router.HEAD("/resources/:id", app.ResourceDetailPage)
	router.GET("/articles", app.ArticlesPage)
	router.HEAD("/articles", app.ArticlesPage)
	router.GET("/articles/:id", app.ArticleDetailPage)
	router.HEAD("/articles/:id", app.ArticleDetailPage)
	router.GET("/login", app.LoginPage)
	router.HEAD("/login", app.LoginPage)
	router.POST("/login", app.LoginHandler)
	router.GET("/register", app.RegisterPage)
	router.HEAD("/register", app.RegisterPage)
	router.POST("/register", app.RegisterHandler)
	router.GET("/logout", app.LogoutHandler)

	// Auth required pages
	auth := router.Group("")
	auth.Use(app.AuthRequired())
	auth.GET("/profile", app.ProfilePage)
	auth.POST("/checkin", app.CheckinHandler)
	auth.POST("/resources/:id/share", app.ShareHandler)
	auth.POST("/resources/:id/comments", app.ResourceCommentHandler)
	auth.POST("/articles/:id/comments", app.ArticleCommentHandler)

	publish := auth.Group("/publish")
	publish.Use(app.PublishAuthorized())
	publish.GET("", app.PublishResourcePage)
	publish.GET("/template.csv", app.PublishTemplateCSV)
	publish.POST("/manual", app.PublishManualHandler)
	publish.POST("/import", app.PublishImportHandler)
	publish.POST("/crawler", app.PublishCrawlerHandler)

	// Admin pages
	admin := router.Group("/admin")
	admin.Use(app.AuthRequired(), app.AdminRequired())
	admin.GET("", app.AdminDashboard)
	admin.GET("/users", app.AdminUsersPage)
	admin.POST("/users/:id", app.AdminUpdateUser)
	admin.GET("/resources", app.AdminResourcesPage)
	admin.POST("/resources/:id/review", app.AdminReviewResource)
	admin.GET("/articles", app.AdminArticlesPage)
	admin.GET("/articles/new", app.AdminArticleFormPage)
	admin.POST("/articles", app.AdminCreateArticle)
	admin.GET("/articles/:id/edit", app.AdminArticleFormPage)
	admin.POST("/articles/:id", app.AdminUpdateArticle)
	admin.GET("/categories", app.AdminCategoriesPage)
	admin.POST("/categories", app.AdminCreateCategory)
	admin.POST("/categories/:id", app.AdminUpdateCategory)
	admin.POST("/categories/:id/delete", app.AdminDeleteCategory)
	admin.GET("/tags", app.AdminTagsPage)
	admin.POST("/tags", app.AdminCreateTag)
	admin.POST("/tags/:id", app.AdminUpdateTag)
	admin.POST("/tags/:id/delete", app.AdminDeleteTag)
	admin.GET("/settings", app.AdminSettingsPage)
	admin.POST("/settings", app.AdminUpdateSettings)
	admin.POST("/backup", app.AdminBackup)
	admin.POST("/restore", app.AdminRestore)

	if err := router.Run(":8080"); err != nil {
		logger.WithError(err).Fatal("server start failed")
	}
}

func getTemplateFuncs() template.FuncMap {
	return template.FuncMap{
		"truncate": func(a interface{}, b interface{}) string {
			var s string
			var n int

			switch v := a.(type) {
			case int:
				n = v
			case int8:
				n = int(v)
			case int16:
				n = int(v)
			case int32:
				n = int(v)
			case int64:
				n = int(v)
			case uint:
				n = int(v)
			case uint8:
				n = int(v)
			case uint16:
				n = int(v)
			case uint32:
				n = int(v)
			case uint64:
				n = int(v)
			case string:
				s = v
			}

			switch v := b.(type) {
			case int:
				n = v
			case int8:
				n = int(v)
			case int16:
				n = int(v)
			case int32:
				n = int(v)
			case int64:
				n = int(v)
			case uint:
				n = int(v)
			case uint8:
				n = int(v)
			case uint16:
				n = int(v)
			case uint32:
				n = int(v)
			case uint64:
				n = int(v)
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
			switch t := v.(type) {
			case time.Time:
				return t.Format("2006-01-02")
			case *time.Time:
				if t == nil {
					return ""
				}
				return t.Format("2006-01-02")
			case string:
				return t
			default:
				return ""
			}
		},
		"safeHTML": func(s string) template.HTML {
			return safeArticleHTML(s)
		},
		"csrfField": func() template.HTML {
			return ""
		},
		"add": func(a, b interface{}) int {
			return toIntOrZero(a) + toIntOrZero(b)
		},
		"sub": func(a, b interface{}) int {
			return toIntOrZero(a) - toIntOrZero(b)
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
			return toIntOrZero(a) > toIntOrZero(b)
		},
		"lt": func(a, b interface{}) bool {
			return toIntOrZero(a) < toIntOrZero(b)
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
		"resourceTypeClass": canonicalResourceType,
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
		"ratingStars": buildRatingStars,
	}
}

func (app *App) templateFuncs(data PageData) template.FuncMap {
	funcs := getTemplateFuncs()

	funcs["T"] = func(key string, args ...interface{}) string {
		return app.I18n.T(data.Locale, key, args...)
	}
	funcs["langURL"] = func(lang string) string {
		return buildLangSwitchURL(data.RequestPath, data.RawQuery, lang)
	}
	funcs["langActive"] = func(lang string) bool {
		resolved := app.I18n.Resolve(lang, data.Locale)
		return resolved == data.Locale
	}
	funcs["resourceTypeLabel"] = func(resourceType string) string {
		key := "resource_type." + canonicalResourceType(resourceType)
		label := app.I18n.T(data.Locale, key)
		if label == key {
			return resourceType
		}
		return label
	}
	funcs["sourceTypeLabel"] = func(sourceType string) string {
		key := "source_type." + canonicalSourceType(sourceType)
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

func loadTemplates() *template.Template {
	tmpl := template.New("").Funcs(getTemplateFuncs())

	// Load all templates
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

// Middleware
func (app *App) CommonMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		session := sessions.Default(c)

		// Load site config
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

		// Load user if logged in
		if userID, ok := session.Get("userID").(uint); ok && userID > 0 {
			var user models.User
			if err := app.DB.First(&user, userID).Error; err == nil {
				c.Set("user", &user)
			}
		}

		// Flash messages
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

func (app *App) PublishAuthorized() gin.HandlerFunc {
	return func(c *gin.Context) {
		user := c.MustGet("user").(*models.User)
		if !canPublishResource(user) {
			app.setFlash(c, "flash_error", app.tr(c, "flash.publish_not_authorized", publishMinPoints))
			c.Redirect(http.StatusFound, "/resources")
			c.Abort()
			return
		}
		c.Next()
	}
}

func (app *App) setFlash(c *gin.Context, key, value string) {
	session := sessions.Default(c)
	session.Set(key, value)
	session.Save()
}

func (app *App) setFlashKey(c *gin.Context, flashKey, i18nKey string, args ...interface{}) {
	app.setFlash(c, flashKey, app.tr(c, i18nKey, args...))
}

func (app *App) tr(c *gin.Context, key string, args ...interface{}) string {
	siteConfig := c.MustGet("siteConfig").(models.SiteConfig)
	defaultLocale := app.I18n.Resolve(siteConfig.LocaleDefault, "zh")
	locale := c.GetString("locale")
	if locale == "" {
		locale = defaultLocale
	}
	return app.I18n.T(locale, key, args...)
}

func (app *App) basePageData(c *gin.Context) PageData {
	siteConfig := c.MustGet("siteConfig").(models.SiteConfig)
	defaultLocale := app.I18n.Resolve(siteConfig.LocaleDefault, "zh")
	locale := c.GetString("locale")
	if locale == "" {
		locale = defaultLocale
	}

	data := PageData{
		SiteConfig:       siteConfig,
		Ads:              app.parseAdSlots(siteConfig.AdsJSON, locale),
		Locale:           locale,
		RequestPath:      c.Request.URL.Path,
		RawQuery:         c.Request.URL.RawQuery,
		CSRFToken:        c.GetString("csrfToken"),
		PublishMinPoints: publishMinPoints,
	}

	if user, exists := c.Get("user"); exists {
		data.User = user.(*models.User)
		data.CanPublish = canPublishResource(data.User)
	}

	if flash, exists := c.Get("flash_success"); exists {
		data.FlashSuccess = flash.(string)
	}
	if flash, exists := c.Get("flash_error"); exists {
		data.FlashError = flash.(string)
	}

	return data
}

func (app *App) renderPage(c *gin.Context, templateFile string, data PageData) {
	// Parse base + page template with shared funcMap
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

// Page Handlers
func (app *App) HomePage(c *gin.Context) {
	data := app.basePageData(c)
	data.Title = ""
	data.ActiveNav = "home"

	if payload, err := app.getHomePagePayload(); err == nil {
		data.Stats = payload.Stats
		data.HotResources = payload.HotResources
		data.LatestArticles = payload.LatestArticles
		data.Categories = payload.Categories
	} else {
		app.Log.WithError(err).Warn("load home page cache payload failed")
	}

	app.renderPage(c, "home.html", data)
}

func (app *App) ResourcesPage(c *gin.Context) {
	data := app.basePageData(c)
	data.Title = app.tr(c, "resource.library_title")
	data.ActiveNav = "resources"

	q := strings.TrimSpace(c.Query("q"))
	categoryID := parseUint(c.Query("category_id"))
	tagID := parseUint(c.Query("tag_id"))
	resourceType := c.Query("type")
	page := parseInt(c.DefaultQuery("page", "1"), 1)
	pageSize := 12

	data.Query = q
	data.CategoryID = categoryID
	data.TagID = tagID
	data.Type = resourceType
	data.Page = page
	data.PageSize = pageSize

	// Build query params for pagination
	var params []string
	if q != "" {
		params = append(params, "q="+q)
	}
	if categoryID > 0 {
		params = append(params, fmt.Sprintf("category_id=%d", categoryID))
	}
	if tagID > 0 {
		params = append(params, fmt.Sprintf("tag_id=%d", tagID))
	}
	if resourceType != "" {
		params = append(params, "type="+resourceType)
	}
	if len(params) > 0 {
		data.QueryParams = "&" + strings.Join(params, "&")
	}

	if payload, err := app.getResourceListPayload(q, categoryID, tagID, resourceType, page, pageSize); err == nil {
		data.Categories = payload.Categories
		data.Tags = payload.Tags
		data.Total = payload.Total
		data.TotalPages = int((payload.Total + int64(pageSize) - 1) / int64(pageSize))
		data.Resources = payload.Resources
	} else {
		app.Log.WithError(err).Warn("load resource list cache payload failed")
	}

	app.renderPage(c, "resources.html", data)
}

func (app *App) PublishResourcePage(c *gin.Context) {
	data := app.basePageData(c)
	data.Title = app.tr(c, "resource.publish_title")
	data.ActiveNav = "resources"

	if categories, err := app.getCategories(); err == nil {
		data.Categories = categories
	} else {
		app.Log.WithError(err).Warn("load cached categories for publish page failed")
	}

	if tags, err := app.getTags(); err == nil {
		data.Tags = tags
	} else {
		app.Log.WithError(err).Warn("load cached tags for publish page failed")
	}

	app.renderPage(c, "resource_publish.html", data)
}

func (app *App) PublishTemplateCSV(c *gin.Context) {
	filename := "resource_import_template.csv"
	content := strings.Join([]string{
		"title,description,resource_type,link",
		"Prompt Toolkit,A ready-to-use prompt toolkit,software,https://example.com/toolkit",
		"Go Performance Notes,Practical performance tuning checklist,document,https://example.com/go-notes",
		"Design Course Replay,UI design walkthrough videos,video,https://example.com/design-course",
	}, "\n")

	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
	c.String(http.StatusOK, content)
}

func (app *App) PublishManualHandler(c *gin.Context) {
	user := c.MustGet("user").(*models.User)

	title := strings.TrimSpace(c.PostForm("title"))
	description := strings.TrimSpace(c.PostForm("description"))
	resourceType := canonicalResourceType(c.PostForm("resource_type"))
	link := strings.TrimSpace(c.PostForm("link"))
	categoryID := parseUint(c.PostForm("category_id"))
	tagIDs := parseUintList(c.PostFormArray("tag_ids"))

	if len([]rune(title)) < 2 {
		app.setFlashKey(c, "flash_error", "flash.publish_title_too_short")
		c.Redirect(http.StatusFound, "/publish")
		return
	}
	if len([]rune(description)) < 5 {
		app.setFlashKey(c, "flash_error", "flash.publish_description_too_short")
		c.Redirect(http.StatusFound, "/publish")
		return
	}
	if link != "" && !isValidHTTPURL(link) {
		app.setFlashKey(c, "flash_error", "flash.publish_invalid_link")
		c.Redirect(http.StatusFound, "/publish")
		return
	}

	fileName := ""
	if file, err := c.FormFile("resource_file"); err == nil && file != nil {
		if file.Size > 100*1024*1024 {
			app.setFlashKey(c, "flash_error", "flash.publish_file_too_large")
			c.Redirect(http.StatusFound, "/publish")
			return
		}

		ext := strings.ToLower(filepath.Ext(file.Filename))
		if ext == "" {
			ext = ".bin"
		}
		fileName = fmt.Sprintf("%d_%d%s", user.ID, time.Now().UnixNano(), ext)
		if err := os.MkdirAll(app.Cfg.UploadDir, 0755); err != nil {
			app.Log.WithError(err).Error("create upload dir failed")
			app.setFlashKey(c, "flash_error", "flash.publish_upload_failed")
			c.Redirect(http.StatusFound, "/publish")
			return
		}
		target := filepath.Join(app.Cfg.UploadDir, fileName)
		if err := c.SaveUploadedFile(file, target); err != nil {
			app.Log.WithError(err).Error("save uploaded file failed")
			app.setFlashKey(c, "flash_error", "flash.publish_upload_failed")
			c.Redirect(http.StatusFound, "/publish")
			return
		}
	}

	if link == "" && fileName == "" {
		app.setFlashKey(c, "flash_error", "flash.publish_link_or_file_required")
		c.Redirect(http.StatusFound, "/publish")
		return
	}

	status := "pending"
	if strings.EqualFold(user.Role, "admin") {
		status = "approved"
	}

	var categoryPtr *uint
	if categoryID > 0 {
		categoryPtr = &categoryID
	}

	resource := models.Resource{
		Title:        title,
		Description:  description,
		ResourceType: resourceType,
		Link:         link,
		FilePath:     fileName,
		SourceType:   "manual",
		Status:       status,
		UserID:       user.ID,
		CategoryID:   categoryPtr,
	}

	if len(tagIDs) > 0 {
		var tags []models.Tag
		if err := app.DB.Find(&tags, tagIDs).Error; err == nil && len(tags) > 0 {
			resource.Tags = tags
		}
	}

	if err := app.DB.Create(&resource).Error; err != nil {
		app.Log.WithError(err).Error("publish manual resource failed")
		app.setFlashKey(c, "flash_error", "flash.publish_failed")
		c.Redirect(http.StatusFound, "/publish")
		return
	}

	app.Cache.Flush()
	if resource.Status == "approved" {
		app.setFlashKey(c, "flash_success", "flash.publish_success_approved")
		c.Redirect(http.StatusFound, fmt.Sprintf("/resources/%d", resource.ID))
		return
	}
	app.setFlashKey(c, "flash_success", "flash.publish_success_pending")
	c.Redirect(http.StatusFound, "/resources")
}

func (app *App) PublishImportHandler(c *gin.Context) {
	user := c.MustGet("user").(*models.User)

	file, err := c.FormFile("import_file")
	if err != nil {
		app.setFlashKey(c, "flash_error", "flash.publish_import_file_required")
		c.Redirect(http.StatusFound, "/publish")
		return
	}

	src, err := file.Open()
	if err != nil {
		app.Log.WithError(err).Error("open import file failed")
		app.setFlashKey(c, "flash_error", "flash.publish_import_failed")
		c.Redirect(http.StatusFound, "/publish")
		return
	}
	defer src.Close()

	defaultType := "other"
	if rawDefaultType := strings.TrimSpace(c.PostForm("default_resource_type")); rawDefaultType != "" {
		defaultType = canonicalResourceType(rawDefaultType)
	}
	categoryID := parseUint(c.PostForm("category_id"))
	tagIDs := parseUintList(c.PostFormArray("tag_ids"))

	status := "pending"
	if strings.EqualFold(user.Role, "admin") {
		status = "approved"
	}

	var categoryPtr *uint
	if categoryID > 0 {
		categoryPtr = &categoryID
	}

	var tags []models.Tag
	if len(tagIDs) > 0 {
		app.DB.Find(&tags, tagIDs)
	}

	reader := csv.NewReader(src)
	reader.FieldsPerRecord = -1

	created := 0
	skipped := 0
	line := 0
	for {
		record, readErr := reader.Read()
		if readErr != nil {
			if readErr == io.EOF {
				break
			}
			skipped++
			continue
		}
		line++

		if line == 1 && looksLikeCSVHeader(record) {
			continue
		}

		title := csvCell(record, 0)
		description := csvCell(record, 1)
		resourceTypeRaw := csvCell(record, 2)
		resourceType := defaultType
		if resourceTypeRaw != "" {
			resourceType = canonicalResourceType(resourceTypeRaw)
		}
		link := csvCell(record, 3)

		if len([]rune(title)) < 2 || len([]rune(description)) < 5 {
			skipped++
			continue
		}
		if link != "" && !isValidHTTPURL(link) {
			skipped++
			continue
		}

		resource := models.Resource{
			Title:        title,
			Description:  description,
			ResourceType: resourceType,
			Link:         link,
			SourceType:   "import",
			Status:       status,
			UserID:       user.ID,
			CategoryID:   categoryPtr,
		}

		if err := app.DB.Create(&resource).Error; err != nil {
			app.Log.WithError(err).Warn("create imported resource failed")
			skipped++
			continue
		}
		if len(tags) > 0 {
			if err := app.DB.Model(&resource).Association("Tags").Append(tags); err != nil {
				app.Log.WithError(err).Warn("append tags for imported resource failed")
			}
		}
		created++
	}

	if created == 0 {
		app.setFlashKey(c, "flash_error", "flash.publish_import_empty")
		c.Redirect(http.StatusFound, "/publish")
		return
	}

	app.Cache.Flush()
	app.setFlash(c, "flash_success", app.tr(c, "flash.publish_import_success", created, skipped))
	c.Redirect(http.StatusFound, "/resources")
}

func (app *App) PublishCrawlerHandler(c *gin.Context) {
	user := c.MustGet("user").(*models.User)

	crawlURL := strings.TrimSpace(c.PostForm("crawl_url"))
	resourceType := canonicalResourceType(c.PostForm("resource_type"))
	categoryID := parseUint(c.PostForm("category_id"))
	tagIDs := parseUintList(c.PostFormArray("tag_ids"))

	if !isValidHTTPURL(crawlURL) {
		app.setFlashKey(c, "flash_error", "flash.publish_invalid_crawl_url")
		c.Redirect(http.StatusFound, "/publish")
		return
	}

	client := &http.Client{Timeout: 10 * time.Second}
	req, reqErr := http.NewRequest(http.MethodGet, crawlURL, nil)
	if reqErr != nil {
		app.setFlashKey(c, "flash_error", "flash.publish_crawl_failed")
		c.Redirect(http.StatusFound, "/publish")
		return
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; GalaxyHubCrawler/1.0)")

	resp, err := client.Do(req)
	if err != nil {
		app.Log.WithError(err).Warn("crawl request failed")
		app.setFlashKey(c, "flash_error", "flash.publish_crawl_failed")
		c.Redirect(http.StatusFound, "/publish")
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 400 {
		app.setFlash(c, "flash_error", app.tr(c, "flash.publish_crawl_bad_status", resp.StatusCode))
		c.Redirect(http.StatusFound, "/publish")
		return
	}

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		app.Log.WithError(err).Warn("parse crawled page failed")
		app.setFlashKey(c, "flash_error", "flash.publish_crawl_parse_failed")
		c.Redirect(http.StatusFound, "/publish")
		return
	}

	title := strings.TrimSpace(doc.Find("meta[property='og:title']").AttrOr("content", ""))
	if title == "" {
		title = strings.TrimSpace(doc.Find("title").First().Text())
	}
	if title == "" {
		parsed, parseErr := url.Parse(crawlURL)
		if parseErr == nil && parsed.Host != "" {
			title = parsed.Host
		} else {
			title = app.tr(c, "resource.crawl_fallback_title")
		}
	}

	description := strings.TrimSpace(doc.Find("meta[name='description']").AttrOr("content", ""))
	if description == "" {
		description = strings.TrimSpace(doc.Find("meta[property='og:description']").AttrOr("content", ""))
	}
	if description == "" {
		description = app.tr(c, "resource.crawl_fallback_desc", crawlURL)
	}

	status := "pending"
	if strings.EqualFold(user.Role, "admin") {
		status = "approved"
	}

	var categoryPtr *uint
	if categoryID > 0 {
		categoryPtr = &categoryID
	}

	resource := models.Resource{
		Title:        title,
		Description:  description,
		ResourceType: resourceType,
		Link:         crawlURL,
		SourceType:   "crawler",
		Status:       status,
		UserID:       user.ID,
		CategoryID:   categoryPtr,
	}

	if len(tagIDs) > 0 {
		var tags []models.Tag
		if err := app.DB.Find(&tags, tagIDs).Error; err == nil && len(tags) > 0 {
			resource.Tags = tags
		}
	}

	if err := app.DB.Create(&resource).Error; err != nil {
		app.Log.WithError(err).Error("publish crawler resource failed")
		app.setFlashKey(c, "flash_error", "flash.publish_failed")
		c.Redirect(http.StatusFound, "/publish")
		return
	}

	app.Cache.Flush()
	if resource.Status == "approved" {
		app.setFlashKey(c, "flash_success", "flash.publish_success_approved")
		c.Redirect(http.StatusFound, fmt.Sprintf("/resources/%d", resource.ID))
		return
	}
	app.setFlashKey(c, "flash_success", "flash.publish_success_pending")
	c.Redirect(http.StatusFound, "/resources")
}

func (app *App) ResourceDetailPage(c *gin.Context) {
	id := parseUint(c.Param("id"))
	if id == 0 {
		c.Redirect(http.StatusFound, "/resources")
		return
	}

	var resource models.Resource
	if err := app.DB.Preload("Category").Preload("Tags").Preload("User").First(&resource, id).Error; err != nil {
		c.Redirect(http.StatusFound, "/resources")
		return
	}

	if resource.Status != "approved" {
		c.Redirect(http.StatusFound, "/resources")
		return
	}

	// Increment views
	app.DB.Model(&resource).Update("views", gorm.Expr("views + ?", 1))

	// Calculate avg rating
	if resource.RatingCount > 0 {
		resource.RatingScore = resource.RatingScore / float64(resource.RatingCount)
	}

	// Comments
	var comments []models.Comment
	app.DB.Preload("User").Where("resource_id = ?", id).Order("created_at desc").Find(&comments)

	data := app.basePageData(c)
	data.Title = resource.Title
	data.ActiveNav = "resources"
	data.Resource = &resource
	data.Comments = comments

	app.renderPage(c, "resource_detail.html", data)
}

func (app *App) ArticlesPage(c *gin.Context) {
	data := app.basePageData(c)
	data.Title = app.tr(c, "article.title")
	data.ActiveNav = "articles"

	page := parseInt(c.DefaultQuery("page", "1"), 1)
	pageSize := 10
	data.Page = page
	data.PageSize = pageSize

	if payload, err := app.getArticleListPayload(page, pageSize); err == nil {
		data.Total = payload.Total
		data.TotalPages = int((payload.Total + int64(pageSize) - 1) / int64(pageSize))
		data.Articles = payload.Articles
	} else {
		app.Log.WithError(err).Warn("load article list cache payload failed")
	}

	app.renderPage(c, "articles.html", data)
}

func (app *App) ArticleDetailPage(c *gin.Context) {
	id := parseUint(c.Param("id"))
	if id == 0 {
		c.Redirect(http.StatusFound, "/articles")
		return
	}

	var article models.Article
	if err := app.DB.Preload("User").First(&article, id).Error; err != nil {
		c.Redirect(http.StatusFound, "/articles")
		return
	}

	if article.Status != "published" {
		c.Redirect(http.StatusFound, "/articles")
		return
	}

	app.DB.Model(&article).Update("views", gorm.Expr("views + ?", 1))

	var comments []models.Comment
	app.DB.Preload("User").Where("article_id = ?", id).Order("created_at desc").Find(&comments)

	data := app.basePageData(c)
	data.Title = article.Title
	data.ActiveNav = "articles"
	data.Article = &article
	data.Comments = comments

	app.renderPage(c, "article_detail.html", data)
}

func (app *App) LoginPage(c *gin.Context) {
	if _, exists := c.Get("user"); exists {
		c.Redirect(http.StatusFound, "/")
		return
	}

	data := app.basePageData(c)
	data.Title = app.tr(c, "auth.login_title")
	app.renderPage(c, "login.html", data)
}

func (app *App) LoginHandler(c *gin.Context) {
	username := strings.TrimSpace(c.PostForm("username"))
	password := c.PostForm("password")

	var user models.User
	if err := app.DB.Where("username = ?", username).First(&user).Error; err != nil {
		app.setFlashKey(c, "flash_error", "flash.invalid_credentials")
		c.Redirect(http.StatusFound, "/login")
		return
	}

	if user.Status != "active" {
		app.setFlashKey(c, "flash_error", "flash.account_disabled")
		c.Redirect(http.StatusFound, "/login")
		return
	}

	if !utils.CheckPassword(user.PasswordHash, password) {
		app.setFlashKey(c, "flash_error", "flash.invalid_credentials")
		c.Redirect(http.StatusFound, "/login")
		return
	}

	session := sessions.Default(c)
	session.Set("userID", user.ID)
	session.Save()

	app.setFlashKey(c, "flash_success", "flash.login_success")
	c.Redirect(http.StatusFound, "/")
}

func (app *App) RegisterPage(c *gin.Context) {
	if _, exists := c.Get("user"); exists {
		c.Redirect(http.StatusFound, "/")
		return
	}

	data := app.basePageData(c)
	data.Title = app.tr(c, "auth.register_title")
	app.renderPage(c, "register.html", data)
}

func (app *App) RegisterHandler(c *gin.Context) {
	username := strings.TrimSpace(c.PostForm("username"))
	password := c.PostForm("password")
	confirmPassword := c.PostForm("confirm_password")

	if len(username) < 3 || len(username) > 30 {
		app.setFlashKey(c, "flash_error", "flash.username_length")
		c.Redirect(http.StatusFound, "/register")
		return
	}

	if len(password) < 6 {
		app.setFlashKey(c, "flash_error", "flash.password_length")
		c.Redirect(http.StatusFound, "/register")
		return
	}

	if password != confirmPassword {
		app.setFlashKey(c, "flash_error", "flash.password_mismatch")
		c.Redirect(http.StatusFound, "/register")
		return
	}

	var existing models.User
	if err := app.DB.Where("username = ?", username).First(&existing).Error; err == nil {
		app.setFlashKey(c, "flash_error", "flash.username_exists")
		c.Redirect(http.StatusFound, "/register")
		return
	}

	hash, err := utils.HashPassword(password)
	if err != nil {
		app.setFlashKey(c, "flash_error", "flash.register_failed")
		c.Redirect(http.StatusFound, "/register")
		return
	}

	user := models.User{
		Username:     username,
		PasswordHash: hash,
		Role:         "user",
		Points:       50,
		Status:       "active",
	}
	if err := app.DB.Create(&user).Error; err != nil {
		app.setFlashKey(c, "flash_error", "flash.register_failed")
		c.Redirect(http.StatusFound, "/register")
		return
	}

	session := sessions.Default(c)
	session.Set("userID", user.ID)
	session.Save()

	app.Cache.Flush()
	app.setFlashKey(c, "flash_success", "flash.register_success")
	c.Redirect(http.StatusFound, "/")
}

func (app *App) LogoutHandler(c *gin.Context) {
	session := sessions.Default(c)
	session.Clear()
	session.Save()
	c.Redirect(http.StatusFound, "/")
}

func (app *App) ProfilePage(c *gin.Context) {
	user := c.MustGet("user").(*models.User)

	// Check if can check in today
	canCheckin := true
	if user.LastCheckinAt != nil {
		last := user.LastCheckinAt.Format("2006-01-02")
		today := time.Now().Format("2006-01-02")
		if last == today {
			canCheckin = false
		}
	}

	data := app.basePageData(c)
	data.Title = app.tr(c, "profile.title")
	data.CanCheckin = canCheckin

	app.renderPage(c, "profile.html", data)
}

func (app *App) CheckinHandler(c *gin.Context) {
	user := c.MustGet("user").(*models.User)

	now := time.Now()
	start, end := dayRange(now)
	if err := app.DB.Transaction(func(tx *gorm.DB) error {
		var lockedUser models.User
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&lockedUser, user.ID).Error; err != nil {
			return err
		}

		if lockedUser.LastCheckinAt != nil && !lockedUser.LastCheckinAt.Before(start) && lockedUser.LastCheckinAt.Before(end) {
			return errAlreadyCheckedIn
		}

		return tx.Model(&models.User{}).Where("id = ?", user.ID).Updates(map[string]interface{}{
			"points":          gorm.Expr("points + ?", checkinRewardPoint),
			"last_checkin_at": now,
		}).Error
	}); err != nil {
		if errors.Is(err, errAlreadyCheckedIn) {
			app.setFlashKey(c, "flash_error", "flash.checkin_done")
		} else {
			app.Log.WithError(err).Error("checkin transaction failed")
			app.setFlashKey(c, "flash_error", "flash.update_failed")
		}
		c.Redirect(http.StatusFound, "/profile")
		return
	}

	app.setFlashKey(c, "flash_success", "flash.checkin_success")
	c.Redirect(http.StatusFound, "/profile")
}

func (app *App) ShareHandler(c *gin.Context) {
	user := c.MustGet("user").(*models.User)
	resourceID := parseUint(c.Param("id"))

	if resourceID == 0 {
		c.Redirect(http.StatusFound, "/resources")
		return
	}

	start, end := dayRange(time.Now())
	if err := app.DB.Transaction(func(tx *gorm.DB) error {
		var lockedUser models.User
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Select("id").First(&lockedUser, user.ID).Error; err != nil {
			return err
		}

		var resource models.Resource
		if err := tx.Select("id", "status").First(&resource, resourceID).Error; err != nil {
			return err
		}
		if resource.Status != "approved" {
			return gorm.ErrRecordNotFound
		}

		var claimedCount int64
		if err := tx.Model(&models.ShareLog{}).
			Where("user_id = ? AND resource_id = ?", user.ID, resourceID).
			Count(&claimedCount).Error; err != nil {
			return err
		}
		if claimedCount > 0 {
			return errShareRewardLimited
		}

		if err := tx.Model(&models.ShareLog{}).
			Where("user_id = ? AND created_at >= ? AND created_at < ?", user.ID, start, end).
			Count(&claimedCount).Error; err != nil {
			return err
		}
		if claimedCount > 0 {
			return errShareRewardLimited
		}

		if err := tx.Create(&models.ShareLog{UserID: user.ID, ResourceID: resourceID}).Error; err != nil {
			return err
		}

		return tx.Model(&models.User{}).Where("id = ?", user.ID).
			Update("points", gorm.Expr("points + ?", shareRewardPoint)).Error
	}); err != nil {
		if errors.Is(err, errShareRewardLimited) {
			app.setFlashKey(c, "flash_error", "flash.share_reward_limited")
		} else {
			app.Log.WithError(err).WithField("resource_id", resourceID).Error("share reward failed")
			app.setFlashKey(c, "flash_error", "flash.share_failed")
		}
		c.Redirect(http.StatusFound, fmt.Sprintf("/resources/%d", resourceID))
		return
	}

	app.setFlashKey(c, "flash_success", "flash.share_success")
	c.Redirect(http.StatusFound, fmt.Sprintf("/resources/%d", resourceID))
}

func (app *App) ResourceCommentHandler(c *gin.Context) {
	userValue, exists := c.Get("user")
	if !exists {
		app.setFlashKey(c, "flash_error", "flash.login_required")
		c.Redirect(http.StatusFound, "/login")
		return
	}
	user, ok := userValue.(*models.User)
	if !ok || user == nil {
		app.setFlashKey(c, "flash_error", "flash.login_required")
		c.Redirect(http.StatusFound, "/login")
		return
	}

	resourceID := parseUint(c.Param("id"))
	content := strings.TrimSpace(c.PostForm("content"))
	rating := parseInt(c.PostForm("rating"), 0)

	if resourceID == 0 || content == "" || rating < 1 || rating > 5 {
		app.setFlashKey(c, "flash_error", "flash.comment_failed")
		if resourceID == 0 {
			c.Redirect(http.StatusFound, "/resources")
			return
		}
		c.Redirect(http.StatusFound, fmt.Sprintf("/resources/%d", resourceID))
		return
	}

	var resource models.Resource
	if err := app.DB.Select("id", "status").First(&resource, resourceID).Error; err != nil || resource.Status != "approved" {
		app.setFlashKey(c, "flash_error", "flash.comment_failed")
		c.Redirect(http.StatusFound, fmt.Sprintf("/resources/%d", resourceID))
		return
	}

	comment := models.Comment{
		Content:    content,
		Rating:     rating,
		UserID:     user.ID,
		ResourceID: ptrUint(resourceID),
	}

	if err := app.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&comment).Error; err != nil {
			return err
		}
		if err := tx.Model(&models.Resource{}).Where("id = ?", resourceID).Updates(map[string]interface{}{
			"rating_score": gorm.Expr("rating_score + ?", float64(rating)),
			"rating_count": gorm.Expr("rating_count + ?", 1),
		}).Error; err != nil {
			return err
		}
		return nil
	}); err != nil {
		app.Log.WithError(err).Error("create resource comment failed")
		app.setFlashKey(c, "flash_error", "flash.comment_failed")
		c.Redirect(http.StatusFound, fmt.Sprintf("/resources/%d", resourceID))
		return
	}

	app.setFlashKey(c, "flash_success", "flash.comment_success")
	app.Cache.Flush()
	c.Redirect(http.StatusFound, fmt.Sprintf("/resources/%d", resourceID))
}

func (app *App) ArticleCommentHandler(c *gin.Context) {
	userValue, exists := c.Get("user")
	if !exists {
		app.setFlashKey(c, "flash_error", "flash.login_required")
		c.Redirect(http.StatusFound, "/login")
		return
	}
	user, ok := userValue.(*models.User)
	if !ok || user == nil {
		app.setFlashKey(c, "flash_error", "flash.login_required")
		c.Redirect(http.StatusFound, "/login")
		return
	}

	articleID := parseUint(c.Param("id"))
	content := strings.TrimSpace(c.PostForm("content"))

	if articleID == 0 || content == "" {
		app.setFlashKey(c, "flash_error", "flash.comment_failed")
		if articleID == 0 {
			c.Redirect(http.StatusFound, "/articles")
			return
		}
		c.Redirect(http.StatusFound, fmt.Sprintf("/articles/%d", articleID))
		return
	}

	var article models.Article
	if err := app.DB.Select("id", "status").First(&article, articleID).Error; err != nil || article.Status != "published" {
		app.setFlashKey(c, "flash_error", "flash.comment_failed")
		c.Redirect(http.StatusFound, fmt.Sprintf("/articles/%d", articleID))
		return
	}

	comment := models.Comment{
		Content:   content,
		UserID:    user.ID,
		ArticleID: ptrUint(articleID),
	}
	if err := app.DB.Create(&comment).Error; err != nil {
		app.Log.WithError(err).Error("create article comment failed")
		app.setFlashKey(c, "flash_error", "flash.comment_failed")
		c.Redirect(http.StatusFound, fmt.Sprintf("/articles/%d", articleID))
		return
	}

	app.setFlashKey(c, "flash_success", "flash.comment_success")
	c.Redirect(http.StatusFound, fmt.Sprintf("/articles/%d", articleID))
}

// Admin Handlers
func (app *App) AdminDashboard(c *gin.Context) {
	data := app.basePageData(c)
	data.Title = app.tr(c, "admin.nav.dashboard")
	data.ActiveNav = "dashboard"

	var resourceCount, pendingCount, articleCount, userCount int64
	app.DB.Model(&models.Resource{}).Count(&resourceCount)
	app.DB.Model(&models.Resource{}).Where("status = ?", "pending").Count(&pendingCount)
	app.DB.Model(&models.Article{}).Count(&articleCount)
	app.DB.Model(&models.User{}).Count(&userCount)

	data.Stats = map[string]int64{
		"Resources": resourceCount,
		"Pending":   pendingCount,
		"Articles":  articleCount,
		"Users":     userCount,
	}

	// Pending resources
	var pendingResources []models.Resource
	app.DB.Preload("User").Where("status = ?", "pending").Order("created_at desc").Limit(10).Find(&pendingResources)
	data.PendingResources = pendingResources

	// Backups
	var backups []BackupInfo
	if err := os.MkdirAll(app.Cfg.BackupDir, 0755); err != nil {
		app.Log.WithError(err).Warn("create backup dir failed")
	}
	files, err := os.ReadDir(app.Cfg.BackupDir)
	if err != nil {
		app.Log.WithError(err).Warn("read backup dir failed")
	} else {
		for _, f := range files {
			if f.IsDir() {
				continue
			}
			filename := strings.ToLower(f.Name())
			if !strings.HasSuffix(filename, ".sql") && !strings.HasSuffix(filename, ".json") {
				continue
			}
			info, infoErr := f.Info()
			if infoErr != nil {
				app.Log.WithError(infoErr).Warn("read backup file info failed")
				continue
			}
			backups = append(backups, BackupInfo{
				Name: f.Name(),
				Time: info.ModTime().Format("2006-01-02 15:04"),
			})
		}
	}
	sort.Slice(backups, func(i, j int) bool {
		return backups[i].Name > backups[j].Name
	})
	data.Backups = backups

	app.renderAdminPage(c, "dashboard.html", data)
}

func (app *App) AdminUsersPage(c *gin.Context) {
	data := app.basePageData(c)
	data.Title = app.tr(c, "admin.nav.users")
	data.ActiveNav = "users"

	var users []models.User
	app.DB.Order("created_at desc").Find(&users)
	data.Users = users

	app.renderAdminPage(c, "users.html", data)
}

func (app *App) AdminUpdateUser(c *gin.Context) {
	userID := parseUint(c.Param("id"))
	role := c.PostForm("role")
	status := c.PostForm("status")

	if userID == 0 {
		app.setFlashKey(c, "flash_error", "flash.update_failed")
		c.Redirect(http.StatusFound, "/admin/users")
		return
	}

	var target models.User
	if err := app.DB.First(&target, userID).Error; err != nil {
		app.setFlashKey(c, "flash_error", "flash.user_not_found")
		c.Redirect(http.StatusFound, "/admin/users")
		return
	}

	updates := map[string]interface{}{}
	if role != "" {
		updates["role"] = role
	}
	if status != "" {
		if strings.EqualFold(target.Username, "admin") && status == "suspended" {
			app.setFlashKey(c, "flash_error", "flash.admin_cannot_disable")
			c.Redirect(http.StatusFound, "/admin/users")
			return
		}
		updates["status"] = status
	}

	if len(updates) > 0 {
		if err := app.DB.Model(&models.User{}).Where("id = ?", userID).Updates(updates).Error; err != nil {
			app.setFlashKey(c, "flash_error", "flash.update_failed")
			c.Redirect(http.StatusFound, "/admin/users")
			return
		}
	}

	app.setFlashKey(c, "flash_success", "flash.user_updated")
	c.Redirect(http.StatusFound, "/admin/users")
}

func (app *App) AdminResourcesPage(c *gin.Context) {
	data := app.basePageData(c)
	data.Title = app.tr(c, "admin.nav.resources")
	data.ActiveNav = "resources"

	status := c.Query("status")
	data.Status = status

	query := app.DB.Preload("Category").Preload("Tags").Preload("User")
	if status != "" {
		query = query.Where("status = ?", status)
	}

	var resources []models.Resource
	query.Order("created_at desc").Find(&resources)
	for i := range resources {
		if resources[i].RatingCount > 0 {
			resources[i].RatingScore = resources[i].RatingScore / float64(resources[i].RatingCount)
		}
	}
	data.Resources = resources

	app.renderAdminPage(c, "resources.html", data)
}

func (app *App) AdminReviewResource(c *gin.Context) {
	resourceID := parseUint(c.Param("id"))
	status := c.PostForm("status")

	if status == "approved" || status == "rejected" {
		app.DB.Model(&models.Resource{}).Where("id = ?", resourceID).Update("status", status)
		app.Cache.Flush()
	}

	app.setFlashKey(c, "flash_success", "flash.review_done")
	c.Redirect(http.StatusFound, "/admin/resources")
}

func (app *App) AdminArticlesPage(c *gin.Context) {
	data := app.basePageData(c)
	data.Title = app.tr(c, "admin.nav.articles")
	data.ActiveNav = "articles"

	var articles []models.Article
	app.DB.Preload("User").Order("created_at desc").Find(&articles)
	data.Articles = articles

	app.renderAdminPage(c, "articles.html", data)
}

func (app *App) AdminArticleFormPage(c *gin.Context) {
	data := app.basePageData(c)
	data.ActiveNav = "articles"

	idStr := c.Param("id")
	if idStr != "" {
		id := parseUint(idStr)
		var article models.Article
		if err := app.DB.First(&article, id).Error; err == nil {
			data.Article = &article
			data.Title = app.tr(c, "admin.article_form.edit_title")
		}
	} else {
		data.Title = app.tr(c, "admin.article_form.create_title")
	}

	app.renderAdminPage(c, "article_form.html", data)
}

func (app *App) AdminCreateArticle(c *gin.Context) {
	user := c.MustGet("user").(*models.User)

	article := models.Article{
		Title:   c.PostForm("title"),
		Summary: c.PostForm("summary"),
		Content: sanitizeArticleContent(c.PostForm("content")),
		Status:  "published",
		UserID:  user.ID,
	}
	app.DB.Create(&article)

	app.Cache.Flush()
	app.setFlashKey(c, "flash_success", "flash.article_created")
	c.Redirect(http.StatusFound, "/admin/articles")
}

func (app *App) AdminUpdateArticle(c *gin.Context) {
	articleID := parseUint(c.Param("id"))

	app.DB.Model(&models.Article{}).Where("id = ?", articleID).Updates(map[string]interface{}{
		"title":   c.PostForm("title"),
		"summary": c.PostForm("summary"),
		"content": sanitizeArticleContent(c.PostForm("content")),
	})

	app.Cache.Flush()
	app.setFlashKey(c, "flash_success", "flash.article_updated")
	c.Redirect(http.StatusFound, "/admin/articles")
}

func (app *App) AdminCategoriesPage(c *gin.Context) {
	data := app.basePageData(c)
	data.Title = app.tr(c, "admin.nav.categories")
	data.ActiveNav = "categories"

	var categories []models.Category
	app.DB.Order("created_at desc").Find(&categories)
	data.Categories = categories

	app.renderAdminPage(c, "categories.html", data)
}

func (app *App) AdminCreateCategory(c *gin.Context) {
	name := c.PostForm("name")
	if name != "" {
		app.DB.Create(&models.Category{Name: name})
		app.Cache.Flush()
	}

	app.setFlashKey(c, "flash_success", "flash.category_created")
	c.Redirect(http.StatusFound, "/admin/categories")
}

func (app *App) AdminUpdateCategory(c *gin.Context) {
	categoryID := parseUint(c.Param("id"))
	name := c.PostForm("name")

	if name != "" {
		app.DB.Model(&models.Category{}).Where("id = ?", categoryID).Update("name", name)
		app.Cache.Flush()
	}

	app.setFlashKey(c, "flash_success", "flash.category_updated")
	c.Redirect(http.StatusFound, "/admin/categories")
}

func (app *App) AdminDeleteCategory(c *gin.Context) {
	categoryID := parseUint(c.Param("id"))
	if categoryID == 0 {
		app.setFlashKey(c, "flash_error", "flash.category_delete_failed")
		c.Redirect(http.StatusFound, "/admin/categories")
		return
	}

	err := app.DB.Transaction(func(tx *gorm.DB) error {
		var category models.Category
		if err := tx.Select("id").First(&category, categoryID).Error; err != nil {
			return err
		}

		if err := tx.Model(&models.Resource{}).Where("category_id = ?", categoryID).Update("category_id", nil).Error; err != nil {
			return err
		}

		result := tx.Delete(&models.Category{}, categoryID)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return gorm.ErrRecordNotFound
		}
		return nil
	})
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			app.setFlashKey(c, "flash_error", "flash.category_not_found")
		} else {
			app.Log.WithError(err).WithField("category_id", categoryID).Error("delete category failed")
			app.setFlashKey(c, "flash_error", "flash.category_delete_failed")
		}
		c.Redirect(http.StatusFound, "/admin/categories")
		return
	}

	app.Cache.Flush()
	app.setFlashKey(c, "flash_success", "flash.category_deleted")
	c.Redirect(http.StatusFound, "/admin/categories")
}

func (app *App) AdminTagsPage(c *gin.Context) {
	data := app.basePageData(c)
	data.Title = app.tr(c, "admin.nav.tags")
	data.ActiveNav = "tags"

	var tags []models.Tag
	app.DB.Order("created_at desc").Find(&tags)
	data.Tags = tags

	app.renderAdminPage(c, "tags.html", data)
}

func (app *App) AdminCreateTag(c *gin.Context) {
	name := c.PostForm("name")
	if name != "" {
		app.DB.Create(&models.Tag{Name: name})
		app.Cache.Flush()
	}

	app.setFlashKey(c, "flash_success", "flash.tag_created")
	c.Redirect(http.StatusFound, "/admin/tags")
}

func (app *App) AdminUpdateTag(c *gin.Context) {
	tagID := parseUint(c.Param("id"))
	name := c.PostForm("name")

	if name != "" {
		app.DB.Model(&models.Tag{}).Where("id = ?", tagID).Update("name", name)
		app.Cache.Flush()
	}

	app.setFlashKey(c, "flash_success", "flash.tag_updated")
	c.Redirect(http.StatusFound, "/admin/tags")
}

func (app *App) AdminDeleteTag(c *gin.Context) {
	tagID := parseUint(c.Param("id"))
	app.DB.Delete(&models.Tag{}, tagID)
	app.Cache.Flush()

	app.setFlashKey(c, "flash_success", "flash.tag_deleted")
	c.Redirect(http.StatusFound, "/admin/tags")
}

func (app *App) AdminSettingsPage(c *gin.Context) {
	data := app.basePageData(c)
	data.Title = app.tr(c, "admin.nav.settings")
	data.ActiveNav = "settings"

	if cfg, err := app.getSiteConfig(); err == nil {
		data.Config = &cfg
	} else {
		app.Log.WithError(err).Warn("load cached site config for admin settings failed")
	}

	app.renderAdminPage(c, "settings.html", data)
}

func (app *App) AdminUpdateSettings(c *gin.Context) {
	var cfg models.SiteConfig
	app.DB.First(&cfg)

	cfg.SiteName = c.PostForm("site_name")
	cfg.SiteDesc = c.PostForm("site_desc")
	cfg.HeroTitle = c.PostForm("hero_title")
	cfg.HeroSubtitle = c.PostForm("hero_subtitle")
	cfg.LocaleDefault = c.PostForm("locale_default")
	cfg.AdsJSON = c.PostForm("ads_json")

	app.DB.Save(&cfg)
	app.Cache.Flush()

	app.setFlashKey(c, "flash_success", "flash.settings_updated")
	c.Redirect(http.StatusFound, "/admin/settings")
}

func (app *App) AdminBackup(c *gin.Context) {
	if err := os.MkdirAll(app.Cfg.BackupDir, 0755); err != nil {
		app.Log.WithError(err).Error("create backup dir failed")
		app.setFlashKey(c, "flash_error", "flash.backup_failed")
		c.Redirect(http.StatusFound, "/admin")
		return
	}

	snapshot, err := app.collectBackupSnapshot()
	if err != nil {
		app.Log.WithError(err).Error("collect backup snapshot failed")
		app.setFlashKey(c, "flash_error", "flash.backup_failed")
		c.Redirect(http.StatusFound, "/admin")
		return
	}

	filename := fmt.Sprintf("backup_%s.json", time.Now().Format("20060102_150405"))
	backupPath := filepath.Join(app.Cfg.BackupDir, filename)
	if err := writeSnapshotAtomic(backupPath, snapshot); err != nil {
		app.Log.WithError(err).Error("write backup file failed")
		app.setFlashKey(c, "flash_error", "flash.backup_failed")
		c.Redirect(http.StatusFound, "/admin")
		return
	}

	app.setFlashKey(c, "flash_success", "flash.backup_success")
	c.Redirect(http.StatusFound, "/admin")
}

func (app *App) AdminRestore(c *gin.Context) {
	filename := c.PostForm("filename")
	if filename == "" {
		app.setFlashKey(c, "flash_error", "flash.select_backup")
		c.Redirect(http.StatusFound, "/admin")
		return
	}

	backupPath, err := app.resolveBackupPath(filename)
	if err != nil {
		app.Log.WithError(err).WithField("filename", filename).Warn("invalid backup file path")
		app.setFlashKey(c, "flash_error", "flash.invalid_backup_file")
		c.Redirect(http.StatusFound, "/admin")
		return
	}

	switch strings.ToLower(filepath.Ext(backupPath)) {
	case ".json":
		err = app.restoreFromJSONBackup(backupPath)
	case ".sql":
		err = app.restoreFromSQLBackup(backupPath)
	default:
		err = fmt.Errorf("unsupported backup extension: %s", filepath.Ext(backupPath))
	}
	if err != nil {
		app.Log.WithError(err).WithField("backup", backupPath).Error("restore backup failed")
		app.setFlashKey(c, "flash_error", "flash.restore_failed")
		c.Redirect(http.StatusFound, "/admin")
		return
	}

	app.Cache.Flush()
	app.setFlashKey(c, "flash_success", "flash.restore_success")
	c.Redirect(http.StatusFound, "/admin")
}

// Helpers
func (app *App) collectBackupSnapshot() (*BackupSnapshot, error) {
	snapshot := &BackupSnapshot{
		Version:     1,
		GeneratedAt: time.Now().UTC(),
	}

	if err := app.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Order("id asc").Find(&snapshot.Users).Error; err != nil {
			return err
		}
		if err := tx.Order("id asc").Find(&snapshot.Categories).Error; err != nil {
			return err
		}
		if err := tx.Order("id asc").Find(&snapshot.Tags).Error; err != nil {
			return err
		}
		if err := tx.Omit(clause.Associations).Order("id asc").Find(&snapshot.Resources).Error; err != nil {
			return err
		}
		if err := tx.Omit(clause.Associations).Order("id asc").Find(&snapshot.Articles).Error; err != nil {
			return err
		}
		if err := tx.Omit(clause.Associations).Order("id asc").Find(&snapshot.Comments).Error; err != nil {
			return err
		}
		if err := tx.Order("id asc").Find(&snapshot.SiteConfigs).Error; err != nil {
			return err
		}
		if err := tx.Order("id asc").Find(&snapshot.ShareLogs).Error; err != nil {
			return err
		}
		if err := tx.Order("resource_id asc, tag_id asc").Find(&snapshot.ResourceTags).Error; err != nil {
			return err
		}
		return nil
	}); err != nil {
		return nil, err
	}

	return snapshot, nil
}

func writeSnapshotAtomic(path string, snapshot *BackupSnapshot) error {
	payload, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return err
	}

	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, payload, 0644); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}

	return nil
}

func (app *App) resolveBackupPath(filename string) (string, error) {
	filename = strings.TrimSpace(filename)
	if filename == "" {
		return "", errors.New("filename is empty")
	}
	if filename != filepath.Base(filename) {
		return "", fmt.Errorf("invalid filename: %s", filename)
	}
	ext := strings.ToLower(filepath.Ext(filename))
	if ext != ".json" && ext != ".sql" {
		return "", fmt.Errorf("unsupported backup extension: %s", ext)
	}

	baseAbs, err := filepath.Abs(app.Cfg.BackupDir)
	if err != nil {
		return "", err
	}
	targetAbs, err := filepath.Abs(filepath.Join(baseAbs, filename))
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(baseAbs, targetAbs)
	if err != nil {
		return "", err
	}
	if rel == "." || strings.HasPrefix(rel, "..") {
		return "", fmt.Errorf("invalid backup file path: %s", filename)
	}
	info, err := os.Stat(targetAbs)
	if err != nil {
		return "", err
	}
	if info.IsDir() {
		return "", fmt.Errorf("backup file is directory: %s", filename)
	}

	return targetAbs, nil
}

func (app *App) restoreFromJSONBackup(path string) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	var snapshot BackupSnapshot
	if err := json.Unmarshal(raw, &snapshot); err != nil {
		return err
	}

	return app.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("SET FOREIGN_KEY_CHECKS = 0").Error; err != nil {
			return err
		}
		defer func() {
			if enableErr := tx.Exec("SET FOREIGN_KEY_CHECKS = 1").Error; enableErr != nil {
				app.Log.WithError(enableErr).Warn("re-enable foreign key checks failed")
			}
		}()

		for _, table := range []string{
			"resource_tags",
			"share_logs",
			"comments",
			"articles",
			"resources",
			"tags",
			"categories",
			"users",
			"site_configs",
		} {
			if err := tx.Exec("DELETE FROM `" + table + "`").Error; err != nil {
				return err
			}
		}

		if err := createRows(tx, "users", snapshot.Users, false); err != nil {
			return err
		}
		if err := createRows(tx, "categories", snapshot.Categories, false); err != nil {
			return err
		}
		if err := createRows(tx, "tags", snapshot.Tags, false); err != nil {
			return err
		}
		if err := createRows(tx, "resources", snapshot.Resources, true); err != nil {
			return err
		}
		if err := createRows(tx, "articles", snapshot.Articles, true); err != nil {
			return err
		}
		if err := createRows(tx, "comments", snapshot.Comments, true); err != nil {
			return err
		}
		if err := createRows(tx, "site_configs", snapshot.SiteConfigs, false); err != nil {
			return err
		}
		if err := createRows(tx, "share_logs", snapshot.ShareLogs, false); err != nil {
			return err
		}
		if err := createRows(tx, "resource_tags", snapshot.ResourceTags, false); err != nil {
			return err
		}

		if err := tx.Exec("SET FOREIGN_KEY_CHECKS = 1").Error; err != nil {
			return err
		}
		return nil
	})
}

func (app *App) restoreFromSQLBackup(path string) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	statements := splitSQLStatements(string(raw))
	if len(statements) == 0 {
		return errors.New("no executable SQL statements found")
	}

	return app.DB.Transaction(func(tx *gorm.DB) error {
		for _, statement := range statements {
			normalized := strings.TrimSpace(statement)
			if normalized == "" {
				continue
			}
			upper := strings.ToUpper(normalized)
			if strings.HasPrefix(upper, "DELIMITER ") {
				continue
			}
			if err := tx.Exec(normalized).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func createRows[T any](tx *gorm.DB, table string, rows []T, omitAssociations bool) error {
	if len(rows) == 0 {
		return nil
	}
	db := tx.Table(table)
	if omitAssociations {
		db = db.Omit(clause.Associations)
	}
	return db.CreateInBatches(rows, 200).Error
}

func splitSQLStatements(sqlText string) []string {
	statements := make([]string, 0)
	var current strings.Builder

	inSingleQuote := false
	inDoubleQuote := false
	inBacktick := false
	inLineComment := false
	inBlockComment := false
	escaped := false

	runes := []rune(sqlText)
	for i := 0; i < len(runes); i++ {
		ch := runes[i]
		next := rune(0)
		if i+1 < len(runes) {
			next = runes[i+1]
		}

		if inLineComment {
			if ch == '\n' {
				inLineComment = false
			}
			continue
		}
		if inBlockComment {
			if ch == '*' && next == '/' {
				inBlockComment = false
				i++
			}
			continue
		}

		if !inSingleQuote && !inDoubleQuote && !inBacktick {
			if ch == '-' && next == '-' {
				after := rune(0)
				if i+2 < len(runes) {
					after = runes[i+2]
				}
				if after == 0 || after == ' ' || after == '\t' || after == '\n' || after == '\r' {
					inLineComment = true
					i++
					continue
				}
			}
			if ch == '#' {
				inLineComment = true
				continue
			}
			if ch == '/' && next == '*' {
				inBlockComment = true
				i++
				continue
			}
		}

		if ch == '\'' && !inDoubleQuote && !inBacktick && !escaped {
			inSingleQuote = !inSingleQuote
		} else if ch == '"' && !inSingleQuote && !inBacktick && !escaped {
			inDoubleQuote = !inDoubleQuote
		} else if ch == '`' && !inSingleQuote && !inDoubleQuote {
			inBacktick = !inBacktick
		}

		if ch == ';' && !inSingleQuote && !inDoubleQuote && !inBacktick {
			statement := strings.TrimSpace(current.String())
			if statement != "" {
				statements = append(statements, statement)
			}
			current.Reset()
			escaped = false
			continue
		}

		current.WriteRune(ch)

		if (inSingleQuote || inDoubleQuote) && ch == '\\' && !escaped {
			escaped = true
		} else {
			escaped = false
		}
	}

	statement := strings.TrimSpace(current.String())
	if statement != "" {
		statements = append(statements, statement)
	}

	return statements
}

func parseInt(s string, defaultVal int) int {
	if v, err := strconv.Atoi(s); err == nil && v > 0 {
		return v
	}
	return defaultVal
}

func parseUint(s string) uint {
	if v, err := strconv.ParseUint(s, 10, 64); err == nil {
		return uint(v)
	}
	return 0
}

func ptrUint(v uint) *uint {
	return &v
}

func parseUintList(values []string) []uint {
	result := make([]uint, 0, len(values))
	seen := make(map[uint]struct{})
	for _, value := range values {
		parsed := parseUint(value)
		if parsed == 0 {
			continue
		}
		if _, exists := seen[parsed]; exists {
			continue
		}
		seen[parsed] = struct{}{}
		result = append(result, parsed)
	}
	return result
}

func csvCell(record []string, index int) string {
	if index < 0 || index >= len(record) {
		return ""
	}
	return strings.TrimSpace(record[index])
}

func looksLikeCSVHeader(record []string) bool {
	if len(record) == 0 {
		return false
	}
	first := strings.ToLower(strings.TrimSpace(record[0]))
	return first == "title" || first == "标题"
}

func isValidHTTPURL(raw string) bool {
	parsed, err := url.Parse(raw)
	if err != nil {
		return false
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return false
	}
	return strings.TrimSpace(parsed.Host) != ""
}

func (app *App) adText(locale, key, fallback string) string {
	value := strings.TrimSpace(app.I18n.T(locale, key))
	if value == "" || value == key {
		return fallback
	}
	return value
}

func (app *App) defaultAdSlots(locale string) map[string]AdSlot {
	defaultContact := app.adText(locale, "ad.default_contact", "Business Contact: WeChat GalaxyHub_BD / Email business@galaxyhub.example")
	return map[string]AdSlot{
		"hero-banner": {
			Code:    "hero-banner",
			Title:   app.adText(locale, "ad.slot_defaults.hero_banner.title", "品牌合作专区"),
			Desc:    app.adText(locale, "ad.slot_defaults.hero_banner.desc", "预留标准广告位，支持后续广告系统对接。"),
			CTA:     app.adText(locale, "ad.slot_defaults.hero_banner.cta", app.adText(locale, "ad.learn_more", "Learn More")),
			Contact: app.adText(locale, "ad.slot_defaults.hero_banner.contact", defaultContact),
			Enabled: true,
		},
		"resource-list-inline": {
			Code:    "resource-list-inline",
			Title:   app.adText(locale, "ad.slot_defaults.resource_list_inline.title", "精选推广资源位"),
			Desc:    app.adText(locale, "ad.slot_defaults.resource_list_inline.desc", "资源列表内标准广告位，可按 code 接入投放系统。"),
			CTA:     app.adText(locale, "ad.slot_defaults.resource_list_inline.cta", app.adText(locale, "ad.learn_more", "Learn More")),
			Contact: app.adText(locale, "ad.slot_defaults.resource_list_inline.contact", defaultContact),
			Enabled: true,
		},
		"resource-detail-sidebar": {
			Code:    "resource-detail-sidebar",
			Title:   app.adText(locale, "ad.slot_defaults.resource_detail_sidebar.title", "侧边广告位"),
			Desc:    app.adText(locale, "ad.slot_defaults.resource_detail_sidebar.desc", "详情页侧栏广告位，支持图文与跳转链接。"),
			CTA:     app.adText(locale, "ad.slot_defaults.resource_detail_sidebar.cta", app.adText(locale, "ad.learn_more", "Learn More")),
			Contact: app.adText(locale, "ad.slot_defaults.resource_detail_sidebar.contact", defaultContact),
			Enabled: true,
		},
		"article-detail-inline": {
			Code:    "article-detail-inline",
			Title:   app.adText(locale, "ad.slot_defaults.article_detail_inline.title", "内容推广位"),
			Desc:    app.adText(locale, "ad.slot_defaults.article_detail_inline.desc", "文章详情中部广告位，便于后续扩展素材形式。"),
			CTA:     app.adText(locale, "ad.slot_defaults.article_detail_inline.cta", app.adText(locale, "ad.learn_more", "Learn More")),
			Contact: app.adText(locale, "ad.slot_defaults.article_detail_inline.contact", defaultContact),
			Enabled: true,
		},
		"global-footer": {
			Code:    "global-footer",
			Title:   app.adText(locale, "ad.slot_defaults.global_footer.title", "全站底部广告位"),
			Desc:    app.adText(locale, "ad.slot_defaults.global_footer.desc", "全站统一广告位，适合品牌露出。"),
			CTA:     app.adText(locale, "ad.slot_defaults.global_footer.cta", app.adText(locale, "ad.learn_more", "Learn More")),
			Contact: app.adText(locale, "ad.slot_defaults.global_footer.contact", "Business Contact: WeChat GalaxyHub_Service / Email contact@galaxyhub.example"),
			Enabled: true,
		},
	}
}

func (app *App) parseAdSlots(adsJSON, locale string) map[string]AdSlot {
	slots := app.defaultAdSlots(locale)
	if strings.TrimSpace(adsJSON) == "" {
		return slots
	}

	var cfg adConfig
	if err := json.Unmarshal([]byte(adsJSON), &cfg); err != nil {
		return slots
	}
	if isLegacySeedAdConfig(cfg) {
		return slots
	}

	for _, incoming := range cfg.Slots {
		code := strings.TrimSpace(incoming.Code)
		if code == "" {
			continue
		}

		slot, ok := slots[code]
		if !ok {
			slot = AdSlot{
				Code:    code,
				CTA:     app.adText(locale, "ad.learn_more", "Learn More"),
				Contact: app.adText(locale, "ad.default_contact", "Business Contact: WeChat GalaxyHub_BD / Email business@galaxyhub.example"),
				Enabled: true,
			}
		}

		if title := strings.TrimSpace(incoming.Title); title != "" {
			slot.Title = title
		}
		if desc := strings.TrimSpace(incoming.Desc); desc != "" {
			slot.Desc = desc
		}
		if link := strings.TrimSpace(incoming.Link); link != "" {
			slot.Link = link
		}
		if imageURL := strings.TrimSpace(incoming.ImageURL); imageURL != "" {
			slot.ImageURL = imageURL
		}
		if cta := strings.TrimSpace(incoming.CTA); cta != "" {
			slot.CTA = cta
		}
		if contact := strings.TrimSpace(incoming.Contact); contact != "" {
			slot.Contact = contact
		}
		if incoming.Enabled != nil {
			slot.Enabled = *incoming.Enabled
		}

		slots[code] = slot
	}

	return slots
}

func isLegacySeedAdConfig(cfg adConfig) bool {
	if len(cfg.Slots) != 1 {
		return false
	}
	slot := cfg.Slots[0]
	if strings.TrimSpace(slot.Code) != "hero-banner" {
		return false
	}
	title := strings.TrimSpace(slot.Title)
	desc := strings.TrimSpace(slot.Desc)
	if title != "品牌合作专区" {
		return false
	}
	if desc != "预留标准广告位，支持后续对接" && desc != "预留标准广告位，支持后续广告系统对接。" {
		return false
	}
	return strings.TrimSpace(slot.Link) == "" &&
		strings.TrimSpace(slot.ImageURL) == "" &&
		strings.TrimSpace(slot.CTA) == "" &&
		strings.TrimSpace(slot.Contact) == ""
}

func toIntOrZero(v interface{}) int {
	switch n := v.(type) {
	case int:
		return n
	case int8:
		return int(n)
	case int16:
		return int(n)
	case int32:
		return int(n)
	case int64:
		return int(n)
	case uint:
		return int(n)
	case uint8:
		return int(n)
	case uint16:
		return int(n)
	case uint32:
		return int(n)
	case uint64:
		return int(n)
	case float32:
		return int(n)
	case float64:
		return int(n)
	case string:
		if parsed, err := strconv.Atoi(strings.TrimSpace(n)); err == nil {
			return parsed
		}
	}
	return 0
}

func canonicalResourceType(resourceType string) string {
	normalized := strings.ToLower(strings.TrimSpace(resourceType))
	switch normalized {
	case "software", "软件", "软件工具", "tool":
		return "software"
	case "document", "文档", "资料", "电子书", "电子资料":
		return "document"
	case "video", "视频", "电影", "电影资源":
		return "video"
	case "audio", "音频":
		return "audio"
	case "other", "其他":
		return "other"
	default:
		return "other"
	}
}

func canonicalSourceType(sourceType string) string {
	normalized := strings.ToLower(strings.TrimSpace(sourceType))
	switch normalized {
	case "manual", "手动", "手动录入":
		return "manual"
	case "crawler", "crawl", "爬虫", "爬虫抓取":
		return "crawler"
	case "import", "bulk", "csv", "导入", "批量导入":
		return "import"
	default:
		return "manual"
	}
}

func buildLangSwitchURL(path, rawQuery, lang string) string {
	values, err := url.ParseQuery(rawQuery)
	if err != nil {
		values = make(url.Values)
	}
	values.Set("lang", lang)
	encoded := values.Encode()
	if encoded == "" {
		return path
	}
	return path + "?" + encoded
}

func buildRatingStars(score float64) string {
	stars := int(score + 0.5)
	if stars < 0 {
		stars = 0
	}
	if stars > 5 {
		stars = 5
	}
	return strings.Repeat("★", stars) + strings.Repeat("☆", 5-stars)
}
