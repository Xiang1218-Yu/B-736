package app

import (
	"errors"
	"net/http"
	"os"
	"sort"
	"strings"

	"prompt736/internal/models"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// AdminDashboard 管理后台首页处理器
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

	// 待审核资源
	var pendingResources []models.Resource
	app.DB.Preload("User").Where("status = ?", "pending").Order("created_at desc").Limit(10).Find(&pendingResources)
	data.PendingResources = pendingResources

	// 备份文件
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

// AdminUsersPage 用户管理页面
func (app *App) AdminUsersPage(c *gin.Context) {
	data := app.basePageData(c)
	data.Title = app.tr(c, "admin.nav.users")
	data.ActiveNav = "users"

	var users []models.User
	app.DB.Order("created_at desc").Find(&users)
	data.Users = users

	app.renderAdminPage(c, "users.html", data)
}

// AdminUpdateUser 更新用户信息
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

// AdminResourcesPage 资源管理页面
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

// AdminReviewResource 审核资源
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

// AdminArticlesPage 文章管理页面
func (app *App) AdminArticlesPage(c *gin.Context) {
	data := app.basePageData(c)
	data.Title = app.tr(c, "admin.nav.articles")
	data.ActiveNav = "articles"

	var articles []models.Article
	app.DB.Preload("User").Order("created_at desc").Find(&articles)
	data.Articles = articles

	app.renderAdminPage(c, "articles.html", data)
}

// AdminArticleFormPage 文章表单页面（新建/编辑）
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

// AdminCreateArticle 创建文章
func (app *App) AdminCreateArticle(c *gin.Context) {
	user := c.MustGet("user").(*models.User)

	article := models.Article{
		Title:   c.PostForm("title"),
		Summary: c.PostForm("summary"),
		Content: SanitizeArticleContent(c.PostForm("content")),
		Status:  "published",
		UserID:  user.ID,
	}
	app.DB.Create(&article)

	app.Cache.Flush()
	app.setFlashKey(c, "flash_success", "flash.article_created")
	c.Redirect(http.StatusFound, "/admin/articles")
}

// AdminUpdateArticle 更新文章
func (app *App) AdminUpdateArticle(c *gin.Context) {
	articleID := parseUint(c.Param("id"))

	app.DB.Model(&models.Article{}).Where("id = ?", articleID).Updates(map[string]interface{}{
		"title":   c.PostForm("title"),
		"summary": c.PostForm("summary"),
		"content": SanitizeArticleContent(c.PostForm("content")),
	})

	app.Cache.Flush()
	app.setFlashKey(c, "flash_success", "flash.article_updated")
	c.Redirect(http.StatusFound, "/admin/articles")
}

// AdminCategoriesPage 分类管理页面
func (app *App) AdminCategoriesPage(c *gin.Context) {
	data := app.basePageData(c)
	data.Title = app.tr(c, "admin.nav.categories")
	data.ActiveNav = "categories"

	var categories []models.Category
	app.DB.Order("created_at desc").Find(&categories)
	data.Categories = categories

	app.renderAdminPage(c, "categories.html", data)
}

// AdminCreateCategory 创建分类
func (app *App) AdminCreateCategory(c *gin.Context) {
	name := c.PostForm("name")
	if name != "" {
		app.DB.Create(&models.Category{Name: name})
		app.Cache.Flush()
	}

	app.setFlashKey(c, "flash_success", "flash.category_created")
	c.Redirect(http.StatusFound, "/admin/categories")
}

// AdminUpdateCategory 更新分类
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

// AdminDeleteCategory 删除分类
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

// AdminTagsPage 标签管理页面
func (app *App) AdminTagsPage(c *gin.Context) {
	data := app.basePageData(c)
	data.Title = app.tr(c, "admin.nav.tags")
	data.ActiveNav = "tags"

	var tags []models.Tag
	app.DB.Order("created_at desc").Find(&tags)
	data.Tags = tags

	app.renderAdminPage(c, "tags.html", data)
}

// AdminCreateTag 创建标签
func (app *App) AdminCreateTag(c *gin.Context) {
	name := c.PostForm("name")
	if name != "" {
		app.DB.Create(&models.Tag{Name: name})
		app.Cache.Flush()
	}

	app.setFlashKey(c, "flash_success", "flash.tag_created")
	c.Redirect(http.StatusFound, "/admin/tags")
}

// AdminUpdateTag 更新标签
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

// AdminDeleteTag 删除标签
func (app *App) AdminDeleteTag(c *gin.Context) {
	tagID := parseUint(c.Param("id"))
	app.DB.Delete(&models.Tag{}, tagID)
	app.Cache.Flush()

	app.setFlashKey(c, "flash_success", "flash.tag_deleted")
	c.Redirect(http.StatusFound, "/admin/tags")
}

// AdminSettingsPage 设置页面
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

// AdminUpdateSettings 更新设置
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
