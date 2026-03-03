package runtime

import "testing"

func TestContextAppendLog(t *testing.T) {
	ctx := NewContext()

	// Initially empty.
	if logs := ctx.Logs(); len(logs) != 0 {
		t.Fatalf("expected 0 logs, got %d", len(logs))
	}

	// Append entries.
	ctx.AppendLog("first entry")
	ctx.AppendLog("second entry")

	logs := ctx.Logs()
	if len(logs) != 2 {
		t.Fatalf("expected 2 logs, got %d", len(logs))
	}
	if logs[0] != "first entry" || logs[1] != "second entry" {
		t.Fatalf("unexpected logs: %v", logs)
	}
}

func TestContextLogsIsCopy(t *testing.T) {
	ctx := NewContext()
	ctx.AppendLog("entry")

	logs := ctx.Logs()
	logs[0] = "modified"

	// Original should be unchanged.
	if ctx.Logs()[0] != "entry" {
		t.Fatal("Logs() should return a copy")
	}
}

func TestContextCloneIncludesLogs(t *testing.T) {
	ctx := NewContext()
	ctx.Set("key", "val")
	ctx.AppendLog("log1")
	ctx.AppendLog("log2")

	clone := ctx.Clone()

	// Clone should have the same logs.
	logs := clone.Logs()
	if len(logs) != 2 || logs[0] != "log1" || logs[1] != "log2" {
		t.Fatalf("clone logs = %v, want [log1, log2]", logs)
	}

	// Mutating clone should not affect original.
	clone.AppendLog("log3")
	if len(ctx.Logs()) != 2 {
		t.Fatal("appending to clone should not affect original")
	}
}
