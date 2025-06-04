# This script is executed every time a user logs into the VM which may be more than once
# Keep in mind that the student is waiting in the exam session for this script to finish
# You should not do any long running actions
#
# This script is executed as the user logging in, typically without admin rights

Param (
    [Parameter(Mandatory = $true)]
    [string]$uid,          # SID of the Windows user logging in

    [Parameter(Mandatory = $true)]
    [string]$gid,          # SID of the Windows user logging in

    [Parameter(Mandatory = $true)]
    [string]$username,     # Username of the Windows user logging in

    [Parameter(Mandatory = $true)]
    [string]$homedir       # Absolute path to the user's home directory
)
