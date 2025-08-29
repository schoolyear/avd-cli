Write-Host "Preventing the Network Profile popup/sidebar..."
if (-not (Test-Path -Path "HKLM:\System\CurrentControlSet\Control\Network\NewNetworkWindowOff")) {
    New-Item -Path "HKLM:\System\CurrentControlSet\Control\Network" -Name "NewNetworkWindowOff" -Force
} else {
    Write-Host "Registry key already exists."
}
Write-Host "[DONE]"