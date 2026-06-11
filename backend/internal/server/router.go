package server

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/ardakimyonok/finance_app/internal/achievements"
	"github.com/ardakimyonok/finance_app/internal/auth"
	"github.com/ardakimyonok/finance_app/internal/competitions"
	"github.com/ardakimyonok/finance_app/internal/leaderboard"
	"github.com/ardakimyonok/finance_app/internal/portfolio"
	"github.com/ardakimyonok/finance_app/internal/prices"
)

// Deps bundles the constructed services the router needs. Grouping them keeps
// New's signature stable as the app grows.
type Deps struct {
	Auth          *auth.Service
	Tokens        *auth.TokenManager
	Portfolio     *portfolio.Service
	Leaderboard   *leaderboard.Service
	Competitions  *competitions.Service
	Achievements  *achievements.Service
	PriceProvider prices.PriceProvider

	// ReadinessChecks are dependency probes for GET /ready (postgres, redis, ...).
	ReadinessChecks []ReadinessCheck
	// Info is static metadata echoed by GET /ready (storage_provider, ...).
	Info map[string]string
}

// New builds the application's HTTP router, wiring public auth routes and
// JWT-protected portfolio and price routes.
func New(d Deps) http.Handler {
	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(devCORS)

	authHandler := auth.NewHandler(d.Auth)
	portfolioHandler := portfolio.NewHandler(d.Portfolio)
	portfolioHandler.SetAchievementEvaluator(d.Achievements) // trigger badges on add/summary
	priceHandler := prices.NewHandler(d.PriceProvider)
	leaderboardHandler := leaderboard.NewHandler(d.Leaderboard)
	competitionHandler := competitions.NewHandler(d.Competitions, d.Achievements)
	achievementHandler := achievements.NewHandler(d.Achievements)

	// Local test UI (development convenience, not a production frontend).
	r.Get("/", serveIndex)

	r.Get("/health", healthHandler)
	r.Get("/ready", readyHandler(d.ReadinessChecks, d.Info))

	// Public auth routes.
	r.Route("/auth", func(r chi.Router) {
		r.Post("/register", authHandler.Register)
		r.Post("/login", authHandler.Login)
	})

	// JWT-protected routes. RequireAuthWithUser also rejects valid tokens whose
	// user no longer exists (e.g. after an in-memory restart).
	r.Group(func(r chi.Router) {
		r.Use(auth.RequireAuthWithUser(d.Tokens, d.Auth))

		r.Get("/me", authHandler.Me)

		r.Get("/portfolio", portfolioHandler.GetPortfolio)
		r.Get("/portfolio/summary", portfolioHandler.Summary)
		r.Post("/portfolio/positions", portfolioHandler.AddPosition)
		r.Get("/portfolio/positions", portfolioHandler.ListPositions)
		r.Put("/portfolio/positions/{positionId}", portfolioHandler.UpdatePosition)
		r.Delete("/portfolio/positions/{positionId}", portfolioHandler.DeletePosition)

		r.Get("/prices/{symbol}", priceHandler.GetPrice)

		r.Get("/leaderboard", leaderboardHandler.GetLeaderboard)

		r.Get("/competitions", competitionHandler.ListCompetitions)
		r.Post("/competitions/{competitionId}/join", competitionHandler.JoinCompetition)
		r.Get("/competitions/{competitionId}/me", competitionHandler.GetMyCompetitionStatus)
		r.Get("/competitions/{competitionId}/leaderboard", competitionHandler.GetCompetitionLeaderboard)

		r.Get("/achievements", achievementHandler.ListAchievements)
		r.Post("/achievements/evaluate", achievementHandler.Evaluate)
	})

	return r
}
