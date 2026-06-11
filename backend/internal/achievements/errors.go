package achievements

import "errors"

// ErrAchievementNotFound is returned when a key does not match a seeded badge.
var ErrAchievementNotFound = errors.New("achievement not found")
