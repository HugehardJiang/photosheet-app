; NSIS installer skeleton for the signed release artifact.
Unicode True
Name "PhotoSheet"
OutFile "PhotoSheet-Setup.exe"
InstallDir "$PROGRAMFILES64\PhotoSheet"
RequestExecutionLevel admin

Section "PhotoSheet"
  SetOutPath "$INSTDIR"
  File "PhotoSheet.exe"
  File "photosheet-updater.exe"
  CreateShortCut "$DESKTOP\PhotoSheet.lnk" "$INSTDIR\PhotoSheet.exe"
  CreateDirectory "$SMPROGRAMS\PhotoSheet"
  CreateShortCut "$SMPROGRAMS\PhotoSheet\PhotoSheet.lnk" "$INSTDIR\PhotoSheet.exe"
  WriteUninstaller "$INSTDIR\uninstall.exe"
SectionEnd
Section "Uninstall"
  Delete "$DESKTOP\PhotoSheet.lnk"
  Delete "$SMPROGRAMS\PhotoSheet\PhotoSheet.lnk"
  RMDir "$SMPROGRAMS\PhotoSheet"
  Delete "$INSTDIR\PhotoSheet.exe"
  Delete "$INSTDIR\photosheet-updater.exe"
  Delete "$INSTDIR\uninstall.exe"
  RMDir "$INSTDIR"
  MessageBox MB_YESNO "是否删除 PhotoSheet 缓存？用户 project 不会被删除。" IDNO done
  RMDir /r "$LOCALAPPDATA\PhotoSheet\Cache"
  done:
SectionEnd
