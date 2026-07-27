package main

import "testing"

func TestMandatoryOutboxStartupPolicy(t *testing.T) {
	if !shouldStartOutbox("postgres", false) {
		t.Fatal("PostgreSQL must start mandatory outbox processing when optional workers are disabled")
	}
	if shouldStartOutbox("memory", false) {
		t.Fatal("memory mode may remain lightweight when optional workers are disabled")
	}
	if !shouldStartOutbox("memory", true) {
		t.Fatal("optional workers should enable memory-mode outbox processing")
	}
	if shouldStartOptionalJobs(false) {
		t.Fatal("periodic snapshots, archives, scans, polling, compaction and reconciliation must remain disabled")
	}
	if !shouldStartOptionalJobs(true) {
		t.Fatal("optional jobs should start when explicitly enabled")
	}
}
