//go:build windows && remote

package system

// Diagnostics for https://github.com/podman-container-tools/podman/issues/30266.
// Not for merging: prints, to stdout, every input doPreparationForGroupMembership
// depends on, so a failing run_as_user context shows which step breaks.

import (
	"fmt"
	"os"
	"os/user"

	"go.podman.io/podman/v6/pkg/machine/hyperv"
	"go.podman.io/podman/v6/pkg/machine/windows"
	syswindows "golang.org/x/sys/windows"
)

func diag(format string, args ...any) {
	fmt.Fprintf(os.Stdout, "DIAG "+format+"\n", args...)
}

func printHypervPrepDiagnostics() {
	diag("env USERNAME=%q USERDOMAIN=%q USERPROFILE=%q", os.Getenv("USERNAME"), os.Getenv("USERDOMAIN"), os.Getenv("USERPROFILE"))
	diag("HasAdminRights=%v", windows.HasAdminRights())

	token, err := syswindows.OpenCurrentProcessToken()
	if err != nil {
		diag("OpenCurrentProcessToken err=%v", err)
	} else {
		defer token.Close()
		if tu, err := token.GetTokenUser(); err != nil {
			diag("GetTokenUser err=%v", err)
		} else {
			account, domain, _, lerr := tu.User.Sid.LookupAccount("")
			diag("token sid=%s account=%s\\%s lookupErr=%v", tu.User.Sid.String(), domain, account, lerr)
		}
		diag("token IsElevated=%v", token.IsElevated())
		dir, err := token.GetUserProfileDirectory()
		diag("GetUserProfileDirectory dir=%q err=%v (errno=%d)", dir, err, errnoOf(err))
		if hvSid, err := syswindows.CreateWellKnownSid(syswindows.WinBuiltinHyperVAdminsSid); err == nil {
			member, err := syswindows.Token(0).IsMember(hvSid)
			diag("CheckTokenMembership(HyperVAdmins) member=%v err=%v", member, err)
		}
	}

	u, err := user.Current()
	if err != nil {
		diag("user.Current err=%v (errno=%d)", err, errnoOf(err))
	} else {
		diag("user.Current username=%q homedir=%q", u.Username, u.HomeDir)
	}

	diag("IsHyperVAdminsGroupMember=%v", hyperv.IsHyperVAdminsGroupMember())
	inGroup, err := hyperv.IsCurrentUserInHyperVAdminGroup()
	diag("IsCurrentUserInHyperVAdminGroup inGroup=%v err=%v", inGroup, err)
}

func errnoOf(err error) int {
	if errno, ok := err.(syswindows.Errno); ok {
		return int(errno)
	}
	return -1
}
