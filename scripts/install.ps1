# Installs Kishtin (kishtind.exe and kishtin.exe) on Windows from a release.
#
#   irm https://github.com/cathalbyrnegit/kishtin/releases/latest/download/install.ps1 | iex
#
# KISHTIN_VERSION         a release such as v0.2.0 (default: the latest)
# KISHTIN_INSTALL_DIR     where to put the programs
#                         (default: %LOCALAPPDATA%\Programs\kishtin\bin)
# KISHTIN_RELEASE_URL     a folder holding the archives and SHA256SUMS, for a
#                         mirror or a local test (default: GitHub releases)
# KISHTIN_NO_MODIFY_PATH  set to 1 to leave the user PATH as it is
# KISHTIN_STARTUP         yes or no: whether Kishtin starts by itself when you
#                         log in (default: ask, or keep it on if it was)
#
# The archive is checked against SHA256SUMS before anything is installed,
# and must hold kishtind.exe and kishtin.exe and nothing else.

$ErrorActionPreference = 'Stop'
$ProgressPreference = 'SilentlyContinue'

function Fail($msg) {
    Write-Host "kishtin install: $msg" -ForegroundColor Red
    throw "kishtin install failed"
}

$repo = 'https://github.com/cathalbyrnegit/kishtin'
$version = if ($env:KISHTIN_VERSION) { $env:KISHTIN_VERSION } else { 'latest' }
$dir = if ($env:KISHTIN_INSTALL_DIR) { $env:KISHTIN_INSTALL_DIR } else { Join-Path $env:LOCALAPPDATA 'Programs\kishtin\bin' }

$arch = switch ($env:PROCESSOR_ARCHITECTURE) {
    'AMD64' { 'amd64' }
    'ARM64' { 'arm64' }
    default { Fail "no Kishtin build for $($env:PROCESSOR_ARCHITECTURE): there are builds for 64-bit Windows (amd64 and arm64) only." }
}

if ($env:KISHTIN_RELEASE_URL) {
    $base = $env:KISHTIN_RELEASE_URL.TrimEnd('/')
} elseif ($version -eq 'latest') {
    $base = "$repo/releases/latest/download"
} else {
    $base = "$repo/releases/download/$version"
}
$archive = "kishtin_windows_$arch.zip"

function Get-File($name, $to) {
    $src = "$base/$name"
    if ($src -match '^[A-Za-z]:\\' -or $src.StartsWith('\\') -or $src.StartsWith('file://')) {
        Copy-Item -LiteralPath ($src -replace '^file://', '') -Destination $to
    } else {
        Invoke-WebRequest -Uri $src -OutFile $to -UseBasicParsing
    }
}

$wasOn = $false
$tmp = Join-Path ([IO.Path]::GetTempPath()) ("kishtin-" + [guid]::NewGuid())
New-Item -ItemType Directory -Path $tmp | Out-Null
try {
    Write-Host "Downloading Kishtin $version for windows/$arch..."
    try { Get-File $archive (Join-Path $tmp $archive) } catch { Fail "could not download $base/$archive. Check the internet connection and the version." }
    try { Get-File 'SHA256SUMS' (Join-Path $tmp 'SHA256SUMS') } catch { Fail "could not download $base/SHA256SUMS, so the download cannot be checked. Nothing was installed." }

    $entries = @(Get-Content (Join-Path $tmp 'SHA256SUMS') | Where-Object {
        $parts = $_ -split '\s+', 2
        $parts.Count -eq 2 -and ($parts[1] -eq $archive -or $parts[1] -eq "*$archive")
    })
    if ($entries.Count -eq 0) { Fail "SHA256SUMS has no entry for $archive. Nothing was installed." }
    if ($entries.Count -gt 1) { Fail "SHA256SUMS has more than one entry for $archive. Nothing was installed." }
    $want = ($entries[0] -split '\s+')[0].ToLowerInvariant()
    $got = (Get-FileHash -Algorithm SHA256 (Join-Path $tmp $archive)).Hash.ToLowerInvariant()
    if ($want -ne $got) { Fail "the download does not match SHA256SUMS (expected $want, got $got). Nothing was installed." }

    Add-Type -AssemblyName System.IO.Compression.FileSystem
    $zip = [IO.Compression.ZipFile]::OpenRead((Join-Path $tmp $archive))
    try { $names = @($zip.Entries | ForEach-Object { $_.FullName }) } finally { $zip.Dispose() }
    if ($names.Count -ne 2 -or -not ($names -ccontains 'kishtind.exe') -or -not ($names -ccontains 'kishtin.exe')) { Fail "the archive holds something other than kishtind.exe and kishtin.exe ($($names -join ', ')). Nothing was installed." }

    $x = Join-Path $tmp 'x'
    Expand-Archive -LiteralPath (Join-Path $tmp $archive) -DestinationPath $x
    New-Item -ItemType Directory -Force -Path $dir | Out-Null
    # A Kishtin the computer started is stopped while it is replaced, and
    # started again below: Windows cannot replace a running program.
    $exe = Join-Path $dir 'kishtind.exe'
    if (Test-Path -LiteralPath $exe) {
        try { $wasOn = [bool]((& $exe startup --json 2>$null) | ConvertFrom-Json).on } catch { $wasOn = $false }
        if ($wasOn) {
            try { & $exe startup stop *> $null } catch {}
            Start-Sleep -Seconds 2
        }
    }
    foreach ($p in 'kishtind.exe', 'kishtin.exe') {
        try {
            Copy-Item -LiteralPath (Join-Path $x $p) -Destination (Join-Path $dir $p) -Force
        } catch {
            Fail "could not replace $p, probably because Kishtin is running: close Kishtin's window (or press Ctrl C in it), then run this line again."
        }
    }
} finally {
    Remove-Item -Recurse -Force -LiteralPath $tmp -ErrorAction SilentlyContinue
}

$installed = & (Join-Path $dir 'kishtind.exe') --version
Write-Host ""
Write-Host "Installed $installed in $dir."

$run = Join-Path $dir 'kishtind.exe'
$userPath = [Environment]::GetEnvironmentVariable('Path', 'User')
$onPath = ($userPath -split ';') -contains $dir
if (-not $onPath -and $env:KISHTIN_NO_MODIFY_PATH -ne '1') {
    $newPath = if ($userPath) { "$userPath;$dir" } else { $dir }
    [Environment]::SetEnvironmentVariable('Path', $newPath, 'User')
    $env:Path = "$env:Path;$dir"
    Write-Host "Added $dir to your PATH. New terminals can run kishtind by name."
    $run = 'kishtind'
} elseif ($onPath) {
    $run = 'kishtind'
}
$startup = $env:KISHTIN_STARTUP
if (-not $startup) {
    if ($wasOn) {
        $startup = 'yes'
    } elseif ([Console]::IsInputRedirected) {
        $startup = 'no'
    } else {
        try {
            Write-Host ""
            $answer = Read-Host "Start Kishtin by itself whenever you log in to this computer? Recommended, so nobody has to remember to start it after a restart. [Y/n]"
            $startup = if ($answer -match '^[Nn]') { 'no' } else { 'yes' }
        } catch {
            $startup = 'no'
        }
    }
}
$started = $false
if ($startup -match '^[Yy]') {
    Write-Host ""
    try {
        & (Join-Path $dir 'kishtind.exe') startup on
        $started = $LASTEXITCODE -eq 0
    } catch {
        Write-Host $_
    }
    if (-not $started) {
        Write-Host "Kishtin could not set itself to start when you log in. Start it by hand as below."
    }
}

Write-Host ""
if ($started) {
    Write-Host "Next: open http://127.0.0.1:8080 in a browser on this computer to set it up."
    Write-Host "Kishtin runs in its own window. Leave it open, or minimise it."
} else {
    Write-Host "Next:"
    Write-Host "  1. Start Kishtin:   $run"
    Write-Host "  2. Open http://127.0.0.1:8080 in a browser on this computer to set it up."
    Write-Host ""
    Write-Host "To have Kishtin start by itself when you log in, later: $run startup on"
}
