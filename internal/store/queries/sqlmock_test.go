package queries

import (
	"context"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestQueries_ScanAndRowsErrPaths(t *testing.T) {
	ctx := context.Background()

	t.Run("GetAllNodes scan error", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { db.Close() })
		cols := []string{"id", "name", "description", "type", "city", "country", "lat", "lon", "credential_id", "active", "is_default", "enabled_cmds", "bgp_config", "agent_url", "agent_token", "created_at", "updated_at"}
		mock.ExpectQuery("SELECT .+ FROM nodes ORDER BY name").WillReturnRows(sqlmock.NewRows(cols).AddRow("bad", nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil))
		q := New(db)
		if _, err := q.GetAllNodes(ctx); err == nil {
			t.Fatal("expected scan error")
		}
	})

	t.Run("GetAllNodes rows err", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { db.Close() })
		cols := []string{"id", "name", "description", "type", "city", "country", "lat", "lon", "credential_id", "active", "is_default", "enabled_cmds", "bgp_config", "agent_url", "agent_token", "created_at", "updated_at"}
		rows := sqlmock.NewRows(cols).AddRow(1, "n", "", "standalone", "", "", nil, nil, nil, 1, 0, "[]", nil, "", "", "now", "now").RowError(0, errors.New("rows err"))
		mock.ExpectQuery("SELECT .+ FROM nodes ORDER BY name").WillReturnRows(rows)
		q := New(db)
		if _, err := q.GetAllNodes(ctx); err == nil {
			t.Fatal("expected rows error")
		}
	})

	t.Run("CreateNode last insert id error", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { db.Close() })
		mock.ExpectExec("INSERT INTO nodes").WillReturnResult(sqlmock.NewErrorResult(errors.New("last insert id")))
		q := New(db)
		if _, err := q.CreateNode(ctx, &Node{Name: "n", Type: "standalone"}); err == nil {
			t.Fatal("expected last insert id error")
		}
	})

	t.Run("SetDefaultNode rows affected error", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { db.Close() })
		mock.ExpectExec("UPDATE nodes SET is_default = 0").WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("UPDATE nodes SET is_default = 1 WHERE id").WillReturnResult(sqlmock.NewErrorResult(errors.New("rows affected")))
		q := New(db)
		if err := q.SetDefaultNode(ctx, 1); err == nil {
			t.Fatal("expected rows affected error")
		}
	})

	t.Run("ListAuditLogs scan error", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { db.Close() })
		cols := []string{"id", "created_at", "source_ip", "user_id", "node_id", "command", "params", "duration_ms", "success", "error_msg", "node_name"}
		mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM audit_log").WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
		mock.ExpectQuery("SELECT a.id, a.created_at").WillReturnRows(sqlmock.NewRows(cols).AddRow("bad", nil, nil, nil, nil, nil, nil, nil, nil, nil, nil))
		q := New(db)
		if _, _, err := q.ListAuditLogs(ctx, &AuditFilter{Limit: 1}); err == nil {
			t.Fatal("expected scan error")
		}
	})

	t.Run("GetCommunityRuleByID scan error", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { db.Close() })
		cols := []string{"id", "community", "severity", "message_i18n", "scope", "active", "created_at", "updated_at"}
		mock.ExpectQuery("SELECT .+ FROM community_rules WHERE id").WillReturnRows(sqlmock.NewRows(cols).AddRow("bad", nil, nil, nil, nil, nil, nil, nil))
		q := New(db)
		if _, err := q.GetCommunityRuleByID(ctx, 1); err == nil {
			t.Fatal("expected scan error")
		}
	})
}

func TestQueries_AllListScanErrors(t *testing.T) {
	ctx := context.Background()
	type listCase struct {
		name string
		run  func(*Queries) error
	}
	cases := []listCase{
		{"GetActiveNodes", func(q *Queries) error { _, err := q.GetActiveNodes(ctx); return err }},
		{"GetAllCommunityRules", func(q *Queries) error { _, err := q.GetAllCommunityRules(ctx); return err }},
		{"GetActiveCommunityRules", func(q *Queries) error { _, err := q.GetActiveCommunityRules(ctx); return err }},
		{"GetActiveCommunityRulesForNode", func(q *Queries) error { _, err := q.GetActiveCommunityRulesForNode(ctx, 1); return err }},
		{"GetAllBGPNeighbors", func(q *Queries) error { _, err := q.GetAllBGPNeighbors(ctx); return err }},
		{"GetBGPNeighborsByNodeID", func(q *Queries) error { _, err := q.GetBGPNeighborsByNodeID(ctx, 1); return err }},
		{"GetAllQuickQueries", func(q *Queries) error { _, err := q.GetAllQuickQueries(ctx); return err }},
		{"GetActiveQuickQueries", func(q *Queries) error { _, err := q.GetActiveQuickQueries(ctx); return err }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { db.Close() })
			mock.ExpectQuery("SELECT").WillReturnRows(sqlmock.NewRows([]string{"bad"}).AddRow("bad"))
			q := New(db)
			if err := tc.run(q); err == nil {
				t.Fatal("expected scan error")
			}
		})
	}
}

func TestQueries_CreateLastInsertErrors(t *testing.T) {
	ctx := context.Background()
	type createCase struct {
		name string
		run  func(*Queries) error
	}
	cases := []createCase{
		{"CreateCommunityRule", func(q *Queries) error {
			_, err := q.CreateCommunityRule(ctx, &CommunityRule{Community: "1:1", Severity: "info", MessageI18n: "{}", Scope: "global", Active: 1})
			return err
		}},
		{"CreateBGPNeighbor", func(q *Queries) error {
			_, err := q.CreateBGPNeighbor(ctx, &BGPNeighbor{NodeID: 1, PeerType: "external"})
			return err
		}},
		{"CreateQuickQuery", func(q *Queries) error {
			_, err := q.CreateQuickQuery(ctx, &QuickQuery{Command: "ping", Name: "n", Target: "1.1.1.1", SortOrder: 1, Active: 1})
			return err
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { db.Close() })
			mock.ExpectExec("INSERT INTO").WillReturnResult(sqlmock.NewErrorResult(errors.New("last insert id")))
			q := New(db)
			if err := tc.run(q); err == nil {
				t.Fatal("expected last insert id error")
			}
		})
	}
}

func TestQueries_ListAuditRowsErr(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	cols := []string{"id", "created_at", "user_id", "source_ip", "command", "params", "duration_ms", "success", "error_msg"}
	rows := sqlmock.NewRows(cols).AddRow(1, "now", nil, "1.1.1.1", "ping", "8.8.8.8", 1, 1, "").RowError(0, errors.New("rows err"))
	mock.ExpectQuery("SELECT .+ FROM audit_log").WillReturnRows(rows)
	q := New(db)
	if _, _, err := q.ListAuditLogs(context.Background(), &AuditFilter{Limit: 1}); err == nil {
		t.Fatal("expected rows error")
	}
}

func TestQueries_GetSettingsRowsErr(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	rows := sqlmock.NewRows([]string{"key", "value"}).AddRow("k", "v").RowError(0, errors.New("rows err"))
	mock.ExpectQuery("SELECT key, value FROM settings").WillReturnRows(rows)
	q := New(db)
	if _, err := q.GetSettings(); err == nil {
		t.Fatal("expected rows error")
	}
}

func TestQueries_AllListRowsErr(t *testing.T) {
	ctx := context.Background()
	nodeCols := []string{"id", "name", "description", "type", "city", "country", "lat", "lon", "credential_id", "active", "is_default", "enabled_cmds", "bgp_config", "agent_url", "agent_token", "created_at", "updated_at"}
	ruleCols := []string{"id", "community", "severity", "message_i18n", "scope", "node_id", "active", "created_at", "updated_at"}
	bgpCols := []string{"id", "node_id", "local_as", "remote_as", "peering_ip", "neighbor_ip", "ipv6_peering_ip", "ipv6_neighbor_ip", "multihop", "default_route_as", "peer_type", "created_at", "updated_at"}
	type listCase struct {
		name string
		cols []string
		run  func(*Queries) error
	}
	cases := []listCase{
		{"GetActiveNodes", nodeCols, func(q *Queries) error { _, err := q.GetActiveNodes(ctx); return err }},
		{"GetAllCommunityRules", ruleCols, func(q *Queries) error { _, err := q.GetAllCommunityRules(ctx); return err }},
		{"GetActiveCommunityRules", ruleCols, func(q *Queries) error { _, err := q.GetActiveCommunityRules(ctx); return err }},
		{"GetActiveCommunityRulesForNode", ruleCols, func(q *Queries) error { _, err := q.GetActiveCommunityRulesForNode(ctx, 1); return err }},
		{"GetAllBGPNeighbors", bgpCols, func(q *Queries) error { _, err := q.GetAllBGPNeighbors(ctx); return err }},
		{"GetBGPNeighborsByNodeID", bgpCols, func(q *Queries) error { _, err := q.GetBGPNeighborsByNodeID(ctx, 1); return err }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { db.Close() })
			rows := sqlmock.NewRows(tc.cols).CloseError(errors.New("rows err"))
			mock.ExpectQuery("SELECT").WillReturnRows(rows)
			q := New(db)
			if err := tc.run(q); err == nil {
				t.Fatal("expected rows error")
			}
		})
	}
}

func TestQueries_SetDefaultNodeErrors(t *testing.T) {
	ctx := context.Background()

	t.Run("begin error", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { db.Close() })
		mock.ExpectBegin().WillReturnError(errors.New("begin failed"))
		q := New(db)
		if err := q.SetDefaultNode(ctx, 1); err == nil {
			t.Fatal("expected begin error")
		}
	})

	t.Run("clear defaults error", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { db.Close() })
		mock.ExpectBegin()
		mock.ExpectExec("UPDATE nodes SET is_default = 0").WillReturnError(errors.New("clear failed"))
		q := New(db)
		if err := q.SetDefaultNode(ctx, 1); err == nil {
			t.Fatal("expected clear error")
		}
	})

	t.Run("set default update error", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { db.Close() })
		mock.ExpectBegin()
		mock.ExpectExec("UPDATE nodes SET is_default = 0").WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("UPDATE nodes SET is_default = 1 WHERE id").WillReturnError(errors.New("update failed"))
		q := New(db)
		if err := q.SetDefaultNode(ctx, 1); err == nil {
			t.Fatal("expected update error")
		}
	})

	t.Run("commit error", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { db.Close() })
		mock.ExpectBegin()
		mock.ExpectExec("UPDATE nodes SET is_default = 0").WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("UPDATE nodes SET is_default = 1 WHERE id").WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectCommit().WillReturnError(errors.New("commit failed"))
		q := New(db)
		if err := q.SetDefaultNode(ctx, 1); err == nil {
			t.Fatal("expected commit error")
		}
	})
}

func TestQueries_ListAuditLogsFiltersAndErrors(t *testing.T) {
	ctx := context.Background()
	cols := []string{"id", "created_at", "source_ip", "user_id", "node_id", "command", "params", "duration_ms", "success", "error_msg", "node_name"}
	nodeID := int64(1)

	t.Run("count error", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { db.Close() })
		mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM audit_log").WillReturnError(errors.New("count failed"))
		q := New(db)
		if _, _, err := q.ListAuditLogs(ctx, &AuditFilter{Limit: 1}); err == nil {
			t.Fatal("expected count error")
		}
	})

	t.Run("list query error", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { db.Close() })
		mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM audit_log").WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
		mock.ExpectQuery("SELECT a.id, a.created_at").WillReturnError(errors.New("list failed"))
		q := New(db)
		if _, _, err := q.ListAuditLogs(ctx, &AuditFilter{Limit: 1}); err == nil {
			t.Fatal("expected list error")
		}
	})

	t.Run("limit with page offset", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { db.Close() })
		mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM audit_log").WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
		mock.ExpectQuery("SELECT a.id, a.created_at").WithArgs(5, 10).WillReturnRows(sqlmock.NewRows(cols))
		q := New(db)
		if _, _, err := q.ListAuditLogs(ctx, &AuditFilter{Limit: 5, Page: 2}); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("source ip filter only", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { db.Close() })
		mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM audit_log a WHERE 1=1 AND a.source_ip = \\?").
			WithArgs("10.0.0.1").
			WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
		mock.ExpectQuery("SELECT a.id, a.created_at").WithArgs("10.0.0.1").WillReturnRows(sqlmock.NewRows(cols))
		q := New(db)
		if _, _, err := q.ListAuditLogs(ctx, &AuditFilter{SourceIP: "10.0.0.1"}); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("command filter only", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { db.Close() })
		mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM audit_log a WHERE 1=1 AND a.command = \\?").
			WithArgs("traceroute").
			WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
		mock.ExpectQuery("SELECT a.id, a.created_at").WithArgs("traceroute").WillReturnRows(sqlmock.NewRows(cols))
		q := New(db)
		if _, _, err := q.ListAuditLogs(ctx, &AuditFilter{Command: "traceroute"}); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("node id filter only", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { db.Close() })
		mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM audit_log a WHERE 1=1 AND a.node_id = \\?").
			WithArgs(nodeID).
			WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
		mock.ExpectQuery("SELECT a.id, a.created_at").
			WithArgs(nodeID, 5).
			WillReturnRows(sqlmock.NewRows(cols).AddRow(1, "now", "1.1.1.1", nil, nodeID, "ping", "", 1, 1, "", ""))
		q := New(db)
		if _, total, err := q.ListAuditLogs(ctx, &AuditFilter{NodeID: &nodeID, Limit: 5}); err != nil || total != 1 {
			t.Fatalf("total=%d err=%v", total, err)
		}
	})

	t.Run("limit without page", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { db.Close() })
		mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM audit_log").WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
		mock.ExpectQuery("SELECT a.id, a.created_at").WithArgs(3).WillReturnRows(sqlmock.NewRows(cols))
		q := New(db)
		if _, _, err := q.ListAuditLogs(ctx, &AuditFilter{Limit: 3, Page: 0}); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("without limit", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { db.Close() })
		mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM audit_log").WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
		mock.ExpectQuery("SELECT a.id, a.created_at").WillReturnRows(sqlmock.NewRows(cols))
		q := New(db)
		if _, total, err := q.ListAuditLogs(ctx, &AuditFilter{SourceIP: "1.1.1.1"}); err != nil || total != 0 {
			t.Fatalf("total=%d err=%v", total, err)
		}
	})

	t.Run("with filters and paging", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { db.Close() })
		mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM audit_log a WHERE 1=1 AND a.node_id = \\? AND a.command = \\? AND a.source_ip = \\?").
			WithArgs(nodeID, "ping", "1.1.1.1").
			WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
		mock.ExpectQuery("SELECT a.id, a.created_at").
			WithArgs(nodeID, "ping", "1.1.1.1", 10, 10).
			WillReturnRows(sqlmock.NewRows(cols))
		q := New(db)
		if _, total, err := q.ListAuditLogs(ctx, &AuditFilter{
			NodeID: &nodeID, Command: "ping", SourceIP: "1.1.1.1", Limit: 10, Page: 1,
		}); err != nil || total != 0 {
			t.Fatalf("total=%d err=%v", total, err)
		}
	})
}

func TestQueries_GetCommunityRuleByIDNotFound(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	mock.ExpectQuery("SELECT .+ FROM community_rules WHERE id").WillReturnRows(sqlmock.NewRows([]string{
		"id", "community", "severity", "message_i18n", "scope", "node_id", "active", "created_at", "updated_at",
	}))
	q := New(db)
	got, err := q.GetCommunityRuleByID(context.Background(), 999)
	if err != nil || got != nil {
		t.Fatalf("got=%+v err=%v", got, err)
	}
}
