package templates

import (
	"html/template"
	"path/filepath"
)

// LoadTemplates 加载所有模板
func LoadTemplates(funcMap template.FuncMap) *template.Template {
	tmpl := template.New("").Funcs(funcMap)

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
