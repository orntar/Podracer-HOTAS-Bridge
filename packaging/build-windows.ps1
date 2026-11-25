$ErrorActionPreference = 'Stop'
$projectRoot = Split-Path -Parent $PSScriptRoot
Set-Location -LiteralPath $projectRoot
$env:GOCACHE = Join-Path $projectRoot '.tools/gocache'
$env:GOTELEMETRY = 'off'
$goExe = Join-Path $projectRoot '.tools/go/bin/go.exe'
$squashExe = Join-Path $projectRoot '.tools/squashfs/tools/squashfs-tools/mksquashfs.exe'
$unsquashExe = Join-Path $projectRoot '.tools/squashfs/tools/squashfs-tools/unsquashfs.exe'
$runtimePath = Join-Path $projectRoot '.tools/runtime-x86_64'
foreach ($required in @($goExe,$squashExe,$runtimePath)) { if (!(Test-Path -LiteralPath $required)) { throw "Missing build tool: $required" } }
& $goExe test ./...
if ($LASTEXITCODE -ne 0) { throw 'Tests failed' }
New-Item -ItemType Directory -Force -Path 'build/AppDir','dist' | Out-Null
$env:GOOS = 'linux'; $env:GOARCH = 'amd64'; $env:CGO_ENABLED = '0'
try {
    & $goExe build -trimpath -ldflags '-s -w -buildid=' -o build/AppDir/AppRun .
    if ($LASTEXITCODE -ne 0) { throw 'Linux build failed' }
} finally { Remove-Item Env:GOOS,Env:GOARCH,Env:CGO_ENABLED -ErrorAction SilentlyContinue }
Copy-Item -LiteralPath 'packaging/podracer-bridge.desktop' -Destination 'build/AppDir/podracer-bridge.desktop'
Copy-Item -LiteralPath 'web/icon.svg' -Destination 'build/AppDir/podracer-bridge.svg'
Copy-Item -LiteralPath 'web/icon.svg' -Destination 'build/AppDir/.DirIcon'
Copy-Item -LiteralPath 'README.md','LICENSE','THIRD_PARTY_NOTICES.md' -Destination 'build/AppDir'
Copy-Item -LiteralPath 'licenses' -Destination 'build/AppDir' -Recurse -Force
Copy-Item -LiteralPath 'BUILD_PROVENANCE.json' -Destination 'build/AppDir'
Copy-Item -LiteralPath 'VALIDATION.md' -Destination 'build/AppDir'
# Cygwin derives Unix read/execute bits from Windows ACLs. The payload is public
# application material; grant read/traverse only on this staging directory.
& icacls.exe (Join-Path $projectRoot 'build/AppDir') /grant '*S-1-1-0:(OI)(CI)(RX)' /T /Q | Out-Null
if ($LASTEXITCODE -ne 0) { throw 'Unable to set readable staging permissions' }
# Cygwin's old Windows port needs explicit POSIX paths. Keep files local to avoid spaces.
Push-Location -LiteralPath 'build'
try {
    & $squashExe AppDir payload.squashfs -noappend -all-root -p 'AppRun m 755 0 0' -comp gzip -no-progress -processors 2
    if ($LASTEXITCODE -ne 0) { throw 'SquashFS packaging failed' }
    & $unsquashExe -s payload.squashfs
    if ($LASTEXITCODE -ne 0) { throw 'SquashFS verification failed' }
} finally { Pop-Location }
$outputPath = Join-Path $projectRoot 'dist/PodracerBridge-0.1.0-x86_64.AppImage'
$outputStream = [System.IO.File]::Create($outputPath)
try {
    foreach ($inputPath in @($runtimePath,(Join-Path $projectRoot 'build/payload.squashfs'))) {
        $inputStream = [System.IO.File]::OpenRead($inputPath)
        try { $inputStream.CopyTo($outputStream) } finally { $inputStream.Dispose() }
    }
} finally { $outputStream.Dispose() }
$hash = (Get-FileHash -LiteralPath $outputPath -Algorithm SHA256).Hash.ToLower()
"$hash  PodracerBridge-0.1.0-x86_64.AppImage" | Set-Content -LiteralPath 'dist/SHA256SUMS' -Encoding ascii
Get-Item -LiteralPath $outputPath | Select-Object Name,Length
Write-Output "SHA256: $hash"
