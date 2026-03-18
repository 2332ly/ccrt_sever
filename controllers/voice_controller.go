package controllers

import (
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"ccrt_sever/global"
	"ccrt_sever/models"
	"ccrt_sever/utils"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const (
	voiceCloneNotConfiguredCode = "VOICE_CLONE_NOT_CONFIGURED"
	voiceCloneInvalidSampleCode = "VOICE_CLONE_INVALID_SAMPLE"
	voiceCloneConsentRequired   = "VOICE_CLONE_CONSENT_REQUIRED"
	voiceCloneCreateFailedCode  = "VOICE_CLONE_CREATE_FAILED"
	voiceCloneNotReadyCode      = "VOICE_CLONE_NOT_READY"
	voiceCloneNotFoundCode      = "VOICE_CLONE_NOT_FOUND"

	voiceCloneVendorDashScope = "dashscope"
	voiceStatusCreating       = "creating"
	voiceStatusReady          = "ready"
	voiceStatusFailed         = "failed"
	voiceStatusDeleted        = "deleted"
	voiceSourceUpload         = "upload"
	voiceSourceRecord         = "record"
	voiceSampleLimitBytes     = 10 * 1024 * 1024
)

var (
	getCurrentVoiceUserFn             = getCurrentUser
	dashScopeVoiceCloneCreateFn       = utils.DashScopeCreateVoiceClone
	dashScopeVoiceCloneListFn         = utils.DashScopeListVoiceClones
	dashScopeVoiceCloneDeleteFn       = utils.DashScopeDeleteVoiceClone
	isDashScopeVoiceCloneConfiguredFn = utils.IsDashScopeVoiceCloneConfigured
)

type createVoiceProfileRequest struct {
	DisplayName      string
	Relationship     string
	SourceType       string
	ConsentConfirmed bool
	SampleDurationMs int64
}

// ListVoiceProfiles returns the current user's family voice candidates.
func ListVoiceProfiles(ctx *gin.Context) {
	user, err := getCurrentVoiceUserFn(ctx)
	if err != nil {
		utils.RespondError(ctx, http.StatusUnauthorized, "USER_NOT_FOUND", "User not found")
		return
	}

	profiles, err := listVoiceProfilesForUser(user.ID)
	if err != nil {
		utils.RespondError(ctx, http.StatusInternalServerError, "DB_ERROR", "Could not fetch voice profiles")
		return
	}
	refreshed, err := refreshVoiceProfiles(user.ID, profiles)
	if err == nil {
		profiles = refreshed
	}

	utils.RespondOK(ctx, buildVoiceProfileListPayload(profiles))
}

// CreateVoiceProfile uploads a family voice sample to DashScope.
func CreateVoiceProfile(ctx *gin.Context) {
	if !isDashScopeVoiceCloneConfiguredFn() {
		utils.RespondError(ctx, http.StatusServiceUnavailable, voiceCloneNotConfiguredCode, "voice clone service is not configured")
		return
	}
	user, err := getCurrentVoiceUserFn(ctx)
	if err != nil {
		utils.RespondError(ctx, http.StatusUnauthorized, "USER_NOT_FOUND", "User not found")
		return
	}

	req, err := parseCreateVoiceProfileRequest(ctx)
	if err != nil {
		utils.RespondError(ctx, http.StatusBadRequest, voiceCloneInvalidSampleCode, err.Error())
		return
	}
	if !req.ConsentConfirmed {
		utils.RespondError(ctx, http.StatusBadRequest, voiceCloneConsentRequired, "consent confirmation is required")
		return
	}

	fileHeader, err := ctx.FormFile("audio")
	if err != nil || fileHeader == nil {
		utils.RespondError(ctx, http.StatusBadRequest, voiceCloneInvalidSampleCode, "audio is required")
		return
	}
	fileName, contentType, data, err := readVoiceSample(fileHeader)
	if err != nil {
		utils.RespondError(ctx, http.StatusBadRequest, voiceCloneInvalidSampleCode, err.Error())
		return
	}

	now := time.Now()
	profile := models.VoiceProfile{
		UserID:             user.ID,
		DisplayName:        req.DisplayName,
		Relationship:       req.Relationship,
		Vendor:             voiceCloneVendorDashScope,
		Status:             voiceStatusCreating,
		SourceType:         req.SourceType,
		SampleFileName:     fileName,
		SampleDurationMs:   req.SampleDurationMs,
		ConsentConfirmedAt: &now,
	}
	if err := global.Db.Create(&profile).Error; err != nil {
		utils.RespondError(ctx, http.StatusInternalServerError, "DB_ERROR", "Could not create voice profile")
		return
	}

	voiceID, taskID, cloneErr := dashScopeVoiceCloneCreateFn(fileName, contentType, data, req.DisplayName, req.Relationship)
	if cloneErr != nil {
		_ = global.Db.Model(&profile).Updates(map[string]any{
			"status":     voiceStatusFailed,
			"last_error": trimVoiceError(cloneErr.Error()),
		}).Error
		utils.RespondError(ctx, http.StatusBadGateway, voiceCloneCreateFailedCode, "voice clone creation is unavailable")
		return
	}

	readyAt := time.Now()
	if err := global.Db.Model(&profile).Updates(map[string]any{
		"vendor_voice_id": voiceID,
		"vendor_task_id":  taskID,
		"status":          voiceStatusReady,
		"ready_at":        &readyAt,
		"last_error":      "",
	}).Error; err != nil {
		utils.RespondError(ctx, http.StatusInternalServerError, "DB_ERROR", "Could not update voice profile")
		return
	}
	profile.VendorVoiceID = voiceID
	profile.VendorTaskID = taskID
	profile.Status = voiceStatusReady
	profile.ReadyAt = &readyAt

	utils.RespondOK(ctx, gin.H{
		"id":           profile.ID,
		"status":       profile.Status,
		"display_name": profile.DisplayName,
		"relationship": profile.Relationship,
		"source_type":  profile.SourceType,
	})
}

// SetDefaultVoiceProfile sets a ready cloned voice as the user's global default.
func SetDefaultVoiceProfile(ctx *gin.Context) {
	user, err := getCurrentVoiceUserFn(ctx)
	if err != nil {
		utils.RespondError(ctx, http.StatusUnauthorized, "USER_NOT_FOUND", "User not found")
		return
	}

	profile, err := findVoiceProfileByID(user.ID, ctx.Param("id"))
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			utils.RespondError(ctx, http.StatusNotFound, voiceCloneNotFoundCode, "voice profile not found")
			return
		}
		utils.RespondError(ctx, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}
	if profile.Status != voiceStatusReady {
		utils.RespondError(ctx, http.StatusBadRequest, voiceCloneNotReadyCode, "voice profile is not ready")
		return
	}

	if err := global.Db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&models.VoiceProfile{}).
			Where("user_id = ?", user.ID).
			Update("is_default", false).Error; err != nil {
			return err
		}
		return tx.Model(&models.VoiceProfile{}).
			Where("id = ? AND user_id = ?", profile.ID, user.ID).
			Update("is_default", true).Error
	}); err != nil {
		utils.RespondError(ctx, http.StatusInternalServerError, "DB_ERROR", "Could not update default voice")
		return
	}

	utils.RespondOK(ctx, gin.H{
		"id":           profile.ID,
		"is_default":   true,
		"status":       profile.Status,
		"display_name": profile.DisplayName,
	})
}

// DeleteVoiceProfile removes a cloned voice from DashScope and local metadata.
func DeleteVoiceProfile(ctx *gin.Context) {
	user, err := getCurrentVoiceUserFn(ctx)
	if err != nil {
		utils.RespondError(ctx, http.StatusUnauthorized, "USER_NOT_FOUND", "User not found")
		return
	}

	profile, err := findVoiceProfileByID(user.ID, ctx.Param("id"))
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			utils.RespondError(ctx, http.StatusNotFound, voiceCloneNotFoundCode, "voice profile not found")
			return
		}
		utils.RespondError(ctx, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}

	if isDashScopeVoiceCloneConfiguredFn() && strings.TrimSpace(profile.VendorVoiceID) != "" && profile.Status != voiceStatusDeleted {
		if err := dashScopeVoiceCloneDeleteFn(profile.VendorVoiceID); err != nil && !isDashScopeVoiceMissingError(err) {
			utils.RespondError(ctx, http.StatusBadGateway, voiceServiceUnavailableCode, "voice clone deletion is unavailable")
			return
		}
	}

	if err := global.Db.Model(&models.VoiceProfile{}).
		Where("id = ? AND user_id = ?", profile.ID, user.ID).
		Updates(map[string]any{
			"status":     voiceStatusDeleted,
			"is_default": false,
		}).Error; err != nil {
		utils.RespondError(ctx, http.StatusInternalServerError, "DB_ERROR", "Could not delete voice profile")
		return
	}

	utils.RespondOK(ctx, gin.H{
		"id":         profile.ID,
		"deleted":    true,
		"is_default": false,
	})
}

func buildVoiceProfileListPayload(profiles []models.VoiceProfile) gin.H {
	payload := make([]gin.H, 0, len(profiles))
	var defaultID uint
	for _, profile := range profiles {
		if profile.IsDefault {
			defaultID = profile.ID
		}
		payload = append(payload, gin.H{
			"id":              profile.ID,
			"display_name":    profile.DisplayName,
			"relationship":    profile.Relationship,
			"status":          profile.Status,
			"is_default":      profile.IsDefault,
			"source_type":     profile.SourceType,
			"last_error":      profile.LastError,
			"created_at":      profile.CreatedAt,
			"vendor_voice_id": profile.VendorVoiceID,
		})
	}
	return gin.H{
		"default_voice_id": defaultID,
		"voices":           payload,
	}
}

func listVoiceProfilesForUser(userID uint) ([]models.VoiceProfile, error) {
	var profiles []models.VoiceProfile
	err := global.Db.
		Where("user_id = ? AND status <> ?", userID, voiceStatusDeleted).
		Order("is_default DESC, created_at DESC").
		Find(&profiles).Error
	return profiles, err
}

func refreshVoiceProfiles(userID uint, profiles []models.VoiceProfile) ([]models.VoiceProfile, error) {
	if !isDashScopeVoiceCloneConfiguredFn() || len(profiles) == 0 {
		return profiles, nil
	}
	vendorVoices, err := dashScopeVoiceCloneListFn()
	if err != nil {
		return profiles, err
	}

	voiceIDs := make(map[string]struct{}, len(vendorVoices))
	for _, item := range vendorVoices {
		voiceIDs[strings.TrimSpace(item.VoiceID)] = struct{}{}
	}

	now := time.Now()
	for _, profile := range profiles {
		voiceID := strings.TrimSpace(profile.VendorVoiceID)
		if voiceID == "" || profile.Status == voiceStatusDeleted {
			continue
		}
		_, exists := voiceIDs[voiceID]
		if exists && profile.Status == voiceStatusCreating {
			_ = global.Db.Model(&models.VoiceProfile{}).
				Where("id = ? AND user_id = ?", profile.ID, userID).
				Updates(map[string]any{
					"status":     voiceStatusReady,
					"ready_at":   &now,
					"last_error": "",
				}).Error
		}
		if !exists && profile.Status == voiceStatusReady {
			_ = global.Db.Model(&models.VoiceProfile{}).
				Where("id = ? AND user_id = ?", profile.ID, userID).
				Updates(map[string]any{
					"status":     voiceStatusFailed,
					"last_error": "voice not found in dashscope list",
					"is_default": false,
				}).Error
		}
	}
	return listVoiceProfilesForUser(userID)
}

func findDefaultVoiceProfile(userID uint) (models.VoiceProfile, bool, error) {
	var profile models.VoiceProfile
	err := global.Db.
		Where("user_id = ? AND is_default = ? AND status <> ?", userID, true, voiceStatusDeleted).
		Order("updated_at DESC").
		First(&profile).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return models.VoiceProfile{}, false, nil
	}
	if err != nil {
		return models.VoiceProfile{}, false, err
	}
	return profile, true, nil
}

func findVoiceProfileByVendorVoiceID(userID uint, vendorVoiceID string) (models.VoiceProfile, bool, error) {
	var profile models.VoiceProfile
	err := global.Db.
		Where("user_id = ? AND vendor_voice_id = ? AND status <> ?", userID, strings.TrimSpace(vendorVoiceID), voiceStatusDeleted).
		First(&profile).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return models.VoiceProfile{}, false, nil
	}
	if err != nil {
		return models.VoiceProfile{}, false, err
	}
	return profile, true, nil
}

func markVoiceProfileFailed(profileID uint, lastError string) error {
	return global.Db.Model(&models.VoiceProfile{}).
		Where("id = ?", profileID).
		Updates(map[string]any{
			"status":     voiceStatusFailed,
			"is_default": false,
			"last_error": trimVoiceError(lastError),
		}).Error
}

func findVoiceProfileByID(userID uint, rawID string) (models.VoiceProfile, error) {
	id, err := strconv.ParseUint(strings.TrimSpace(rawID), 10, 64)
	if err != nil || id == 0 {
		return models.VoiceProfile{}, errors.New("invalid voice profile id")
	}
	var profile models.VoiceProfile
	if err := global.Db.Where("id = ? AND user_id = ?", uint(id), userID).First(&profile).Error; err != nil {
		return models.VoiceProfile{}, err
	}
	return profile, nil
}

func parseCreateVoiceProfileRequest(ctx *gin.Context) (createVoiceProfileRequest, error) {
	req := createVoiceProfileRequest{
		DisplayName:  strings.TrimSpace(ctx.PostForm("display_name")),
		Relationship: strings.TrimSpace(ctx.PostForm("relationship")),
		SourceType:   strings.TrimSpace(ctx.PostForm("source_type")),
	}
	if req.DisplayName == "" {
		return req, errors.New("display_name is required")
	}
	if req.Relationship == "" {
		return req, errors.New("relationship is required")
	}
	switch req.SourceType {
	case voiceSourceUpload, voiceSourceRecord:
	default:
		return req, errors.New("source_type must be upload or record")
	}

	consentRaw := strings.TrimSpace(ctx.PostForm("consent_confirmed"))
	req.ConsentConfirmed = strings.EqualFold(consentRaw, "true") || consentRaw == "1"
	if durationRaw := strings.TrimSpace(ctx.PostForm("sample_duration_ms")); durationRaw != "" {
		if durationMs, err := strconv.ParseInt(durationRaw, 10, 64); err == nil && durationMs > 0 {
			req.SampleDurationMs = durationMs
		}
	}
	return req, nil
}

func readVoiceSample(fileHeader *multipart.FileHeader) (string, string, []byte, error) {
	if fileHeader.Size <= 0 {
		return "", "", nil, errors.New("audio is empty")
	}
	if fileHeader.Size > voiceSampleLimitBytes {
		return "", "", nil, errors.New("audio is too large")
	}
	ext := strings.ToLower(filepath.Ext(fileHeader.Filename))
	switch ext {
	case ".wav", ".mp3", ".m4a":
	default:
		return "", "", nil, errors.New("unsupported audio format")
	}

	f, err := fileHeader.Open()
	if err != nil {
		return "", "", nil, err
	}
	defer f.Close()

	data, err := io.ReadAll(io.LimitReader(f, voiceSampleLimitBytes+1))
	if err != nil {
		return "", "", nil, err
	}
	if len(data) == 0 {
		return "", "", nil, errors.New("audio is empty")
	}
	if len(data) > voiceSampleLimitBytes {
		return "", "", nil, errors.New("audio is too large")
	}

	contentType := fileHeader.Header.Get("Content-Type")
	if strings.TrimSpace(contentType) == "" {
		switch ext {
		case ".wav":
			contentType = "audio/wav"
		case ".mp3":
			contentType = "audio/mpeg"
		case ".m4a":
			contentType = "audio/mp4"
		}
	}
	return fileHeader.Filename, contentType, data, nil
}

func trimVoiceError(message string) string {
	cleaned := strings.TrimSpace(message)
	if len(cleaned) <= 512 {
		return cleaned
	}
	return cleaned[:512]
}

func isDashScopeVoiceMissingError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "not found") ||
		strings.Contains(msg, "voice not exists") ||
		strings.Contains(msg, "voice does not exist") ||
		strings.Contains(msg, "not_exist")
}
