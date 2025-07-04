# This script is executed on each sessionhost during deployment
# Note that any time spent in this script adds to the deployment time of each VM (and thus the deployment time of exams)

$ErrorActionPreference = "Stop"
Set-StrictMode -Version Latest

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