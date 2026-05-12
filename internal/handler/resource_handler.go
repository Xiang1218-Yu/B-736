package handler

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"prompt736/internal/models"
	"prompt736/internal/types"
	"prompt736/internal/utils"

	"github.com/gin-gonic/gin"
)

// ResourcesPage 资源列表页
func (h *Handler) ResourcesPage(c *gin.Context) {
	data := h.basePageData(c)
	data.Title = h.tr(c, "resource.library_title")
	data.ActiveNav = "resources"

	q := strings.TrimSpace(c.Query("q"))
	categoryID := utils.ParseUint(c.Query("category_id"))
	tagID := utils.ParseUint(c.Query("tag_id"))
	resourceType := c.Query("type")
	page := utils.ParseInt(c.DefaultQuery("page", "1"), 1)
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

	if payload, err := h.App.GetResourceListPayload(q, categoryID, tagID, resourceType, page, pageSize); err == nil {
		data.Categories = payload.Categories
		data.Tags = payload.Tags
		data.Total = payload.Total
		data.TotalPages = int((payload.Total + int64(pageSize) - 1) / int64(pageSize))
		data.Resources = payload.Resources
	} else {
		h.App.Log.WithError(err).Warn("load resource list cache payload failed")
	}

	h.renderPage(c, "resources.html", data)
}

// ResourceDetailPage 资源详情页
func (h *Handler) ResourceDetailPage(c *gin.Context) {
	id := utils.ParseUint(c.Param("id"))
	if id == 0 {
		c.Redirect(http.StatusFound, "/resources")
		return
	}

	resource, err := h.App.GetResourceByID(id)
	if err != nil || resource.Status != "approved" {
		c.Redirect(http.StatusFound, "/resources")
		return
	}

	h.App.IncrementResourceViews(id)

	if resource.RatingCount > 0 {
		resource.RatingScore = resource.RatingScore / float64(resource.RatingCount)
	}

	comments, _ := h.App.GetResourceComments(id)

	data := h.basePageData(c)
	data.Title = resource.Title
	data.ActiveNav = "resources"
	data.Resource = resource
	data.Comments = comments

	h.renderPage(c, "resource_detail.html", data)
}

// PublishResourcePage 发布资源页面
func (h *Handler) PublishResourcePage(c *gin.Context) {
	data := h.basePageData(c)
	data.Title = h.tr(c, "resource.publish_title")
	data.ActiveNav = "resources"

	if categories, err := h.App.GetCategories(); err == nil {
		data.Categories = categories
	} else {
		h.App.Log.WithError(err).Warn("load cached categories for publish page failed")
	}

	if tags, err := h.App.GetTags(); err == nil {
		data.Tags = tags
	} else {
		h.App.Log.WithError(err).Warn("load cached tags for publish page failed")
	}

	h.renderPage(c, "resource_publish.html", data)
}

// PublishTemplateCSV 下载CSV模板
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

// PublishManualHandler 手动发布资源处理
func (h *Handler) PublishManualHandler(c *gin.Context) {
	user := c.MustGet("user").(*models.User)

	title := strings.TrimSpace(c.PostForm("title"))
	description := strings.TrimSpace(c.PostForm("description"))
	resourceType := utils.CanonicalResourceType(c.PostForm("resource_type"))
	link := strings.TrimSpace(c.PostForm("link"))
	categoryID := utils.ParseUint(c.PostForm("category_id"))
	tagIDs := utils.ParseUintList(c.PostFormArray("tag_ids"))

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
	if link != "" && !utils.IsValidHTTPURL(link) {
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

		ext := strings.ToLower(filepath.Ext(file.Filename))
		if ext == "" {
			ext = ".bin"
		}
		fileName = fmt.Sprintf("%d_%d%s", user.ID, time.Now().UnixNano(), ext)
		if err := os.MkdirAll(h.App.Cfg.UploadDir, 0755); err != nil {
			h.App.Log.WithError(err).Error("create upload dir failed")
			h.setFlashKey(c, "flash_error", "flash.publish_upload_failed")
			c.Redirect(http.StatusFound, "/publish")
			return
		}
		target := filepath.Join(h.App.Cfg.UploadDir, fileName)
		if err := c.SaveUploadedFile(file, target); err != nil {
			h.App.Log.WithError(err).Error("save uploaded file failed")
			h.setFlashKey(c, "flash_error", "flash.publish_upload_failed")
			c.Redirect(http.StatusFound, "/publish")
			return
		}
	}

	if link == "" && fileName == "" {
		h.setFlashKey(c, "flash_error", "flash.publish_link_or_file_required")
		c.Redirect(http.StatusFound, "/publish")
		return
	}

	resource, err := h.App.PublishManualResource(user.ID, title, description, resourceType, link, categoryID, tagIDs, fileName)
	if err != nil {
		h.App.Log.WithError(err).Error("publish manual resource failed")
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

// PublishImportHandler CSV导入资源处理
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
		h.App.Log.WithError(err).Error("open import file failed")
		h.setFlashKey(c, "flash_error", "flash.publish_import_failed")
		c.Redirect(http.StatusFound, "/publish")
		return
	}
	defer src.Close()

	defaultType := "other"
	if rawDefaultType := strings.TrimSpace(c.PostForm("default_resource_type")); rawDefaultType != "" {
		defaultType = utils.CanonicalResourceType(rawDefaultType)
	}
	categoryID := utils.ParseUint(c.PostForm("category_id"))
	tagIDs := utils.ParseUintList(c.PostFormArray("tag_ids"))

	created, skipped, err := h.App.ImportResourcesFromCSV(user.ID, src, defaultType, categoryID, tagIDs)
	if err != nil {
		h.App.Log.WithError(err).Error("import resources failed")
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

// PublishCrawlerHandler 爬虫发布资源处理
func (h *Handler) PublishCrawlerHandler(c *gin.Context) {
	user := c.MustGet("user").(*models.User)

	crawlURL := strings.TrimSpace(c.PostForm("crawl_url"))
	resourceType := utils.CanonicalResourceType(c.PostForm("resource_type"))
	categoryID := utils.ParseUint(c.PostForm("category_id"))
	tagIDs := utils.ParseUintList(c.PostFormArray("tag_ids"))

	if !utils.IsValidHTTPURL(crawlURL) {
		h.setFlashKey(c, "flash_error", "flash.publish_invalid_crawl_url")
		c.Redirect(http.StatusFound, "/publish")
		return
	}

	resource, err := h.App.CrawlResource(user.ID, crawlURL, resourceType, categoryID, tagIDs)
	if err != nil {
		h.App.Log.WithError(err).Error("publish crawler resource failed")
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

// ShareHandler 分享处理
func (h *Handler) ShareHandler(c *gin.Context) {
	user := c.MustGet("user").(*models.User)
	resourceID := utils.ParseUint(c.Param("id"))

	if resourceID == 0 {
		c.Redirect(http.StatusFound, "/resources")
		return
	}

	err := h.App.ShareReward(user.ID, resourceID)
	if err != nil {
		if errors.Is(err, utils.ErrShareRewardLimited) {
			h.setFlashKey(c, "flash_error", "flash.share_reward_limited")
		} else {
			h.App.Log.WithError(err).WithField("resource_id", resourceID).Error("share reward failed")
			h.setFlashKey(c, "flash_error", "flash.share_failed")
		}
		c.Redirect(http.StatusFound, fmt.Sprintf("/resources/%d", resourceID))
		return
	}

	h.setFlashKey(c, "flash_success", "flash.share_success")
	c.Redirect(http.StatusFound, fmt.Sprintf("/resources/%d", resourceID))
}

// ResourceCommentHandler 资源评论处理
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

	resourceID := utils.ParseUint(c.Param("id"))
	content := strings.TrimSpace(c.PostForm("content"))
	rating := utils.ParseInt(c.PostForm("rating"), 0)

	if resourceID == 0 || content == "" || rating < 1 || rating > 5 {
		h.setFlashKey(c, "flash_error", "flash.comment_failed")
		if resourceID == 0 {
			c.Redirect(http.StatusFound, "/resources")
			return
		}
		c.Redirect(http.StatusFound, fmt.Sprintf("/resources/%d", resourceID))
		return
	}

	err := h.App.AddResourceComment(user.ID, resourceID, content, rating)
	if err != nil {
		h.App.Log.WithError(err).Error("create resource comment failed")
		h.setFlashKey(c, "flash_error", "flash.comment_failed")
		c.Redirect(http.StatusFound, fmt.Sprintf("/resources/%d", resourceID))
		return
	}

	h.setFlashKey(c, "flash_success", "flash.comment_success")
	h.App.ClearCache()
	c.Redirect(http.StatusFound, fmt.Sprintf("/resources/%d", resourceID))
}
