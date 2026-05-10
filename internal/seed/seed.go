package seed

import (
	"time"

	"prompt736/internal/models"
	"prompt736/internal/utils"

	"gorm.io/gorm"
)

func Ensure(db *gorm.DB) error {
	var count int64
	if err := db.Model(&models.User{}).Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return nil
	}

	adminPass, _ := utils.HashPassword("123456")
	admin := models.User{
		Username:     "admin",
		PasswordHash: adminPass,
		Role:         "admin",
		Points:       999,
		Status:       "active",
	}
	editorPass, _ := utils.HashPassword("123456")
	editor := models.User{
		Username:     "editor",
		PasswordHash: editorPass,
		Role:         "editor",
		Points:       200,
		Status:       "active",
	}
	userPass, _ := utils.HashPassword("123456")
	user := models.User{
		Username:     "demo",
		PasswordHash: userPass,
		Role:         "user",
		Points:       80,
		Status:       "active",
	}
	if err := db.Create(&admin).Error; err != nil {
		return err
	}
	if err := db.Create(&editor).Error; err != nil {
		return err
	}
	if err := db.Create(&user).Error; err != nil {
		return err
	}

	categories := []models.Category{
		{Name: "软件工具"},
		{Name: "电子资料"},
		{Name: "视频课程"},
		{Name: "音频资源"},
		{Name: "设计素材"},
	}
	if err := db.Create(&categories).Error; err != nil {
		return err
	}

	tags := []models.Tag{
		{Name: "效率"},
		{Name: "学习"},
		{Name: "开源"},
		{Name: "AI"},
		{Name: "前端"},
		{Name: "实战"},
	}
	if err := db.Create(&tags).Error; err != nil {
		return err
	}

	resources := []models.Resource{
		{
			Title:        "Prompt 模板管理器",
			Description:  "可视化管理提示词模板，支持版本对比与团队协作，适合内容运营和研发联动。",
			ResourceType: "software",
			Link:         "https://example.com/prompt-kit",
			SourceType:   "manual",
			Status:       "approved",
			UserID:       admin.ID,
			CategoryID:   &categories[0].ID,
			Tags:         []models.Tag{tags[0], tags[2], tags[3]},
			Views:        164,
			RatingScore:  24,
			RatingCount:  6,
		},
		{
			Title:        "Go 服务性能调优清单 PDF",
			Description:  "覆盖 pprof、数据库连接池、缓存击穿防护和请求链路压测的实操清单。",
			ResourceType: "document",
			Link:         "https://example.com/go-perf",
			SourceType:   "manual",
			Status:       "approved",
			UserID:       editor.ID,
			CategoryID:   &categories[1].ID,
			Tags:         []models.Tag{tags[1], tags[5]},
			Views:        98,
			RatingScore:  19,
			RatingCount:  4,
		},
		{
			Title:        "系统设计面试实战视频课",
			Description:  "从需求拆分到容量预估，提供完整案例推演，适合准备中高级技术面试。",
			ResourceType: "video",
			Link:         "https://example.com/system-design",
			SourceType:   "manual",
			Status:       "approved",
			UserID:       admin.ID,
			CategoryID:   &categories[2].ID,
			Tags:         []models.Tag{tags[1], tags[5]},
			Views:        143,
			RatingScore:  26,
			RatingCount:  6,
		},
		{
			Title:        "产品需求拆解音频合集",
			Description:  "10 节短音频，聚焦需求澄清、优先级决策和跨团队沟通。",
			ResourceType: "audio",
			Link:         "https://example.com/product-audio",
			SourceType:   "manual",
			Status:       "approved",
			UserID:       editor.ID,
			CategoryID:   &categories[3].ID,
			Tags:         []models.Tag{tags[0], tags[1]},
			Views:        61,
			RatingScore:  13,
			RatingCount:  3,
		},
		{
			Title:        "品牌视觉组件素材包",
			Description:  "包含登录页、数据看板与活动页组件，适配 Figma 和 Sketch。",
			ResourceType: "other",
			Link:         "https://example.com/brand-kit",
			SourceType:   "manual",
			Status:       "approved",
			UserID:       admin.ID,
			CategoryID:   &categories[4].ID,
			Tags:         []models.Tag{tags[4]},
			Views:        47,
			RatingScore:  9,
			RatingCount:  2,
		},
		{
			Title:        "本周待审核资源示例",
			Description:  "用于演示后台审核流程的资源数据。",
			ResourceType: "software",
			Link:         "https://example.com/pending-resource",
			SourceType:   "manual",
			Status:       "pending",
			UserID:       user.ID,
			CategoryID:   &categories[0].ID,
			Tags:         []models.Tag{tags[2]},
			Views:        12,
		},
	}
	if err := db.Create(&resources).Error; err != nil {
		return err
	}

	articles := []models.Article{
		{
			Title:   "如何高效整理资源库",
			Summary: "建立标签体系、分类树与评分机制，提升资源发现效率。",
			Content: "<p>本文分享资源整理的三大步骤：分类、标签、评分。</p><p>配合搜索与统计模块可以更好地管理资源。</p>",
			Status:  "published",
			UserID:  admin.ID,
		},
		{
			Title:   "资源分享社区的积分设计",
			Summary: "合理的积分与等级制度让内容生态更加活跃。",
			Content: "<p>积分系统需要明确的获取渠道与消耗路径。</p>",
			Status:  "published",
			UserID:  editor.ID,
		},
		{
			Title:   "用评论数据反向优化资源质量",
			Summary: "如何从评论内容与评分分布中提炼可执行的优化建议。",
			Content: "<p>通过聚类评论关键词，可以快速识别资源内容中的高频痛点。</p><p>将评分趋势与更新记录关联，能更好衡量改进效果。</p>",
			Status:  "published",
			UserID:  admin.ID,
		},
		{
			Title:   "多语言站点的最小可用实践",
			Summary: "从模板文本抽取、语言包管理到切换交互的一套最小实现。",
			Content: "<p>优先抽取导航与核心流程文案，再逐步覆盖细节页面，是改造多语言成本最低的路径。</p>",
			Status:  "published",
			UserID:  editor.ID,
		},
	}
	if err := db.Create(&articles).Error; err != nil {
		return err
	}

	comments := []models.Comment{
		{
			Content:    "资源质量很高，信息整理清晰。",
			Rating:     5,
			UserID:     user.ID,
			ResourceID: &resources[0].ID,
			CreatedAt:  time.Now(),
		},
		{
			Content:    "文档结构很清楚，拿来就能用。",
			Rating:     4,
			UserID:     admin.ID,
			ResourceID: &resources[1].ID,
			CreatedAt:  time.Now(),
		},
		{
			Content:    "视频案例讲解节奏很好，适合复习。",
			Rating:     5,
			UserID:     user.ID,
			ResourceID: &resources[2].ID,
			CreatedAt:  time.Now(),
		},
		{
			Content:   "文章内容很实用。",
			Rating:    0,
			UserID:    user.ID,
			ArticleID: &articles[0].ID,
			CreatedAt: time.Now(),
		},
		{
			Content:   "希望后续补充更多真实项目模板。",
			Rating:    0,
			UserID:    admin.ID,
			ArticleID: &articles[3].ID,
			CreatedAt: time.Now(),
		},
	}
	if err := db.Create(&comments).Error; err != nil {
		return err
	}

	config := models.SiteConfig{
		SiteName:      "星河资源共享站",
		SiteDesc:      "汇聚高质量软件、文档、视频与音频资源，支持评论评分与多语言切换。",
		HeroTitle:     "大气资源库 · 一站式分享",
		HeroSubtitle:  "精选资源、深度文章、真实评分，让每一次分享更有价值。",
		LocaleDefault: "zh",
		AdsJSON:       "",
	}
	if err := db.Create(&config).Error; err != nil {
		return err
	}

	return nil
}
