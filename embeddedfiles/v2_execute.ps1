## Parameters
param (
    [Parameter(Mandatory=$false)]
    [string[]]$LayerPaths,

    [Parameter(Mandatory=$false)]
    [switch]$ScanForDirectories = $false,

    [Parameter(Mandatory=$false)]
    [switch]$NoCleanup = $false,

    [Parameter(Mandatory=$false)]
    [switch]$Force = $false,

    [Parameter(Mandatory=$false)]
    [string]$BuildParametersPath = "build_parameters.json"
)

## Make sure this script fails on an error and does not continue executing
$ErrorActionPreference = "Stop"
Set-StrictMode -Version Latest

Get-Host

## Add detailed error handling helper function
function Write-ExceptionDetails {
    param (
        [Parameter(Mandatory=$true, ValueFromPipeline=$true)]
        [System.Management.Automation.ErrorRecord]$ErrorRecord
    )

    process {
        Write-Host "=== EXCEPTION DETAILS ===" -ForegroundColor Red
        Write-Host "Error Message: $($ErrorRecord.Exception.Message)" -ForegroundColor Yellow
        Write-Host "Exception Type: $($ErrorRecord.Exception.GetType().FullName)" -ForegroundColor Yellow

        Write-Host "`n=== ERROR RECORD DETAILS ===" -ForegroundColor Red
        Write-Host "CategoryInfo: $($ErrorRecord.CategoryInfo)" -ForegroundColor Yellow
        Write-Host "FullyQualifiedErrorId: $($ErrorRecord.FullyQualifiedErrorId)" -ForegroundColor Yellow

        if ($ErrorRecord.ScriptStackTrace) {
            Write-Host "`n=== SCRIPT STACK TRACE ===" -ForegroundColor Red
            Write-Host $ErrorRecord.ScriptStackTrace -ForegroundColor Yellow
        }

        if ($ErrorRecord.Exception.StackTrace) {
            Write-Host "`n=== EXCEPTION STACK TRACE ===" -ForegroundColor Red
            Write-Host $ErrorRecord.Exception.StackTrace -ForegroundColor Yellow
        }

        if ($ErrorRecord.Exception.InnerException) {
            Write-Host "`n=== INNER EXCEPTION ===" -ForegroundColor Red
            Write-Host "Message: $($ErrorRecord.Exception.InnerException.Message)" -ForegroundColor Yellow
            Write-Host "Type: $($ErrorRecord.Exception.InnerException.GetType().FullName)" -ForegroundColor Yellow

            if ($ErrorRecord.Exception.InnerException.StackTrace) {
                Write-Host "`n=== INNER EXCEPTION STACK TRACE ===" -ForegroundColor Red
                Write-Host $ErrorRecord.Exception.InnerException.StackTrace -ForegroundColor Yellow
            }
        }

        # Additional PowerShell specific details
        Write-Host "`n=== INVOCATION INFO ===" -ForegroundColor Red
        Write-Host "ScriptName: $($ErrorRecord.InvocationInfo.ScriptName)" -ForegroundColor Yellow
        Write-Host "Line Number: $($ErrorRecord.InvocationInfo.ScriptLineNumber)" -ForegroundColor Yellow
        Write-Host "Position Message: $($ErrorRecord.InvocationInfo.PositionMessage)" -ForegroundColor Yellow
        Write-Host "Line: $($ErrorRecord.InvocationInfo.Line)" -ForegroundColor Yellow
    }
}

# Validate parameters: either LayerPaths or ScanForDirectories must be set, but not both
if (($LayerPaths -and $ScanForDirectories) -or (-not $LayerPaths -and -not $ScanForDirectories)) {
    Write-Host "Error: You must specify either -LayerPaths or -ScanForDirectories, but not both." -ForegroundColor Red
    Write-Host "Usage examples:" -ForegroundColor Yellow
    Write-Host "  .\v2_execute.ps1 -LayerPaths '.\layer1','..\layer2','C:\layer3'" -ForegroundColor Yellow
    Write-Host "  .\v2_execute.ps1 -ScanForDirectories" -ForegroundColor Yellow
    Write-Host "  .\v2_execute.ps1 -LayerPaths '.\layer1' -BuildParametersPath 'path\to\custom_parameters.json'" -ForegroundColor Yellow
    Write-Host "  Add -NoCleanup to either command to prevent removing layer directories after processing" -ForegroundColor Yellow
    Write-Host "  Add -Force to either command to skip all prompts (processing and cleanup)" -ForegroundColor Yellow
    exit 1
}

## List layers dir paths
# If ScanForDirectories is set, scan the current working directory for all folders
if ($ScanForDirectories) {
    $LayerPaths = Get-ChildItem -Directory | Sort-Object Name | Select-Object -ExpandProperty FullName
}

# Print a summary of all paths that will be used
Write-Host "The following layer directories will be processed:"
foreach ($path in $LayerPaths) {
    Write-Host " - $path"
}

$ValidLayers = @()
$AllValid = $true
$LayerNames = @{}

Write-Host "`nValidating layers..."

foreach ($path in $LayerPaths) {
    $validationErrors = @()
    $layerName = ""

    # Check if the path exists
    if (-not (Test-Path -Path $path)) {
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
                if (-not $properties.version -or $properties.version -ne "v2") {
                    $validationErrors += "Invalid or missing 'version' property. Must be 'v2'"
                }

                # Validate name
                if (-not $properties.name -or $properties.name -eq "") {
                    $validationErrors += "Missing or empty 'name' property"
                } elseif ($properties.name -notmatch '^[a-zA-Z0-9-.]+$') {
                    $validationErrors += "'name' property must only contain alphanumeric characters and dashes"
                } else {
                    $layerName = $properties.name

                    # Check name uniqueness
                    if ($LayerNames.ContainsKey($layerName)) {
                        $validationErrors += "Duplicate layer name '$layerName' also found in $($LayerNames[$layerName])"
                    } else {
                        $LayerNames[$layerName] = $path
                    }
                }
            } catch {
                $validationErrors += "Error parsing properties file: $($_.Exception.Message)"
            }
        }
    }

    # Print validation result
    if ($validationErrors.Count -eq 0) {
        Write-Host " - ${path}: " -NoNewline
        Write-Host "[Valid]" -ForegroundColor Green -NoNewline
        Write-Host " Layer: $layerName"
        $ValidLayers += @{ Path = $path; Properties = $properties }
    }
    else {
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
    Write-Error "`nValidation failed. Please fix the issues and try again."
    exit 1
}

Write-Host "`nAll layers validated successfully." -ForegroundColor Green

## Validate and load build parameters file
$BuildParameters = $null
Write-Host "`nChecking for build parameters file: $BuildParametersPath"
if (Test-Path -Path $BuildParametersPath) {
    try {
        $BuildParametersContent = Get-Content -Path $BuildParametersPath -Raw
        $BuildParameters = $BuildParametersContent | ConvertFrom-Json

        # Validate build parameters file
        if (-not $BuildParameters.version -or $BuildParameters.version -ne "v2") {
            Write-Error "Invalid or missing 'version' property in build parameters file. Must be 'v2'."
            exit 1
        }

        if (-not $BuildParameters.layers -or $BuildParameters.layers -isnot [PSCustomObject]) {
            Write-Error "Missing or invalid 'layers' property in build parameters file. Must be an object."
            exit 1
        }

        Write-Host " - Build parameters file loaded successfully." -ForegroundColor Green
    } catch {
        Write-Error "Error parsing build parameters file: $($_.Exception.Message)"
        exit 1
    }
} else {
    Write-Host " - Build parameters file not found. Parameters will not be passed to installation scripts." -ForegroundColor Yellow
}

## Prompt for confirmation unless Force is specified
if (-not $Force) {
    Write-Host "`nYou are about to process the following layers:" -ForegroundColor Yellow
    foreach ($layer in $ValidLayers) {
        Write-Host " - $($layer.Properties.name) ($($layer.Path))"
    }

    $confirmation = Read-Host "`nDo you want to continue? (Y/N)"
    if ($confirmation -ne "Y" -and $confirmation -ne "y") {
        Write-Host "Operation cancelled by user." -ForegroundColor Yellow
        exit 0
    }
}

## Create script directories once before processing any layers
Write-Host "`nCreating script directories...`n"
$setupScriptDestDir = "C:\SessionhostScripts"
$userAdminScriptDestDir = "C:\Scripts"
$userScriptDestDir = "C:\UserScripts"

# Create directories if they don't exist
foreach ($dir in @($setupScriptDestDir, $userAdminScriptDestDir, $userScriptDestDir)) {
    if (-not (Test-Path -Path $dir)) {
        Write-Host " - Creating directory: $dir"
        New-Item -Path $dir -ItemType Directory -Force | Out-Null
    } else {
        Write-Host " - Directory already exists: $dir"
    }
}

## Execute each layer directory one by one
Write-Host "`nProcessing layers...`n"

for ($layerIndex = 0; $layerIndex -lt $ValidLayers.Count; $layerIndex++) {
    $layer = $ValidLayers[$layerIndex]

    $layerPath = $layer.Path
    $layerName = $layer.Properties.name

    # Print layer name
    Write-Host "Processing layer: $layerName ($layerPath)" -ForegroundColor Cyan

    # Run installation script
    $installScriptPath = Join-Path -Path $layerPath -ChildPath "install.ps1"
    if (Test-Path -Path $installScriptPath) {
        Write-Host " - Running installation script..."
        Push-Location $layerPath
        try {
            # Check if we have parameters for this layer in the build parameters file
            $scriptParams = @{}
            if ($BuildParameters -and $BuildParameters.layers.PSObject.Properties.Name -contains $layerName) {
                $layerParams = $BuildParameters.layers.$layerName

                # Process each parameter in the layer
                foreach ($paramName in $layerParams.PSObject.Properties.Name) {
                    $paramValue = $layerParams.$paramName.value
                    $scriptParams[$paramName] = $paramValue
                }

                # Log the parameters
                if ($scriptParams.Count -gt 0) {
                    Write-Host " - Passing the following parameters to the installation script:" -ForegroundColor Cyan
                    foreach ($param in $scriptParams.GetEnumerator()) {
                        Write-Host "   - $($param.Key): $($param.Value)" -ForegroundColor Cyan
                    }
                }
            }

            # Execute script with parameters if any
            if ($scriptParams.Count -gt 0) {
                & ".\install.ps1" @scriptParams
            } else {
                & ".\install.ps1"
            }

            if (!$?)
            {
               throw "Installation script failed"
            }
            Write-Host "Done" -ForegroundColor Green
        } catch {
            Write-Host "Error occurred in installation script for layer " -NoNewline -ForegroundColor Red
            Write-Host "$( $layerName )" -NoNewline -ForegroundColor Magenta
            Write-Host ":" -ForegroundColor Red
            $_ | Write-ExceptionDetails
            exit 1
        } finally {
            Pop-Location
        }
    }
    else
    {
        Write-Host " - Warning: This layer has no install.ps1 script" -ForegroundColor Yellow
    }

    $indexPrefix = "{0:D3}" -f $($layerIndex + 1)

    # Move setup script
    $setupScriptPath = Join-Path -Path $layerPath -ChildPath "on_sessionhost_setup.ps1"
    $setupScriptDestPath = Join-Path -Path $setupScriptDestDir -ChildPath "$($indexPrefix)_$layerName.ps1"

    if (Test-Path -Path $setupScriptPath) {
        Write-Host " - Copying setup script to $setupScriptDestPath"
        Copy-Item -Path $setupScriptPath -Destination $setupScriptDestPath -Force
    } else {
        Write-Host " - No on_sessionhost_setup.ps1 script found"
    }

    # Move user system script
    $userAdminScriptPath = Join-Path -Path $layerPath -ChildPath "on_user_login.admin.ps1"
    $userAdminScriptDestPath = Join-Path -Path $userAdminScriptDestDir -ChildPath "$($indexPrefix)_$layerName.ps1"

    if (Test-Path -Path $userAdminScriptPath) {
        Write-Host " - Copying user admin script to $userAdminScriptDestPath"
        Copy-Item -Path $userAdminScriptPath -Destination $userAdminScriptDestPath -Force
    } else {
        Write-Host " - No on_user_login.admin.ps1 script found"
    }

    # Move user script
    $userScriptPath = Join-Path -Path $layerPath -ChildPath "on_user_login.user.ps1"
    $userScriptDestPath = Join-Path -Path $userScriptDestDir -ChildPath "$($indexPrefix)_$layerName.ps1"

    if (Test-Path -Path $userScriptPath) {
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
            $layerName = $layer.Properties.name

            Write-Host " - Removing layer directory: $layerName ($layerPath)" -ForegroundColor Yellow
            try {
                Remove-Item -Path $layerPath -Recurse -Force
                Write-Host "   Done" -ForegroundColor Green
            } catch {
                Write-Error "   Error: $($_.Exception.Message)"
                exit 1
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