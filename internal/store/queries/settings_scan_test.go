package queries

import (
	"errors"
	"testing"
)

func TestGetSettingsScanError(t *testing.T) {
	old := settingsScanRow
	settingsScanRow = func(scanner interface{ Scan(...interface{}) error }, key, value *string) error {
		return errors.New("scan failed")
	}
	t.Cleanup(func() { settingsScanRow = old })

	_, q := setupMigratedDB(t)
	if _, err := q.GetSettings(); err == nil {
		t.Fatal("expected scan error")
	}
}
