param (
    [Parameter(Mandatory=$true)]
    [ValidateSet("Development", "Testing", "Beta", "Production")]
    [string]$environment
)

$ErrorActionPreference = "Stop"

# Map environment to appropriate URL
$environmentUrls = @{
    "Development" = "https://dev.install.exams.schoolyear.app/schoolyear-exams-browser-win-3.14.1.msi"
    "Testing" = "https://testing.install.exams.schoolyear.app/schoolyear-exams-browser-win-3.14.1.msi"
    "Beta" = "https://beta.install.exams.schoolyear.app/schoolyear-exams-browser-win-3.14.1.msi"
    "Production" = "https://install.exams.schoolyear.app/schoolyear-exams-browser-win-3.14.1.msi"
}

$vdiBrowserUrl = $environmentUrls[$environment]

function Exe-Download-Install {
    param (
        [string]$ExeUrl,
        [string]$Name,
        [string]$LocalFilePath
    )

    Write-Host "Downloading $Name EXE"
    Invoke-WebRequest $ExeUrl -OutFile $LocalFilePath
    Write-Host "Download of $Name completed"

    Write-Host "Installing $Name"

    $main_process = Start-Process -Wait -FilePath $LocalFilePath -ArgumentList "/install","/quiet" -PassThru
    Write-Host "Installation of $Name complete"

    Write-Host "Removing $Name EXE"
    Remove-Item "$LocalFilePath"

    return $main_process.ExitCode
}

function Msi-Download-Install {
    param (
        [string]$MsiUrl,
        [string]$Name,
        [string]$LocalFilePath,
        [string]$MsiArguments
    )


    Write-Host "Downloading $Name MSI"
    Invoke-WebRequest $MsiUrl -OutFile $LocalFilePath
    Write-Host "Download of $Name completed"

    Write-Host "Installing $Name"

    $msiExecArguments = "/i `"$LocalFilePath`" /q /l*! output.log"
    if (-not [string]::IsNullOrEmpty($MsiArguments)) {
        $msiExecArguments += " " + $MsiArguments
    }

    $main_process = Start-Process msiexec.exe -ArgumentList $msiExecArguments -NoNewWindow -PassThru
    $log_process = Start-Process "powershell" "Get-Content -Path output.log -Wait" -NoNewWindow -PassThru

    # for some ungodly reason, accessing the proc.Handle property, we can properly read the ExitCode
    # https://stackoverflow.com/a/23797762
    $handle = $main_process.Handle # cache proc.Handle
    $main_process.WaitForExit()
    $main_exit_code = $main_process.ExitCode

    $log_process.Kill()
    Write-Host "Installation of $Name complete"

    Write-Host "Removing $Name MSI"
    Remove-Item "$LocalFilePath"

    Write-Host "Removing MSI installation log"
    Remove-Item "output.log"

    return $main_exit_code
}


Write-Host "Download & install .NET"
$exeOut = Exe-Download-Install -ExeUrl "https://download.visualstudio.microsoft.com/download/pr/4cb113f7-9553-4a2b-9c13-cd4fbd0cea30/02da5b68097af3c33b1b4ee5842f327e/dotnet-runtime-6.0.31-win-x86.exe" -Name ".NET runtime" -LocalFilePath "C:\netruntime.exe"
if ($exeOut -ne 0)
{
    Write-Error "Exiting after .NET installation, because exit code is not 0, but $exeOut"
    exit $exeOut
}

Write-Host "Download & install .NET Desktop"
$exeOut = Exe-Download-Install -ExeUrl "https://download.visualstudio.microsoft.com/download/pr/b5fbd3de-7a12-43ba-b460-2f938fd802c3/627f6335ef3ba17bd3ef901c790d7575/windowsdesktop-runtime-6.0.31-win-x86.exe" -Name ".NET desktop runtime" -LocalFilePath "C:\netdesktopruntime.exe"
if ($exeOut -ne 0)
{
    Write-Error "Exiting after .NET Desktop installation, because exit code is not 0, but $exeOut"
    exit $exeOut
}

Write-Host "Download & install VDI browser"
$msiOut = Msi-Download-Install -MsiUrl $vdiBrowserUrl -Name "VDI Browser" -LocalFilePath "C:\vdibrowser.msi" -MsiArguments 'VDIPROVIDER="avd"'
if ($msiOut -ne 0)
{
    Write-Error "Exiting after VDI Browser, because exit code is not 0, but $msiOut"
    exit $msiOut
}

Write-Host "Whitelisting VDI browser in the firewall"
$syFolder = "C:\Program Files (x86)\Schoolyear\"
$installationFolders = @("Schoolyear Browser Development (confidential)", "Schoolyear Browser Testing", "Schoolyear Browser Preview", "Schoolyear Browser")

foreach ($folder in $installationFolders) {
    $syExamsFolder= Join-Path -Path $syFolder -ChildPath $folder

    $syExamsExe = "$syExamsFolder\shell\Schoolyear Exams.exe"
    $syVDIServiceExe = "$syExamsFolder\schoolyear-vdi-service.exe"
    $avdTokenProviderExe = "$syExamsFolder\shell\resources\avd_token_provider.exe"

    # Allow the VDI browser
    New-NetFirewallRule -DisplayName "Allow Schoolyear Browser outbound" -Program $syExamsExe -Direction Outbound -Action Allow -Profile Any | Out-Null
    # Allow the background service
    New-NetFirewallRule -DisplayName "Allow Schoolyear VDI service" -Program $syVDIServiceExe -Direction Outbound -Action Allow -Profile Any | Out-Null
    # Allow the token provider
    New-NetFirewallRule -DisplayName "Allow AVD token provider" -Program $avdTokenProviderExe -Direction Outbound -Action Allow -Profile Any | Out-Null
}

Write-Host "VDI browser installation completed"
