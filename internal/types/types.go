package types

import (
	"time"

	"prompt736/internal/models"
)

// BackupInfo 备份文件信息
type BackupInfo struct {
	Name string // 文件名
	Time string // 修改时间
}

// BackupSnapshot 备份快照结构
type BackupSnapshot struct {
	Version      int                  `json:"version"`      // 版本号
	GeneratedAt  time.Time            `json:"generated_at"` // 生成时间
	Users        []models.User        `json:"users"`
	Categories   []models.Category    `json:"categories"`
	Tags         []models.Tag         `json:"tags"`
	Resources    []models.Resource    `json:"resources"`
	Articles     []models.Article     `json:"articles"`
	Comments     []models.Comment     `json:"comments"`
	SiteConfigs  []models.SiteConfig  `json:"site_configs"`
	ShareLogs    []models.ShareLog    `json:"share_logs"`
	ResourceTags []models.ResourceTag `json:"resource_tags"`
}

// AdSlot 广告位配置
type AdSlot struct {
	Code     string // 广告位代码
	Title    string // 标题
	Desc     string // 描述
	Link     string // 跳转链接
	ImageURL string // 图片URL
	CTA      string // 号召性用语
	Contact  string // 联系方式
	Enabled  bool   // 是否启用
}

// AdConfig 广告配置
type AdConfig struct {
	Slots []AdSlotConfig `json:"slots"`
}

// AdSlotConfig 单个广告位配置
type AdSlotConfig struct {
	Code     string `json:"code"`
	Title    string `json:"title"`
	Desc     string `json:"desc"`
	Link     string `json:"link"`
	ImageURL string `json:"image_url"`
	CTA      string `json:"cta"`
	Contact  string `json:"contact"`
	Enabled  *bool  `json:"enabled"`
}

// PageData 页面通用数据结构
type PageData struct {
	Title        string
	ActiveNav    string
	User         *models.User
	SiteConfig   models.SiteConfig
	Ads          map[string]AdSlot
	FlashSuccess string
	FlashError   string
	Data         interface{}
	Locale       string
	RequestPath  string
	RawQuery     string
	CSRFToken    string

	// 首页数据
	Stats          map[string]int64
	HotResources   []models.Resource
	LatestArticles []models.Article
	Categories     []models.Category

	// 列表页数据
	Resources   []models.Resource
	Articles    []models.Article
	Tags        []models.Tag
	Query       string
	CategoryID  uint
	TagID       uint
	Type        string
	QueryParams string
	Page        int
	PageSize    int
	Total       int64
	TotalPages  int

	// 详情页数据
	Resource *models.Resource
	Article  *models.Article
	Comments []models.Comment

	// 个人中心
	CanCheckin       bool
	CanPublish       bool
	PublishMinPoints int

	// 管理后台
	Users            []models.User
	Status           string
	PendingResources []models.Resource
	Backups          []BackupInfo
	Config           *models.SiteConfig
}

// 常量定义
const (
	CSRFSessionKey     = "csrf_token"
	CSRFFormField      = "_csrf"
	CheckinRewardPoint = 10
	ShareRewardPoint   = 5
	PublishMinPoints   = 100
	PublicCacheTTL     = 2 * time.Minute
)

// 缓存键常量
const (
	SiteConfigCacheKey = "site-config"
	CategoriesCacheKey = "categories"
	TagsCacheKey       = "tags"
	HomePageCacheKey   = "home-page"
)

// HomePageCachePayload 首页缓存数据
type HomePageCachePayload struct {
	Stats          map[string]int64
	HotResources   []models.Resource
	LatestArticles []models.Article
	Categories     []models.Category
}

// ResourceListCachePayload 资源列表缓存数据
type ResourceListCachePayload struct {
	Categories []models.Category
	Tags       []models.Tag
	Resources  []models.Resource
	Total      int64
}

// ArticleListCachePayload 文章列表缓存数据
type ArticleListCachePayload struct {
	Articles []models.Article
	Total    int64
}
