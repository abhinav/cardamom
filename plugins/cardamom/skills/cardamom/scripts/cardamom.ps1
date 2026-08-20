$ErrorActionPreference = "Stop"

$Installed = Get-Command card -CommandType Application -ErrorAction SilentlyContinue
if ($Installed) {
    & $Installed.Source @args
    exit $LASTEXITCODE
}

$Version = "0.9.2"

$CacheRoot = $env:LOCALAPPDATA
if (-not $CacheRoot) {
    $CacheRoot = [Environment]::GetFolderPath(
        [Environment+SpecialFolder]::LocalApplicationData
    )
}
$CacheDirectory = Join-Path $CacheRoot "cardamom-skill\versions\$Version"
$Executable = Join-Path $CacheDirectory "cardamom.exe"
if (Test-Path $Executable -PathType Leaf) {
    & $Executable @args
    exit $LASTEXITCODE
}

$Architecture = [System.Runtime.InteropServices.RuntimeInformation]::OSArchitecture
$Target = switch ($Architecture) {
    "X64" { "Windows-x86_64" }
    "Arm64" { "Windows-arm64" }
    default {
        throw "cardamom: unsupported Windows architecture $Architecture; install card and place it on PATH"
    }
}

New-Item -ItemType Directory -Force -Path $CacheDirectory | Out-Null
$TemporaryDirectory = Join-Path $CacheDirectory (
    ".install." + [Guid]::NewGuid().ToString("N")
)
New-Item -ItemType Directory -Path $TemporaryDirectory | Out-Null

try {
    $ArchiveName = "cardamom.$Target.tar.gz"
    $ReleaseURL = "https://github.com/abhinav/cardamom/releases/download/v$Version"
    $Archive = Join-Path $TemporaryDirectory $ArchiveName
    $Checksums = Join-Path $TemporaryDirectory "checksums.txt"
    Invoke-WebRequest "$ReleaseURL/$ArchiveName" -OutFile $Archive
    Invoke-WebRequest "$ReleaseURL/checksums.txt" -OutFile $Checksums

    $Expected = $null
    foreach ($Line in Get-Content $Checksums) {
        if ($Line -match "^([0-9A-Fa-f]{64})\s+\*?(.+)$" -and $Matches[2] -eq $ArchiveName) {
            $Expected = $Matches[1]
            break
        }
    }
    if (-not $Expected) {
        throw "cardamom: checksums.txt has no entry for $ArchiveName"
    }

    $Actual = (Get-FileHash $Archive -Algorithm SHA256).Hash
    if ($Actual -ne $Expected) {
        throw "cardamom: checksum mismatch for $ArchiveName"
    }

    tar -xzf $Archive -C $TemporaryDirectory card.exe
    $Extracted = Join-Path $TemporaryDirectory "card.exe"
    if (-not (Test-Path $Extracted -PathType Leaf)) {
        throw "cardamom: $ArchiveName does not contain card.exe"
    }

    $Staged = Join-Path $CacheDirectory (
        ".cardamom." + [Guid]::NewGuid().ToString("N") + ".exe"
    )
    Move-Item $Extracted $Staged
    Move-Item -Force $Staged $Executable
} finally {
    Remove-Item -Recurse -Force $TemporaryDirectory -ErrorAction SilentlyContinue
}

& $Executable @args
exit $LASTEXITCODE
