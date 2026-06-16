package handlers

import (
	"database/sql"
	"net/http"
	"strconv"
)

type AdminHandler struct {
	db       *sql.DB
	renderer *Renderer
}

func NewAdminHandler(db *sql.DB, renderer *Renderer) *AdminHandler {
	return &AdminHandler{db: db, renderer: renderer}
}

type adminUserRow struct {
	ID               int64
	Name             string
	Points           int
	PointsAdjustment int
	Total            int
}

func (h *AdminHandler) UsersPage(w http.ResponseWriter, r *http.Request) {
	user := GetUserFromSession(r)
	if user == nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	rows, err := h.db.Query(`
		SELECT u.id, u.name,
			COALESCE((SELECT SUM(points) FROM bets WHERE user_id = u.id), 0) +
			COALESCE((SELECT SUM(points) FROM special_bets WHERE user_id = u.id), 0) as points,
			COALESCE(u.points_adjustment, 0) as adjustment
		FROM users u
		WHERE u.is_admin = 0 AND u.group_id = $1
		ORDER BY u.name
	`, user.GroupID)
	if err != nil {
		http.Error(w, "Erro ao carregar usuários", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var users []adminUserRow
	for rows.Next() {
		var u adminUserRow
		if err := rows.Scan(&u.ID, &u.Name, &u.Points, &u.PointsAdjustment); err != nil {
			continue
		}
		u.Total = u.Points + u.PointsAdjustment
		users = append(users, u)
	}

	data := PageData{
		Title: "Administração",
		User:  user,
		Data:  users,
		Flash: r.URL.Query().Get("flash"),
	}
	h.renderer.Render(w, "cmd/web/templates/pages/admin.html", data)
}

func (h *AdminHandler) UpdatePoints(w http.ResponseWriter, r *http.Request) {
	user := GetUserFromSession(r)
	if user == nil {
		http.Error(w, "Não autorizado", http.StatusUnauthorized)
		return
	}

	userIDStr := r.FormValue("user_id")
	if userIDStr == "" {
		http.Error(w, "ID do usuário não informado", http.StatusBadRequest)
		return
	}

	userID, err := strconv.ParseInt(userIDStr, 10, 64)
	if err != nil {
		http.Error(w, "ID inválido", http.StatusBadRequest)
		return
	}

	adjustmentStr := r.FormValue("adjustment")
	if adjustmentStr == "" {
		adjustmentStr = "0"
	}

	adjustment, err := strconv.Atoi(adjustmentStr)
	if err != nil {
		http.Error(w, "Ajuste inválido", http.StatusBadRequest)
		return
	}

	var targetGroupID int64
	err = h.db.QueryRow("SELECT group_id FROM users WHERE id = $1", userID).Scan(&targetGroupID)
	if err != nil || targetGroupID != user.GroupID {
		http.Error(w, "Usuário não encontrado", http.StatusNotFound)
		return
	}

	if _, err := h.db.Exec("UPDATE users SET points_adjustment = $1 WHERE id = $2 AND is_admin = 0", adjustment, userID); err != nil {
		http.Error(w, "Erro ao atualizar pontos", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/admin?flash=ajuste+salvo", http.StatusSeeOther)
}

func (h *AdminHandler) DeleteUser(w http.ResponseWriter, r *http.Request) {
	user := GetUserFromSession(r)
	if user == nil {
		http.Error(w, "Não autorizado", http.StatusUnauthorized)
		return
	}

	userIDStr := r.FormValue("user_id")
	if userIDStr == "" {
		http.Error(w, "ID do usuário não informado", http.StatusBadRequest)
		return
	}

	userID, err := strconv.ParseInt(userIDStr, 10, 64)
	if err != nil {
		http.Error(w, "ID inválido", http.StatusBadRequest)
		return
	}

	var targetGroupID int64
	err = h.db.QueryRow("SELECT group_id FROM users WHERE id = $1", userID).Scan(&targetGroupID)
	if err != nil || targetGroupID != user.GroupID {
		http.Error(w, "Usuário não encontrado", http.StatusNotFound)
		return
	}

	tx, err := h.db.Begin()
	if err != nil {
		http.Error(w, "Erro ao iniciar transação", http.StatusInternalServerError)
		return
	}
	defer tx.Rollback()

	if _, err := tx.Exec("DELETE FROM bets WHERE user_id = $1", userID); err != nil {
		http.Error(w, "Erro ao excluir palpites", http.StatusInternalServerError)
		return
	}

	if _, err := tx.Exec("DELETE FROM special_bets WHERE user_id = $1", userID); err != nil {
		http.Error(w, "Erro ao excluir palpites especiais", http.StatusInternalServerError)
		return
	}

	if _, err := tx.Exec("DELETE FROM users WHERE id = $1 AND is_admin = 0", userID); err != nil {
		http.Error(w, "Erro ao excluir usuário", http.StatusInternalServerError)
		return
	}

	if err := tx.Commit(); err != nil {
		http.Error(w, "Erro ao finalizar transação", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/admin?flash=usuario+excluido", http.StatusSeeOther)
}
