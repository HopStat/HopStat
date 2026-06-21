package domain

import "testing"

func TestMatchCommunityRules(t *testing.T) {
	rules := []*CommunityRule{
		{ID: 1, Community: "65000:100", Severity: SeverityWarning, Active: true, MessageI18n: "blackhole"},
		{ID: 2, Community: "65000:200", Severity: SeverityInfo, Active: false, MessageI18n: "inactive"},
		{ID: 3, Community: "65100:10", Severity: SeverityAlert, Active: true, MessageI18n: "alert"},
	}
	matched := MatchCommunityRules(rules, []string{"65000:100", "65100:10"})
	if len(matched) != 2 {
		t.Fatalf("matched=%d want 2", len(matched))
	}
	if matched[0].ID != 1 || matched[1].ID != 3 {
		t.Fatalf("unexpected ids: %d, %d", matched[0].ID, matched[1].ID)
	}
}

func TestMatchCommunityRulesEmptyInputs(t *testing.T) {
	rules := []*CommunityRule{{ID: 1, Community: "65000:100", Active: true}}
	if MatchCommunityRules(nil, []string{"65000:100"}) != nil {
		t.Fatal("nil rules should return nil")
	}
	if MatchCommunityRules(rules, nil) != nil {
		t.Fatal("nil communities should return nil")
	}
	if MatchCommunityRules(rules, []string{"  ", ""}) != nil {
		t.Fatal("blank communities should return nil")
	}
}

func TestMatchCommunityRulesSkipsNilRule(t *testing.T) {
	rules := []*CommunityRule{
		nil,
		{ID: 1, Community: " 65000:100 ", Active: true},
	}
	matched := MatchCommunityRules(rules, []string{"65000:100"})
	if len(matched) != 1 || matched[0].ID != 1 {
		t.Fatalf("unexpected match: %+v", matched)
	}
}
