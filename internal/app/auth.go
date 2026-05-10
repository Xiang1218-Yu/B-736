package app

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"prompt736/internal/models"
	"prompt736/internal/utils"

	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// LoginPage 登录页处理器
func (app *App) LoginPage(c *gin.Context) {
	if _, exists := c.Get("user"); exists {
		c.Redirect(http.StatusFound, "/")
		return
	}

	data := app.basePageData(c)
	data.Title = app.tr(c, "auth.login_title")
	app.renderPage(c, "login.html", data)
}

// LoginHandler 登录处理
func (app *App) LoginHandler(c *gin.Context) {
	username := strings.TrimSpace(c.PostForm("username"))
	password := c.PostForm("password")

	var user models.User
	if err := app.DB.Where("username = ?", username).First(&user).Error; err != nil {
		app.setFlashKey(c, "flash_error", "flash.invalid_credentials")
		c.Redirect(http.StatusFound, "/login")
		return
	}

	if user.Status != "active" {
		app.setFlashKey(c, "flash_error", "flash.account_disabled")
		c.Redirect(http.StatusFound, "/login")
		return
	}

	if !utils.CheckPassword(user.PasswordHash, password) {
		app.setFlashKey(c, "flash_error", "flash.invalid_credentials")
		c.Redirect(http.StatusFound, "/login")
		return
	}

	session := sessions.Default(c)
	session.Set("userID", user.ID)
	session.Save()

	app.setFlashKey(c, "flash_success", "flash.login_success")
	c.Redirect(http.StatusFound, "/")
}

// RegisterPage 注册页处理器
func (app *App) RegisterPage(c *gin.Context) {
	if _, exists := c.Get("user"); exists {
		c.Redirect(http.StatusFound, "/")
		return
	}

	data := app.basePageData(c)
	data.Title = app.tr(c, "auth.register_title")
	app.renderPage(c, "register.html", data)
}

// RegisterHandler 注册处理
func (app *App) RegisterHandler(c *gin.Context) {
	username := strings.TrimSpace(c.PostForm("username"))
	password := c.PostForm("password")
	confirmPassword := c.PostForm("confirm_password")

	if len(username) < 3 || len(username) > 30 {
		app.setFlashKey(c, "flash_error", "flash.username_length")
		c.Redirect(http.StatusFound, "/register")
		return
	}

	if len(password) < 6 {
		app.setFlashKey(c, "flash_error", "flash.password_length")
		c.Redirect(http.StatusFound, "/register")
		return
	}

	if password != confirmPassword {
		app.setFlashKey(c, "flash_error", "flash.password_mismatch")
		c.Redirect(http.StatusFound, "/register")
		return
	}

	var existing models.User
	if err := app.DB.Where("username = ?", username).First(&existing).Error; err == nil {
		app.setFlashKey(c, "flash_error", "flash.username_exists")
		c.Redirect(http.StatusFound, "/register")
		return
	}

	hash, err := utils.HashPassword(password)
	if err != nil {
		app.setFlashKey(c, "flash_error", "flash.register_failed")
		c.Redirect(http.StatusFound, "/register")
		return
	}

	user := models.User{
		Username:     username,
		PasswordHash: hash,
		Role:         "user",
		Points:       50,
		Status:       "active",
	}
	if err := app.DB.Create(&user).Error; err != nil {
		app.setFlashKey(c, "flash_error", "flash.register_failed")
		c.Redirect(http.StatusFound, "/register")
		return
	}

	session := sessions.Default(c)
	session.Set("userID", user.ID)
	session.Save()

	app.Cache.Flush()
	app.setFlashKey(c, "flash_success", "flash.register_success")
	c.Redirect(http.StatusFound, "/")
}

// LogoutHandler 登出处理
func (app *App) LogoutHandler(c *gin.Context) {
	session := sessions.Default(c)
	session.Clear()
	session.Save()
	c.Redirect(http.StatusFound, "/")
}

// CheckinHandler 签到处理
func (app *App) CheckinHandler(c *gin.Context) {
	user := c.MustGet("user").(*models.User)

	now := time.Now()
	start, end := dayRange(now)
	if err := app.DB.Transaction(func(tx *gorm.DB) error {
		var lockedUser models.User
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&lockedUser, user.ID).Error; err != nil {
			return err
		}

		if lockedUser.LastCheckinAt != nil && !lockedUser.LastCheckinAt.Before(start) && lockedUser.LastCheckinAt.Before(end) {
			return ErrAlreadyCheckedIn
		}

		return tx.Model(&models.User{}).Where("id = ?", user.ID).Updates(map[string]interface{}{
			"points":          gorm.Expr("points + ?", CheckinRewardPoint),
			"last_checkin_at": now,
		}).Error
	}); err != nil {
		if errors.Is(err, ErrAlreadyCheckedIn) {
			app.setFlashKey(c, "flash_error", "flash.checkin_done")
		} else {
			app.Log.WithError(err).Error("checkin transaction failed")
			app.setFlashKey(c, "flash_error", "flash.update_failed")
		}
		c.Redirect(http.StatusFound, "/profile")
		return
	}

	app.setFlashKey(c, "flash_success", "flash.checkin_success")
	c.Redirect(http.StatusFound, "/profile")
}

// ShareHandler 分享奖励处理
func (app *App) ShareHandler(c *gin.Context) {
	user := c.MustGet("user").(*models.User)
	resourceID := parseUint(c.Param("id"))

	if resourceID == 0 {
		c.Redirect(http.StatusFound, "/resources")
		return
	}

	start, end := dayRange(time.Now())
	if err := app.DB.Transaction(func(tx *gorm.DB) error {
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
			Update("points", gorm.Expr("points + ?", ShareRewardPoint)).Error
	}); err != nil {
		if errors.Is(err, ErrShareRewardLimited) {
			app.setFlashKey(c, "flash_error", "flash.share_reward_limited")
		} else {
			app.Log.WithError(err).WithField("resource_id", resourceID).Error("share reward failed")
			app.setFlashKey(c, "flash_error", "flash.share_failed")
		}
		c.Redirect(http.StatusFound, fmt.Sprintf("/resources/%d", resourceID))
		return
	}

	app.setFlashKey(c, "flash_success", "flash.share_success")
	c.Redirect(http.StatusFound, fmt.Sprintf("/resources/%d", resourceID))
}

// ResourceCommentHandler 资源评论处理
func (app *App) ResourceCommentHandler(c *gin.Context) {
	userValue, exists := c.Get("user")
	if !exists {
		app.setFlashKey(c, "flash_error", "flash.login_required")
		c.Redirect(http.StatusFound, "/login")
		return
	}
	user, ok := userValue.(*models.User)
	if !ok || user == nil {
		app.setFlashKey(c, "flash_error", "flash.login_required")
		c.Redirect(http.StatusFound, "/login")
		return
	}

	resourceID := parseUint(c.Param("id"))
	content := strings.TrimSpace(c.PostForm("content"))
	rating := parseInt(c.PostForm("rating"), 0)

	if resourceID == 0 || content == "" || rating < 1 || rating > 5 {
		app.setFlashKey(c, "flash_error", "flash.comment_failed")
		if resourceID == 0 {
			c.Redirect(http.StatusFound, "/resources")
			return
		}
		c.Redirect(http.StatusFound, fmt.Sprintf("/resources/%d", resourceID))
		return
	}

	var resource models.Resource
	if err := app.DB.Select("id", "status").First(&resource, resourceID).Error; err != nil || resource.Status != "approved" {
		app.setFlashKey(c, "flash_error", "flash.comment_failed")
		c.Redirect(http.StatusFound, fmt.Sprintf("/resources/%d", resourceID))
		return
	}

	comment := models.Comment{
		Content:    content,
		Rating:     rating,
		UserID:     user.ID,
		ResourceID: ptrUint(resourceID),
	}

	if err := app.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&comment).Error; err != nil {
			return err
		}
		if err := tx.Model(&models.Resource{}).Where("id = ?", resourceID).Updates(map[string]interface{}{
			"rating_score": gorm.Expr("rating_score + ?", float64(rating)),
			"rating_count": gorm.Expr("rating_count + ?", 1),
		}).Error; err != nil {
			return err
		}
		return nil
	}); err != nil {
		app.Log.WithError(err).Error("create resource comment failed")
		app.setFlashKey(c, "flash_error", "flash.comment_failed")
		c.Redirect(http.StatusFound, fmt.Sprintf("/resources/%d", resourceID))
		return
	}

	app.setFlashKey(c, "flash_success", "flash.comment_success")
	app.Cache.Flush()
	c.Redirect(http.StatusFound, fmt.Sprintf("/resources/%d", resourceID))
}

// ArticleCommentHandler 文章评论处理
func (app *App) ArticleCommentHandler(c *gin.Context) {
	userValue, exists := c.Get("user")
	if !exists {
		app.setFlashKey(c, "flash_error", "flash.login_required")
		c.Redirect(http.StatusFound, "/login")
		return
	}
	user, ok := userValue.(*models.User)
	if !ok || user == nil {
		app.setFlashKey(c, "flash_error", "flash.login_required")
		c.Redirect(http.StatusFound, "/login")
		return
	}

	articleID := parseUint(c.Param("id"))
	content := strings.TrimSpace(c.PostForm("content"))

	if articleID == 0 || content == "" {
		app.setFlashKey(c, "flash_error", "flash.comment_failed")
		if articleID == 0 {
			c.Redirect(http.StatusFound, "/articles")
			return
		}
		c.Redirect(http.StatusFound, fmt.Sprintf("/articles/%d", articleID))
		return
	}

	var article models.Article
	if err := app.DB.Select("id", "status").First(&article, articleID).Error; err != nil || article.Status != "published" {
		app.setFlashKey(c, "flash_error", "flash.comment_failed")
		c.Redirect(http.StatusFound, fmt.Sprintf("/articles/%d", articleID))
		return
	}

	comment := models.Comment{
		Content:   content,
		UserID:    user.ID,
		ArticleID: ptrUint(articleID),
	}
	if err := app.DB.Create(&comment).Error; err != nil {
		app.Log.WithError(err).Error("create article comment failed")
		app.setFlashKey(c, "flash_error", "flash.comment_failed")
		c.Redirect(http.StatusFound, fmt.Sprintf("/articles/%d", articleID))
		return
	}

	app.setFlashKey(c, "flash_success", "flash.comment_success")
	c.Redirect(http.StatusFound, fmt.Sprintf("/articles/%d", articleID))
}
