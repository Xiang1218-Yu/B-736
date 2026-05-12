package service

import (
	"prompt736/internal/models"
)

// ReviewResource 审核资源
func (app *App) ReviewResource(id uint, status string) error {
	if status == "approved" || status == "rejected" {
		err := app.DB.Model(&models.Resource{}).Where("id = ?", id).Update("status", status).Error
		if err != nil {
			return err
		}
		app.ClearCache()
	}
	return nil
}

// GetResourcesByStatus 按状态获取资源列表
func (app *App) GetResourcesByStatus(status string) ([]models.Resource, error) {
	query := app.DB.Preload("Category").Preload("Tags").Preload("User")
	if status != "" {
		query = query.Where("status = ?", status)
	}

	var resources []models.Resource
	if err := query.Order("created_at desc").Find(&resources).Error; err != nil {
		return nil, err
	}

	for i := range resources {
		if resources[i].RatingCount > 0 {
			resources[i].RatingScore = resources[i].RatingScore / float64(resources[i].RatingCount)
		}
	}
	return resources, nil
}

// CreateCategory 创建分类
func (app *App) CreateCategory(name string) (*models.Category, error) {
	category := &models.Category{Name: name}
	if err := app.DB.Create(category).Error; err != nil {
		return nil, err
	}
	app.ClearCache()
	return category, nil
}

// UpdateCategory 更新分类
func (app *App) UpdateCategory(id uint, name string) error {
	if name != "" {
		err := app.DB.Model(&models.Category{}).Where("id = ?", id).Update("name", name).Error
		if err != nil {
			return err
		}
		app.ClearCache()
	}
	return nil
}

// DeleteCategory 删除分类
func (app *App) DeleteCategory(id uint) error {
	return app.DB.Transaction(func(tx *gorm.DB) error {
		var category models.Category
		if err := tx.Select("id").First(&category, id).Error; err != nil {
			return err
		}

		if err := tx.Model(&models.Resource{}).Where("category_id = ?", id).Update("category_id", nil).Error; err != nil {
			return err
		}

		result := tx.Delete(&models.Category{}, id)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return gorm.ErrRecordNotFound
		}
		return nil
	})
}

// CreateTag 创建标签
func (app *App) CreateTag(name string) (*models.Tag, error) {
	tag := &models.Tag{Name: name}
	if err := app.DB.Create(tag).Error; err != nil {
		return nil, err
	}
	app.ClearCache()
	return tag, nil
}

// UpdateTag 更新标签
func (app *App) UpdateTag(id uint, name string) error {
	if name != "" {
		err := app.DB.Model(&models.Tag{}).Where("id = ?", id).Update("name", name).Error
		if err != nil {
			return err
		}
		app.ClearCache()
	}
	return nil
}

// DeleteTag 删除标签
func (app *App) DeleteTag(id uint) error {
	if err := app.DB.Delete(&models.Tag{}, id).Error; err != nil {
		return err
	}
	app.ClearCache()
	return nil
}

// UpdateSiteConfig 更新站点配置
func (app *App) UpdateSiteConfig(siteName, siteDesc, heroTitle, heroSubtitle, localeDefault, adsJSON string) error {
	var cfg models.SiteConfig
	app.DB.First(&cfg)

	cfg.SiteName = siteName
	cfg.SiteDesc = siteDesc
	cfg.HeroTitle = heroTitle
	cfg.HeroSubtitle = heroSubtitle
	cfg.LocaleDefault = localeDefault
	cfg.AdsJSON = adsJSON

	if err := app.DB.Save(&cfg).Error; err != nil {
		return err
	}
	app.ClearCache()
	return nil
}
