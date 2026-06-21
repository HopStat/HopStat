package repo

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"encoding/hex"
	"errors"
	"io"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/HopStat/HopStat/internal/domain"
)

func TestEncryptDecryptErrors(t *testing.T) {
	key := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

	t.Run("hex decode key error", func(t *testing.T) {
		if _, err := Encrypt("x", "not-hex"); err == nil {
			t.Fatal("expected hex decode error")
		}
	})
	t.Run("key length error encrypt", func(t *testing.T) {
		if _, err := Encrypt("x", "abcd"); err == nil {
			t.Fatal("expected key length error")
		}
	})
	t.Run("ciphertext decode error", func(t *testing.T) {
		if _, err := Decrypt("not-hex", key); err == nil {
			t.Fatal("expected ciphertext decode error")
		}
	})
	t.Run("short ciphertext error", func(t *testing.T) {
		if _, err := Decrypt("0123456789abcdef", key); err == nil {
			t.Fatal("expected short ciphertext error")
		}
	})
	t.Run("rand error", func(t *testing.T) {
		oldRand := readRandom
		readRandom = func(io.Reader, []byte) (int, error) { return 0, errors.New("rand failed") }
		t.Cleanup(func() { readRandom = oldRand })
		if _, err := Encrypt("secret", key); err == nil {
			t.Fatal("expected rand error")
		}
	})
	t.Run("cipher error encrypt", func(t *testing.T) {
		oldCipher := newAESCipher
		newAESCipher = func([]byte) (cipher.Block, error) { return nil, errors.New("cipher failed") }
		t.Cleanup(func() { newAESCipher = oldCipher })
		if _, err := Encrypt("secret", key); err == nil {
			t.Fatal("expected cipher error")
		}
	})
	t.Run("gcm error encrypt", func(t *testing.T) {
		oldGCM := newAESGCM
		newAESGCM = func(cipher.Block) (cipher.AEAD, error) { return nil, errors.New("gcm failed") }
		t.Cleanup(func() { newAESGCM = oldGCM })
		if _, err := Encrypt("secret", key); err == nil {
			t.Fatal("expected gcm error on encrypt")
		}
	})
	t.Run("gcm error decrypt", func(t *testing.T) {
		rawKey, _ := hex.DecodeString(key)
		block, err := aes.NewCipher(rawKey)
		if err != nil {
			t.Fatal(err)
		}
		gcm, err := cipher.NewGCM(block)
		if err != nil {
			t.Fatal(err)
		}
		nonce := make([]byte, gcm.NonceSize())
		ciphertext := gcm.Seal(nonce, nonce, []byte("secret"), nil)
		encoded := hex.EncodeToString(ciphertext)

		oldGCM := newAESGCM
		newAESGCM = func(cipher.Block) (cipher.AEAD, error) { return nil, errors.New("gcm failed") }
		t.Cleanup(func() { newAESGCM = oldGCM })
		if _, err := Decrypt(encoded, key); err == nil {
			t.Fatal("expected gcm error on decrypt")
		}
	})
}

func TestDecryptMoreErrors(t *testing.T) {
	key := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	if _, err := Decrypt("0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef", "abcd"); err == nil {
		t.Fatal("expected key length error")
	}

	oldCipher := newAESCipher
	newAESCipher = func([]byte) (cipher.Block, error) { return nil, errors.New("cipher failed") }
	t.Cleanup(func() { newAESCipher = oldCipher })
	if _, err := Decrypt("0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef", key); err == nil {
		t.Fatal("expected cipher error on decrypt")
	}

	newAESCipher = aes.NewCipher
	rawKey, _ := hex.DecodeString(key)
	block, err := aes.NewCipher(rawKey)
	if err != nil {
		t.Fatal(err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		t.Fatal(err)
	}
	nonce := make([]byte, gcm.NonceSize())
	ciphertext := gcm.Seal(nonce, nonce, []byte("secret"), nil)
	tampered := append([]byte(nil), ciphertext...)
	tampered[len(tampered)-1] ^= 0xff
	encoded := hex.EncodeToString(tampered)
	if _, err := Decrypt(encoded, key); err == nil {
		t.Fatal("expected decrypt open error")
	}
}

func TestEncryptCipherError(t *testing.T) {
	key := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	oldCipher := newAESCipher
	newAESCipher = func([]byte) (cipher.Block, error) { return nil, errors.New("cipher failed") }
	t.Cleanup(func() { newAESCipher = oldCipher })
	if _, err := Encrypt("secret", key); err == nil {
		t.Fatal("expected cipher error")
	}
}

func TestNodeRepoCreateWithDefault(t *testing.T) {
	db := setupRepoDB(t)
	repo := NewNodeRepo(db, testCredKey)
	created, err := repo.Create(context.Background(), &domain.Node{
		Name: "def", Type: domain.NodeTypeStandalone, Active: true, IsDefault: true,
	})
	if err != nil || !created.IsDefault {
		t.Fatalf("created=%+v err=%v", created, err)
	}
}

func TestNodeRepoEnsureDefaultAndMapNodeFields(t *testing.T) {
	db := setupRepoDB(t)
	repo := NewNodeRepo(db, testCredKey)
	ctx := context.Background()

	lat, lon := 1.0, 2.0
	credID := int64(9)
	_, err := repo.Create(ctx, &domain.Node{
		Name: "a", Type: domain.NodeTypeStandalone, Active: true,
		Lat: &lat, Lon: &lon, CredentialID: &credID,
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = repo.Create(ctx, &domain.Node{
		Name: "b", Type: domain.NodeTypeStandalone, Active: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	active, err := repo.GetActive(ctx)
	if err != nil || len(active) != 2 {
		t.Fatalf("active=%d err=%v", len(active), err)
	}
	if !active[0].IsDefault && !active[1].IsDefault {
		t.Fatal("expected one default node")
	}
}

func TestMapNodeTimestamps(t *testing.T) {
	db := setupRepoDB(t)
	if _, err := db.Exec(`INSERT INTO nodes (name, type, enabled_cmds, created_at, updated_at) VALUES ('ts', 'standalone', '[]', '2024-01-01 00:00:00', '2024-01-02 00:00:00')`); err != nil {
		t.Fatal(err)
	}
	repo := NewNodeRepo(db, "")
	got, err := repo.GetByID(context.Background(), 1)
	if err != nil || got.CreatedAt.IsZero() || got.UpdatedAt.IsZero() {
		t.Fatalf("got=%+v err=%v", got, err)
	}
}

func TestNodeRepoGetActiveError(t *testing.T) {
	db := setupRepoDB(t)
	repo := NewNodeRepo(db, "")
	ctx := context.Background()
	if _, err := repo.Create(ctx, &domain.Node{Name: "a", Type: domain.NodeTypeStandalone, Active: true}); err != nil {
		t.Fatal(err)
	}
	db.Close()
	if _, err := repo.GetActive(ctx); err == nil {
		t.Fatal("expected get active error")
	}
}

func TestNodeRepoGetActiveNodesError(t *testing.T) {
	db := setupRepoDB(t)
	repo := NewNodeRepo(db, "")
	ctx := context.Background()
	if _, err := repo.Create(ctx, &domain.Node{Name: "a", Type: domain.NodeTypeStandalone, Active: true, IsDefault: true}); err != nil {
		t.Fatal(err)
	}
	db.Close()
	if _, err := repo.GetActive(ctx); err == nil {
		t.Fatal("expected get active nodes error")
	}
}

func TestNodeRepoEnsureDefaultPicksMinID(t *testing.T) {
	db := setupRepoDB(t)
	repo := NewNodeRepo(db, "")
	ctx := context.Background()
	if _, err := db.Exec(`INSERT INTO nodes (id, name, type, active, is_default, enabled_cmds) VALUES (5, 'a', 'standalone', 1, 0, '[]'), (2, 'b', 'standalone', 1, 0, '[]')`); err != nil {
		t.Fatal(err)
	}
	active, err := repo.GetActive(ctx)
	if err != nil || len(active) != 2 {
		t.Fatalf("active=%d err=%v", len(active), err)
	}
	for _, n := range active {
		if n.IsDefault && n.ID != 2 {
			t.Fatalf("default id=%d", n.ID)
		}
	}
}

func TestNodeRepoMapNodeOptionalFields(t *testing.T) {
	db := setupRepoDB(t)
	if _, err := db.Exec(`INSERT INTO nodes (name, type, lat, lon, credential_id, enabled_cmds) VALUES ('opt', 'standalone', 1.5, 2.5, 9, '[]')`); err != nil {
		t.Fatal(err)
	}
	repo := NewNodeRepo(db, "")
	got, err := repo.GetByID(context.Background(), 1)
	if err != nil || got.Lat == nil || got.Lon == nil || got.CredentialID == nil {
		t.Fatalf("got=%+v err=%v", got, err)
	}
}

func TestNodeRepoCreateSetDefaultError(t *testing.T) {
	db := setupRepoDB(t)
	repo := NewNodeRepo(db, "")
	ctx := context.Background()
	created, err := repo.Create(ctx, &domain.Node{Name: "x", Type: domain.NodeTypeStandalone, Active: true, IsDefault: true})
	if err != nil || !created.IsDefault {
		t.Fatalf("created=%+v err=%v", created, err)
	}
}

func TestNodeRepoCreateEnsureDefaultError(t *testing.T) {
	db := setupRepoDB(t)
	repo := NewNodeRepo(db, "")
	ctx := context.Background()
	if _, err := repo.Create(ctx, &domain.Node{Name: "first", Type: domain.NodeTypeStandalone, Active: true}); err != nil {
		t.Fatal(err)
	}
	db.Close()
	if _, err := repo.Create(context.Background(), &domain.Node{Name: "second", Type: domain.NodeTypeStandalone, Active: true}); err == nil {
		t.Fatal("expected ensure default error")
	}
}

func TestMapNodeInvalidEnabledCmds(t *testing.T) {
	db := setupRepoDB(t)
	if _, err := db.Exec(`INSERT INTO nodes (name, type, enabled_cmds) VALUES ('bad', 'standalone', 'not-json')`); err != nil {
		t.Fatal(err)
	}
	repo := NewNodeRepo(db, "")
	got, err := repo.GetByID(context.Background(), 1)
	if err != nil || got == nil {
		t.Fatalf("got=%+v err=%v", got, err)
	}
}

func TestNodeRepoCreateSetDefaultFails(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	cols := []string{"id", "name", "description", "type", "city", "country", "lat", "lon", "credential_id", "active", "is_default", "enabled_cmds", "bgp_config", "agent_url", "agent_token", "created_at", "updated_at"}
	mock.ExpectExec("INSERT INTO nodes").WillReturnResult(sqlmock.NewResult(5, 1))
	mock.ExpectQuery("SELECT .+ FROM nodes WHERE id").WillReturnRows(sqlmock.NewRows(cols).AddRow(5, "x", "", "standalone", "", "", nil, nil, nil, 1, 1, "[]", nil, "", "", "now", "now"))
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE nodes SET is_default = 0").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("UPDATE nodes SET is_default = 1 WHERE id").WillReturnError(errors.New("set default failed"))
	repo := NewNodeRepo(db, testCredKey)
	_, err = repo.Create(context.Background(), &domain.Node{Name: "x", Type: domain.NodeTypeStandalone, Active: true, IsDefault: true})
	if err == nil {
		t.Fatal("expected set default error")
	}
}

func TestNodeRepoCreateEnsureDefaultFails(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	cols := []string{"id", "name", "description", "type", "city", "country", "lat", "lon", "credential_id", "active", "is_default", "enabled_cmds", "bgp_config", "agent_url", "agent_token", "created_at", "updated_at"}
	mock.ExpectExec("INSERT INTO nodes").WillReturnResult(sqlmock.NewResult(5, 1))
	mock.ExpectQuery("SELECT .+ FROM nodes WHERE id").WillReturnRows(sqlmock.NewRows(cols).AddRow(5, "x", "", "standalone", "", "", nil, nil, nil, 1, 0, "[]", nil, "", "", "now", "now"))
	mock.ExpectQuery("SELECT .+ FROM nodes ORDER BY name").WillReturnError(errors.New("get all failed"))
	repo := NewNodeRepo(db, testCredKey)
	_, err = repo.Create(context.Background(), &domain.Node{Name: "x", Type: domain.NodeTypeStandalone, Active: true})
	if err == nil {
		t.Fatal("expected ensure default error")
	}
}

func TestNodeRepoGetActiveAfterEnsureDefaultFails(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	cols := []string{"id", "name", "description", "type", "city", "country", "lat", "lon", "credential_id", "active", "is_default", "enabled_cmds", "bgp_config", "agent_url", "agent_token", "created_at", "updated_at"}
	mock.ExpectQuery("SELECT .+ FROM nodes ORDER BY name").WillReturnRows(sqlmock.NewRows(cols).AddRow(1, "a", "", "standalone", "", "", nil, nil, nil, 1, 0, "[]", nil, "", "", "now", "now"))
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE nodes SET is_default = 0").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("UPDATE nodes SET is_default = 1 WHERE id").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	mock.ExpectQuery("SELECT .+ FROM nodes WHERE active = 1").WillReturnError(errors.New("active query failed"))
	repo := NewNodeRepo(db, "")
	if _, err := repo.GetActive(context.Background()); err == nil {
		t.Fatal("expected get active nodes error")
	}
}
