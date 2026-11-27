package model

import (
	"errors"

	"gorm.io/gorm"
)

// SnowAppAuthCode 是 Snow App 桌面端的一次性授权码，只保存哈希值。
type SnowAppAuthCode struct {
	Id            int    `json:"id"`
	CodeHash      string `json:"-" gorm:"type:varchar(64);uniqueIndex"`
	UserId        int    `json:"user_id" gorm:"index"`
	RedirectUri   string `json:"redirect_uri" gorm:"type:varchar(512)"`
	CodeChallenge string `json:"code_challenge" gorm:"type:varchar(128)"`
	ExpiresAt     int64  `json:"expires_at" gorm:"bigint;index"`
	UsedAt        int64  `json:"used_at" gorm:"bigint;default:0"`
	CreatedAt     int64  `json:"created_at" gorm:"bigint"`
}

func (SnowAppAuthCode) TableName() string {
	return "snow_app_auth_codes"
}

var ErrSnowAppAuthCodeInvalid = errors.New("the authorization code is invalid or expired")

func CreateSnowAppAuthCode(code *SnowAppAuthCode) error {
	if code == nil || code.CodeHash == "" || code.UserId <= 0 {
		return ErrSnowAppAuthCodeInvalid
	}
	return DB.Create(code).Error
}

// ConsumeSnowAppAuthCode 原子地标记授权码已使用，保证只能换取一次令牌。
func ConsumeSnowAppAuthCode(codeHash string, now int64) (*SnowAppAuthCode, error) {
	if codeHash == "" {
		return nil, ErrSnowAppAuthCodeInvalid
	}
	var record SnowAppAuthCode
	err := DB.Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&SnowAppAuthCode{}).
			Where("code_hash = ? AND used_at = 0 AND expires_at > ?", codeHash, now).
			Update("used_at", now)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return ErrSnowAppAuthCodeInvalid
		}
		return tx.Where("code_hash = ?", codeHash).First(&record).Error
	})
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrSnowAppAuthCodeInvalid
		}
		return nil, err
	}
	return &record, nil
}

func DeleteExpiredSnowAppAuthCodes(before int64) error {
	return DB.Where("expires_at < ?", before).Delete(&SnowAppAuthCode{}).Error
}
