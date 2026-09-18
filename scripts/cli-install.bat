@echo off

set curDir=%~dp0

cacls.exe "%SystemDrive%\System Volume Information" >nul 2>nul
if %errorlevel%==0 goto Admin
if exist "%temp%\getadmin.vbs" del /f /q "%temp%\getadmin.vbs"
echo Set RequestUAC = CreateObject^("Shell.Application"^)>"%temp%\getadmin.vbs"
echo RequestUAC.ShellExecute "%~s0","","","runas",1 >>"%temp%\getadmin.vbs"
echo WScript.Quit >>"%temp%\getadmin.vbs"
"%temp%\getadmin.vbs" /f
if exist "%temp%\getadmin.vbs" del /f /q "%temp%\getadmin.vbs"
exit

:Admin
echo.
echo ---------准备安装Galaxy Cli---------
echo ------------开始安装----------------
echo.

set userBin=%USERPROFILE%\bin
set GalaxyFilePath=%curDir%galaxy.exe

echo --- 将要把GalaxyCli安装到 %userBin% ---
echo.

if not exist %GalaxyFilePath% (
    echo.
    echo --- 没有在目录 %curDir% 下找到 galaxy.exe ---
    echo.
    pause
    exit
)

if not exist %userBin% (
   echo.
   echo --- %userBin% 目录不存在,创建它. ---
   echo.
   mkdir  %userBin%
)

for /f "usebackq skip=1 tokens=*" %%i in (`wmic environment where ^(name^="PATH" and username^="<system>"^) get variableValue ^| findstr /r /v "^$"`) do (
    set remain=%%i
)

wmic ENVIRONMENT  where "name='path' and username='<system>'" set VariableValue="%userBin%;%remain%"

echo.
echo --- 添加 %userBin% 到环境变量PATH. ---
echo.

copy %GalaxyFilePath% %userBin%

echo --- 把 %GalaxyFilePath% 移动到 %userBin% 完成. ---
echo.

echo --- 安装GalaxyCli已完成. ---
echo.
pause