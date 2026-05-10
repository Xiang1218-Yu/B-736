// Package app 提供应用程序的核心类型和共享依赖
package app

import (
	"html/template"
	"time"

	"prompt736/internal/config"
	"prompt736/internal/i18n"
	"prompt736/internal/models"

	"github.com/patrickmn/go-cache"
	"github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

// App 包含应用程序的核心依赖，在各层之间共享
type App struct {
	DB        *gorm.DB          // 数据库连接
	Cache     *cache.Cache      // 缓存实例
	Cfg       config.Config     // 配置信息
	Log       *logrus.Logger    // 日志实例
	Templates *template.Template // 模板集合
	I18n      *i18n.Bundle      // 国际化包
}

// PageData 页面渲染所需的通用数据结构
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

// BackupInfo 备份文件信息
type BackupInfo struct {
	Name string // 备份文件名
	Time string // 备份时间
}

// BackupSnapshot 数据库备份快照结构
type BackupSnapshot struct {
	Version      int                  `json:"version"`      // 快照版本
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
	CTA      string // 行动号召文本
	Contact  string // 联系方式
	Enabled  bool   // 是否启用
}

// adConfig 广告配置（用于JSON解析）
type adConfig struct {
	Slots []adSlotConfig `json:"slots"`
}

// adSlotConfig 单个广告位的JSON配置
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

// 业务常量定义
const (
	// 积分奖励
	CheckinRewardPoint = 10  // 签到奖励积分
	ShareRewardPoint   = 5   // 分享奖励积分
	PublishMinPoints   = 100 // 发布资源所需最低积分

	// 缓存键名
	SiteConfigCacheKey = "site-config"
	CategoriesCacheKey = "categories"
	TagsCacheKey       = "tags"
	HomePageCacheKey   = "home-page"
	PublicCacheTTL     = 2 * time.Minute // 公共缓存过期时间
)
