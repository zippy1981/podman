//go:build windows

package hyperv

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestIsHyperVAdminsGroupMemberDoesNotPanic verifies the function returns
// without panicking in any execution context.
func TestIsHyperVAdminsGroupMemberDoesNotPanic(t *testing.T) {
	t.Parallel()
	// We can't assert the return value because it depends on the test runner's
	// group membership, but the function must not panic.
	_ = IsHyperVAdminsGroupMember()
}

// TestIsCurrentUserInHyperVAdminGroupConsistency verifies that
// IsCurrentUserInHyperVAdminGroup either returns an error or returns a value
// consistent with what the calling context would expect.
func TestIsCurrentUserInHyperVAdminGroupConsistency(t *testing.T) {
	t.Parallel()
	inGroup, err := IsCurrentUserInHyperVAdminGroup()
	if err != nil {
		// Error is acceptable; the function should not panic.
		t.Logf("IsCurrentUserInHyperVAdminGroup returned error: %v", err)
		return
	}
	t.Logf("IsCurrentUserInHyperVAdminGroup returned inGroup=%v", inGroup)
}

// TestGetCurrentUserSIDIsNonNil verifies that getCurrentUserSID succeeds
// and returns a non-nil SID in a normal execution context.
func TestGetCurrentUserSIDIsNonNil(t *testing.T) {
	t.Parallel()
	sid, err := getCurrentUserSID()
	require.NoError(t, err, "getCurrentUserSID should not fail in a normal test environment")
	assert.NotNil(t, sid, "returned SID should be non-nil")
}

// TestVerifyHyperVPermissionsReturnsKnownErrors verifies that
// VerifyHyperVPermissions either succeeds or returns one of the known typed errors.
func TestVerifyHyperVPermissionsReturnsKnownErrors(t *testing.T) {
	t.Parallel()
	err := VerifyHyperVPermissions()
	if err == nil {
		t.Log("VerifyHyperVPermissions: caller has Hyper-V permissions")
		return
	}

	knownErrors := []error{
		ErrHypervUserNotInAdminGroup,
		ErrHypervUserSessionNotUpdated,
	}
	for _, known := range knownErrors {
		if err == known {
			t.Logf("VerifyHyperVPermissions returned expected error: %v", err)
			return
		}
	}
	t.Errorf("VerifyHyperVPermissions returned unexpected error: %v", err)
}
