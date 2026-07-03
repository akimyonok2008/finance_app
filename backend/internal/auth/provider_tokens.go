package auth

import (
	"context"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"math/big"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type ProviderVerifier interface {
	Verify(ctx context.Context, token string) (ProviderClaims, error)
}

type OIDCVerifier struct {
	Provider AuthProvider
	ClientID string
	Issuers  map[string]bool
	JWKSURL  string
	Client   *http.Client
}

func NewGoogleVerifier(clientID string) *OIDCVerifier {
	return &OIDCVerifier{
		Provider: ProviderGoogle,
		ClientID: clientID,
		Issuers: map[string]bool{
			"accounts.google.com":         true,
			"https://accounts.google.com": true,
		},
		JWKSURL: "https://www.googleapis.com/oauth2/v3/certs",
		Client:  &http.Client{Timeout: 10 * time.Second},
	}
}

func NewAppleVerifier(clientID string) *OIDCVerifier {
	return &OIDCVerifier{
		Provider: ProviderApple,
		ClientID: clientID,
		Issuers:  map[string]bool{"https://appleid.apple.com": true},
		JWKSURL:  "https://appleid.apple.com/auth/keys",
		Client:   &http.Client{Timeout: 10 * time.Second},
	}
}

func (v *OIDCVerifier) Verify(ctx context.Context, raw string) (ProviderClaims, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || strings.TrimSpace(v.ClientID) == "" {
		return ProviderClaims{}, ErrProviderNotConfigured
	}
	keys, err := v.fetchKeys(ctx)
	if err != nil {
		return ProviderClaims{}, ErrInvalidProviderToken
	}
	claims := jwt.MapClaims{}
	token, err := jwt.ParseWithClaims(raw, claims, func(token *jwt.Token) (any, error) {
		if _, ok := token.Method.(*jwt.SigningMethodRSA); !ok {
			return nil, ErrInvalidProviderToken
		}
		kid, _ := token.Header["kid"].(string)
		key, ok := keys[kid]
		if !ok {
			return nil, ErrInvalidProviderToken
		}
		return key, nil
	}, jwt.WithAudience(v.ClientID), jwt.WithExpirationRequired())
	if err != nil || token == nil || !token.Valid {
		return ProviderClaims{}, ErrInvalidProviderToken
	}
	issuer, _ := claims["iss"].(string)
	if !v.Issuers[issuer] {
		return ProviderClaims{}, ErrInvalidProviderToken
	}
	sub, _ := claims["sub"].(string)
	if sub == "" {
		return ProviderClaims{}, ErrInvalidProviderToken
	}
	email, _ := claims["email"].(string)
	emailVerified := boolClaim(claims["email_verified"])
	if v.Provider == ProviderGoogle && email != "" && !emailVerified {
		return ProviderClaims{}, ErrProviderEmailUnverified
	}
	name, _ := claims["name"].(string)
	if name == "" && v.Provider == ProviderApple {
		name = "Apple Investor"
	}
	picture, _ := claims["picture"].(string)
	return ProviderClaims{
		Provider: v.Provider, Subject: sub, Email: email, EmailVerified: emailVerified,
		DisplayName: name, AvatarKey: picture,
	}, nil
}

func (v *OIDCVerifier) fetchKeys(ctx context.Context) (map[string]*rsa.PublicKey, error) {
	client := v.Client
	if client == nil {
		client = http.DefaultClient
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, v.JWKSURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, errors.New("jwks unavailable")
	}
	var body struct {
		Keys []struct {
			Kid string `json:"kid"`
			Kty string `json:"kty"`
			N   string `json:"n"`
			E   string `json:"e"`
		} `json:"keys"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, err
	}
	out := map[string]*rsa.PublicKey{}
	for _, key := range body.Keys {
		if key.Kty != "RSA" || key.Kid == "" {
			continue
		}
		pub, err := rsaPublicKey(key.N, key.E)
		if err == nil {
			out[key.Kid] = pub
		}
	}
	if len(out) == 0 {
		return nil, errors.New("no jwks keys")
	}
	return out, nil
}

func rsaPublicKey(nRaw, eRaw string) (*rsa.PublicKey, error) {
	nBytes, err := base64.RawURLEncoding.DecodeString(nRaw)
	if err != nil {
		return nil, err
	}
	eBytes, err := base64.RawURLEncoding.DecodeString(eRaw)
	if err != nil {
		return nil, err
	}
	e := 0
	for _, b := range eBytes {
		e = e<<8 + int(b)
	}
	if e == 0 {
		return nil, errors.New("invalid exponent")
	}
	return &rsa.PublicKey{N: new(big.Int).SetBytes(nBytes), E: e}, nil
}

func boolClaim(v any) bool {
	switch x := v.(type) {
	case bool:
		return x
	case string:
		return strings.EqualFold(x, "true")
	default:
		return false
	}
}
