package service

import (
	"fmt"
	"net/url"
	"strings"
	"time"

	"prompt736/internal/models"
	"prompt736/internal/types"
)

// GetSiteConfig 获取站点配置（带缓存）
func (app *App) GetSiteConfig() (models.SiteConfig, error) {
	if cached, found := getCachedValue[models.SiteConfig](app, types.SiteConfigCacheKey); found {
		return cached, nil
	}

	var siteConfig models.SiteConfig
	if err := app.DB.First(&siteConfig).Error; err != nil {
		return models.SiteConfig{}, err
	}

	setCachedValue(app, types.SiteConfigCacheKey, siteConfig, types.PublicCacheTTL)
	return siteConfig, nil
}

// GetCategories 获取分类列表（带缓存）
func (app *App) GetCategories() ([]models.Category, error) {
	if cached, found := getCachedValue[[]models.Category](app, types.CategoriesCacheKey); found {
		return cached, nil
	}

	var categories []models.Category
	if err := app.DB.Order("name asc").Find(&categories).Error; err != nil {
		return nil, err
	}

	setCachedValue(app, types.CategoriesCacheKey, categories, types.PublicCacheTTL)
	return categories, nil
}

// GetTags 获取标签列表（带缓存）
func (app *App) GetTags() ([]models.Tag, error) {
	if cached, found := getCachedValue[[]models.Tag](app, types.TagsCacheKey); found {
		return cached, nil
	}

	var tags []models.Tag
	if err := app.DB.Order("name asc").Find(&tags).Error; err != nil {
		return nil, err
	}

	setCachedValue(app, types.TagsCacheKey, tags, types.PublicCacheTTL)
	return tags, nil
}

// GetHomePagePayload 获取首页数据（带缓存）
func (app *App) GetHomePagePayload() (types.HomePageCachePayload, error) {
	if cached, found := getCachedValue[types.HomePageCachePayload](app, types.HomePageCacheKey); found {
		return cached, nil
	}

	var resourceCount, articleCount, userCount, commentCount int64
	if err := app.DB.Model(&models.Resource{}).Where("status = ?", "approved").Count(&resourceCount).Error; err != nil {
		return types.HomePageCachePayload{}, err
	}
	if err := app.DB.Model(&models.Article{}).Where("status = ?", "published").Count(&articleCount).Error; err != nil {
		return types.HomePageCachePayload{}, err
	}
	if err := app.DB.Model(&models.User{}).Count(&userCount).Error; err != nil {
		return types.HomePageCachePayload{}, err
	}
	if err := app.DB.Model(&models.Comment{}).Count(&commentCount).Error; err != nil {
		return types.HomePageCachePayload{}, err
	}

	var hotResources []models.Resource
	if err := app.DB.Preload("Category").Where("status = ?", "approved").Order("views desc").Limit(6).Find(&hotResources).Error; err != nil {
		return types.HomePageCachePayload{}, err
	}
	for i := range hotResources {
		if hotResources[i].RatingCount > 0 {
			hotResources[i].RatingScore = hotResources[i].RatingScore / float64(hotResources[i].RatingCount)
		}
	}

	var latestArticles []models.Article
	if err := app.DB.Preload("User").Where("status = ?", "published").Order("created_at desc").Limit(5).Find(&latestArticles).Error; err != nil {
		return types.HomePageCachePayload{}, err
	}

	categories, err := app.GetCategories()
	if err != nil {
		return types.HomePageCachePayload{}, err
	}

	payload := types.HomePageCachePayload{
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

	setCachedValue(app, types.HomePageCacheKey, payload, types.PublicCacheTTL)
	return payload, nil
}

// GetResourceListPayload 获取资源列表数据（带缓存）
func (app *App) GetResourceListPayload(q string, categoryID, tagID uint, resourceType string, page, pageSize int) (types.ResourceListCachePayload, error) {
	key := buildResourceListCacheKey(q, categoryID, tagID, resourceType, page, pageSize)
	if cached, found := getCachedValue[types.ResourceListCachePayload](app, key); found {
		return cached, nil
	}

	categories, err := app.GetCategories()
	if err != nil {
		return types.ResourceListCachePayload{}, err
	}

	tags, err := app.GetTags()
	if err != nil {
		return types.ResourceListCachePayload{}, err
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
		return types.ResourceListCachePayload{}, err
	}

	var resources []models.Resource
	if err := query.Order("created_at desc").Offset((page - 1) * pageSize).Limit(pageSize).Find(&resources).Error; err != nil {
		return types.ResourceListCachePayload{}, err
	}
	for i := range resources {
		if resources[i].RatingCount > 0 {
			resources[i].RatingScore = resources[i].RatingScore / float64(resources[i].RatingCount)
		}
	}

	payload := types.ResourceListCachePayload{
		Categories: categories,
		Tags:       tags,
		Resources:  resources,
		Total:      total,
	}

	setCachedValue(app, key, payload, types.PublicCacheTTL)
	return payload, nil
}

// GetArticleListPayload 获取文章列表数据（带缓存）
func (app *App) GetArticleListPayload(page, pageSize int) (types.ArticleListCachePayload, error) {
	key := fmt.Sprintf("articles:list:%d:%d", page, pageSize)
	if cached, found := getCachedValue[types.ArticleListCachePayload](app, key); found {
		return cached, nil
	}

	var total int64
	if err := app.DB.Model(&models.Article{}).Where("status = ?", "published").Count(&total).Error; err != nil {
		return types.ArticleListCachePayload{}, err
	}

	var articles []models.Article
	if err := app.DB.Preload("User").Where("status = ?", "published").Order("created_at desc").Offset((page - 1) * pageSize).Limit(pageSize).Find(&articles).Error; err != nil {
		return types.ArticleListCachePayload{}, err
	}

	payload := types.ArticleListCachePayload{
		Articles: articles,
		Total:    total,
	}
	setCachedValue(app, key, payload, types.PublicCacheTTL)
	return payload, nil
}

// ClearCache 清空所有缓存
func (app *App) ClearCache() {
	app.Cache.Flush()
}

// buildResourceListCacheKey 构建资源列表缓存键
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
