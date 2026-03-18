package controllers

import (
	"net/http"
	"strings"

	"ccrt_sever/global"
	"ccrt_sever/models"
	"ccrt_sever/utils"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type emergencyContactRequest struct {
	Name  string `json:"name"`
	Phone string `json:"phone"`
}

// Me 示例受保护接口：返回当前 token 里的 username
func Me(ctx *gin.Context) {
	user, err := getCurrentUser(ctx)
	if err != nil {
		utils.RespondError(ctx, http.StatusUnauthorized, "USER_NOT_FOUND", "User not found")
		return
	}
	utils.RespondOK(ctx, gin.H{
		"username":                user.Username,
		"city":                    user.City,
		"address":                 user.Address,
		"emergency_contact_name":  user.EmergencyContactName,
		"emergency_contact_phone": user.EmergencyContactPhone,
		"status":                  http.StatusOK,
		"default_voice_id":        voiceProfileSummaryID(user.ID),
		"default_voice_name":      voiceProfileSummaryName(user.ID),
		"default_voice_status":    voiceProfileSummaryStatus(user.ID),
	})
}

// UpdateEmergencyContact updates the emergency contact for the current user.
func UpdateEmergencyContact(ctx *gin.Context) {
	var req emergencyContactRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		utils.RespondError(ctx, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}

	user, err := getCurrentUser(ctx)
	if err != nil {
		utils.RespondError(ctx, http.StatusUnauthorized, "USER_NOT_FOUND", "User not found")
		return
	}

	name := strings.TrimSpace(req.Name)
	phone := strings.TrimSpace(req.Phone)
	if phone == "" {
		utils.RespondError(ctx, http.StatusBadRequest, "INVALID_REQUEST", "phone is required")
		return
	}

	if err := global.Db.Transaction(func(tx *gorm.DB) error {
		contacts, err := ensureCareContactsForUserTx(tx, &user)
		if err != nil {
			return err
		}

		var primary *models.CareContact
		for i := range contacts {
			if contacts[i].IsPrimary {
				primary = &contacts[i]
				break
			}
		}

		if primary == nil {
			created := models.CareContact{
				UserID:       user.ID,
				Name:         name,
				Relationship: "other",
				Phone:        phone,
				IsPrimary:    true,
			}
			if created.Name == "" {
				created.Name = "紧急联系人"
			}
			if err := tx.Create(&created).Error; err != nil {
				return err
			}
		} else {
			primary.Name = name
			if primary.Name == "" {
				primary.Name = "紧急联系人"
			}
			primary.Phone = phone
			if err := tx.Save(primary).Error; err != nil {
				return err
			}
		}

		return syncLegacyEmergencyContactTx(tx, user.ID)
	}); err != nil {
		utils.RespondError(ctx, http.StatusInternalServerError, "DB_ERROR", "Could not update emergency contact")
		return
	}

	utils.RespondOK(ctx, gin.H{
		"emergency_contact_name":  name,
		"emergency_contact_phone": phone,
	})
}

func voiceProfileSummaryID(userID uint) uint {
	profile, found, err := findDefaultVoiceProfile(userID)
	if err != nil || !found {
		return 0
	}
	return profile.ID
}

func voiceProfileSummaryName(userID uint) string {
	profile, found, err := findDefaultVoiceProfile(userID)
	if err != nil || !found {
		return ""
	}
	return profile.DisplayName
}

func voiceProfileSummaryStatus(userID uint) string {
	profile, found, err := findDefaultVoiceProfile(userID)
	if err != nil || !found {
		return ""
	}
	return profile.Status
}
