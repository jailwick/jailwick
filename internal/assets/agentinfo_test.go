package assets

import "testing"

func TestChangeText(t *testing.T) {
	for in, want := range map[string]string{
		"fix(worktrees): copy databases":     "Worktrees: copy databases",
		"feat(image)!: a minimal base":       "Image: a minimal base",
		"feat: one login for all projects":   "One login for all projects",
		"Merge branch 'main'":                "Merge branch 'main'",
		"note: keep this: as is":             "Keep this: as is",
		"a plain subject with: a colon here": "A plain subject with: a colon here",
	} {
		if got := ChangeText(in); got != want {
			t.Errorf("ChangeText(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestAgentInfo(t *testing.T) {
	old := &AgentInfo{Built: "2026-10-13", Changes: []AgentChange{{Commit: "a", Text: "one"}}}
	now := &AgentInfo{Built: "2026-10-15", Changes: []AgentChange{{Commit: "b", Text: "two"}, {Commit: "a", Text: "one"}}}
	if got := now.Since(old); len(got) != 1 || got[0].Text != "two" {
		t.Fatalf("since: %+v", got)
	}
	if now.Label() != "jailwick build of 15 Oct 2026" || (&AgentInfo{Release: "0.2.5", Built: "2026-10-15"}).Label() != "jailwick 0.2.5 (15 Oct 2026)" {
		t.Fatalf("labels: %q", now.Label())
	}
	if ParseAgentInfo([]byte(`{}`)) != nil || ParseAgentInfo([]byte(`{"built":"2026-10-15"}`)) == nil {
		t.Fatal("parse")
	}
}
