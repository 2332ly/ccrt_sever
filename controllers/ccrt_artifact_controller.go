package controllers

import (
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"ccrt_sever/utils"

	"github.com/gin-gonic/gin"
)

const ccrtArtifactPresignExpirySeconds = 900

func PresignCCRTArtifact(ctx *gin.Context) {
	var req CCRTArtifactPresignRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		utils.RespondError(ctx, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}

	user, err := getCurrentUser(ctx)
	if err != nil {
		utils.RespondError(ctx, http.StatusUnauthorized, "USER_NOT_FOUND", "User not found")
		return
	}

	kind := sanitizeCCRTArtifactSegment(req.Kind, "artifact")
	extension := strings.ToLower(filepath.Ext(strings.TrimSpace(req.FileName)))
	if extension == "" {
		extension = ".bin"
	}

	segments := []string{
		"ccrt",
		fmt.Sprintf("user-%d", user.ID),
		kind,
	}
	if req.AssessmentID != nil && *req.AssessmentID > 0 {
		segments = append(segments, fmt.Sprintf("assessment-%d", *req.AssessmentID))
	}
	if req.QuestionID != nil && *req.QuestionID > 0 {
		segments = append(segments, fmt.Sprintf("question-%d", *req.QuestionID))
	}
	segments = append(
		segments,
		fmt.Sprintf("%d%s", time.Now().UnixNano(), extension),
	)

	objectKey := strings.Join(segments, "/")
	uploadURL := fmt.Sprintf("https://stub-upload.local/%s", objectKey)

	utils.RespondOK(ctx, gin.H{
		"provider":     "stub",
		"method":       "PUT",
		"object_key":   objectKey,
		"upload_url":   uploadURL,
		"expires_in":   ccrtArtifactPresignExpirySeconds,
		"headers":      gin.H{"Content-Type": strings.TrimSpace(req.ContentType)},
		"content_type": strings.TrimSpace(req.ContentType),
	})
}

func sanitizeCCRTArtifactSegment(raw string, fallback string) string {
	raw = strings.TrimSpace(strings.ToLower(raw))
	if raw == "" {
		return fallback
	}
	var builder strings.Builder
	for _, r := range raw {
		switch {
		case r >= 'a' && r <= 'z':
			builder.WriteRune(r)
		case r >= '0' && r <= '9':
			builder.WriteRune(r)
		case r == '-' || r == '_':
			builder.WriteRune(r)
		default:
			builder.WriteRune('-')
		}
	}
	sanitized := strings.Trim(builder.String(), "-_")
	if sanitized == "" {
		return fallback
	}
	return sanitized
}
