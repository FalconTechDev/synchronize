Unicode true

!define APPNAME      "synchronize"
!define SHORTNAME    "syn"
!define PUBLISHER    "FalconTechDev"
!define DESCRIPTION  "Automatic git sync across your machines"
!define HELPURL      "https://github.com/FalconTechDev/synchronize"

!ifndef APPVERSION
  !define APPVERSION "1.0.0"
!endif

!include "MUI2.nsh"
!include "LogicLib.nsh"
!include "FileFunc.nsh"
!include "Sections.nsh"
!include "StrFunc.nsh"

${StrStr}
${UnStrRep}

Name "${APPNAME} ${APPVERSION}"
OutFile "..\dist\syn-setup.exe"
InstallDir "$LOCALAPPDATA\Programs\${SHORTNAME}"
InstallDirRegKey HKCU "Software\${APPNAME}" "InstallDir"
RequestExecutionLevel user
SetCompressor /SOLID lzma
ShowInstDetails show
ShowUnInstDetails show

VIProductVersion "${APPVERSION}.0"
VIAddVersionKey "ProductName"     "${APPNAME}"
VIAddVersionKey "FileDescription" "${APPNAME} installer"
VIAddVersionKey "FileVersion"     "${APPVERSION}"
VIAddVersionKey "ProductVersion"  "${APPVERSION}"
VIAddVersionKey "CompanyName"     "${PUBLISHER}"
VIAddVersionKey "LegalCopyright"  "MIT License"

; ---------------------------------------------------------------- appearance
!define MUI_ICON   "syn.ico"
!define MUI_UNICON "syn.ico"

!define MUI_HEADERIMAGE
!define MUI_HEADERIMAGE_BITMAP        "header.bmp"
!define MUI_HEADERIMAGE_UNBITMAP      "header.bmp"
!define MUI_HEADERIMAGE_RIGHT

!define MUI_WELCOMEFINISHPAGE_BITMAP   "welcome.bmp"
!define MUI_UNWELCOMEFINISHPAGE_BITMAP "welcome.bmp"

!define MUI_ABORTWARNING
!define MUI_COMPONENTSPAGE_SMALLDESC

!define MUI_WELCOMEPAGE_TITLE "Install ${APPNAME} ${APPVERSION}"
!define MUI_WELCOMEPAGE_TEXT  "${DESCRIPTION}.$\r$\n$\r$\nsyn keeps your repositories in step across every machine you work on: it pulls when you sit down and pushes when you walk away.$\r$\n$\r$\nThis installer places the syn command on your system. It needs no administrator rights and installs only for you.$\r$\n$\r$\nClick Next to continue."

!define MUI_FINISHPAGE_TITLE "${APPNAME} is installed"
!define MUI_FINISHPAGE_TEXT  "Open a new terminal and run syn to get started:$\r$\n$\r$\n    syn auth login --pat <token>$\r$\n    syn repo add owner/repo$\r$\n    syn watch$\r$\n$\r$\nIf you did not add syn to PATH, run it from:$\r$\n$INSTDIR"
!define MUI_FINISHPAGE_LINK      "Open the documentation on GitHub"
!define MUI_FINISHPAGE_LINK_LOCATION "${HELPURL}"
!define MUI_FINISHPAGE_NOAUTOCLOSE
!define MUI_UNFINISHPAGE_NOAUTOCLOSE

; ---------------------------------------------------------------- pages
!insertmacro MUI_PAGE_WELCOME
!insertmacro MUI_PAGE_COMPONENTS
!insertmacro MUI_PAGE_DIRECTORY
!insertmacro MUI_PAGE_INSTFILES
!insertmacro MUI_PAGE_FINISH

!insertmacro MUI_UNPAGE_CONFIRM
!insertmacro MUI_UNPAGE_INSTFILES
!insertmacro MUI_UNPAGE_FINISH

!insertmacro MUI_LANGUAGE "English"

; ---------------------------------------------------------------- sections
Var AddedToPath
Var GitFound
Var GitVer

; syn drives the real git, so git must be present.
Section "Git for Windows (required by syn)" SecGit
  ${If} $GitFound == "1"
    DetailPrint "Git already installed ($GitVer) - skipping."
    Return
  ${EndIf}

  DetailPrint "Git was not found on this machine."

  ; --- preferred path: winget (handles download, verify and PATH) ---
  nsExec::ExecToStack 'cmd /c where winget'
  Pop $0
  ${If} $0 == 0
    DetailPrint "Installing Git via winget - this may take a few minutes..."
    nsExec::ExecToLog 'cmd /c winget install --id Git.Git -e --source winget \
      --accept-package-agreements --accept-source-agreements --silent'
    Pop $0
    ${If} $0 == 0
      DetailPrint "Git installed."
      Goto git_done
    ${EndIf}
    DetailPrint "winget failed (code $0) - falling back to direct download."
  ${EndIf}

  ; --- fallback: fetch the latest Git for Windows release from GitHub ---
  DetailPrint "Downloading the latest Git for Windows..."
  StrCpy $1 "$TEMP\git-installer.exe"

  nsExec::ExecToLog 'powershell -NoProfile -ExecutionPolicy Bypass -Command \
    "[Net.ServicePointManager]::SecurityProtocol=[Net.SecurityProtocolType]::Tls12; \
     $$r = Invoke-RestMethod https://api.github.com/repos/git-for-windows/git/releases/latest; \
     $$a = $$r.assets | Where-Object { $$_.name -like \"*64-bit.exe\" } | Select-Object -First 1; \
     if (-not $$a) { exit 1 }; \
     Write-Host $$a.name; \
     Invoke-WebRequest $$a.browser_download_url -OutFile \"$1\" -UseBasicParsing"'
  Pop $0

  ${If} $0 != 0
    MessageBox MB_ICONEXCLAMATION|MB_OK \
      "Could not download Git automatically.$\r$\n$\r$\nsynchronize will still be installed, but it will not work until you install Git yourself:$\r$\n$\r$\n    https://git-scm.com/download/win"
    Goto git_done
  ${EndIf}

  DetailPrint "Running the Git installer..."
  ExecWait '"$1" /VERYSILENT /NORESTART /NOCANCEL /SP- /COMPONENTS="icons,ext\reg\shellhere,assoc,assoc_sh"' $0
  Delete "$1"
  ${If} $0 == 0
    DetailPrint "Git installed."
  ${Else}
    DetailPrint "Git installer returned code $0."
  ${EndIf}

  git_done:
SectionEnd


Section "!${APPNAME} (required)" SecCore
  SectionIn RO
  SetOutPath "$INSTDIR"

  DetailPrint "Closing any running syn processes..."
  nsExec::Exec 'taskkill /F /IM syn.exe'
  Pop $0

  File "..\bin\syn.exe"

  CreateDirectory "$PROFILE\.synchronize"
  SetOutPath "$PROFILE\.synchronize"
  File "/oname=logo.png" "..\internal\syn\assets\tray.png"
  SetOutPath "$INSTDIR"

  WriteRegStr HKCU "Software\${APPNAME}" "InstallDir" "$INSTDIR"
  WriteRegStr HKCU "Software\${APPNAME}" "Version"    "${APPVERSION}"
  WriteUninstaller "$INSTDIR\uninstall.exe"

  ; Add/Remove Programs entry
  !define UNKEY "Software\Microsoft\Windows\CurrentVersion\Uninstall\${APPNAME}"
  WriteRegStr   HKCU "${UNKEY}" "DisplayName"     "${APPNAME}"
  WriteRegStr   HKCU "${UNKEY}" "DisplayVersion"  "${APPVERSION}"
  WriteRegStr   HKCU "${UNKEY}" "DisplayIcon"     "$INSTDIR\syn.exe"
  WriteRegStr   HKCU "${UNKEY}" "Publisher"       "${PUBLISHER}"
  WriteRegStr   HKCU "${UNKEY}" "URLInfoAbout"    "${HELPURL}"
  WriteRegStr   HKCU "${UNKEY}" "UninstallString" '"$INSTDIR\uninstall.exe"'
  WriteRegStr   HKCU "${UNKEY}" "InstallLocation" "$INSTDIR"
  WriteRegDWORD HKCU "${UNKEY}" "NoModify" 1
  WriteRegDWORD HKCU "${UNKEY}" "NoRepair" 1

  ${GetSize} "$INSTDIR" "/S=0K" $0 $1 $2
  IntFmt $0 "0x%08X" $0
  WriteRegDWORD HKCU "${UNKEY}" "EstimatedSize" "$0"
SectionEnd

Section "Add syn to PATH" SecPath
  DetailPrint "Adding $INSTDIR to your PATH..."
  ReadRegStr $0 HKCU "Environment" "PATH"

  ${If} $0 == ""
    StrCpy $1 "$INSTDIR"
  ${Else}
    ${StrStr} $2 "$0" "$INSTDIR"
    ${If} $2 != ""
      DetailPrint "Already on PATH - nothing to do."
      StrCpy $AddedToPath "1"
      Goto done
    ${EndIf}
    StrCpy $1 "$0;$INSTDIR"
  ${EndIf}

  WriteRegExpandStr HKCU "Environment" "PATH" "$1"
  SendMessage ${HWND_BROADCAST} ${WM_WININICHANGE} 0 "STR:Environment" /TIMEOUT=5000
  StrCpy $AddedToPath "1"
  DetailPrint "Added. Open a new terminal to use syn."
  done:
SectionEnd

Section "Start Menu shortcut" SecShortcut
  CreateDirectory "$SMPROGRAMS\${APPNAME}"
  CreateShortcut "$SMPROGRAMS\${APPNAME}\${APPNAME} dashboard.lnk" \
    "$INSTDIR\syn.exe" "serve" "$INSTDIR\syn.exe" 0
  CreateShortcut "$SMPROGRAMS\${APPNAME}\Uninstall ${APPNAME}.lnk" \
    "$INSTDIR\uninstall.exe"
SectionEnd

Section /o "Sync in the background" SecAutostart
  DetailPrint "Registering the startup and shutdown tasks..."
  nsExec::ExecToLog '"$INSTDIR\syn.exe" auto on --quiet'
  Pop $0
  ${If} $0 == 0
    DetailPrint "syn will sync when you log in and publish before shutdown."
  ${Else}
    DetailPrint "Could not register the tasks yet (code $0)."
    DetailPrint "Add a repository first, then run:  syn autostart on"
  ${EndIf}
SectionEnd

; ---------------------------------------------------------------- init
Function .onInit
  StrCpy $GitFound "0"

  ; on PATH?
  nsExec::ExecToStack 'cmd /c git --version'
  Pop $0
  Pop $1
  ${If} $0 == 0
    StrCpy $GitFound "1"
    StrCpy $GitVer $1 20
    Goto checked
  ${EndIf}

  ; installed but not on PATH? (common after a fresh install without reopening the shell)
  ${If} ${FileExists} "$PROGRAMFILES64\Git\cmd\git.exe"
  ${OrIf} ${FileExists} "$PROGRAMFILES32\Git\cmd\git.exe"
  ${OrIf} ${FileExists} "$LOCALAPPDATA\Programs\Git\cmd\git.exe"
    StrCpy $GitFound "1"
    StrCpy $GitVer "already installed"
  ${EndIf}

  checked:
  ${If} $GitFound == "1"
    ; nothing to do - show it ticked, greyed out, and skip at install time
    SectionSetText   ${SecGit} "Git for Windows (already installed)"
    SectionSetFlags  ${SecGit} ${SF_SELECTED}|${SF_RO}
  ${Else}
    ; required - force it on and stop the user unticking it
    SectionSetFlags  ${SecGit} ${SF_SELECTED}|${SF_RO}
  ${EndIf}
FunctionEnd

!insertmacro MUI_FUNCTION_DESCRIPTION_BEGIN
  !insertmacro MUI_DESCRIPTION_TEXT ${SecGit}       "syn drives the real git. If git is missing it will be downloaded and installed for you."
  !insertmacro MUI_DESCRIPTION_TEXT ${SecCore}      "The syn command-line tool. Required."
  !insertmacro MUI_DESCRIPTION_TEXT ${SecPath}      "Let you type 'syn' in any terminal. Recommended."
  !insertmacro MUI_DESCRIPTION_TEXT ${SecShortcut}  "Add ${APPNAME} to the Start Menu."
  !insertmacro MUI_DESCRIPTION_TEXT ${SecAutostart} "Sync when you log in, and publish your work before the machine shuts down. You can change this later with: syn auto on|off"
!insertmacro MUI_FUNCTION_DESCRIPTION_END

; ---------------------------------------------------------------- uninstall
Section "Uninstall"
  nsExec::Exec '"$INSTDIR\syn.exe" auto off --quiet'
  Pop $0
  nsExec::Exec 'taskkill /F /IM syn.exe'
  Pop $0

  Delete "$INSTDIR\syn.exe"
  Delete "$INSTDIR\uninstall.exe"
  RMDir  "$INSTDIR"

  Delete "$SMPROGRAMS\${APPNAME}\${APPNAME} dashboard.lnk"
  Delete "$SMPROGRAMS\${APPNAME}\Uninstall ${APPNAME}.lnk"
  RMDir  "$SMPROGRAMS\${APPNAME}"
  Delete "$SMSTARTUP\${APPNAME}.lnk"

  ; strip from PATH
  ReadRegStr $0 HKCU "Environment" "PATH"
  ${If} $0 != ""
    ${UnStrRep} $1 "$0" ";$INSTDIR" ""
    ${UnStrRep} $1 "$1" "$INSTDIR;" ""
    ${UnStrRep} $1 "$1" "$INSTDIR"  ""
    WriteRegExpandStr HKCU "Environment" "PATH" "$1"
    SendMessage ${HWND_BROADCAST} ${WM_WININICHANGE} 0 "STR:Environment" /TIMEOUT=5000
  ${EndIf}

  DeleteRegKey HKCU "Software\${APPNAME}"
  DeleteRegKey HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\${APPNAME}"

  DetailPrint "Your settings in $PROFILE\.synchronize were kept."
SectionEnd
