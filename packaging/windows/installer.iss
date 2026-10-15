#ifndef AppVersion
  #define AppVersion "0.1.0"
#endif
#ifndef AppIdentifier
  #define AppIdentifier "{{C37CD8EA-90C5-4BCB-9F87-75123FB7E890}"
#endif

[Setup]
AppId={#AppIdentifier}
AppName=SC Player
AppVersion={#AppVersion}
AppPublisher=SC Player
DefaultDirName={localappdata}\Programs\SC Player
DefaultGroupName=SC Player
PrivilegesRequired=lowest
ArchitecturesAllowed=x64compatible
ArchitecturesInstallIn64BitMode=x64compatible
OutputDir=..\..\dist
OutputBaseFilename=SC-Player-{#AppVersion}-windows-amd64-setup
Compression=lzma2
SolidCompression=yes
WizardStyle=modern
SetupIconFile=sc-player.ico
UninstallDisplayIcon={app}\sc-player.exe
CloseApplications=yes

[Languages]
Name: "english"; MessagesFile: "compiler:Default.isl"
Name: "russian"; MessagesFile: "compiler:Languages\Russian.isl"

[Tasks]
Name: "desktopicon"; Description: "Create a desktop shortcut"; Flags: unchecked

[Files]
Source: "..\..\dist\windows-amd64\sc-player.exe"; DestDir: "{app}"; Flags: ignoreversion
Source: "..\..\dist\windows-amd64\README.md"; DestDir: "{app}"; Flags: ignoreversion
Source: "..\..\dist\windows-amd64\sc-player.example.json"; DestDir: "{app}"; Flags: ignoreversion
Source: "..\..\dist\windows-amd64\sc-player.json"; DestDir: "{app}"; Flags: onlyifdoesntexist skipifsourcedoesntexist uninsneveruninstall
Source: "..\..\dist\windows-amd64\bin\*"; DestDir: "{app}\bin"; Flags: ignoreversion recursesubdirs createallsubdirs

[Icons]
Name: "{group}\SC Player"; Filename: "{app}\sc-player.exe"
Name: "{group}\Uninstall SC Player"; Filename: "{uninstallexe}"
Name: "{autodesktop}\SC Player"; Filename: "{app}\sc-player.exe"; Tasks: desktopicon

[Run]
Filename: "{app}\sc-player.exe"; Description: "Launch SC Player"; Flags: nowait postinstall skipifsilent
