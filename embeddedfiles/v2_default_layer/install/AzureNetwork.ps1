# Allow hardcoded IP addresses used by Azure
New-NetFirewallRule -DisplayName "Allow metadata service outbound" -RemoteAddress 169.254.169.254 -Direction Outbound -Action Allow -Profile Any | Out-Null
New-NetFirewallRule -DisplayName "Allow health service monitor outbound" -RemoteAddress 168.63.129.16 -Direction Outbound -Action Allow -Profile Any | Out-Null