package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func createTransferTestUser(t *testing.T, username string, quota int) *User {
	t.Helper()
	user := &User{
		Username: username,
		Password: "password",
		Email:    username + "@example.com",
		Status:   common.UserStatusEnabled,
		Role:     common.RoleCommonUser,
		Quota:    quota,
		AffCode:  username,
	}
	require.NoError(t, DB.Create(user).Error)
	return user
}

func TestTransferUserQuotaMovesBalanceAndRecordsLogs(t *testing.T) {
	truncateTables(t)
	minimumQuota := int(common.QuotaPerUnit)
	sender := createTransferTestUser(t, "transfer-sender", minimumQuota*3)
	receiver := createTransferTestUser(t, "transfer-receiver", minimumQuota)

	transfer, err := TransferUserQuota(sender.Id, receiver.Id, minimumQuota, "127.0.0.1")
	require.NoError(t, err)
	require.NotNil(t, transfer)
	assert.Equal(t, sender.Id, transfer.FromUserId)
	assert.Equal(t, receiver.Id, transfer.ToUserId)
	assert.Equal(t, sender.Username, transfer.FromUsername)
	assert.Equal(t, receiver.Username, transfer.ToUsername)
	assert.Equal(t, minimumQuota, transfer.Quota)

	var senderAfter, receiverAfter User
	require.NoError(t, DB.First(&senderAfter, sender.Id).Error)
	require.NoError(t, DB.First(&receiverAfter, receiver.Id).Error)
	assert.Equal(t, minimumQuota*2, senderAfter.Quota)
	assert.Equal(t, minimumQuota*2, receiverAfter.Quota)

	var logs []Log
	require.NoError(t, LOG_DB.
		Where("type = ? AND user_id IN ?", LogTypeTransfer, []int{sender.Id, receiver.Id}).
		Order("id ASC").
		Find(&logs).Error)
	require.Len(t, logs, 2)
	assert.Equal(t, sender.Id, logs[0].UserId)
	assert.Equal(t, -minimumQuota, logs[0].Quota)
	assert.Equal(t, "127.0.0.1", logs[0].Ip)
	assert.Contains(t, logs[0].Content, "Transferred")
	assert.Equal(t, receiver.Id, logs[1].UserId)
	assert.Equal(t, minimumQuota, logs[1].Quota)
	assert.Equal(t, "127.0.0.1", logs[1].Ip)
	assert.Contains(t, logs[1].Content, "Received")
}

func TestTransferUserQuotaRejectsInvalidTransfers(t *testing.T) {
	truncateTables(t)
	minimumQuota := int(common.QuotaPerUnit)
	sender := createTransferTestUser(t, "transfer-sender-invalid", minimumQuota)
	receiver := createTransferTestUser(t, "transfer-receiver-invalid", minimumQuota)

	_, err := TransferUserQuota(sender.Id, sender.Id, minimumQuota, "127.0.0.1")
	require.Error(t, err)
	assert.Equal(t, "You cannot transfer balance to yourself", err.Error())

	_, err = TransferUserQuota(sender.Id, receiver.Id, minimumQuota-1, "127.0.0.1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "at least")

	_, err = TransferUserQuota(sender.Id, receiver.Id, minimumQuota*2, "127.0.0.1")
	require.Error(t, err)
	assert.Equal(t, "Insufficient balance", err.Error())

	_, err = TransferUserQuota(sender.Id, 99999999, minimumQuota, "127.0.0.1")
	require.Error(t, err)
	assert.Equal(t, "Target user does not exist", err.Error())

	disabled := createTransferTestUser(t, "transfer-receiver-disabled", minimumQuota)
	require.NoError(t, DB.Model(&User{}).Where("id = ?", disabled.Id).Update("status", common.UserStatusDisabled).Error)
	_, err = TransferUserQuota(sender.Id, disabled.Id, minimumQuota, "127.0.0.1")
	require.Error(t, err)
	assert.Equal(t, "The recipient account is disabled", err.Error())

	var senderAfter User
	require.NoError(t, DB.First(&senderAfter, sender.Id).Error)
	assert.Equal(t, minimumQuota, senderAfter.Quota)

	var logCount int64
	require.NoError(t, LOG_DB.Model(&Log{}).
		Where("type = ? AND user_id IN ?", LogTypeTransfer, []int{sender.Id, receiver.Id, disabled.Id}).
		Count(&logCount).Error)
	assert.Zero(t, logCount)
}
