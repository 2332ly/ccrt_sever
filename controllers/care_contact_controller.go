package controllers

import (
	"net/http"
	"strconv"
	"strings"

	"ccrt_sever/global"
	"ccrt_sever/models"
	"ccrt_sever/utils"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type careContactRequest struct {
	Name         string `json:"name" binding:"required"`
	Relationship string `json:"relationship"`
	Phone        string `json:"phone" binding:"required"`
	Note         string `json:"note"`
}

var allowedCareRelationships = map[string]struct{}{
	"spouse":    {},
	"child":     {},
	"sibling":   {},
	"caregiver": {},
	"doctor":    {},
	"other":     {},
}

func ListCareContacts(ctx *gin.Context) {
	user, err := getCurrentUser(ctx)
	if err != nil {
		utils.RespondError(ctx, http.StatusUnauthorized, "USER_NOT_FOUND", "User not found")
		return
	}

	contacts, err := ensureCareContactsForUser(user)
	if err != nil {
		utils.RespondError(ctx, http.StatusInternalServerError, "DB_ERROR", "Could not load care contacts")
		return
	}

	utils.RespondOK(ctx, gin.H{
		"contacts": contactsToResponse(contacts),
	})
}

func CreateCareContact(ctx *gin.Context) {
	var req careContactRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		utils.RespondError(ctx, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}

	user, err := getCurrentUser(ctx)
	if err != nil {
		utils.RespondError(ctx, http.StatusUnauthorized, "USER_NOT_FOUND", "User not found")
		return
	}

	var created models.CareContact
	if err := global.Db.Transaction(func(tx *gorm.DB) error {
		contacts, err := ensureCareContactsForUserTx(tx, &user)
		if err != nil {
			return err
		}

		created = models.CareContact{
			UserID:       user.ID,
			Name:         strings.TrimSpace(req.Name),
			Relationship: normalizeCareRelationship(req.Relationship),
			Phone:        strings.TrimSpace(req.Phone),
			Note:         strings.TrimSpace(req.Note),
			IsPrimary:    len(contacts) == 0,
		}
		if err := tx.Create(&created).Error; err != nil {
			return err
		}
		return syncLegacyEmergencyContactTx(tx, user.ID)
	}); err != nil {
		utils.RespondError(ctx, http.StatusInternalServerError, "DB_ERROR", "Could not create care contact")
		return
	}

	utils.RespondOK(ctx, gin.H{
		"contact": careContactToResponse(created),
	})
}

func UpdateCareContact(ctx *gin.Context) {
	var req careContactRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		utils.RespondError(ctx, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}

	user, err := getCurrentUser(ctx)
	if err != nil {
		utils.RespondError(ctx, http.StatusUnauthorized, "USER_NOT_FOUND", "User not found")
		return
	}

	contactID, err := parseCareContactID(ctx.Param("id"))
	if err != nil {
		utils.RespondError(ctx, http.StatusBadRequest, "INVALID_CONTACT_ID", "Invalid care contact id")
		return
	}

	var updated models.CareContact
	if err := global.Db.Transaction(func(tx *gorm.DB) error {
		if _, err := ensureCareContactsForUserTx(tx, &user); err != nil {
			return err
		}
		if err := tx.Where("id = ? AND user_id = ?", contactID, user.ID).First(&updated).Error; err != nil {
			return err
		}

		updated.Name = strings.TrimSpace(req.Name)
		updated.Relationship = normalizeCareRelationship(req.Relationship)
		updated.Phone = strings.TrimSpace(req.Phone)
		updated.Note = strings.TrimSpace(req.Note)
		if err := tx.Save(&updated).Error; err != nil {
			return err
		}
		return syncLegacyEmergencyContactTx(tx, user.ID)
	}); err != nil {
		status := http.StatusInternalServerError
		code := "DB_ERROR"
		message := "Could not update care contact"
		if err == gorm.ErrRecordNotFound {
			status = http.StatusNotFound
			code = "CARE_CONTACT_NOT_FOUND"
			message = "Care contact not found"
		}
		utils.RespondError(ctx, status, code, message)
		return
	}

	utils.RespondOK(ctx, gin.H{
		"contact": careContactToResponse(updated),
	})
}

func DeleteCareContact(ctx *gin.Context) {
	user, err := getCurrentUser(ctx)
	if err != nil {
		utils.RespondError(ctx, http.StatusUnauthorized, "USER_NOT_FOUND", "User not found")
		return
	}

	contactID, err := parseCareContactID(ctx.Param("id"))
	if err != nil {
		utils.RespondError(ctx, http.StatusBadRequest, "INVALID_CONTACT_ID", "Invalid care contact id")
		return
	}

	if err := global.Db.Transaction(func(tx *gorm.DB) error {
		if _, err := ensureCareContactsForUserTx(tx, &user); err != nil {
			return err
		}

		var target models.CareContact
		if err := tx.Where("id = ? AND user_id = ?", contactID, user.ID).First(&target).Error; err != nil {
			return err
		}
		if err := tx.Delete(&target).Error; err != nil {
			return err
		}

		if target.IsPrimary {
			var replacement models.CareContact
			err := tx.Where("user_id = ?", user.ID).
				Order("created_at ASC").
				First(&replacement).Error
			if err == nil {
				if err := tx.Model(&models.CareContact{}).
					Where("user_id = ?", user.ID).
					Update("is_primary", false).Error; err != nil {
					return err
				}
				if err := tx.Model(&replacement).Update("is_primary", true).Error; err != nil {
					return err
				}
			} else if err != nil && err != gorm.ErrRecordNotFound {
				return err
			}
		}

		return syncLegacyEmergencyContactTx(tx, user.ID)
	}); err != nil {
		status := http.StatusInternalServerError
		code := "DB_ERROR"
		message := "Could not delete care contact"
		if err == gorm.ErrRecordNotFound {
			status = http.StatusNotFound
			code = "CARE_CONTACT_NOT_FOUND"
			message = "Care contact not found"
		}
		utils.RespondError(ctx, status, code, message)
		return
	}

	utils.RespondOK(ctx, gin.H{
		"deleted": true,
	})
}

func SetPrimaryCareContact(ctx *gin.Context) {
	user, err := getCurrentUser(ctx)
	if err != nil {
		utils.RespondError(ctx, http.StatusUnauthorized, "USER_NOT_FOUND", "User not found")
		return
	}

	contactID, err := parseCareContactID(ctx.Param("id"))
	if err != nil {
		utils.RespondError(ctx, http.StatusBadRequest, "INVALID_CONTACT_ID", "Invalid care contact id")
		return
	}

	var updated models.CareContact
	if err := global.Db.Transaction(func(tx *gorm.DB) error {
		if _, err := ensureCareContactsForUserTx(tx, &user); err != nil {
			return err
		}
		if err := tx.Where("user_id = ?", user.ID).Model(&models.CareContact{}).Update("is_primary", false).Error; err != nil {
			return err
		}
		if err := tx.Where("id = ? AND user_id = ?", contactID, user.ID).First(&updated).Error; err != nil {
			return err
		}
		updated.IsPrimary = true
		if err := tx.Save(&updated).Error; err != nil {
			return err
		}
		return syncLegacyEmergencyContactTx(tx, user.ID)
	}); err != nil {
		status := http.StatusInternalServerError
		code := "DB_ERROR"
		message := "Could not set primary care contact"
		if err == gorm.ErrRecordNotFound {
			status = http.StatusNotFound
			code = "CARE_CONTACT_NOT_FOUND"
			message = "Care contact not found"
		}
		utils.RespondError(ctx, status, code, message)
		return
	}

	utils.RespondOK(ctx, gin.H{
		"contact": careContactToResponse(updated),
	})
}

func ensureCareContactsForUser(user models.User) ([]models.CareContact, error) {
	return ensureCareContactsForUserTx(global.Db, &user)
}

func ensureCareContactsForUserTx(tx *gorm.DB, user *models.User) ([]models.CareContact, error) {
	var contacts []models.CareContact
	if err := tx.Where("user_id = ?", user.ID).
		Order("is_primary DESC, created_at ASC").
		Find(&contacts).Error; err != nil {
		return nil, err
	}
	if len(contacts) == 0 && strings.TrimSpace(user.EmergencyContactPhone) != "" {
		seed := models.CareContact{
			UserID:       user.ID,
			Name:         strings.TrimSpace(user.EmergencyContactName),
			Relationship: "other",
			Phone:        strings.TrimSpace(user.EmergencyContactPhone),
			IsPrimary:    true,
		}
		if seed.Name == "" {
			seed.Name = "紧急联系人"
		}
		if err := tx.Create(&seed).Error; err != nil {
			return nil, err
		}
		contacts = []models.CareContact{seed}
	}

	if len(contacts) > 0 {
		hasPrimary := false
		for _, contact := range contacts {
			if contact.IsPrimary {
				hasPrimary = true
				break
			}
		}
		if !hasPrimary {
			first := contacts[0]
			if err := tx.Model(&first).Update("is_primary", true).Error; err != nil {
				return nil, err
			}
			contacts[0].IsPrimary = true
		}
	}

	if err := syncLegacyEmergencyContactTx(tx, user.ID); err != nil {
		return nil, err
	}

	if err := tx.Where("user_id = ?", user.ID).
		Order("is_primary DESC, created_at ASC").
		Find(&contacts).Error; err != nil {
		return nil, err
	}
	return contacts, nil
}

func syncLegacyEmergencyContactTx(tx *gorm.DB, userID uint) error {
	var primary models.CareContact
	err := tx.Where("user_id = ? AND is_primary = ?", userID, true).
		Order("created_at ASC").
		First(&primary).Error
	if err != nil && err != gorm.ErrRecordNotFound {
		return err
	}

	updates := map[string]any{
		"emergency_contact_name":  "",
		"emergency_contact_phone": "",
	}
	if err == nil {
		updates["emergency_contact_name"] = primary.Name
		updates["emergency_contact_phone"] = primary.Phone
	}
	return tx.Model(&models.User{}).Where("id = ?", userID).Updates(updates).Error
}

func contactsToResponse(contacts []models.CareContact) []gin.H {
	items := make([]gin.H, 0, len(contacts))
	for _, contact := range contacts {
		items = append(items, careContactToResponse(contact))
	}
	return items
}

func careContactToResponse(contact models.CareContact) gin.H {
	return gin.H{
		"id":           contact.ID,
		"name":         contact.Name,
		"relationship": contact.Relationship,
		"phone":        contact.Phone,
		"note":         contact.Note,
		"is_primary":   contact.IsPrimary,
		"created_at":   contact.CreatedAt,
		"updated_at":   contact.UpdatedAt,
	}
}

func normalizeCareRelationship(raw string) string {
	value := strings.ToLower(strings.TrimSpace(raw))
	if _, ok := allowedCareRelationships[value]; ok {
		return value
	}
	return "other"
}

func parseCareContactID(raw string) (uint, error) {
	id, err := strconv.ParseUint(raw, 10, 64)
	return uint(id), err
}
