package auth

import (
	"errors"
	"net/http"
	"os"
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
		// Ensure token algorithm is RSA
		if _, ok := token.Method.(*jwt.SigningMethodRSA); !ok {
			return nil, errors.New("unexpected signing method")
		}

		keyBytes, err := os.ReadFile("jwt.public.pem") // mounted from docker-compose
		if err != nil {
			return nil, errors.New("could not read jwt public key")
		}

		pubKey, err := jwt.ParseRSAPublicKeyFromPEM(keyBytes)
		if err != nil {
			return nil, errors.New("could not parse rsa public key")
		}

		return pubKey, nil
	})

	if err != nil || !token.Valid {
		return nil, errors.New("invalid token or signature")
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

// CanRecordScene verifies if the user has publish rights to scene objects,
// mirroring the arena-persist checkTokenRights logic for SCENE_OBJECTS.
func CanRecordScene(claims *ArenaClaims, namespace, sceneId string) bool {
	// 1. Check wildcards first (e.g., admin or whole scene)
	topicAny := "realm/s/" + namespace + "/" + sceneId + "/o/+/+"
	for _, pub := range claims.Publ {
		if MatchTopic(pub, topicAny) {
			return true
		}
	}

	// 2. Check per-client permissions
	for _, pub := range claims.Publ {
		parts := strings.Split(pub, "/")
		if len(parts) > 5 {
			clientId := parts[5]
			topicClient := "realm/s/" + namespace + "/" + sceneId + "/o/" + clientId + "/+"
			if MatchTopic(pub, topicClient) {
				return true
			}
		}
	}
	return false
}
