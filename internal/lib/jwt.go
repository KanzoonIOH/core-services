package lib

import (
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

type JWTSigner struct {
	Secret []byte
	Issuer string
	Ttl    time.Duration
}

func NewJWTSigner(secret, issuer string, ttl time.Duration) *JWTSigner {
	return &JWTSigner{
		Secret: []byte(secret),
		Issuer: issuer,
		Ttl:    ttl,
	}
}

type JWTClaims struct {
	UserID uuid.UUID `json:"user_id"`
	Role   string    `json:"role"`
	jwt.RegisteredClaims
}

func (s *JWTSigner) Issue(userID uuid.UUID, role string) (string, error) {
	now := time.Now().UTC()
	expiresAt := now.Add(s.Ttl)
	claims := JWTClaims{
		UserID: userID,
		Role:   role,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    s.Issuer,
			Subject:   userID.String(),
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(expiresAt),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(s.Secret)
}

func (s *JWTSigner) Verify(tokenStr string) (*JWTClaims, error) {
	claims := &JWTClaims{}
	parsedToken, err := jwt.ParseWithClaims(tokenStr, claims, func(token *jwt.Token) (any, error) {
		return s.Secret, nil
	}, jwt.WithIssuer(s.Issuer), jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}))
	if err != nil {
		return nil, fmt.Errorf("verify token: %w", err)
	}
	if !parsedToken.Valid {
		return nil, fmt.Errorf("verify token: invalid token")
	}
	if _, err := uuid.Parse(claims.Subject); err != nil {
		return nil, fmt.Errorf("verify token: invalid subject")
	}

	return claims, nil
}
