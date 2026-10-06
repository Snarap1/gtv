# Installs the latest gtv release on Windows.
#
#   irm https://raw.githubusercontent.com/Snarap1/gtv/main/install.ps1 | iex
#
# Downloads gtv-windows-amd64.exe, verifies it against checksums.txt and puts it
# at %LOCALAPPDATA%\Programs\gtv\gtv.exe, adding that directory to the user PATH.
# Set $env:GTV_VERSION (e.g. "v0.2.0") before running to pin a release.
# Re-running upgrades in place. Works in Windows PowerShell 5.1 and PowerShell 7.

$ErrorActionPreference = 'Stop'
$ProgressPreference = 'SilentlyContinue'  # Invoke-WebRequest is very slow with the progress bar in 5.1

& {
    if (-not [Environment]::Is64BitOperatingSystem) {
        throw 'gtv: only 64-bit Windows (amd64) builds are published.'
    }

    # Windows PowerShell 5.1 may default to TLS 1.0, which GitHub rejects.
    [Net.ServicePointManager]::SecurityProtocol = [Net.ServicePointManager]::SecurityProtocol -bor [Net.SecurityProtocolType]::Tls12

    $asset = 'gtv-windows-amd64.exe'
    $base = if ($env:GTV_VERSION) {
        "https://github.com/Snarap1/gtv/releases/download/$($env:GTV_VERSION)"
    } else {
        'https://github.com/Snarap1/gtv/releases/latest/download'
    }

    $dir = Join-Path $env:LOCALAPPDATA 'Programs\gtv'
    $exe = Join-Path $dir 'gtv.exe'
    $tmp = Join-Path $dir 'gtv.exe.download'
    New-Item -ItemType Directory -Force -Path $dir | Out-Null

    Write-Host "gtv: downloading $base/$asset"
    try {
        Invoke-WebRequest -UseBasicParsing -Uri "$base/$asset" -OutFile $tmp
        $sums = (Invoke-WebRequest -UseBasicParsing -Uri "$base/checksums.txt").Content
        if ($sums -is [byte[]]) { $sums = [Text.Encoding]::UTF8.GetString($sums) }

        $line = $sums -split "`r?`n" | Where-Object { $_ -match "^\s*([0-9a-fA-F]{64})\s+\*?$([regex]::Escape($asset))\s*$" } | Select-Object -First 1
        if (-not $line) { throw "gtv: $asset not listed in checksums.txt" }
        $expected = ($line -split '\s+')[0].ToLowerInvariant()
        $actual = (Get-FileHash -Algorithm SHA256 -Path $tmp).Hash.ToLowerInvariant()
        if ($actual -ne $expected) {
            throw "gtv: checksum mismatch for $asset (expected $expected, got $actual)"
        }

        Move-Item -Force -Path $tmp -Destination $exe
    } finally {
        Remove-Item -Force -ErrorAction SilentlyContinue -Path $tmp
    }

    # Read the user PATH from the registry rather than $env:Path, which also
    # holds the machine PATH and would get duplicated into the user scope.
    $userPath = [Environment]::GetEnvironmentVariable('Path', 'User')
    $entries = @($userPath -split ';' | Where-Object { $_ })
    if ($entries -notcontains $dir) {
        [Environment]::SetEnvironmentVariable('Path', (($entries + $dir) -join ';'), 'User')
        Write-Host "gtv: added $dir to the user PATH"
    }
    if (@($env:Path -split ';') -notcontains $dir) {
        $env:Path = "$env:Path;$dir"
    }

    Write-Host "gtv: installed $(& $exe --version) to $exe"
    Write-Host 'gtv: open a new terminal so other shells pick up the PATH change.'
}
