package geo

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/HopStat/HopStat/internal/domain"
)

func TestLookupIPPrefersBlocksOverMMDB(t *testing.T) {
	dir := t.TempDir()
	blocksPath := filepath.Join(dir, asnBlocksIPv4Name)
	if err := os.WriteFile(blocksPath, []byte(`network,geoname_id,registered_country_geoname_id,represented_country_geoname_id,is_anonymous_proxy,is_satellite_provider,autonomous_system_number,autonomous_system_organization
31.223.39.0/24,,,,0,0,9121,TTNet
`), 0644); err != nil {
		t.Fatal(err)
	}

	g := New(filepath.Join(dir, "missing.mmdb"), "")
	g.asnPath = filepath.Join(dir, "missing.mmdb")
	g.reloadASNIndex()

	report, err := g.LookupIP(context.Background(), "31.223.39.210")
	if err != nil {
		t.Fatal(err)
	}
	if !report.Blocks.Matched {
		t.Fatal("expected blocks match")
	}
	if report.Blocks.Info == nil || report.Blocks.Info.ASN != 9121 {
		t.Fatalf("blocks asn: %+v", report.Blocks.Info)
	}
	if report.Result == nil || report.Result.ASN != 9121 {
		t.Fatalf("result asn: %+v", report.Result)
	}
	if report.ChosenSource != SourceBlocks {
		t.Fatalf("chosen source: %q, want blocks", report.ChosenSource)
	}
}

func TestLookupIPInvalid(t *testing.T) {
	g := New("", "")
	if _, err := g.LookupIP(context.Background(), "not-an-ip"); err == nil {
		t.Fatal("expected error")
	}
}

func TestInferChosenSourceFallback(t *testing.T) {
	if got := inferChosenSource(true, true, false); got != SourceBlocks {
		t.Fatalf("got %q, want blocks", got)
	}
	if got := inferChosenSource(false, true, false); got != SourceMMDB {
		t.Fatalf("got %q, want mmdb", got)
	}
	if got := chosenSource(true, true, &domain.ASInfo{ASN: 9121}, &domain.ASInfo{ASN: 9121}, &domain.ASInfo{ASN: 15169}); got != SourceBlocks {
		t.Fatalf("got %q, want blocks", got)
	}
}
