package auth

import (
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v4"
	"github.com/google/uuid"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/authz"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/config"
)

type Claims struct {
	UserID         uuid.UUID `json:"user_id"`
	OrganizationID uuid.UUID `json:"organization_id"`
	Role           string    `json:"role"`
	jwt.RegisteredClaims
}

type TokenManager struct {
	cfg config.JWTConfig
}

func NewTokenManager(cfg config.JWTConfig) *TokenManager {
	return &TokenManager{cfg: cfg}
}

func (m *TokenManager) Issue(userID, organizationID uuid.UUID, role authz.Role) (string, error) {
	now := time.Now().UTC()
	claims := Claims{
		UserID:         userID,
		OrganizationID: organizationID,
		Role:           role.String(),
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    m.cfg.Issuer,
			Subject:   userID.String(),
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(m.cfg.TTL())),
		},
	}

	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(m.cfg.Secret))
}

func (m *TokenManager) Parse(raw string) (Claims, error) {
	claims := Claims{}
	token, err := jwt.ParseWithClaims(raw, &claims, func(token *jwt.Token) (interface{}, error) {
		if token.Method != jwt.SigningMethodHS256 {
			return nil, errors.New("unexpected signing method")
		}
		return []byte(m.cfg.Secret), nil
	})
	if err != nil {
		return Claims{}, err
	}
	if !token.Valid {
		return Claims{}, errors.New("invalid token")
	}
	if !authz.IsValid(claims.Role) {
		return Claims{}, errors.New("invalid role")
	}
	return claims, nil
}
