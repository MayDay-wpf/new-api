package model

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"

	"gorm.io/gorm"
)

// UserQuotaTransfer is the committed snapshot of a balance transfer between two
// users. It carries the display names so callers and logs do not need another
// lookup after the transaction.
type UserQuotaTransfer struct {
	FromUserId   int
	FromUsername string
	ToUserId     int
	ToUsername   string
	Quota        int
}

// TransferUserQuota moves quota from one user's wallet to another user's
// wallet. The two balance updates happen in a single transaction, with both
// user rows locked in ascending ID order so concurrent transfers cannot
// deadlock or overdraw a balance.
func TransferUserQuota(fromUserId, toUserId, quota int, callerIp string) (*UserQuotaTransfer, error) {
	if fromUserId <= 0 || toUserId <= 0 || quota <= 0 {
		return nil, common.NewMessage("Invalid parameters")
	}
	if fromUserId == toUserId {
		return nil, common.NewMessage("You cannot transfer balance to yourself")
	}
	minimumQuota := int(common.QuotaPerUnit)
	if quota < minimumQuota {
		return nil, common.NewMessage("Transfer amount must be at least {{amount}}", map[string]any{
			"amount": logger.FormatQuota(minimumQuota),
		})
	}
	if err := common.ValidateWalletQuota(quota); err != nil {
		return nil, ErrWalletQuotaLimitExceeded
	}

	var transfer UserQuotaTransfer
	err := DB.Transaction(func(tx *gorm.DB) error {
		lockedIds := []int{fromUserId, toUserId}
		if lockedIds[0] > lockedIds[1] {
			lockedIds[0], lockedIds[1] = lockedIds[1], lockedIds[0]
		}
		var users []User
		if err := lockForUpdate(tx).Where("id IN ?", lockedIds).Order("id ASC").Find(&users).Error; err != nil {
			return err
		}

		var fromUser, toUser *User
		for i := range users {
			switch users[i].Id {
			case fromUserId:
				fromUser = &users[i]
			case toUserId:
				toUser = &users[i]
			}
		}
		if fromUser == nil {
			return gorm.ErrRecordNotFound
		}
		if toUser == nil {
			return common.NewMessage("Target user does not exist")
		}
		if toUser.Status != common.UserStatusEnabled {
			return common.NewMessage("The recipient account is disabled")
		}
		if fromUser.Quota < quota {
			return common.NewMessage("Insufficient balance")
		}
		if toUser.Quota > common.MaxWalletQuota-quota {
			return ErrWalletQuotaLimitExceeded
		}

		result := tx.Model(&User{}).
			Where("id = ? AND quota >= ?", fromUserId, quota).
			Update("quota", gorm.Expr("quota - ?", quota))
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return common.NewMessage("Insufficient balance")
		}
		result = tx.Model(&User{}).
			Where("id = ?", toUserId).
			Update("quota", gorm.Expr("quota + ?", quota))
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return gorm.ErrRecordNotFound
		}

		transfer = UserQuotaTransfer{
			FromUserId:   fromUser.Id,
			FromUsername: fromUser.Username,
			ToUserId:     toUser.Id,
			ToUsername:   toUser.Username,
			Quota:        quota,
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	if err := cacheDecrUserQuota(fromUserId, int64(quota)); err != nil {
		common.SysLog(common.LogText("failed to decrease user quota cache after transfer: %s", err.Error()))
	}
	if err := cacheIncrUserQuota(toUserId, int64(quota)); err != nil {
		common.SysLog(common.LogText("failed to increase user quota cache after transfer: %s", err.Error()))
	}

	formattedQuota := logger.FormatQuota(quota)
	RecordTransferLog(fromUserId, -quota, common.NewMessage("Transferred {{quota}} to {{username}} (ID: {{id}})", map[string]any{
		"quota":    formattedQuota,
		"username": transfer.ToUsername,
		"id":       transfer.ToUserId,
	}), callerIp)
	RecordTransferLog(toUserId, quota, common.NewMessage("Received {{quota}} from {{username}} (ID: {{id}})", map[string]any{
		"quota":    formattedQuota,
		"username": transfer.FromUsername,
		"id":       transfer.FromUserId,
	}), callerIp)
	return &transfer, nil
}
