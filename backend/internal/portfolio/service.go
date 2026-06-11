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
// portfolio. The symbol must pass format checks AND be priceable by the active
// provider, so unpriceable tickers can never enter the repository.
func (s *Service) AddPosition(ctx context.Context, userID string, in PositionInput) (*Position, error) {
	clean, err := s.validatePosition(ctx, in)
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
		AverageBuyPrice: clean.AverageBuyPrice,
		Currency:        clean.Currency,
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

// UpdatePosition validates input and updates a position the user owns. Updating
// a position that does not exist or belongs to another user returns
// ErrPositionNotFound.
func (s *Service) UpdatePosition(ctx context.Context, userID, positionID string, in PositionInput) (*Position, error) {
	clean, err := s.validatePosition(ctx, in)
	if err != nil {
		return nil, err
	}

	existing, err := s.ownedPosition(userID, positionID)
	if err != nil {
		return nil, err
	}

	existing.Symbol = clean.Symbol
	existing.AssetType = clean.AssetType
	existing.Quantity = clean.Quantity
	existing.AverageBuyPrice = clean.AverageBuyPrice
	existing.Currency = clean.Currency
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

// validatePosition runs format validation then confirms the currency is
// FX-convertible and the symbol is priceable by the active provider. This is
// the gate that keeps unpriceable tickers and unknown currencies out of the
// repository (so they can never later break summaries or leaderboards).
func (s *Service) validatePosition(ctx context.Context, in PositionInput) (PositionInput, error) {
	clean, err := validateAndNormalize(in)
	if err != nil {
		return PositionInput{}, err
	}
	if _, err := s.fx.GetRate(ctx, clean.Currency, fx.BaseCurrency); err != nil {
		return PositionInput{}, ErrUnsupportedCurrency
	}
	if _, err := s.provider.GetLatestPrice(ctx, clean.Symbol); err != nil {
		return PositionInput{}, ErrUnsupportedSymbol
	}
	return clean, nil
}

// validateAndNormalize checks a PositionInput and returns a normalized copy
// (symbol and currency upper-cased, whitespace trimmed). It enforces the
// safe-symbol format but does NOT check priceability.
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
	if in.AverageBuyPrice <= 0 {
		return PositionInput{}, ErrInvalidPrice
	}
	currency := strings.ToUpper(strings.TrimSpace(in.Currency))
	if currency == "" {
		return PositionInput{}, ErrCurrencyRequired
	}

	return PositionInput{
		Symbol:          symbol,
		AssetType:       assetType,
		Quantity:        in.Quantity,
		AverageBuyPrice: in.AverageBuyPrice,
		Currency:        currency,
	}, nil
}
