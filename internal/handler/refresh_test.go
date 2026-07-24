package handler

import (
	db "aic3-service/db/postgres/sqlc"
	"aic3-service/internal/lib"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

// fakeRefreshQueries implements only the queries Refresh/issueSession touch.
// Embedding db.Querier means any unexpected call nil-panics the test loudly.
type fakeRefreshQueries struct {
	db.Querier
	sess         db.SelectActiveRefreshTokenRow
	selectErr    error
	revokedHash  string
	revokedAll   bool
	insertedHash string
}

func (f *fakeRefreshQueries) SelectActiveRefreshToken(_ context.Context, _ string) (db.SelectActiveRefreshTokenRow, error) {
	return f.sess, f.selectErr
}
func (f *fakeRefreshQueries) RevokeRefreshTokenByHash(_ context.Context, hash string) error {
	f.revokedHash = hash
	return nil
}
func (f *fakeRefreshQueries) RevokeAllUserRefreshTokens(_ context.Context, _ uuid.UUID) error {
	f.revokedAll = true
	return nil
}
func (f *fakeRefreshQueries) InsertRefreshToken(_ context.Context, arg db.InsertRefreshTokenParams) (db.InsertRefreshTokenRow, error) {
	f.insertedHash = arg.TokenHash
	return db.InsertRefreshTokenRow{ID: uuid.New(), UserID: arg.UserID}, nil
}

func postRefresh(h *AuthHandler, token string) *httptest.ResponseRecorder {
	body, _ := json.Marshal(map[string]string{"refresh_token": token})
	r := httptest.NewRequest(http.MethodPost, "/api/auth/refresh", strings.NewReader(string(body)))
	w := httptest.NewRecorder()
	h.Refresh(w, r)
	return w
}

func TestRefreshRotatesToken(t *testing.T) {
	uid := uuid.New()
	fake := &fakeRefreshQueries{
		sess: db.SelectActiveRefreshTokenRow{UserID: uid, Role: db.UserRoleVIEWER},
	}
	h := &AuthHandler{
		Queries:    fake,
		Signer:     lib.NewJWTSigner("secret", "aic3", 15*time.Minute),
		RefreshTTL: 24 * time.Hour,
	}

	old := "old-refresh-token"
	w := postRefresh(h, old)

	if w.Code != http.StatusOK {
		t.Fatalf("status: got %d want 200 (body %s)", w.Code, w.Body.String())
	}
	// Old token must be revoked by its hash.
	if fake.revokedHash != lib.HashToken(old) {
		t.Errorf("old token not revoked: got %q", fake.revokedHash)
	}
	// A new session must have been inserted with a DIFFERENT hash.
	if fake.insertedHash == "" || fake.insertedHash == lib.HashToken(old) {
		t.Errorf("new session not rotated: inserted=%q", fake.insertedHash)
	}
	// Response must carry a fresh access + refresh token.
	var resp struct {
		Data struct {
			Token   string `json:"token"`
			Refresh string `json:"refresh_token"`
		} `json:"data"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Data.Token == "" || resp.Data.Refresh == "" {
		t.Errorf("missing tokens in response: %s", w.Body.String())
	}
}

func TestRefreshRejectsDeactivatedUser(t *testing.T) {
	uid := uuid.New()
	deleted := time.Now()
	fake := &fakeRefreshQueries{
		sess: db.SelectActiveRefreshTokenRow{UserID: uid, Role: db.UserRoleVIEWER, DeletedAt: &deleted},
	}
	h := &AuthHandler{Queries: fake, Signer: lib.NewJWTSigner("s", "aic3", time.Minute), RefreshTTL: time.Hour}

	w := postRefresh(h, "tok")
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status: got %d want 401", w.Code)
	}
	if !fake.revokedAll {
		t.Error("deactivated user's sessions should be revoked")
	}
}
