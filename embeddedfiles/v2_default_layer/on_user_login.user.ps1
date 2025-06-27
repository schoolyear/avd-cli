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
    [string]$homedir,       # Absolute path to the user's home directory

    # To make sure this script doesn't break when new parameters are added
    [Parameter(ValueFromRemainingArguments)]
    [string[]]$RemainingArgs
)

$ErrorActionPreference = "Stop"
Set-StrictMode -Version Latest

$proxyDomain = "proxies.local"

$proxyBytes = [System.Text.Encoding]::UTF8.GetBytes("PROXY $($proxyDomain):8080")
$proxyStringBase64 = [Convert]::ToBase64String($proxyBytes)
$matchingProxyBase64 = $proxyStringBase64.Replace('+','-').Replace('/','_').TrimEnd('=')
$pacUrl = "http://${proxyDomain}:8080/proxy.pac?matchingProxyBase64=$matchingProxyBase64&defaultProxy=DIRECT"

try
{
    Write-Host "Setting user-level proxy..."
    $regPath = "HKCU:\Software\Microsoft\Windows\CurrentVersion\Internet Settings"

    Set-ItemProperty -Path $regPath -Name AutoConfigURL -Value "$pacUrl"
    Set-ItemProperty -Path $regPath -Name ProxyEnable -Value 1
    netsh winhttp import proxy source=ie
}
catch
{
    Write-Error "Could not set user-level proxy: $_"
    exit 1
}