package admin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/codex2api/auth"
	"github.com/codex2api/cache"
	"github.com/codex2api/proxy"
	"github.com/gin-gonic/gin"
)

func TestExchangeOAuthCodeSeedsAccessTokenFromExchangeResponse(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db := newTestAdminDB(t)
	store := auth.NewStore(db, cache.NewMemory(1), nil)
	handler := &Handler{db: db, store: store}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"access_token": "access-from-exchange",
			"refresh_token": "refresh-from-exchange",
			"id_token": "id-from-exchange",
			"expires_in": 3600
		}`))
	}))
	defer server.Close()

	oldResinCfg := proxy.GetResinConfig()
	oldDecorator := auth.ResinRequestDecorator
	proxy.SetResinConfig(&proxy.ResinConfig{BaseURL: server.URL, PlatformName: "codex2api"})
	t.Cleanup(func() {
		proxy.SetResinConfig(oldResinCfg)
		auth.ResinRequestDecorator = oldDecorator
	})

	sessionID := "oauth-test-session"
	globalOAuthStore.set(sessionID, &oauthSession{
		State:        "state-test",
		CodeVerifier: "verifier-test",
		RedirectURI:  oauthDefaultRedirectURI,
		CreatedAt:    time.Now(),
	})
	t.Cleanup(func() {
		globalOAuthStore.delete(sessionID)
	})

	body := `{"session_id":"oauth-test-session","code":"code-test","state":"state-test"}`
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/admin/oauth/exchange-code", strings.NewReader(body))
	ctx.Request.Header.Set("Content-Type", "application/json")

	handler.ExchangeOAuthCode(ctx)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", recorder.Code, http.StatusOK, recorder.Body.String())
	}

	var payload struct {
		ID int64 `json:"id"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.ID == 0 {
		t.Fatal("response id is empty")
	}

	account := store.FindByID(payload.ID)
	if account == nil {
		t.Fatalf("runtime account %d not found", payload.ID)
	}
	account.Mu().RLock()
	gotAccessToken := account.AccessToken
	gotRefreshToken := account.RefreshToken
	account.Mu().RUnlock()
	if gotAccessToken != "access-from-exchange" || gotRefreshToken != "refresh-from-exchange" {
		t.Fatalf("runtime tokens = access:%q refresh:%q, want exchange tokens", gotAccessToken, gotRefreshToken)
	}

	row, err := db.GetAccountByID(context.Background(), payload.ID)
	if err != nil {
		t.Fatalf("GetAccountByID: %v", err)
	}
	if got := row.GetCredential("access_token"); got != "access-from-exchange" {
		t.Fatalf("stored access_token = %q, want exchange access token", got)
	}
	if got := row.GetCredential("id_token"); got != "id-from-exchange" {
		t.Fatalf("stored id_token = %q, want exchange id token", got)
	}
}

func TestReauthorizeOAuthUpdatesExistingAccountWithoutInsertingNewOne(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db := newTestAdminDB(t)
	store := auth.NewStore(db, cache.NewMemory(1), nil)
	handler := &Handler{db: db, store: store, adminSecretEnv: "admin-secret"}

	accessToken := makeAdminTestJWT(t, map[string]interface{}{
		"https://api.openai.com/auth": map[string]interface{}{
			"chatgpt_account_id": "account-new",
			"chatgpt_plan_type":  "pro",
		},
		"https://api.openai.com/profile": map[string]interface{}{
			"email": "new@example.com",
		},
	})
	idToken := makeAdminTestJWT(t, map[string]interface{}{
		"email":              "new@example.com",
		"chatgpt_account_id": "account-new",
		"chatgpt_plan_type":  "pro",
	})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"access_token": "` + accessToken + `",
			"refresh_token": "reauth-refresh",
			"id_token": "` + idToken + `",
			"expires_in": 3600
		}`))
	}))
	defer server.Close()

	oldResinCfg := proxy.GetResinConfig()
	oldDecorator := auth.ResinRequestDecorator
	proxy.SetResinConfig(&proxy.ResinConfig{BaseURL: server.URL, PlatformName: "codex2api"})
	t.Cleanup(func() {
		proxy.SetResinConfig(oldResinCfg)
		auth.ResinRequestDecorator = oldDecorator
	})

	accountID, err := db.InsertAccount(context.Background(), "existing-account", "old-refresh", "")
	if err != nil {
		t.Fatalf("insert account: %v", err)
	}
	if err := db.UpdateCredentials(context.Background(), accountID, map[string]interface{}{
		"access_token": "old-access",
		"id_token":     "old-id",
		"email":        "old@example.com",
		"plan_type":    "free",
		"account_id":   "account-old",
	}); err != nil {
		t.Fatalf("seed credentials: %v", err)
	}

	runtimeAccount := accountFromCredentialSeed(accountID, "", tokenCredentialSeed{
		refreshToken: "old-refresh",
		accessToken:  "old-access",
		idToken:      "old-id",
		email:        "old@example.com",
		planType:     "free",
		accountID:    "account-old",
	})
	runtimeAccount.SetCooldownUntil(time.Now().Add(1*time.Hour), "rate_limited")
	runtimeAccount.ErrorMsg = "stale error"
	store.AddAccount(runtimeAccount)

	sessionID := "reauth-test-session"
	globalOAuthStore.set(sessionID, &oauthSession{
		State:        "state-test",
		CodeVerifier: "verifier-test",
		RedirectURI:  oauthDefaultRedirectURI,
		CreatedAt:    time.Now(),
	})
	t.Cleanup(func() {
		globalOAuthStore.delete(sessionID)
	})

	beforeCount, err := db.CountAll(context.Background())
	if err != nil {
		t.Fatalf("CountAll before: %v", err)
	}

	router := gin.New()
	handler.RegisterRoutes(router)

	body := `{"session_id":"reauth-test-session","code":"code-test","state":"state-test"}`
	req := httptest.NewRequest(http.MethodPost, "/api/admin/accounts/1/oauth/reauthorize", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Admin-Key", "admin-secret")
	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, req)

	if recorder.Code == http.StatusNotFound {
		t.Fatalf("reauthorize endpoint missing: %s", recorder.Body.String())
	}
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", recorder.Code, http.StatusOK, recorder.Body.String())
	}

	afterCount, err := db.CountAll(context.Background())
	if err != nil {
		t.Fatalf("CountAll after: %v", err)
	}
	if afterCount != beforeCount {
		t.Fatalf("account count = %d, want %d", afterCount, beforeCount)
	}

	row, err := db.GetAccountByID(context.Background(), accountID)
	if err != nil {
		t.Fatalf("GetAccountByID: %v", err)
	}
	if got := row.GetCredential("refresh_token"); got != "reauth-refresh" {
		t.Fatalf("stored refresh_token = %q, want reauth refresh token", got)
	}
	if got := row.GetCredential("access_token"); got != accessToken {
		t.Fatalf("stored access_token = %q, want reauth access token", got)
	}
	if got := row.GetCredential("id_token"); got != idToken {
		t.Fatalf("stored id_token = %q, want reauth id token", got)
	}
	if got := row.GetCredential("email"); got != "new@example.com" {
		t.Fatalf("stored email = %q, want new email", got)
	}
	if got := row.GetCredential("plan_type"); got != "pro" {
		t.Fatalf("stored plan_type = %q, want pro", got)
	}
	if got := row.GetCredential("account_id"); got != "account-new" {
		t.Fatalf("stored account_id = %q, want account-new", got)
	}

	acc := store.FindByID(accountID)
	if acc == nil {
		t.Fatalf("runtime account %d not found", accountID)
	}
	acc.Mu().RLock()
	gotRefresh := acc.RefreshToken
	gotAccess := acc.AccessToken
	gotEmail := acc.Email
	gotPlan := acc.PlanType
	gotAccountID := acc.AccountID
	gotStatus := acc.Status
	gotCooldown := acc.CooldownUtil
	gotCooldownReason := acc.CooldownReason
	gotError := acc.ErrorMsg
	acc.Mu().RUnlock()
	if gotRefresh != "reauth-refresh" || gotAccess != accessToken {
		t.Fatalf("runtime tokens = refresh:%q access:%q, want reauth values", gotRefresh, gotAccess)
	}
	if gotEmail != "new@example.com" || gotPlan != "pro" || gotAccountID != "account-new" {
		t.Fatalf("runtime identity = email:%q plan:%q account_id:%q, want updated exchange-derived values", gotEmail, gotPlan, gotAccountID)
	}
	if gotStatus != auth.StatusReady {
		t.Fatalf("runtime status = %v, want ready", gotStatus)
	}
	if !gotCooldown.IsZero() || gotCooldownReason != "" || gotError != "" {
		t.Fatalf("runtime cooldown/error not cleared: cooldown=%v reason=%q error=%q", gotCooldown, gotCooldownReason, gotError)
	}
}

func TestOAuthCallbackReauthorizeUpdatesExistingAccountWithoutInsertingNewOne(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db := newTestAdminDB(t)
	store := auth.NewStore(db, cache.NewMemory(1), nil)
	handler := &Handler{db: db, store: store}

	accessToken := makeAdminTestJWT(t, map[string]interface{}{
		"https://api.openai.com/auth": map[string]interface{}{
			"chatgpt_account_id": "callback-account-new",
			"chatgpt_plan_type":  "plus",
		},
		"https://api.openai.com/profile": map[string]interface{}{
			"email": "callback-new@example.com",
		},
	})
	idToken := makeAdminTestJWT(t, map[string]interface{}{
		"email":              "callback-new@example.com",
		"chatgpt_account_id": "callback-account-new",
		"chatgpt_plan_type":  "plus",
	})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"access_token": "` + accessToken + `",
			"refresh_token": "callback-reauth-refresh",
			"id_token": "` + idToken + `",
			"expires_in": 3600
		}`))
	}))
	defer server.Close()

	oldResinCfg := proxy.GetResinConfig()
	oldDecorator := auth.ResinRequestDecorator
	proxy.SetResinConfig(&proxy.ResinConfig{BaseURL: server.URL, PlatformName: "codex2api"})
	t.Cleanup(func() {
		proxy.SetResinConfig(oldResinCfg)
		auth.ResinRequestDecorator = oldDecorator
	})

	accountID, err := db.InsertAccount(context.Background(), "callback-existing", "callback-old-refresh", "")
	if err != nil {
		t.Fatalf("insert account: %v", err)
	}
	store.AddAccount(accountFromCredentialSeed(accountID, "", tokenCredentialSeed{
		refreshToken: "callback-old-refresh",
		accessToken:  "callback-old-access",
		email:        "callback-old@example.com",
		planType:     "free",
		accountID:    "callback-account-old",
	}))

	sessionID := "callback-reauth-session"
	globalOAuthStore.set(sessionID, &oauthSession{
		State:                "callback-state",
		CodeVerifier:         "callback-verifier",
		RedirectURI:          oauthDefaultRedirectURI,
		ReauthorizeAccountID: accountID,
		CreatedAt:            time.Now(),
	})
	t.Cleanup(func() {
		globalOAuthStore.delete(sessionID)
	})

	beforeCount, err := db.CountAll(context.Background())
	if err != nil {
		t.Fatalf("CountAll before: %v", err)
	}

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/auth/callback?code=callback-code&state=callback-state", nil)

	handler.OAuthCallback(ctx)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", recorder.Code, http.StatusOK, recorder.Body.String())
	}

	afterCount, err := db.CountAll(context.Background())
	if err != nil {
		t.Fatalf("CountAll after: %v", err)
	}
	if afterCount != beforeCount {
		t.Fatalf("account count = %d, want %d", afterCount, beforeCount)
	}

	row, err := db.GetAccountByID(context.Background(), accountID)
	if err != nil {
		t.Fatalf("GetAccountByID: %v", err)
	}
	if got := row.GetCredential("refresh_token"); got != "callback-reauth-refresh" {
		t.Fatalf("stored refresh_token = %q, want callback reauth refresh token", got)
	}
	if got := row.GetCredential("email"); got != "callback-new@example.com" {
		t.Fatalf("stored email = %q, want callback-new@example.com", got)
	}

	acc := store.FindByID(accountID)
	if acc == nil {
		t.Fatalf("runtime account %d not found", accountID)
	}
	acc.Mu().RLock()
	gotRefresh := acc.RefreshToken
	gotEmail := acc.Email
	gotPlan := acc.PlanType
	acc.Mu().RUnlock()
	if gotRefresh != "callback-reauth-refresh" || gotEmail != "callback-new@example.com" || gotPlan != "plus" {
		t.Fatalf("runtime account = refresh:%q email:%q plan:%q, want callback reauth values", gotRefresh, gotEmail, gotPlan)
	}
}
