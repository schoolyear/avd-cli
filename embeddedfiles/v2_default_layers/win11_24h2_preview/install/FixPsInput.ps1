$ErrorActionPreference = "Stop"
# This script fixes Powershell not receiving keystrokes. It does this by updating the
# responsible Powershell module. In Win11, the version is very low. (1.0.0)

Write-Host "=== Implementing PS input fix ==="
#NuGet is required to perform PSReadline update
Write-Host "=== Installing NuGet ==="
Install-PackageProvider -Name NuGet -Force -Scope AllUsers
Write-Host "=== Installing PSReadline update ==="
Install-Module PSReadLine -Force -Scope AllUsers
Write-Host "=== Implemented PS input fix ==="