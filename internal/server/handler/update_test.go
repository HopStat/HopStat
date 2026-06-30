package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/HopStat/HopStat/internal/updater"
)

func TestUpdateStatus_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"tag_name": "v2.0.0",
			"html_url": "https://github.com/HopStat/HopStat/releases/tag/v2.0.0",
		})
	}))
	defer srv.Close()

	upd := updater.New("HopStat/HopStat", "v1.0.0", true)
	upd.SetReleaseAPIURL(srv.URL)
	c, w := setupAdminContext(nil, http.MethodGet, "/admin/update/status", "", 1)

	UpdateStatus(upd)(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
}

func TestUpdateStatus_Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	upd := updater.New("HopStat/HopStat", "v1.0.0", true)
	upd.SetReleaseAPIURL(srv.URL)
	c, w := setupAdminContext(nil, http.MethodGet, "/admin/update/status", "", 1)

	UpdateStatus(upd)(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	var resp struct {
		Data updater.Status `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Data.Current != "v1.0.0" {
		t.Fatalf("current = %q", resp.Data.Current)
	}
	if resp.Data.Latest == "" {
		t.Fatal("expected embedded latest version")
	}
}

func TestUpdateApply_StatusError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	upd := updater.New("HopStat/HopStat", "v1.0.0", true)
	upd.SetReleaseAPIURL(srv.URL)
	c, w := setupAdminContext(nil, http.MethodPost, "/admin/update/apply", "", 1)

	UpdateApply(upd)(c)

	// GitHub errors fall back to embedded release metadata; apply then depends on self-update support.
	if w.Code != http.StatusAccepted && w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
}

func TestUpdateApply_Accepted(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"tag_name": "v2.0.0",
			"html_url": "https://github.com/HopStat/HopStat/releases/tag/v2.0.0",
		})
	}))
	defer srv.Close()

	upd := updater.New("HopStat/HopStat", "v1.0.0", true)
	upd.SetReleaseAPIURL(srv.URL)
	c, w := setupAdminContext(nil, http.MethodPost, "/admin/update/apply", "", 1)

	UpdateApply(upd)(c)

	if w.Code != http.StatusAccepted && w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
}

func TestUpdateApply_NoUpdateAvailable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"tag_name": "v1.0.0",
			"html_url": "https://github.com/HopStat/HopStat/releases/tag/v1.0.0",
		})
	}))
	defer srv.Close()

	upd := updater.New("HopStat/HopStat", "v1.0.0", true)
	upd.SetReleaseAPIURL(srv.URL)
	c, w := setupAdminContext(nil, http.MethodPost, "/admin/update/apply", "", 1)

	UpdateApply(upd)(c)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
}

func TestUpdateStatus_StatusHookError(t *testing.T) {
	orig := updater.StatusErrHook
	updater.StatusErrHook = errors.New("forced status error")
	t.Cleanup(func() { updater.StatusErrHook = orig })

	upd := updater.New("HopStat/HopStat", "v1.0.0", true)
	c, w := setupAdminContext(nil, http.MethodGet, "/admin/update/status", "", 1)
	UpdateStatus(upd)(c)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
}

func TestUpdateApply_StatusHookError(t *testing.T) {
	orig := updater.StatusErrHook
	updater.StatusErrHook = errors.New("forced apply status error")
	t.Cleanup(func() { updater.StatusErrHook = orig })

	upd := updater.New("HopStat/HopStat", "v1.0.0", true)
	c, w := setupAdminContext(nil, http.MethodPost, "/admin/update/apply", "", 1)
	UpdateApply(upd)(c)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
}
