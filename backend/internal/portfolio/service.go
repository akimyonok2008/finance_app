package portfolio

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/ardakimyonok/finance_app/internal/fx"
	"github.com/ardakimyonok/finance_app/internal/prices"
)

// Service holds the portfolio business logic: validation, ownership
// enforcement, and summary calculation. It depends only on the Repository,
// PriceProvider, and FXProvider interfaces.
type Service struct {
	repo     Repository
	provider prices.PriceProvider
	fx       fx.FXProvider
}

// NewService wires a Service with its repository, price provider, and FX
// provider (for base-currency normalization).
func NewService(repo Repository, provider prices.PriceProvider, fxp fx.FXProvider) *Service {
	return &Service{repo: repo, provider: provider, fx: fxp}
}

// GetOrCreateDefaultPortfolio returns the user's portfolio, creating the default
// one on first access.
func (s *Service) GetOrCreateDefaultPortfolio(userID string) (*Portfolio, error) {
	p, err := s.repo.GetPortfolioByUser(userID)
	if err == nil {
		return p, nil
	}
	if err != ErrPortfolioNotFound {
		return nil, err
	}

	now := time.Now().UTC()
	p = &Portfolio{
		ID:        uuid.NewString(),
		UserID:    userID,
		Name:      DefaultPortfolioName,
		Currency:  "USD",
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := s.repo.CreatePortfolio(p); err != nil {
		return nil, err
	}
	return p, nil
}

// AddPosition validates the input and creates a position in the user's default
// portfolio, LOCKING the baseline at the current market price. The client never
// supplies a price or currency: the backend fetches today's quote and stores
// price + quote currency as the immutable baseline. A fresh position therefore
// starts at exactly 0% gain (portfolio index contribution of 100), and ranked
// performance can only come from market moves after the add — historical buy
// prices do not exist in this model.
func (s *Service) AddPosition(ctx context.Context, userID string, in PositionInput) (*Position, error) {
	clean, quote, err := s.validatePosition(ctx, in)
	if err != nil {
		return nil, err
	}

	pf, err := s.GetOrCreateDefaultPortfolio(userID)
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	pos := &Position{
		ID:              uuid.NewString(),
		UserID:          userID,
		PortfolioID:     pf.ID,
		Symbol:          clean.Symbol,
		AssetType:       clean.AssetType,
		Quantity:        clean.Quantity,
		AverageBuyPrice: quote.Price,    // locked baseline: today's price
		Currency:        quote.Currency, // quote currency of the baseline
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	if err := s.repo.CreatePosition(pos); err != nil {
		return nil, err
	}
	return pos, nil
}

// ListPositions returns the requesting user's positions only.
func (s *Service) ListPositions(userID string) ([]*Position, error) {
	return s.repo.ListPositionsByUser(userID)
}

// UpdatePosition updates the QUANTITY of a position the user owns. The symbol
// and locked baseline price are immutable — re-pricing on edit would let users
// reset a losing baseline, which breaks ranking fairness. To change symbols,
// delete and re-add (which locks a fresh baseline at that day's price).
// Updating a position that does not exist or belongs to another user returns
// ErrPositionNotFound.
func (s *Service) UpdatePosition(_ context.Context, userID, positionID string, quantity float64) (*Position, error) {
	if quantity <= 0 {
		return nil, ErrInvalidQuantity
	}

	existing, err := s.ownedPosition(userID, positionID)
	if err != nil {
		return nil, err
	}

	existing.Quantity = quantity
	existing.UpdatedAt = time.Now().UTC()

	if err := s.repo.UpdatePosition(existing); err != nil {
		return nil, err
	}
	return existing, nil
}

// DeletePosition deletes a position the user owns, else ErrPositionNotFound.
func (s *Service) DeletePosition(userID, positionID string) error {
	if _, err := s.ownedPosition(userID, positionID); err != nil {
		return err
	}
	return s.repo.DeletePosition(positionID)
}

// Summary fetches the user's positions, prices each through the provider, and
// returns the calculated portfolio summary. Any provider failure is wrapped in
// ErrPriceProvider.
func (s *Service) Summary(ctx context.Context, userID string) (*PortfolioSummary, error) {
	pf, err := s.GetOrCreateDefaultPortfolio(userID)
	if err != nil {
		return nil, err
	}

	positions, err := s.repo.ListPositionsByUser(userID)
	if err != nil {
		return nil, err
	}

	summaries := make([]PositionSummary, 0, len(positions))
	for _, pos := range positions {
		price, err := s.provider.GetLatestPrice(ctx, pos.Symbol)
		if err != nil {
			return nil, fmt.Errorf("%w: %s: %v", ErrPriceProvider, pos.Symbol, err)
		}

		costLocal := pos.Quantity * pos.AverageBuyPrice
		valueLocal := pos.Quantity * price.Price
		// Cost basis converts from the position's purchase currency; current
		// value converts from the price's quote currency.
		costBase, err := s.fx.Convert(ctx, costLocal, pos.Currency, fx.BaseCurrency)
		if err != nil {
			return nil, fmt.Errorf("%w: %s: %v", ErrPriceProvider, pos.Symbol, err)
		}
		valueBase, err := s.fx.Convert(ctx, valueLocal, price.Currency, fx.BaseCurrency)
		if err != nil {
			return nil, fmt.Errorf("%w: %s: %v", ErrPriceProvider, pos.Symbol, err)
		}

		summaries = append(summaries, CalculatePositionSummary(pos, price.Price, price.Currency, costBase, valueBase, fx.BaseCurrency))
	}

	summary := CalculatePortfolioSummary(userID, pf.ID, fx.BaseCurrency, summaries)
	return &summary, nil
}

// ownedPosition fetches a position and confirms it belongs to userID. Both a
// missing position and a foreign one collapse to ErrPositionNotFound so the API
// never discloses another user's data.
func (s *Service) ownedPosition(userID, positionID string) (*Position, error) {
	pos, err := s.repo.GetPosition(positionID)
	if err != nil {
		return nil, ErrPositionNotFound
	}
	if pos.UserID != userID {
		return nil, ErrPositionNotFound
	}
	return pos, nil
}

// validatePosition runs format validation, fetches the current quote (which is
// both the priceability gate AND the baseline to lock), and confirms the quote
// currency is FX-convertible. This keeps unpriceable tickers and unknown
// currencies out of the repository so they can never later break summaries or
// leaderboards.
func (s *Service) validatePosition(ctx context.Context, in PositionInput) (PositionInput, *prices.Price, error) {
	clean, err := validateAndNormalize(in)
	if err != nil {
		return PositionInput{}, nil, err
	}
	quote, err := s.provider.GetLatestPrice(ctx, clean.Symbol)
	if err != nil || quote == nil {
		return PositionInput{}, nil, ErrUnsupportedSymbol
	}
	if _, err := s.fx.GetRate(ctx, quote.Currency, fx.BaseCurrency); err != nil {
		return PositionInput{}, nil, ErrUnsupportedCurrency
	}
	return clean, quote, nil
}

// validateAndNormalize checks a PositionInput and returns a normalized copy
// (symbol upper-cased, whitespace trimmed). It enforces the safe-symbol format
// but does NOT check priceability.
func validateAndNormalize(in PositionInput) (PositionInput, error) {
	if strings.TrimSpace(in.Symbol) == "" {
		return PositionInput{}, ErrSymbolRequired
	}
	symbol, err := prices.ValidateAndNormalizeSymbol(in.Symbol)
	if err != nil {
		return PositionInput{}, ErrUnsupportedSymbol
	}
	assetType := strings.ToLower(strings.TrimSpace(in.AssetType))
	if !validAssetTypes[assetType] {
		return PositionInput{}, ErrInvalidAssetType
	}
	if in.Quantity <= 0 {
		return PositionInput{}, ErrInvalidQuantity
	}

	return PositionInput{
		Symbol:    symbol,
		AssetType: assetType,
		Quantity:  in.Quantity,
	}, nil
}
