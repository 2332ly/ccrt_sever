package models

import "time"

// RefreshToken stores hashed refresh tokens for session rotation.
type RefreshToken struct {
	ID        uint       `gorm:"primaryKey"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
	RevokedAt *time.Time `json:"revoked_at" gorm:"index"`

	UserID    uint      `json:"user_id" gorm:"index"`
	TokenHash string    `json:"-" gorm:"size:64;uniqueIndex"`
	ExpiresAt time.Time `json:"expires_at" gorm:"index"`
}
