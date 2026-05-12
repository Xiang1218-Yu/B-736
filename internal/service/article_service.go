package service

import (
	"prompt736/internal/models"
	"prompt736/internal/utils"
)

// GetArticleByID 根据ID获取文章
func (app *App) GetArticleByID(id uint) (*models.Article, error) {
	var article models.Article
	if err := app.DB.Preload("User").First(&article, id).Error; err != nil {
		return nil, err
	}
	return &article, nil
}

// IncrementArticleViews 增加文章浏览量
func (app *App) IncrementArticleViews(id uint) error {
	return app.DB.Model(&models.Article{}).Where("id = ?", id).Update("views", gorm.Expr("views + ?", 1)).Error
}

// GetArticleComments 获取文章评论
func (app *App) GetArticleComments(articleID uint) ([]models.Comment, error) {
	var comments []models.Comment
	if err := app.DB.Preload("User").Where("article_id = ?", articleID).Order("created_at desc").Find(&comments).Error; err != nil {
		return nil, err
	}
	return comments, nil
}

// AddArticleComment 添加文章评论
func (app *App) AddArticleComment(userID, articleID uint, content string) error {
	var article models.Article
	if err := app.DB.Select("id", "status").First(&article, articleID).Error; err != nil || article.Status != "published" {
		return err
	}

	comment := models.Comment{
		Content:   content,
		UserID:    userID,
		ArticleID: utils.PtrUint(articleID),
	}
	if err := app.DB.Create(&comment).Error; err != nil {
		return err
	}
	return nil
}

// CreateArticle 创建文章
func (app *App) CreateArticle(userID uint, title, summary, content string) (*models.Article, error) {
	article := models.Article{
		Title:   title,
		Summary: summary,
		Content: utils.SanitizeArticleContent(content),
		Status:  "published",
		UserID:  userID,
	}
	if err := app.DB.Create(&article).Error; err != nil {
		return nil, err
	}
	app.ClearCache()
	return &article, nil
}

// UpdateArticle 更新文章
func (app *App) UpdateArticle(id uint, title, summary, content string) error {
	err := app.DB.Model(&models.Article{}).Where("id = ?", id).Updates(map[string]interface{}{
		"title":   title,
		"summary": summary,
		"content": utils.SanitizeArticleContent(content),
	}).Error
	if err != nil {
		return err
	}
	app.ClearCache()
	return nil
}

// GetAllArticles 获取所有文章（管理后台用）
func (app *App) GetAllArticles() ([]models.Article, error) {
	var articles []models.Article
	if err := app.DB.Preload("User").Order("created_at desc").Find(&articles).Error; err != nil {
		return nil, err
	}
	return articles, nil
}
