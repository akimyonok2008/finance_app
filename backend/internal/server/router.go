package server

import (
	"context"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/redis/go-redis/v9"

	"github.com/ardakimyonok/finance_app/internal/achievements"
	"github.com/ardakimyonok/finance_app/internal/auth"
	"github.com/ardakimyonok/finance_app/internal/competitions"
	"github.com/ardakimyonok/finance_app/internal/leaderboard"
	"github.com/ardakimyonok/finance_app/internal/marketdata"
	"github.com/ardakimyonok/finance_app/internal/moderation"
	"github.com/ardakimyonok/finance_app/internal/performancehistory"
	"github.com/ardakimyonok/finance_app/internal/portfolio"
	"github.com/ardakimyonok/finance_app/internal/profile"
	"github.com/ardakimyonok/finance_app/internal/safety"
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
	Safety       *safety.Service
	Moderation   *moderation.Service
	// PerformanceHistory is the CANONICAL ranked-performance snapshot service —
	// the same source the leaderboard and benchmark achievements read. It backs
	// GET /performance/history.
	PerformanceHistory *performancehistory.Service
	// CorporateActionView is the optional read-only automatic-adjustments reader.
	CorporateActionView portfolio.CorporateActionViewReader
	// IncomeEventView is the optional read-only automatic-income reader + the
	// constrained correction path.
	IncomeEventView portfolio.IncomeEventViewReader

	// ReadinessChecks are dependency probes for GET /ready (postgres, redis, ...).
	ReadinessChecks []ReadinessCheck
	// Info is static metadata echoed by GET /ready (storage_provider, ...).
	Info map[string]string

	// AppEnv and CORSAllowedOrigins configure the CORS policy (see corsMiddleware
	// in web.go): production requires an explicit allow-list rather than
	// silently falling back to "*".
	AppEnv             string
	CORSAllowedOrigins []string

	// RateLimitRedis is optional. When set, the auth/account rate limiters
	// (see ratelimit.go) share their budget across every replica via Redis
	// instead of each replica enforcing its own separate in-memory limit.
	RateLimitRedis              *redis.Client
	DisablePasswordRegistration bool
}

// moderationUserAdmin bridges auth.Service to moderation.UserAdmin so the
// router can gate moderation endpoints on backend account state (role,
// suspension, ban) without moderation importing auth directly.
type moderationUserAdmin struct{ auth *auth.Service }

func (m moderationUserAdmin) UserByID(_ context.Context, id string) (moderation.UserView, error) {
	u, err := m.auth.UserByID(id)
	if err != nil {
		return moderation.UserView{}, err
	}
	return moderation.UserView{ID: u.ID, Role: u.Role, IsAdmin: u.IsAdmin()}, nil
}

func (m moderationUserAdmin) Suspend(ctx context.Context, userID string, until *time.Time, reason string) error {
	return m.auth.Suspend(ctx, userID, until, reason)
}

func (m moderationUserAdmin) Ban(ctx context.Context, userID string, reason string) error {
	return m.auth.Ban(ctx, userID, reason)
}

// New builds the application's HTTP router, wiring public auth routes and
// JWT-protected portfolio and price routes.
func New(d Deps) http.Handler {
	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(metricsMiddleware)
	r.Use(corsMiddleware(d.AppEnv, d.CORSAllowedOrigins))

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
	var safetyHandler *safety.Handler
	if d.Safety != nil {
		safetyHandler = safety.NewHandler(d.Safety)
	}
	var moderationHandler *moderation.Handler
	if d.Moderation != nil {
		moderationHandler = moderation.NewHandler(d.Moderation, moderationUserAdmin{d.Auth})
	}
	var performanceHistoryHandler *performancehistory.Handler
	if d.PerformanceHistory != nil {
		performanceHistoryHandler = performancehistory.NewHandler(d.PerformanceHistory)
	}
	var profileHandler *profile.Handler
	if d.Profile != nil {
		profileHandler = profile.NewHandler(d.Profile)
	}

	// Local test UI (development convenience, not a production frontend).
	r.Get("/", serveIndex)

	r.Get("/health", healthHandler)
	r.Get("/ready", readyHandler(d.ReadinessChecks, d.Info))
	r.Handle("/metrics", metricsHandler())

	// Per-IP rate limits. Without them, /auth/login and /auth/register are an
	// unbounded credential-stuffing surface: each guess costs the attacker
	// nothing but forces the server through a deliberately slow bcrypt compare
	// every time. accountLimiter covers change-password/delete-account too —
	// those need a valid JWT already, but a stolen token still shouldn't turn
	// "verify the current password" into an unlimited guessing oracle. Both
	// are Redis-backed when d.RateLimitRedis is set, so every replica behind a
	// load balancer shares one budget instead of each getting its own.
	authLimiter := newRateLimiter(d.RateLimitRedis, "auth", 20, 5*time.Minute)
	// registerLimiter is deliberately much stricter than authLimiter and keyed
	// on a much longer window: it exists to bound how many accounts one IP can
	// create per day, not to stop credential-stuffing (that's authLimiter's
	// job). Without it, the cheapest version of leaderboard "survivorship
	// gaming" — spin up many accounts from one machine, invest each
	// differently, only publicize whichever got lucky — costs an attacker
	// nothing.
	registerLimiter := newRateLimiter(d.RateLimitRedis, "register", 5, 24*time.Hour)
	accountLimiter := newRateLimiter(d.RateLimitRedis, "account", 10, 5*time.Minute)
	recoveryLimiter := newRateLimiter(d.RateLimitRedis, "auth-recovery", 5, 15*time.Minute)
	// Social-safety abuse limits: reporting is rate-limited to deter report
	// spam/harassment-via-reporting; block/unblock is looser since it's a
	// purely defensive, self-directed action; message sending is limited
	// per-authenticated-user to bound unsolicited-message volume even within
	// the friends-only messaging policy. The social service also enforces
	// durable sender/conversation/duplicate/burst limits at write time.
	reportLimiter := newRateLimiter(d.RateLimitRedis, "reports", 5, 15*time.Minute)
	blockLimiter := newRateLimiter(d.RateLimitRedis, "blocks", 30, 5*time.Minute)
	messageLimiter := newRateLimiter(d.RateLimitRedis, "messages", 30, time.Minute)

	// Public auth routes.
	r.Route("/auth", func(r chi.Router) {
		r.Use(rateLimitMiddleware(authLimiter))
		if !d.DisablePasswordRegistration {
			r.With(rateLimitMiddleware(registerLimiter)).Post("/register", authHandler.Register)
		}
		r.Post("/login", authHandler.Login)
		r.Post("/google", authHandler.Google)
		r.Post("/verify-email", authHandler.VerifyEmail)
		r.With(rateLimitMiddleware(recoveryLimiter)).Post("/resend-verification", authHandler.ResendVerification)
		r.With(rateLimitMiddleware(recoveryLimiter)).Post("/forgot-password", authHandler.ForgotPassword)
		r.With(rateLimitMiddleware(recoveryLimiter)).Post("/reset-password", authHandler.ResetPassword)
	})

	// JWT-protected routes. RequireAuthWithUser also rejects valid tokens whose
	// user no longer exists (e.g. after an in-memory restart).
	r.Group(func(r chi.Router) {
		r.Use(auth.RequireAuthWithUser(d.Tokens, d.Auth))

		r.Get("/me", authHandler.Me)
		r.With(rateLimitMiddleware(accountLimiter)).Post("/auth/change-password", authHandler.ChangePassword)
		r.With(rateLimitMiddleware(accountLimiter)).Post("/auth/set-password", authHandler.SetPassword)
		r.With(rateLimitMiddleware(accountLimiter)).Post("/auth/reauthenticate", authHandler.Reauthenticate)
		r.With(rateLimitMiddleware(accountLimiter)).Post("/auth/revoke-sessions", authHandler.RevokeSessions)
		r.With(rateLimitMiddleware(accountLimiter)).Post("/auth/delete-account", authHandler.DeleteAccount)

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
			r.With(rateLimitMiddlewareByKey(messageLimiter, authenticatedUserOrIPKey)).
				Post("/dm/conversations/{conversationId}/messages", socialHandler.SendMessage)
			r.Post("/dm/conversations/{conversationId}/read", socialHandler.MarkRead)
			r.Get("/dm/unread-count", socialHandler.UnreadCount)
			r.Delete("/dm/messages/{messageId}", socialHandler.HideMessage)
		}

		if safetyHandler != nil {
			r.With(rateLimitMiddleware(blockLimiter)).Post("/social/users/{handle}/block", safetyHandler.Block)
			r.With(rateLimitMiddleware(blockLimiter)).Delete("/social/users/{handle}/block", safetyHandler.Unblock)
			r.Get("/me/blocked-users", safetyHandler.BlockedUsers)
		}

		if moderationHandler != nil {
			r.With(rateLimitMiddleware(reportLimiter)).Post("/reports/users/{userId}", moderationHandler.ReportUser)
			r.With(rateLimitMiddleware(reportLimiter)).Post("/reports/messages/{messageId}", moderationHandler.ReportMessage)

			r.Get("/notifications", moderationHandler.Notifications)
			r.Get("/notifications/unread-count", moderationHandler.UnreadNotificationCount)
			r.Post("/notifications/{notificationId}/read", moderationHandler.MarkNotificationRead)
			r.Post("/notifications/read-all", moderationHandler.MarkAllNotificationsRead)

			r.Group(func(r chi.Router) {
				r.Use(moderation.RequireModerator(moderationUserAdmin{d.Auth}))
				r.Get("/moderation/reports", moderationHandler.ListReports)
				r.Get("/moderation/reports/{reportId}", moderationHandler.GetReport)
				r.Post("/moderation/reports/{reportId}/resolve", moderationHandler.ResolveReport)
			})
		}

		r.Get("/portfolio", portfolioHandler.GetPortfolio)
		r.Patch("/portfolio/settings", portfolioHandler.UpdatePortfolioSettings)
		r.Get("/portfolio/summary", portfolioHandler.Summary)
		r.Get("/portfolio/archives", portfolioHandler.Archives)
		r.Post("/portfolio/deposits", portfolioHandler.DepositCash)
		r.Post("/portfolio/withdrawals", portfolioHandler.WithdrawCash)
		r.Post("/portfolio/buys/preview", portfolioHandler.PreviewBuy)
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
		// Narrow escape hatch: only succeeds when the position's symbol has no
		// available market price. A priceable position must be sold instead.
		r.Post("/portfolio/positions/{positionId}/write-off", portfolioHandler.WriteOffPosition)

		r.Get("/activity", portfolioHandler.ActivityList)
		r.Get("/activity/{activityId}", portfolioHandler.ActivityDetail)
		r.Get("/performance/summary", portfolioHandler.PerformanceSummary)
		// Ranked history comes from the canonical ranked snapshot service, NOT
		// from the private portfolio valuation archive (which is value/cost-basis
		// history and is distorted by deposits and withdrawals). The archive is
		// still served separately at /portfolio/archives for the value chart.
		if performanceHistoryHandler != nil {
			r.Get("/performance/history", performanceHistoryHandler.History)
		}

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
