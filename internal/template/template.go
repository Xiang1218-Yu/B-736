package template

import (
	"fmt"
	"html/template"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"prompt736/internal/app"
	"prompt736/internal/service"
)

// Load 加载所有模板文件并返回 *template.Template 实例
func Load() *template.Template {
	tmpl := template.New("").Funcs(GetTemplateFuncs())

	patterns := []string{
		"templates/layouts/*.html",
		"templates/pages/*.html",
		"templates/admin/*.html",
	}

	for _, pattern := range patterns {
		files, _ := filepath.Glob(pattern)
		for _, file := range files {
			tmpl = template.Must(tmpl.ParseFiles(file))
		}
	}

	return tmpl
}

// GetTemplateFuncs 返回全局模板函数映射（不依赖 PageData）
func GetTemplateFuncs() template.FuncMap {
	return template.FuncMap{
		"truncate": func(a interface{}, b interface{}) string {
			var s string
			var n int

			switch v := a.(type) {
			case int:
				n = v
			case int8:
				n = int(v)
			case int16:
				n = int(v)
			case int32:
				n = int(v)
			case int64:
				n = int(v)
			case uint:
				n = int(v)
			case uint8:
				n = int(v)
			case uint16:
				n = int(v)
			case uint32:
				n = int(v)
			case uint64:
				n = int(v)
			case string:
				s = v
			}

			switch v := b.(type) {
			case int:
				n = v
			case int8:
				n = int(v)
			case int16:
				n = int(v)
			case int32:
				n = int(v)
			case int64:
				n = int(v)
			case uint:
				n = int(v)
			case uint8:
				n = int(v)
			case uint16:
				n = int(v)
			case uint32:
				n = int(v)
			case uint64:
				n = int(v)
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
			case time.Time:
				return t.Format("2006-01-02")
			case *time.Time:
				if t == nil {
					return ""
				}
				return t.Format("2006-01-02")
			case string:
				return t
			default:
				return ""
			}
		},
		"safeHTML": func(s string) template.HTML {
			return service.SafeArticleHTML(s)
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
		"resourceTypeClass": service.CanonicalResourceType,
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
		"ratingStars": service.BuildRatingStars,
	}
}

// BuildTemplateFuncs 根据 PageData 构建完整模板函数映射（包含 i18n、CSRF 等上下文相关函数）
func BuildTemplateFuncs(a *app.App, data app.PageData) template.FuncMap {
	funcs := GetTemplateFuncs()

	funcs["T"] = func(key string, args ...interface{}) string {
		return a.I18n.T(data.Locale, key, args...)
	}
	funcs["langURL"] = func(lang string) string {
		return service.BuildLangSwitchURL(data.RequestPath, data.RawQuery, lang)
	}
	funcs["langActive"] = func(lang string) bool {
		resolved := a.I18n.Resolve(lang, data.Locale)
		return resolved == data.Locale
	}
	funcs["resourceTypeLabel"] = func(resourceType string) string {
		key := "resource_type." + service.CanonicalResourceType(resourceType)
		label := a.I18n.T(data.Locale, key)
		if label == key {
			return resourceType
		}
		return label
	}
	funcs["sourceTypeLabel"] = func(sourceType string) string {
		key := "source_type." + service.CanonicalSourceType(sourceType)
		label := a.I18n.T(data.Locale, key)
		if label == key {
			return sourceType
		}
		return label
	}
	funcs["roleLabel"] = func(role string) string {
		key := "role." + strings.ToLower(role)
		label := a.I18n.T(data.Locale, key)
		if label == key {
			return role
		}
		return label
	}
	funcs["statusLabel"] = func(status string) string {
		key := "status." + strings.ToLower(status)
		label := a.I18n.T(data.Locale, key)
		if label == key {
			return status
		}
		return label
	}
	funcs["siteText"] = func(key, fallback string) string {
		value := a.I18n.T(data.Locale, key)
		if value == key || strings.TrimSpace(value) == "" {
			return fallback
		}
		return value
	}
	funcs["csrfField"] = func() template.HTML {
		if data.CSRFToken == "" {
			return ""
		}
		return template.HTML(fmt.Sprintf(
			`<input type="hidden" name="%s" value="%s">`,
			service.CSRFFormField,
			template.HTMLEscapeString(data.CSRFToken),
		))
	}

	return funcs
}

// ---- 工具函数 ----

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
