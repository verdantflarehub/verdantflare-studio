Unicode true
RequestExecutionLevel user
ManifestDPIAware true
AllowSkipFiles off
SetCompressor /SOLID lzma

!include "MUI2.nsh"
!include "LogicLib.nsh"
!include "WinVer.nsh"
!include "x64.nsh"

!ifndef APP_VERSION
  !error "APP_VERSION is required"
!endif
!ifndef APP_BINARY
  !error "APP_BINARY is required"
!endif
!ifndef APP_ICON
  !error "APP_ICON is required"
!endif
!ifndef BUILD_INFO
  !error "BUILD_INFO is required"
!endif
!ifndef OUTPUT_FILE
  !error "OUTPUT_FILE is required"
!endif

!define PRODUCT "VerdantFlare Studio"
!define OWNER "VerdantFlare.Studio.Desktop.v1"
!define MARKER ".verdantflare-studio-install.ini"
!define UNINSTALL_KEY "Software\Microsoft\Windows\CurrentVersion\Uninstall\VerdantFlareStudio"
!define WEBVIEW_KEY "Software\Microsoft\EdgeUpdate\Clients\{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}"

Name "${PRODUCT} (unsigned development)"
OutFile "${OUTPUT_FILE}"
InstallDir "$LOCALAPPDATA\Programs\${PRODUCT}"
VIProductVersion "${APP_VERSION}.0"
VIFileVersion "${APP_VERSION}.0"
VIAddVersionKey "ProductName" "${PRODUCT}"
VIAddVersionKey "CompanyName" "VerdantFlare"
VIAddVersionKey "FileDescription" "Unsigned development acceptance installer"
VIAddVersionKey "FileVersion" "${APP_VERSION}"
VIAddVersionKey "LegalCopyright" "VerdantFlare contributors"

!define MUI_ICON "${APP_ICON}"
!define MUI_UNICON "${APP_ICON}"
!define MUI_ABORTWARNING
!insertmacro MUI_PAGE_WELCOME
; Fixed install directory: no directory-selection page or launch-on-finish action.
!insertmacro MUI_PAGE_INSTFILES
!insertmacro MUI_PAGE_FINISH
!insertmacro MUI_UNPAGE_CONFIRM
!insertmacro MUI_UNPAGE_INSTFILES
!insertmacro MUI_LANGUAGE "English"
ShowInstDetails show
ShowUninstDetails show

!macro RequireOrdinaryPath PATH
  System::Call 'kernel32::GetFileAttributesW(w "${PATH}") i .r0'
  ${If} $0 != -1
    IntOp $0 $0 & 0x400
    ${If} $0 != 0
      MessageBox MB_OK|MB_ICONSTOP "The dedicated install path contains a reparse point. No files were changed." /SD IDOK
      SetErrorLevel 2
      Abort
    ${EndIf}
  ${EndIf}
!macroend

!macro RequireSafeInstallPath
  !insertmacro RequireOrdinaryPath "$LOCALAPPDATA\Programs"
  !insertmacro RequireOrdinaryPath "$INSTDIR"
  !insertmacro RequireOrdinaryPath "$INSTDIR\${MARKER}"
  !insertmacro RequireOrdinaryPath "$INSTDIR\studio-desktop.exe"
  !insertmacro RequireOrdinaryPath "$INSTDIR\build-info.json"
  !insertmacro RequireOrdinaryPath "$INSTDIR\uninstall.exe"
!macroend

Function .onInit
  SetShellVarContext current
  StrCpy $INSTDIR "$LOCALAPPDATA\Programs\${PRODUCT}"
  ${IfNot} ${AtLeastWin10}
  ${OrIfNot} ${IsNativeAMD64}
    MessageBox MB_OK|MB_ICONSTOP "This build requires Windows 10 or later on amd64." /SD IDOK
    SetErrorLevel 2
    Abort
  ${EndIf}
  !insertmacro RequireSafeInstallPath
  System::Call 'kernel32::GetFileAttributesW(w "$INSTDIR") i .r0'
  ${If} $0 != -1
    ReadINIStr $0 "$INSTDIR\${MARKER}" "Install" "Product"
    ${If} $0 != "${OWNER}"
      MessageBox MB_OK|MB_ICONSTOP "The dedicated install directory exists without this product's ownership marker. Review its contents before installing." /SD IDOK
      SetErrorLevel 2
      Abort
    ${EndIf}
  ${EndIf}
  ; Mirror the actual Wails beta.22 registry discovery, without installing a runtime.
  SetRegView 32
  ReadRegStr $0 HKLM "${WEBVIEW_KEY}" "pv"
  SetRegView 64
  ${If} $0 == ""
  ${OrIf} $0 == "0.0.0.0"
    ReadRegStr $0 HKCU "${WEBVIEW_KEY}" "pv"
  ${EndIf}
  ${If} $0 == ""
  ${OrIf} $0 == "0.0.0.0"
    MessageBox MB_OK|MB_ICONSTOP "Microsoft Edge WebView2 Evergreen Runtime is required. Install it from https://developer.microsoft.com/microsoft-edge/webview2/ and run this installer again." /SD IDOK
    SetErrorLevel 2
    Abort
  ${EndIf}
FunctionEnd

Section "Install"
  ; Reassert the fixed path even if /D was supplied to the installer.
  StrCpy $INSTDIR "$LOCALAPPDATA\Programs\${PRODUCT}"
  !insertmacro RequireSafeInstallPath
  SetOutPath "$INSTDIR"
  SetOverwrite on
  File /oname=studio-desktop.exe "${APP_BINARY}"
  File /oname=build-info.json "${BUILD_INFO}"
  WriteUninstaller "$INSTDIR\uninstall.exe"
  WriteINIStr "$INSTDIR\${MARKER}" "Install" "Product" "${OWNER}"
  CreateShortCut "$SMPROGRAMS\${PRODUCT}.lnk" "$INSTDIR\studio-desktop.exe"
  WriteRegStr HKCU "${UNINSTALL_KEY}" "DisplayName" "${PRODUCT} (unsigned development)"
  WriteRegStr HKCU "${UNINSTALL_KEY}" "DisplayVersion" "${APP_VERSION}"
  WriteRegStr HKCU "${UNINSTALL_KEY}" "Publisher" "VerdantFlare"
  WriteRegStr HKCU "${UNINSTALL_KEY}" "InstallLocation" "$INSTDIR"
  WriteRegStr HKCU "${UNINSTALL_KEY}" "DisplayIcon" "$INSTDIR\studio-desktop.exe"
  WriteRegStr HKCU "${UNINSTALL_KEY}" "UninstallString" '$\"$INSTDIR\uninstall.exe$\"'
  WriteRegStr HKCU "${UNINSTALL_KEY}" "QuietUninstallString" '$\"$INSTDIR\uninstall.exe$\" /S'
  WriteRegDWORD HKCU "${UNINSTALL_KEY}" "NoModify" 1
  WriteRegDWORD HKCU "${UNINSTALL_KEY}" "NoRepair" 1
SectionEnd

Function un.onInit
  SetShellVarContext current
  SetRegView 64
  ${If} $INSTDIR != "$LOCALAPPDATA\Programs\${PRODUCT}"
    MessageBox MB_OK|MB_ICONSTOP "Run the uninstaller from the dedicated product directory." /SD IDOK
    SetErrorLevel 2
    Abort
  ${EndIf}
  !insertmacro RequireSafeInstallPath
  ReadINIStr $0 "$INSTDIR\${MARKER}" "Install" "Product"
  ${If} $0 != "${OWNER}"
    MessageBox MB_OK|MB_ICONSTOP "The product ownership marker is missing. No files were removed." /SD IDOK
    SetErrorLevel 2
    Abort
  ${EndIf}
FunctionEnd

Section "Uninstall"
  !insertmacro RequireSafeInstallPath
  ClearErrors
  Delete "$INSTDIR\studio-desktop.exe"
  ${If} ${Errors}
    MessageBox MB_OK|MB_ICONSTOP "Close Studio and run the uninstaller again." /SD IDOK
    SetErrorLevel 2
    Abort
  ${EndIf}
  Delete "$INSTDIR\build-info.json"
  Delete "$INSTDIR\uninstall.exe"
  Delete "$INSTDIR\${MARKER}"
  Delete "$SMPROGRAMS\${PRODUCT}.lnk"
  DeleteRegKey HKCU "${UNINSTALL_KEY}"
  ; No recursive removal. Unknown files, work copies and WebView data are preserved.
  RMDir "$INSTDIR"
SectionEnd
