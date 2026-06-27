package handlers

import (
	"html/template"
	"os"
	"time"

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
	"sub": func(a, b int) int {
		return a - b
	},
	"div": func(a, b int) int {
		if b == 0 {
			return 0
		}
		return a / b
	},
	"mod": func(a, b int) int {
		if b == 0 {
			return 0
		}
		return a % b
	},
	"dict": func(values ...interface{}) map[string]interface{} {
		m := make(map[string]interface{})
		for i := 0; i+1 < len(values); i += 2 {
			key, ok := values[i].(string)
			if !ok {
				continue
			}
			m[key] = values[i+1]
		}
		return m
	},
	"seq": func(start, end int) []int {
		var s []int
		for i := start; i <= end; i++ {
			s = append(s, i)
		}
		return s
	},
	"first3": func(data interface{}) []models.UserRanking {
		users, ok := data.([]models.UserRanking)
		if !ok {
			return []models.UserRanking{{}, {}, {}}
		}
		result := make([]models.UserRanking, 3)
		for i := 0; i < 3 && i < len(users); i++ {
			result[i] = users[i]
		}
		return result
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
	"toBRT": func(t time.Time) string {
		loc, _ := time.LoadLocation("America/Sao_Paulo")
		return t.In(loc).Format("02/01/2006 15:04")
	},
	"toBRTStr": func(s string) string {
		if s == "" {
			return s
		}
		loc, _ := time.LoadLocation("America/Sao_Paulo")
		for _, layout := range []string{
			"2006-01-02 15:04:05",
			"2006-01-02T15:04:05Z",
			"2006-01-02T15:04:05-07:00",
			time.RFC3339,
		} {
			if t, err := time.ParseInLocation(layout, s, time.UTC); err == nil {
				return t.In(loc).Format("02/01/2006 15:04")
			}
		}
		return s
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
	"safeURL": func(s string) template.URL {
		return template.URL(s)
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
