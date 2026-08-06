[CmdletBinding()]
param(
    [Parameter(Mandatory = $true)]
    [ValidateNotNullOrEmpty()]
    [string]$OutputPath,

    [string]$MySQLDumpPath = "mysqldump",

    [switch]$Force
)

$ErrorActionPreference = "Stop"

function Get-RequiredEnvironmentValue {
    param([Parameter(Mandatory = $true)][string]$Name)

    $value = [Environment]::GetEnvironmentVariable($Name)
    if ([string]::IsNullOrWhiteSpace($value)) {
        throw "$Name is required"
    }
    return $value.Trim()
}

function Resolve-NativeExecutable {
    param([Parameter(Mandatory = $true)][string]$Value)

    if (Test-Path -LiteralPath $Value -PathType Leaf) {
        return (Resolve-Path -LiteralPath $Value).Path
    }
    $command = Get-Command -Name $Value -CommandType Application -ErrorAction SilentlyContinue | Select-Object -First 1
    if ($null -eq $command) {
        throw "Cannot find $Value. Install the MySQL 8.4 client or pass -MySQLDumpPath."
    }
    return $command.Source
}

function New-MySQLClientOptionFile {
    param([Parameter(Mandatory = $true)][string]$Password)

    if ($Password.Contains("`r") -or $Password.Contains("`n")) {
        throw "NOVRO_DATABASE_PASSWORD cannot contain a newline"
    }
    $escaped = $Password.Replace("\", "\\").Replace('"', '\"')
    $path = Join-Path ([IO.Path]::GetTempPath()) ("novro-mysql-" + [guid]::NewGuid().ToString("N") + ".cnf")
    $content = "[client]`r`npassword=`"$escaped`"`r`n"
    [IO.File]::WriteAllText($path, $content, [Text.UTF8Encoding]::new($false))
    return $path
}

function Invoke-NativeCommand {
    param(
        [Parameter(Mandatory = $true)][string]$Executable,
        [Parameter(Mandatory = $true)][string[]]$Arguments
    )

    & $Executable @Arguments
    if ($LASTEXITCODE -ne 0) {
        throw "mysqldump failed with exit code $LASTEXITCODE"
    }
}

$hostName = Get-RequiredEnvironmentValue "NOVRO_DATABASE_HOST"
$portText = Get-RequiredEnvironmentValue "NOVRO_DATABASE_PORT"
$databaseName = Get-RequiredEnvironmentValue "NOVRO_DATABASE_NAME"
$userName = Get-RequiredEnvironmentValue "NOVRO_DATABASE_USER"
$password = Get-RequiredEnvironmentValue "NOVRO_DATABASE_PASSWORD"
$tlsText = Get-RequiredEnvironmentValue "NOVRO_DATABASE_TLS"

$port = 0
if (-not [int]::TryParse($portText, [ref]$port) -or $port -lt 1 -or $port -gt 65535) {
    throw "NOVRO_DATABASE_PORT must be between 1 and 65535"
}
if ($databaseName -notmatch '^[A-Za-z0-9_-]+$') {
    throw "NOVRO_DATABASE_NAME contains unsupported characters"
}
$sslMode = switch ($tlsText.ToLowerInvariant()) {
    "true" { "REQUIRED" }
    "false" { "DISABLED" }
    default { throw "NOVRO_DATABASE_TLS must be true or false" }
}

$executable = Resolve-NativeExecutable $MySQLDumpPath
$fullOutputPath = [IO.Path]::GetFullPath($OutputPath)
$outputDirectory = Split-Path -Parent $fullOutputPath
if ([string]::IsNullOrWhiteSpace($outputDirectory)) {
    $outputDirectory = (Get-Location).Path
}
if (-not (Test-Path -LiteralPath $outputDirectory -PathType Container)) {
    New-Item -ItemType Directory -Path $outputDirectory -Force | Out-Null
}
if ((Test-Path -LiteralPath $fullOutputPath) -and -not $Force) {
    throw "Backup already exists: $fullOutputPath. Pass -Force to replace it."
}

$partialPath = "$fullOutputPath.partial"
$checksumPath = "$fullOutputPath.sha256"
$optionFile = New-MySQLClientOptionFile $password
try {
    if (Test-Path -LiteralPath $partialPath) {
        Remove-Item -LiteralPath $partialPath -Force
    }
    $arguments = @(
        "--defaults-extra-file=$optionFile",
        "--protocol=TCP",
        "--host=$hostName",
        "--port=$port",
        "--user=$userName",
        "--ssl-mode=$sslMode",
        "--default-character-set=utf8mb4",
        "--single-transaction",
        "--quick",
        "--routines",
        "--events",
        "--triggers",
        "--hex-blob",
        "--no-tablespaces",
        "--set-gtid-purged=OFF",
        "--result-file=$partialPath",
        $databaseName
    )
    Invoke-NativeCommand -Executable $executable -Arguments $arguments
    if (-not (Test-Path -LiteralPath $partialPath -PathType Leaf) -or (Get-Item -LiteralPath $partialPath).Length -eq 0) {
        throw "mysqldump produced an empty backup"
    }
    Move-Item -LiteralPath $partialPath -Destination $fullOutputPath -Force
    $hash = (Get-FileHash -LiteralPath $fullOutputPath -Algorithm SHA256).Hash.ToLowerInvariant()
    [IO.File]::WriteAllText($checksumPath, "$hash  $([IO.Path]::GetFileName($fullOutputPath))`r`n", [Text.UTF8Encoding]::new($false))
    [pscustomobject]@{
        BackupPath = $fullOutputPath
        ChecksumPath = $checksumPath
        SHA256 = $hash
        Bytes = (Get-Item -LiteralPath $fullOutputPath).Length
    }
}
finally {
    if (Test-Path -LiteralPath $optionFile) {
        Remove-Item -LiteralPath $optionFile -Force
    }
    if (Test-Path -LiteralPath $partialPath) {
        Remove-Item -LiteralPath $partialPath -Force
    }
}
