function Optimize-WDOTDefaultUserSettings {
    [CmdletBinding()]

    Param
    (
        [Parameter()]
        [ValidateNotNullOrEmpty()]
        [ValidatePattern('^[^\\/:*?"<>|]+$')]
        [string]$MountedHiveName = 'WDOT_TEMP'
    )

    Begin {
        Write-Verbose "Entering Function '$($MyInvocation.MyCommand.Name)'"

        function ConvertTo-RegistryValueKind {
            param(
                [Parameter(Mandatory = $true)]
                [string]$PropertyType
            )

            switch ($PropertyType.ToUpperInvariant()) {
                'BINARY' { return [Microsoft.Win32.RegistryValueKind]::Binary }
                'DWORD' { return [Microsoft.Win32.RegistryValueKind]::DWord }
                'EXPANDSTRING' { return [Microsoft.Win32.RegistryValueKind]::ExpandString }
                'MULTISTRING' { return [Microsoft.Win32.RegistryValueKind]::MultiString }
                'QWORD' { return [Microsoft.Win32.RegistryValueKind]::QWord }
                'STRING' { return [Microsoft.Win32.RegistryValueKind]::String }
                default { throw "Unsupported registry property type: $PropertyType" }
            }
        }

        function ConvertTo-RegistryValueData {
            param(
                [Parameter(Mandatory = $true)]
                [string]$PropertyType,

                [Parameter()]
                $PropertyValue
            )

            switch ($PropertyType.ToUpperInvariant()) {
                'BINARY' {
                    return , ([byte[]]@(
                            foreach ($Token in @($PropertyValue -split ',')) {
                                $TrimmedToken = [string]$Token
                                if ([string]::IsNullOrWhiteSpace($TrimmedToken)) {
                                    continue
                                }

                                $TrimmedToken = $TrimmedToken.Trim()
                                if ($TrimmedToken.StartsWith('0x', [System.StringComparison]::OrdinalIgnoreCase)) {
                                    [System.Convert]::ToByte($TrimmedToken.Substring(2), 16)
                                }
                                else {
                                    [System.Convert]::ToByte($TrimmedToken, 10)
                                }
                            }
                        ))
                }
                'DWORD' { return [int]$PropertyValue }
                'MULTISTRING' { return [string[]]$PropertyValue }
                'QWORD' { return [long]$PropertyValue }
                default { return [string]$PropertyValue }
            }
        }

        function ConvertTo-RegistrySubKeyPath {
            param(
                [Parameter(Mandatory = $true)]
                [string]$HivePath
            )

            if ($HivePath -notmatch '^[A-Za-z]+:\\[^\\]+\\') {
                throw "Unsupported default user hive path: $HivePath"
            }

            return ($HivePath -replace '^[A-Za-z]+:\\[^\\]+\\', '')
        }
    }
    Process {
        $DefaultUserSettingsFilePath = ".\DefaultUserSettings.json"
        $MountedHiveRegistryPath = 'HKLM\{0}' -f $MountedHiveName
        $MountedHiveProviderPath = 'HKLM:\{0}' -f $MountedHiveName
        If (Test-Path $DefaultUserSettingsFilePath) {
            Write-Host "[Windows Optimize] Set Default User Settings" -ForegroundColor Cyan
            $UserSettings = (Get-Content $DefaultUserSettingsFilePath | ConvertFrom-Json).Where( { $_.OptimizationState -eq "Apply" })
            If ($UserSettings.Count -gt 0) {
                Write-Host "Processing Default User Settings (Registry Keys)"
                $HiveLoaded = $false
                $LocalMachineRoot = $null
                $DefaultUserRoot = $null

                try {
                    & reg.exe load $MountedHiveRegistryPath 'C:\Users\Default\NTUSER.DAT' | Out-Null
                    if ($LASTEXITCODE -ne 0) {
                        throw "Failed to load default user hive into $MountedHiveRegistryPath"
                    }

                    $HiveLoaded = $true
                    Write-Host "Successfully loaded $MountedHiveRegistryPath"

                    $LocalMachineRoot = [Microsoft.Win32.RegistryKey]::OpenBaseKey(
                        [Microsoft.Win32.RegistryHive]::LocalMachine,
                        [Microsoft.Win32.RegistryView]::Default
                    )
                    $DefaultUserRoot = $LocalMachineRoot.OpenSubKey($MountedHiveName, $true)
                    if (-not $DefaultUserRoot) {
                        throw "Failed to open mounted default user hive $MountedHiveRegistryPath"
                    }

                    Foreach ($Item in $UserSettings) {
                        $SubKeyPath = ConvertTo-RegistrySubKeyPath -HivePath $Item.HivePath
                        $ResolvedHivePath = '{0}\{1}' -f $MountedHiveProviderPath, $SubKeyPath
                        $RegistryValueKind = ConvertTo-RegistryValueKind -PropertyType $Item.PropertyType
                        $Value = ConvertTo-RegistryValueData -PropertyType $Item.PropertyType -PropertyValue $Item.PropertyValue
                        $RegistryKey = $null

                        try {
                            $RegistryKey = $DefaultUserRoot.CreateSubKey($SubKeyPath)

                            if (-not $RegistryKey) {
                                Write-EventLog -EventId 140 -Message "Failed to create new Registry Key" -LogName 'WDOT' -Source 'DefaultUserSettings' -EntryType Error
                                continue
                            }

                            Write-Host "Setting $ResolvedHivePath - $($Item.KeyName) to $Value ($($Item.PropertyType))"
                            $RegistryKey.SetValue($Item.KeyName, $Value, $RegistryValueKind)
                            Write-Host "Set $ResolvedHivePath - $Value"
                        }
                        finally {
                            if ($RegistryKey) { $RegistryKey.Dispose() }
                        }
                    }
                }
                finally {
                    if ($DefaultUserRoot) { $DefaultUserRoot.Dispose() }
                    if ($LocalMachineRoot) { $LocalMachineRoot.Dispose() }

                    if ($HiveLoaded) {
                        & reg.exe unload $MountedHiveRegistryPath | Out-Null
                        if ($LASTEXITCODE -ne 0) {
                            throw "Failed to unload default user hive from $MountedHiveRegistryPath"
                        }

                        Write-Host "Successfully unloaded $MountedHiveRegistryPath"
                    }
                }
            }
            Else {
                Write-Host "No Default User Settings to set"
            }
        }
        Else {
            Write-Host "Default User Settings file not found at path: $DefaultUserSettingsFilePath"
        }
    }
    End {
        Write-Host "Exiting Function '$($MyInvocation.MyCommand.Name)'"
    }
}
