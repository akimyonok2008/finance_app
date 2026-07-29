package httpx

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDecodeJSONAcceptsOneValueAndWhitespace(t *testing.T) {
	req := httptest.NewRequest("POST", "/", strings.NewReader(`{"name":"Ada"}  `))
	var dst struct {
		Name string `json:"name"`
	}
	require.NoError(t, DecodeJSON(req, &dst))
}

func TestDecodeJSONRejectsTrailingJSONValue(t *testing.T) {
	req := httptest.NewRequest("POST", "/", strings.NewReader(`{"name":"Ada"} {"name":"Eve"}`))
	var dst struct {
		Name string `json:"name"`
	}
	require.Error(t, DecodeJSON(req, &dst))
}
