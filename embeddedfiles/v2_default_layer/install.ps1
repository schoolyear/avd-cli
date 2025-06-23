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

    # deactivated, because we dont expect students to do (video)calls during an exam
    # additionally, Chrome nor Edge are used and our VDI browser has no support for this extension
    #Push-Location; Write-Host "=== Executing MultiMediaRedirection.ps1 ==="
    #& .\install\rds_templates\MultiMediaRedirection.ps1 -VCRedistributableLink "https://aka.ms/vs/17/release/vc_redist.x64.exe" -EnableEdge "false" -EnableChrome "false" # before windows optimization because it cleans the TEMP folder used by this script
    #Pop-Location

    Push-Location; Write-Host "=== Executing RemoveAppxPackages.ps1 ==="
    & .\install\rds_templates\RemoveAppxPackages.ps1 -AppxPackages "Microsoft.XboxApp","Microsoft.ZuneVideo","Microsoft.ZuneMusic","Microsoft.YourPhone","Microsoft.XboxSpeechToTextOverlay","Microsoft.XboxIdentityProvider","Microsoft.XboxGamingOverlay","Microsoft.XboxGameOverlay","Microsoft.Xbox.TCUI","Microsoft.WindowsSoundRecorder","Microsoft.WindowsMaps","Microsoft.WindowsFeedbackHub","Microsoft.WindowsCamera","Microsoft.WindowsAlarms","Microsoft.Todos","Microsoft.SkypeApp","Microsoft.ScreenSketch","Microsoft.PowerAutomateDesktop","Microsoft.People","Microsoft.MicrosoftStickyNotes","Microsoft.MicrosoftSolitaireCollection","Microsoft.Office.OneNote","Microsoft.MicrosoftOfficeHub","Microsoft.Getstarted","Microsoft.GetHelp","Microsoft.BingWeather","Microsoft.GamingApp","Microsoft.BingNews"
    Pop-Location

    Push-Location; Write-Host "=== Executing TimezoneRedirection.ps1 ==="
    & .\install\rds_templates\TimezoneRedirection.ps1
    Pop-Location

    Push-Location; Write-Host "=== Executing WindowsOptimization.ps1 ==="
    & .\install\rds_templates\WindowsOptimization.ps1 -Optimizations "All"
    Pop-Location

    # VDOT is very similar to WindowsOptimization, which was specifically adapted to fit AVD
    # Historically, VDOT was run, because it is a bit more aggresive
    # If WindowsOptimization.ps1 doesn't turn out to be enough, we may have to enable VDOT again
    #Push-Location; Write-Host "=== Executing Windows_VDOT.ps1 ==="
    #& .\install\vdot\Windows_VDOT.ps1 -Optimizations All -AdvancedOptimizations All -AcceptEULA -Verbose
    #Pop-Location

    Push-Location; Write-Host "=== Executing UninstallTeams.ps1 ==="
    & .\install\UninstallTeams.ps1 -DisableOfficeTeamsInstall
    Pop-Location
}

# Make sure the user doesn't get a network profile selection popup when they login
Push-Location; Write-Host "=== Executing DisableNetworkProfilePopup.ps1 ==="
& .\install\DisableNetworkProfilePopup.ps1
Pop-Location

Push-Location; Write-Host "=== Executing AzureNetwork.ps1 ==="
& .\install\AzureNetwork.ps1
Pop-Location

# Install the Schoolyear VDI browser
Push-Location; Write-Host "=== Executing InstallVDIBrowser.ps1 ==="
& .\install\InstallVDIBrowser.ps1 # todo: download URL depending on environment
Pop-Location
