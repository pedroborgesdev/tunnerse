@echo off
title Tunnerse Installer Builder
color 0B
setlocal enabledelayedexpansion

REM =========================================================
REM Tunnerse CLI and Daemon Windows Graphical Installer Builder
REM Builds both amd64 and x86 binaries
REM =========================================================

set "SCRIPT_DIR=%~dp0"
set "PROJECT_ROOT=%SCRIPT_DIR%..\.."

cd /d "%PROJECT_ROOT%"

set "DIST_DIR=%PROJECT_ROOT%\dist"
set "PAYLOAD_DIR=%DIST_DIR%\installer-payload"
set "VERSION_FILE=%PROJECT_ROOT%\internal\version\VERSION"
set "VERSION=%TUNNERSE_VERSION%"
if "%VERSION%"=="" (
    if exist "%VERSION_FILE%" (
        set /p VERSION=<"%VERSION_FILE%"
    )
)
if "%VERSION%"=="" set "VERSION=dev"
set "VERSION_LDFLAGS=-X github.com/pedroborgesdev/tunnerse-cli/internal/version.Version=%VERSION%"
set "INSTALLER_AMD64=tunnerse-installer_amd64_%VERSION%.exe"
set "INSTALLER_X86=tunnerse-installer_x86_%VERSION%.exe"

set "BIN_CLI_AMD64=%PAYLOAD_DIR%\tunnerse-amd64.exe"
set "BIN_CLI_X86=%PAYLOAD_DIR%\tunnerse-x86.exe"

set "BIN_DAEMON_AMD64=%PAYLOAD_DIR%\tunnerse-daemon-amd64.exe"
set "BIN_DAEMON_X86=%PAYLOAD_DIR%\tunnerse-daemon-x86.exe"

set "INSTALLER_AMD64_PATH=%DIST_DIR%\%INSTALLER_AMD64%"
set "INSTALLER_X86_PATH=%DIST_DIR%\%INSTALLER_X86%"
set "ISS_FILE=%PROJECT_ROOT%\scripts\win\inno\tunnerse-installer.iss"
set "ICON_FILE=%PROJECT_ROOT%\assets\icons\win\tunnerse.ico"

set "ICON_RESOURCE_AMD64=%PROJECT_ROOT%\cmd\cli\tunnerse_icon_windows_amd64.syso"
set "ICON_RESOURCE_X86=%PROJECT_ROOT%\cmd\cli\tunnerse_icon_windows_386.syso"

cls
echo.
echo =====================================================
echo              TUNNERSE INSTALLER BUILDER
echo =====================================================
echo.
echo   Building Windows graphical setup wizard...
echo   Architectures: amd64 and x86
echo.
echo =====================================================
echo.

REM =========================================================
REM Prepare dist
REM =========================================================

echo [1/7] Preparing output directory...
echo.

if not exist "%DIST_DIR%" (
    mkdir "%DIST_DIR%"
    if errorlevel 1 (
        echo   [ERROR] Failed to create dist directory:
        echo.
        echo   %DIST_DIR%
        echo.
        goto :fail
    )
)

if exist "%PAYLOAD_DIR%" (
    rmdir /s /q "%PAYLOAD_DIR%"
)

mkdir "%PAYLOAD_DIR%"
if errorlevel 1 (
    echo   [ERROR] Failed to create installer payload directory:
    echo.
    echo   %PAYLOAD_DIR%
    echo.
    goto :fail
)

del "%DIST_DIR%\tunnerse-installer*.exe" >nul 2>&1

echo   [OK] Output directory:
echo        %DIST_DIR%
echo   [OK] Installer payload:
echo        %PAYLOAD_DIR%
echo.

REM =========================================================
REM Build CLI amd64
REM =========================================================

echo [2/7] Building Windows CLI amd64...
echo.

set GOOS=windows
set GOARCH=amd64

del "%ICON_RESOURCE_AMD64%" >nul 2>&1
del "%ICON_RESOURCE_X86%" >nul 2>&1

go run github.com/akavel/rsrc@latest -arch amd64 -ico "%ICON_FILE%" -o "%ICON_RESOURCE_AMD64%"
if errorlevel 1 (
    echo   [ERROR] Failed to generate amd64 icon resource.
    echo.
    goto :fail
)

go build -buildvcs=false -ldflags "%VERSION_LDFLAGS%" -o "%BIN_CLI_AMD64%" ./cmd/cli
if errorlevel 1 (
    echo   [ERROR] Failed to build tunnerse-amd64.exe
    echo.
    goto :fail
)

del "%ICON_RESOURCE_AMD64%" >nul 2>&1

echo   [OK] Built:
echo        %BIN_CLI_AMD64%
echo.

REM =========================================================
REM Build CLI x86
REM =========================================================

echo [3/7] Building Windows CLI x86...
echo.

set GOOS=windows
set GOARCH=386

del "%ICON_RESOURCE_AMD64%" >nul 2>&1
del "%ICON_RESOURCE_X86%" >nul 2>&1

go run github.com/akavel/rsrc@latest -arch 386 -ico "%ICON_FILE%" -o "%ICON_RESOURCE_X86%"
if errorlevel 1 (
    echo   [ERROR] Failed to generate x86 icon resource.
    echo.
    goto :fail
)

go build -buildvcs=false -ldflags "%VERSION_LDFLAGS%" -o "%BIN_CLI_X86%" ./cmd/cli
if errorlevel 1 (
    echo   [ERROR] Failed to build tunnerse-x86.exe
    echo.
    goto :fail
)

del "%ICON_RESOURCE_X86%" >nul 2>&1

echo   [OK] Built:
echo        %BIN_CLI_X86%
echo.

REM =========================================================
REM Build daemon amd64
REM =========================================================

echo [4/7] Building Windows daemon amd64...
echo.

set GOOS=windows
set GOARCH=amd64

go build -buildvcs=false -ldflags "%VERSION_LDFLAGS%" -o "%BIN_DAEMON_AMD64%" ./cmd/daemon
if errorlevel 1 (
    echo   [ERROR] Failed to build tunnerse-daemon-amd64.exe
    echo.
    goto :fail
)

echo   [OK] Built:
echo        %BIN_DAEMON_AMD64%
echo.

REM =========================================================
REM Build daemon x86
REM =========================================================

echo [5/7] Building Windows daemon x86...
echo.

set GOOS=windows
set GOARCH=386

go build -buildvcs=false -ldflags "%VERSION_LDFLAGS%" -o "%BIN_DAEMON_X86%" ./cmd/daemon
if errorlevel 1 (
    echo   [ERROR] Failed to build tunnerse-daemon-x86.exe
    echo.
    goto :fail
)

echo   [OK] Built:
echo        %BIN_DAEMON_X86%
echo.

REM =========================================================
REM Locate Inno Setup
REM =========================================================

echo [6/7] Locating Inno Setup compiler...
echo.

set "ISCC="

where ISCC.exe >nul 2>&1
if not errorlevel 1 (
    for /f "delims=" %%I in ('where ISCC.exe') do (
        set "ISCC=%%I"
        goto :found_iscc
    )
)

if exist "%ProgramFiles(x86)%\Inno Setup 6\ISCC.exe" (
    set "ISCC=%ProgramFiles(x86)%\Inno Setup 6\ISCC.exe"
    goto :found_iscc
)

if exist "%ProgramFiles%\Inno Setup 6\ISCC.exe" (
    set "ISCC=%ProgramFiles%\Inno Setup 6\ISCC.exe"
    goto :found_iscc
)

echo   [ERROR] Inno Setup compiler was not found.
echo.
echo   Please install Inno Setup 6:
echo.
echo     https://jrsoftware.org/isinfo.php
echo.
goto :fail

:found_iscc
echo   [OK] Found Inno Setup:
echo        %ISCC%
echo.

REM =========================================================
REM Build installer
REM =========================================================

echo [7/7] Building graphical installer...
echo.

if not exist "%ISS_FILE%" (
    echo   [ERROR] Inno Setup script not found:
    echo.
    echo   %ISS_FILE%
    echo.
    goto :fail
)

"%ISCC%" /DMyAppVersion="%VERSION%" /DMyInstallerArch="amd64" /DMyTargetIs64=1 "%ISS_FILE%"
if errorlevel 1 (
    echo.
    echo   [ERROR] Failed to build amd64 graphical installer.
    echo.
    goto :fail
)

"%ISCC%" /DMyAppVersion="%VERSION%" /DMyInstallerArch="x86" /DMyTargetIs64=0 "%ISS_FILE%"
if errorlevel 1 (
    echo.
    echo   [ERROR] Failed to build x86 graphical installer.
    echo.
    goto :fail
)

if not exist "%INSTALLER_AMD64_PATH%" (
    echo.
    echo   [ERROR] amd64 installer not found:
    echo.
    echo   %INSTALLER_AMD64_PATH%
    echo.
    goto :fail
)

if not exist "%INSTALLER_X86_PATH%" (
    echo.
    echo   [ERROR] x86 installer not found:
    echo.
    echo   %INSTALLER_X86_PATH%
    echo.
    goto :fail
)

rmdir /s /q "%PAYLOAD_DIR%" >nul 2>&1

echo.
echo =====================================================
echo                  BUILD COMPLETE
echo =====================================================
echo.
echo   Installers:
echo     %INSTALLER_AMD64_PATH%
echo     %INSTALLER_X86_PATH%
echo.
echo =====================================================
echo.
echo   To test:
echo.
echo     Right-click the installer matching your architecture
echo     Run as administrator
echo.
echo =====================================================
echo.

endlocal
exit /b 0

:fail

del "%ICON_RESOURCE_AMD64%" >nul 2>&1
del "%ICON_RESOURCE_X86%" >nul 2>&1

echo.
echo =====================================================
echo                   BUILD FAILED
echo =====================================================
echo.
echo   Review the messages above and try again.
echo.
echo =====================================================
echo.

endlocal
exit /b 1
