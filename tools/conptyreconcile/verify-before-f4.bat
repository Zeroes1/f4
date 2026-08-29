@echo off
setlocal
rem ---------------------------------------------------------------------------
rem The three runs that have to be green before f4 gets any of this code.
rem
rem They are here rather than in the binary because they are not one check with
rem a list of inputs (that is `-suite`, whose seed list lives in seeds.go) but
rem three different questions, each about a part of the pipeline the field has
rem not exercised yet:
rem
rem   1. -suite            every seed that ever failed, on the current build.
rem
rem   2. ring eviction     the one piece of this tool that is NOT a port of
rem                        Microsoft's code: conhost keeps no scrollback (P16),
rem                        so the mirror taps rows as IncrementCircularBuffer
rem                        retires them. Every field run so far put 151 lines
rem                        into a 2000-row buffer, which never evicts anything,
rem                        so that tap has never once run against real conhost.
rem                        900 lines into 200 rows makes it run for its life.
rem
rem   3. resize mid-output the capture is cut while the child is still
rem                        printing, so the repaint interleaves with live
rem                        output. 20 rounds, because this one is timing
rem                        dependent and a single round proves little.
rem
rem Runs 1 and 2 write a log and a dump; a dump replays offline on any machine,
rem no Windows needed, so send it for anything that fails. Run 3 keeps no dump
rem (the rounds mode holds its capture in memory) -- for a failing round send
rem the log and re-run that seed alone with -seed <n>, which does write one.
rem ---------------------------------------------------------------------------

set EXE=%~dp0conptyreconcile.exe
if not exist "%EXE%" (
  echo conptyreconcile.exe not found next to this script.
  exit /b 2
)

set FAILED=
set BAD=0

echo.
echo ============================================================
echo  1/3  the seeds that once failed
echo ============================================================
"%EXE%" -suite
if errorlevel 1 (
  set /a BAD+=1
  set FAILED=%FAILED% suite
)

echo.
echo ============================================================
echo  2/3  ring eviction: 900 lines into a 200-row buffer
echo ============================================================
"%EXE%" -height 200 -lines 900 -log conptyreconcile-eviction.log -out conptydump-eviction.txt
if errorlevel 1 (
  set /a BAD+=1
  set FAILED=%FAILED% eviction
)

echo.
echo ============================================================
echo  3/3  resize during output, 20 rounds
echo ============================================================
"%EXE%" -fuzz 20 -resize-during-output -log conptyreconcile-during.log
if errorlevel 1 (
  set /a BAD+=1
  set FAILED=%FAILED% during
)

echo.
echo ============================================================
if %BAD%==0 (
  echo   all three green -- the parts f4 depends on are measured,
  echo   including the one piece that is not Microsoft's code.
  exit /b 0
)
echo   %BAD% of 3 failed:%FAILED%
echo   send conptyreconcile-*.log and conptydump-*.txt for those runs.
exit /b 1
