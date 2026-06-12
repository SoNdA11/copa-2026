package handlers

import (
	"net/http"

	"github.com/gorilla/sessions"

	"copa-2026/internal/services"
)

var store sessions.Store

func InitSessionStore(secretKey string) {
	store = sessions.NewCookieStore([]byte(secretKey))
}

type UserInfo struct {
	ID      int64
	Name    string
	IsAdmin bool
}

func GetUserFromSession(r *http.Request) *UserInfo {
	session, err := store.Get(r, "session")
	if err != nil {
		return nil
	}
	userID, ok := session.Values["user_id"].(int64)
	if !ok || userID == 0 {
		return nil
	}
	userName, _ := session.Values["user_name"].(string)
	isAdmin, _ := session.Values["is_admin"].(bool)
	return &UserInfo{ID: userID, Name: userName, IsAdmin: isAdmin}
}

func RequireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user := GetUserFromSession(r)
		if user == nil {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func RequireAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user := GetUserFromSession(r)
		if user == nil || !user.IsAdmin {
			http.Error(w, "Acesso negado", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

type AuthHandler struct {
	authService *services.AuthService
	renderer    *Renderer
}

func NewAuthHandler(authService *services.AuthService, renderer *Renderer) *AuthHandler {
	return &AuthHandler{authService: authService, renderer: renderer}
}

func (h *AuthHandler) LoginPage(w http.ResponseWriter, r *http.Request) {
	data := PageData{Title: "Login"}
	h.renderer.Render(w, "cmd/web/templates/pages/login.html", data)
}

func (h *AuthHandler) RegisterPage(w http.ResponseWriter, r *http.Request) {
	data := PageData{Title: "Cadastro"}
	h.renderer.Render(w, "cmd/web/templates/pages/register.html", data)
}

func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Erro ao processar formulário", http.StatusBadRequest)
		return
	}

	name := r.FormValue("name")
	password := r.FormValue("password")

	if name == "" || password == "" {
		data := PageData{Title: "Login", Error: "Preencha todos os campos"}
		h.renderer.Render(w, "cmd/web/templates/pages/login.html", data)
		return
	}

	result, err := h.authService.Authenticate(name, password)
	if err != nil {
		data := PageData{Title: "Login", Error: err.Error()}
		h.renderer.Render(w, "cmd/web/templates/pages/login.html", data)
		return
	}

	session, _ := store.Get(r, "session")
	session.Values["user_id"] = result.UserID
	session.Values["user_name"] = name
	session.Values["is_admin"] = result.IsAdmin
	session.Options = &sessions.Options{
		Path:     "/",
		MaxAge:   86400 * 30,
		HttpOnly: true,
	}
	if err := session.Save(r, w); err != nil {
		http.Error(w, "Erro ao criar sessão", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (h *AuthHandler) Register(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Erro ao processar formulário", http.StatusBadRequest)
		return
	}

	name := r.FormValue("name")
	password := r.FormValue("password")
	confirm := r.FormValue("confirm_password")

	if name == "" || password == "" || confirm == "" {
		data := PageData{Title: "Cadastro", Error: "Preencha todos os campos"}
		h.renderer.Render(w, "cmd/web/templates/pages/register.html", data)
		return
	}

	if password != confirm {
		data := PageData{Title: "Cadastro", Error: "Senhas não conferem"}
		h.renderer.Render(w, "cmd/web/templates/pages/register.html", data)
		return
	}

	if len(password) < 4 {
		data := PageData{Title: "Cadastro", Error: "Senha deve ter no mínimo 4 caracteres"}
		h.renderer.Render(w, "cmd/web/templates/pages/register.html", data)
		return
	}

	userID, err := h.authService.Register(name, password)
	if err != nil {
		data := PageData{Title: "Cadastro", Error: err.Error()}
		h.renderer.Render(w, "cmd/web/templates/pages/register.html", data)
		return
	}

	session, _ := store.Get(r, "session")
	session.Values["user_id"] = userID
	session.Values["user_name"] = name
	session.Values["is_admin"] = false
	session.Options = &sessions.Options{
		Path:     "/",
		MaxAge:   86400 * 30,
		HttpOnly: true,
	}
	if err := session.Save(r, w); err != nil {
		http.Error(w, "Erro ao criar sessão", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	session, _ := store.Get(r, "session")
	session.Values["user_id"] = nil
	session.Values["user_name"] = nil
	session.Values["is_admin"] = nil
	session.Options.MaxAge = -1
	session.Save(r, w)
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}
