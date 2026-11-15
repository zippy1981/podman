//go:build windows

// Package hyperv — tests that reproduce the Azure VM Run Command failure mode
// described in https://github.com/podman-container-tools/podman/issues/30266.
//
// # Problem
//
// When a user has no loaded profile, user.Current() on Windows returns an
// error (ERROR_INVALID_FUNCTION = 1) because GetUserProfileDirectory fails.
// Azure VM Run Commands use a Service (S4U) logon that does NOT load the
// user profile, triggering this failure.
//
// # How to reproduce manually
//
//  1. Build podman.exe: .\winmake.ps1 podman-remote
//  2. Run (from an elevated terminal):
//       .\test\windows\Test-HypervPrepNoProfile.ps1
//     The script creates a temporary local user, runs hyperv-prep via a
//     scheduled task (S4U logon = no profile load), and reports the result.
//
// # What fails today
//
// doPreparationForGroupMembership calls:
//   - hyperv.IsHyperVAdminsGroupMember() — silently returns false on token errors
//   - user.Current()                     — fails with "incorrect function."
//
// # What to fix
//
// See fix/hyperv-prep-hutil-error-handling: add getCurrentUserSIDFromToken()
// to read the SID from the process token without requiring a loaded profile.
package hyperv

import (
	"errors"
	"os/user"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestUserCurrentSucceedsOrDocumentsFailure documents the behaviour of
// user.Current() under the current execution context.  On machines where the
// profile is loaded this passes silently.  On machines without a loaded
// profile (e.g., a Service logon) it fails, reproducing the root cause of
// issue #30266.
func TestUserCurrentSucceedsOrDocumentsFailure(t *testing.T) {
	t.Parallel()
	u, err := user.Current()
	if err != nil {
		// This is the failure mode from issue #30266.
		t.Logf("REPRODUCE: user.Current() failed: %v", err)
		t.Logf("           This matches the Azure VM Run Command failure.")
		// Do not call t.Fatal — the test is intentionally documenting the
		// failure, not asserting it must succeed.
		return
	}
	t.Logf("user.Current() succeeded: Username=%q HomeDir=%q", u.Username, u.HomeDir)
}

// TestGetCurrentUserSIDSucceedsOrDocumentsFailure calls the internal
// getCurrentUserSID helper to exercise the user.Current()+LookupSID path.
// If that path fails (issue #30266 scenario), the test documents the error.
func TestGetCurrentUserSIDSucceedsOrDocumentsFailure(t *testing.T) {
	t.Parallel()
	sid, err := getCurrentUserSID()
	if err != nil {
		t.Logf("REPRODUCE: getCurrentUserSID() failed: %v", err)
		t.Logf("           Fix: add a token-based fallback (see fix/hyperv-prep-hutil-error-handling).")
		return
	}
	require.NotNil(t, sid)
	t.Logf("getCurrentUserSID() succeeded: SID=%v", sid)
}

// TestIsHyperVAdminsGroupMemberSwallowsErrors documents that
// IsHyperVAdminsGroupMember returns false (not an error) when the underlying
// token check fails.  This masks failures in restricted execution contexts.
func TestIsHyperVAdminsGroupMemberSwallowsErrors(t *testing.T) {
	t.Parallel()
	// We call both the token-based and the database-based variants.
	tokenResult := IsHyperVAdminsGroupMember()
	dbResult, dbErr := IsCurrentUserInHyperVAdminGroup()

	t.Logf("IsHyperVAdminsGroupMember (token):   %v", tokenResult)
	t.Logf("IsCurrentUserInHyperVAdminGroup (DB): inGroup=%v err=%v", dbResult, dbErr)

	// In a normal context both should agree.  In a service logon without a
	// profile the token call may silently return false while the DB call
	// returns an error (profile-dependent LookupSID failure).
	if dbErr == nil {
		assert.Equal(t, tokenResult, dbResult,
			"token-based and DB-based membership checks should agree in a normal context")
	}
}

// TestVerifyHyperVPermissionsNoPanic verifies the main permission check does
// not panic, returning either nil or one of the two typed sentinel errors.
func TestVerifyHyperVPermissionsNoPanic(t *testing.T) {
	t.Parallel()
	err := VerifyHyperVPermissions()
	if err == nil {
		return
	}
	assert.True(t,
		errors.Is(err, ErrHypervUserNotInAdminGroup) || errors.Is(err, ErrHypervUserSessionNotUpdated),
		"expected a known sentinel error, got: %v", err)
}

// TestGetHyperVAdminGroupNamePtrDoesNotPanic verifies that the SID->name
// lookup for the Hyper-V Administrators group does not panic on this machine.
func TestGetHyperVAdminGroupNamePtrDoesNotPanic(t *testing.T) {
	t.Parallel()
	ptr, err := getHyperVAdminGroupNamePtr()
	if err != nil {
		t.Logf("getHyperVAdminGroupNamePtr returned error (Hyper-V may not be installed): %v", err)
		return
	}
	assert.NotNil(t, ptr)
}

// TestIsUserInGroupNilSIDReturnsError verifies the nil-SID guard in
// isUserInGroup so that callers passing a nil SID (e.g., from a failed
// getCurrentUserSID) get a clear error instead of a nil-pointer panic.
func TestIsUserInGroupNilSIDReturnsError(t *testing.T) {
	t.Parallel()
	groupPtr, err := getHyperVAdminGroupNamePtr()
	if err != nil {
		t.Skipf("Hyper-V not available: %v", err)
	}
	inGroup, err := isUserInGroup(groupPtr, nil)
	assert.False(t, inGroup)
	require.Error(t, err)
}
