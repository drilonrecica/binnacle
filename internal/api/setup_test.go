// SPDX-License-Identifier: AGPL-3.0-only
package api

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/drilonrecica/binnacle/internal/auth"
	"github.com/drilonrecica/binnacle/internal/storage"
)

func TestSetupClaimReturns404OnceClaimed(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	manager := storage.New(filepath.Join(dir, "binnacle.db"), filepath.Join(dir, "run"))
	if err := manager.Open(ctx); err != nil {
		t.Fatal(err)
	}
	defer manager.Close()
	setup := auth.NewSetupService(manager.DB())
	token := "0123456789abcdefghijklmnopqrstuvwxyz-CLAIM"
	if _, err := setup.Initialize(ctx, ":8080", token); err != nil {
		t.Fatal(err)
	}
	server := New()
	server.EnableSetup(setup, auth.NewProtection(128, auth.TrustedProxies{}), nil)
	claim := func(body string) *httptest.ResponseRecorder {
		request := httptest.NewRequest(http.MethodPost, "http://binnacle.test/api/v1/setup/claim", bytes.NewBufferString(body))
		request.RemoteAddr = "192.0.2.10:1234"
		request.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		server.Handler().ServeHTTP(response, request)
		return response
	}
	if response := claim(`{"token":"wrong","username":"admin","password":"correct horse battery staple"}`); response.Code != http.StatusBadRequest {
		t.Fatalf("wrong token status=%d body=%s", response.Code, response.Body.String())
	}
	if response := claim(`{"token":"` + token + `","username":"admin","password":"correct horse battery staple"}`); response.Code != http.StatusNoContent {
		t.Fatalf("claim status=%d body=%s", response.Code, response.Body.String())
	}
	response := claim(`{"token":"` + token + `","username":"other","password":"correct horse battery staple"}`)
	if response.Code != http.StatusNotFound {
		t.Fatalf("claim after setup status=%d body=%s", response.Code, response.Body.String())
	}
	if !bytes.Contains(response.Body.Bytes(), []byte(`"not_found"`)) {
		t.Fatalf("claim after setup body=%s", response.Body.String())
	}
}
