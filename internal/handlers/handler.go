package handlers

import (
	"html/template"
	"os"

	"copa-2026/internal/models"
	"copa-2026/internal/services"
)

type PageData struct {
	Title  string
	User   *UserInfo
	Error  string
	Flash  string
	Data   interface{}
	Bet    interface{}
	Stage  string
	Groups []models.Group
}

var funcMap = template.FuncMap{
	"add": func(a, b int) int {
		return a + b
	},
	"seq": func(start, end int) []int {
		var s []int
		for i := start; i <= end; i++ {
			s = append(s, i)
		}
		return s
	},
	"deref": func(p *int) int {
		if p == nil {
			return -1
		}
		return *p
	},
	"specialBettingOpen": func() bool {
		return services.IsSpecialBettingOpen()
	},
	"stageLabel": func(stage string) string {
		labels := map[string]string{
			"group": "Fase de Grupos",
			"r32":   "16 Avos",
			"r16":   "Oitavas",
			"qf":    "Quartas",
			"sf":    "Semi",
			"third": "3º Lugar",
			"final": "Final",
		}
		if l, ok := labels[stage]; ok {
			return l
		}
		return stage
	},
}

func LoadPageTemplate(files ...string) (*template.Template, error) {
	tmpl := template.New("").Funcs(funcMap)
	for _, f := range files {
		content, err := os.ReadFile(f)
		if err != nil {
			return nil, err
		}
		if _, err := tmpl.Parse(string(content)); err != nil {
			return nil, err
		}
	}
	return tmpl, nil
}
