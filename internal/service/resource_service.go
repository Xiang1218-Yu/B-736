package service

import (
	"encoding/csv"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"prompt736/internal/models"
	"prompt736/internal/utils"

	"github.com/PuerkitoBio/goquery"
	"gorm.io/gorm"
)

// GetResourceByID 根据ID获取资源
func (app *App) GetResourceByID(id uint) (*models.Resource, error) {
	var resource models.Resource
	if err := app.DB.Preload("Category").Preload("Tags").Preload("User").First(&resource, id).Error; err != nil {
		return nil, err
	}
	return &resource, nil
}

// IncrementResourceViews 增加资源浏览量
func (app *App) IncrementResourceViews(id uint) error {
	return app.DB.Model(&models.Resource{}).Where("id = ?", id).Update("views", gorm.Expr("views + ?", 1)).Error
}

// GetResourceComments 获取资源评论
func (app *App) GetResourceComments(resourceID uint) ([]models.Comment, error) {
	var comments []models.Comment
	if err := app.DB.Preload("User").Where("resource_id = ?", resourceID).Order("created_at desc").Find(&comments).Error; err != nil {
		return nil, err
	}
	return comments, nil
}

// PublishManualResource 手动发布资源
func (app *App) PublishManualResource(userID uint, title, description, resourceType, link string, categoryID uint, tagIDs []uint, fileName string) (*models.Resource, error) {
	status := "pending"
	var user models.User
	if err := app.DB.First(&user, userID).Error; err == nil {
		if strings.EqualFold(user.Role, "admin") {
			status = "approved"
		}
	}

	var categoryPtr *uint
	if categoryID > 0 {
		categoryPtr = &categoryID
	}

	resource := models.Resource{
		Title:        title,
		Description:  description,
		ResourceType: resourceType,
		Link:         link,
		FilePath:     fileName,
		SourceType:   "manual",
		Status:       status,
		UserID:       userID,
		CategoryID:   categoryPtr,
	}

	if len(tagIDs) > 0 {
		var tags []models.Tag
		if err := app.DB.Find(&tags, tagIDs).Error; err == nil && len(tags) > 0 {
			resource.Tags = tags
		}
	}

	if err := app.DB.Create(&resource).Error; err != nil {
		return nil, err
	}

	app.ClearCache()
	return &resource, nil
}

// SaveUploadedFile 保存上传的文件
func (app *App) SaveUploadedFile(c interface {
	FormFile(string) (interface{}, error)
	SaveUploadedFile(interface{}, string) error
}, userID uint, fieldName string) (string, error) {
	file, err := c.FormFile(fieldName)
	if err != nil {
		return "", nil
	}

	fh, ok := file.(interface{ Size() int64 })
	if !ok {
		return "", fmt.Errorf("invalid file")
	}
	if fh.Size() > 100*1024*1024 {
		return "", fmt.Errorf("file too large")
	}

	fn, ok := file.(interface{ Filename() string })
	if !ok {
		return "", fmt.Errorf("invalid file")
	}

	ext := strings.ToLower(filepath.Ext(fn.Filename()))
	if ext == "" {
		ext = ".bin"
	}
	fileName := fmt.Sprintf("%d_%d%s", userID, time.Now().UnixNano(), ext)

	if err := os.MkdirAll(app.Cfg.UploadDir, 0755); err != nil {
		return "", err
	}

	target := filepath.Join(app.Cfg.UploadDir, fileName)
	if err := c.SaveUploadedFile(file, target); err != nil {
		return "", err
	}

	return fileName, nil
}

// ImportResourcesFromCSV 从CSV导入资源
func (app *App) ImportResourcesFromCSV(userID uint, src io.Reader, defaultType string, categoryID uint, tagIDs []uint) (int, int, error) {
	defaultType = utils.CanonicalResourceType(defaultType)
	status := "pending"
	var user models.User
	if err := app.DB.First(&user, userID).Error; err == nil {
		if strings.EqualFold(user.Role, "admin") {
			status = "approved"
		}
	}

	var categoryPtr *uint
	if categoryID > 0 {
		categoryPtr = &categoryID
	}

	var tags []models.Tag
	if len(tagIDs) > 0 {
		app.DB.Find(&tags, tagIDs)
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

		if line == 1 && utils.LooksLikeCSVHeader(record) {
			continue
		}

		title := utils.CsvCell(record, 0)
		description := utils.CsvCell(record, 1)
		resourceTypeRaw := utils.CsvCell(record, 2)
		resourceType := defaultType
		if resourceTypeRaw != "" {
			resourceType = utils.CanonicalResourceType(resourceTypeRaw)
		}
		link := utils.CsvCell(record, 3)

		if len([]rune(title)) < 2 || len([]rune(description)) < 5 {
			skipped++
			continue
		}
		if link != "" && !utils.IsValidHTTPURL(link) {
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
			UserID:       userID,
			CategoryID:   categoryPtr,
		}

		if err := app.DB.Create(&resource).Error; err != nil {
			skipped++
			continue
		}
		if len(tags) > 0 {
			_ = app.DB.Model(&resource).Association("Tags").Append(tags)
		}
		created++
	}

	app.ClearCache()
	return created, skipped, nil
}

// CrawlResource 抓取网页内容并创建资源
func (app *App) CrawlResource(userID uint, crawlURL, resourceType string, categoryID uint, tagIDs []uint) (*models.Resource, error) {
	client := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequest(http.MethodGet, crawlURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; GalaxyHubCrawler/1.0)")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 400 {
		return nil, fmt.Errorf("bad status: %d", resp.StatusCode)
	}

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return nil, err
	}

	title := strings.TrimSpace(doc.Find("meta[property='og:title']").AttrOr("content", ""))
	if title == "" {
		title = strings.TrimSpace(doc.Find("title").First().Text())
	}
	if title == "" {
		parsed, parseErr := url.Parse(crawlURL)
		if parseErr == nil && parsed.Host != "" {
			title = parsed.Host
		} else {
			title = "Crawled Resource"
		}
	}

	description := strings.TrimSpace(doc.Find("meta[name='description']").AttrOr("content", ""))
	if description == "" {
		description = strings.TrimSpace(doc.Find("meta[property='og:description']").AttrOr("content", ""))
	}
	if description == "" {
		description = fmt.Sprintf("Crawled from: %s", crawlURL)
	}

	status := "pending"
	var user models.User
	if err := app.DB.First(&user, userID).Error; err == nil {
		if strings.EqualFold(user.Role, "admin") {
			status = "approved"
		}
	}

	var categoryPtr *uint
	if categoryID > 0 {
		categoryPtr = &categoryID
	}

	resource := models.Resource{
		Title:        title,
		Description:  description,
		ResourceType: utils.CanonicalResourceType(resourceType),
		Link:         crawlURL,
		SourceType:   "crawler",
		Status:       status,
		UserID:       userID,
		CategoryID:   categoryPtr,
	}

	if len(tagIDs) > 0 {
		var tags []models.Tag
		if err := app.DB.Find(&tags, tagIDs).Error; err == nil && len(tags) > 0 {
			resource.Tags = tags
		}
	}

	if err := app.DB.Create(&resource).Error; err != nil {
		return nil, err
	}

	app.ClearCache()
	return &resource, nil
}

// AddResourceComment 添加资源评论
func (app *App) AddResourceComment(userID, resourceID uint, content string, rating int) error {
	var resource models.Resource
	if err := app.DB.Select("id", "status").First(&resource, resourceID).Error; err != nil || resource.Status != "approved" {
		return fmt.Errorf("resource not found")
	}

	comment := models.Comment{
		Content:    content,
		Rating:     rating,
		UserID:     userID,
		ResourceID: utils.PtrUint(resourceID),
	}

	return app.DB.Transaction(func(tx *gorm.DB) error {
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
	})
}

// ShareReward 分享奖励
func (app *App) ShareReward(userID, resourceID uint) error {
	start, end := utils.DayRange(time.Now())

	return app.DB.Transaction(func(tx *gorm.DB) error {
		var lockedUser models.User
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Select("id").First(&lockedUser, userID).Error; err != nil {
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
			Where("user_id = ? AND resource_id = ?", userID, resourceID).
			Count(&claimedCount).Error; err != nil {
			return err
		}
		if claimedCount > 0 {
			return utils.ErrShareRewardLimited
		}

		if err := tx.Model(&models.ShareLog{}).
			Where("user_id = ? AND created_at >= ? AND created_at < ?", userID, start, end).
			Count(&claimedCount).Error; err != nil {
			return err
		}
		if claimedCount > 0 {
			return utils.ErrShareRewardLimited
		}

		if err := tx.Create(&models.ShareLog{UserID: userID, ResourceID: resourceID}).Error; err != nil {
			return err
		}

		return tx.Model(&models.User{}).Where("id = ?", userID).
			Update("points", gorm.Expr("points + ?", utils.ShareRewardPoint)).Error
	})
}
