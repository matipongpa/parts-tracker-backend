package jwtAuth

import (
	"errors"
	utilis "partlist/src/utilis"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

var jwtSecret = []byte(utilis.GetConfig().JWTSecret)

type Claims struct {
	UserName string `json:"userName"`
	jwt.RegisteredClaims
}

type key string

const UserNameKey key = "userName"

// GenerateToken creates a signed JWT token
func GenerateToken(userName string) (string, error) {
	claims := Claims{
		UserName: userName,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(1 * time.Hour)), // 1 hour expiry
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
	// token from userName
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	// Sign the token with your secret
	signedToken, err := token.SignedString(jwtSecret)
	if err != nil {
		return "", err
	}

	return signedToken, nil
}

// VerifyToken parses and validates a JWT token string
func VerifyToken(tokenString string) (*Claims, error) {
	// Parse the token
	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}
		return jwtSecret, nil
	})
	if err != nil {
		return nil, err
	}

	// Extract claims and validate expiration
	if claims, ok := token.Claims.(*Claims); ok && token.Valid {
		if claims.ExpiresAt.Time.Before(time.Now()) {
			return nil, errors.New("token has expired")
		}
		return claims, nil
	}

	return nil, errors.New("invalid token")
}
