package main

import (
	"log"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"copa-2026/internal/config"
	"copa-2026/internal/database"
	"copa-2026/internal/handlers"
	"copa-2026/internal/services"
)

func main() {
	cfg := config.Load()
	handlers.InitSessionStore(cfg.SecretKey)

	db, err := database.Connect(cfg)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer db.Close()

	if err := database.RunMigrations(db); err != nil {
		log.Fatalf("Failed to run migrations: %v", err)
	}

	seedSvc := services.NewSeedService(db)
	if err := seedSvc.SeedFromFiles(); err != nil {
		log.Printf("Seed error: %v", err)
	}

	// validate templates load
	if _, err := handlers.LoadPageTemplate(
		"cmd/web/templates/layout.html",
		"cmd/web/templates/partials/nav.html",
		"cmd/web/templates/partials/match_row.html",
		"cmd/web/templates/partials/bet_form.html",
		"cmd/web/templates/partials/ranking_table.html",
		"cmd/web/templates/partials/group_standings.html",
		"cmd/web/templates/partials/match_bets_group.html",
		"cmd/web/templates/partials/user_bets_partial.html",
	); err != nil {
		log.Fatalf("Failed to validate templates: %v", err)
	}

	renderer := handlers.NewRenderer()

	betSvc := services.NewBetService(db)
	rankingSvc := services.NewRankingService(db)

	syncSvc := services.NewSyncService(db, cfg.APIURL, betSvc)
	go func() {
		if err := syncSvc.SyncAllData(); err != nil {
			log.Printf("API sync error: %v", err)
		}
	}()
	syncSvc.Start(5 * time.Minute)

	authHandler := handlers.NewAuthHandler(services.NewAuthService(db), renderer)
	matchHandler := handlers.NewMatchHandler(db, betSvc, renderer)
	betHandler := handlers.NewBetHandler(betSvc, db, renderer)
	rankingHandler := handlers.NewRankingHandler(rankingSvc, renderer)
	specialBetHandler := handlers.NewSpecialBetHandler(db, renderer)
	settingsHandler := handlers.NewSettingsHandler(db, renderer)
	adminHandler := handlers.NewAdminHandler(db, renderer)
	profileHandler := handlers.NewProfileHandler(betSvc, rankingSvc, db, renderer)

	authSvc := services.NewAuthService(db)
	if cfg.AdminPassword != "" {
		if err := authSvc.CreateAdmin("admin", cfg.AdminPassword); err != nil {
			log.Printf("Failed to create admin user: %v", err)
		}
	}

	r := chi.NewRouter()

	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Compress(5))

	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
	})

	r.Get("/dashboard", func(w http.ResponseWriter, r *http.Request) {
		user := handlers.GetUserFromSession(r)
		data := handlers.PageData{
			Title: "Início",
			User:  user,
		}
		renderer.Render(w, "cmd/web/templates/pages/dashboard.html", data)
	})

	// Auth
	r.Get("/login", authHandler.LoginPage)
	r.Post("/login", authHandler.Login)
	r.Get("/register", authHandler.RegisterPage)
	r.Post("/register", authHandler.Register)
	r.Get("/logout", authHandler.Logout)

	// Matches
	r.Get("/matches", matchHandler.List)
	r.Get("/matches/{id}", matchHandler.Detail)
	r.Get("/matches/{id}/bets", matchHandler.GroupBets)

	// Bets (requires auth)
	r.Group(func(r chi.Router) {
		r.Use(handlers.RequireAuth)
		r.Post("/matches/{id}/bet", betHandler.Place)
		r.Get("/my-bets", betHandler.MyBets)
	})

	// Ranking
	r.Get("/ranking", rankingHandler.List)
	r.Get("/ranking/partial", rankingHandler.Partial)

	// User profile (public)
	r.Get("/user/{id}/bets", profileHandler.UserBets)
	r.Get("/user/{id}/bets/partial", profileHandler.UserBetsPartial)

	// Special bets
	r.Group(func(r chi.Router) {
		r.Use(handlers.RequireAuth)
		r.Get("/special", specialBetHandler.List)
		r.Get("/special/all", specialBetHandler.AllBets)
		r.Post("/special/bet", specialBetHandler.Place)
	})

	// Settings
	r.Group(func(r chi.Router) {
		r.Use(handlers.RequireAuth)
		r.Get("/settings", settingsHandler.Page)
		r.Post("/settings/delete", settingsHandler.DeleteAccount)
	})

	// Admin
	r.Group(func(r chi.Router) {
		r.Use(handlers.RequireAuth)
		r.Use(handlers.RequireAdmin)
		r.Get("/admin", adminHandler.UsersPage)
		r.Post("/admin/update-points", adminHandler.UpdatePoints)
		r.Post("/admin/delete-user", adminHandler.DeleteUser)
	})

	log.Printf("Server starting on :%s", cfg.Port)
	if err := http.ListenAndServe(":"+cfg.Port, r); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}
