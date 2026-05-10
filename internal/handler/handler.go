package handler

import (
	"errors"
	"fmt"
	"html/template"
	"net/http"
	"strconv"
	"strings"
	"time"

	"prompt736/internal/app"
	"prompt736/internal/models"
	"prompt736/internal/service"
	tmplPkg "prompt736/internal/template"
	"prompt736/internal/utils"

	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// Handler 封装前台页面所有 HTTP 处理方法
type Handler struct {
	app *app.App
	svc *service.Service
}

// New 创建 Handler 实例
func New(a *app.App, s *service.Service) *Handler {
	return &Handler{app: a, svc: s}
}

// ---- 公共辅助方法 ----

// setFlash 在 session 中设置 flash 消息
func (h *Handler) setFlash(c *gin.Context, key, value string) {
	session := sessions.Default(c)
	session.Set(key, value)
	session.Save()
}

// setFlashKey 设置 i18n key 对应的翻译文本为 flash 消息
func (h *Handler) setFlashKey(c *gin.Context, flashKey, i18nKey string, args ...interface{}) {
	h.setFlash(c, flashKey, h.tr(c, i18nKey, args...))
}

// tr 根据 context 获取翻译文本
func (h *Handler) tr(c *gin.Context, key string, args ...interface{}) string {
	siteConfig := c.MustGet("siteConfig").(models.SiteConfig)
	defaultLocale := h.app.I18n.Resolve(siteConfig.LocaleDefault, "zh")
	locale := c.GetString("locale")
	if locale == "" {
		locale = defaultLocale
	}
	return h.app.I18n.T(locale, key, args...)
}

// basePageData 构造页面通用数据
func (h *Handler) basePageData(c *gin.Context) app.PageData {
	siteConfig := c.MustGet("siteConfig").(models.SiteConfig)
	defaultLocale := h.app.I18n.Resolve(siteConfig.LocaleDefault, "zh")
	locale := c.GetString("locale")
	if locale == "" {
		locale = defaultLocale
	}

	data := app.PageData{
		SiteConfig:       siteConfig,
		Ads:              h.svc.ParseAdSlots(siteConfig.AdsJSON, locale),
		Locale:           locale,
		RequestPath:      c.Request.URL.Path,
		RawQuery:         c.Request.URL.RawQuery,
		CSRFToken:        c.GetString("csrfToken"),
		PublishMinPoints: service.PublishMinPoints,
	}

	if user, exists := c.Get("user"); exists {
		data.User = user.(*models.User)
		data.CanPublish = service.CanPublishResource(data.User)
	}

	if flash, exists := c.Get("flash_success"); exists {
		data.FlashSuccess = flash.(string)
	}
	if flash, exists := c.Get("flash_error"); exists {
		data.FlashError = flash.(string)
	}

	return data
}

// renderPage 渲染前台页面模板
func (h *Handler) renderPage(c *gin.Context, templateFile string, data app.PageData) {
	tmpl := template.Must(template.New("").Funcs(tmplPkg.BuildTemplateFuncs(h.app, data)).ParseFiles(
		"templates/layouts/base.html",
		"templates/pages/"+templateFile,
	))

	c.Header("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.ExecuteTemplate(c.Writer, "base.html", data); err != nil {
		h.app.Log.WithError(err).Error("template render failed")
		c.String(http.StatusInternalServerError, h.tr(c, "flash.render_failed"))
	}
}

// ---- 首页 ----

// HomePage 渲染首页
func (h *Handler) HomePage(c *gin.Context) {
	data := h.basePageData(c)
	data.Title = ""
	data.ActiveNav = "home"

	if payload, err := h.svc.GetHomePagePayload(); err == nil {
		data.Stats = payload.Stats
		data.HotResources = payload.HotResources
		data.LatestArticles = payload.LatestArticles
		data.Categories = payload.Categories
	} else {
		h.app.Log.WithError(err).Warn("load home page cache payload failed")
	}

	h.renderPage(c, "home.html", data)
}

// ---- 资源列表页 ----

// ResourcesPage 渲染资源列表页
func (h *Handler) ResourcesPage(c *gin.Context) {
	data := h.basePageData(c)
	data.Title = h.tr(c, "resource.library_title")
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

	if payload, err := h.svc.GetResourceListPayload(q, categoryID, tagID, resourceType, page, pageSize); err == nil {
		data.Categories = payload.Categories
		data.Tags = payload.Tags
		data.Total = payload.Total
		data.TotalPages = int((payload.Total + int64(pageSize) - 1) / int64(pageSize))
		data.Resources = payload.Resources
	} else {
		h.app.Log.WithError(err).Warn("load resource list cache payload failed")
	}

	h.renderPage(c, "resources.html", data)
}

// ---- 资源详情页 ----

// ResourceDetailPage 渲染资源详情页
func (h *Handler) ResourceDetailPage(c *gin.Context) {
	id := parseUint(c.Param("id"))
	if id == 0 {
		c.Redirect(http.StatusFound, "/resources")
		return
	}

	var resource models.Resource
	if err := h.app.DB.Preload("Category").Preload("Tags").Preload("User").First(&resource, id).Error; err != nil {
		c.Redirect(http.StatusFound, "/resources")
		return
	}

	if resource.Status != "approved" {
		c.Redirect(http.StatusFound, "/resources")
		return
	}

	h.app.DB.Model(&resource).Update("views", gorm.Expr("views + ?", 1))

	if resource.RatingCount > 0 {
		resource.RatingScore = resource.RatingScore / float64(resource.RatingCount)
	}

	var comments []models.Comment
	h.app.DB.Preload("User").Where("resource_id = ?", id).Order("created_at desc").Find(&comments)

	data := h.basePageData(c)
	data.Title = resource.Title
	data.ActiveNav = "resources"
	data.Resource = &resource
	data.Comments = comments

	h.renderPage(c, "resource_detail.html", data)
}

// ---- 文章列表页 ----

// ArticlesPage 渲染文章列表页
func (h *Handler) ArticlesPage(c *gin.Context) {
	data := h.basePageData(c)
	data.Title = h.tr(c, "article.title")
	data.ActiveNav = "articles"

	page := parseInt(c.DefaultQuery("page", "1"), 1)
	pageSize := 10
	data.Page = page
	data.PageSize = pageSize

	if payload, err := h.svc.GetArticleListPayload(page, pageSize); err == nil {
		data.Total = payload.Total
		data.TotalPages = int((payload.Total + int64(pageSize) - 1) / int64(pageSize))
		data.Articles = payload.Articles
	} else {
		h.app.Log.WithError(err).Warn("load article list cache payload failed")
	}

	h.renderPage(c, "articles.html", data)
}

// ---- 文章详情页 ----

// ArticleDetailPage 渲染文章详情页
func (h *Handler) ArticleDetailPage(c *gin.Context) {
	id := parseUint(c.Param("id"))
	if id == 0 {
		c.Redirect(http.StatusFound, "/articles")
		return
	}

	var article models.Article
	if err := h.app.DB.Preload("User").First(&article, id).Error; err != nil {
		c.Redirect(http.StatusFound, "/articles")
		return
	}

	if article.Status != "published" {
		c.Redirect(http.StatusFound, "/articles")
		return
	}

	h.app.DB.Model(&article).Update("views", gorm.Expr("views + ?", 1))

	var comments []models.Comment
	h.app.DB.Preload("User").Where("article_id = ?", id).Order("created_at desc").Find(&comments)

	data := h.basePageData(c)
	data.Title = article.Title
	data.ActiveNav = "articles"
	data.Article = &article
	data.Comments = comments

	h.renderPage(c, "article_detail.html", data)
}

// ---- 登录页 ----

// LoginPage 渲染登录页
func (h *Handler) LoginPage(c *gin.Context) {
	if _, exists := c.Get("user"); exists {
		c.Redirect(http.StatusFound, "/")
		return
	}

	data := h.basePageData(c)
	data.Title = h.tr(c, "auth.login_title")
	h.renderPage(c, "login.html", data)
}

// LoginHandler 处理登录表单提交
func (h *Handler) LoginHandler(c *gin.Context) {
	username := strings.TrimSpace(c.PostForm("username"))
	password := c.PostForm("password")

	var user models.User
	if err := h.app.DB.Where("username = ?", username).First(&user).Error; err != nil {
		h.setFlashKey(c, "flash_error", "flash.invalid_credentials")
		c.Redirect(http.StatusFound, "/login")
		return
	}

	if user.Status != "active" {
		h.setFlashKey(c, "flash_error", "flash.account_disabled")
		c.Redirect(http.StatusFound, "/login")
		return
	}

	if !utils.CheckPassword(user.PasswordHash, password) {
		h.setFlashKey(c, "flash_error", "flash.invalid_credentials")
		c.Redirect(http.StatusFound, "/login")
		return
	}

	session := sessions.Default(c)
	session.Set("userID", user.ID)
	session.Save()

	h.setFlashKey(c, "flash_success", "flash.login_success")
	c.Redirect(http.StatusFound, "/")
}

// ---- 注册页 ----

// RegisterPage 渲染注册页
func (h *Handler) RegisterPage(c *gin.Context) {
	if _, exists := c.Get("user"); exists {
		c.Redirect(http.StatusFound, "/")
		return
	}

	data := h.basePageData(c)
	data.Title = h.tr(c, "auth.register_title")
	h.renderPage(c, "register.html", data)
}

// RegisterHandler 处理注册表单提交
func (h *Handler) RegisterHandler(c *gin.Context) {
	username := strings.TrimSpace(c.PostForm("username"))
	password := c.PostForm("password")
	confirmPassword := c.PostForm("confirm_password")

	if len(username) < 3 || len(username) > 30 {
		h.setFlashKey(c, "flash_error", "flash.username_length")
		c.Redirect(http.StatusFound, "/register")
		return
	}

	if len(password) < 6 {
		h.setFlashKey(c, "flash_error", "flash.password_length")
		c.Redirect(http.StatusFound, "/register")
		return
	}

	if password != confirmPassword {
		h.setFlashKey(c, "flash_error", "flash.password_mismatch")
		c.Redirect(http.StatusFound, "/register")
		return
	}

	var existing models.User
	if err := h.app.DB.Where("username = ?", username).First(&existing).Error; err == nil {
		h.setFlashKey(c, "flash_error", "flash.username_exists")
		c.Redirect(http.StatusFound, "/register")
		return
	}

	hash, err := utils.HashPassword(password)
	if err != nil {
		h.setFlashKey(c, "flash_error", "flash.register_failed")
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
	if err := h.app.DB.Create(&user).Error; err != nil {
		h.setFlashKey(c, "flash_error", "flash.register_failed")
		c.Redirect(http.StatusFound, "/register")
		return
	}

	session := sessions.Default(c)
	session.Set("userID", user.ID)
	session.Save()

	h.app.Cache.Flush()
	h.setFlashKey(c, "flash_success", "flash.register_success")
	c.Redirect(http.StatusFound, "/")
}

// ---- 登出 ----

// LogoutHandler 清除 session 并重定向到首页
func (h *Handler) LogoutHandler(c *gin.Context) {
	session := sessions.Default(c)
	session.Clear()
	session.Save()
	c.Redirect(http.StatusFound, "/")
}

// ---- 个人中心 ----

// ProfilePage 渲染个人中心页
func (h *Handler) ProfilePage(c *gin.Context) {
	user := c.MustGet("user").(*models.User)

	canCheckin := true
	if user.LastCheckinAt != nil {
		last := user.LastCheckinAt.Format("2006-01-02")
		today := time.Now().Format("2006-01-02")
		if last == today {
			canCheckin = false
		}
	}

	data := h.basePageData(c)
	data.Title = h.tr(c, "profile.title")
	data.CanCheckin = canCheckin

	h.renderPage(c, "profile.html", data)
}

// CheckinHandler 处理签到请求
func (h *Handler) CheckinHandler(c *gin.Context) {
	user := c.MustGet("user").(*models.User)

	if err := h.svc.Checkin(user); err != nil {
		if errors.Is(err, service.ErrAlreadyCheckedIn) {
			h.setFlashKey(c, "flash_error", "flash.checkin_done")
		} else {
			h.app.Log.WithError(err).Error("checkin transaction failed")
			h.setFlashKey(c, "flash_error", "flash.update_failed")
		}
		c.Redirect(http.StatusFound, "/profile")
		return
	}

	h.setFlashKey(c, "flash_success", "flash.checkin_success")
	c.Redirect(http.StatusFound, "/profile")
}

// ---- 分享奖励 ----

// ShareHandler 处理分享资源领取积分
func (h *Handler) ShareHandler(c *gin.Context) {
	user := c.MustGet("user").(*models.User)
	resourceID := parseUint(c.Param("id"))

	if resourceID == 0 {
		c.Redirect(http.StatusFound, "/resources")
		return
	}

	if err := h.svc.ClaimShareReward(user, resourceID); err != nil {
		if errors.Is(err, service.ErrShareRewardLimited) {
			h.setFlashKey(c, "flash_error", "flash.share_reward_limited")
		} else {
			h.app.Log.WithError(err).WithField("resource_id", resourceID).Error("share reward failed")
			h.setFlashKey(c, "flash_error", "flash.share_failed")
		}
		c.Redirect(http.StatusFound, fmt.Sprintf("/resources/%d", resourceID))
		return
	}

	h.setFlashKey(c, "flash_success", "flash.share_success")
	c.Redirect(http.StatusFound, fmt.Sprintf("/resources/%d", resourceID))
}

// ---- 评论 ----

// ResourceCommentHandler 处理资源评论提交
func (h *Handler) ResourceCommentHandler(c *gin.Context) {
	userValue, exists := c.Get("user")
	if !exists {
		h.setFlashKey(c, "flash_error", "flash.login_required")
		c.Redirect(http.StatusFound, "/login")
		return
	}
	user, ok := userValue.(*models.User)
	if !ok || user == nil {
		h.setFlashKey(c, "flash_error", "flash.login_required")
		c.Redirect(http.StatusFound, "/login")
		return
	}

	resourceID := parseUint(c.Param("id"))
	content := strings.TrimSpace(c.PostForm("content"))
	rating := parseInt(c.PostForm("rating"), 0)

	if resourceID == 0 || content == "" || rating < 1 || rating > 5 {
		h.setFlashKey(c, "flash_error", "flash.comment_failed")
		if resourceID == 0 {
			c.Redirect(http.StatusFound, "/resources")
			return
		}
		c.Redirect(http.StatusFound, fmt.Sprintf("/resources/%d", resourceID))
		return
	}

	var resource models.Resource
	if err := h.app.DB.Select("id", "status").First(&resource, resourceID).Error; err != nil || resource.Status != "approved" {
		h.setFlashKey(c, "flash_error", "flash.comment_failed")
		c.Redirect(http.StatusFound, fmt.Sprintf("/resources/%d", resourceID))
		return
	}

	if err := h.svc.CreateResourceComment(user.ID, resourceID, content, rating); err != nil {
		h.app.Log.WithError(err).Error("create resource comment failed")
		h.setFlashKey(c, "flash_error", "flash.comment_failed")
		c.Redirect(http.StatusFound, fmt.Sprintf("/resources/%d", resourceID))
		return
	}

	h.setFlashKey(c, "flash_success", "flash.comment_success")
	h.app.Cache.Flush()
	c.Redirect(http.StatusFound, fmt.Sprintf("/resources/%d", resourceID))
}

// ArticleCommentHandler 处理文章评论提交
func (h *Handler) ArticleCommentHandler(c *gin.Context) {
	userValue, exists := c.Get("user")
	if !exists {
		h.setFlashKey(c, "flash_error", "flash.login_required")
		c.Redirect(http.StatusFound, "/login")
		return
	}
	user, ok := userValue.(*models.User)
	if !ok || user == nil {
		h.setFlashKey(c, "flash_error", "flash.login_required")
		c.Redirect(http.StatusFound, "/login")
		return
	}

	articleID := parseUint(c.Param("id"))
	content := strings.TrimSpace(c.PostForm("content"))

	if articleID == 0 || content == "" {
		h.setFlashKey(c, "flash_error", "flash.comment_failed")
		if articleID == 0 {
			c.Redirect(http.StatusFound, "/articles")
			return
		}
		c.Redirect(http.StatusFound, fmt.Sprintf("/articles/%d", articleID))
		return
	}

	var article models.Article
	if err := h.app.DB.Select("id", "status").First(&article, articleID).Error; err != nil || article.Status != "published" {
		h.setFlashKey(c, "flash_error", "flash.comment_failed")
		c.Redirect(http.StatusFound, fmt.Sprintf("/articles/%d", articleID))
		return
	}

	if err := h.svc.CreateArticleComment(user.ID, articleID, content); err != nil {
		h.app.Log.WithError(err).Error("create article comment failed")
		h.setFlashKey(c, "flash_error", "flash.comment_failed")
		c.Redirect(http.StatusFound, fmt.Sprintf("/articles/%d", articleID))
		return
	}

	h.setFlashKey(c, "flash_success", "flash.comment_success")
	c.Redirect(http.StatusFound, fmt.Sprintf("/articles/%d", articleID))
}

// ---- 资源发布 ----

// PublishResourcePage 渲染资源发布页
func (h *Handler) PublishResourcePage(c *gin.Context) {
	data := h.basePageData(c)
	data.Title = h.tr(c, "resource.publish_title")
	data.ActiveNav = "resources"

	if categories, err := h.svc.GetCategories(); err == nil {
		data.Categories = categories
	} else {
		h.app.Log.WithError(err).Warn("load cached categories for publish page failed")
	}

	if tags, err := h.svc.GetTags(); err == nil {
		data.Tags = tags
	} else {
		h.app.Log.WithError(err).Warn("load cached tags for publish page failed")
	}

	h.renderPage(c, "resource_publish.html", data)
}

// PublishTemplateCSV 下载 CSV 导入模板
func (h *Handler) PublishTemplateCSV(c *gin.Context) {
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

// PublishManualHandler 处理手动发布资源
func (h *Handler) PublishManualHandler(c *gin.Context) {
	user := c.MustGet("user").(*models.User)

	title := strings.TrimSpace(c.PostForm("title"))
	description := strings.TrimSpace(c.PostForm("description"))
	resourceType := c.PostForm("resource_type")
	link := strings.TrimSpace(c.PostForm("link"))
	categoryID := parseUint(c.PostForm("category_id"))
	tagIDs := parseUintList(c.PostFormArray("tag_ids"))

	if len([]rune(title)) < 2 {
		h.setFlashKey(c, "flash_error", "flash.publish_title_too_short")
		c.Redirect(http.StatusFound, "/publish")
		return
	}
	if len([]rune(description)) < 5 {
		h.setFlashKey(c, "flash_error", "flash.publish_description_too_short")
		c.Redirect(http.StatusFound, "/publish")
		return
	}
	if link != "" && !service.IsValidHTTPURL(link) {
		h.setFlashKey(c, "flash_error", "flash.publish_invalid_link")
		c.Redirect(http.StatusFound, "/publish")
		return
	}

	fileName := ""
	if file, err := c.FormFile("resource_file"); err == nil && file != nil {
		if file.Size > 100*1024*1024 {
			h.setFlashKey(c, "flash_error", "flash.publish_file_too_large")
			c.Redirect(http.StatusFound, "/publish")
			return
		}

		src, err := file.Open()
		if err != nil {
			h.app.Log.WithError(err).Error("open uploaded file failed")
			h.setFlashKey(c, "flash_error", "flash.publish_upload_failed")
			c.Redirect(http.StatusFound, "/publish")
			return
		}
		defer src.Close()

		savedName, err := h.svc.SaveUploadedFile(src, file.Filename, user.ID)
		if err != nil {
			h.app.Log.WithError(err).Error("save uploaded file failed")
			h.setFlashKey(c, "flash_error", "flash.publish_upload_failed")
			c.Redirect(http.StatusFound, "/publish")
			return
		}
		fileName = savedName
	}

	if link == "" && fileName == "" {
		h.setFlashKey(c, "flash_error", "flash.publish_link_or_file_required")
		c.Redirect(http.StatusFound, "/publish")
		return
	}

	resource, err := h.svc.PublishManualResource(user, title, description, resourceType, link, fileName, categoryID, tagIDs)
	if err != nil {
		h.app.Log.WithError(err).Error("publish manual resource failed")
		h.setFlashKey(c, "flash_error", "flash.publish_failed")
		c.Redirect(http.StatusFound, "/publish")
		return
	}

	if resource.Status == "approved" {
		h.setFlashKey(c, "flash_success", "flash.publish_success_approved")
		c.Redirect(http.StatusFound, fmt.Sprintf("/resources/%d", resource.ID))
		return
	}
	h.setFlashKey(c, "flash_success", "flash.publish_success_pending")
	c.Redirect(http.StatusFound, "/resources")
}

// PublishImportHandler 处理 CSV 批量导入资源
func (h *Handler) PublishImportHandler(c *gin.Context) {
	user := c.MustGet("user").(*models.User)

	file, err := c.FormFile("import_file")
	if err != nil {
		h.setFlashKey(c, "flash_error", "flash.publish_import_file_required")
		c.Redirect(http.StatusFound, "/publish")
		return
	}

	src, err := file.Open()
	if err != nil {
		h.app.Log.WithError(err).Error("open import file failed")
		h.setFlashKey(c, "flash_error", "flash.publish_import_failed")
		c.Redirect(http.StatusFound, "/publish")
		return
	}
	defer src.Close()

	defaultType := "other"
	if rawDefaultType := strings.TrimSpace(c.PostForm("default_resource_type")); rawDefaultType != "" {
		defaultType = rawDefaultType
	}
	categoryID := parseUint(c.PostForm("category_id"))
	tagIDs := parseUintList(c.PostFormArray("tag_ids"))

	created, skipped, err := h.svc.PublishImportResources(user, src, defaultType, categoryID, tagIDs)
	if err != nil {
		h.app.Log.WithError(err).Error("publish import failed")
		h.setFlashKey(c, "flash_error", "flash.publish_import_failed")
		c.Redirect(http.StatusFound, "/publish")
		return
	}

	if created == 0 {
		h.setFlashKey(c, "flash_error", "flash.publish_import_empty")
		c.Redirect(http.StatusFound, "/publish")
		return
	}

	h.setFlash(c, "flash_success", h.tr(c, "flash.publish_import_success", created, skipped))
	c.Redirect(http.StatusFound, "/resources")
}

// PublishCrawlerHandler 处理爬虫抓取发布资源
func (h *Handler) PublishCrawlerHandler(c *gin.Context) {
	user := c.MustGet("user").(*models.User)

	crawlURL := strings.TrimSpace(c.PostForm("crawl_url"))
	resourceType := c.PostForm("resource_type")
	categoryID := parseUint(c.PostForm("category_id"))
	tagIDs := parseUintList(c.PostFormArray("tag_ids"))

	if !service.IsValidHTTPURL(crawlURL) {
		h.setFlashKey(c, "flash_error", "flash.publish_invalid_crawl_url")
		c.Redirect(http.StatusFound, "/publish")
		return
	}

	resource, err := h.svc.PublishCrawlerResource(user, crawlURL, resourceType, categoryID, tagIDs)
	if err != nil {
		h.app.Log.WithError(err).Warn("publish crawler resource failed")
		h.setFlashKey(c, "flash_error", "flash.publish_crawl_failed")
		c.Redirect(http.StatusFound, "/publish")
		return
	}

	if resource.Status == "approved" {
		h.setFlashKey(c, "flash_success", "flash.publish_success_approved")
		c.Redirect(http.StatusFound, fmt.Sprintf("/resources/%d", resource.ID))
		return
	}
	h.setFlashKey(c, "flash_success", "flash.publish_success_pending")
	c.Redirect(http.StatusFound, "/resources")
}

// ---- 工具函数 ----

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
