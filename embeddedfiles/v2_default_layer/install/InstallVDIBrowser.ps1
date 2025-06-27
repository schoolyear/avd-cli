param (
    [Parameter(Mandatory=$true)]
    [ValidateSet("Development", "Testing", "Beta", "Production")]
    [string]$environment
)

$ErrorActionPreference = "Stop"

# Map environment to appropriate URL
$environmentUrls = @{
    "Development" = "https://dev.install.exams.schoolyear.app/schoolyear-exams-browser-win-3.8.5.msi"
    "Testing" = "https://testing.install.exams.schoolyear.app/schoolyear-exams-browser-win-3.8.5.msi"
    "Beta" = "https://beta.install.exams.schoolyear.app/schoolyear-exams-browser-win-3.8.5.msi"
    "Production" = "https://install.exams.schoolyear.app/schoolyear-exams-browser-win-3.8.5.msi"
}

$vdiBrowserUrl = $environmentUrls[$environment]

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

Write-Host "Download & install VDI browser"
$msiOut = Msi-Download-Install -MsiUrl $vdiBrowserUrl -Name "VDI Browser" -LocalFilePath "C:\vdibrowser.msi" -MsiArguments 'VDIPROVIDER="avd"'
if ($msiOut -ne 0)
{
    Write-Error "Exiting after VDI Browser, because exit code is not 0, but $msiOut"
    exit $msiOut
}

Write-Host "VDI browser installation completed"
