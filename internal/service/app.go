package service

import (
	"html/template"

	"prompt736/internal/config"
	"prompt736/internal/i18n"

	"github.com/patrickmn/go-cache"
	"github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

// App 应用程序核心结构，包含所有依赖
type App struct {
	DB        *gorm.DB          // 数据库连接
	Cache     *cache.Cache      // 缓存实例
	Cfg       config.Config     // 配置信息
	Log       *logrus.Logger    // 日志实例
	Templates *template.Template // 模板引擎
	I18n      *i18n.Bundle     // 国际化bundle
}

// NewApp 创建新的应用实例
func NewApp(db *gorm.DB, cache *cache.Cache, cfg config.Config, log *logrus.Logger, tmpl *template.Template, i18n *i18n.Bundle) *App {
	return &App{
		DB:        db,
		Cache:     cache,
		Cfg:       cfg,
		Log:       log,
		Templates: tmpl,
		I18n:      i18n,
	}
}

// getCachedValue 从缓存获取值
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

// setCachedValue 设置缓存值
func setCachedValue[T any](app *App, key string, value T, ttl cache.Expiration) {
	app.Cache.Set(key, value, ttl)
}
