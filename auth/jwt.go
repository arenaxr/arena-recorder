package auth

import (
	"crypto/rsa"
	"errors"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"

	"github.com/arenaxr/arena-recorder/mqtt"
	"github.com/golang-jwt/jwt/v4"
)

// Cached RSA public key, parsed once at first use via sync.Once
var (
	cachedPubKey *rsa.PublicKey
	pubKeyOnce   sync.Once
	pubKeyErr    error
)

// loadPublicKey reads and parses the RSA public key from the mounted PEM file.
// Called once via sync.Once to avoid redundant disk I/O and parsing on every request.
func loadPublicKey() (*rsa.PublicKey, error) {
	pubKeyOnce.Do(func() {
		keyPath := "/app/jwt.public.pem" // mounted from docker-compose
		keyBytes, err := os.ReadFile(keyPath)
		if err != nil {
			pubKeyErr = errors.New("could not read jwt public key")
			log.Printf("Error: Failed to read JWT public key from %s: %v", keyPath, err)
			return
		}

		cachedPubKey, pubKeyErr = jwt.ParseRSAPublicKeyFromPEM(keyBytes)
		if pubKeyErr != nil {
			pubKeyErr = errors.New("could not parse rsa public key")
			log.Printf("Error: Failed to parse RSA public key: %v", pubKeyErr)
			return
		}
		log.Println("Loaded and cached RSA public key for JWT validation")
	})
	return cachedPubKey, pubKeyErr
}

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

		return loadPublicKey()
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

// MatchTopic performs a simple wildcard match for MQTT topic patterns (+ and #)
func MatchTopic(pattern, topic string) bool {
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
	topicArgs := map[string]string{
		"nameSpace":  namespace,
		"sceneName":  sceneId,
		"userClient": "+",
		"objectId":   "+",
	}
	// 1. Check wildcards first (e.g., admin or whole scene)
	topicAny := mqtt.FormatTopic(mqtt.Topics.Publish.SceneObjects, topicArgs)
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
			topicArgs["userClient"] = clientId
			topicClient := mqtt.FormatTopic(mqtt.Topics.Publish.SceneObjects, topicArgs)
			if MatchTopic(pub, topicClient) {
				return true
			}
		}
	}
	return false
}
