package handler

import (
	"errors"
	"fmt"
	"html/template"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"prompt736/internal/app"
	"prompt736/internal/models"
	"prompt736/internal/service"
	tmplPkg "prompt736/internal/template"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// AdminHandler 封装后台管理所有 HTTP 处理方法
type AdminHandler struct {
	app *app.App
	svc *service.Service
	h   *Handler
}

// NewAdminHandler 创建 AdminHandler 实例
func NewAdminHandler(a *app.App, s *service.Service, h *Handler) *AdminHandler {
	return &AdminHandler{app: a, svc: s, h: h}
}

// renderAdminPage 渲染后台管理页面模板
func (ah *AdminHandler) renderAdminPage(c *gin.Context, templateFile string, data app.PageData) {
	tmpl := template.Must(template.New("").Funcs(tmplPkg.BuildTemplateFuncs(ah.app, data)).ParseFiles(
		"templates/admin/base.html",
		"templates/admin/"+templateFile,
	))

	c.Header("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.ExecuteTemplate(c.Writer, "base.html", data); err != nil {
		ah.app.Log.WithError(err).Error("admin template render failed")
		c.String(http.StatusInternalServerError, ah.h.tr(c, "flash.render_failed"))
	}
}

// ---- 仪表盘 ----

// AdminDashboard 渲染管理后台首页
func (ah *AdminHandler) AdminDashboard(c *gin.Context) {
	data := ah.h.basePageData(c)
	data.Title = ah.h.tr(c, "admin.nav.dashboard")
	data.ActiveNav = "dashboard"

	data.Stats = ah.svc.GetDashboardStats()
	data.PendingResources = ah.svc.GetPendingResources(10)

	backups := ah.svc.ListBackups()
	sort.Slice(backups, func(i, j int) bool {
		return backups[i].Name > backups[j].Name
	})
	data.Backups = backups

	ah.renderAdminPage(c, "dashboard.html", data)
}

// ---- 用户管理 ----

// AdminUsersPage 渲染用户管理页
func (ah *AdminHandler) AdminUsersPage(c *gin.Context) {
	data := ah.h.basePageData(c)
	data.Title = ah.h.tr(c, "admin.nav.users")
	data.ActiveNav = "users"

	var users []models.User
	ah.app.DB.Order("created_at desc").Find(&users)
	data.Users = users

	ah.renderAdminPage(c, "users.html", data)
}

// AdminUpdateUser 处理更新用户角色和状态
func (ah *AdminHandler) AdminUpdateUser(c *gin.Context) {
	userID := parseUint(c.Param("id"))
	role := c.PostForm("role")
	status := c.PostForm("status")

	if userID == 0 {
		ah.h.setFlashKey(c, "flash_error", "flash.update_failed")
		c.Redirect(http.StatusFound, "/admin/users")
		return
	}

	var target models.User
	if err := ah.app.DB.First(&target, userID).Error; err != nil {
		ah.h.setFlashKey(c, "flash_error", "flash.user_not_found")
		c.Redirect(http.StatusFound, "/admin/users")
		return
	}

	if status != "" {
		if strings.EqualFold(target.Username, "admin") && status == "suspended" {
			ah.h.setFlashKey(c, "flash_error", "flash.admin_cannot_disable")
			c.Redirect(http.StatusFound, "/admin/users")
			return
		}
	}

	if err := ah.svc.UpdateUser(userID, role, status); err != nil {
		ah.h.setFlashKey(c, "flash_error", "flash.update_failed")
		c.Redirect(http.StatusFound, "/admin/users")
		return
	}

	ah.h.setFlashKey(c, "flash_success", "flash.user_updated")
	c.Redirect(http.StatusFound, "/admin/users")
}

// ---- 资源管理 ----

// AdminResourcesPage 渲染资源管理页
func (ah *AdminHandler) AdminResourcesPage(c *gin.Context) {
	data := ah.h.basePageData(c)
	data.Title = ah.h.tr(c, "admin.nav.resources")
	data.ActiveNav = "resources"

	status := c.Query("status")
	data.Status = status

	query := ah.app.DB.Preload("Category").Preload("Tags").Preload("User")
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

	ah.renderAdminPage(c, "resources.html", data)
}

// AdminReviewResource 处理资源审核
func (ah *AdminHandler) AdminReviewResource(c *gin.Context) {
	resourceID := parseUint(c.Param("id"))
	status := c.PostForm("status")

	ah.svc.ReviewResource(resourceID, status)
	ah.h.setFlashKey(c, "flash_success", "flash.review_done")
	c.Redirect(http.StatusFound, "/admin/resources")
}

// ---- 文章管理 ----

// AdminArticlesPage 渲染文章管理页
func (ah *AdminHandler) AdminArticlesPage(c *gin.Context) {
	data := ah.h.basePageData(c)
	data.Title = ah.h.tr(c, "admin.nav.articles")
	data.ActiveNav = "articles"

	var articles []models.Article
	ah.app.DB.Preload("User").Order("created_at desc").Find(&articles)
	data.Articles = articles

	ah.renderAdminPage(c, "articles.html", data)
}

// AdminArticleFormPage 渲染文章编辑/新建表单页
func (ah *AdminHandler) AdminArticleFormPage(c *gin.Context) {
	data := ah.h.basePageData(c)
	data.ActiveNav = "articles"

	idStr := c.Param("id")
	if idStr != "" {
		id := parseUint(idStr)
		var article models.Article
		if err := ah.app.DB.First(&article, id).Error; err == nil {
			data.Article = &article
			data.Title = ah.h.tr(c, "admin.article_form.edit_title")
		}
	} else {
		data.Title = ah.h.tr(c, "admin.article_form.create_title")
	}

	ah.renderAdminPage(c, "article_form.html", data)
}

// AdminCreateArticle 处理创建文章
func (ah *AdminHandler) AdminCreateArticle(c *gin.Context) {
	user := c.MustGet("user").(*models.User)
	ah.svc.CreateArticle(user.ID, c.PostForm("title"), c.PostForm("summary"), c.PostForm("content"))
	ah.h.setFlashKey(c, "flash_success", "flash.article_created")
	c.Redirect(http.StatusFound, "/admin/articles")
}

// AdminUpdateArticle 处理更新文章
func (ah *AdminHandler) AdminUpdateArticle(c *gin.Context) {
	articleID := parseUint(c.Param("id"))
	ah.svc.UpdateArticle(articleID, c.PostForm("title"), c.PostForm("summary"), c.PostForm("content"))
	ah.h.setFlashKey(c, "flash_success", "flash.article_updated")
	c.Redirect(http.StatusFound, "/admin/articles")
}

// ---- 分类管理 ----

// AdminCategoriesPage 渲染分类管理页
func (ah *AdminHandler) AdminCategoriesPage(c *gin.Context) {
	data := ah.h.basePageData(c)
	data.Title = ah.h.tr(c, "admin.nav.categories")
	data.ActiveNav = "categories"

	var categories []models.Category
	ah.app.DB.Order("created_at desc").Find(&categories)
	data.Categories = categories

	ah.renderAdminPage(c, "categories.html", data)
}

// AdminCreateCategory 处理创建分类
func (ah *AdminHandler) AdminCreateCategory(c *gin.Context) {
	ah.svc.CreateCategory(c.PostForm("name"))
	ah.h.setFlashKey(c, "flash_success", "flash.category_created")
	c.Redirect(http.StatusFound, "/admin/categories")
}

// AdminUpdateCategory 处理更新分类
func (ah *AdminHandler) AdminUpdateCategory(c *gin.Context) {
	categoryID := parseUint(c.Param("id"))
	ah.svc.UpdateCategory(categoryID, c.PostForm("name"))
	ah.h.setFlashKey(c, "flash_success", "flash.category_updated")
	c.Redirect(http.StatusFound, "/admin/categories")
}

// AdminDeleteCategory 处理删除分类
func (ah *AdminHandler) AdminDeleteCategory(c *gin.Context) {
	categoryID := parseUint(c.Param("id"))
	if categoryID == 0 {
		ah.h.setFlashKey(c, "flash_error", "flash.category_delete_failed")
		c.Redirect(http.StatusFound, "/admin/categories")
		return
	}

	if err := ah.svc.DeleteCategory(categoryID); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			ah.h.setFlashKey(c, "flash_error", "flash.category_not_found")
		} else {
			ah.app.Log.WithError(err).WithField("category_id", categoryID).Error("delete category failed")
			ah.h.setFlashKey(c, "flash_error", "flash.category_delete_failed")
		}
		c.Redirect(http.StatusFound, "/admin/categories")
		return
	}

	ah.h.setFlashKey(c, "flash_success", "flash.category_deleted")
	c.Redirect(http.StatusFound, "/admin/categories")
}

// ---- 标签管理 ----

// AdminTagsPage 渲染标签管理页
func (ah *AdminHandler) AdminTagsPage(c *gin.Context) {
	data := ah.h.basePageData(c)
	data.Title = ah.h.tr(c, "admin.nav.tags")
	data.ActiveNav = "tags"

	var tags []models.Tag
	ah.app.DB.Order("created_at desc").Find(&tags)
	data.Tags = tags

	ah.renderAdminPage(c, "tags.html", data)
}

// AdminCreateTag 处理创建标签
func (ah *AdminHandler) AdminCreateTag(c *gin.Context) {
	ah.svc.CreateTag(c.PostForm("name"))
	ah.h.setFlashKey(c, "flash_success", "flash.tag_created")
	c.Redirect(http.StatusFound, "/admin/tags")
}

// AdminUpdateTag 处理更新标签
func (ah *AdminHandler) AdminUpdateTag(c *gin.Context) {
	tagID := parseUint(c.Param("id"))
	ah.svc.UpdateTag(tagID, c.PostForm("name"))
	ah.h.setFlashKey(c, "flash_success", "flash.tag_updated")
	c.Redirect(http.StatusFound, "/admin/tags")
}

// AdminDeleteTag 处理删除标签
func (ah *AdminHandler) AdminDeleteTag(c *gin.Context) {
	tagID := parseUint(c.Param("id"))
	ah.svc.DeleteTag(tagID)
	ah.h.setFlashKey(c, "flash_success", "flash.tag_deleted")
	c.Redirect(http.StatusFound, "/admin/tags")
}

// ---- 站点设置 ----

// AdminSettingsPage 渲染站点设置页
func (ah *AdminHandler) AdminSettingsPage(c *gin.Context) {
	data := ah.h.basePageData(c)
	data.Title = ah.h.tr(c, "admin.nav.settings")
	data.ActiveNav = "settings"

	if cfg, err := ah.svc.GetSiteConfig(); err == nil {
		data.Config = &cfg
	} else {
		ah.app.Log.WithError(err).Warn("load cached site config for admin settings failed")
	}

	ah.renderAdminPage(c, "settings.html", data)
}

// AdminUpdateSettings 处理更新站点设置
func (ah *AdminHandler) AdminUpdateSettings(c *gin.Context) {
	var cfg models.SiteConfig
	ah.app.DB.First(&cfg)

	ah.svc.UpdateSettings(&cfg,
		c.PostForm("site_name"),
		c.PostForm("site_desc"),
		c.PostForm("hero_title"),
		c.PostForm("hero_subtitle"),
		c.PostForm("locale_default"),
		c.PostForm("ads_json"),
	)

	ah.h.setFlashKey(c, "flash_success", "flash.settings_updated")
	c.Redirect(http.StatusFound, "/admin/settings")
}

// ---- 备份与恢复 ----

// AdminBackup 处理创建备份
func (ah *AdminHandler) AdminBackup(c *gin.Context) {
	if err := os.MkdirAll(ah.app.Cfg.BackupDir, 0755); err != nil {
		ah.app.Log.WithError(err).Error("create backup dir failed")
		ah.h.setFlashKey(c, "flash_error", "flash.backup_failed")
		c.Redirect(http.StatusFound, "/admin")
		return
	}

	snapshot, err := ah.svc.CollectBackupSnapshot()
	if err != nil {
		ah.app.Log.WithError(err).Error("collect backup snapshot failed")
		ah.h.setFlashKey(c, "flash_error", "flash.backup_failed")
		c.Redirect(http.StatusFound, "/admin")
		return
	}

	filename := fmt.Sprintf("backup_%s.json", time.Now().Format("20060102_150405"))
	backupPath := filepath.Join(ah.app.Cfg.BackupDir, filename)
	if err := service.WriteSnapshotAtomic(backupPath, snapshot); err != nil {
		ah.app.Log.WithError(err).Error("write backup file failed")
		ah.h.setFlashKey(c, "flash_error", "flash.backup_failed")
		c.Redirect(http.StatusFound, "/admin")
		return
	}

	ah.h.setFlashKey(c, "flash_success", "flash.backup_success")
	c.Redirect(http.StatusFound, "/admin")
}

// AdminRestore 处理恢复备份
func (ah *AdminHandler) AdminRestore(c *gin.Context) {
	filename := c.PostForm("filename")
	if filename == "" {
		ah.h.setFlashKey(c, "flash_error", "flash.select_backup")
		c.Redirect(http.StatusFound, "/admin")
		return
	}

	backupPath, err := ah.svc.ResolveBackupPath(filename)
	if err != nil {
		ah.app.Log.WithError(err).WithField("filename", filename).Warn("invalid backup file path")
		ah.h.setFlashKey(c, "flash_error", "flash.invalid_backup_file")
		c.Redirect(http.StatusFound, "/admin")
		return
	}

	switch strings.ToLower(filepath.Ext(backupPath)) {
	case ".json":
		err = ah.svc.RestoreFromJSONBackup(backupPath)
	case ".sql":
		err = ah.svc.RestoreFromSQLBackup(backupPath)
	default:
		err = fmt.Errorf("unsupported backup extension: %s", filepath.Ext(backupPath))
	}
	if err != nil {
		ah.app.Log.WithError(err).WithField("backup", backupPath).Error("restore backup failed")
		ah.h.setFlashKey(c, "flash_error", "flash.restore_failed")
		c.Redirect(http.StatusFound, "/admin")
		return
	}

	ah.app.Cache.Flush()
	ah.h.setFlashKey(c, "flash_success", "flash.restore_success")
	c.Redirect(http.StatusFound, "/admin")
}
