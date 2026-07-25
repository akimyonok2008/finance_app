package server

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/ardakimyonok/finance_app/internal/achievements"
	"github.com/ardakimyonok/finance_app/internal/auth"
	"github.com/ardakimyonok/finance_app/internal/competitions"
	"github.com/ardakimyonok/finance_app/internal/leaderboard"
	"github.com/ardakimyonok/finance_app/internal/marketdata"
	"github.com/ardakimyonok/finance_app/internal/portfolio"
	"github.com/ardakimyonok/finance_app/internal/profile"
	"github.com/ardakimyonok/finance_app/internal/social"
	"github.com/ardakimyonok/finance_app/internal/strategy"
)

// Deps bundles the constructed services the router needs. Grouping them keeps
// New's signature stable as the app grows.
type Deps struct {
	Auth         *auth.Service
	Tokens       *auth.TokenManager
	Portfolio    *portfolio.Service
	Leaderboard  *leaderboard.Service
	Competitions *competitions.Service
	Achievements *achievements.Service
	Profile      *profile.Service
	Strategy     *strategy.Service
	MarketData   *marketdata.Service
	Social       *social.Service
	// CorporateActionView is the optional read-only automatic-adjustments reader.
	CorporateActionView portfolio.CorporateActionViewReader
	// IncomeEventView is the optional read-only automatic-income reader + the
	// constrained correction path.
	IncomeEventView portfolio.IncomeEventViewReader

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
	if d.CorporateActionView != nil {
		portfolioHandler.SetCorporateActionView(d.CorporateActionView)
	}
	if d.IncomeEventView != nil {
		portfolioHandler.SetIncomeEventView(d.IncomeEventView)
	}
	leaderboardHandler := leaderboard.NewHandler(d.Leaderboard)
	competitionHandler := competitions.NewHandler(d.Competitions, d.Achievements)
	achievementHandler := achievements.NewHandler(d.Achievements)
	strategyHandler := strategy.NewHandler(d.Strategy)
	var marketDataHandler *marketdata.Handler
	if d.MarketData != nil {
		marketDataHandler = marketdata.NewHandler(d.MarketData)
	}
	var socialHandler *social.Handler
	if d.Social != nil {
		socialHandler = social.NewHandler(d.Social)
	}
	var profileHandler *profile.Handler
	if d.Profile != nil {
		profileHandler = profile.NewHandler(d.Profile)
	}

	// Local test UI (development convenience, not a production frontend).
	r.Get("/", serveIndex)

	r.Get("/health", healthHandler)
	r.Get("/ready", readyHandler(d.ReadinessChecks, d.Info))

	// Public auth routes.
	r.Route("/auth", func(r chi.Router) {
		r.Post("/register", authHandler.Register)
		r.Post("/login", authHandler.Login)
		r.Post("/google", authHandler.Google)
	})

	// JWT-protected routes. RequireAuthWithUser also rejects valid tokens whose
	// user no longer exists (e.g. after an in-memory restart).
	r.Group(func(r chi.Router) {
		r.Use(auth.RequireAuthWithUser(d.Tokens, d.Auth))

		r.Get("/me", authHandler.Me)

		if marketDataHandler != nil {
			r.Get("/instruments/search", marketDataHandler.SearchInstruments)
			r.Get("/quotes", marketDataHandler.Quotes)
		}
		if socialHandler != nil {
			r.Post("/social/follows/{handle}", socialHandler.Follow)
			r.Delete("/social/follows/{handle}", socialHandler.Unfollow)
			r.Get("/social/follow-state/{handle}", socialHandler.FollowState)
			r.Get("/social/following", socialHandler.Following)
			r.Get("/social/followers", socialHandler.Followers)
			r.Get("/social/friends", socialHandler.Friends)

			r.Get("/dm/conversations", socialHandler.Conversations)
			r.Post("/dm/conversations", socialHandler.CreateConversation)
			r.Get("/dm/conversations/{conversationId}/messages", socialHandler.Messages)
			r.Post("/dm/conversations/{conversationId}/messages", socialHandler.SendMessage)
		}

		r.Get("/portfolio", portfolioHandler.GetPortfolio)
		r.Get("/portfolio/summary", portfolioHandler.Summary)
		r.Get("/portfolio/archives", portfolioHandler.Archives)
		r.Post("/portfolio/deposits", portfolioHandler.DepositCash)
		r.Post("/portfolio/withdrawals", portfolioHandler.WithdrawCash)
		r.Post("/portfolio/buys", portfolioHandler.BuyPosition)
		r.Post("/portfolio/sells/preview", portfolioHandler.PreviewSell)
		r.Post("/portfolio/sells", portfolioHandler.SellPosition)
		r.Post("/portfolio/fees", portfolioHandler.RecordFee)
		// Ordinary income (dividends, ETF/fund distributions, bond coupons, return
		// of capital, stock dividends) is detected and credited AUTOMATICALLY by the
		// background pipeline. Users cannot create arbitrary income; these endpoints
		// are read-only plus a constrained, account-specific correction path.
		r.Get("/portfolio/income-events", portfolioHandler.ListIncomeEvents)
		r.Get("/portfolio/income-events/{id}", portfolioHandler.GetIncomeEvent)
		r.Post("/portfolio/income-events/{id}/correction", portfolioHandler.CorrectIncomeEvent)
		// Corporate actions are applied automatically by the background pipeline;
		// users cannot record them. This is a read-only, owner-private view.
		r.Get("/portfolio/corporate-actions", portfolioHandler.ListCorporateActions)
		r.Get("/portfolio/cash", portfolioHandler.ListCash)
		r.Get("/portfolio/activities", portfolioHandler.ListActivities)
		r.Post("/portfolio/activities/{id}/correction", portfolioHandler.CorrectActivity)
		r.Get("/portfolio/positions/closed", portfolioHandler.ListClosedPositions)
		r.Get("/portfolio/positions", portfolioHandler.ListPositions)

		r.Get("/activity", portfolioHandler.ActivityList)
		r.Get("/activity/{activityId}", portfolioHandler.ActivityDetail)
		r.Get("/performance/summary", portfolioHandler.PerformanceSummary)
		r.Get("/performance/history", portfolioHandler.Archives)

		r.Get("/leaderboard", leaderboardHandler.GetLeaderboard)
		r.Get("/leaderboard/me", leaderboardHandler.GetMyStanding)

		r.Get("/competitions", competitionHandler.ListCompetitions)
		r.Post("/competitions/{competitionId}/join", competitionHandler.JoinCompetition)
		r.Get("/competitions/{competitionId}/me", competitionHandler.GetMyCompetitionStatus)
		r.Get("/competitions/{competitionId}/leaderboard", competitionHandler.GetCompetitionLeaderboard)

		r.Get("/achievements", achievementHandler.ListAchievements)
		r.Post("/achievements/evaluate", achievementHandler.Evaluate)

		r.Post("/strategy-portfolio/copy-preview", strategyHandler.CopyPreview)
		r.Post("/strategy-portfolio/copy-from-profile", strategyHandler.CopyFromProfile)
		r.Post("/strategy-portfolio/compare-profile", strategyHandler.CompareProfile)

		if profileHandler != nil {
			r.Get("/profiles/me", profileHandler.GetMe)
			r.Patch("/profiles/me", profileHandler.UpdateMe)
			// Static segment registered before {handle} so Explore is never
			// shadowed by the public-profile wildcard.
			r.Get("/profiles/explore", profileHandler.Explore)
			r.Get("/profiles/{handle}", profileHandler.GetPublic)
		}
	})

	return r
}
