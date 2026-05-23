package admin

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestParseAccountListQuerySupportsUngrouped(t *testing.T) {
	gin.SetMode(gin.TestMode)

	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(
		"GET",
		"/api/admin/accounts?page=2&page_size=50&status=normal&plan=pro&q=test&tag=ops&group_id=ungrouped&sort_key=requests&sort_dir=asc",
		nil,
	)

	query, ok := parseAccountListQuery(ctx)
	if !ok {
		t.Fatal("parseAccountListQuery returned false")
	}
	if query.Page != 2 || query.PageSize != 50 || query.Status != "normal" || query.Plan != "pro" {
		t.Fatalf("unexpected query: %#v", query)
	}
	if query.Search != "test" || query.Tag != "ops" || query.GroupMode != ungroupedGroupFilter {
		t.Fatalf("unexpected query filters: %#v", query)
	}
	if query.SortKey != "requests" || query.SortDir != "asc" {
		t.Fatalf("unexpected sort query: %#v", query)
	}
}

func TestParseAccountListQuerySupportsUpdatedAtSort(t *testing.T) {
	gin.SetMode(gin.TestMode)

	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(
		"GET",
		"/api/admin/accounts?sort_key=updatedAt&sort_dir=desc",
		nil,
	)

	query, ok := parseAccountListQuery(ctx)
	if !ok {
		t.Fatal("parseAccountListQuery returned false")
	}
	if query.SortKey != "updatedAt" || query.SortDir != "desc" {
		t.Fatalf("unexpected sort query: %#v", query)
	}
}

func TestParseAccountListQuerySupportsCooldownUntilSort(t *testing.T) {
	gin.SetMode(gin.TestMode)

	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(
		"GET",
		"/api/admin/accounts?sort_key=cooldownUntil&sort_dir=asc",
		nil,
	)

	query, ok := parseAccountListQuery(ctx)
	if !ok {
		t.Fatal("parseAccountListQuery returned false")
	}
	if query.SortKey != "cooldownUntil" || query.SortDir != "asc" {
		t.Fatalf("unexpected sort query: %#v", query)
	}
}

func TestAccountListHelpersFilterAndPaginate(t *testing.T) {
	accounts := []accountResponse{
		{ID: 1, Name: "Alpha", Email: "alpha@example.com", Status: "active", PlanType: "pro", Tags: []string{"ops"}, HealthTier: "healthy", Enabled: true, DispatchScore: 30},
		{ID: 2, Name: "Beta", Email: "beta@example.com", Status: "active", PlanType: "free", Tags: []string{"ops"}, HealthTier: "warm", Enabled: true, UsagePercent7d: float64Ptr(100), Reset7dAt: "2099-01-01T00:00:00Z"},
		{ID: 3, Name: "Gamma", Email: "gamma@example.com", Status: "error", PlanType: "free", Enabled: false, Locked: true, GroupIDs: []int64{8}},
	}

	summary := buildAccountListSummary(accounts)
	if summary.TotalAccounts != 3 || summary.NormalAccounts != 1 || summary.RateLimitedAccounts != 1 || summary.AbnormalAccounts != 1 || summary.SubscriptionAccountsToLock != 1 {
		t.Fatalf("unexpected summary: %#v", summary)
	}

	filtered := filterAccountResponses(accounts, accountListQuery{
		Status:    "normal",
		Plan:      "pro",
		Search:    "alpha",
		Tag:       "ops",
		GroupMode: "all",
		SortKey:   "score",
		SortDir:   "desc",
	})
	if len(filtered) != 1 || filtered[0].ID != 1 {
		t.Fatalf("unexpected filtered result: %#v", filtered)
	}

	groupFiltered := filterAccountResponses(accounts, accountListQuery{
		GroupMode: ungroupedGroupFilter,
	})
	if len(groupFiltered) != 2 {
		t.Fatalf("unexpected ungrouped filter result: %#v", groupFiltered)
	}

	paged, page, pageSize, totalPages := paginateAccountResponses(accounts, accountListQuery{
		Page:     2,
		PageSize: 2,
	})
	if page != 2 || pageSize != 2 || totalPages != 2 || len(paged) != 1 || paged[0].ID != 3 {
		t.Fatalf("unexpected pagination result: page=%d pageSize=%d totalPages=%d paged=%#v", page, pageSize, totalPages, paged)
	}
}

func TestSortAccountResponsesByUpdatedAt(t *testing.T) {
	accounts := []accountResponse{
		{ID: 1, UpdatedAt: "2024-01-02T00:00:00Z"},
		{ID: 2, UpdatedAt: "2024-02-02T00:00:00Z"},
	}

	sortAccountResponses(accounts, "updatedAt", "asc")
	if accounts[0].ID != 1 || accounts[1].ID != 2 {
		t.Fatalf("ascending updatedAt sort failed: %#v", accounts)
	}

	sortAccountResponses(accounts, "updatedAt", "desc")
	if accounts[0].ID != 2 || accounts[1].ID != 1 {
		t.Fatalf("descending updatedAt sort failed: %#v", accounts)
	}
}

func TestSortAccountResponsesByCooldownUntil(t *testing.T) {
	accounts := []accountResponse{
		{ID: 1, CooldownUntil: "2024-02-02T00:00:00Z"},
		{ID: 2, CooldownUntil: ""},
		{ID: 3, CooldownUntil: "2024-01-02T00:00:00Z"},
		{ID: 4, CooldownUntil: ""},
	}

	sortAccountResponses(accounts, "cooldownUntil", "asc")
	if accounts[0].ID != 3 || accounts[1].ID != 1 || accounts[2].ID != 2 || accounts[3].ID != 4 {
		t.Fatalf("ascending cooldownUntil sort failed: %#v", accounts)
	}

	sortAccountResponses(accounts, "cooldownUntil", "desc")
	if accounts[0].ID != 1 || accounts[1].ID != 3 || accounts[2].ID != 4 || accounts[3].ID != 2 {
		t.Fatalf("descending cooldownUntil sort failed: %#v", accounts)
	}
}

func float64Ptr(v float64) *float64 {
	return &v
}
