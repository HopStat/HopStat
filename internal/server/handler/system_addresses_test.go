package handler

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/HopStat/HopStat/internal/hostips"
)

func TestSystemAddresses_Success(t *testing.T) {
	c, w := setupAdminContext(nil, http.MethodGet, "/admin/system/addresses", "", 1)
	SystemAddresses()(c)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	var resp struct {
		Data struct {
			IPv4 []hostips.Address `json:"ipv4"`
			IPv6 []hostips.Address `json:"ipv6"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if resp.Data.IPv4 == nil || resp.Data.IPv6 == nil {
		t.Fatalf("expected ipv4 and ipv6 arrays, got body=%s", w.Body.String())
	}
	_, _, err := hostips.List()
	if err != nil {
		t.Fatalf("hostips.List: %v", err)
	}
}
