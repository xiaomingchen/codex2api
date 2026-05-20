package admin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/codex2api/database"
	"github.com/gin-gonic/gin"
)

func TestAssignUngroupedAccountsEndpoint(t *testing.T) {
	gin.SetMode(gin.TestMode)

	dbPath := filepath.Join(t.TempDir(), "codex2api.db")
	db, err := database.New("sqlite", dbPath)
	if err != nil {
		t.Fatalf("database.New 返回错误: %v", err)
	}
	defer db.Close()

	ctx := context.Background()
	groupID, err := db.CreateAccountGroup(ctx, "Team A", "", "#2563eb", 0)
	if err != nil {
		t.Fatalf("CreateAccountGroup 返回错误: %v", err)
	}
	accountID, err := db.InsertAccount(ctx, "account-1", "rt-1", "")
	if err != nil {
		t.Fatalf("InsertAccount 返回错误: %v", err)
	}
	if _, err := db.InsertAccount(ctx, "account-2", "rt-2", ""); err != nil {
		t.Fatalf("InsertAccount 返回错误: %v", err)
	}

	handler := &Handler{db: db}
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	groupIDStr := strconv.FormatInt(groupID, 10)
	c.Params = gin.Params{{Key: "id", Value: groupIDStr}}
	c.Request = httptest.NewRequest(http.MethodPost, "/api/admin/account-groups/"+groupIDStr+"/assign-ungrouped", nil)

	handler.AssignUngroupedAccountsToGroup(c)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", recorder.Code, http.StatusOK, recorder.Body.String())
	}

	var payload map[string]interface{}
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got := payload["message"]; got != "未分组账号已分配到分组" {
		t.Fatalf("message = %v, want 未分组账号已分配到分组", got)
	}
	if got := int(payload["assigned"].(float64)); got != 2 {
		t.Fatalf("assigned = %d, want 2", got)
	}

	memberships, err := db.ListAccountGroupMemberships(ctx)
	if err != nil {
		t.Fatalf("ListAccountGroupMemberships 返回错误: %v", err)
	}
	if got := memberships[accountID]; len(got) != 1 || got[0] != groupID {
		t.Fatalf("memberships = %v, want account assigned to group %d", memberships, groupID)
	}
}
