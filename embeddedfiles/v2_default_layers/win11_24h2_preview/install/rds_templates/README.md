# RDS Templates

Based on: https://github.com/Azure/RDS-Templates/blob/master/CustomImageTemplateScripts/CustomImageTemplateScripts_2024-03-27

## Modifications

(may be non-exhaustive)

- DisableAutoUpdates: the scripts didn't have a colon after `HKLM`/`HKCU`. Unclear how these scripts ever worked.
- WindowsOptimization: commented out a few lines, because they cause Access-Denied and fail silently otherwise anyway
  - `Get-ChildItem 'C:\*' -Recurse -Force -EA SilentlyContinue -Include 'OneDrive','OneDrive.*' | Remove-Item -Force -Recurse -EA SilentlyContinue`
  - `Get-ChildItem -Path c:\ -Include *.tmp, *.dmp, *.etl, *.evtx, thumbcache*.db, *.log -File -Recurse -Force -ErrorAction SilentlyContinue | Remove-Item -ErrorAction SilentlyContinue`
  - `Clear-BCCache -Force -ErrorAction SilentlyContinue`

# WDOT

Based on [WDOT github repo](https://github.com/The-Virtual-Desktop-Team/Windows-Desktop-Optimization-Tool).
Included the readme for future reference.

## Modifications

- Corrected the log messages for the registry edits (Optimize-WDOTLocalPolicySettings.ps1#L47). They wrongly stated errors for expected scenarios.
- Set it to Verbose to make troubleshooting easier.

## Configuration comments

  - GetStarted can't be removed, as it breaks snipping tool. Getstarted the AppxPackage is the Tips app, they have the same name, that is removed.
  - One reg item optimization is skipped since it returned an error. (HKLM:\\SOFTWARE\\Microsoft\\Windows\\ScriptedDiagnosticsProvider\\Policy)