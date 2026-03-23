# RDS Templates

Based on: https://github.com/Azure/RDS-Templates/blob/master/CustomImageTemplateScripts/CustomImageTemplateScripts_2024-03-27

## Modifications

(may be non-exhaustive)

- DisableAutoUpdates: the scripts didn't have a colon after `HKLM`/`HKCU`. Unclear how these scripts ever worked.

# WDOT

Based on [WDOT github repo](https://github.com/The-Virtual-Desktop-Team/Windows-Desktop-Optimization-Tool).
Included the readme for future reference.

## Modifications

- Changed the Registry edits in Optimize-WDOTDefaultUserSettings.ps1 to use .NET a.o.t. Powershell cmdlets & made unloading of the Registry hive critical.
  The powershell cmdlets that edited the Registry, sometimes prevented the hive from being unloaded, hence the change.
- Corrected the log messages for the registry edits (Optimize-WDOTLocalPolicySettings.ps1). They wrongly stated errors for expected scenarios.
- Set it to Verbose to make troubleshooting easier.

- Commented out a few lines, because they sometimes break, or fail anyway:
  - `Get-ChildItem 'C:\*' -Recurse -Force -EA SilentlyContinue -Include 'OneDrive','OneDrive.*' | Remove-Item -Force -Recurse -EA SilentlyContinue`
  - `Get-ChildItem -Path c:\ -Include *.tmp, *.dmp, *.etl, *.evtx, thumbcache*.db, *.log -File -Recurse -Force -ErrorAction SilentlyContinue | Remove-Item -ErrorAction SilentlyContinue`
  - `Clear-BCCache -Force -ErrorAction SilentlyContinue`

## Configuration comments

  - GetStarted can't be removed, as it breaks snipping tool. Getstarted the AppxPackage is the Tips app, they have the same name, that is removed.
  - One reg item optimization is skipped since it returned an error. (HKLM:\\SOFTWARE\\Microsoft\\Windows\\ScriptedDiagnosticsProvider\\Policy)