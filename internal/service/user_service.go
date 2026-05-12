package service

import (
	"errors"
	"time"

	"prompt736/internal/models"
	"prompt736/internal/utils"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// GetUserByUsername 根据用户名获取用户
func (app *App) GetUserByUsername(username string) (*models.User, error) {
	var user models.User
	if err := app.DB.Where("username = ?", username).First(&user).Error; err != nil {
		return nil, err
	}
	return &user, nil
}

// GetUserByID 根据ID获取用户
func (app *App) GetUserByID(id uint) (*models.User, error) {
	var user models.User
	if err := app.DB.First(&user, id).Error; err != nil {
		return nil, err
	}
	return &user, nil
}

// CreateUser 创建用户
func (app *App) CreateUser(username, passwordHash string) (*models.User, error) {
	user := models.User{
		Username:     username,
		PasswordHash: passwordHash,
		Role:         "user",
		Points:       50,
		Status:       "active",
	}
	if err := app.DB.Create(&user).Error; err != nil {
		return nil, err
	}
	app.ClearCache()
	return &user, nil
}

// Checkin 签到
func (app *App) Checkin(userID uint) error {
	now := time.Now()
	start, end := utils.DayRange(now)

	return app.DB.Transaction(func(tx *gorm.DB) error {
		var lockedUser models.User
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&lockedUser, userID).Error; err != nil {
			return err
		}

		if lockedUser.LastCheckinAt != nil && !lockedUser.LastCheckinAt.Before(start) && lockedUser.LastCheckinAt.Before(end) {
			return utils.ErrAlreadyCheckedIn
		}

		return tx.Model(&models.User{}).Where("id = ?", userID).Updates(map[string]interface{}{
			"points":          gorm.Expr("points + ?", utils.CheckinRewardPoint),
			"last_checkin_at": now,
		}).Error
	})
}

// GetAllUsers 获取所有用户（管理后台用）
func (app *App) GetAllUsers() ([]models.User, error) {
	var users []models.User
	if err := app.DB.Order("created_at desc").Find(&users).Error; err != nil {
		return nil, err
	}
	return users, nil
}

// UpdateUser 更新用户
func (app *App) UpdateUser(id uint, role, status string) error {
	var target models.User
	if err := app.DB.First(&target, id).Error; err != nil {
		return err
	}

	updates := map[string]interface{}{}
	if role != "" {
		updates["role"] = role
	}
	if status != "" {
		if errors.Is(app.DB.Where("username = ? AND status = ?", "admin", "suspended").First(&models.User{}).Error, gorm.ErrRecordNotFound) {
			if strings.EqualFold(target.Username, "admin") && status == "suspended" {
				return errors.New("cannot disable admin")
			}
		}
		updates["status"] = status
	}

	if len(updates) > 0 {
		if err := app.DB.Model(&models.User{}).Where("id = ?", id).Updates(updates).Error; err != nil {
			return err
		}
	}
	return nil
}
