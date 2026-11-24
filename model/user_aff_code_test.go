package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveInviterIdByAffCode(t *testing.T) {
	setupUserUpdateTestState(t)

	active := User{Username: "aff-active", Password: "password", Status: common.UserStatusEnabled, AffCode: "aff-active"}
	banned := User{Username: "aff-banned", Password: "password", Status: common.UserStatusDisabled, AffCode: "aff-banned"}
	require.NoError(t, DB.Create(&active).Error)
	require.NoError(t, DB.Create(&banned).Error)

	for _, tc := range []struct {
		name, code string
		want       int
		wantErr    error
	}{
		{"active inviter", "aff-active", active.Id, nil},
		{"trimmed code", "  aff-active ", active.Id, nil},
		{"empty code", "", 0, nil},
		{"unknown code", "aff-unknown", 0, nil},
		{"banned inviter", "aff-banned", 0, ErrAffiliateCodeUnavailable},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ResolveInviterIdByAffCode(tc.code)
			if tc.wantErr != nil {
				assert.ErrorIs(t, err, tc.wantErr)
			} else {
				assert.NoError(t, err)
			}
			assert.Equal(t, tc.want, got)
		})
	}
}
