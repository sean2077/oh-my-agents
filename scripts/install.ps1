<#
.SYNOPSIS
  Install oma on native Windows from PowerShell.

.DESCRIPTION
  Downloads the prebuilt oma.exe for a GitHub release and verifies BOTH its
  SHA-256 against the release checksums.txt AND the installed binary's reported
  version against the requested release (the same fail-closed contract as
  scripts/install.sh and self-update; see docs/reference/security-contract.md
  §5). This prebuilt path is the only default.

  FAIL-CLOSED: if it cannot resolve a release, find a matching asset, verify the
  checksum, or confirm the version, it stops. It never silently builds from
  source or from the unreleased 'main' branch — a source build is an explicit
  opt-in (-FromSource), and even then it builds the newest released tag unless
  -Ref overrides it.

.PARAMETER Version
  'latest' (default) or a tag like v0.9.0. Also read from $env:OMA_INSTALL_VERSION.

.PARAMETER BinDir
  Install directory (default: $HOME\.local\bin). Also $env:OMA_INSTALL_BIN_DIR.

.PARAMETER FromSource
  Opt into a source build (needs git + go). Also $env:OMA_INSTALL_FROM_SOURCE=1.

.PARAMETER Ref
  Source-build git ref. Also $env:OMA_INSTALL_REF.

.PARAMETER File
  Install a local prebuilt oma.exe at this path instead of downloading (requires
  -Version <expected tag>; used by CI to smoke-test the just-built asset). Also
  $env:OMA_INSTALL_FILE.

.EXAMPLE
  # Latest release (default):
  irm https://raw.githubusercontent.com/sean2077/oh-my-agents/main/scripts/install.ps1 | iex
.EXAMPLE
  # Pin to a release: set the version, fetch the script at that tag, then run it.
  $v = 'v0.9.1'; $env:OMA_INSTALL_VERSION = $v
  irm "https://raw.githubusercontent.com/sean2077/oh-my-agents/$v/scripts/install.ps1" | iex
#>
[CmdletBinding()]
param(
  [string]$Version  = $(if ($env:OMA_INSTALL_VERSION) { $env:OMA_INSTALL_VERSION } else { 'latest' }),
  [string]$BinDir   = $(if ($env:OMA_INSTALL_BIN_DIR) { $env:OMA_INSTALL_BIN_DIR } else { Join-Path $HOME '.local\bin' }),
  [string]$Repo     = $(if ($env:OMA_INSTALL_REPO) { $env:OMA_INSTALL_REPO } else { 'sean2077/oh-my-agents' }),
  [string]$Ref      = $env:OMA_INSTALL_REF,
  [string]$File     = $env:OMA_INSTALL_FILE,
  [switch]$FromSource
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'
[Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12 -bor [Net.ServicePointManager]::SecurityProtocol

if (-not $FromSource -and $env:OMA_INSTALL_FROM_SOURCE -eq '1') { $FromSource = $true }

$BinName = if ($env:OMA_INSTALL_BIN_NAME) { $env:OMA_INSTALL_BIN_NAME } else { 'oma.exe' }
$dest    = Join-Path $BinDir $BinName

function Die($msg) { Write-Error "ERR $msg"; exit 1 }

function Get-Arch {
  $a = $env:PROCESSOR_ARCHITECTURE
  if ($env:PROCESSOR_ARCHITEW6432) { $a = $env:PROCESSOR_ARCHITEW6432 }
  switch ($a) {
    'AMD64' { 'amd64' }
    'ARM64' { 'arm64' }
    default { Die "unsupported architecture '$a'. Set -FromSource to build from source (needs git + go)." }
  }
}

function Resolve-LatestTag {
  try {
    $rel = Invoke-RestMethod -UseBasicParsing -Uri "https://api.github.com/repos/$Repo/releases/latest" -Headers @{ 'User-Agent' = 'oma-install' }
    if ($rel.tag_name) { return $rel.tag_name }
  } catch { }
  return $null
}

function Get-BinaryVersion($path) {
  try { return (& $path version --json | ConvertFrom-Json).version } catch { return $null }
}

# Validate an artifact BEFORE it is allowed to replace the target, so a wrong or
# unstartable binary never clobbers a working install (self-update validates the
# temp binary the same way before swapping).
function Confirm-ArtifactVersion($path, $version) {
  $got = Get-BinaryVersion $path
  if (-not $got)         { Die "downloaded binary did not report a version (wanted $version); target left untouched" }
  if ($got -ne $version) { Die "version mismatch: downloaded binary reports '$got', expected '$version'; target left untouched" }
}

# Place an ALREADY-VERIFIED binary at $dest atomically: back up any existing
# target, move into place, re-check, and roll the previous one back on failure
# (the install-time analogue of self-update's backup + post-replace self-check).
function Install-Atomic($srcFile, $version) {
  New-Item -ItemType Directory -Force -Path $BinDir | Out-Null
  $tmp = Join-Path $BinDir (".{0}.tmp.{1}" -f $BinName, $PID)
  Copy-Item -Force $srcFile $tmp
  $backup = $null
  if (Test-Path $dest) {
    $backup = Join-Path $BinDir (".{0}.old.{1}" -f $BinName, $PID)
    Copy-Item -Force $dest $backup
  }
  Move-Item -Force $tmp $dest
  $got = Get-BinaryVersion $dest
  if ($got -ne $version) {
    if ($backup) {
      Move-Item -Force $backup $dest
      Die "post-install check failed (installed binary reports '$got', expected '$version'); previous binary restored"
    }
    Remove-Item -Force $dest -ErrorAction SilentlyContinue
    Die "post-install check failed (installed binary reports '$got', expected '$version'); removed the bad install"
  }
  if ($backup) { Remove-Item -Force $backup -ErrorAction SilentlyContinue }
  Write-Host "installed $dest"
  Write-Host "version verified: $got"
}

function Path-Note {
  $parts = $env:PATH -split ';'
  if ($parts -notcontains $BinDir) {
    Write-Host "NOTE: $BinDir is not on PATH. Add it (current user):"
    Write-Host "  setx PATH `"$BinDir;`$env:PATH`""
  }
}

function Install-FromRelease {
  $arch = Get-Arch
  $tag = $Version
  if ($tag -eq 'latest') {
    $tag = Resolve-LatestTag
    if (-not $tag) { Die "could not resolve the latest release for $Repo. Check your network, pass -Version vX.Y.Z, or -FromSource to build from source." }
  }
  $asset = "oma_${tag}_windows_${arch}.exe"
  $base  = "https://github.com/$Repo/releases/download/$tag"
  $work  = Join-Path ([IO.Path]::GetTempPath()) ("oma-install-" + [Guid]::NewGuid().ToString('N'))
  New-Item -ItemType Directory -Force -Path $work | Out-Null
  try {
    Write-Host "downloading $asset ($tag)"
    try { Invoke-WebRequest -UseBasicParsing -Uri "$base/$asset" -OutFile (Join-Path $work $asset) }
    catch { Die "no prebuilt asset $asset for $tag. Pass -FromSource to build $tag from source (needs git + go)." }

    try { Invoke-WebRequest -UseBasicParsing -Uri "$base/checksums.txt" -OutFile (Join-Path $work 'checksums.txt') }
    catch { Die "release $tag has no checksums.txt (unverifiable)" }

    $want = $null
    $matches = 0
    foreach ($line in Get-Content (Join-Path $work 'checksums.txt')) {
      $f = $line -split '\s+'
      if ($f.Count -ge 2 -and $f[1] -eq $asset) {
        $matches += 1
        $want = $f[0].ToLower()
      }
    }
    if ($matches -gt 1) { Die "checksums.txt has duplicate entries for $asset" }
    if (-not $want) { Die "checksums.txt has no entry for $asset" }
    $got = (Get-FileHash -Algorithm SHA256 (Join-Path $work $asset)).Hash.ToLower()
    if ($got -ne $want) { Die "checksum mismatch for $asset (want $want, got $got)" }
    Write-Host "checksum ok"

    Confirm-ArtifactVersion (Join-Path $work $asset) $tag
    Install-Atomic (Join-Path $work $asset) $tag
    Path-Note
  } finally {
    Remove-Item -Recurse -Force $work -ErrorAction SilentlyContinue
  }
}

function Resolve-SourceRef {
  if ($Ref) { return $Ref }
  if ($Version -ne 'latest') { return $Version }
  $tag = Resolve-LatestTag
  if ($tag) { return $tag }
  return 'main'
}

function Build-FromSource {
  if (-not (Get-Command git -ErrorAction SilentlyContinue)) { Die "missing required command: git" }
  if (-not (Get-Command go  -ErrorAction SilentlyContinue)) { Die "missing required command: go" }
  $ref = Resolve-SourceRef
  if ($ref -eq 'main') { Write-Warning "building from the unreleased 'main' branch — no pinned tag and no release could be resolved." }
  Write-Host "building oma from source (ref: $ref)"
  $work = Join-Path ([IO.Path]::GetTempPath()) ("oma-src-" + [Guid]::NewGuid().ToString('N'))
  New-Item -ItemType Directory -Force -Path $work | Out-Null
  try {
    git -c advice.detachedHead=false clone --quiet --depth 1 --branch $ref "https://github.com/$Repo.git" "$work\src"
    if ($LASTEXITCODE -ne 0) { Die "git clone failed for ref $ref" }
    $commit = (& git -C "$work\src" rev-parse --short HEAD) 2>$null
    if (-not $commit) { $commit = 'none' }
    if ($ref -ne 'main') { $sv = $ref } else { $sv = (& git -C "$work\src" describe --tags --always) 2>$null; if (-not $sv) { $sv = 'main' } }
    $ld = "-s -w -X github.com/sean2077/oh-my-agents/internal/version.Version=$sv -X github.com/sean2077/oh-my-agents/internal/version.Commit=$commit"
    Push-Location "$work\src"
    try {
      & go build -trimpath -ldflags $ld -o "$work\oma.exe" ./cmd/oma
      if ($LASTEXITCODE -ne 0) { Die "go build failed" }
    } finally { Pop-Location }
    Confirm-ArtifactVersion "$work\oma.exe" $sv
    Install-Atomic "$work\oma.exe" $sv
    Path-Note
  } finally {
    Remove-Item -Recurse -Force $work -ErrorAction SilentlyContinue
  }
}

function Install-FromFile {
  if (-not (Test-Path $File)) { Die "file not found: $File" }
  if ($Version -eq 'latest')  { Die "-File requires -Version <expected tag>" }
  Write-Host "installing local binary $File ($Version)"
  Confirm-ArtifactVersion $File $Version
  Install-Atomic $File $Version
  Path-Note
}

if ($File)           { Install-FromFile }
elseif ($FromSource) { Build-FromSource }
else                 { Install-FromRelease }
