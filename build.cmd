@echo off
chcp 65001
pushd %~dp0
taskkill /f /im:ftp_sync.exe 2>nul
taskkill /f /im:wget.exe 2>nul
del ftp_sync.exe 2>nul
go build -o ftp_sync.exe
start "ftp_sync.exe" ftp_sync.exe
