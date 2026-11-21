package agenttypes

import (
	"testing"

	"gitee.com/kobeyoung81/nanoprotocol"
)

// TestGoalJudgeIsAOneShotWithNoTools pins the shape the judge's safety rests on:
// it is a one-shot utility with no context and no tools, and its temperature is
// pinned to 0. A judge that could search or run would be able to manufacture the
// evidence it is supposed to weigh.
func TestGoalJudgeIsAOneShotWithNoTools(t *testing.T) {
	var def nanoprotocol.AgentTypeDef
	found := false
	for _, d := range Definitions() {
		if d.ID == "role_goal_judge" {
			def, found = d, true
			break
		}
	}
	if !found {
		t.Fatal("role_goal_judge is not in the catalog")
	}
	if def.Strategy != nanoprotocol.StrategyOneshot {
		t.Errorf("strategy = %q, want oneshot", def.Strategy)
	}
	if def.Context != nanoprotocol.ContextNone {
		t.Errorf("context = %q, want none: the judge sees only what the tool hands it", def.Context)
	}
	if def.Output != nanoprotocol.OutputCaller {
		t.Errorf("output = %q, want caller", def.Output)
	}
	if def.SessionBinding != nanoprotocol.SessionBindingScratch {
		t.Errorf("binding = %q, want scratch", def.SessionBinding)
	}
	if got := def.Settings["judge_temperature"]; got != "0" {
		t.Errorf("judge_temperature = %q, want \"0\"", got)
	}
	if len(def.Capabilities) != 0 {
		t.Errorf("the judge declares %d capabilities; its belt is the empty list in CompleteBelts", len(def.Capabilities))
	}
}

// TestGoalJudgeHasAnEmptyBelt keeps "no tools" true at the belt level too.
// Absent means the catalog default, and the catalog default is on, so a role
// that wants nothing has to say so explicitly.
func TestGoalJudgeHasAnEmptyBelt(t *testing.T) {
	found := false
	for _, belt := range CompleteBelts() {
		if belt.TypeID != "role_goal_judge" {
			continue
		}
		found = true
		if len(belt.BuiltinTools) != 0 {
			t.Errorf("builtin tools = %v, want none", belt.BuiltinTools)
		}
		if belt.McpEnabled {
			t.Error("the judge must not reach an MCP server")
		}
	}
	if !found {
		t.Fatal("role_goal_judge has no belt entry: it would inherit every tool")
	}
}
