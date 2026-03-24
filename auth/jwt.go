package auth

import (
	"errors"
	"net/http"
	"strings"

	"github.com/golang-jwt/jwt/v4"
)

var JWKSecret = []byte("TODO_LOAD_FROM_ENV") // In production this would use the real JWK logic config

type ArenaClaims struct {
	Subs []string `json:"subs"`
	Publ []string `json:"publ"`
	jwt.RegisteredClaims
}

func ValidateMQTTToken(r *http.Request) (*ArenaClaims, error) {
	cookie, err := r.Cookie("mqtt_token")
	if err != nil {
		return nil, errors.New("missing mqtt_token cookie")
	}

	token, err := jwt.ParseWithClaims(cookie.Value, &ArenaClaims{}, func(token *jwt.Token) (interface{}, error) {
		// Verify signature method...
		// In a real JWK setup we would fetch the key ID from headers
		// For simplicity/parity with typical node JWT unless JWK is strictly enforced:
		return JWKSecret, nil
	})

	if err != nil || (!token.Valid) {
		return nil, errors.New("invalid token")
	}

	claims, ok := token.Claims.(*ArenaClaims)
	if !ok {
		return nil, errors.New("invalid claims type")
	}

	return claims, nil
}

// Function to roughly match MQTTPattern
func MatchTopic(pattern, topic string) bool {
	// A simple wildcard matcher for MQTT (+ and #)
	patternParts := strings.Split(pattern, "/")
	topicParts := strings.Split(topic, "/")

	for i, p := range patternParts {
		if p == "#" {
			return true
		}
		if i >= len(topicParts) {
			return false
		}
		if p != "+" && p != topicParts[i] {
			return false
		}
	}
	return len(patternParts) == len(topicParts)
}

func HasSubRight(claims *ArenaClaims, topic string) bool {
	for _, sub := range claims.Subs {
		if MatchTopic(sub, topic) {
			return true
		}
	}
	return false
}

func HasPublRight(claims *ArenaClaims, topic string) bool {
	for _, pub := range claims.Publ {
		if MatchTopic(pub, topic) {
			return true
		}
	}
	return false
}
