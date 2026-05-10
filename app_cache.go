package main

import (
	"fmt"
	"net/url"
	"strings"
	"time"

	"prompt736/internal/models"
)

const publicCacheTTL = 2 * time.Minute

const (
	siteConfigCacheKey = "site-config"
	categoriesCacheKey = "categories"
	tagsCacheKey       = "tags"
	homePageCacheKey   = "home-page"
)

type homePageCachePayload struct {
	Stats          map[string]int64
	HotResources   []models.Resource
	LatestArticles []models.Article
	Categories     []models.Category
}

type resourceListCachePayload struct {
	Categories []models.Category
	Tags       []models.Tag
	Resources  []models.Resource
	Total      int64
}

type articleListCachePayload struct {
	Articles []models.Article
	Total    int64
}

func getCachedValue[T any](app *App, key string) (T, bool) {
	var zero T
	value, found := app.Cache.Get(key)
	if !found {
		return zero, false
	}

	typed, ok := value.(T)
	if !ok {
		return zero, false
	}
	return typed, true
}

func setCachedValue[T any](app *App, key string, value T, ttl time.Duration) {
	app.Cache.Set(key, value, ttl)
}

func (app *App) getSiteConfig() (models.SiteConfig, error) {
	if cached, found := getCachedValue[models.SiteConfig](app, siteConfigCacheKey); found {
		return cached, nil
	}

	var siteConfig models.SiteConfig
	if err := app.DB.First(&siteConfig).Error; err != nil {
		return models.SiteConfig{}, err
	}

	setCachedValue(app, siteConfigCacheKey, siteConfig, publicCacheTTL)
	return siteConfig, nil
}

func (app *App) getCategories() ([]models.Category, error) {
	if cached, found := getCachedValue[[]models.Category](app, categoriesCacheKey); found {
		return cached, nil
	}

	var categories []models.Category
	if err := app.DB.Order("name asc").Find(&categories).Error; err != nil {
		return nil, err
	}

	setCachedValue(app, categoriesCacheKey, categories, publicCacheTTL)
	return categories, nil
}

func (app *App) getTags() ([]models.Tag, error) {
	if cached, found := getCachedValue[[]models.Tag](app, tagsCacheKey); found {
		return cached, nil
	}

	var tags []models.Tag
	if err := app.DB.Order("name asc").Find(&tags).Error; err != nil {
		return nil, err
	}

	setCachedValue(app, tagsCacheKey, tags, publicCacheTTL)
	return tags, nil
}

func (app *App) getHomePagePayload() (homePageCachePayload, error) {
	if cached, found := getCachedValue[homePageCachePayload](app, homePageCacheKey); found {
		return cached, nil
	}

	var resourceCount, articleCount, userCount, commentCount int64
	if err := app.DB.Model(&models.Resource{}).Where("status = ?", "approved").Count(&resourceCount).Error; err != nil {
		return homePageCachePayload{}, err
	}
	if err := app.DB.Model(&models.Article{}).Where("status = ?", "published").Count(&articleCount).Error; err != nil {
		return homePageCachePayload{}, err
	}
	if err := app.DB.Model(&models.User{}).Count(&userCount).Error; err != nil {
		return homePageCachePayload{}, err
	}
	if err := app.DB.Model(&models.Comment{}).Count(&commentCount).Error; err != nil {
		return homePageCachePayload{}, err
	}

	var hotResources []models.Resource
	if err := app.DB.Preload("Category").Where("status = ?", "approved").Order("views desc").Limit(6).Find(&hotResources).Error; err != nil {
		return homePageCachePayload{}, err
	}
	for i := range hotResources {
		if hotResources[i].RatingCount > 0 {
			hotResources[i].RatingScore = hotResources[i].RatingScore / float64(hotResources[i].RatingCount)
		}
	}

	var latestArticles []models.Article
	if err := app.DB.Preload("User").Where("status = ?", "published").Order("created_at desc").Limit(5).Find(&latestArticles).Error; err != nil {
		return homePageCachePayload{}, err
	}

	categories, err := app.getCategories()
	if err != nil {
		return homePageCachePayload{}, err
	}

	payload := homePageCachePayload{
		Stats: map[string]int64{
			"Resources": resourceCount,
			"Articles":  articleCount,
			"Users":     userCount,
			"Comments":  commentCount,
		},
		HotResources:   hotResources,
		LatestArticles: latestArticles,
		Categories:     categories,
	}

	setCachedValue(app, homePageCacheKey, payload, publicCacheTTL)
	return payload, nil
}

func (app *App) getResourceListPayload(q string, categoryID, tagID uint, resourceType string, page, pageSize int) (resourceListCachePayload, error) {
	key := buildResourceListCacheKey(q, categoryID, tagID, resourceType, page, pageSize)
	if cached, found := getCachedValue[resourceListCachePayload](app, key); found {
		return cached, nil
	}

	categories, err := app.getCategories()
	if err != nil {
		return resourceListCachePayload{}, err
	}

	tags, err := app.getTags()
	if err != nil {
		return resourceListCachePayload{}, err
	}

	query := app.DB.Preload("Category").Preload("Tags").Where("status = ?", "approved")
	if q != "" {
		like := "%" + q + "%"
		query = query.Where("title LIKE ? OR description LIKE ?", like, like)
	}
	if categoryID > 0 {
		query = query.Where("category_id = ?", categoryID)
	}
	if resourceType != "" {
		query = query.Where("resource_type = ?", resourceType)
	}
	if tagID > 0 {
		query = query.Joins("JOIN resource_tags ON resource_tags.resource_id = resources.id AND resource_tags.tag_id = ?", tagID)
	}

	var total int64
	if err := query.Model(&models.Resource{}).Count(&total).Error; err != nil {
		return resourceListCachePayload{}, err
	}

	var resources []models.Resource
	if err := query.Order("created_at desc").Offset((page - 1) * pageSize).Limit(pageSize).Find(&resources).Error; err != nil {
		return resourceListCachePayload{}, err
	}
	for i := range resources {
		if resources[i].RatingCount > 0 {
			resources[i].RatingScore = resources[i].RatingScore / float64(resources[i].RatingCount)
		}
	}

	payload := resourceListCachePayload{
		Categories: categories,
		Tags:       tags,
		Resources:  resources,
		Total:      total,
	}

	setCachedValue(app, key, payload, publicCacheTTL)
	return payload, nil
}

func (app *App) getArticleListPayload(page, pageSize int) (articleListCachePayload, error) {
	key := fmt.Sprintf("articles:list:%d:%d", page, pageSize)
	if cached, found := getCachedValue[articleListCachePayload](app, key); found {
		return cached, nil
	}

	var total int64
	if err := app.DB.Model(&models.Article{}).Where("status = ?", "published").Count(&total).Error; err != nil {
		return articleListCachePayload{}, err
	}

	var articles []models.Article
	if err := app.DB.Preload("User").Where("status = ?", "published").Order("created_at desc").Offset((page - 1) * pageSize).Limit(pageSize).Find(&articles).Error; err != nil {
		return articleListCachePayload{}, err
	}

	payload := articleListCachePayload{
		Articles: articles,
		Total:    total,
	}
	setCachedValue(app, key, payload, publicCacheTTL)
	return payload, nil
}

func buildResourceListCacheKey(q string, categoryID, tagID uint, resourceType string, page, pageSize int) string {
	return fmt.Sprintf(
		"resources:list:q=%s:category=%d:tag=%d:type=%s:page=%d:size=%d",
		url.QueryEscape(strings.TrimSpace(q)),
		categoryID,
		tagID,
		strings.TrimSpace(resourceType),
		page,
		pageSize,
	)
}
