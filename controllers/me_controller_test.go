package controllers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"ccrt_sever/global"
	"ccrt_sever/models"

	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestListCareContactsBackfillsLegacyEmergencyContact(t *testing.T) {
	db := setupCareContactControllerTestDB(t)
	user := seedCareContactTestUser(t, db, "Legacy Contact", "13800000001")

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/me/care-contacts", nil)
	ctx.Set("username", user.Username)

	ListCareContacts(ctx)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body=%s", recorder.Code, recorder.Body.String())
	}

	payload := decodeCareContactJSONBody(t, recorder)
	contacts, ok := payload["contacts"].([]any)
	if !ok || len(contacts) != 1 {
		t.Fatalf("expected one backfilled contact, got %#v", payload["contacts"])
	}
	first := contacts[0].(map[string]any)
	if first["is_primary"] != true || first["phone"] != "13800000001" {
		t.Fatalf("unexpected backfilled contact: %#v", first)
	}
}

func TestCreateSetPrimaryAndDeleteCareContactsSyncLegacyFields(t *testing.T) {
	db := setupCareContactControllerTestDB(t)
	user := seedCareContactTestUser(t, db, "", "")

	firstID := createCareContactThroughController(t, user.Username, map[string]any{
		"name":         "女儿",
		"relationship": "child",
		"phone":        "13800000011",
		"note":         "白天优先联系",
	})
	secondID := createCareContactThroughController(t, user.Username, map[string]any{
		"name":         "医生",
		"relationship": "doctor",
		"phone":        "13800000022",
		"note":         "工作日门诊",
	})

	var current models.User
	if err := db.First(&current, user.ID).Error; err != nil {
		t.Fatalf("load user after create: %v", err)
	}
	if current.EmergencyContactPhone != "13800000011" {
		t.Fatalf("expected first contact to seed legacy emergency phone, got %#v", current.EmergencyContactPhone)
	}

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/me/care-contacts/%d/primary", secondID), nil)
	ctx.Params = []gin.Param{{Key: "id", Value: fmt.Sprintf("%d", secondID)}}
	ctx.Set("username", user.Username)
	SetPrimaryCareContact(ctx)
	if recorder.Code != http.StatusOK {
		t.Fatalf("set primary failed: %d %s", recorder.Code, recorder.Body.String())
	}

	if err := db.First(&current, user.ID).Error; err != nil {
		t.Fatalf("load user after set primary: %v", err)
	}
	if current.EmergencyContactPhone != "13800000022" {
		t.Fatalf("expected legacy emergency phone to follow new primary, got %#v", current.EmergencyContactPhone)
	}

	deleteRecorder := httptest.NewRecorder()
	deleteCtx, _ := gin.CreateTestContext(deleteRecorder)
	deleteCtx.Request = httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/api/me/care-contacts/%d", secondID), nil)
	deleteCtx.Params = []gin.Param{{Key: "id", Value: fmt.Sprintf("%d", secondID)}}
	deleteCtx.Set("username", user.Username)
	DeleteCareContact(deleteCtx)
	if deleteRecorder.Code != http.StatusOK {
		t.Fatalf("delete primary failed: %d %s", deleteRecorder.Code, deleteRecorder.Body.String())
	}

	if err := db.First(&current, user.ID).Error; err != nil {
		t.Fatalf("load user after delete: %v", err)
	}
	if current.EmergencyContactPhone != "13800000011" {
		t.Fatalf("expected fallback primary to sync legacy emergency phone, got %#v", current.EmergencyContactPhone)
	}

	var contacts []models.CareContact
	if err := db.Where("user_id = ?", user.ID).Order("created_at ASC").Find(&contacts).Error; err != nil {
		t.Fatalf("load contacts after delete: %v", err)
	}
	if len(contacts) != 1 || contacts[0].ID != firstID || !contacts[0].IsPrimary {
		t.Fatalf("expected first contact to remain as primary, got %#v", contacts)
	}
}

func TestUpdateEmergencyContactKeepsCompatibilityWithPrimaryContact(t *testing.T) {
	db := setupCareContactControllerTestDB(t)
	user := seedCareContactTestUser(t, db, "", "")
	createCareContactThroughController(t, user.Username, map[string]any{
		"name":         "儿子",
		"relationship": "child",
		"phone":        "13800000031",
	})

	body, _ := json.Marshal(map[string]any{
		"name":  "妻子",
		"phone": "13800000032",
	})

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPut, "/api/me/emergency", bytes.NewReader(body))
	ctx.Request.Header.Set("Content-Type", "application/json")
	ctx.Set("username", user.Username)
	UpdateEmergencyContact(ctx)

	if recorder.Code != http.StatusOK {
		t.Fatalf("compat emergency update failed: %d %s", recorder.Code, recorder.Body.String())
	}

	var current models.User
	if err := db.First(&current, user.ID).Error; err != nil {
		t.Fatalf("load user after compat update: %v", err)
	}
	if current.EmergencyContactName != "妻子" || current.EmergencyContactPhone != "13800000032" {
		t.Fatalf("unexpected legacy fields after compat update: %#v", current)
	}

	var primary models.CareContact
	if err := db.Where("user_id = ? AND is_primary = ?", user.ID, true).First(&primary).Error; err != nil {
		t.Fatalf("load primary contact after compat update: %v", err)
	}
	if primary.Name != "妻子" || primary.Phone != "13800000032" {
		t.Fatalf("unexpected primary contact after compat update: %#v", primary)
	}
}

func setupCareContactControllerTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	gin.SetMode(gin.TestMode)
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite db: %v", err)
	}
	if err := db.AutoMigrate(&models.User{}, &models.CareContact{}); err != nil {
		t.Fatalf("automigrate: %v", err)
	}

	originalDB := global.Db
	global.Db = db
	t.Cleanup(func() {
		global.Db = originalDB
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	return db
}

func seedCareContactTestUser(t *testing.T, db *gorm.DB, legacyName string, legacyPhone string) models.User {
	t.Helper()

	user := models.User{
		Username:              fmt.Sprintf("tester_%s", t.Name()),
		Phone:                 fmt.Sprintf("139%s", fmt.Sprintf("%08d", len(t.Name()))),
		Password:              "secret",
		EmergencyContactName:  legacyName,
		EmergencyContactPhone: legacyPhone,
	}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	return user
}

func createCareContactThroughController(t *testing.T, username string, payload map[string]any) uint {
	t.Helper()

	body, _ := json.Marshal(payload)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/me/care-contacts", bytes.NewReader(body))
	ctx.Request.Header.Set("Content-Type", "application/json")
	ctx.Set("username", username)
	CreateCareContact(ctx)
	if recorder.Code != http.StatusOK {
		t.Fatalf("create care contact failed: %d %s", recorder.Code, recorder.Body.String())
	}
	response := decodeCareContactJSONBody(t, recorder)
	contact := response["contact"].(map[string]any)
	return uint(contact["id"].(float64))
}

func decodeCareContactJSONBody(t *testing.T, recorder *httptest.ResponseRecorder) map[string]any {
	t.Helper()

	var payload map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode json: %v", err)
	}
	return payload
}
