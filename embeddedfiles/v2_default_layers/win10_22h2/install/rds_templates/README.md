# RDS Templates

Based on: https://github.com/Azure/RDS-Templates/blob/master/CustomImageTemplateScripts/CustomImageTemplateScripts_2024-03-27

## Modifications

(may be non-exhaustive)

- DisableAutoUpdates: the scripts didn't have a colon after `HKLM`/`HKCU`. Unclear how these scripts ever worked.
- WindowsOptimization: commented out a few lines, because they cause Access-Denied and fail silently otherwise anyway
  - `Get-ChildItem 'C:\*' -Recurse -Force -EA SilentlyContinue -Include 'OneDrive','OneDrive.*' | Remove-Item -Force -Recurse -EA SilentlyContinue`
  - `Get-ChildItem -Path c:\ -Include *.tmp, *.dmp, *.etl, *.evtx, thumbcache*.db, *.log -File -Recurse -Force -ErrorAction SilentlyContinue | Remove-Item -ErrorAction SilentlyContinue`
  - `Clear-BCCache -Force -ErrorAction SilentlyContinue`