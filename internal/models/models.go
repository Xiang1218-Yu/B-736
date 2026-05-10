package models

import "time"

type User struct {
	ID            uint       `gorm:"primaryKey" json:"id"`
	Username      string     `gorm:"size:64;uniqueIndex" json:"username"`
	PasswordHash  string     `gorm:"size:255" json:"-"`
	Role          string     `gorm:"size:20" json:"role"`
	Points        int        `gorm:"default:0" json:"points"`
	Status        string     `gorm:"size:20" json:"status"`
	LastCheckinAt *time.Time `json:"last_checkin_at,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
}

type Category struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	Name      string    `gorm:"size:100;uniqueIndex" json:"name"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type Tag struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	Name      string    `gorm:"size:100;uniqueIndex" json:"name"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type Resource struct {
	ID           uint      `gorm:"primaryKey" json:"id"`
	Title        string    `gorm:"size:200" json:"title"`
	Description  string    `gorm:"type:text" json:"description"`
	ResourceType string    `gorm:"size:50" json:"resource_type"`
	Link         string    `gorm:"size:500" json:"link"`
	FilePath     string    `gorm:"size:500" json:"file_path"`
	SourceType   string    `gorm:"size:50" json:"source_type"`
	Status       string    `gorm:"size:20" json:"status"`
	RatingScore  float64   `gorm:"default:0" json:"rating_score"`
	RatingCount  int       `gorm:"default:0" json:"rating_count"`
	Views        int       `gorm:"default:0" json:"views"`
	UserID       uint      `json:"user_id"`
	User         User      `json:"user"`
	CategoryID   *uint     `json:"category_id"`
	Category     Category  `json:"category"`
	Tags         []Tag     `gorm:"many2many:resource_tags;" json:"tags"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type Article struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	Title     string    `gorm:"size:200" json:"title"`
	Summary   string    `gorm:"size:500" json:"summary"`
	Content   string    `gorm:"type:text" json:"content"`
	Status    string    `gorm:"size:20" json:"status"`
	Views     int       `gorm:"default:0" json:"views"`
	UserID    uint      `json:"user_id"`
	User      User      `json:"user"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type Comment struct {
	ID         uint      `gorm:"primaryKey" json:"id"`
	Content    string    `gorm:"type:text" json:"content"`
	Rating     int       `gorm:"default:0" json:"rating"`
	UserID     uint      `json:"user_id"`
	User       User      `json:"user"`
	ResourceID *uint     `json:"resource_id"`
	ArticleID  *uint     `json:"article_id"`
	CreatedAt  time.Time `json:"created_at"`
}

type SiteConfig struct {
	ID            uint      `gorm:"primaryKey" json:"id"`
	SiteName      string    `gorm:"size:200" json:"site_name"`
	SiteDesc      string    `gorm:"size:500" json:"site_desc"`
	HeroTitle     string    `gorm:"size:200" json:"hero_title"`
	HeroSubtitle  string    `gorm:"size:500" json:"hero_subtitle"`
	LocaleDefault string    `gorm:"size:20" json:"locale_default"`
	AdsJSON       string    `gorm:"type:text" json:"ads_json"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

type ShareLog struct {
	ID         uint      `gorm:"primaryKey" json:"id"`
	UserID     uint      `gorm:"index:idx_share_logs_user_resource,priority:1;index:idx_share_logs_user_created_at,priority:1" json:"user_id"`
	ResourceID uint      `gorm:"index:idx_share_logs_user_resource,priority:2" json:"resource_id"`
	CreatedAt  time.Time `gorm:"index:idx_share_logs_user_created_at,priority:2" json:"created_at"`
}

type ResourceTag struct {
	ResourceID uint `gorm:"primaryKey"`
	TagID      uint `gorm:"primaryKey"`
}
