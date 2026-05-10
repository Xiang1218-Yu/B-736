package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"net/url"
	"strconv"
	"strings"

	"prompt736/internal/models"
)

var (
	// 业务错误定义
	ErrShareRewardLimited = errors.New("share reward already claimed")
	ErrAlreadyCheckedIn   = errors.New("already checked in today")
)

// parseInt 解析整数，解析失败返回默认值
func parseInt(s string, defaultVal int) int {
	if v, err := strconv.Atoi(s); err == nil && v > 0 {
		return v
	}
	return defaultVal
}

// parseUint 解析无符号整数，解析失败返回0
func parseUint(s string) uint {
	if v, err := strconv.ParseUint(s, 10, 64); err == nil {
		return uint(v)
	}
	return 0
}

// ptrUint 返回 uint 的指针
func ptrUint(v uint) *uint {
	return &v
}

// parseUintList 解析 uint 列表，去重
func parseUintList(values []string) []uint {
	result := make([]uint, 0, len(values))
	seen := make(map[uint]struct{})
	for _, value := range values {
		parsed := parseUint(value)
		if parsed == 0 {
			continue
		}
		if _, exists := seen[parsed]; exists {
			continue
		}
		seen[parsed] = struct{}{}
		result = append(result, parsed)
	}
	return result
}

// csvCell 获取 CSV 记录中的指定单元格
func csvCell(record []string, index int) string {
	if index < 0 || index >= len(record) {
		return ""
	}
	return strings.TrimSpace(record[index])
}

// looksLikeCSVHeader 判断是否为 CSV 表头
func looksLikeCSVHeader(record []string) bool {
	if len(record) == 0 {
		return false
	}
	first := strings.ToLower(strings.TrimSpace(record[0]))
	return first == "title" || first == "标题"
}

// isValidHTTPURL 验证是否为有效的 HTTP/HTTPS URL
func isValidHTTPURL(raw string) bool {
	parsed, err := url.Parse(raw)
	if err != nil {
		return false
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return false
	}
	return strings.TrimSpace(parsed.Host) != ""
}

// CanonicalResourceType 规范化资源类型
func CanonicalResourceType(resourceType string) string {
	normalized := strings.ToLower(strings.TrimSpace(resourceType))
	switch normalized {
	case "software", "软件", "软件工具", "tool":
		return "software"
	case "document", "文档", "资料", "电子书", "电子资料":
		return "document"
	case "video", "视频", "电影", "电影资源":
		return "video"
	case "audio", "音频":
		return "audio"
	case "other", "其他":
		return "other"
	default:
		return "other"
	}
}

// CanonicalSourceType 规范化来源类型
func CanonicalSourceType(sourceType string) string {
	normalized := strings.ToLower(strings.TrimSpace(sourceType))
	switch normalized {
	case "manual", "手动", "手动录入":
		return "manual"
	case "crawler", "crawl", "爬虫", "爬虫抓取":
		return "crawler"
	case "import", "bulk", "csv", "导入", "批量导入":
		return "import"
	default:
		return "manual"
	}
}

// BuildLangSwitchURL 构建语言切换URL
func BuildLangSwitchURL(path, rawQuery, lang string) string {
	values, err := url.ParseQuery(rawQuery)
	if err != nil {
		values = make(url.Values)
	}
	values.Set("lang", lang)
	encoded := values.Encode()
	if encoded == "" {
		return path
	}
	return path + "?" + encoded
}

// buildRatingStars 构建评分星星显示
func buildRatingStars(score float64) string {
	stars := int(score + 0.5)
	if stars < 0 {
		stars = 0
	}
	if stars > 5 {
		stars = 5
	}
	return strings.Repeat("★", stars) + strings.Repeat("☆", 5-stars)
}

// toIntOrZero 将任意类型转换为 int，失败返回0
func toIntOrZero(v interface{}) int {
	switch n := v.(type) {
	case int:
		return n
	case int8:
		return int(n)
	case int16:
		return int(n)
	case int32:
		return int(n)
	case int64:
		return int(n)
	case uint:
		return int(n)
	case uint8:
		return int(n)
	case uint16:
		return int(n)
	case uint32:
		return int(n)
	case uint64:
		return int(n)
	case float32:
		return int(n)
	case float64:
		return int(n)
	case string:
		if parsed, err := strconv.Atoi(strings.TrimSpace(n)); err == nil {
			return parsed
		}
	}
	return 0
}

// getTemplateFuncs 获取模板函数
func getTemplateFuncs() template.FuncMap {
	return template.FuncMap{
		"truncate": func(a interface{}, b interface{}) string {
			var s string
			var n int

			switch v := a.(type) {
			case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
				n = toIntOrZero(v)
			case string:
				s = v
			}

			switch v := b.(type) {
			case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
				n = toIntOrZero(v)
			case string:
				s = v
			}

			if n <= 0 || len(s) <= n {
				return s
			}
			runes := []rune(s)
			if len(runes) <= n {
				return s
			}
			return string(runes[:n]) + "..."
		},
		"formatDate": func(v interface{}) string {
			switch t := v.(type) {
			case string:
				return t
			default:
				return ""
			}
		},
		"safeHTML": func(s string) template.HTML {
			return template.HTML(s)
		},
		"csrfField": func() template.HTML {
			return ""
		},
		"add": func(a, b interface{}) int {
			return toIntOrZero(a) + toIntOrZero(b)
		},
		"sub": func(a, b interface{}) int {
			return toIntOrZero(a) - toIntOrZero(b)
		},
		"iterate": func(n int) []int {
			result := make([]int, n)
			for i := range result {
				result[i] = i
			}
			return result
		},
		"upper": strings.ToUpper,
		"lower": strings.ToLower,
		"slice": func(s string, start, end int) string {
			if start > len(s) {
				return ""
			}
			if end > len(s) {
				end = len(s)
			}
			return s[start:end]
		},
		"eq": func(a, b interface{}) bool {
			return fmt.Sprintf("%v", a) == fmt.Sprintf("%v", b)
		},
		"gt": func(a, b interface{}) bool {
			return toIntOrZero(a) > toIntOrZero(b)
		},
		"lt": func(a, b interface{}) bool {
			return toIntOrZero(a) < toIntOrZero(b)
		},
		"T": func(key string, args ...interface{}) string {
			if len(args) > 0 {
				return fmt.Sprintf(key, args...)
			}
			return key
		},
		"langURL": func(lang string) string {
			return "/?lang=" + lang
		},
		"langActive": func(lang string) bool {
			return lang == "zh"
		},
		"resourceTypeClass": canonicalResourceType,
		"resourceTypeLabel": func(resourceType string) string {
			return resourceType
		},
		"sourceTypeLabel": func(sourceType string) string {
			return sourceType
		},
		"roleLabel": func(role string) string {
			return role
		},
		"statusLabel": func(status string) string {
			return status
		},
		"siteText": func(_ string, fallback string) string {
			return fallback
		},
		"ratingStars": buildRatingStars,
	}
}

// UnmarshalAdConfig 解析广告配置 JSON
func UnmarshalAdConfig(adsJSON string) (*adConfig, error) {
	if strings.TrimSpace(adsJSON) == "" {
		return nil, nil
	}
	var cfg adConfig
	if err := json.Unmarshal([]byte(adsJSON), &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// IsLegacySeedAdConfig 判断是否为旧版本的种子广告配置
func IsLegacySeedAdConfig(cfg adConfig) bool {
	if len(cfg.Slots) != 1 {
		return false
	}
	slot := cfg.Slots[0]
	if strings.TrimSpace(slot.Code) != "hero-banner" {
		return false
	}
	title := strings.TrimSpace(slot.Title)
	desc := strings.TrimSpace(slot.Desc)
	if title != "品牌合作专区" {
		return false
	}
	if desc != "预留标准广告位，支持后续对接" && desc != "预留标准广告位，支持后续广告系统对接。" {
		return false
	}
	return strings.TrimSpace(slot.Link) == "" &&
		strings.TrimSpace(slot.ImageURL) == "" &&
		strings.TrimSpace(slot.CTA) == "" &&
		strings.TrimSpace(slot.Contact) == ""
}

// canPublishResource 判断用户是否可以发布资源
func canPublishResource(user *models.User) bool {
	if user == nil {
		return false
	}
	if strings.ToLower(strings.TrimSpace(user.Status)) != "active" {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(user.Role)) {
	case "admin", "editor":
		return true
	case "user":
		return user.Points >= PublishMinPoints
	default:
		return false
	}
}
