function Remove-WDOTRemoveOneDrive
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
        Write-EventLog -EventId 80 -Message "Remove OneDrive Commercial" -LogName 'WDOT' -Source 'AdvancedOptimizations' -EntryType Information
        Write-Host "Windows Advanced Optimize] Removing OneDrive Commercial" -ForegroundColor Cyan
        $OneDrivePath = @('C:\Windows\System32\OneDriveSetup.exe', 'C:\Windows\SysWOW64\OneDriveSetup.exe')   
        $OneDrivePath | ForEach-Object {
            If (Test-Path $_)
            {
                Write-Host "`tAttempting to uninstall $_"
                Write-EventLog -EventId 80 -Message "Commercial $_" -LogName 'WDOT' -Source 'AdvancedOptimizations' -EntryType Information
                Start-Process $_ -ArgumentList "/uninstall" -Wait
            }
        }

        Write-EventLog -EventId 80 -Message "Removing shortcut links for OneDrive" -LogName 'WDOT' -Source 'AdvancedOptimizations' -EntryType Information
        Get-ChildItem 'C:\*' -Recurse -Force -EA SilentlyContinue -Include 'OneDrive', 'OneDrive.exe', 'OneDrive.ico' | Where-Object { $_.FullName -notlike '*\WinSxS\*' } | Remove-Item -Force -Recurse -EA SilentlyContinue

    }
    End
    {
        Write-Verbose "Exiting Function '$($MyInvocation.MyCommand.Name)'"
    }
}
