## Parameters
param (
    [Parameter(Mandatory=$false)]
    [string[]]$LayerPaths,

    [Parameter(Mandatory=$false)]
    [switch]$ScanForDirectories = $false,

    [Parameter(Mandatory=$false)]
    [switch]$NoCleanup = $false,

    [Parameter(Mandatory=$false)]
    [switch]$Force = $false
)

## Make sure this script fails on an error and does not continue executing
$ErrorActionPreference = "Stop"
Set-StrictMode -Version Latest

# Validate parameters: either LayerPaths or ScanForDirectories must be set, but not both
if (($LayerPaths -and $ScanForDirectories) -or (-not $LayerPaths -and -not $ScanForDirectories)) {
    Write-Host "Error: You must specify either -LayerPaths or -ScanForDirectories, but not both." -ForegroundColor Red
    Write-Host "Usage examples:" -ForegroundColor Yellow
    Write-Host "  .\v2_execute.ps1 -LayerPaths '.\layer1','..\layer2','C:\layer3'" -ForegroundColor Yellow
    Write-Host "  .\v2_execute.ps1 -ScanForDirectories" -ForegroundColor Yellow
    Write-Host "  Add -NoCleanup to either command to prevent removing layer directories after processing" -ForegroundColor Yellow
    Write-Host "  Add -Force to either command to skip all prompts (processing and cleanup)" -ForegroundColor Yellow
    exit 1
}

## List layers dir paths
# If ScanForDirectories is set, scan the current working directory for all folders
if ($ScanForDirectories) {
    $LayerPaths = Get-ChildItem -Directory | Select-Object -ExpandProperty FullName
}

# Print a summary of all paths that will be used
Write-Host "The following layer directories will be processed:"
foreach ($path in $LayerPaths) {
    Write-Host " - $path"
}


## Validate layers
# Check if the given paths actually exist
# Each directory must at least contain a properties.json or json5 file
# Make sure the following keys exist in the file:
# - version: must be "2"
# - name: must be a non-empty, alphanumeric (with dashes) string
#
# The names must be unique among all layers
# Print a summary of each dir validation status and reason for failing.
# exit with an explicit exit code if not all layers are valid

$ValidLayers = @()
$AllValid = $true
$LayerNames = @{}

Write-Host "`nValidating layers..."

foreach ($path in $LayerPaths) {
    $isValid = $true
    $validationErrors = @()
    $layerName = ""

    # Check if the path exists
    if (-not (Test-Path -Path $path)) {
        $isValid = $false
        $validationErrors += "Path does not exist"
    } else {
        # Check if properties file exists (either json or json5)
        $propertiesJsonPath = Join-Path -Path $path -ChildPath "properties.json"
        $propertiesJson5Path = Join-Path -Path $path -ChildPath "properties.json5"

        $propertiesPath = $null
        if (Test-Path -Path $propertiesJsonPath) {
            $propertiesPath = $propertiesJsonPath
        } elseif (Test-Path -Path $propertiesJson5Path) {
            $propertiesPath = $propertiesJson5Path
        }

        if (-not $propertiesPath) {
            $isValid = $false
            $validationErrors += "Missing properties.json or properties.json5 file"
        } else {
            # Read and validate properties file
            try {
                $content = Get-Content -Path $propertiesPath -Raw

                # Handle JSON5 features (improved comment handling)
                # Remove single-line comments that start with //
                $content = $content -replace '//.*?(?=[\r\n]|$)', ""
                # Remove multi-line comments /* ... */
                $content = $content -replace '/\*[\s\S]*?\*/', ""
                # Allow trailing commas in objects and arrays
                $content = $content -replace ',\s*([\]\}])', '$1'
                # Replace single quotes with double quotes for property names and string values
                $content = $content -replace "([{,]\s*)'([^']*)'\s*:", '$1"$2":'
                $content = $content -replace ":\s*'([^']*)'([,}\]]|$)", ': "$1"$2'

                # Convert to PowerShell object
                $properties = $content | ConvertFrom-Json

                # Validate version
                if (-not $properties.version -or $properties.version -ne "2") {
                    $isValid = $false
                    $validationErrors += "Invalid or missing 'version' property. Must be '2'"
                }

                # Validate name
                if (-not $properties.name -or $properties.name -eq "") {
                    $isValid = $false
                    $validationErrors += "Missing or empty 'name' property"
                } elseif ($properties.name -notmatch '^[a-zA-Z0-9-]+$') {
                    $isValid = $false
                    $validationErrors += "'name' property must only contain alphanumeric characters and dashes"
                } else {
                    $layerName = $properties.name

                    # Check name uniqueness
                    if ($LayerNames.ContainsKey($layerName)) {
                        $isValid = $false
                        $validationErrors += "Duplicate layer name '$layerName' also found in $($LayerNames[$layerName])"
                    } else {
                        $LayerNames[$layerName] = $path
                    }
                }
            } catch {
                $isValid = $false
                $validationErrors += "Error parsing properties file: $($_.Exception.Message)"
            }
        }
    }

    # Print validation result
    if ($isValid) {
        Write-Host " - ${path}: " -NoNewline
        Write-Host "[Valid]" -ForegroundColor Green -NoNewline
        Write-Host " Layer: $layerName"
        $ValidLayers += @{ Path = $path; Name = $layerName }
    } else {
        Write-Host " - ${path}: " -NoNewline
        Write-Host "[Invalid]" -ForegroundColor Red
        foreach ($validationError in $validationErrors) {
            Write-Host "   - $validationError" -ForegroundColor Red
        }
        $AllValid = $false
    }
}

# Exit if not all layers are valid
if (-not $AllValid) {
    Write-Host "`nValidation failed. Please fix the issues and try again." -ForegroundColor Red
    exit 1
}

Write-Host "`nAll layers validated successfully." -ForegroundColor Green

## Prompt for confirmation unless Force is specified
if (-not $Force) {
    Write-Host "`nYou are about to process the following layers:" -ForegroundColor Yellow
    foreach ($layer in $ValidLayers) {
        Write-Host " - $($layer.Name) ($($layer.Path))"
    }

    $confirmation = Read-Host "`nDo you want to continue? (Y/N)"
    if ($confirmation -ne "Y" -and $confirmation -ne "y") {
        Write-Host "Operation cancelled by user." -ForegroundColor Yellow
        exit 0
    }
}

## Execute each layer directory one by one
Write-Host "`nProcessing layers...`n"

foreach ($layer in $ValidLayers) {
    $layerPath = $layer.Path
    $layerName = $layer.Name

    # Print layer name
    Write-Host "Processing layer: $layerName ($layerPath)" -ForegroundColor Cyan

    # Run installation script
    $installScriptPath = Join-Path -Path $layerPath -ChildPath "install.ps1"
    if (Test-Path -Path $installScriptPath) {
        Write-Host " - Running installation script..." -NoNewline
        try {
            # Start a new PowerShell process in the install script's directory
            # This ensures the script runs with its own directory as the working directory
            $process = Start-Process -FilePath "powershell.exe" -ArgumentList "-ExecutionPolicy Bypass -File .\install.ps1" -WorkingDirectory $layerPath -Wait -PassThru -NoNewWindow

            # Check if the process completed successfully
            if ($process.ExitCode -ne 0) {
                throw "Installation script exited with code $($process.ExitCode)"
            }

            Write-Host " Done" -ForegroundColor Green
        } catch {
            Write-Host " Error: $($_.Exception.Message)" -ForegroundColor Red
        }
    } else {
        Write-Host " - Warning: No install.ps1 script found" -ForegroundColor Yellow
    }

    # Move setup script
    $setupScriptPath = Join-Path -Path $layerPath -ChildPath "on_sessionhost_setup.ps1"
    $setupScriptDestDir = "C:\SessionhostScripts"
    $setupScriptDestPath = Join-Path -Path $setupScriptDestDir -ChildPath "$layerName.ps1"

    if (Test-Path -Path $setupScriptPath) {
        if (-not (Test-Path -Path $setupScriptDestDir)) {
            Write-Host " - Creating directory: $setupScriptDestDir"
            New-Item -Path $setupScriptDestDir -ItemType Directory -Force | Out-Null
        }

        Write-Host " - Copying setup script to $setupScriptDestPath"
        Copy-Item -Path $setupScriptPath -Destination $setupScriptDestPath -Force
    } else {
        Write-Host " - No on_sessionhost_setup.ps1 script found"
    }

    # Move user system script
    $userAdminScriptPath = Join-Path -Path $layerPath -ChildPath "on_user_login.admin.ps1"
    $userAdminScriptDestDir = "C:\Scripts"
    $userAdminScriptDestPath = Join-Path -Path $userAdminScriptDestDir -ChildPath "$layerName.ps1"

    if (Test-Path -Path $userAdminScriptPath) {
        if (-not (Test-Path -Path $userAdminScriptDestDir)) {
            Write-Host " - Creating directory: $userAdminScriptDestDir"
            New-Item -Path $userAdminScriptDestDir -ItemType Directory -Force | Out-Null
        }

        Write-Host " - Copying user admin script to $userAdminScriptDestPath"
        Copy-Item -Path $userAdminScriptPath -Destination $userAdminScriptDestPath -Force
    } else {
        Write-Host " - No on_user_login.admin.ps1 script found"
    }

    # Move user script
    $userScriptPath = Join-Path -Path $layerPath -ChildPath "on_user_login.user.ps1"
    $userScriptDestDir = "C:\UserScripts"
    $userScriptDestPath = Join-Path -Path $userScriptDestDir -ChildPath "$layerName.ps1"

    if (Test-Path -Path $userScriptPath) {
        if (-not (Test-Path -Path $userScriptDestDir)) {
            Write-Host " - Creating directory: $userScriptDestDir"
            New-Item -Path $userScriptDestDir -ItemType Directory -Force | Out-Null
        }

        Write-Host " - Copying user script to $userScriptDestPath"
        Copy-Item -Path $userScriptPath -Destination $userScriptDestPath -Force
    } else {
        Write-Host " - No on_user_login.user.ps1 script found"
    }

    Write-Host "Layer processing completed: $layerName`n" -ForegroundColor Cyan
}

## Cleanup
# By default, cleanup is enabled but will prompt before deletion
# If the NoCleanup flag is set, skip the cleanup entirely
Write-Host "Cleanup phase:"

if (-not $NoCleanup) {
    Write-Host "Cleanup is enabled by default." -ForegroundColor Yellow

    $proceedWithCleanup = $false
    if ($Force) {
        Write-Host "Force flag set, proceeding with automatic cleanup..." -ForegroundColor Yellow
        $proceedWithCleanup = $true
    } else {
        $cleanupConfirmation = Read-Host "Do you want to delete all layer directories? (Y/N)"
        $proceedWithCleanup = ($cleanupConfirmation -eq "Y" -or $cleanupConfirmation -eq "y")
    }

    if ($proceedWithCleanup) {
        Write-Host "Deleting all layer directories..." -ForegroundColor Yellow

        foreach ($layer in $ValidLayers) {
            $layerPath = $layer.Path
            $layerName = $layer.Name

            Write-Host " - Removing layer directory: $layerName ($layerPath)" -ForegroundColor Yellow
            try {
                Remove-Item -Path $layerPath -Recurse -Force
                Write-Host "   Done" -ForegroundColor Green
            } catch {
                Write-Host "   Error: $($_.Exception.Message)" -ForegroundColor Red
            }
        }

        Write-Host "Cleanup completed." -ForegroundColor Green
    } else {
        Write-Host "Cleanup cancelled by user." -ForegroundColor Yellow
    }
} else {
    Write-Host "Cleanup is disabled. Run the script without the -NoCleanup parameter to enable cleanup." -ForegroundColor Yellow
}

Write-Host "`nScript execution completed successfully." -ForegroundColor Green