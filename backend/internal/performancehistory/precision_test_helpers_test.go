package performancehistory

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/ardakimyonok/finance_app/internal/money"
)

func testIndex(value string) money.IndexValue { return money.MustIndexValue(value) }

func assertIndexEqual(t *testing.T, expected string, actual money.IndexValue) {
	t.Helper()
	assert.Equal(t, 0, testIndex(expected).Cmp(actual),
		"expected %s, got %s", expected, actual.String())
}
