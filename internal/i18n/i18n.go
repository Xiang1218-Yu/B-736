package i18n

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

type Bundle struct {
	fallback string
	messages map[string]map[string]string
}

func New(fallback string) *Bundle {
	return &Bundle{
		fallback: normalizeLang(fallback),
		messages: make(map[string]map[string]string),
	}
}

func (b *Bundle) LoadDir(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		ext := strings.ToLower(filepath.Ext(entry.Name()))
		if ext != ".yaml" && ext != ".yml" {
			continue
		}

		lang := normalizeLang(strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name())))
		if lang == "" {
			continue
		}

		content, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			return fmt.Errorf("read locale file %s failed: %w", entry.Name(), err)
		}

		var tree map[string]interface{}
		if err := yaml.Unmarshal(content, &tree); err != nil {
			return fmt.Errorf("parse locale file %s failed: %w", entry.Name(), err)
		}

		flat := make(map[string]string)
		flatten("", tree, flat)
		if len(flat) == 0 {
			continue
		}
		b.messages[lang] = flat
	}

	if len(b.messages) == 0 {
		return fmt.Errorf("no locale files loaded from %s", dir)
	}

	if _, ok := b.messages[b.fallback]; !ok {
		langs := b.Languages()
		if len(langs) > 0 {
			b.fallback = langs[0]
		}
	}

	return nil
}

func (b *Bundle) Languages() []string {
	langs := make([]string, 0, len(b.messages))
	for lang := range b.messages {
		langs = append(langs, lang)
	}
	sort.Strings(langs)
	return langs
}

func (b *Bundle) Resolve(lang, defaultLang string) string {
	resolved := normalizeLang(lang)
	if resolved != "" {
		if _, ok := b.messages[resolved]; ok {
			return resolved
		}
	}

	fallbackDefault := normalizeLang(defaultLang)
	if fallbackDefault != "" {
		if _, ok := b.messages[fallbackDefault]; ok {
			return fallbackDefault
		}
	}

	if _, ok := b.messages[b.fallback]; ok {
		return b.fallback
	}

	langs := b.Languages()
	if len(langs) > 0 {
		return langs[0]
	}
	return ""
}

func (b *Bundle) T(lang, key string, args ...interface{}) string {
	resolved := b.Resolve(lang, b.fallback)
	msg := b.lookup(resolved, key)
	if msg == "" && resolved != b.fallback {
		msg = b.lookup(b.fallback, key)
	}
	if msg == "" {
		msg = key
	}
	if len(args) > 0 {
		return fmt.Sprintf(msg, args...)
	}
	return msg
}

func (b *Bundle) lookup(lang, key string) string {
	if lang == "" || key == "" {
		return ""
	}
	if dict, ok := b.messages[lang]; ok {
		if value, found := dict[key]; found {
			return value
		}
	}
	return ""
}

func normalizeLang(lang string) string {
	lang = strings.TrimSpace(strings.ToLower(strings.ReplaceAll(lang, "_", "-")))
	if lang == "" {
		return ""
	}
	if strings.HasPrefix(lang, "zh") {
		return "zh"
	}
	if strings.HasPrefix(lang, "en") {
		return "en"
	}
	return lang
}

func flatten(prefix string, value interface{}, out map[string]string) {
	switch node := value.(type) {
	case map[string]interface{}:
		for k, v := range node {
			next := k
			if prefix != "" {
				next = prefix + "." + k
			}
			flatten(next, v, out)
		}
	case []interface{}:
		for idx, item := range node {
			next := fmt.Sprintf("%s.%d", prefix, idx)
			flatten(next, item, out)
		}
	case string:
		if prefix != "" {
			out[prefix] = node
		}
	case int, int64, float64, bool:
		if prefix != "" {
			out[prefix] = fmt.Sprintf("%v", node)
		}
	}
}
