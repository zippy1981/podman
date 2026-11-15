<#
.SYNOPSIS
    Reproduces the Azure VM Run Command failure of `podman system hyperv-prep`
    when the executing user has no loaded profile.

.DESCRIPTION
    Azure VM Run Commands with run_as_user set up a Windows logon session for
    the target user but do NOT load the user profile.  On such sessions
    user.Current() in Go calls GetUserProfileDirectory, which fails with
    ERROR_INVALID_FUNCTION (1) because the profile hive is not loaded.

    This script reproduces that scenario locally by:
      1. Creating a temporary local user account.
      2. Running `podman system hyperv-prep` via a scheduled task, which uses
         a Service logon (LOGON32_LOGON_SERVICE).  Service logons do not load
         the user profile, matching the Azure Run Command behaviour.
      3. Reporting the exit code and captured output.
      4. Cleaning up the temporary user and scheduled task.

    The script must be run from an elevated (Administrator) PowerShell prompt.

.PARAMETER PodmanExe
    Path to the podman.exe to test.  Defaults to the first podman.exe found
    on PATH.

.PARAMETER SkipCleanup
    When set, the temporary user and scheduled task are not removed after the
    test, which is useful for manual investigation.

.EXAMPLE
    .\Test-HypervPrepNoProfile.ps1
    .\Test-HypervPrepNoProfile.ps1 -PodmanExe C:\path\to\podman.exe -SkipCleanup
#>
[CmdletBinding()]
param(
    [string]$PodmanExe = (Get-Command podman.exe -ErrorAction SilentlyContinue).Source,
    [switch]$SkipCleanup
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------
function Write-Step([string]$msg) { Write-Host "==> $msg" -ForegroundColor Cyan }
function Write-Pass([string]$msg) { Write-Host "PASS: $msg" -ForegroundColor Green }
function Write-Fail([string]$msg) { Write-Host "FAIL: $msg" -ForegroundColor Red }

# ---------------------------------------------------------------------------
# Validate prerequisites
# ---------------------------------------------------------------------------
if (-not $PodmanExe -or -not (Test-Path $PodmanExe)) {
    throw "podman.exe not found.  Build it first with .\winmake.ps1 podman-remote or pass -PodmanExe."
}

$currentPrincipal = [Security.Principal.WindowsPrincipal][Security.Principal.WindowsIdentity]::GetCurrent()
if (-not $currentPrincipal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) {
    throw "This script must be run from an elevated (Administrator) terminal."
}

# ---------------------------------------------------------------------------
# Setup
# ---------------------------------------------------------------------------
$tmpUser     = "podman-test-$([System.IO.Path]::GetRandomFileName().Replace('.','').Substring(0,8))"
$tmpPassword = [System.Web.Security.Membership]::GeneratePassword(16, 2)
$taskName    = "PodmanHypervPrepTest-$tmpUser"
$logFile     = Join-Path $env:TEMP "$taskName.log"

Write-Step "Creating temporary local user '$tmpUser' (no profile load)"
$secPwd = ConvertTo-SecureString $tmpPassword -AsPlainText -Force
New-LocalUser -Name $tmpUser -Password $secPwd -PasswordNeverExpires -UserMayNotChangePassword | Out-Null
# Add to Administrators so the command can pass the HasAdminRights() check.
Add-LocalGroupMember -Group Administrators -Member $tmpUser

Write-Step "Registering scheduled task to run hyperv-prep as '$tmpUser' (service logon = no profile)"
$action  = New-ScheduledTaskAction -Execute $PodmanExe -Argument "system hyperv-prep"
# RunLevel Highest + LogonType S4U = service-style logon, profile NOT loaded.
$principal = New-ScheduledTaskPrincipal -UserId $tmpUser -LogonType S4U -RunLevel Highest
$settings  = New-ScheduledTaskSettingsSet -ExecutionTimeLimit (New-TimeSpan -Minutes 2)
Register-ScheduledTask -TaskName $taskName -Action $action -Principal $principal -Settings $settings | Out-Null

try {
    Write-Step "Starting scheduled task '$taskName'"
    Start-ScheduledTask -TaskName $taskName

    # Wait for the task to finish (up to 60 seconds).
    $deadline = (Get-Date).AddSeconds(60)
    do {
        Start-Sleep -Milliseconds 500
        $info = Get-ScheduledTaskInfo -TaskName $taskName
    } until ($info.LastTaskResult -ne 267009 -or (Get-Date) -gt $deadline)
    # 267009 = SCHED_S_TASK_RUNNING

    $exitCode = $info.LastTaskResult
    Write-Step "Task finished with exit code $exitCode"

    if ($exitCode -eq 0) {
        Write-Pass "podman system hyperv-prep succeeded under a no-profile service logon."
    } else {
        Write-Fail "podman system hyperv-prep exited with code $exitCode under a no-profile service logon."
        Write-Host "  This reproduces the Azure VM Run Command failure described in issue #30266."
    }
} finally {
    if (-not $SkipCleanup) {
        Write-Step "Cleaning up task '$taskName' and user '$tmpUser'"
        Unregister-ScheduledTask -TaskName $taskName -Confirm:$false -ErrorAction SilentlyContinue
        Remove-LocalUser -Name $tmpUser -ErrorAction SilentlyContinue
    } else {
        Write-Host "Skipping cleanup.  Task: $taskName  User: $tmpUser" -ForegroundColor Yellow
    }
}
