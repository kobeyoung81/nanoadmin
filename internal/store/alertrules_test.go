package store

import "testing"

// Ids come from a counter because the console deletes a rule by id; an insert
// must therefore hand one out, and the second insert must get a different one.
func TestAlertRulesAssignIDsAndListEnabledOnly(t *testing.T) {
	st := testStore(t)

	first, err := st.UpsertAlertRule(AlertRule{Severity: "warning", Message: "cpu", Enabled: true})
	if err != nil {
		t.Fatalf("insert first: %v", err)
	}
	second, err := st.UpsertAlertRule(AlertRule{Severity: "critical", Message: "disk", Enabled: true})
	if err != nil {
		t.Fatalf("insert second: %v", err)
	}
	if first == 0 || second == 0 || first == second {
		t.Fatalf("ids = %d, %d; want two distinct non-zero ids", first, second)
	}

	// Disabled rules are not returned: the list feeds an enabled-rules view.
	if _, err := st.UpsertAlertRule(AlertRule{Severity: "info", Message: "off", Enabled: false}); err != nil {
		t.Fatalf("insert disabled: %v", err)
	}

	rules, err := st.ListAlertRules()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(rules) != 2 {
		t.Fatalf("got %d rules, want 2 enabled", len(rules))
	}
	if rules[0].ID > rules[1].ID {
		t.Fatalf("rules are not ordered by id: %d, %d", rules[0].ID, rules[1].ID)
	}

	// An update by id changes the fields and keeps the id.
	if _, err := st.UpsertAlertRule(AlertRule{ID: first, Severity: "critical", Message: "cpu", Enabled: true}); err != nil {
		t.Fatalf("update: %v", err)
	}
	rules, _ = st.ListAlertRules()
	if rules[0].Severity != "critical" {
		t.Fatalf("severity = %q, want the updated critical", rules[0].Severity)
	}

	// Deleting a missing rule is not an error, matching the SQL it replaced.
	if err := st.DeleteAlertRule(9999); err != nil {
		t.Fatalf("delete missing: %v", err)
	}
	if err := st.DeleteAlertRule(first); err != nil {
		t.Fatalf("delete: %v", err)
	}
	rules, _ = st.ListAlertRules()
	if len(rules) != 1 {
		t.Fatalf("got %d rules after delete, want 1", len(rules))
	}
}
