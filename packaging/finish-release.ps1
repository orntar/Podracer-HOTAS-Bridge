$ErrorActionPreference = 'Stop'
$projectRoot = Split-Path -Parent $PSScriptRoot
Set-Location -LiteralPath $projectRoot
& './packaging/build-windows.ps1'
$unsquashExe = Join-Path $projectRoot '.tools/squashfs/tools/squashfs-tools/unsquashfs.exe'
$manifest = & $unsquashExe -ll build/payload.squashfs
if ($LASTEXITCODE -ne 0) { throw 'Archive listing failed' }
$manifest | Set-Content -LiteralPath 'dist/package-manifest.txt' -Encoding utf8
if (!($manifest | Where-Object { $_ -match '^drwx..xr.x.* squashfs-root$' })) { throw 'Archive root is not readable/traversable by normal users' }
if (!($manifest | Where-Object { $_ -match '^-rwxr-xr-x.* squashfs-root/AppRun$' })) { throw 'AppRun is not executable' }
& $unsquashExe -d build/verified.AppDir -f -no-progress -processors 2 build/payload.squashfs
if ($LASTEXITCODE -ne 0) { throw 'Payload extraction failed' }
$expected = (Get-FileHash 'build/AppDir/AppRun').Hash
$actual = (Get-FileHash 'build/verified.AppDir/AppRun').Hash
if ($expected -ne $actual) { throw 'Packaged executable differs from build' }
$imageBytes=[System.IO.File]::ReadAllBytes((Join-Path $projectRoot 'dist/PodracerBridge-0.1.0-x86_64.AppImage'))
$runtimeBytes=[System.IO.File]::ReadAllBytes((Join-Path $projectRoot '.tools/runtime-x86_64'))
$payloadBytes=[System.IO.File]::ReadAllBytes((Join-Path $projectRoot 'build/payload.squashfs'))
if ($imageBytes.Length -ne ($runtimeBytes.Length+$payloadBytes.Length)) { throw 'Invalid AppImage length' }
if ([System.BitConverter]::ToString($imageBytes,8,3) -ne '41-49-02') { throw 'Missing type-2 AppImage signature' }
if ([System.Text.Encoding]::ASCII.GetString($imageBytes,$runtimeBytes.Length,4) -ne 'hsqs') { throw 'Missing SquashFS payload signature' }
$payloadFromImage = New-Object byte[] $payloadBytes.Length
[Array]::Copy($imageBytes,$runtimeBytes.Length,$payloadFromImage,0,$payloadBytes.Length)
$sha=[System.Security.Cryptography.SHA256]::Create()
if ([System.BitConverter]::ToString($sha.ComputeHash($payloadFromImage)) -ne [System.BitConverter]::ToString($sha.ComputeHash($payloadBytes))) { throw 'Embedded filesystem mismatch' }
$sha.Dispose()
Copy-Item -LiteralPath 'README.md','VALIDATION.md','BUILD_PROVENANCE.json','THIRD_PARTY_NOTICES.md','LICENSE' -Destination dist
Copy-Item -LiteralPath 'licenses' -Destination dist -Recurse -Force
Copy-Item -LiteralPath 'packaging/START-HERE.txt' -Destination 'dist/START-HERE.txt'
Write-Output 'PASS: AppImage signature, payload integrity, directory access, executable permissions, and extracted executable hash.'
$manifest | Select-Object -Last 15
