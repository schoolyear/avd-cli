# This script is executed on each sessionhost during deployment
# Note that any time spent in this script adds to the deployment time of each VM (and thus the deployment time of exams)

$ErrorActionPreference = "Stop"
Set-StrictMode -Version Latest

# set the default network profile to public
Set-NetConnectionProfile -NetworkCategory Public

# fetch userData
# this is a hardcoded IP to fetch VM metadata in Azure
$url = "http://169.254.169.254/metadata/instance/compute/userData?api-version=2021-01-01&format=text"
$headers = @{
    "Metadata" = "true"
}

try {
    $response = Invoke-RestMethod -Uri $url -Headers $headers -Method Get
} catch {
    Write-Error "Could not make request to metadata endpoint (userData): $_"
    exit 1
}

# decode userData string blob (base64) and parse as json
$userData = [System.Text.Encoding]::UTF8.GetString([System.Convert]::FromBase64String($response)) | ConvertFrom-Json

# Find proxy LB private IP address to whitelist
$proxyLoadBalancerPrivateIpAddress = $userData.proxyLoadBalancerPrivateIpAddress
if ([string]::IsNullOrEmpty($proxyLoadBalancerPrivateIpAddress)) {
    Write-Error "proxyLoadBalancerPrivateIpAddress is empty"
    exit 1
}

# Find AVD Endpoints ip range to whitelist
$avdEndpointsIpRange = $userData.avdEndpointsIpRange
if ([string]::IsNullOrEmpty($avdEndpointsIpRange)) {
    Write-Error "avdEndpointsIpRange is empty"
    exit 1
}

# Allow communication to the AVD services
New-NetFirewallRule -DisplayName "Allow sessionhost proxy LB outbound ($proxyLoadBalancerPrivateIpAddress)" -RemoteAddress $proxyLoadBalancerPrivateIpAddress -Direction Outbound -Action Allow -Profile Any | Out-Null

# Allow communication to the AVD endpoints subnet
New-NetFirewallRule -DisplayName "Allow all outbound to azure AVD endpoints $avdEndpointsIpRange" -Direction Outbound -RemoteAddress $avdEndpointsIpRange -Action Allow -Profile Any | Out-Null

# We map a local domain name to point to the LB private IP
$hostsFilepath = "C:\Windows\System32\drivers\etc\hosts"
$domain = "proxies.local"
$hostsFileUpdated = $false

$retryWaitTimeInSeconds = 5
for($($retry = 1; $maxRetries = 5); $retry -le $maxRetries; $retry++) {
    try {
        # Prior to PowerShell 6.2, Add-Content takes a read lock, so if another process is already reading
        # the hosts file by the time we attempt to write to it, the cmdlet fails. This is a bug in older versions of PS.
        # https://github.com/PowerShell/PowerShell/issues/5924
        #
        # Using Out-File cmdlet with -Append flag reduces the chances of failure.

        "$proxyLoadBalancerPrivateIpAddress $domain" | Out-File -FilePath $hostsFilepath -Encoding Default -Append
        $hostsFileUpdated = $true;
        break
    } catch {
        Write-Host "Failed to update hosts file. Trying again... ($retry/$maxRetries)";
        Start-Sleep -Seconds $retryWaitTimeInSeconds
    }
}

if (!$hostsFileUpdated) {
    Write-Error "Could not update hosts file."
    exit 1
}

Write-Host "Updated hosts file"

ipconfig /flushdns

$proxyBytes = [System.Text.Encoding]::UTF8.GetBytes("PROXY $($proxyLoadBalancerPrivateIpAddress):8080")
$proxyStringBase64 = [Convert]::ToBase64String($proxyBytes)
$matchingProxyBase64 = $proxyStringBase64.Replace('+','-').Replace('/','_').TrimEnd('=')
$pacUrl = "http://${domain}:8080/proxy.pac?matchingProxyBase64=$matchingProxyBase64&defaultProxy=DIRECT"

try
{
    Write-Host "Setting system proxy..."
    bitsadmin /util /setieproxy LOCALSYSTEM AUTOSCRIPT "$pacUrl"
    bitsadmin /util /setieproxy NETWORKSERVICE AUTOSCRIPT "$pacUrl"
}
catch
{
    Write-Error "Could not set system proxy: $_"
    exit 1
}