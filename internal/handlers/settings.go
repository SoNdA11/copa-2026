package handlers

import (
	"database/sql"
	"net/http"
	"net/url"
	"strings"

	"github.com/gorilla/sessions"

	"copa-2026/internal/services"
)

type SettingsHandler struct {
	db          *sql.DB
	renderer    *Renderer
	authService *services.AuthService
}

func NewSettingsHandler(db *sql.DB, renderer *Renderer, authService *services.AuthService) *SettingsHandler {
	return &SettingsHandler{db: db, renderer: renderer, authService: authService}
}

func (h *SettingsHandler) Page(w http.ResponseWriter, r *http.Request) {
	user := GetUserFromSession(r)
	if user == nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	errMsg := r.URL.Query().Get("error")
	flashMsg := r.URL.Query().Get("flash")

	data := PageData{
		Title: "Configurações",
		User:  user,
		Error: errMsg,
		Flash: flashMsg,
	}

	h.renderer.Render(w, "cmd/web/templates/pages/settings.html", data)
}

func (h *SettingsHandler) UpdateName(w http.ResponseWriter, r *http.Request) {
	user := GetUserFromSession(r)
	if user == nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Redirect(w, r, "/settings?error=Erro+ao+processar+formul%C3%A1rio", http.StatusSeeOther)
		return
	}

	newName := strings.TrimSpace(r.FormValue("name"))
	if newName == "" {
		http.Redirect(w, r, "/settings?error=Nome+n%C3%A3o+pode+ficar+vazio", http.StatusSeeOther)
		return
	}
	if len(newName) < 3 {
		http.Redirect(w, r, "/settings?error=Nome+deve+ter+pelo+menos+3+caracteres", http.StatusSeeOther)
		return
	}

	if err := h.authService.ChangeName(user.ID, newName, user.GroupID); err != nil {
		http.Redirect(w, r, "/settings?error="+url.QueryEscape(err.Error()), http.StatusSeeOther)
		return
	}

	session, _ := store.Get(r, "session")
	session.Values["user_name"] = newName
	session.Options = &sessions.Options{
		Path:     "/",
		MaxAge:   86400 * 30,
		HttpOnly: true,
	}
	if err := session.Save(r, w); err != nil {
		http.Redirect(w, r, "/settings?error=Erro+ao+salvar+sess%C3%A3o", http.StatusSeeOther)
		return
	}

	http.Redirect(w, r, "/settings", http.StatusSeeOther)
}

func (h *SettingsHandler) UploadAvatar(w http.ResponseWriter, r *http.Request) {
	user := GetUserFromSession(r)
	if user == nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 10<<20)
	if err := r.ParseMultipartForm(10 << 20); err != nil {
		http.Redirect(w, r, "/settings?error=Arquivo+muito+grande+(m%C3%A1x+10MB)", http.StatusSeeOther)
		return
	}

	file, _, err := r.FormFile("avatar")
	if err != nil {
		http.Redirect(w, r, "/settings?error=Nenhum+arquivo+enviado", http.StatusSeeOther)
		return
	}
	defer file.Close()

	data, err := processAvatar(file, 256)
	if err != nil {
		http.Redirect(w, r, "/settings?error=Erro+ao+processar+imagem", http.StatusSeeOther)
		return
	}

	avatarURL := avatarDataURI(data)
	if _, err := h.db.Exec("UPDATE users SET avatar_url = $1 WHERE id = $2", avatarURL, user.ID); err != nil {
		http.Redirect(w, r, "/settings?error=Erro+ao+atualizar+avatar", http.StatusSeeOther)
		return
	}

	http.Redirect(w, r, "/settings", http.StatusSeeOther)
}

func (h *SettingsHandler) UpdatePassword(w http.ResponseWriter, r *http.Request) {
	user := GetUserFromSession(r)
	if user == nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Redirect(w, r, "/settings?error=Erro+ao+processar+formul%C3%A1rio", http.StatusSeeOther)
		return
	}

	current := r.FormValue("current_password")
	newPass := r.FormValue("new_password")
	confirm := r.FormValue("confirm_password")

	if current == "" || newPass == "" || confirm == "" {
		http.Redirect(w, r, "/settings?error=Preencha+todos+os+campos", http.StatusSeeOther)
		return
	}
	if newPass != confirm {
		http.Redirect(w, r, "/settings?error=Senhas+n%C3%A3o+conferem", http.StatusSeeOther)
		return
	}
	if len(newPass) < 4 {
		http.Redirect(w, r, "/settings?error=Senha+deve+ter+pelo+menos+4+caracteres", http.StatusSeeOther)
		return
	}

	if err := h.authService.ChangePassword(user.ID, current, newPass); err != nil {
		http.Redirect(w, r, "/settings?error="+url.QueryEscape(err.Error()), http.StatusSeeOther)
		return
	}

	http.Redirect(w, r, "/settings?flash=Senha+alterada+com+sucesso", http.StatusSeeOther)
}

func (h *SettingsHandler) RemoveAvatar(w http.ResponseWriter, r *http.Request) {
	user := GetUserFromSession(r)
	if user == nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	if _, err := h.db.Exec("UPDATE users SET avatar_url = '' WHERE id = $1", user.ID); err != nil {
		http.Redirect(w, r, "/settings?error=Erro+ao+remover+avatar", http.StatusSeeOther)
		return
	}

	http.Redirect(w, r, "/settings", http.StatusSeeOther)
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
