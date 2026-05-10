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

// App 是应用核心结构体，持有数据库连接、缓存、配置、日志、模板引擎和国际化实例
type App struct {
	DB        *gorm.DB
	Cache     *cache.Cache
	Cfg       config.Config
	Log       *logrus.Logger
	Templates *template.Template
	I18n      *i18n.Bundle
}

// PageData 是传递给模板的统一数据结构，包含所有页面可能用到的字段
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

	Stats          map[string]int64
	HotResources   []models.Resource
	LatestArticles []models.Article
	Categories     []models.Category

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

	Resource *models.Resource
	Article  *models.Article
	Comments []models.Comment

	CanCheckin       bool
	CanPublish       bool
	PublishMinPoints int

	Users            []models.User
	Status           string
	PendingResources []models.Resource
	Backups          []BackupInfo
	Config           *models.SiteConfig
}

// BackupInfo 描述一条备份文件信息
type BackupInfo struct {
	Name string
	Time string
}

// BackupSnapshot 是 JSON 备份的完整数据快照
type BackupSnapshot struct {
	Version      int                  `json:"version"`
	GeneratedAt  time.Time            `json:"generated_at"`
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

// AdSlot 描述一个广告位的完整信息
type AdSlot struct {
	Code     string
	Title    string
	Desc     string
	Link     string
	ImageURL string
	CTA      string
	Contact  string
	Enabled  bool
}

// New 创建并返回 App 实例
func New(db *gorm.DB, cacheStore *cache.Cache, cfg config.Config, log *logrus.Logger, tmpl *template.Template, bundle *i18n.Bundle) *App {
	return &App{
		DB:        db,
		Cache:     cacheStore,
		Cfg:       cfg,
		Log:       log,
		Templates: tmpl,
		I18n:      bundle,
	}
}
