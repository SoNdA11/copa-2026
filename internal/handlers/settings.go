package handlers

import (
	"database/sql"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gorilla/sessions"
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

	errMsg := r.URL.Query().Get("error")

	data := PageData{
		Title: "Configurações",
		User:  user,
		Error: errMsg,
	}

	h.renderer.Render(w, "cmd/web/templates/pages/settings.html", data)
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

	avatarDir := "data/avatars"
	if err := os.MkdirAll(avatarDir, 0755); err != nil {
		http.Redirect(w, r, "/settings?error=Erro+ao+criar+diret%C3%B3rio", http.StatusSeeOther)
		return
	}

	oldFiles, _ := filepath.Glob(filepath.Join(avatarDir, fmt.Sprintf("%d_%d_*.png", user.ID, user.GroupID)))
	for _, f := range oldFiles {
		os.Remove(f)
	}

	filename := fmt.Sprintf("%d_%d_%d.png", user.ID, user.GroupID, time.Now().UnixMilli())
	dst, err := os.Create(filepath.Join(avatarDir, filename))
	if err != nil {
		http.Redirect(w, r, "/settings?error=Erro+ao+salvar+arquivo", http.StatusSeeOther)
		return
	}
	defer dst.Close()

	if err := processAvatar(file, dst, 256); err != nil {
		http.Redirect(w, r, "/settings?error=Erro+ao+processar+imagem", http.StatusSeeOther)
		return
	}

	avatarURL := "/static/avatars/" + filename
	if _, err := h.db.Exec("UPDATE users SET avatar_url = $1 WHERE id = $2", avatarURL, user.ID); err != nil {
		http.Redirect(w, r, "/settings?error=Erro+ao+atualizar+avatar", http.StatusSeeOther)
		return
	}

	session, _ := store.Get(r, "session")
	session.Values["avatar_url"] = avatarURL
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

func (h *SettingsHandler) RemoveAvatar(w http.ResponseWriter, r *http.Request) {
	user := GetUserFromSession(r)
	if user == nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	if user.AvatarURL != "" {
		oldPath := user.AvatarURL
		if strings.HasPrefix(oldPath, "/static/avatars/") {
			os.Remove("data/avatars/" + strings.TrimPrefix(oldPath, "/static/avatars/"))
		}

		oldFiles, _ := filepath.Glob(filepath.Join("data/avatars", fmt.Sprintf("%d_%d_*.png", user.ID, user.GroupID)))
		for _, f := range oldFiles {
			os.Remove(f)
		}

		if _, err := h.db.Exec("UPDATE users SET avatar_url = '' WHERE id = $1", user.ID); err != nil {
			http.Redirect(w, r, "/settings?error=Erro+ao+remover+avatar", http.StatusSeeOther)
			return
		}

		session, _ := store.Get(r, "session")
		session.Values["avatar_url"] = ""
		session.Options = &sessions.Options{
			Path:     "/",
			MaxAge:   86400 * 30,
			HttpOnly: true,
		}
		session.Save(r, w)
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

	// Remove avatar file if exists
	if user.AvatarURL != "" {
		avatarPath := user.AvatarURL
		if strings.HasPrefix(avatarPath, "/static/avatars/") {
			os.Remove("data/avatars/" + strings.TrimPrefix(avatarPath, "/static/avatars/"))
		}
	}
	oldFiles, _ := filepath.Glob(filepath.Join("data/avatars", fmt.Sprintf("%d_%d_*.png", user.ID, user.GroupID)))
	for _, f := range oldFiles {
		os.Remove(f)
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
