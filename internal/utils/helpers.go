package utils

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"html/template"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"prompt736/internal/models"

	"github.com/microcosm-cc/bluemonday"
)

var (
	ErrShareRewardLimited = errors.New("share reward already claimed")
	ErrAlreadyCheckedIn   = errors.New("already checked in today")
	articleHTMLPolicy     = newArticleHTMLPolicy()
)

// newArticleHTMLPolicy 创建文章内容清洗策略
func newArticleHTMLPolicy() *bluemonday.Policy {
	policy := bluemonday.UGCPolicy()
	policy.AllowAttrs("class").Globally()
	policy.AllowAttrs("target").OnElements("a")
	policy.AllowAttrs("rel").OnElements("a")
	policy.AllowAttrs("src", "alt", "title", "width", "height").OnElements("img")
	policy.AllowAttrs("loading").OnElements("img")
	policy.AllowURLSchemes("http", "https", "mailto")
	return policy
}

// SanitizeArticleContent 清洗文章内容
func SanitizeArticleContent(raw string) string {
	cleaned := articleHTMLPolicy.Sanitize(strings.TrimSpace(raw))
	return strings.TrimSpace(cleaned)
}

// SafeArticleHTML 将内容转换为安全的HTML
func SafeArticleHTML(raw string) template.HTML {
	return template.HTML(SanitizeArticleContent(raw))
}

// SecureStringEqual 安全的字符串比较（防止时序攻击）
func SecureStringEqual(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

// IsSafeHTTPMethod 判断是否为安全的HTTP方法
func IsSafeHTTPMethod(method string) bool {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodOptions, http.MethodTrace:
		return true
	default:
		return false
	}
}

// GenerateSecureToken 生成安全的随机令牌
func GenerateSecureToken(size int) string {
	if size <= 0 {
		size = 32
	}

	buf := make([]byte, size)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Sprintf("fallback-%d", time.Now().UnixNano())
	}

	return base64.RawURLEncoding.EncodeToString(buf)
}

// CanPublishResource 判断用户是否有权限发布资源
func CanPublishResource(user *models.User, minPoints int) bool {
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
		return user.Points >= minPoints
	default:
		return false
	}
}

// DayRange 获取一天的时间范围（开始和结束）
func DayRange(now time.Time) (time.Time, time.Time) {
	year, month, day := now.Date()
	location := now.Location()
	start := time.Date(year, month, day, 0, 0, 0, 0, location)
	return start, start.Add(24 * time.Hour)
}

// ParseInt 解析整数，失败返回默认值
func ParseInt(s string, defaultVal int) int {
	if v, err := strconv.Atoi(s); err == nil && v > 0 {
		return v
	}
	return defaultVal
}

// ParseUint 解析无符号整数，失败返回0
func ParseUint(s string) uint {
	if v, err := strconv.ParseUint(s, 10, 64); err == nil {
		return uint(v)
	}
	return 0
}

// PtrUint 返回uint指针
func PtrUint(v uint) *uint {
	return &v
}

// ParseUintList 解析无符号整数列表
func ParseUintList(values []string) []uint {
	result := make([]uint, 0, len(values))
	seen := make(map[uint]struct{})
	for _, value := range values {
		parsed := ParseUint(value)
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

// CsvCell 获取CSV单元格内容
func CsvCell(record []string, index int) string {
	if index < 0 || index >= len(record) {
		return ""
	}
	return strings.TrimSpace(record[index])
}

// LooksLikeCSVHeader 判断是否为CSV表头
func LooksLikeCSVHeader(record []string) bool {
	if len(record) == 0 {
		return false
	}
	first := strings.ToLower(strings.TrimSpace(record[0]))
	return first == "title" || first == "标题"
}

// IsValidHTTPURL 验证是否为有效的HTTP/HTTPS URL
func IsValidHTTPURL(raw string) bool {
	parsed, err := url.Parse(raw)
	if err != nil {
		return false
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return false
	}
	return strings.TrimSpace(parsed.Host) != ""
}

// ToIntOrZero 将任意类型转换为整数，失败返回0
func ToIntOrZero(v interface{}) int {
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

// BuildRatingStars 构建评分星级显示
func BuildRatingStars(score float64) string {
	stars := int(score + 0.5)
	if stars < 0 {
		stars = 0
	}
	if stars > 5 {
		stars = 5
	}
	return strings.Repeat("★", stars) + strings.Repeat("☆", 5-stars)
}

// RedirectBackOrDefault 重定向回来源页面或默认页面
func RedirectBackOrDefault(c interface{ Request() *http.Request }, fallback string) (string, error) {
	req := c.Request()
	ref := strings.TrimSpace(req.Referer())
	if ref == "" {
		return fallback, nil
	}

	parsed, err := url.Parse(ref)
	if err != nil {
		return fallback, nil
	}

	if parsed.Host != "" && !strings.EqualFold(parsed.Host, req.Host) {
		return fallback, nil
	}

	target := parsed.Path
	if target == "" || !strings.HasPrefix(target, "/") {
		target = fallback
	}
	if parsed.RawQuery != "" {
		target += "?" + parsed.RawQuery
	}

	return target, nil
}

// SplitSQLStatements 分割SQL语句
func SplitSQLStatements(sqlText string) []string {
	statements := make([]string, 0)
	var current strings.Builder

	inSingleQuote := false
	inDoubleQuote := false
	inBacktick := false
	inLineComment := false
	inBlockComment := false
	escaped := false

	runes := []rune(sqlText)
	for i := 0; i < len(runes); i++ {
		ch := runes[i]
		next := rune(0)
		if i+1 < len(runes) {
			next = runes[i+1]
		}

		if inLineComment {
			if ch == '\n' {
				inLineComment = false
			}
			continue
		}
		if inBlockComment {
			if ch == '*' && next == '/' {
				inBlockComment = false
				i++
			}
			continue
		}

		if !inSingleQuote && !inDoubleQuote && !inBacktick {
			if ch == '-' && next == '-' {
				after := rune(0)
				if i+2 < len(runes) {
					after = runes[i+2]
				}
				if after == 0 || after == ' ' || after == '\t' || after == '\n' || after == '\r' {
					inLineComment = true
					i++
					continue
				}
			}
			if ch == '#' {
				inLineComment = true
				continue
			}
			if ch == '/' && next == '*' {
				inBlockComment = true
				i++
				continue
			}
		}

		if ch == '\'' && !inDoubleQuote && !inBacktick && !escaped {
			inSingleQuote = !inSingleQuote
		} else if ch == '"' && !inSingleQuote && !inBacktick && !escaped {
			inDoubleQuote = !inDoubleQuote
		} else if ch == '`' && !inSingleQuote && !inDoubleQuote {
			inBacktick = !inBacktick
		}

		if ch == ';' && !inSingleQuote && !inDoubleQuote && !inBacktick {
			statement := strings.TrimSpace(current.String())
			if statement != "" {
				statements = append(statements, statement)
			}
			current.Reset()
			escaped = false
			continue
		}

		current.WriteRune(ch)

		if (inSingleQuote || inDoubleQuote) && ch == '\\' && !escaped {
			escaped = true
		} else {
			escaped = false
		}
	}

	statement := strings.TrimSpace(current.String())
	if statement != "" {
		statements = append(statements, statement)
	}

	return statements
}
