package models

type Team struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	FifaCode  string `json:"fifa_code"`
	GroupName string `json:"group_name"`
	FlagURL   string `json:"flag_url"`
}
