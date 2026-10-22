; Inno Setup script for the Stage Wand Windows host (built by .github/workflows/release.yml).
#ifndef AppVersion
  #define AppVersion "0.0.0"
#endif
[Setup]
AppId={{6B0E3B8E-5C1D-4C5A-9F3E-57A6E2D0B1A1}
AppName=Stage Wand
AppVersion={#AppVersion}
AppPublisher=Edmund Lim
AppPublisherURL=https://github.com/EdmundLimBoEn/stage-wand
DefaultDirName={autopf}\Stage Wand
DefaultGroupName=Stage Wand
DisableProgramGroupPage=yes
PrivilegesRequiredOverridesAllowed=dialog
ArchitecturesAllowed=x64compatible
ArchitecturesInstallIn64BitMode=x64compatible
OutputDir={#OutDir}
Compression=lzma2
SolidCompression=yes
UninstallDisplayIcon={app}\stagewand-host.exe

[Files]
Source: "{#SrcDir}\*"; DestDir: "{app}"; Flags: ignoreversion

[Icons]
Name: "{group}\Stage Wand Host"; Filename: "{app}\stagewand-host.exe"
Name: "{group}\Uninstall Stage Wand"; Filename: "{uninstallexe}"

[Run]
Filename: "{app}\stagewand-host.exe"; Description: "Start Stage Wand host"; Flags: nowait postinstall skipifsilent
