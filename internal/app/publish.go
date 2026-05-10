package app

import (
	"encoding/csv"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"prompt736/internal/models"

	"github.com/PuerkitoBio/goquery"
	"github.com/gin-gonic/gin"
)

// PublishResourcePage 发布资源页处理器
func (app *App) PublishResourcePage(c *gin.Context) {
	data := app.basePageData(c)
	data.Title = app.tr(c, "resource.publish_title")
	data.ActiveNav = "resources"

	if categories, err := app.GetCategories(); err == nil {
		data.Categories = categories
	} else {
		app.Log.WithError(err).Warn("load cached categories for publish page failed")
	}

	if tags, err := app.GetTags(); err == nil {
		data.Tags = tags
	} else {
		app.Log.WithError(err).Warn("load cached tags for publish page failed")
	}

	app.renderPage(c, "resource_publish.html", data)
}

// PublishTemplateCSV 发布资源模板 CSV 下载
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

// PublishManualHandler 手动发布资源处理
func (app *App) PublishManualHandler(c *gin.Context) {
	user := c.MustGet("user").(*models.User)

	title := strings.TrimSpace(c.PostForm("title"))
	description := strings.TrimSpace(c.PostForm("description"))
	resourceType := CanonicalResourceType(c.PostForm("resource_type"))
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

// PublishImportHandler CSV 导入发布资源处理
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
		defaultType = CanonicalResourceType(rawDefaultType)
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
			resourceType = CanonicalResourceType(resourceTypeRaw)
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

// PublishCrawlerHandler 爬虫发布资源处理
func (app *App) PublishCrawlerHandler(c *gin.Context) {
	user := c.MustGet("user").(*models.User)

	crawlURL := strings.TrimSpace(c.PostForm("crawl_url"))
	resourceType := CanonicalResourceType(c.PostForm("resource_type"))
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
