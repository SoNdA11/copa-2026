package handlers

import (
	"database/sql"
	"net/http"
)

type SettingsHandler struct {
	db       *sql.DB
	renderer *Renderer
}

func NewSettingsHandler(db *sql.DB, renderer *Renderer) *SettingsHandler {
	return &SettingsHandler{db: db, renderer: renderer}
}

func (h *SettingsHandler) Page(w http.ResponseWriter, r *http.Request) {
	user := GetUserFromSession(r)
	if user == nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	data := PageData{
		Title: "Configurações",
		User:  user,
	}

	h.renderer.Render(w, "cmd/web/templates/pages/settings.html", data)
}

func (h *SettingsHandler) DeleteAccount(w http.ResponseWriter, r *http.Request) {
	user := GetUserFromSession(r)
	if user == nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "Erro no formulário", http.StatusBadRequest)
		return
	}

	confirm := r.FormValue("confirm")
	if confirm != "EXCLUIR" {
		data := PageData{
			Title: "Configurações",
			User:  user,
			Error: "Digite EXCLUIR para confirmar a exclusão da conta",
		}
		h.renderer.Render(w, "cmd/web/templates/pages/settings.html", data)
		return
	}

	tx, err := h.db.Begin()
	if err != nil {
		http.Error(w, "Erro ao iniciar transação", http.StatusInternalServerError)
		return
	}
	defer tx.Rollback()

	if _, err := tx.Exec("DELETE FROM bets WHERE user_id = $1", user.ID); err != nil {
		http.Error(w, "Erro ao excluir palpites", http.StatusInternalServerError)
		return
	}

	if _, err := tx.Exec("DELETE FROM special_bets WHERE user_id = $1", user.ID); err != nil {
		http.Error(w, "Erro ao excluir palpites especiais", http.StatusInternalServerError)
		return
	}

	if _, err := tx.Exec("DELETE FROM users WHERE id = $1", user.ID); err != nil {
		http.Error(w, "Erro ao excluir conta", http.StatusInternalServerError)
		return
	}

	if err := tx.Commit(); err != nil {
		http.Error(w, "Erro ao finalizar exclusão", http.StatusInternalServerError)
		return
	}

	session, _ := store.Get(r, "session")
	session.Values["user_id"] = nil
	session.Values["user_name"] = nil
	session.Options.MaxAge = -1
	session.Save(r, w)

	http.Redirect(w, r, "/", http.StatusSeeOther)
}
