[CmdletBinding()]
param(
    [string]$OutputDirectory = 'build/desktop',
    [switch]$PrepareOnly
)

$ErrorActionPreference = 'Stop'
$PSNativeCommandUseErrorActionPreference = $true
Set-StrictMode -Version Latest

$projectRoot = Split-Path $PSScriptRoot -Parent
Push-Location $projectRoot
try {
    $package = Get-Content -LiteralPath 'frontend/package.json' -Raw | ConvertFrom-Json
    $version = [string]$package.version
    if ($version -notmatch '^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$' -or
        @($version.Split('.') | Where-Object { [long]$_ -gt 65535 }).Count -ne 0) {
        throw 'Desktop version must be a three-part semantic version that fits Windows version fields.'
    }
    if ($package.packageManager -ne 'pnpm@9.15.9') { throw 'Update the desktop tool lock with packageManager.' }
    $goModule = Get-Content -LiteralPath 'go.mod' -Raw
    if ($goModule -notmatch '(?m)^\s*github\.com/wailsapp/wails/v3 v3\.0\.0-beta\.22\s*$') {
        throw 'Review desktop resource and installer compatibility before changing Wails.'
    }
    if (-not $PrepareOnly -and $env:GITHUB_ACTIONS -ne 'true') {
        throw 'Delivery compilation runs in GitHub Actions. Use -PrepareOnly for local metadata review.'
    }
    $output = [IO.Path]::GetFullPath($OutputDirectory, $projectRoot)
    if (Test-Path -LiteralPath $output) { throw 'Desktop output already exists; choose a new review directory.' }
    New-Item -ItemType Directory -Path $output | Out-Null
    $info = [ordered]@{
        fixed = @{ file_version = "$version.0"; product_version = "$version.0" }
        info = @{ '0000' = [ordered]@{
            ProductVersion = $version; FileVersion = $version
            CompanyName = 'VerdantFlare'; ProductName = 'VerdantFlare Studio'
            FileDescription = 'VerdantFlare Studio desktop'
            OriginalFilename = 'studio-desktop.exe'
            Comments = 'Unsigned development acceptance artifact'
        } }
    }
    $info | ConvertTo-Json -Depth 6 | Set-Content -LiteralPath (Join-Path $output 'info.json') -Encoding utf8NoBOM
    $manifest = (Get-Content -LiteralPath 'packaging/windows/studio.manifest.xml' -Raw).Replace('@VERSION@', $version)
    $manifest | Set-Content -LiteralPath (Join-Path $output 'studio.manifest') -Encoding utf8NoBOM
    if ($PrepareOnly) {
        Write-Output "Prepared Windows resource metadata for $version; no GUI compiled."
        return
    }

    $goVersion = (& go version).Trim()
    $nodeVersion = (& node --version).Trim()
    $pnpmVersion = (& pnpm --version).Trim()
    $nsisVersion = (& makensis /VERSION).Trim()
    if ($goVersion -ne 'go version go1.26.5 windows/amd64' -or $nodeVersion -ne 'v22.20.0' -or
        $pnpmVersion -ne '9.15.9' -or $nsisVersion -ne 'v3.11') { throw 'Desktop build tool versions differ from the reviewed lock.' }
    $wailsMetadata = & go version -m (Get-Command wails3).Source
    if (-not ($wailsMetadata -match '\bmod\s+github\.com/wailsapp/wails/v3\s+v3\.0\.0-beta\.22\s')) {
        throw 'Expected the locked Wails v3 CLI.'
    }
    $revision = (& git rev-parse HEAD).Trim()
    if ($revision -cne $env:GITHUB_SHA -or $revision -notmatch '^[0-9a-f]{40}$') { throw 'Checkout revision differs from Actions source.' }
    if ($env:GITHUB_REPOSITORY -notmatch '^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$' -or
        $env:GITHUB_RUN_ID -notmatch '^[0-9]+$') { throw 'Missing Actions source evidence.' }

    & pnpm --dir frontend install --frozen-lockfile
    $env:VITE_APPCENTER_MODE = 'unified'
    & pnpm --dir frontend exec vite build
    $icon = Join-Path $output 'studio.ico'
    & wails3 generate icons -input frontend/public/logo/verdantflare-logo.png -windowsfilename $icon -macfilename (Join-Path $output 'unused.icns')
    $syso = Join-Path $projectRoot 'cmd/desktop/rsrc_windows_amd64.syso'
    if (Test-Path -LiteralPath $syso) { throw 'Refusing to replace an existing Windows resource object.' }
    $portable = Join-Path $output 'portable'
    $dist = Join-Path $output 'dist'
    New-Item -ItemType Directory -Path $portable, $dist | Out-Null
    try {
        & wails3 generate syso -arch amd64 -icon $icon -manifest (Join-Path $output 'studio.manifest') -info (Join-Path $output 'info.json') -out $syso
        $env:GOOS = 'windows'
        $env:GOARCH = 'amd64'
        $env:CGO_ENABLED = '0'
        & go build -mod=readonly -tags production -trimpath -buildvcs=true -ldflags '-s -w -H windowsgui' -o (Join-Path $portable 'studio-desktop.exe') ./cmd/desktop
    } finally {
        # This single generated file is inside the checked main package, never a directory.
        if (Test-Path -LiteralPath $syso) { Remove-Item -LiteralPath $syso }
    }
    $evidence = [ordered]@{
        schemaVersion = 1; product = 'verdantflare-studio'; version = $version
        platform = 'windows/amd64'; signing = 'unsigned'; purpose = 'development-acceptance'
        repository = $env:GITHUB_REPOSITORY; revision = $revision
        workflowRun = "https://github.com/$($env:GITHUB_REPOSITORY)/actions/runs/$($env:GITHUB_RUN_ID)"
        tools = @{ go = $goVersion; node = $nodeVersion; pnpm = $pnpmVersion; wails = 'v3.0.0-beta.22'; nsis = $nsisVersion }
        build = @{ tags = @('production'); cgoEnabled = 0; subsystem = 'windowsgui'; appCenterMode = 'unified' }
        inputs = @('go.sum', 'frontend/pnpm-lock.yaml', 'frontend/public/logo/verdantflare-logo.png') | ForEach-Object {
            @{ path = $_; sha256 = (Get-FileHash -LiteralPath $_ -Algorithm SHA256).Hash.ToLowerInvariant() }
        }
    }
    $evidence | ConvertTo-Json -Depth 7 | Set-Content -LiteralPath (Join-Path $portable 'build-info.json') -Encoding utf8NoBOM
    @'
VerdantFlare Studio - unsigned Windows amd64 development acceptance artifact

Requires Windows 10 or later and Microsoft Edge WebView2 Evergreen Runtime.
Runtime download: https://developer.microsoft.com/microsoft-edge/webview2/

Set STUDIO_SERVER_URL to the reviewed, reachable HTTPS Studio origin in a
PowerShell session, then launch studio-desktop.exe from that same session.
Example only (replace the origin):
  $env:STUDIO_SERVER_URL = 'https://studio.example.com'
  & '.\studio-desktop.exe'

The installer does not configure endpoints or credentials and does not start
Studio automatically. Without STUDIO_SERVER_URL the existing desktop code uses
its local Core development fallback. Log in through Studio after launch.

Close Studio before updating or uninstalling. The installer uses only the fixed
per-user Programs\VerdantFlare Studio directory. Uninstall preserves WebView data,
work copies, and any unowned files; nonempty directories are retained.
This build is not Authenticode signed and is not a production release.
See build-info.json for the source revision and exact tool versions.
'@ | Set-Content -LiteralPath (Join-Path $portable 'README.txt') -Encoding utf8NoBOM
    Copy-Item -LiteralPath (Join-Path $portable 'build-info.json') -Destination $dist
    Copy-Item -LiteralPath (Join-Path $portable 'README.txt') -Destination $dist
    $baseName = "studio-desktop-v$version-windows-amd64-unsigned"
    Compress-Archive -LiteralPath (Join-Path $portable 'studio-desktop.exe'), (Join-Path $portable 'build-info.json'), (Join-Path $portable 'README.txt') -DestinationPath (Join-Path $dist "$baseName-portable.zip")
    & makensis /V3 "/DAPP_VERSION=$version" "/DAPP_BINARY=$(Join-Path $portable 'studio-desktop.exe')" "/DAPP_ICON=$icon" "/DBUILD_INFO=$(Join-Path $portable 'build-info.json')" "/DOUTPUT_FILE=$(Join-Path $dist "$baseName-installer.exe")" packaging/windows/installer.nsi
    Push-Location $output
    try { & go version -m portable/studio-desktop.exe | Set-Content -LiteralPath 'dist/go-build-info.txt' -Encoding utf8NoBOM }
    finally { Pop-Location }
    $checksums = @(Get-ChildItem -LiteralPath $dist -File | Sort-Object Name | ForEach-Object {
        '{0}  {1}' -f (Get-FileHash -LiteralPath $_.FullName -Algorithm SHA256).Hash.ToLowerInvariant(), $_.Name
    })
    $checksums | Set-Content -LiteralPath (Join-Path $dist 'SHA256SUMS') -Encoding ascii
    "version=$version" >> $env:GITHUB_OUTPUT
    Write-Output "Prepared unsigned Windows amd64 acceptance artifacts for $version."
} finally {
    Pop-Location
}
