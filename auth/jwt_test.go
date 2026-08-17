package auth

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v4"
)

// signingKey is the RSA key the package-level cache is seeded with, so
// loadPublicKey() never touches the container path (/app/jwt.public.pem).
// foreignKey is an unrelated key used to forge signatures.
var (
	signingKey *rsa.PrivateKey
	foreignKey *rsa.PrivateKey
)

func TestMain(m *testing.M) {
	var err error
	if signingKey, err = rsa.GenerateKey(rand.Reader, 2048); err != nil {
		panic("could not generate signing key: " + err.Error())
	}
	if foreignKey, err = rsa.GenerateKey(rand.Reader, 2048); err != nil {
		panic("could not generate foreign key: " + err.Error())
	}

	// Consume the sync.Once up front with an in-memory key. This keeps the
	// tests hermetic: no PEM file on disk, no mounted volume, no network.
	pubKeyOnce.Do(func() {
		cachedPubKey = &signingKey.PublicKey
		pubKeyErr = nil
	})

	os.Exit(m.Run())
}

// --- helpers ---

func signToken(t *testing.T, key *rsa.PrivateKey, claims *ArenaClaims) string {
	t.Helper()
	signed, err := jwt.NewWithClaims(jwt.SigningMethodRS256, claims).SignedString(key)
	if err != nil {
		t.Fatalf("could not sign token: %v", err)
	}
	return signed
}

// requestWithToken builds a request carrying the token in the mqtt_token cookie,
// mirroring how the api package receives it from the browser.
func requestWithToken(t *testing.T, token string) *http.Request {
	t.Helper()
	r := httptest.NewRequest(http.MethodPost, "/record/start", nil)
	r.AddCookie(&http.Cookie{Name: "mqtt_token", Value: token})
	return r
}

func testClaims(subject string, subs, publ []string) *ArenaClaims {
	return &ArenaClaims{
		Subs: subs,
		Publ: publ,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   subject,
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
}

// --- ValidateMQTTToken tests ---

func TestValidateMQTTToken_ValidToken(t *testing.T) {
	want := testClaims("alice", []string{"realm/s/alice/#"}, []string{"realm/s/alice/scene/o/+/+"})
	r := requestWithToken(t, signToken(t, signingKey, want))

	claims, err := ValidateMQTTToken(r)
	if err != nil {
		t.Fatalf("expected valid token to be accepted, got error: %v", err)
	}
	if claims == nil {
		t.Fatal("expected claims, got nil")
	}
	if claims.Subject != "alice" {
		t.Errorf("expected subject=alice, got %q", claims.Subject)
	}
	if len(claims.Subs) != 1 || claims.Subs[0] != "realm/s/alice/#" {
		t.Errorf("expected subs=[realm/s/alice/#], got %v", claims.Subs)
	}
	if len(claims.Publ) != 1 || claims.Publ[0] != "realm/s/alice/scene/o/+/+" {
		t.Errorf("expected publ=[realm/s/alice/scene/o/+/+], got %v", claims.Publ)
	}
	if claims.ExpiresAt == nil {
		t.Fatal("expected exp claim to survive round trip, got nil")
	}
	if !claims.ExpiresAt.After(time.Now()) {
		t.Errorf("expected exp in the future, got %v", claims.ExpiresAt.Time)
	}
}

func TestValidateMQTTToken_MissingCookie(t *testing.T) {
	r := httptest.NewRequest(http.MethodPost, "/record/start", nil)

	claims, err := ValidateMQTTToken(r)
	if err == nil {
		t.Fatal("expected error for request without mqtt_token cookie")
	}
	if err.Error() != "missing mqtt_token cookie" {
		t.Errorf("expected %q, got %q", "missing mqtt_token cookie", err.Error())
	}
	if claims != nil {
		t.Errorf("expected nil claims, got %v", claims)
	}
}

func TestValidateMQTTToken_WrongCookieName(t *testing.T) {
	r := httptest.NewRequest(http.MethodPost, "/record/start", nil)
	r.AddCookie(&http.Cookie{Name: "auth_token", Value: signToken(t, signingKey, testClaims("alice", nil, nil))})

	if _, err := ValidateMQTTToken(r); err == nil || err.Error() != "missing mqtt_token cookie" {
		t.Errorf("expected %q, got %v", "missing mqtt_token cookie", err)
	}
}

func TestValidateMQTTToken_RejectsBadTokens(t *testing.T) {
	// A token signed with an unrelated RSA key.
	forged := signToken(t, foreignKey, testClaims("alice", nil, nil))

	// A token whose signature bytes have been altered.
	tampered := signToken(t, signingKey, testClaims("alice", nil, nil))
	if tampered[len(tampered)-1] == 'A' {
		tampered = tampered[:len(tampered)-1] + "B"
	} else {
		tampered = tampered[:len(tampered)-1] + "A"
	}

	// Correct payload, but symmetrically signed: the keyfunc must refuse any
	// non-RSA signing method rather than treat the HMAC secret as a key.
	hmacToken, err := jwt.NewWithClaims(jwt.SigningMethodHS256, testClaims("alice", nil, nil)).SignedString([]byte("shared-secret"))
	if err != nil {
		t.Fatalf("could not sign hmac token: %v", err)
	}

	// Hand-rolled alg=none token (classic algorithm-confusion attempt).
	noneToken := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none","typ":"JWT"}`)) + "." +
		base64.RawURLEncoding.EncodeToString([]byte(`{"sub":"alice","publ":["realm/s/alice/#"]}`)) + "."

	expired := testClaims("alice", nil, nil)
	expired.ExpiresAt = jwt.NewNumericDate(time.Now().Add(-time.Hour))
	expired.IssuedAt = jwt.NewNumericDate(time.Now().Add(-2 * time.Hour))

	notYetValid := testClaims("alice", nil, nil)
	notYetValid.NotBefore = jwt.NewNumericDate(time.Now().Add(time.Hour))

	tests := []struct {
		name  string
		token string
	}{
		{"empty token", ""},
		{"garbage", "not-a-jwt"},
		{"wrong segment count", "aaaa.bbbb"},
		{"undecodable segments", "aaaa.bbbb.cccc"},
		{"signed with wrong key", forged},
		{"tampered signature", tampered},
		{"hmac signing method", hmacToken},
		{"none signing method", noneToken},
		{"expired", signToken(t, signingKey, expired)},
		{"not yet valid", signToken(t, signingKey, notYetValid)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			claims, err := ValidateMQTTToken(requestWithToken(t, tt.token))
			if err == nil {
				t.Fatalf("expected token to be rejected, got claims %v", claims)
			}
			if err.Error() != "invalid token or signature" {
				t.Errorf("expected %q, got %q", "invalid token or signature", err.Error())
			}
			if claims != nil {
				t.Errorf("expected nil claims, got %v", claims)
			}
		})
	}
}

// TestValidateMQTTToken_NoExpiryAccepted documents current behavior: exp is not
// a required claim, so a token that never expires is accepted.
func TestValidateMQTTToken_NoExpiryAccepted(t *testing.T) {
	claims := testClaims("alice", nil, nil)
	claims.ExpiresAt = nil

	got, err := ValidateMQTTToken(requestWithToken(t, signToken(t, signingKey, claims)))
	if err != nil {
		t.Fatalf("expected token without exp to be accepted, got error: %v", err)
	}
	if got.ExpiresAt != nil {
		t.Errorf("expected nil exp, got %v", got.ExpiresAt.Time)
	}
}

func TestValidateMQTTToken_EmptyPermissionArrays(t *testing.T) {
	got, err := ValidateMQTTToken(requestWithToken(t, signToken(t, signingKey, testClaims("bob", []string{}, []string{}))))
	if err != nil {
		t.Fatalf("expected token to be accepted, got error: %v", err)
	}
	if len(got.Subs) != 0 {
		t.Errorf("expected no subs, got %v", got.Subs)
	}
	if len(got.Publ) != 0 {
		t.Errorf("expected no publ, got %v", got.Publ)
	}
	if HasSubRight(got, "realm/s/bob/scene/o/+/+") {
		t.Error("expected no sub rights for empty subs")
	}
}

// --- MatchTopic tests ---

func TestMatchTopic(t *testing.T) {
	tests := []struct {
		name    string
		pattern string
		topic   string
		want    bool
	}{
		{"exact match", "realm/s/ns/scene/o/client/obj", "realm/s/ns/scene/o/client/obj", true},
		{"exact mismatch", "realm/s/ns/scene/o/client/obj", "realm/s/ns/scene/o/client/other", false},
		{"single level wildcard", "realm/s/ns/scene/o/+/+", "realm/s/ns/scene/o/client/obj", true},
		{"single level wildcard does not span levels", "realm/s/ns/+", "realm/s/ns/scene/o", false},
		{"multi level wildcard", "realm/s/ns/#", "realm/s/ns/scene/o/client/obj", true},
		{"multi level wildcard matches parent level", "realm/s/ns/#", "realm/s/ns", true},
		{"root multi level wildcard matches everything", "#", "realm/s/ns/scene/o/client/obj", true},
		{"mixed wildcards", "realm/+/ns/#", "realm/s/ns/scene/o", true},
		{"prefix mismatch before wildcard", "realm/s/other/#", "realm/s/ns/scene", false},
		{"topic shorter than pattern", "realm/s/ns/scene", "realm/s/ns", false},
		{"topic longer than pattern", "realm/s/ns", "realm/s/ns/scene", false},
		{"pattern longer with literal after wildcard", "realm/+/ns/scene", "realm/s/ns", false},
		{"empty pattern and topic", "", "", true},
		{"empty pattern non empty topic", "", "realm", false},
		{"wildcard in topic matched literally", "realm/s/ns/scene/o/client/obj", "realm/s/ns/scene/o/+/+", false},
		{"wildcard in topic matched by wildcard pattern", "realm/s/ns/scene/o/+/+", "realm/s/ns/scene/o/+/+", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := MatchTopic(tt.pattern, tt.topic); got != tt.want {
				t.Errorf("MatchTopic(%q, %q) = %v, expected %v", tt.pattern, tt.topic, got, tt.want)
			}
		})
	}
}

// --- Permission tests ---

func TestHasSubRight(t *testing.T) {
	tests := []struct {
		name  string
		subs  []string
		topic string
		want  bool
	}{
		{"nil subs", nil, "realm/s/ns/scene/o/client/obj", false},
		{"exact grant", []string{"realm/s/ns/scene/o/client/obj"}, "realm/s/ns/scene/o/client/obj", true},
		{"scene wildcard grant", []string{"realm/s/ns/scene/#"}, "realm/s/ns/scene/o/client/obj", true},
		{"grant for another scene", []string{"realm/s/ns/other/#"}, "realm/s/ns/scene/o/client/obj", false},
		{"grant for another namespace", []string{"realm/s/other/scene/#"}, "realm/s/ns/scene/o/client/obj", false},
		{"second grant matches", []string{"realm/s/other/#", "realm/s/ns/#"}, "realm/s/ns/scene/o/client/obj", true},
		{"no grant matches", []string{"realm/s/a/#", "realm/s/b/#"}, "realm/s/ns/scene/o/client/obj", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			claims := &ArenaClaims{Subs: tt.subs}
			if got := HasSubRight(claims, tt.topic); got != tt.want {
				t.Errorf("HasSubRight(%v, %q) = %v, expected %v", tt.subs, tt.topic, got, tt.want)
			}
			// Sub grants must not leak into publish rights.
			if HasPublRight(claims, tt.topic) {
				t.Errorf("expected no publ right from subs %v", tt.subs)
			}
		})
	}
}

func TestHasPublRight(t *testing.T) {
	tests := []struct {
		name  string
		publ  []string
		topic string
		want  bool
	}{
		{"nil publ", nil, "realm/s/ns/scene/o/client/obj", false},
		{"empty publ", []string{}, "realm/s/ns/scene/o/client/obj", false},
		{"exact grant", []string{"realm/s/ns/scene/o/client/obj"}, "realm/s/ns/scene/o/client/obj", true},
		{"client wildcard grant", []string{"realm/s/ns/scene/o/+/+"}, "realm/s/ns/scene/o/client/obj", true},
		{"admin grant", []string{"#"}, "realm/s/ns/scene/o/client/obj", true},
		{"scene hash grant matches hash topic", []string{"realm/s/ns/scene/#"}, "realm/s/ns/scene/#", true},
		{"other client denied", []string{"realm/s/ns/scene/o/alice/+"}, "realm/s/ns/scene/o/bob/obj", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			claims := &ArenaClaims{Publ: tt.publ}
			if got := HasPublRight(claims, tt.topic); got != tt.want {
				t.Errorf("HasPublRight(%v, %q) = %v, expected %v", tt.publ, tt.topic, got, tt.want)
			}
			// Publish grants must not leak into subscribe rights.
			if HasSubRight(claims, tt.topic) {
				t.Errorf("expected no sub right from publ %v", tt.publ)
			}
		})
	}
}

// --- CanRecordScene tests ---

func TestCanRecordScene(t *testing.T) {
	const (
		namespace = "ns"
		sceneID   = "scene"
	)

	tests := []struct {
		name string
		publ []string
		want bool
	}{
		{"nil publ", nil, false},
		{"admin hash", []string{"#"}, true},
		{"realm scene hash", []string{"realm/s/#"}, true},
		{"namespace hash", []string{"realm/s/ns/#"}, true},
		{"scene hash", []string{"realm/s/ns/scene/#"}, true},
		{"scene objects wildcard", []string{"realm/s/ns/scene/o/+/+"}, true},
		{"namespace wildcard grant", []string{"realm/s/+/+/o/+/+"}, true},
		{"per client grant", []string{"realm/s/ns/scene/o/alice/+"}, true},
		{"per client grant among others", []string{"realm/s/other/#", "realm/s/ns/scene/o/alice/+"}, true},
		{"other namespace", []string{"realm/s/other/scene/#"}, false},
		{"other scene", []string{"realm/s/ns/other/#"}, false},
		{"other namespace per client", []string{"realm/s/other/scene/o/alice/+"}, false},
		{"presence only grant", []string{"realm/s/ns/scene/x/+/+"}, false},
		{"single object grant", []string{"realm/s/ns/scene/o/alice/box"}, false},
		{"topic too short for client index", []string{"realm/s/ns/scene/o"}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			claims := &ArenaClaims{Publ: tt.publ}
			if got := CanRecordScene(claims, namespace, sceneID); got != tt.want {
				t.Errorf("CanRecordScene(%v, %q, %q) = %v, expected %v", tt.publ, namespace, sceneID, got, tt.want)
			}
		})
	}
}

// TestCanRecordScene_UsesClaimsFromToken exercises the api package's real flow:
// validate the cookie, then authorize the scene from the returned claims.
func TestCanRecordScene_UsesClaimsFromToken(t *testing.T) {
	token := signToken(t, signingKey, testClaims("alice", nil, []string{"realm/s/alice/myscene/o/+/+"}))

	claims, err := ValidateMQTTToken(requestWithToken(t, token))
	if err != nil {
		t.Fatalf("expected valid token, got error: %v", err)
	}
	if !CanRecordScene(claims, "alice", "myscene") {
		t.Error("expected publish rights to alice/myscene to allow recording")
	}
	if CanRecordScene(claims, "alice", "otherscene") {
		t.Error("expected no recording rights on alice/otherscene")
	}
	if CanRecordScene(claims, "bob", "myscene") {
		t.Error("expected no recording rights on bob/myscene")
	}
}
