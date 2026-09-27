@echo off
setlocal
cd /d "%~dp0"
if not exist dist mkdir dist
set GOOS=linux
set GOARCH=arm
set GOARM=7
set CGO_ENABLED=0
go build -trimpath -ldflags="-s -w" -o dist\rm100_spotify_v29 .
if errorlevel 1 exit /b 1
echo Built dist\rm100_spotify_v29 for Linux ARMv7.







