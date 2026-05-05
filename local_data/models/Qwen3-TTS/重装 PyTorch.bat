@echo off
@setlocal DisableDelayedExpansion
set "__WorkPath__=%~dp0"
if "%__WorkPath__:~-1%"=="\" set "__WorkPath__=%__WorkPath__:~0,-1%"
powershell -ExecutionPolicy Bypass -File "%__WorkPath__%\reinstall_pytorch.ps1" %*
exit %errorlevel%