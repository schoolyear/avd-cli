param (
    [Parameter(Mandatory=$true)]
    [ValidateSet("Development", "Testing", "Beta", "Production")]
    [string]$environment
)

# Recommended snippet to make sure PowerShell stops execution on failure
# https://learn.microsoft.com/en-us/powershell/module/microsoft.powershell.core/about/about_preference_variables?view=powershell-7.5#erroractionpreference
# https://learn.microsoft.com/en-us/powershell/module/microsoft.powershell.core/set-strictmode?view=powershell-7.4
$ErrorActionPreference = "Stop"
Set-StrictMode -Version Latest

# Recommended snippet to make sure PowerShell doesn't show a progress bar when downloading files
# This makes the downloads considerably faster
$ProgressPreference = 'SilentlyContinue'

## All these script calls are wrapper in Push/Pop location to make sure any working-dir change during the scripts
# do not affect the working-dir of the other scripts

# Clean up Windows
# executed in a separate script block, because these rds_templates scripts are sensitive to the "$ErrorActionPreference" and/or Set-StrictMode -Version Latest
& {
    $ErrorActionPreference = "Continue"
    Set-StrictMode -Off

    Push-Location; Write-Host "=== Executing DisableAutoUpdates.ps1 ==="
    & .\install\rds_templates\DisableAutoUpdates.ps1 # first, so Windows doesn't start pulling updates during the image build
    Pop-Location

    Push-Location; Write-Host "=== Executing DisableStorageSense.ps1 ==="
    & .\install\rds_templates\DisableStorageSense.ps1
    Pop-Location

    Push-Location; Write-Host "=== Executing RemoveAppxPackages.ps1 ==="
    & .\install\rds_templates\RemoveAppxPackages.ps1 -AppxPackages "Microsoft.XboxApp","Microsoft.ZuneVideo","Microsoft.ZuneMusic","Microsoft.YourPhone","Microsoft.XboxSpeechToTextOverlay","Microsoft.XboxIdentityProvider","Microsoft.XboxGamingOverlay","Microsoft.XboxGameOverlay","Microsoft.Xbox.TCUI","Microsoft.WindowsSoundRecorder","Microsoft.WindowsMaps","Microsoft.WindowsFeedbackHub","Microsoft.WindowsCamera","Microsoft.WindowsAlarms","Microsoft.Todos","Microsoft.SkypeApp","Microsoft.PowerAutomateDesktop","Microsoft.People","Microsoft.MicrosoftStickyNotes","Microsoft.MicrosoftSolitaireCollection","Microsoft.Office.OneNote","Microsoft.MicrosoftOfficeHub","Microsoft.Getstarted","Microsoft.GetHelp","Microsoft.BingWeather","Microsoft.GamingApp","Microsoft.BingNews","microsoft.windowscommunicationsapps","Clipchamp","QuickAssist","Microsoft.OutlookForWindows","Microsoft.StorePurchaseApp","Microsoft.WindowsStore"
    Pop-Location

    Push-Location; Write-Host "=== Executing TimezoneRedirection.ps1 ==="
    & .\install\rds_templates\TimezoneRedirection.ps1
    Pop-Location

    Push-Location; Write-Host "=== Executing WindowsOptimization.ps1 ==="
    & .\install\rds_templates\\WDOT\Windows_Optimization.ps1 -ConfigProfile "Schoolyear-AVD" -Optimizations All -AdvancedOptimizations @("Edge", "RemoveOnedrive") -AcceptEULA -Verbose
    Pop-Location

    Push-Location; Write-Host "=== Executing UninstallTeams.ps1 ==="
    & .\install\UninstallTeams.ps1
    Pop-Location
}

# Make sure the user doesn't get a network profile selection popup when they login
Push-Location; Write-Host "=== Executing DisableNetworkProfilePopup.ps1 ==="
& .\install\DisableNetworkProfilePopup.ps1
Pop-Location

# Allow hardcoded IP addresses used by Azure
Push-Location; Write-Host "=== Executing AzureNetwork.ps1 ==="
& .\install\AzureNetwork.ps1
Pop-Location

# Allow hardcoded IP addresses used for Windows Activation
Push-Location; Write-Host "=== Executing WindowsActivationNetwork.ps1 ==="
& .\install\WindowsActivationNetwork.ps1
Pop-Location

# Install the Schoolyear VDI browser
Push-Location; Write-Host "=== Executing InstallVDIBrowser.ps1 ==="
& .\install\InstallVDIBrowser.ps1 -environment $environment
Pop-Location

# Fix Powershell input not working
Push-Location; Write-Host "=== Executing FixPsInput.ps1 ==="
& .\install\FixPsInput.ps1
Pop-Location

# Disable most Outbound firewall rules
Push-Location; Write-Host "=== Executing DisableMostOutboundFirewallRules.ps1 ==="
& .\install\DisableMostOutboundFirewallRules.ps1
Pop-Location

# Fix health status alert bug
Push-Location; Write-Host "=== Executing FixUrlAccessibleCheck.ps1 ==="
& .\install\FixUrlAccessibleCheck.ps1
Pop-Location