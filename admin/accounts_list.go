package admin

import (
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/codex2api/auth"
	"github.com/gin-gonic/gin"
)

const (
	defaultAccountsPageSize = 20
	maxAccountsPageSize     = 100
	ungroupedGroupFilter    = "ungrouped"
)

type accountListQuery struct {
	Page      int
	PageSize  int
	All       bool
	Status    string
	Plan      string
	Search    string
	Tag       string
	GroupID   int64
	GroupMode string
	SortKey   string
	SortDir   string
}

func parseAccountListQuery(c *gin.Context) (accountListQuery, bool) {
	query := accountListQuery{
		Page:     1,
		PageSize: defaultAccountsPageSize,
		Status:   strings.ToLower(strings.TrimSpace(c.Query("status"))),
		Plan:     strings.ToLower(strings.TrimSpace(c.Query("plan"))),
		Search:   strings.TrimSpace(c.Query("q")),
		Tag:      strings.TrimSpace(c.Query("tag")),
		SortKey:  strings.TrimSpace(c.Query("sort_key")),
		SortDir:  strings.ToLower(strings.TrimSpace(c.Query("sort_dir"))),
	}

	if raw := strings.TrimSpace(c.Query("all")); raw != "" {
		query.All = raw == "1" || strings.EqualFold(raw, "true")
	}

	if raw := strings.TrimSpace(c.Query("page")); raw != "" {
		if page, err := strconv.Atoi(raw); err == nil && page > 0 {
			query.Page = page
		}
	}
	if raw := strings.TrimSpace(c.Query("page_size")); raw != "" {
		if pageSize, err := strconv.Atoi(raw); err == nil && pageSize > 0 {
			if pageSize > maxAccountsPageSize {
				pageSize = maxAccountsPageSize
			}
			query.PageSize = pageSize
		}
	}

	switch query.SortKey {
	case "", "score":
		query.SortKey = "score"
	case "requests", "usage", "importTime", "updatedAt":
	default:
		query.SortKey = "score"
	}

	if query.SortDir != "asc" && query.SortDir != "desc" {
		query.SortDir = "desc"
	}

	switch query.Status {
	case "", "all", "normal", "rate_limited", "abnormal", "banned", "error", "disabled", "locked":
	default:
		query.Status = "all"
	}

	if query.Plan == "" {
		query.Plan = "all"
	}

	groupRaw := strings.TrimSpace(c.Query("group_id"))
	switch {
	case groupRaw == "", strings.EqualFold(groupRaw, "all"):
		query.GroupMode = "all"
	case strings.EqualFold(groupRaw, ungroupedGroupFilter):
		query.GroupMode = ungroupedGroupFilter
	default:
		groupID, err := strconv.ParseInt(groupRaw, 10, 64)
		if err != nil || groupID <= 0 {
			writeError(c, 400, "group_id 参数无效")
			return accountListQuery{}, false
		}
		query.GroupMode = "group"
		query.GroupID = groupID
	}

	return query, true
}

func filterAccountResponses(accounts []accountResponse, query accountListQuery) []accountResponse {
	if len(accounts) == 0 {
		return nil
	}
	filtered := make([]accountResponse, 0, len(accounts))
	search := strings.ToLower(query.Search)
	for _, account := range accounts {
		if !matchesAccountStatusFilter(account, query.Status) {
			continue
		}
		if query.Plan != "" && query.Plan != "all" {
			plan := strings.ToLower(strings.TrimSpace(account.PlanType))
			if plan != query.Plan {
				continue
			}
		}
		if search != "" {
			email := strings.ToLower(strings.TrimSpace(account.Email))
			name := strings.ToLower(strings.TrimSpace(account.Name))
			if !strings.Contains(email, search) && !strings.Contains(name, search) {
				continue
			}
		}
		if query.Tag != "" && !containsString(account.Tags, query.Tag) {
			continue
		}
		switch query.GroupMode {
		case ungroupedGroupFilter:
			if len(account.GroupIDs) > 0 {
				continue
			}
		case "group":
			if !containsInt64(account.GroupIDs, query.GroupID) {
				continue
			}
		}
		filtered = append(filtered, account)
	}
	return filtered
}

func matchesAccountStatusFilter(account accountResponse, status string) bool {
	switch status {
	case "", "all":
		return true
	case "normal":
		if isAccountAbnormal(account) || isAccountRateLimited(account) {
			return false
		}
		return account.Status == "active" || account.Status == "ready"
	case "rate_limited":
		if isAccountAbnormal(account) {
			return false
		}
		return isAccountRateLimited(account)
	case "abnormal":
		return isAccountAbnormal(account)
	case "banned":
		return account.Status == "unauthorized"
	case "error":
		return account.Status == "error"
	case "disabled":
		return !account.Enabled
	case "locked":
		return account.Locked
	default:
		return true
	}
}

func sortAccountResponses(accounts []accountResponse, sortKey, sortDir string) {
	if len(accounts) <= 1 {
		return
	}
	desc := sortDir != "asc"
	sort.SliceStable(accounts, func(i, j int) bool {
		left := accounts[i]
		right := accounts[j]

		var diff float64
		switch sortKey {
		case "requests":
			diff = float64((left.SuccessRequests + left.ErrorRequests) - (right.SuccessRequests + right.ErrorRequests))
		case "usage":
			diff = usageSortValue(left.UsagePercent7d) - usageSortValue(right.UsagePercent7d)
		case "importTime":
			diff = float64(compareRFC3339(left.CreatedAt, right.CreatedAt))
		case "updatedAt":
			diff = float64(compareRFC3339(left.UpdatedAt, right.UpdatedAt))
		case "score":
			fallthrough
		default:
			diff = left.DispatchScore - right.DispatchScore
		}

		if diff == 0 {
			if left.ID == right.ID {
				return false
			}
			if desc {
				return left.ID > right.ID
			}
			return left.ID < right.ID
		}
		if desc {
			return diff > 0
		}
		return diff < 0
	})
}

func paginateAccountResponses(accounts []accountResponse, query accountListQuery) ([]accountResponse, int, int, int) {
	total := len(accounts)
	if query.All {
		pageSize := total
		if pageSize == 0 {
			pageSize = 0
		}
		return accounts, 1, pageSize, 1
	}

	totalPages := 1
	if total > 0 {
		totalPages = (total + query.PageSize - 1) / query.PageSize
	}
	page := query.Page
	if page > totalPages {
		page = totalPages
	}
	if page < 1 {
		page = 1
	}

	start := (page - 1) * query.PageSize
	if start > total {
		start = total
	}
	end := start + query.PageSize
	if end > total {
		end = total
	}
	return accounts[start:end], page, query.PageSize, totalPages
}

func buildAccountListSummary(accounts []accountResponse) accountListSummaryResponse {
	summary := accountListSummaryResponse{
		TotalAccounts: len(accounts),
	}
	for _, account := range accounts {
		if account.Status == "unauthorized" {
			summary.BannedAccounts++
		}
		if account.Status == "error" {
			summary.ErrorAccounts++
		}
		if !account.Enabled {
			summary.DisabledAccounts++
		}
		if account.Locked {
			summary.LockedAccounts++
		}
		if isAccountRateLimited(account) && !isAccountAbnormal(account) {
			summary.RateLimitedAccounts++
			switch getAccountRateLimitWindow(account) {
			case "5h":
				summary.RateLimited5hAccounts++
			case "7d":
				summary.RateLimited7dAccounts++
			}
		}
		if isAccountAbnormal(account) {
			summary.AbnormalAccounts++
		}
		switch account.HealthTier {
		case "healthy":
			summary.HealthyAccounts++
		case "warm":
			summary.WarmAccounts++
		case "risky":
			summary.RiskyAccounts++
		}
		if auth.IsPlusOrHigherPlan(account.PlanType) && !account.Locked {
			summary.SubscriptionAccountsToLock++
		}
	}
	summary.NormalAccounts = summary.TotalAccounts - summary.AbnormalAccounts - summary.RateLimitedAccounts
	if summary.NormalAccounts < 0 {
		summary.NormalAccounts = 0
	}
	return summary
}

func collectAccountTags(accounts []accountResponse) []string {
	if len(accounts) == 0 {
		return []string{}
	}
	seen := make(map[string]struct{})
	tags := make([]string, 0)
	for _, account := range accounts {
		for _, tag := range account.Tags {
			tag = strings.TrimSpace(tag)
			if tag == "" {
				continue
			}
			if _, ok := seen[tag]; ok {
				continue
			}
			seen[tag] = struct{}{}
			tags = append(tags, tag)
		}
	}
	sort.Slice(tags, func(i, j int) bool {
		return strings.ToLower(tags[i]) < strings.ToLower(tags[j])
	})
	return tags
}

func isAccountAbnormal(account accountResponse) bool {
	return account.Status == "unauthorized" || account.Status == "error" || !account.Enabled
}

func isAccountRateLimited(account accountResponse) bool {
	return getAccountRateLimitWindow(account) != ""
}

func getAccountRateLimitWindow(account accountResponse) string {
	status := strings.ToLower(strings.TrimSpace(account.Status))
	reason := strings.ToLower(strings.TrimSpace(account.CooldownReason))
	explicitlyRateLimited := status == "rate_limited" ||
		status == "usage_exhausted" ||
		status == "rate_limited_5h" ||
		status == "rate_limited_7d" ||
		reason == "rate_limited" ||
		reason == "rate_limited_5h" ||
		reason == "rate_limited_7d"

	has7dLimit := isActiveUsageWindowExhausted(account.UsagePercent7d, account.Reset7dAt)
	has5hLimit := isPremiumUsagePlan(account.PlanType) && isActiveUsageWindowExhausted(account.UsagePercent5h, account.Reset5hAt)

	if status == "usage_exhausted" || status == "rate_limited_7d" || reason == "rate_limited_7d" || has7dLimit {
		return "7d"
	}
	if status == "rate_limited_5h" || reason == "rate_limited_5h" || has5hLimit {
		return "5h"
	}
	if explicitlyRateLimited {
		return "5h"
	}
	return ""
}

func isPremiumUsagePlan(planType string) bool {
	switch auth.NormalizePlanType(planType) {
	case "plus", "pro", "team":
		return true
	default:
		return false
	}
}

func isActiveUsageWindowExhausted(value *float64, resetAt string) bool {
	if value == nil || *value < 100 {
		return false
	}
	if resetAt == "" {
		return true
	}
	resetTime, err := time.Parse(time.RFC3339, resetAt)
	if err != nil {
		return true
	}
	return resetTime.After(time.Now())
}

func usageSortValue(value *float64) float64 {
	if value == nil {
		return -1
	}
	return *value
}

func compareRFC3339(left, right string) int {
	leftTime, leftErr := time.Parse(time.RFC3339, left)
	rightTime, rightErr := time.Parse(time.RFC3339, right)
	switch {
	case leftErr == nil && rightErr == nil:
		if leftTime.Before(rightTime) {
			return -1
		}
		if leftTime.After(rightTime) {
			return 1
		}
		return 0
	case left < right:
		return -1
	case left > right:
		return 1
	default:
		return 0
	}
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
