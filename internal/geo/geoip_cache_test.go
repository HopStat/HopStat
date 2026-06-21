package geo_test

import (
	"context"
	"testing"

	"github.com/HopStat/HopStat/internal/geo"
)

func TestResolveASNCache(t *testing.T) {
	g := geo.New("", "")
	ctx := context.Background()

	first, err := g.ResolveASN(ctx, "8.8.8.8")
	if err != nil {
		t.Fatalf("first resolve: %v", err)
	}
	second, err := g.ResolveASN(ctx, "8.8.8.8")
	if err != nil {
		t.Fatalf("second resolve: %v", err)
	}
	if first == nil && second == nil {
		return
	}
	if (first == nil) != (second == nil) {
		t.Fatal("cache returned inconsistent nil results")
	}
	if first.ASN != second.ASN {
		t.Fatalf("ASN mismatch: %d vs %d", first.ASN, second.ASN)
	}
}

func BenchmarkResolveASNCacheHit(b *testing.B) {
	g := geo.New("", "")
	ctx := context.Background()
	if _, err := g.ResolveASN(ctx, "8.8.8.8"); err != nil {
		b.Skip("geo not available")
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = g.ResolveASN(ctx, "8.8.8.8")
	}
}
