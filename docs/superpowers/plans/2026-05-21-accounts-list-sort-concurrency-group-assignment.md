# Accounts List Sort, Concurrency Column, and Ungrouped Assignment Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make account-list sorting fully backend-driven for `updatedAt`, show concurrency in its own column, and add a one-click action to assign all ungrouped accounts to an existing group.

**Architecture:** Keep the existing account-list API as the single source of truth for paging and filters, extend its sort key set to include `updatedAt`, and add a dedicated batch group-assignment endpoint that updates both the database and in-memory scheduler state. On the UI, keep the table query-driven, add a dedicated concurrency column, and surface the new batch action inside group management so the account list stays paged while bulk operations still work on the full dataset.

**Tech Stack:** Go, Gin, database package (`database/account_groups.go`), React, TypeScript, existing admin API client, existing i18n files.

---

### Task 1: Backend sort support and ungrouped batch assignment

**Files:**
- Modify: `admin/accounts_list.go`
- Modify: `admin/accounts_list_test.go`
- Modify: `database/account_groups.go`
- Create: `database/account_groups_test.go`
- Modify: `admin/account_groups.go`
- Modify: `admin/handler.go`
- Create: `admin/account_groups_test.go`

- [ ] **Step 1: Write the failing tests**

```go
func TestParseAccountListQueryAllowsUpdatedAtSort(t *testing.T) {
    // sort_key=updatedAt should parse successfully and preserve sort_dir.
}

func TestAssignUngroupedAccountsToGroup(t *testing.T) {
    // Arrange 3 active accounts:
    // - account 1: no groups
    // - account 2: no groups
    // - account 3: already in another group
    // Act: assign the target group to all ungrouped accounts
    // Assert:
    // - returned assigned IDs are [1, 2]
    // - account_group_members contains target group for accounts 1 and 2
    // - account 3 is unchanged
}

func TestAssignUngroupedAccountsEndpoint(t *testing.T) {
    // POST /api/admin/account-groups/:id/assign-ungrouped
    // should return 200 and the assigned count
}
```

- [ ] **Step 2: Run the focused tests to confirm they fail**

Run:
```bash
go test ./admin -run 'TestParseAccountListQueryAllowsUpdatedAtSort|TestAssignUngroupedAccountsToGroup|TestAssignUngroupedAccountsEndpoint' -v
```
Expected: fail before the new behavior exists.

- [ ] **Step 3: Implement the backend changes**

```go
// admin/accounts_list.go
// Accept "updatedAt" as a valid sort key and sort by accountResponse.UpdatedAt.

// database/account_groups.go
// Add AssignUngroupedAccountsToGroup(ctx, groupID int64) ([]int64, error) that:
// 1. selects active accounts with no group memberships
// 2. inserts (account_id, group_id) rows for the provided group
// 3. returns the affected account IDs

// admin/account_groups.go
// Add AssignUngroupedAccountsToGroup(c *gin.Context) that validates the group
// exists, calls the database helper, and applies the new group membership to runtime store accounts via
// h.store.ApplyAccountGroups(dbID, []int64{groupID}) when store is present.

// admin/handler.go
// Register POST /api/admin/account-groups/:id/assign-ungrouped
```

- [ ] **Step 4: Run the focused tests again**

Run:
```bash
go test ./admin -run 'TestParseAccountListQueryAllowsUpdatedAtSort|TestAssignUngroupedAccountsToGroup|TestAssignUngroupedAccountsEndpoint' -v
```
Expected: PASS.

- [ ] **Step 5: Run the full backend suite**

Run:
```bash
go test ./...
```
Expected: PASS.

### Task 2: Frontend updatedAt sorting, concurrency column, and group action

**Files:**
- Modify: `frontend/src/types.ts`
- Modify: `frontend/src/api.ts`
- Modify: `frontend/src/pages/Accounts.tsx`
- Modify: `frontend/src/locales/zh.json`
- Modify: `frontend/src/locales/en.json`

- [ ] **Step 1: Make the UI changes against the current type surface**

```ts
// frontend/src/types.ts
// Add "updatedAt" to AccountListQuery.sort_key.

// frontend/src/api.ts
// Add api.assignUngroupedAccountsToGroup(groupId: number).

// frontend/src/pages/Accounts.tsx
// - Add a dedicated "concurrency" column to ACCOUNT_TABLE_COLUMNS and the column settings labels.
// - Move the active request visualization out of the status column into the new concurrency column.
// - Make the updatedAt header sortable, reusing the same backend sort-key pattern as importTime.
// - Add a "一键分配未分组账号" action in the group manager for each group row.
// - Confirm before calling the new API, then reload accounts and groups on success.

// frontend/src/locales/zh.json / en.json
// Add labels for:
// - accounts.assignUngrouped
// - accounts.assignUngroupedConfirm
// - accounts.assignUngroupedDone
// - accounts.assignUngroupedFailed
```

- [ ] **Step 2: Run type checking to catch any missing wiring**

Run:
```bash
cd frontend && npm run typecheck
```
Expected: PASS.

- [ ] **Step 3: Run the production build**

Run:
```bash
cd frontend && npm run build
```
Expected: PASS.

### Task 3: Final verification

**Files:**
- No new code files; verify the completed diff.

- [ ] **Step 1: Re-run the full backend and frontend checks**

Run:
```bash
go test ./...
cd frontend && npm run typecheck && npm run build
```
Expected: all commands pass with no new errors.

- [ ] **Step 2: Review the diff for scope**

Confirm the final diff only includes:
- `updatedAt` sorting
- the new ungrouped-to-group batch action
- the dedicated concurrency column
- the related locale/type/API wiring

- [ ] **Step 3: Commit when the implementation is clean**

```bash
git add admin database frontend/src docs/superpowers/plans/2026-05-21-accounts-list-sort-concurrency-group-assignment.md
git commit -m "feat: improve account list sorting and group assignment"
```
