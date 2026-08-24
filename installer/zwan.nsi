; zwan Windows installer.
;
; Installs the client GUI (zwan.exe), the SYSTEM engine service (zwan-service.exe)
; and the Wintun virtual-WAN driver (wintun.dll), installs the driver and the
; service at setup time, and starts the tray app at login.
;
; Build:  makensis -DVERSION=0.1.0 installer\zwan.nsi   ->  dist\zwan-setup.exe

!define PRODUCT "zwan"
!define COMPANY "zwan"
!ifndef VERSION
  !define VERSION "0.0.0"
!endif
!define UNINSTKEY "Software\Microsoft\Windows\CurrentVersion\Uninstall\${PRODUCT}"

Name "${PRODUCT} ${VERSION}"
OutFile "..\dist\zwan-setup.exe"
Unicode true
InstallDir "$PROGRAMFILES64\${PRODUCT}"
RequestExecutionLevel admin
ShowInstDetails show
ShowUninstDetails show

!include "MUI2.nsh"
!define MUI_ABORTWARNING
!insertmacro MUI_PAGE_WELCOME
!insertmacro MUI_PAGE_DIRECTORY
!insertmacro MUI_PAGE_INSTFILES
!define MUI_FINISHPAGE_RUN "$INSTDIR\zwan.exe"
!define MUI_FINISHPAGE_RUN_TEXT "Launch zwan"
!insertmacro MUI_PAGE_FINISH
!insertmacro MUI_UNPAGE_CONFIRM
!insertmacro MUI_UNPAGE_INSTFILES
!insertmacro MUI_LANGUAGE "English"

Section "Install"
  SetOutPath "$INSTDIR"
  File "stage\zwan.exe"
  File "stage\zwan-service.exe"
  File "stage\wintun.dll"

  DetailPrint "Installing virtual WAN driver (Wintun)..."
  nsExec::ExecToLog '"$INSTDIR\zwan-service.exe" driver-install'

  DetailPrint "Installing engine service..."
  nsExec::ExecToLog '"$INSTDIR\zwan-service.exe" install'

  CreateDirectory "$SMPROGRAMS\${PRODUCT}"
  CreateShortcut "$SMPROGRAMS\${PRODUCT}\zwan.lnk" "$INSTDIR\zwan.exe"
  CreateShortcut "$SMSTARTUP\zwan.lnk" "$INSTDIR\zwan.exe"

  WriteUninstaller "$INSTDIR\uninstall.exe"
  WriteRegStr HKLM "${UNINSTKEY}" "DisplayName" "${PRODUCT}"
  WriteRegStr HKLM "${UNINSTKEY}" "DisplayVersion" "${VERSION}"
  WriteRegStr HKLM "${UNINSTKEY}" "Publisher" "${COMPANY}"
  WriteRegStr HKLM "${UNINSTKEY}" "InstallLocation" "$INSTDIR"
  WriteRegStr HKLM "${UNINSTKEY}" "UninstallString" '"$INSTDIR\uninstall.exe"'
  WriteRegDWORD HKLM "${UNINSTKEY}" "NoModify" 1
  WriteRegDWORD HKLM "${UNINSTKEY}" "NoRepair" 1
SectionEnd

Section "Uninstall"
  nsExec::ExecToLog '"$INSTDIR\zwan-service.exe" stop'
  nsExec::ExecToLog '"$INSTDIR\zwan-service.exe" uninstall'

  Delete "$SMSTARTUP\zwan.lnk"
  Delete "$SMPROGRAMS\${PRODUCT}\zwan.lnk"
  RMDir "$SMPROGRAMS\${PRODUCT}"

  Delete "$INSTDIR\zwan.exe"
  Delete "$INSTDIR\zwan-service.exe"
  Delete "$INSTDIR\wintun.dll"
  Delete "$INSTDIR\uninstall.exe"
  RMDir "$INSTDIR"

  DeleteRegKey HKLM "${UNINSTKEY}"
SectionEnd
