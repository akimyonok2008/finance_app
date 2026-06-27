package profile

import "context"

type Repository interface {
	Create(ctx context.Context, profile Profile) error
	GetByUserID(ctx context.Context, userID string) (Profile, error)
	GetByHandle(ctx context.Context, handle string) (Profile, error)
	Update(ctx context.Context, profile Profile) error
	// ListPublicProfiles returns every profile with is_public=true. The Explore
	// service does the filtering/sorting/pagination in memory for prototype
	// scale. TODO: push filtering + server-side pagination into the query once
	// the public-profile set outgrows a single page fetch.
	ListPublicProfiles(ctx context.Context) ([]Profile, error)
}
