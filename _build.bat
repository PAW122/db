@echo off
setlocal enabledelayedexpansion

REM Ustawienia
set "BUILD_DIR=builds"
set "BASE_NAME=pawiu-db"

REM Znajdź najwyższą wersję w folderze builds
set "latest_version="
for /f "tokens=*" %%A in ('dir /b /a-d "%BUILD_DIR%\%BASE_NAME% *" 2^>nul') do (
    for /f "tokens=2 delims= " %%B in ("%%~nA") do (
        set "version=%%B"
        set "version=!version:version=v!"
        set "major=!version:~0,-2!"
        set "minor=!version:~-1!"
        set /a "numeric_version=!major! * 10 + !minor!"
        if not defined latest_version set "latest_version=!numeric_version!"
        if !numeric_version! gtr !latest_version! set "latest_version=!numeric_version!"
    )
)

REM Jeśli nie ma wcześniejszych wersji, ustaw wersję na 1.0
if not defined latest_version (
    set "next_version=1.0"
) else (
    set /a "major=latest_version / 10"
    set /a "minor=latest_version %% 10 + 1"
    set "next_version=!major!.!minor!"
)

REM Ścieżki do plików
set "output_file=%BASE_NAME%.exe"
set "versioned_output_file=%BUILD_DIR%\%BASE_NAME% %next_version%.exe"

REM Budowanie aplikacji
go build -o %output_file%

REM Kopiowanie pliku do folderu builds z odpowiednią wersją
if not exist "%BUILD_DIR%" mkdir "%BUILD_DIR%"
copy "%output_file%" "%versioned_output_file%"

REM Wyświetl informację o wersji
echo Zbudowano wersję: %next_version%
echo Plik został zapisany jako: %versioned_output_file%
