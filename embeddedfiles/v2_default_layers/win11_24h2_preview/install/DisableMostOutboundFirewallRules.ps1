$ErrorActionPreference = "Stop"

# Firewall rules from the following groups are excluded from the disable action below. This is because they are arbitrarily determined to be essential.
# The comments list the reasons shortly.
# Also, since the default layer is not apparent to most customers, removing more of these might cause intensive troubleshooting.

$exclude = @(
    "Core Networking", 
    "Cloud Identity", 
    "Print Queue", #At least one customer prints in AVD exams.
    "Windows Device Management", # Excluding this might interfere with for example Intune. While Intune is not recommended/supported, it might break older image builds.
    "Windows Feature Experience Pack", #Integrated in Windows UI, leaving it out breaks it.
    "Windows Print", #At least one customer prints in AVD exams.
    "Windows Shell Experience", # Breaks Windows UI if excluded.
    "Windows Terminal", #Terminal might be used by students or in installation scripts.
    "Work or school account",
    "Schoolyear AVD"   #IPs used by Azure and Windows, added to firewall in our own scripts
)


Get-NetFirewallRule |
Where-Object {
    $_.Direction -eq "Outbound" -and
    $_.DisplayGroup -notin $exclude
} |
Disable-NetFirewallRule

Write-Host "Disabled most outbound firewall rules"