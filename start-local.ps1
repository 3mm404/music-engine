$ErrorActionPreference = 'Stop'
$env:ENGINE_AUDIO_BACKEND = 'asio'
if (-not $env:ENGINE_ASIO_DRIVER) { $env:ENGINE_ASIO_DRIVER = 'Dante Virtual Soundcard (x64)' }
if (-not $env:ENGINE_ASIO_SAMPLE_RATE) { $env:ENGINE_ASIO_SAMPLE_RATE = '48000' }
if (-not $env:ENGINE_ASIO_BUFFER_SIZE) { $env:ENGINE_ASIO_BUFFER_SIZE = '256' }
$connectionPath = Join-Path $PSScriptRoot 'engine-state/connection.json'
if (-not (Test-Path -LiteralPath $connectionPath)) {
    throw 'Falta engine-state/connection.json con server, token y mode.'
}
$connection = Get-Content -LiteralPath $connectionPath -Raw | ConvertFrom-Json
$env:ENGINE_SERVER = $connection.server
$env:ENGINE_TOKEN = $connection.token
$env:ENGINE_CHANNEL_MODE = $connection.mode
$env:ENGINE_PROFILE = 'shared_stereo_mp3'
$env:ENGINE_STATE_DIR = Join-Path $PSScriptRoot 'engine-state'
try {
    & (Join-Path $PSScriptRoot 'bin/agent.exe')
    if ($LASTEXITCODE -ne 0) { throw "Engine ASIO termino con codigo $LASTEXITCODE" }
} finally {
    Remove-Item Env:ENGINE_TOKEN -ErrorAction SilentlyContinue
}
