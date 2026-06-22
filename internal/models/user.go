package models

import "time"

type User struct {
	ID           int64     `json:"id"`
	Name         string    `json:"name"`
	PasswordHash string    `json:"-"`
	CreatedAt    time.Time `json:"created_at"`
}

type UserRanking struct {
	UserID   int64  `json:"user_id"`
	Name     string `json:"name"`
	Points   int    `json:"points"`
	Position int    `json:"position"`
	AvatarURL string `json:"avatar_url"`
}
