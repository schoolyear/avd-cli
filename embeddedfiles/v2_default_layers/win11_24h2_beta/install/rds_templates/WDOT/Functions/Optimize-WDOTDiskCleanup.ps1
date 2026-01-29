function Optimize-WDOTDiskCleanup
{
    [CmdletBinding()]

    Param
    (

    )

    Begin
    {
        Write-Verbose "Entering Function '$($MyInvocation.MyCommand.Name)'"
    }
    Process
    {
        try
        {
            Write-EventLog -EventId 90 -Message "Removing .tmp, .etl, .evtx, thumbcache*.db, *.log files not in use" -LogName 'WDOT' -Source 'DiskCleanup' -EntryType Information
            Write-Verbose "Removing .tmp, .etl, .evtx, thumbcache*.db, *.log files not in use"
            Get-ChildItem -Path c:\ -Include *.tmp, *.dmp, *.etl, *.evtx, thumbcache*.db, *.log -File -Recurse -Force -ErrorAction SilentlyContinue | Remove-Item -ErrorAction SilentlyContinue

            # Delete "RetailDemo" content (if it exits)
            Write-EventLog -EventId 90 -Message "Removing Retail Demo content (if it exists)" -LogName 'WDOT' -Source 'DiskCleanup' -EntryType Information
            Write-Verbose "Removing Retail Demo content (if it exists)"
            Get-ChildItem -Path $env:ProgramData\Microsoft\Windows\RetailDemo\* -Recurse -Force -ErrorAction SilentlyContinue | Remove-Item -Recurse -ErrorAction SilentlyContinue

            # Delete not in-use anything in the C:\Windows\Temp folder
            Write-EventLog -EventId 90 -Message "Removing all files not in use in $env:windir\TEMP" -LogName 'WDOT' -Source 'DiskCleanup' -EntryType Information
            Write-Verbose "Removing all files not in use in $env:windir\TEMP"
            Remove-Item -Path $env:windir\Temp\* -Recurse -Force -ErrorAction SilentlyContinue -Exclude packer*.ps1

            # Clear out Windows Error Reporting (WER) report archive folders
            Write-EventLog -EventId 90 -Message "Cleaning up WER report archive" -LogName 'WDOT' -Source 'DiskCleanup' -EntryType Information
            Write-Verbose "Cleaning up WER report archive"
            Remove-Item -Path $env:ProgramData\Microsoft\Windows\WER\Temp\* -Recurse -Force -ErrorAction SilentlyContinue
            Remove-Item -Path $env:ProgramData\Microsoft\Windows\WER\ReportArchive\* -Recurse -Force -ErrorAction SilentlyContinue
            Remove-Item -Path $env:ProgramData\Microsoft\Windows\WER\ReportQueue\* -Recurse -Force -ErrorAction SilentlyContinue

            # Delete not in-use anything in your %temp% folder
            Write-EventLog -EventId 90 -Message "Removing files not in use in $env:temp directory" -LogName 'WDOT' -Source 'DiskCleanup' -EntryType Information
            Write-Verbose "Removing files not in use in $env:temp directory"
            Remove-Item -Path $env:TEMP\* -Recurse -Force -ErrorAction SilentlyContinue -Exclude packer*.ps1

            # Clear out ALL visible Recycle Bins
            Write-EventLog -EventId 90 -Message "Clearing out ALL Recycle Bins" -LogName 'WDOT' -Source 'DiskCleanup' -EntryType Information
            Write-Verbose "Clearing out ALL Recycle Bins"
            Clear-RecycleBin -Force -ErrorAction SilentlyContinue

            # Clear out BranchCache cache
            Write-EventLog -EventId 90 -Message "Clearing BranchCache cache" -LogName 'WDOT' -Source 'DiskCleanup' -EntryType Information
            Write-Verbose "Clearing BranchCache cache"
            Clear-BCCache -Force -ErrorAction SilentlyContinue
        }
        catch
        {
            Write-Verbose "DiskCleanup encountered a non-critical error: $($_.Exception.Message)"
        }

    }
    End
    {
        Write-Verbose "Exiting Function '$($MyInvocation.MyCommand.Name)'"
    }
}
