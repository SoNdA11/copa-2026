package services

import (
	"database/sql"
	"errors"

	"copa-2026/internal/models"
	"golang.org/x/crypto/bcrypt"
)

type AuthService struct {
	db *sql.DB
}

func NewAuthService(db *sql.DB) *AuthService {
	return &AuthService{db: db}
}

type AuthResult struct {
	UserID    int64
	IsAdmin   bool
	GroupID   int64
	GroupName string
	GroupSlug string
	AvatarURL string
}

func (s *AuthService) Register(name, password string, groupID int64) (int64, error) {
	var existing int
	err := s.db.QueryRow("SELECT COUNT(*) FROM users WHERE name = $1 AND group_id = $2", name, groupID).Scan(&existing)
	if err != nil {
		return 0, err
	}
	if existing > 0 {
		return 0, errors.New("nome de usuário já existe nesse grupo")
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return 0, err
	}

	var userID int64
	err = s.db.QueryRow("INSERT INTO users (name, password_hash, group_id) VALUES ($1, $2, $3) RETURNING id", name, string(hash), groupID).Scan(&userID)
	if err != nil {
		return 0, err
	}

	return userID, nil
}

func (s *AuthService) Authenticate(name, password string, groupID int64) (*AuthResult, error) {
	var id int64
	var hash string
	var isAdmin bool
	var gID int64
	var groupName, groupSlug, avatarURL string
	err := s.db.QueryRow(`
		SELECT u.id, u.password_hash, COALESCE(u.is_admin, 0), u.group_id, g.name, g.slug, COALESCE(u.avatar_url, '')
		FROM users u
		JOIN groups g ON g.id = u.group_id
		WHERE u.name = $1 AND u.group_id = $2
	`, name, groupID).Scan(&id, &hash, &isAdmin, &gID, &groupName, &groupSlug, &avatarURL)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errors.New("usuário ou senha inválidos")
		}
		return nil, err
	}

	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)); err != nil {
		return nil, errors.New("usuário ou senha inválidos")
	}

	return &AuthResult{UserID: id, IsAdmin: isAdmin, GroupID: gID, GroupName: groupName, GroupSlug: groupSlug, AvatarURL: avatarURL}, nil
}

func (s *AuthService) CreateAdmin(name, password string, groupID int64) error {
	var existing int
	s.db.QueryRow("SELECT COUNT(*) FROM users WHERE name = $1 AND group_id = $2", name, groupID).Scan(&existing)
	if existing > 0 {
		hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
		if err != nil {
			return err
		}
		_, err = s.db.Exec("UPDATE users SET is_admin = 1, password_hash = $1 WHERE name = $2 AND group_id = $3", string(hash), name, groupID)
		return err
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}

	_, err = s.db.Exec("INSERT INTO users (name, password_hash, is_admin, group_id) VALUES ($1, $2, 1, $3)", name, string(hash), groupID)
	return err
}

func (s *AuthService) ChangeName(userID int64, newName string, groupID int64) error {
	var existing int
	err := s.db.QueryRow("SELECT COUNT(*) FROM users WHERE name = $1 AND group_id = $2 AND id != $3", newName, groupID, userID).Scan(&existing)
	if err != nil {
		return err
	}
	if existing > 0 {
		return errors.New("nome de usuário já existe nesse grupo")
	}

	_, err = s.db.Exec("UPDATE users SET name = $1 WHERE id = $2", newName, userID)
	return err
}

func (s *AuthService) ResetPassword(userID int64, newPassword string) error {
	hash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	_, err = s.db.Exec("UPDATE users SET password_hash = $1 WHERE id = $2", string(hash), userID)
	return err
}

func (s *AuthService) ChangePassword(userID int64, currentPassword, newPassword string) error {
	var hash string
	err := s.db.QueryRow("SELECT password_hash FROM users WHERE id = $1", userID).Scan(&hash)
	if err != nil {
		return errors.New("usuário não encontrado")
	}

	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(currentPassword)); err != nil {
		return errors.New("senha atual incorreta")
	}

	newHash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return err
	}

	_, err = s.db.Exec("UPDATE users SET password_hash = $1 WHERE id = $2", string(newHash), userID)
	return err
}

func (s *AuthService) GetGroups() ([]models.Group, error) {
	rows, err := s.db.Query("SELECT id, name, slug FROM groups ORDER BY id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var groups []models.Group
	for rows.Next() {
		var g models.Group
		if err := rows.Scan(&g.ID, &g.Name, &g.Slug); err != nil {
			return nil, err
		}
		groups = append(groups, g)
	}
	return groups, rows.Err()
}

func (s *AuthService) GetAllUsers() ([]struct {
	ID     int64
	Name   string
	Points int
}, error) {
	rows, err := s.db.Query(`
		SELECT u.id, u.name,
			COALESCE(b.total, 0) + COALESCE(s.total, 0) + COALESCE(u.points_adjustment, 0) as points
		FROM users u
		LEFT JOIN (SELECT user_id, SUM(points) as total FROM bets GROUP BY user_id) b ON b.user_id = u.id
		LEFT JOIN (SELECT user_id, SUM(points) as total FROM special_bets GROUP BY user_id) s ON s.user_id = u.id
		WHERE u.is_admin = 0
		ORDER BY points DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []struct {
		ID     int64
		Name   string
		Points int
	}
	for rows.Next() {
		var u struct {
			ID     int64
			Name   string
			Points int
		}
		if err := rows.Scan(&u.ID, &u.Name, &u.Points); err != nil {
			return nil, err
		}
		users = append(users, u)
	}
	return users, rows.Err()
}
