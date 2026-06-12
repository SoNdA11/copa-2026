package services

import (
	"database/sql"
	"errors"

	"golang.org/x/crypto/bcrypt"
)

type AuthService struct {
	db *sql.DB
}

func NewAuthService(db *sql.DB) *AuthService {
	return &AuthService{db: db}
}

type AuthResult struct {
	UserID  int64
	IsAdmin bool
}

func (s *AuthService) Register(name, password string) (int64, error) {
	var existing int
	err := s.db.QueryRow("SELECT COUNT(*) FROM users WHERE name = $1", name).Scan(&existing)
	if err != nil {
		return 0, err
	}
	if existing > 0 {
		return 0, errors.New("nome de usuário já existe")
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return 0, err
	}

	result, err := s.db.Exec("INSERT INTO users (name, password_hash) VALUES ($1, $2)", name, string(hash))
	if err != nil {
		return 0, err
	}

	return result.LastInsertId()
}

func (s *AuthService) Authenticate(name, password string) (*AuthResult, error) {
	var id int64
	var hash string
	var isAdmin bool
	err := s.db.QueryRow("SELECT id, password_hash, COALESCE(is_admin, 0) FROM users WHERE name = $1", name).Scan(&id, &hash, &isAdmin)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errors.New("usuário ou senha inválidos")
		}
		return nil, err
	}

	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)); err != nil {
		return nil, errors.New("usuário ou senha inválidos")
	}

	return &AuthResult{UserID: id, IsAdmin: isAdmin}, nil
}

func (s *AuthService) CreateAdmin(name, password string) error {
	var existing int
	s.db.QueryRow("SELECT COUNT(*) FROM users WHERE name = $1", name).Scan(&existing)
	if existing > 0 {
		hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
		if err != nil {
			return err
		}
		_, err = s.db.Exec("UPDATE users SET is_admin = 1, password_hash = $1 WHERE name = $2", string(hash), name)
		return err
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}

	_, err = s.db.Exec("INSERT INTO users (name, password_hash, is_admin) VALUES ($1, $2, 1)", name, string(hash))
	return err
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
