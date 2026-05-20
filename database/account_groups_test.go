package database

import (
	"context"
	"path/filepath"
	"testing"
)

func TestAssignUngroupedAccountsToGroup(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "codex2api.db")

	db, err := New("sqlite", dbPath)
	if err != nil {
		t.Fatalf("New(sqlite) 返回错误: %v", err)
	}
	defer db.Close()

	ctx := context.Background()
	groupID, err := db.CreateAccountGroup(ctx, "Team A", "", "#2563eb", 0)
	if err != nil {
		t.Fatalf("CreateAccountGroup 返回错误: %v", err)
	}
	otherGroupID, err := db.CreateAccountGroup(ctx, "Team B", "", "#16a34a", 1)
	if err != nil {
		t.Fatalf("CreateAccountGroup 返回错误: %v", err)
	}

	account1, err := db.InsertAccount(ctx, "account-1", "rt-1", "")
	if err != nil {
		t.Fatalf("InsertAccount 1 返回错误: %v", err)
	}
	account2, err := db.InsertAccount(ctx, "account-2", "rt-2", "")
	if err != nil {
		t.Fatalf("InsertAccount 2 返回错误: %v", err)
	}
	account3, err := db.InsertAccount(ctx, "account-3", "rt-3", "")
	if err != nil {
		t.Fatalf("InsertAccount 3 返回错误: %v", err)
	}
	if err := db.SetAccountGroups(ctx, account3, []int64{otherGroupID}); err != nil {
		t.Fatalf("SetAccountGroups 返回错误: %v", err)
	}

	assignedIDs, err := db.AssignUngroupedAccountsToGroup(ctx, groupID)
	if err != nil {
		t.Fatalf("AssignUngroupedAccountsToGroup 返回错误: %v", err)
	}

	wantIDs := []int64{account1, account2}
	if len(assignedIDs) != len(wantIDs) {
		t.Fatalf("assignedIDs = %v, want %v", assignedIDs, wantIDs)
	}
	for i := range wantIDs {
		if assignedIDs[i] != wantIDs[i] {
			t.Fatalf("assignedIDs[%d] = %d, want %d (full=%v)", i, assignedIDs[i], wantIDs[i], assignedIDs)
		}
	}

	memberships, err := db.ListAccountGroupMemberships(ctx)
	if err != nil {
		t.Fatalf("ListAccountGroupMemberships 返回错误: %v", err)
	}
	if got := memberships[account1]; len(got) != 1 || got[0] != groupID {
		t.Fatalf("account1 memberships = %v, want [%d]", got, groupID)
	}
	if got := memberships[account2]; len(got) != 1 || got[0] != groupID {
		t.Fatalf("account2 memberships = %v, want [%d]", got, groupID)
	}
	if got := memberships[account3]; len(got) != 1 || got[0] != otherGroupID {
		t.Fatalf("account3 memberships = %v, want [%d]", got, otherGroupID)
	}
}
