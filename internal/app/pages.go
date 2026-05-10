package app

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"prompt736/internal/models"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// HomePage 首页处理器
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

// ResourcesPage 资源列表页处理器
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

// ResourceDetailPage 资源详情页处理器
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

	app.DB.Model(&resource).Update("views", gorm.Expr("views + ?", 1))

	if resource.RatingCount > 0 {
		resource.RatingScore = resource.RatingScore / float64(resource.RatingCount)
	}

	var comments []models.Comment
	app.DB.Preload("User").Where("resource_id = ?", id).Order("created_at desc").Find(&comments)

	data := app.basePageData(c)
	data.Title = resource.Title
	data.ActiveNav = "resources"
	data.Resource = &resource
	data.Comments = comments

	app.renderPage(c, "resource_detail.html", data)
}

// ArticlesPage 文章列表页处理器
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

// ArticleDetailPage 文章详情页处理器
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

// ProfilePage 个人中心页处理器
func (app *App) ProfilePage(c *gin.Context) {
	user := c.MustGet("user").(*models.User)

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
