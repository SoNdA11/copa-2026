package main

import (
	"log"
	"net/http"

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

	if _, err := handlers.LoadPageTemplate(
		"cmd/web/templates/layout.html",
		"cmd/web/templates/partials/nav.html",
		"cmd/web/templates/partials/match_row.html",
		"cmd/web/templates/partials/bet_form.html",
		"cmd/web/templates/partials/inline_bet.html",
		"cmd/web/templates/partials/ranking_table.html",
		"cmd/web/templates/partials/group_standings.html",
		"cmd/web/templates/partials/match_bets_group.html",
		"cmd/web/templates/partials/user_bets_partial.html",
		"cmd/web/templates/pages/bracket.html",
	); err != nil {
		log.Fatalf("Failed to validate templates: %v", err)
	}

	renderer := handlers.NewRenderer(db)

	authSvc := services.NewAuthService(db)
	betSvc := services.NewBetService(db)
	rankingSvc := services.NewRankingService(db)
	statsSvc := services.NewStatsService(db)
	knockoutSvc := services.NewKnockoutService(db)
	bracketSvc := services.NewBracketService(db, knockoutSvc)

	syncSvc := services.NewSyncService(db, cfg.APIURL, betSvc)
	go func() {
		knockoutSvc.RecalculateAll()
		if err := syncSvc.SyncAllData(); err != nil {
			log.Printf("API sync error: %v", err)
		}
		betSvc.RecalculateAllFinishedMatches()
		knockoutSvc.RecalculateAll()
	}()
	syncSvc.Start()

	authHandler := handlers.NewAuthHandler(authSvc, renderer)
	matchHandler := handlers.NewMatchHandler(db, betSvc, syncSvc, bracketSvc, renderer)
	betHandler := handlers.NewBetHandler(betSvc, bracketSvc, db, renderer)
	rankingHandler := handlers.NewRankingHandler(rankingSvc, renderer)
	specialBetHandler := handlers.NewSpecialBetHandler(db, renderer)
	settingsHandler := handlers.NewSettingsHandler(db, renderer, authSvc)
	adminHandler := handlers.NewAdminHandler(db, betSvc, syncSvc, knockoutSvc, authSvc, renderer)
	profileHandler := handlers.NewProfileHandler(betSvc, rankingSvc, db, renderer)
	statsHandler := handlers.NewStatsHandler(statsSvc, renderer)

	if cfg.AdminPassword != "" {
		groups, err := authSvc.GetGroups()
		if err != nil {
			log.Printf("Failed to load groups for admin creation: %v", err)
		} else {
			for _, g := range groups {
				if err := authSvc.CreateAdmin("admin", cfg.AdminPassword, g.ID); err != nil {
					log.Printf("Failed to create admin user for group %s: %v", g.Name, err)
				}
			}
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

	r.Get("/dashboard", statsHandler.DashboardPage)

	// Auth
	r.Get("/login", authHandler.LoginPage)
	r.Post("/login", authHandler.Login)
	r.Get("/register", authHandler.RegisterPage)
	r.Post("/register", authHandler.Register)
	r.Get("/logout", authHandler.Logout)

	// Matches
	r.Get("/matches", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
		w.Header().Set("Pragma", "no-cache")
		w.Header().Set("Expires", "0")
		matchHandler.List(w, r)
	})
	r.Get("/matches/{id}", matchHandler.Detail)
	r.Get("/matches/{id}/bets", matchHandler.GroupBets)
	r.Get("/matches/{id}/inline-bet", matchHandler.InlineBetForm)
	r.Get("/knockout", matchHandler.Bracket)

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
		r.Post("/settings/name", settingsHandler.UpdateName)
		r.Post("/settings/delete", settingsHandler.DeleteAccount)
		r.Post("/settings/avatar", settingsHandler.UploadAvatar)
		r.Post("/settings/avatar/remove", settingsHandler.RemoveAvatar)
		r.Post("/settings/password", settingsHandler.UpdatePassword)
	})

	// Static files
	r.Get("/static/*", func(w http.ResponseWriter, r *http.Request) {
		http.StripPrefix("/static/", http.FileServer(http.Dir("data"))).ServeHTTP(w, r)
	})

	// Admin
	r.Group(func(r chi.Router) {
		r.Use(handlers.RequireAuth)
		r.Use(handlers.RequireAdmin)
		r.Get("/admin", adminHandler.UsersPage)
		r.Post("/admin/update-points", adminHandler.UpdatePoints)
		r.Post("/admin/delete-user", adminHandler.DeleteUser)
		r.Get("/admin/matches", adminHandler.MatchesPage)
		r.Post("/admin/update-match", adminHandler.UpdateMatch)
		r.Get("/admin/match-bets", adminHandler.MatchBetsPage)
		r.Post("/admin/sync", adminHandler.ForceSync)
		r.Post("/admin/reset-password", adminHandler.ResetPassword)
		r.Get("/admin/special", adminHandler.SpecialPage)
		r.Post("/admin/special/resolve", adminHandler.ResolveSpecial)
	})

	// Stats API
	r.Get("/api/stats/ranking-evolution", statsHandler.RankingEvolution)
	r.Get("/api/stats/match-distribution/{id}", statsHandler.MatchDistribution)
	r.Get("/api/stats/user-accuracy", statsHandler.UserAccuracy)
	r.Get("/api/stats/user-points-per-day", statsHandler.UserPointsPerDay)
	r.Get("/api/stats/score-heatmap", statsHandler.ScoreHeatmap)
	r.Get("/api/stats/global-insights", statsHandler.GlobalInsights)
	r.Get("/api/stats/bubble-data", statsHandler.BubbleData)
	r.Get("/api/stats/radar-data", statsHandler.RadarData)

	// Insights page
	r.Get("/insights", statsHandler.InsightsPage)

	log.Printf("Server starting on :%s", cfg.Port)
	if err := http.ListenAndServe(":"+cfg.Port, r); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}
