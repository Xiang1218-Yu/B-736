package service

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
	"strings"
	"time"

	"prompt736/internal/app"
	"prompt736/internal/models"

	"github.com/PuerkitoBio/goquery"
	"github.com/microcosm-cc/bluemonday"
	"github.com/patrickmn/go-cache"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ---- 常量与错误定义 ----

const (
	PublicCacheTTL    = 2 * time.Minute
	CheckinReward     = 10
	ShareReward       = 5
	PublishMinPoints  = 100
	csrfSessionKey    = "csrf_token"
	CSRFFormField     = "_csrf"
)

// CSRFSessionKey 返回 session 中存储 CSRF token 的 key
func CSRFSessionKey() string {
	return csrfSessionKey
}

// adConfig 用于解析广告 JSON 配置
type adConfig struct {
	Slots []adSlotConfig `json:"slots"`
}

// adSlotConfig 描述广告位 JSON 配置中的单条记录
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

var (
	ErrShareRewardLimited = errors.New("share reward already claimed")
	ErrAlreadyCheckedIn   = errors.New("already checked in today")
	articleHTMLPolicy     = newArticleHTMLPolicy()
)

// newArticleHTMLPolicy 创建 HTML 安全策略，允许 UGC 常用标签和属性
func newArticleHTMLPolicy() *bluemonday.Policy {
	policy := bluemonday.UGCPolicy()
	policy.AllowAttrs("class").Globally()
	policy.AllowAttrs("target").OnElements("a")
	policy.AllowAttrs("rel").OnElements("a")
	policy.AllowAttrs("src", "alt", "title", "width", "height").OnElements("img")
	policy.AllowAttrs("loading").OnElements("img")
	policy.AllowURLSchemes("http", "https", "mailto")
	return policy
}

// ---- 缓存 key 常量 ----

const (
	siteConfigCacheKey = "site-config"
	categoriesCacheKey = "categories"
	tagsCacheKey       = "tags"
	homePageCacheKey   = "home-page"
)

// ---- 缓存载荷结构体 ----

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

// Service 封装业务逻辑，依赖 App 实例
type Service struct {
	app *app.App
}

// New 创建 Service 实例
func New(a *app.App) *Service {
	return &Service{app: a}
}

// ---- 通用缓存泛型方法 ----

func getCachedValue[T any](c *cache.Cache, key string) (T, bool) {
	var zero T
	value, found := c.Get(key)
	if !found {
		return zero, false
	}
	typed, ok := value.(T)
	if !ok {
		return zero, false
	}
	return typed, true
}

func setCachedValue[T any](c *cache.Cache, key string, value T, ttl time.Duration) {
	c.Set(key, value, ttl)
}

// ---- 翻译与国际化辅助 ----

// Tr 根据 context 中的 locale 返回翻译后的文本
func (s *Service) Tr(locale, key string, args ...interface{}) string {
	return s.app.I18n.T(locale, key, args...)
}

// ResolveLocale 根据候选语言和默认语言，返回最终 locale
func (s *Service) ResolveLocale(candidate, defaultLocale string) string {
	return s.app.I18n.Resolve(candidate, defaultLocale)
}

// ---- 站点配置缓存 ----

// GetSiteConfig 获取站点配置（带缓存）
func (s *Service) GetSiteConfig() (models.SiteConfig, error) {
	if cached, found := getCachedValue[models.SiteConfig](s.app.Cache, siteConfigCacheKey); found {
		return cached, nil
	}
	var siteConfig models.SiteConfig
	if err := s.app.DB.First(&siteConfig).Error; err != nil {
		return models.SiteConfig{}, err
	}
	setCachedValue(s.app.Cache, siteConfigCacheKey, siteConfig, PublicCacheTTL)
	return siteConfig, nil
}

// ---- 分类与标签缓存 ----

// GetCategories 获取全部分类（带缓存）
func (s *Service) GetCategories() ([]models.Category, error) {
	if cached, found := getCachedValue[[]models.Category](s.app.Cache, categoriesCacheKey); found {
		return cached, nil
	}
	var categories []models.Category
	if err := s.app.DB.Order("name asc").Find(&categories).Error; err != nil {
		return nil, err
	}
	setCachedValue(s.app.Cache, categoriesCacheKey, categories, PublicCacheTTL)
	return categories, nil
}

// GetTags 获取全部标签（带缓存）
func (s *Service) GetTags() ([]models.Tag, error) {
	if cached, found := getCachedValue[[]models.Tag](s.app.Cache, tagsCacheKey); found {
		return cached, nil
	}
	var tags []models.Tag
	if err := s.app.DB.Order("name asc").Find(&tags).Error; err != nil {
		return nil, err
	}
	setCachedValue(s.app.Cache, tagsCacheKey, tags, PublicCacheTTL)
	return tags, nil
}

// ---- 首页数据缓存 ----

// GetHomePagePayload 获取首页所需数据（带缓存）
func (s *Service) GetHomePagePayload() (homePageCachePayload, error) {
	if cached, found := getCachedValue[homePageCachePayload](s.app.Cache, homePageCacheKey); found {
		return cached, nil
	}

	var resourceCount, articleCount, userCount, commentCount int64
	if err := s.app.DB.Model(&models.Resource{}).Where("status = ?", "approved").Count(&resourceCount).Error; err != nil {
		return homePageCachePayload{}, err
	}
	if err := s.app.DB.Model(&models.Article{}).Where("status = ?", "published").Count(&articleCount).Error; err != nil {
		return homePageCachePayload{}, err
	}
	if err := s.app.DB.Model(&models.User{}).Count(&userCount).Error; err != nil {
		return homePageCachePayload{}, err
	}
	if err := s.app.DB.Model(&models.Comment{}).Count(&commentCount).Error; err != nil {
		return homePageCachePayload{}, err
	}

	var hotResources []models.Resource
	if err := s.app.DB.Preload("Category").Where("status = ?", "approved").Order("views desc").Limit(6).Find(&hotResources).Error; err != nil {
		return homePageCachePayload{}, err
	}
	for i := range hotResources {
		if hotResources[i].RatingCount > 0 {
			hotResources[i].RatingScore = hotResources[i].RatingScore / float64(hotResources[i].RatingCount)
		}
	}

	var latestArticles []models.Article
	if err := s.app.DB.Preload("User").Where("status = ?", "published").Order("created_at desc").Limit(5).Find(&latestArticles).Error; err != nil {
		return homePageCachePayload{}, err
	}

	categories, err := s.GetCategories()
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

	setCachedValue(s.app.Cache, homePageCacheKey, payload, PublicCacheTTL)
	return payload, nil
}

// ---- 资源列表缓存 ----

// GetResourceListPayload 获取资源列表数据（带缓存）
func (s *Service) GetResourceListPayload(q string, categoryID, tagID uint, resourceType string, page, pageSize int) (resourceListCachePayload, error) {
	key := buildResourceListCacheKey(q, categoryID, tagID, resourceType, page, pageSize)
	if cached, found := getCachedValue[resourceListCachePayload](s.app.Cache, key); found {
		return cached, nil
	}

	categories, err := s.GetCategories()
	if err != nil {
		return resourceListCachePayload{}, err
	}

	tags, err := s.GetTags()
	if err != nil {
		return resourceListCachePayload{}, err
	}

	query := s.app.DB.Preload("Category").Preload("Tags").Where("status = ?", "approved")
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

	setCachedValue(s.app.Cache, key, payload, PublicCacheTTL)
	return payload, nil
}

// ---- 文章列表缓存 ----

// GetArticleListPayload 获取文章列表数据（带缓存）
func (s *Service) GetArticleListPayload(page, pageSize int) (articleListCachePayload, error) {
	key := fmt.Sprintf("articles:list:%d:%d", page, pageSize)
	if cached, found := getCachedValue[articleListCachePayload](s.app.Cache, key); found {
		return cached, nil
	}

	var total int64
	if err := s.app.DB.Model(&models.Article{}).Where("status = ?", "published").Count(&total).Error; err != nil {
		return articleListCachePayload{}, err
	}

	var articles []models.Article
	if err := s.app.DB.Preload("User").Where("status = ?", "published").Order("created_at desc").Offset((page - 1) * pageSize).Limit(pageSize).Find(&articles).Error; err != nil {
		return articleListCachePayload{}, err
	}

	payload := articleListCachePayload{
		Articles: articles,
		Total:    total,
	}
	setCachedValue(s.app.Cache, key, payload, PublicCacheTTL)
	return payload, nil
}

// ---- 签到 ----

// Checkin 执行用户签到，返回是否成功
func (s *Service) Checkin(user *models.User) error {
	now := time.Now()
	start, end := dayRange(now)
	return s.app.DB.Transaction(func(tx *gorm.DB) error {
		var lockedUser models.User
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&lockedUser, user.ID).Error; err != nil {
			return err
		}
		if lockedUser.LastCheckinAt != nil && !lockedUser.LastCheckinAt.Before(start) && lockedUser.LastCheckinAt.Before(end) {
			return ErrAlreadyCheckedIn
		}
		return tx.Model(&models.User{}).Where("id = ?", user.ID).Updates(map[string]interface{}{
			"points":          gorm.Expr("points + ?", CheckinReward),
			"last_checkin_at": now,
		}).Error
	})
}

// ---- 分享奖励 ----

// ClaimShareReward 领取分享奖励积分，同一资源每天只能领取一次
func (s *Service) ClaimShareReward(user *models.User, resourceID uint) error {
	start, end := dayRange(time.Now())
	return s.app.DB.Transaction(func(tx *gorm.DB) error {
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
			return ErrShareRewardLimited
		}

		if err := tx.Model(&models.ShareLog{}).
			Where("user_id = ? AND created_at >= ? AND created_at < ?", user.ID, start, end).
			Count(&claimedCount).Error; err != nil {
			return err
		}
		if claimedCount > 0 {
			return ErrShareRewardLimited
		}

		if err := tx.Create(&models.ShareLog{UserID: user.ID, ResourceID: resourceID}).Error; err != nil {
			return err
		}

		return tx.Model(&models.User{}).Where("id = ?", user.ID).
			Update("points", gorm.Expr("points + ?", ShareReward)).Error
	})
}

// ---- 评论 ----

// CreateResourceComment 创建资源评论并更新评分统计
func (s *Service) CreateResourceComment(userID, resourceID uint, content string, rating int) error {
	comment := models.Comment{
		Content:    content,
		Rating:     rating,
		UserID:     userID,
		ResourceID: ptrUint(resourceID),
	}
	return s.app.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&comment).Error; err != nil {
			return err
		}
		return tx.Model(&models.Resource{}).Where("id = ?", resourceID).Updates(map[string]interface{}{
			"rating_score": gorm.Expr("rating_score + ?", float64(rating)),
			"rating_count": gorm.Expr("rating_count + ?", 1),
		}).Error
	})
}

// CreateArticleComment 创建文章评论
func (s *Service) CreateArticleComment(userID, articleID uint, content string) error {
	comment := models.Comment{
		Content:   content,
		UserID:    userID,
		ArticleID: ptrUint(articleID),
	}
	return s.app.DB.Create(&comment).Error
}

// ---- 资源发布 ----

// PublishManualResource 手动发布资源
func (s *Service) PublishManualResource(user *models.User, title, description, resourceType, link, fileName string, categoryID uint, tagIDs []uint) (*models.Resource, error) {
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
		ResourceType: CanonicalResourceType(resourceType),
		Link:         link,
		FilePath:     fileName,
		SourceType:   "manual",
		Status:       status,
		UserID:       user.ID,
		CategoryID:   categoryPtr,
	}

	if len(tagIDs) > 0 {
		var tags []models.Tag
		if err := s.app.DB.Find(&tags, tagIDs).Error; err == nil && len(tags) > 0 {
			resource.Tags = tags
		}
	}

	if err := s.app.DB.Create(&resource).Error; err != nil {
		return nil, err
	}
	s.app.Cache.Flush()
	return &resource, nil
}

// PublishImportResources 从 CSV 导入资源，返回 (成功数, 跳过数, error)
func (s *Service) PublishImportResources(user *models.User, src io.Reader, defaultType string, categoryID uint, tagIDs []uint) (int, int, error) {
	defaultType = CanonicalResourceType(defaultType)
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
		s.app.DB.Find(&tags, tagIDs)
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
		if link != "" && !IsValidHTTPURL(link) {
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

		if err := s.app.DB.Create(&resource).Error; err != nil {
			skipped++
			continue
		}
		if len(tags) > 0 {
			if err := s.app.DB.Model(&resource).Association("Tags").Append(tags); err != nil {
				s.app.Log.WithError(err).Warn("append tags for imported resource failed")
			}
		}
		created++
	}

	if created > 0 {
		s.app.Cache.Flush()
	}
	return created, skipped, nil
}

// PublishCrawlerResource 通过爬虫抓取网页信息发布资源
func (s *Service) PublishCrawlerResource(user *models.User, crawlURL, resourceType string, categoryID uint, tagIDs []uint) (*models.Resource, error) {
	client := &http.Client{Timeout: 10 * time.Second}
	req, reqErr := http.NewRequest(http.MethodGet, crawlURL, nil)
	if reqErr != nil {
		return nil, fmt.Errorf("create crawl request failed: %w", reqErr)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; GalaxyHubCrawler/1.0)")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("crawl request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 400 {
		return nil, fmt.Errorf("crawl returned status %d", resp.StatusCode)
	}

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("parse crawled page failed: %w", err)
	}

	title := strings.TrimSpace(doc.Find("meta[property='og:title']").AttrOr("content", ""))
	if title == "" {
		title = strings.TrimSpace(doc.Find("title").First().Text())
	}
	if title == "" {
		parsed, parseErr := url.Parse(crawlURL)
		if parseErr == nil && parsed.Host != "" {
			title = parsed.Host
		}
	}

	description := strings.TrimSpace(doc.Find("meta[name='description']").AttrOr("content", ""))
	if description == "" {
		description = strings.TrimSpace(doc.Find("meta[property='og:description']").AttrOr("content", ""))
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
		ResourceType: CanonicalResourceType(resourceType),
		Link:         crawlURL,
		SourceType:   "crawler",
		Status:       status,
		UserID:       user.ID,
		CategoryID:   categoryPtr,
	}

	if len(tagIDs) > 0 {
		var tags []models.Tag
		if err := s.app.DB.Find(&tags, tagIDs).Error; err == nil && len(tags) > 0 {
			resource.Tags = tags
		}
	}

	if err := s.app.DB.Create(&resource).Error; err != nil {
		return nil, err
	}
	s.app.Cache.Flush()
	return &resource, nil
}

// ---- 管理员：资源审核 ----

// ReviewResource 审核资源（批准或拒绝）
func (s *Service) ReviewResource(resourceID uint, status string) {
	if status == "approved" || status == "rejected" {
		s.app.DB.Model(&models.Resource{}).Where("id = ?", resourceID).Update("status", status)
		s.app.Cache.Flush()
	}
}

// ---- 管理员：文章管理 ----

// CreateArticle 创建文章
func (s *Service) CreateArticle(userID uint, title, summary, content string) {
	article := models.Article{
		Title:   title,
		Summary: summary,
		Content: SanitizeArticleContent(content),
		Status:  "published",
		UserID:  userID,
	}
	s.app.DB.Create(&article)
	s.app.Cache.Flush()
}

// UpdateArticle 更新文章
func (s *Service) UpdateArticle(articleID uint, title, summary, content string) {
	s.app.DB.Model(&models.Article{}).Where("id = ?", articleID).Updates(map[string]interface{}{
		"title":   title,
		"summary": summary,
		"content": SanitizeArticleContent(content),
	})
	s.app.Cache.Flush()
}

// ---- 管理员：分类管理 ----

// CreateCategory 创建分类
func (s *Service) CreateCategory(name string) {
	if name != "" {
		s.app.DB.Create(&models.Category{Name: name})
		s.app.Cache.Flush()
	}
}

// UpdateCategory 更新分类名称
func (s *Service) UpdateCategory(categoryID uint, name string) {
	if name != "" {
		s.app.DB.Model(&models.Category{}).Where("id = ?", categoryID).Update("name", name)
		s.app.Cache.Flush()
	}
}

// DeleteCategory 删除分类并解除关联资源
func (s *Service) DeleteCategory(categoryID uint) error {
	return s.app.DB.Transaction(func(tx *gorm.DB) error {
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
		s.app.Cache.Flush()
		return nil
	})
}

// ---- 管理员：标签管理 ----

// CreateTag 创建标签
func (s *Service) CreateTag(name string) {
	if name != "" {
		s.app.DB.Create(&models.Tag{Name: name})
		s.app.Cache.Flush()
	}
}

// UpdateTag 更新标签名称
func (s *Service) UpdateTag(tagID uint, name string) {
	if name != "" {
		s.app.DB.Model(&models.Tag{}).Where("id = ?", tagID).Update("name", name)
		s.app.Cache.Flush()
	}
}

// DeleteTag 删除标签
func (s *Service) DeleteTag(tagID uint) {
	s.app.DB.Delete(&models.Tag{}, tagID)
	s.app.Cache.Flush()
}

// ---- 管理员：用户管理 ----

// UpdateUser 更新用户角色和状态
func (s *Service) UpdateUser(userID uint, role, status string) error {
	updates := map[string]interface{}{}
	if role != "" {
		updates["role"] = role
	}
	if status != "" {
		updates["status"] = status
	}
	if len(updates) > 0 {
		return s.app.DB.Model(&models.User{}).Where("id = ?", userID).Updates(updates).Error
	}
	return nil
}

// ---- 管理员：站点设置 ----

// UpdateSettings 更新站点配置
func (s *Service) UpdateSettings(cfg *models.SiteConfig, siteName, siteDesc, heroTitle, heroSubtitle, localeDefault, adsJSON string) {
	cfg.SiteName = siteName
	cfg.SiteDesc = siteDesc
	cfg.HeroTitle = heroTitle
	cfg.HeroSubtitle = heroSubtitle
	cfg.LocaleDefault = localeDefault
	cfg.AdsJSON = adsJSON
	s.app.DB.Save(cfg)
	s.app.Cache.Flush()
}

// ---- 管理员：备份与恢复 ----

// CollectBackupSnapshot 收集完整数据快照
func (s *Service) CollectBackupSnapshot() (*app.BackupSnapshot, error) {
	snapshot := &app.BackupSnapshot{
		Version:     1,
		GeneratedAt: time.Now().UTC(),
	}

	if err := s.app.DB.Transaction(func(tx *gorm.DB) error {
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

// WriteSnapshotAtomic 原子写入备份快照到 JSON 文件
func WriteSnapshotAtomic(path string, snapshot *app.BackupSnapshot) error {
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

// ResolveBackupPath 安全地解析备份文件路径，防止目录遍历攻击
func (s *Service) ResolveBackupPath(filename string) (string, error) {
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

	baseAbs, err := filepath.Abs(s.app.Cfg.BackupDir)
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

// RestoreFromJSONBackup 从 JSON 备份恢复数据
func (s *Service) RestoreFromJSONBackup(path string) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	var snapshot app.BackupSnapshot
	if err := json.Unmarshal(raw, &snapshot); err != nil {
		return err
	}

	return s.app.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("SET FOREIGN_KEY_CHECKS = 0").Error; err != nil {
			return err
		}
		defer func() {
			if enableErr := tx.Exec("SET FOREIGN_KEY_CHECKS = 1").Error; enableErr != nil {
				s.app.Log.WithError(enableErr).Warn("re-enable foreign key checks failed")
			}
		}()

		for _, table := range []string{
			"resource_tags", "share_logs", "comments", "articles",
			"resources", "tags", "categories", "users", "site_configs",
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

		return tx.Exec("SET FOREIGN_KEY_CHECKS = 1").Error
	})
}

// RestoreFromSQLBackup 从 SQL 备份恢复数据
func (s *Service) RestoreFromSQLBackup(path string) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	statements := splitSQLStatements(string(raw))
	if len(statements) == 0 {
		return errors.New("no executable SQL statements found")
	}

	return s.app.DB.Transaction(func(tx *gorm.DB) error {
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

// ---- 管理员：仪表盘 ----

// GetDashboardStats 获取管理后台统计信息
func (s *Service) GetDashboardStats() map[string]int64 {
	var resourceCount, pendingCount, articleCount, userCount int64
	s.app.DB.Model(&models.Resource{}).Count(&resourceCount)
	s.app.DB.Model(&models.Resource{}).Where("status = ?", "pending").Count(&pendingCount)
	s.app.DB.Model(&models.Article{}).Count(&articleCount)
	s.app.DB.Model(&models.User{}).Count(&userCount)
	return map[string]int64{
		"Resources": resourceCount,
		"Pending":   pendingCount,
		"Articles":  articleCount,
		"Users":     userCount,
	}
}

// GetPendingResources 获取待审核资源列表
func (s *Service) GetPendingResources(limit int) []models.Resource {
	var pendingResources []models.Resource
	s.app.DB.Preload("User").Where("status = ?", "pending").Order("created_at desc").Limit(limit).Find(&pendingResources)
	return pendingResources
}

// ListBackups 列出备份目录中的备份文件
func (s *Service) ListBackups() []app.BackupInfo {
	if err := os.MkdirAll(s.app.Cfg.BackupDir, 0755); err != nil {
		s.app.Log.WithError(err).Warn("create backup dir failed")
	}
	var backups []app.BackupInfo
	files, err := os.ReadDir(s.app.Cfg.BackupDir)
	if err != nil {
		s.app.Log.WithError(err).Warn("read backup dir failed")
		return backups
	}
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
			s.app.Log.WithError(infoErr).Warn("read backup file info failed")
			continue
		}
		backups = append(backups, app.BackupInfo{
			Name: f.Name(),
			Time: info.ModTime().Format("2006-01-02 15:04"),
		})
	}
	return backups
}

// ---- 管理员：保存上传文件 ----

// SaveUploadedFile 保存上传文件到指定目录，返回生成的文件名
func (s *Service) SaveUploadedFile(fileHeader io.Reader, originalName string, userID uint) (string, error) {
	ext := strings.ToLower(filepath.Ext(originalName))
	if ext == "" {
		ext = ".bin"
	}
	fileName := fmt.Sprintf("%d_%d%s", userID, time.Now().UnixNano(), ext)
	if err := os.MkdirAll(s.app.Cfg.UploadDir, 0755); err != nil {
		return "", err
	}
	target := filepath.Join(s.app.Cfg.UploadDir, fileName)
	if f, err := os.Create(target); err != nil {
		return "", err
	} else {
		defer f.Close()
		if _, err := io.Copy(f, fileHeader); err != nil {
			return "", err
		}
	}
	return fileName, nil
}

// ---- 广告位解析 ----

// ParseAdSlots 解析广告 JSON 配置，合并默认广告位
func (s *Service) ParseAdSlots(adsJSON, locale string) map[string]app.AdSlot {
	slots := s.defaultAdSlots(locale)
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
			slot = app.AdSlot{
				Code:    code,
				CTA:     s.adText(locale, "ad.learn_more", "Learn More"),
				Contact: s.adText(locale, "ad.default_contact", "Business Contact: WeChat GalaxyHub_BD / Email business@galaxyhub.example"),
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

func (s *Service) adText(locale, key, fallback string) string {
	value := strings.TrimSpace(s.app.I18n.T(locale, key))
	if value == "" || value == key {
		return fallback
	}
	return value
}

func (s *Service) defaultAdSlots(locale string) map[string]app.AdSlot {
	defaultContact := s.adText(locale, "ad.default_contact", "Business Contact: WeChat GalaxyHub_BD / Email business@galaxyhub.example")
	return map[string]app.AdSlot{
		"hero-banner": {
			Code: "hero-banner", Title: s.adText(locale, "ad.slot_defaults.hero_banner.title", "品牌合作专区"),
			Desc: s.adText(locale, "ad.slot_defaults.hero_banner.desc", "预留标准广告位，支持后续广告系统对接。"),
			CTA: s.adText(locale, "ad.slot_defaults.hero_banner.cta", s.adText(locale, "ad.learn_more", "Learn More")),
			Contact: s.adText(locale, "ad.slot_defaults.hero_banner.contact", defaultContact), Enabled: true,
		},
		"resource-list-inline": {
			Code: "resource-list-inline", Title: s.adText(locale, "ad.slot_defaults.resource_list_inline.title", "精选推广资源位"),
			Desc: s.adText(locale, "ad.slot_defaults.resource_list_inline.desc", "资源列表内标准广告位，可按 code 接入投放系统。"),
			CTA: s.adText(locale, "ad.slot_defaults.resource_list_inline.cta", s.adText(locale, "ad.learn_more", "Learn More")),
			Contact: s.adText(locale, "ad.slot_defaults.resource_list_inline.contact", defaultContact), Enabled: true,
		},
		"resource-detail-sidebar": {
			Code: "resource-detail-sidebar", Title: s.adText(locale, "ad.slot_defaults.resource_detail_sidebar.title", "侧边广告位"),
			Desc: s.adText(locale, "ad.slot_defaults.resource_detail_sidebar.desc", "详情页侧栏广告位，支持图文与跳转链接。"),
			CTA: s.adText(locale, "ad.slot_defaults.resource_detail_sidebar.cta", s.adText(locale, "ad.learn_more", "Learn More")),
			Contact: s.adText(locale, "ad.slot_defaults.resource_detail_sidebar.contact", defaultContact), Enabled: true,
		},
		"article-detail-inline": {
			Code: "article-detail-inline", Title: s.adText(locale, "ad.slot_defaults.article_detail_inline.title", "内容推广位"),
			Desc: s.adText(locale, "ad.slot_defaults.article_detail_inline.desc", "文章详情中部广告位，便于后续扩展素材形式。"),
			CTA: s.adText(locale, "ad.slot_defaults.article_detail_inline.cta", s.adText(locale, "ad.learn_more", "Learn More")),
			Contact: s.adText(locale, "ad.slot_defaults.article_detail_inline.contact", defaultContact), Enabled: true,
		},
		"global-footer": {
			Code: "global-footer", Title: s.adText(locale, "ad.slot_defaults.global_footer.title", "全站底部广告位"),
			Desc: s.adText(locale, "ad.slot_defaults.global_footer.desc", "全站统一广告位，适合品牌露出。"),
			CTA: s.adText(locale, "ad.slot_defaults.global_footer.cta", s.adText(locale, "ad.learn_more", "Learn More")),
			Contact: s.adText(locale, "ad.slot_defaults.global_footer.contact", "Business Contact: WeChat GalaxyHub_Service / Email contact@galaxyhub.example"),
			Enabled: true,
		},
	}
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

// ---- 工具函数 ----

func buildResourceListCacheKey(q string, categoryID, tagID uint, resourceType string, page, pageSize int) string {
	return fmt.Sprintf(
		"resources:list:q=%s:category=%d:tag=%d:type=%s:page=%d:size=%d",
		url.QueryEscape(strings.TrimSpace(q)),
		categoryID, tagID,
		strings.TrimSpace(resourceType),
		page, pageSize,
	)
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

func dayRange(now time.Time) (time.Time, time.Time) {
	year, month, day := now.Date()
	location := now.Location()
	start := time.Date(year, month, day, 0, 0, 0, 0, location)
	return start, start.Add(24 * time.Hour)
}

func looksLikeCSVHeader(record []string) bool {
	if len(record) == 0 {
		return false
	}
	first := strings.ToLower(strings.TrimSpace(record[0]))
	return first == "title" || first == "标题"
}

func csvCell(record []string, index int) string {
	if index < 0 || index >= len(record) {
		return ""
	}
	return strings.TrimSpace(record[index])
}

func ptrUint(v uint) *uint {
	return &v
}

// CanonicalResourceType 将各种资源类型别名统一为标准类型
func CanonicalResourceType(resourceType string) string {
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

// CanonicalSourceType 将各种来源类型别名统一为标准来源
func CanonicalSourceType(sourceType string) string {
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

// IsValidHTTPURL 验证是否为合法 HTTP/HTTPS URL
func IsValidHTTPURL(raw string) bool {
	parsed, err := url.Parse(raw)
	if err != nil {
		return false
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return false
	}
	return strings.TrimSpace(parsed.Host) != ""
}

// CanPublishResource 判断用户是否有权发布资源
func CanPublishResource(user *models.User) bool {
	if user == nil {
		return false
	}
	if strings.ToLower(strings.TrimSpace(user.Status)) != "active" {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(user.Role)) {
	case "admin", "editor":
		return true
	case "user":
		return user.Points >= PublishMinPoints
	default:
		return false
	}
}

// SanitizeArticleContent 使用 bluemonday 清洗 HTML 内容
func SanitizeArticleContent(raw string) string {
	cleaned := articleHTMLPolicy.Sanitize(strings.TrimSpace(raw))
	return strings.TrimSpace(cleaned)
}

// SafeArticleHTML 将清洗后的 HTML 内容标记为安全输出
func SafeArticleHTML(raw string) template.HTML {
	return template.HTML(SanitizeArticleContent(raw))
}

// BuildRatingStars 根据评分生成星级字符串
func BuildRatingStars(score float64) string {
	stars := int(score + 0.5)
	if stars < 0 {
		stars = 0
	}
	if stars > 5 {
		stars = 5
	}
	return strings.Repeat("★", stars) + strings.Repeat("☆", 5-stars)
}

// BuildLangSwitchURL 构建语言切换 URL
func BuildLangSwitchURL(path, rawQuery, lang string) string {
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
