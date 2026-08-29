@echo off
setlocal enabledelayedexpansion
rem ---------------------------------------------------------------------------
rem Run the whole-pipeline probe over several seeds, one run per seed.
rem
rem A single green run proves the seed, not the code. This walks a list that
rem includes the seed that used to fail, so a regression there is visible
rem immediately, plus fresh clock-derived seeds for cases nobody has seen yet.
rem
rem   run-seeds.bat                 the default list below
rem   run-seeds.bat 5               the default list, then 5 random seeds
rem   run-seeds.bat 0 123 456       just seeds 123 and 456
rem
rem Each run writes conptyreconcile-<seed>.log and conptydump-<seed>.txt in the
rem current directory. The summary at the end names every seed that failed, so
rem the log to open is never in question.
rem ---------------------------------------------------------------------------

set EXE=%~dp0conptyreconcile.exe
if not exist "%EXE%" (
  echo conptyreconcile.exe not found next to this script.
  echo Build it with:  GOOS=windows go build -o conptyreconcile.exe ./...
  exit /b 2
)

rem Seeds worth keeping: the one that failed before the conhost port, and the
rem one that first passed after it. Regressions show up here first.
set SEEDS=1787985364328457600 1788001644056794200

set EXTRA=0
if not "%~1"=="" set EXTRA=%~1

rem Any further arguments replace the default list entirely.
if not "%~2"=="" (
  set SEEDS=
  shift
  :collect
  if "%~1"=="" goto collected
  set SEEDS=!SEEDS! %~1
  shift
  goto collect
)
:collected

rem Append the requested number of clock-derived seeds.
for /l %%i in (1,1,%EXTRA%) do (
  set /a R=!random! * 32768 + !random!
  set SEEDS=!SEEDS! !R!
  rem The clock moves between iterations, so the seeds differ.
  ping -n 2 127.0.0.1 >nul
)

set PASSED=0
set FAILED=0
set BAD=

for %%S in (%SEEDS%) do (
  echo.
  echo === seed %%S
  "%EXE%" -seed %%S -log conptyreconcile-%%S.log -out conptydump-%%S.txt
  if errorlevel 1 (
    set /a FAILED+=1
    set BAD=!BAD! %%S
  ) else (
    set /a PASSED+=1
  )
)

echo.
echo ===========================================================
echo   passed: %PASSED%   failed: %FAILED%
if not "%BAD%"=="" (
  echo   failing seeds:%BAD%
  echo   open conptyreconcile-^<seed^>.log and conptydump-^<seed^>.txt
  exit /b 1
)
echo   every seed agreed with the reference console.
exit /b 0
