package profile

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"
)

var (
	ErrNotFound     = errors.New("profile not found")
	ErrHandleExists = errors.New("handle already exists")
	ErrInvalid      = errors.New("invalid profile")

	handlePattern = regexp.MustCompile(`^[a-z0-9_-]{3,30}$`)
	invalidHandle = regexp.MustCompile(`[^a-z0-9_-]+`)
	repeatedSep   = regexp.MustCompile(`[_-]{2,}`)
)

var reservedHandles = map[string]struct{}{
	"me": {}, "admin": {}, "api": {}, "auth": {}, "portfolio": {},
	"leaderboard": {}, "competitions": {}, "achievements": {},
	"profile": {}, "profiles": {}, "settings": {}, "explore": {},
	"coach": {}, "ai": {},
}

var allowedStrategyTags = map[string]struct{}{
	"conservative": {}, "balanced_global": {}, "growth": {},
	"dividend_income": {}, "tech_focused": {}, "value": {},
	"crypto_heavy": {}, "esg": {}, "active_trader": {},
	"long_term_investor": {},
}

func ValidateProfile(p Profile) error {
	if err := ValidateHandle(p.Handle); err != nil {
		return err
	}
	if err := validateLength("display_name", strings.TrimSpace(p.DisplayName), 2, 40); err != nil {
		return err
	}
	if utf8.RuneCountInString(strings.TrimSpace(p.Bio)) > 160 {
		return invalid("bio must be at most 160 characters")
	}
	if utf8.RuneCountInString(strings.TrimSpace(p.AvatarKey)) > 40 {
		return invalid("avatar_key must be at most 40 characters")
	}
	if _, ok := allowedStrategyTags[p.StrategyTag]; !ok {
		return invalid("invalid strategy_tag")
	}
	return nil
}

func ValidateHandle(handle string) error {
	if handle != strings.ToLower(handle) || !handlePattern.MatchString(handle) {
		return invalid("handle must be 3-30 lower-case URL-safe characters")
	}
	if _, reserved := reservedHandles[handle]; reserved {
		return invalid("handle is reserved")
	}
	return nil
}

func NormalizeInput(input UpdateInput) UpdateInput {
	input.Handle = trimPointer(input.Handle)
	input.DisplayName = trimPointer(input.DisplayName)
	input.AvatarKey = trimPointer(input.AvatarKey)
	input.Bio = trimPointer(input.Bio)
	input.StrategyTag = trimPointer(input.StrategyTag)
	if input.Handle != nil {
		normalized := strings.ToLower(*input.Handle)
		input.Handle = &normalized
	}
	return input
}

func HandleCandidate(value, fallback string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = invalidHandle.ReplaceAllString(value, "_")
	value = repeatedSep.ReplaceAllString(value, "_")
	value = strings.Trim(value, "_-")
	if len(value) > 24 {
		value = value[:24]
	}
	if len(value) < 3 || isReserved(value) {
		value = "investor_" + shortIdentifier(fallback)
	}
	if len(value) > 30 {
		value = value[:30]
	}
	return value
}

func HandleWithSuffix(base string, attempt int) string {
	suffix := fmt.Sprintf("_%d", attempt)
	if len(base)+len(suffix) > 30 {
		base = base[:30-len(suffix)]
	}
	return base + suffix
}

func isReserved(handle string) bool {
	_, ok := reservedHandles[handle]
	return ok
}

func shortIdentifier(value string) string {
	value = invalidHandle.ReplaceAllString(strings.ToLower(value), "")
	if len(value) > 8 {
		return value[len(value)-8:]
	}
	if len(value) < 3 {
		return "user"
	}
	return value
}

func trimPointer(value *string) *string {
	if value == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*value)
	return &trimmed
}

func validateLength(name, value string, min, max int) error {
	length := utf8.RuneCountInString(value)
	if length < min || length > max {
		return invalid(fmt.Sprintf("%s must be %d-%d characters", name, min, max))
	}
	return nil
}

func invalid(message string) error {
	return fmt.Errorf("%w: %s", ErrInvalid, message)
}

func truncate(value string, max int) string {
	runes := []rune(value)
	if len(runes) > max {
		runes = runes[:max]
	}
	return string(runes)
}
