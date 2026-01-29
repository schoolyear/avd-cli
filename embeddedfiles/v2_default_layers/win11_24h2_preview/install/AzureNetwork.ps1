# Allow hardcoded IP addresses used by Azure
# https://learn.microsoft.com/en-us/azure/virtual-desktop/required-fqdn-endpoint?tabs=azure#:~:text=Azure%20Instance%20Metadata%20service%20endpoint
# https://learn.microsoft.com/en-us/azure/virtual-desktop/required-fqdn-endpoint?tabs=azure#:~:text=Session%20host%20health%20monitoring

New-NetFirewallRule -DisplayName "Allow metadata service outbound" -Group "Schoolyear AVD" -RemoteAddress 169.254.169.254 -Direction Outbound -Action Allow -Profile Any | Out-Null
New-NetFirewallRule -DisplayName "Allow health service monitor outbound" -Group "Schoolyear AVD" -RemoteAddress 168.63.129.16 -Direction Outbound -Action Allow -Profile Any | Out-Null