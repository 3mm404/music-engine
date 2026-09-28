$ErrorActionPreference = 'Stop'
& (Join-Path $PSScriptRoot 'bin/engine.exe') run
exit $LASTEXITCODE
