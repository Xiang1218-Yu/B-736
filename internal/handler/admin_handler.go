package handler

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"prompt736/internal/models"
	"prompt736/internal/utils"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// AdminDashboard 管理后台仪表盘
func (h *Handler) AdminDashboard(c *gin.Context) {
	data := h.basePageData(c)
	data.Title = h.tr(c, "admin.nav.dashboard")
	data.ActiveNav = "dashboard"

	var resourceCount, pendingCount, articleCount, userCount int64
	h.App.DB.Model(&models.Resource{}).Count(&resourceCount)
	h.App.DB.Model(&models.Resource{}).Where("status = ?", "pending").Count(&pendingCount)
	h.App.DB.Model(&models.Article{}).Count(&articleCount)
	h.App.DB.Model(&models.User{}).Count(&userCount)

	data.Stats = map[string]int64{
		"Resources": resourceCount,
		"Pending":   pendingCount,
		"Articles":  articleCount,
		"Users":     userCount,
	}

	var pendingResources []models.Resource
	h.App.DB.Preload("User").Where("status = ?", "pending").Order("created_at desc").Limit(10).Find(&pendingResources)
	data.PendingResources = pendingResources

	backups, _ := h.App.GetBackupList()
	data.Backups = backups

	h.renderAdminPage(c, "dashboard.html", data)
}

// AdminUsersPage 用户管理页面
func (h *Handler) AdminUsersPage(c *gin.Context) {
	data := h.basePageData(c)
	data.Title = h.tr(c, "admin.nav.users")
	data.ActiveNav = "users"

	users, _ := h.App.GetAllUsers()
	data.Users = users

	h.renderAdminPage(c, "users.html", data)
}

// AdminUpdateUser 更新用户
func (h *Handler) AdminUpdateUser(c *gin.Context) {
	userID := utils.ParseUint(c.Param("id"))
	role := c.PostForm("role")
	status := c.PostForm("status")

	if userID == 0 {
		h.setFlashKey(c, "flash_error", "flash.update_failed")
		c.Redirect(http.StatusFound, "/admin/users")
		return
	}

	err := h.App.UpdateUser(userID, role, status)
	if err != nil {
		if strings.Contains(err.Error(), "cannot disable admin") {
			h.setFlashKey(c, "flash_error", "flash.admin_cannot_disable")
		} else {
			h.setFlashKey(c, "flash_error", "flash.update_failed")
		}
		c.Redirect(http.StatusFound, "/admin/users")
		return
	}

	h.setFlashKey(c, "flash_success", "flash.user_updated")
	c.Redirect(http.StatusFound, "/admin/users")
}

// AdminResourcesPage 资源管理页面
func (h *Handler) AdminResourcesPage(c *gin.Context) {
	data := h.basePageData(c)
	data.Title = h.tr(c, "admin.nav.resources")
	data.ActiveNav = "resources"

	status := c.Query("status")
	data.Status = status

	resources, _ := h.App.GetResourcesByStatus(status)
	data.Resources = resources

	h.renderAdminPage(c, "resources.html", data)
}

// AdminReviewResource 审核资源
func (h *Handler) AdminReviewResource(c *gin.Context) {
	resourceID := utils.ParseUint(c.Param("id"))
	status := c.PostForm("status")

	h.App.ReviewResource(resourceID, status)

	h.setFlashKey(c, "flash_success", "flash.review_done")
	c.Redirect(http.StatusFound, "/admin/resources")
}

// AdminArticlesPage 文章管理页面
func (h *Handler) AdminArticlesPage(c *gin.Context) {
	data := h.basePageData(c)
	data.Title = h.tr(c, "admin.nav.articles")
	data.ActiveNav = "articles"

	articles, _ := h.App.GetAllArticles()
	data.Articles = articles

	h.renderAdminPage(c, "articles.html", data)
}

// AdminArticleFormPage 文章表单页面
func (h *Handler) AdminArticleFormPage(c *gin.Context) {
	data := h.basePageData(c)
	data.ActiveNav = "articles"

	idStr := c.Param("id")
	if idStr != "" {
		id := utils.ParseUint(idStr)
		article, err := h.App.GetArticleByID(id)
		if err == nil {
			data.Article = article
			data.Title = h.tr(c, "admin.article_form.edit_title")
		}
	} else {
		data.Title = h.tr(c, "admin.article_form.create_title")
	}

	h.renderAdminPage(c, "article_form.html", data)
}

// AdminCreateArticle 创建文章
func (h *Handler) AdminCreateArticle(c *gin.Context) {
	user := c.MustGet("user").(*models.User)

	title := c.PostForm("title")
	summary := c.PostForm("summary")
	content := c.PostForm("content")

	_, err := h.App.CreateArticle(user.ID, title, summary, content)
	if err != nil {
		h.setFlashKey(c, "flash_error", "flash.article_create_failed")
		c.Redirect(http.StatusFound, "/admin/articles")
		return
	}

	h.setFlashKey(c, "flash_success", "flash.article_created")
	c.Redirect(http.StatusFound, "/admin/articles")
}

// AdminUpdateArticle 更新文章
func (h *Handler) AdminUpdateArticle(c *gin.Context) {
	articleID := utils.ParseUint(c.Param("id"))

	title := c.PostForm("title")
	summary := c.PostForm("summary")
	content := c.PostForm("content")

	err := h.App.UpdateArticle(articleID, title, summary, content)
	if err != nil {
		h.setFlashKey(c, "flash_error", "flash.article_update_failed")
		c.Redirect(http.StatusFound, "/admin/articles")
		return
	}

	h.setFlashKey(c, "flash_success", "flash.article_updated")
	c.Redirect(http.StatusFound, "/admin/articles")
}

// AdminCategoriesPage 分类管理页面
func (h *Handler) AdminCategoriesPage(c *gin.Context) {
	data := h.basePageData(c)
	data.Title = h.tr(c, "admin.nav.categories")
	data.ActiveNav = "categories"

	categories, _ := h.App.GetCategories()
	data.Categories = categories

	h.renderAdminPage(c, "categories.html", data)
}

// AdminCreateCategory 创建分类
func (h *Handler) AdminCreateCategory(c *gin.Context) {
	name := c.PostForm("name")
	if name != "" {
		h.App.CreateCategory(name)
	}

	h.setFlashKey(c, "flash_success", "flash.category_created")
	c.Redirect(http.StatusFound, "/admin/categories")
}

// AdminUpdateCategory 更新分类
func (h *Handler) AdminUpdateCategory(c *gin.Context) {
	categoryID := utils.ParseUint(c.Param("id"))
	name := c.PostForm("name")

	h.App.UpdateCategory(categoryID, name)

	h.setFlashKey(c, "flash_success", "flash.category_updated")
	c.Redirect(http.StatusFound, "/admin/categories")
}

// AdminDeleteCategory 删除分类
func (h *Handler) AdminDeleteCategory(c *gin.Context) {
	categoryID := utils.ParseUint(c.Param("id"))
	if categoryID == 0 {
		h.setFlashKey(c, "flash_error", "flash.category_delete_failed")
		c.Redirect(http.StatusFound, "/admin/categories")
		return
	}

	err := h.App.DeleteCategory(categoryID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			h.setFlashKey(c, "flash_error", "flash.category_not_found")
		} else {
			h.App.Log.WithError(err).WithField("category_id", categoryID).Error("delete category failed")
			h.setFlashKey(c, "flash_error", "flash.category_delete_failed")
		}
		c.Redirect(http.StatusFound, "/admin/categories")
		return
	}

	h.App.ClearCache()
	h.setFlashKey(c, "flash_success", "flash.category_deleted")
	c.Redirect(http.StatusFound, "/admin/categories")
}

// AdminTagsPage 标签管理页面
func (h *Handler) AdminTagsPage(c *gin.Context) {
	data := h.basePageData(c)
	data.Title = h.tr(c, "admin.nav.tags")
	data.ActiveNav = "tags"

	tags, _ := h.App.GetTags()
	data.Tags = tags

	h.renderAdminPage(c, "tags.html", data)
}

// AdminCreateTag 创建标签
func (h *Handler) AdminCreateTag(c *gin.Context) {
	name := c.PostForm("name")
	if name != "" {
		h.App.CreateTag(name)
	}

	h.setFlashKey(c, "flash_success", "flash.tag_created")
	c.Redirect(http.StatusFound, "/admin/tags")
}

// AdminUpdateTag 更新标签
func (h *Handler) AdminUpdateTag(c *gin.Context) {
	tagID := utils.ParseUint(c.Param("id"))
	name := c.PostForm("name")

	h.App.UpdateTag(tagID, name)

	h.setFlashKey(c, "flash_success", "flash.tag_updated")
	c.Redirect(http.StatusFound, "/admin/tags")
}

// AdminDeleteTag 删除标签
func (h *Handler) AdminDeleteTag(c *gin.Context) {
	tagID := utils.ParseUint(c.Param("id"))
	h.App.DeleteTag(tagID)

	h.setFlashKey(c, "flash_success", "flash.tag_deleted")
	c.Redirect(http.StatusFound, "/admin/tags")
}

// AdminSettingsPage 设置页面
func (h *Handler) AdminSettingsPage(c *gin.Context) {
	data := h.basePageData(c)
	data.Title = h.tr(c, "admin.nav.settings")
	data.ActiveNav = "settings"

	if cfg, err := h.App.GetSiteConfig(); err == nil {
		data.Config = &cfg
	} else {
		h.App.Log.WithError(err).Warn("load cached site config for admin settings failed")
	}

	h.renderAdminPage(c, "settings.html", data)
}

// AdminUpdateSettings 更新设置
func (h *Handler) AdminUpdateSettings(c *gin.Context) {
	siteName := c.PostForm("site_name")
	siteDesc := c.PostForm("site_desc")
	heroTitle := c.PostForm("hero_title")
	heroSubtitle := c.PostForm("hero_subtitle")
	localeDefault := c.PostForm("locale_default")
	adsJSON := c.PostForm("ads_json")

	h.App.UpdateSiteConfig(siteName, siteDesc, heroTitle, heroSubtitle, localeDefault, adsJSON)

	h.setFlashKey(c, "flash_success", "flash.settings_updated")
	c.Redirect(http.StatusFound, "/admin/settings")
}

// AdminBackup 创建备份
func (h *Handler) AdminBackup(c *gin.Context) {
	_, err := h.App.CreateBackup()
	if err != nil {
		h.App.Log.WithError(err).Error("create backup failed")
		h.setFlashKey(c, "flash_error", "flash.backup_failed")
		c.Redirect(http.StatusFound, "/admin")
		return
	}

	h.setFlashKey(c, "flash_success", "flash.backup_success")
	c.Redirect(http.StatusFound, "/admin")
}

// AdminRestore 恢复备份
func (h *Handler) AdminRestore(c *gin.Context) {
	filename := c.PostForm("filename")
	if filename == "" {
		h.setFlashKey(c, "flash_error", "flash.select_backup")
		c.Redirect(http.StatusFound, "/admin")
		return
	}

	err := h.App.RestoreBackup(filename)
	if err != nil {
		h.App.Log.WithError(err).WithField("filename", filename).Error("restore backup failed")
		h.setFlashKey(c, "flash_error", "flash.restore_failed")
		c.Redirect(http.StatusFound, "/admin")
		return
	}

	h.setFlashKey(c, "flash_success", "flash.restore_success")
	c.Redirect(http.StatusFound, "/admin")
}
