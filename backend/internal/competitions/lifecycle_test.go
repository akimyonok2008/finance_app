package competitions

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateLifecycleTransition_AllowsTheDocumentedPath(t *testing.T) {
	path := []string{
		LifecycleDraft, LifecyclePublished, LifecycleRegistrationOpen,
		LifecycleRegistrationClosed, LifecycleActive, LifecycleFinalizing, LifecycleCompleted,
	}
	for i := 0; i < len(path)-1; i++ {
		assert.NoError(t, ValidateLifecycleTransition(path[i], path[i+1]),
			"%s -> %s must be allowed", path[i], path[i+1])
	}
	// Every non-terminal state may cancel.
	for _, from := range path[:len(path)-1] {
		assert.NoError(t, ValidateLifecycleTransition(from, LifecycleCancelled),
			"%s -> cancelled must be allowed", from)
	}
}

func TestValidateLifecycleTransition_RejectsInvalidMoves(t *testing.T) {
	invalid := [][2]string{
		{LifecycleCompleted, LifecycleActive},    // completed may never return to active
		{LifecycleCompleted, LifecycleFinalizing},
		{LifecycleCancelled, LifecyclePublished}, // cancelled is terminal
		{LifecycleActive, LifecycleDraft},        // no state re-enters draft
		{LifecyclePublished, LifecycleDraft},
		{LifecycleActive, LifecycleCompleted},    // must pass through finalizing
		{LifecycleDraft, LifecycleActive},        // no skipping registration
		{LifecycleRegistrationOpen, LifecycleActive},
	}
	for _, tc := range invalid {
		err := ValidateLifecycleTransition(tc[0], tc[1])
		require.ErrorIs(t, err, ErrInvalidLifecycleTransition, "%s -> %s must be rejected", tc[0], tc[1])
	}
}

func TestValidateLifecycleTransition_LegacyAndUnknownStatesNeverTransition(t *testing.T) {
	assert.ErrorIs(t, ValidateLifecycleTransition(LifecycleLegacy, LifecyclePublished),
		ErrInvalidLifecycleTransition, "legacy rows are outside the engine's state machine")
	assert.ErrorIs(t, ValidateLifecycleTransition("bogus", LifecyclePublished), ErrInvalidLifecycleTransition)
}
