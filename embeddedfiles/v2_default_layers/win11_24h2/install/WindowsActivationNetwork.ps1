# Allow hardcoded IP addresses used by Windows to activate Windows in Azure
# Retrieved from https://github.com/MicrosoftDocs/SupportArticles-docs/blob/main/support/azure/virtual-machines/windows/custom-routes-enable-kms-activation.md?plain=1#L42

New-NetFirewallRule -DisplayName "Allow Windows Activation 1" -Group "Schoolyear AVD" -RemoteAddress 20.118.99.224 -Direction Outbound -Action Allow -Profile Any | Out-Null
New-NetFirewallRule -DisplayName "Allow Windows activation 2" -Group "Schoolyear AVD" -RemoteAddress 40.83.235.53 -Direction Outbound -Action Allow -Profile Any | Out-Null
New-NetFirewallRule -DisplayName "Allow Windows activation 3" -Group "Schoolyear AVD" -RemoteAddress 23.102.135.246 -Direction Outbound -Action Allow -Profile Any | Out-Null
