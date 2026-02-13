# This script fixes that Azure wrongly assumes it's monitoring is not working.
# It does so by adding a firewall rule for the program that checks if
# the monitoring connection is possible. The check does not respect the proxy configuration,
# while the monitoring itself does. This is why the rule
# is required to prevent the incorrect assumption.
# If this firewall exemption is not applied, the VM will show as "needs assistance".
Param (
    [Parameter(ValueFromRemainingArguments)]
    [string[]]$RemainingArgs                    # To make sure this script doesn't break when new parameters are added
)

# Recommended snippet to make sure PowerShell stops execution on failure
# https://learn.microsoft.com/en-us/powershell/module/microsoft.powershell.core/about/about_preference_variables?view=powershell-7.5#erroractionpreference
# https://learn.microsoft.com/en-us/powershell/module/microsoft.powershell.core/set-strictmode?view=powershell-7.4
$ErrorActionPreference = "Stop"
Set-StrictMode -Version Latest

Write-Host "Start to add scheduled task to add Health Status monitoring"
$CompanyRoot = "C:\ProgramData\SYAzureHealthStatus"
Write-Host "Join folder paths"
$EnforcerPs1 = Join-Path $CompanyRoot "Ensure-RDInfraFirewall.ps1"

$TaskName    = "SY - Ensure RDInfra Firewall Rules"
$TaskPath    = "\"

$ScheduledTaskExecutionPolicy = "RemoteSigned"

Write-Host "Create folder"
New-Item -ItemType Directory -Path $CompanyRoot -Force | Out-Null

$enforcerContent = @'
#Requires -RunAsAdministrator
[CmdletBinding()]
param()

$BasePath     = "C:\Program Files\Microsoft RDInfra"
$ExeName      = "RDAgentBootLoader.exe"
$RuleBaseName = "Allow RDAgentBootLoader Outbound"
$RuleGroup    = "SY-RDInfra-Allowlist"

# Discover current executables (skip reparse points, tolerate access denied)
$found = New-Object System.Collections.Generic.List[string]
$dirs  = New-Object System.Collections.Generic.Queue[string]
$dirs.Enqueue($BasePath)

while ($dirs.Count -gt 0) {
    $d = $dirs.Dequeue()

    try {
        $di = Get-Item -LiteralPath $d -ErrorAction Stop
        if ($di.Attributes -band [IO.FileAttributes]::ReparsePoint) { continue }
    } catch { continue }

    try {
        Get-ChildItem -LiteralPath $d -Filter $ExeName -File -ErrorAction Stop |
            ForEach-Object { $found.Add($_.FullName) }
    } catch { }

    try {
        Get-ChildItem -LiteralPath $d -Directory -ErrorAction Stop |
            ForEach-Object { $dirs.Enqueue($_.FullName) }
    } catch { }
}

$CurrentPaths = $found | Select-Object -Unique
if (-not $CurrentPaths -or $CurrentPaths.Count -eq 0) { return }

# Ensure rules exist
$existingRules = @(Get-NetFirewallRule -Group $RuleGroup -ErrorAction SilentlyContinue)
$existingApps  = @()
if ($existingRules.Count -gt 0) {
    $existingApps = $existingRules | Get-NetFirewallApplicationFilter -ErrorAction SilentlyContinue
}

foreach ($path in $CurrentPaths) {
    $already = $existingApps | Where-Object { $_.Program -eq $path } | Select-Object -First 1
    if (-not $already) {
        $version = Split-Path (Split-Path $path -Parent) -Leaf
        New-NetFirewallRule `
            -DisplayName "$RuleBaseName ($version)" `
            -Group $RuleGroup `
            -Direction Outbound `
            -Program $path `
            -Action Allow `
            -Profile Any `
            -Enabled True | Out-Null
    }
}

# Remove stale rules
$rulesNow = @(Get-NetFirewallRule -Group $RuleGroup -ErrorAction SilentlyContinue)
if ($rulesNow.Count -eq 0) { return }

$appNow = $rulesNow | Get-NetFirewallApplicationFilter -ErrorAction SilentlyContinue

$staleRuleNames = $appNow |
    Where-Object { $_.Program -and ($_.Program -notin $CurrentPaths) } |
    ForEach-Object {
        $instanceId = $_.InstanceID
        $rule = $rulesNow | Where-Object { $_.InstanceID -eq $instanceId } | Select-Object -First 1
        if ($rule) { $rule.Name }
    } |
    Where-Object { $_ } |
    Select-Object -Unique

foreach ($name in $staleRuleNames) {
    Remove-NetFirewallRule -Name $name -ErrorAction SilentlyContinue
}
'@

Write-Host "Write script to folder"
Set-Content -Path $EnforcerPs1 -Value $enforcerContent -Encoding UTF8 -Force
Write-Host "Checking and removing old task"
# Create/replace the scheduled task (SYSTEM, every minute + at startup)
try {
    if (Get-ScheduledTask -TaskName $TaskName -TaskPath $TaskPath -ErrorAction SilentlyContinue) {
        Unregister-ScheduledTask -TaskName $TaskName -TaskPath $TaskPath -Confirm:$false
    }
} catch { 
	Write-Host "Scheduled task cleanup skipped due to an unexpected condition. Continuing with task creation. Error was: $_"
}

Write-Host "Creating new scheduled task"
$action = New-ScheduledTaskAction -Execute "powershell.exe" -Argument (
    "-NoProfile -ExecutionPolicy $ScheduledTaskExecutionPolicy -File `"$EnforcerPs1`""
)
Write-Host "Adding first trigger"
$startup = New-ScheduledTaskTrigger -AtStartup
Write-Host "Adding second trigger"
$repeat  = New-ScheduledTaskTrigger -Once -At (Get-Date).AddMinutes(1) `
  -RepetitionInterval (New-TimeSpan -Minutes 1)

Write-Host "Adding principal"
$principal = New-ScheduledTaskPrincipal -UserId "SYSTEM" -LogonType ServiceAccount -RunLevel Highest
Write-Host "Changing task settings"
$settings = New-ScheduledTaskSettingsSet -Hidden -MultipleInstances IgnoreNew -StartWhenAvailable
Write-Host "Registering scheduled task"
Register-ScheduledTask -TaskName $TaskName -TaskPath $TaskPath `
    -Action $action -Trigger @($startup, $repeat) -Principal $principal -Settings $settings -Force | Out-Null
Write-Host "Finished adding scheduled task to add Health Status monitoring"