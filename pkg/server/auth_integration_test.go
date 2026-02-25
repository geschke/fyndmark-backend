package server_test

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/geschke/fyndmark/config"
	"github.com/geschke/fyndmark/pkg/controller"
	"github.com/geschke/fyndmark/pkg/db"
	"github.com/geschke/fyndmark/pkg/users"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/sessions"
)

// TestAuthLoginLogoutFlow tests the expected behavior of this component.
func TestAuthLoginLogoutFlow(t *testing.T) {
	gin.SetMode(gin.TestMode)

	oldCfg := config.Cfg
	t.Cleanup(func() { config.Cfg = oldCfg })

	config.Cfg.WebAdmin.Enabled = true
	config.Cfg.WebAdmin.SessionKey = strings.Repeat("k", 32)
	config.Cfg.WebAdmin.SessionName = "fyndmark_session"
	config.Cfg.WebAdmin.CookieSecure = false
	config.Cfg.WebAdmin.CookieSameSite = "lax"
	config.Cfg.WebAdmin.CookieMaxAgeDays = 30
	config.Cfg.WebAdmin.CORSAllowedOrigins = []string{"http://localhost:3000"}

	dbPath := filepath.Join(t.TempDir(), "auth-it.sqlite")
	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })

	if err := database.Migrate(); err != nil {
		t.Fatalf("migrate db: %v", err)
	}

	_, err = users.Create(context.Background(), database, users.CreateParams{
		Email:     "admin@example.com",
		Password:  "Secret123!",
		FirstName: "Ada",
		LastName:  "Lovelace",
	})
	if err != nil {
		t.Fatalf("seed user: %v", err)
	}

	store := sessions.NewCookieStore([]byte(config.Cfg.WebAdmin.SessionKey))
	authCtl := controller.NewAuthController(database, store, config.Cfg.WebAdmin.SessionName)
	usersCtl := controller.NewUsersController(database, store, config.Cfg.WebAdmin.SessionName)

	r := gin.New()
	r.POST("/api/auth/login", authCtl.PostLogin)
	r.POST("/api/auth/logout", authCtl.PostLogout)
	r.GET("/api/users/list", usersCtl.GetList)

	srv := httptest.NewServer(r)
	defer srv.Close()

	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("cookie jar: %v", err)
	}
	client := &http.Client{Jar: jar}
	origin := "http://localhost:3000"

	loginReq, _ := http.NewRequest(http.MethodPost, srv.URL+"/api/auth/login", bytes.NewBufferString(
		`{"email":"admin@example.com","password":"Secret123!"}`,
	))
	loginReq.Header.Set("Content-Type", "application/json")
	loginReq.Header.Set("Origin", origin)

	loginRes, err := client.Do(loginReq)
	if err != nil {
		t.Fatalf("login request: %v", err)
	}
	defer loginRes.Body.Close()

	if loginRes.StatusCode != http.StatusOK {
		t.Fatalf("login status=%d body=%s", loginRes.StatusCode, mustReadBody(t, loginRes))
	}
	if !strings.Contains(strings.Join(loginRes.Header.Values("Set-Cookie"), ";"), config.Cfg.WebAdmin.SessionName+"=") {
		t.Fatalf("login should set session cookie")
	}

	listReqBefore, _ := http.NewRequest(http.MethodGet, srv.URL+"/api/users/list", nil)
	listReqBefore.Header.Set("Origin", origin)

	listResBefore, err := client.Do(listReqBefore)
	if err != nil {
		t.Fatalf("users/list before logout request: %v", err)
	}
	defer listResBefore.Body.Close()

	if listResBefore.StatusCode != http.StatusOK {
		t.Fatalf("users/list before logout status=%d body=%s", listResBefore.StatusCode, mustReadBody(t, listResBefore))
	}

	logoutReq, _ := http.NewRequest(http.MethodPost, srv.URL+"/api/auth/logout", nil)
	logoutReq.Header.Set("Origin", origin)

	logoutRes, err := client.Do(logoutReq)
	if err != nil {
		t.Fatalf("logout request: %v", err)
	}
	defer logoutRes.Body.Close()

	if logoutRes.StatusCode != http.StatusOK {
		t.Fatalf("logout status=%d body=%s", logoutRes.StatusCode, mustReadBody(t, logoutRes))
	}

	logoutSetCookie := strings.Join(logoutRes.Header.Values("Set-Cookie"), ";")
	if !strings.Contains(logoutSetCookie, config.Cfg.WebAdmin.SessionName+"=") {
		t.Fatalf("logout should return updated session cookie")
	}
	if !strings.Contains(logoutSetCookie, "Max-Age=0") && !strings.Contains(strings.ToLower(logoutSetCookie), "expires=") {
		t.Fatalf("logout cookie should expire session")
	}

	listReqAfter, _ := http.NewRequest(http.MethodGet, srv.URL+"/api/users/list", nil)
	listReqAfter.Header.Set("Origin", origin)

	listResAfter, err := client.Do(listReqAfter)
	if err != nil {
		t.Fatalf("users/list after logout request: %v", err)
	}
	defer listResAfter.Body.Close()

	if listResAfter.StatusCode != http.StatusUnauthorized {
		t.Fatalf("users/list after logout status=%d body=%s", listResAfter.StatusCode, mustReadBody(t, listResAfter))
	}

	logoutReqAgain, _ := http.NewRequest(http.MethodPost, srv.URL+"/api/auth/logout", nil)
	logoutReqAgain.Header.Set("Origin", origin)

	logoutResAgain, err := client.Do(logoutReqAgain)
	if err != nil {
		t.Fatalf("second logout request: %v", err)
	}
	defer logoutResAgain.Body.Close()

	if logoutResAgain.StatusCode != http.StatusOK {
		t.Fatalf("second logout status=%d body=%s", logoutResAgain.StatusCode, mustReadBody(t, logoutResAgain))
	}
}

// mustReadBody performs its package-specific operation.
func mustReadBody(t *testing.T, res *http.Response) string {
	t.Helper()
	b, err := io.ReadAll(res.Body)
	if err != nil {
		return "<read error>"
	}
	return string(b)
}
