package profile

import "context"

type Repository interface {
	Create(ctx context.Context, profile Profile) error
	GetByUserID(ctx context.Context, userID string) (Profile, error)
	GetByHandle(ctx context.Context, handle string) (Profile, error)
	Update(ctx context.Context, profile Profile) error
	// ListPublicProfiles is used by the background Explore projection refresh
	// and by the in-memory/cold-start fallback. Production Explore requests use
	// the optional exploreProjectionRepository capability instead.
	ListPublicProfiles(ctx context.Context) ([]Profile, error)
}
