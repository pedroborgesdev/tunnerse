#define MyAppName "Tunnerse"
#ifndef MyAppVersion
#define MyAppVersion "dev"
#endif
#ifndef MyInstallerArch
#define MyInstallerArch "amd64"
#endif
#ifndef MyTargetIs64
#define MyTargetIs64 1
#endif
#define MyAppPublisher "Tunnerse"
#define MyAppExeName "tunnerse.exe"
#define MyServerExeName "tunnerse-daemon.exe"
#define MyServiceName "TunnerseDaemon"
#define MyLegacyServiceName "TunnerseServer"

[Setup]
AppId={{B5A96D8A-7156-4D3F-9F8D-2AB2C54D05F0}
AppName={#MyAppName}
AppVersion={#MyAppVersion}
AppPublisher={#MyAppPublisher}
DefaultDirName={autopf}\Tunnerse
DefaultGroupName=Tunnerse
DisableProgramGroupPage=yes
OutputDir=..\..\..\dist
OutputBaseFilename=tunnerse-installer_{#MyInstallerArch}_{#MyAppVersion}
SetupIconFile=..\..\..\assets\icons\win\tunnerse.ico
WizardImageFile=..\..\..\assets\installer\download-conclued.bmp
WizardSmallImageFile=..\..\..\assets\installer\download-icon.bmp
Compression=lzma2
SolidCompression=yes
WizardStyle=modern
PrivilegesRequired=admin
#if MyTargetIs64
ArchitecturesAllowed=x64compatible
ArchitecturesInstallIn64BitMode=x64compatible
#else
ArchitecturesAllowed=x86compatible
#endif
ChangesEnvironment=yes
UninstallDisplayIcon={app}\{#MyAppExeName}
UninstallFilesDir={app}\inno

[Languages]
Name: "english"; MessagesFile: "compiler:Default.isl"

[Files]
#if MyTargetIs64
Source: "..\..\..\dist\installer-payload\tunnerse-amd64.exe"; DestDir: "{app}"; DestName: "tunnerse.exe"; Flags: ignoreversion
Source: "..\..\..\dist\installer-payload\tunnerse-daemon-amd64.exe"; DestDir: "{app}"; DestName: "tunnerse-daemon.exe"; Flags: ignoreversion
#else
Source: "..\..\..\dist\installer-payload\tunnerse-x86.exe"; DestDir: "{app}"; DestName: "tunnerse.exe"; Flags: ignoreversion
Source: "..\..\..\dist\installer-payload\tunnerse-daemon-x86.exe"; DestDir: "{app}"; DestName: "tunnerse-daemon.exe"; Flags: ignoreversion
#endif

[Dirs]
Name: "{commonappdata}\Tunnerse"
Name: "{commonappdata}\Tunnerse\logs"
Name: "{app}\inno"

[Icons]
Name: "{group}\Tunnerse CLI"; Filename: "{app}\{#MyAppExeName}"
Name: "{group}\Tunnerse Service Manager"; Filename: "{sys}\services.msc"
Name: "{group}\Uninstall Tunnerse"; Filename: "{uninstallexe}"

[Registry]
Root: HKLM; Subkey: "SYSTEM\CurrentControlSet\Services\{#MyServiceName}"; ValueType: string; ValueName: "AppDirectory"; ValueData: "{app}"; Flags: uninsdeletevalue
Root: HKLM; Subkey: "SYSTEM\CurrentControlSet\Control\Session Manager\Environment"; ValueType: string; ValueName: "TUNNERSE_DATA_DIR"; ValueData: "{commonappdata}\Tunnerse"; Flags: preservestringtype

[Run]
Filename: "{sys}\sc.exe"; Parameters: "create {#MyServiceName} binPath= ""{app}\{#MyServerExeName}"" DisplayName= ""Tunnerse Daemon"" start= auto obj= LocalSystem"; Flags: runhidden waituntilterminated; StatusMsg: "Creating Tunnerse Windows service..."
Filename: "{sys}\sc.exe"; Parameters: "description {#MyServiceName} ""Local tunnel daemon for Tunnerse CLI"""; Flags: runhidden waituntilterminated; StatusMsg: "Configuring service description..."
Filename: "{sys}\sc.exe"; Parameters: "failure {#MyServiceName} reset= 86400 actions= restart/5000/restart/5000/restart/5000"; Flags: runhidden waituntilterminated; StatusMsg: "Configuring service recovery..."
Filename: "{sys}\sc.exe"; Parameters: "start {#MyServiceName}"; Flags: runhidden waituntilterminated; StatusMsg: "Starting Tunnerse Daemon..."

[UninstallRun]
Filename: "{sys}\sc.exe"; Parameters: "stop {#MyServiceName}"; Flags: runhidden waituntilterminated; RunOnceId: "StopTunnerseDaemonService"
Filename: "{sys}\sc.exe"; Parameters: "delete {#MyServiceName}"; Flags: runhidden waituntilterminated; RunOnceId: "DeleteTunnerseDaemonService"

[Code]
const
  EnvKey = 'SYSTEM\CurrentControlSet\Control\Session Manager\Environment';

function NormalizePath(Path: string): string;
begin
  Result := RemoveBackslashUnlessRoot(Path);
end;

function PathContains(ExistingPath, Entry: string): Boolean;
var
  LowerExisting: string;
  LowerEntry: string;
begin
  LowerExisting := Lowercase(';' + ExistingPath + ';');
  LowerEntry := Lowercase(';' + NormalizePath(Entry) + ';');
  Result := Pos(LowerEntry, LowerExisting) > 0;
end;

procedure AddInstallDirToPath();
var
  ExistingPath: string;
  NewPath: string;
begin
  if not RegQueryStringValue(HKLM, EnvKey, 'Path', ExistingPath) then begin
    ExistingPath := '';
  end;

  if PathContains(ExistingPath, ExpandConstant('{app}')) then begin
    Exit;
  end;

  if ExistingPath = '' then begin
    NewPath := ExpandConstant('{app}');
  end else begin
    NewPath := ExistingPath + ';' + ExpandConstant('{app}');
  end;

  RegWriteStringValue(HKLM, EnvKey, 'Path', NewPath);
end;

procedure StopAndDeleteExistingService();
var
  ResultCode: Integer;
begin
  Exec(ExpandConstant('{sys}\sc.exe'), 'stop {#MyServiceName}', '', SW_HIDE, ewWaitUntilTerminated, ResultCode);
  Exec(ExpandConstant('{sys}\sc.exe'), 'delete {#MyServiceName}', '', SW_HIDE, ewWaitUntilTerminated, ResultCode);
  Exec(ExpandConstant('{sys}\sc.exe'), 'stop {#MyLegacyServiceName}', '', SW_HIDE, ewWaitUntilTerminated, ResultCode);
  Exec(ExpandConstant('{sys}\sc.exe'), 'delete {#MyLegacyServiceName}', '', SW_HIDE, ewWaitUntilTerminated, ResultCode);
end;

procedure KillExistingProcesses();
var
  ResultCode: Integer;
begin
  Exec(ExpandConstant('{sys}\taskkill.exe'), '/IM {#MyAppExeName} /F', '', SW_HIDE, ewWaitUntilTerminated, ResultCode);
  Exec(ExpandConstant('{sys}\taskkill.exe'), '/IM {#MyServerExeName} /F', '', SW_HIDE, ewWaitUntilTerminated, ResultCode);
end;

procedure CurStepChanged(CurStep: TSetupStep);
begin
  if CurStep = ssInstall then begin
    StopAndDeleteExistingService();
    KillExistingProcesses();
    AddInstallDirToPath();
  end;
end;
