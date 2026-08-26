; zwan Windows installer.
;
; Installs the desktop app (zwan.exe: window + tray) plus either or both roles:
;
;   Client  the SYSTEM engine service (zwan-service.exe) and the Wintun
;           virtual-WAN driver, for joining networks.
;   Server  the SYSTEM control-server service (zwan-server.exe), for hosting a
;           network. Selecting only this gives a server box with no tunnel
;           driver installed.
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
!include "Sections.nsh"
!include "LogicLib.nsh"
!define MUI_ABORTWARNING
!insertmacro MUI_PAGE_WELCOME
!insertmacro MUI_PAGE_COMPONENTS
!insertmacro MUI_PAGE_DIRECTORY
!insertmacro MUI_PAGE_INSTFILES
!define MUI_FINISHPAGE_RUN "$INSTDIR\zwan.exe"
!define MUI_FINISHPAGE_RUN_TEXT "Launch zwan"
!insertmacro MUI_PAGE_FINISH
!insertmacro MUI_UNPAGE_CONFIRM
!insertmacro MUI_UNPAGE_INSTFILES
!insertmacro MUI_LANGUAGE "English"


; The app itself is always installed: it is the UI for both roles.
Section "-Core" SEC_CORE
  SetOutPath "$INSTDIR"

  ; The app holds its own binary open - and on the update path it is the very
  ; thing that launched this installer. No /T: the installer is its child.
  nsExec::ExecToLog 'taskkill /F /IM zwan.exe'

  File "stage\zwan.exe"

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

Section "Client (join networks)" SEC_CLIENT
  SetOutPath "$INSTDIR"

  ; A running service holds its own binary open, so Windows refuses to write the
  ; new one over it and the upgrade silently keeps the old code. Nothing to stop
  ; on a first install, and the failure is only logged.
  DetailPrint "Stopping the engine service if it is running..."
  nsExec::ExecToLog '"$INSTDIR\zwan-service.exe" stop'

  File "stage\zwan-service.exe"
  File "stage\wintun.dll"

  DetailPrint "Installing virtual WAN driver (Wintun)..."
  nsExec::ExecToLog '"$INSTDIR\zwan-service.exe" driver-install'

  ; install creates the service on a first run and starts the existing one on an
  ; upgrade, so this is also what brings it back up after the stop above.
  DetailPrint "Installing engine service..."
  nsExec::ExecToLog '"$INSTDIR\zwan-service.exe" install'
SectionEnd

Section /o "Server (host a network)" SEC_SERVER
  SetOutPath "$INSTDIR"

  DetailPrint "Stopping the control-server service if it is running..."
  nsExec::ExecToLog '"$INSTDIR\zwan-server.exe" service stop'

  File "stage\zwan-server.exe"

  DetailPrint "Installing control-server service..."
  nsExec::ExecToLog '"$INSTDIR\zwan-server.exe" service install'
SectionEnd

!insertmacro MUI_FUNCTION_DESCRIPTION_BEGIN
  !insertmacro MUI_DESCRIPTION_TEXT ${SEC_CLIENT} "Join networks from this device: the background engine service and the Wintun virtual network adapter."
  !insertmacro MUI_DESCRIPTION_TEXT ${SEC_SERVER} "Host a network on this machine: the background control-server service (control API, DNS/service registry and relay)."
!insertmacro MUI_FUNCTION_DESCRIPTION_END

; This has to sit below both sections: a section's ${SEC_...} constant only
; exists from the line the section is written on, and referring to it earlier
; compiles with a warning and then does nothing at all.
; An upgrade must install the same roles the machine already has.
;
; The component page decides that when someone is watching, but an automatic
; update runs this silently, and a silent run takes the default selection -
; where Server is off. Without this, updating a hosting machine would quietly
; leave its server on the old version while reporting success, which is exactly
; what happened to 0.3.1.
Function .onInit
  ReadRegStr $0 HKLM "${UNINSTKEY}" "InstallLocation"
  ${If} $0 != ""
    StrCpy $INSTDIR $0
    ${If} ${FileExists} "$INSTDIR\zwan-service.exe"
      !insertmacro SelectSection ${SEC_CLIENT}
    ${Else}
      !insertmacro UnselectSection ${SEC_CLIENT}
    ${EndIf}
    ${If} ${FileExists} "$INSTDIR\zwan-server.exe"
      !insertmacro SelectSection ${SEC_SERVER}
    ${Else}
      !insertmacro UnselectSection ${SEC_SERVER}
    ${EndIf}
  ${EndIf}
FunctionEnd

Section "Uninstall"
  ; Both services may or may not be present; failures here are expected.
  nsExec::ExecToLog '"$INSTDIR\zwan-server.exe" service stop'
  nsExec::ExecToLog '"$INSTDIR\zwan-server.exe" service uninstall'
  nsExec::ExecToLog '"$INSTDIR\zwan-service.exe" stop'
  nsExec::ExecToLog '"$INSTDIR\zwan-service.exe" uninstall'

  Delete "$SMSTARTUP\zwan.lnk"
  Delete "$SMPROGRAMS\${PRODUCT}\zwan.lnk"
  RMDir "$SMPROGRAMS\${PRODUCT}"

  Delete "$INSTDIR\zwan.exe"
  Delete "$INSTDIR\zwan-service.exe"
  Delete "$INSTDIR\zwan-server.exe"
  Delete "$INSTDIR\wintun.dll"
  Delete "$INSTDIR\uninstall.exe"
  RMDir "$INSTDIR"

  DeleteRegKey HKLM "${UNINSTKEY}"
SectionEnd
