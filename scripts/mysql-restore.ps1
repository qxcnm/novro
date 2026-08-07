[CmdletBinding()]
param(
    [Parameter(Mandatory = $true)]
    [ValidateScript({ Test-Path -LiteralPath $_ -PathType Leaf })]
    [string]$BackupPath,

    [Parameter(Mandatory = $true)]
    [ValidateNotNullOrEmpty()]
    [string]$TargetDatabase,

    [string]$MySQLPath = "mysql",

    [switch]$AllowNonTestTarget,

    [switch]$AllowMissingBackupChecksum,

    [switch]$CompareSourceRowCounts
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
        throw "Cannot find $Value. Install the MySQL 8.4 client or pass -MySQLPath."
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

function Invoke-MySQLFile {
    param(
        [Parameter(Mandatory = $true)][string]$Executable,
        [Parameter(Mandatory = $true)][string[]]$BaseArguments,
        [Parameter(Mandatory = $true)][string]$InputPath
    )

    $startInfo = [Diagnostics.ProcessStartInfo]::new()
    $startInfo.FileName = $Executable
    $startInfo.UseShellExecute = $false
    $startInfo.CreateNoWindow = $true
    $startInfo.RedirectStandardInput = $true
    $startInfo.RedirectStandardOutput = $true
    $startInfo.RedirectStandardError = $true
    foreach ($argument in $BaseArguments) {
        $startInfo.ArgumentList.Add($argument)
    }

    $process = [Diagnostics.Process]::new()
    $process.StartInfo = $startInfo
    try {
        if (-not $process.Start()) {
            throw "mysql did not start"
        }
        $standardOutput = $process.StandardOutput.ReadToEndAsync()
        $standardError = $process.StandardError.ReadToEndAsync()
        $input = [IO.File]::OpenRead($InputPath)
        try {
            $input.CopyTo($process.StandardInput.BaseStream)
        }
        finally {
            $input.Dispose()
            $process.StandardInput.Close()
        }
        $process.WaitForExit()
        $output = $standardOutput.GetAwaiter().GetResult()
        $errorOutput = $standardError.GetAwaiter().GetResult().Trim()
        if ($process.ExitCode -ne 0) {
            if ($errorOutput.Length -gt 600) {
                $errorOutput = $errorOutput.Substring(0, 600)
            }
            $suffix = if ($errorOutput) { ": $errorOutput" } else { "" }
            throw "mysql failed with exit code $($process.ExitCode)$suffix"
        }
        return $output
    }
    finally {
        $process.Dispose()
    }
}

$hostName = Get-RequiredEnvironmentValue "NOVRO_DATABASE_HOST"
$portText = Get-RequiredEnvironmentValue "NOVRO_DATABASE_PORT"
$sourceDatabase = Get-RequiredEnvironmentValue "NOVRO_DATABASE_NAME"
$userName = Get-RequiredEnvironmentValue "NOVRO_DATABASE_USER"
$password = Get-RequiredEnvironmentValue "NOVRO_DATABASE_PASSWORD"
$tlsText = Get-RequiredEnvironmentValue "NOVRO_DATABASE_TLS"

$port = 0
if (-not [int]::TryParse($portText, [ref]$port) -or $port -lt 1 -or $port -gt 65535) {
    throw "NOVRO_DATABASE_PORT must be between 1 and 65535"
}
if ($TargetDatabase -notmatch '^[A-Za-z0-9_]+$') {
    throw "TargetDatabase contains unsupported characters"
}
if ($sourceDatabase -notmatch '^[A-Za-z0-9_-]+$') {
    throw "NOVRO_DATABASE_NAME contains unsupported characters"
}
if ($TargetDatabase -ieq $sourceDatabase) {
    throw "Refusing to restore over NOVRO_DATABASE_NAME. Restore into a separate database and switch only after verification."
}
if (-not $AllowNonTestTarget -and $TargetDatabase -notmatch '^novro_restore_[A-Za-z0-9_]+$') {
    throw "TargetDatabase must start with novro_restore_. Pass -AllowNonTestTarget only for a reviewed cutover database."
}
$sslMode = switch ($tlsText.ToLowerInvariant()) {
    "true" { "REQUIRED" }
    "false" { "DISABLED" }
    default { throw "NOVRO_DATABASE_TLS must be true or false" }
}

$fullBackupPath = (Resolve-Path -LiteralPath $BackupPath).Path
$checksumPath = "$fullBackupPath.sha256"
$backupChecksumVerified = $false
if (Test-Path -LiteralPath $checksumPath -PathType Leaf) {
    $expectedHash = ((Get-Content -LiteralPath $checksumPath -Raw).Trim() -split '\s+')[0].ToLowerInvariant()
    if ($expectedHash -notmatch '^[a-f0-9]{64}$') {
        throw "Backup checksum file is invalid: $checksumPath"
    }
    $actualHash = (Get-FileHash -LiteralPath $fullBackupPath -Algorithm SHA256).Hash.ToLowerInvariant()
    if ($expectedHash -ne $actualHash) {
        throw "Backup checksum does not match $checksumPath"
    }
    $backupChecksumVerified = $true
}
elseif (-not $AllowMissingBackupChecksum) {
    throw "Backup checksum file is required: $checksumPath. Pass -AllowMissingBackupChecksum only for a reviewed disaster recovery."
}

$executable = Resolve-NativeExecutable $MySQLPath
$migrationDirectory = Join-Path (Split-Path -Parent $PSScriptRoot) "ent\migrate\migrations"
$expectedMigrations = @(
    Get-ChildItem -LiteralPath $migrationDirectory -Filter "*.sql" -File |
        Sort-Object Name |
        ForEach-Object {
            [pscustomobject]@{
                Version = $_.BaseName
                Checksum = (Get-FileHash -LiteralPath $_.FullName -Algorithm SHA256).Hash.ToLowerInvariant()
            }
        }
)
if ($expectedMigrations.Count -eq 0) {
    throw "No versioned migrations were found in $migrationDirectory"
}
$tablesIntroducedByMigration = @{
    "0001_users_and_sessions" = @("user_sessions", "users")
    "0002_registration_oidc_and_setup" = @("system_settings", "user_identities")
    "0003_api_keys" = @("api_keys")
    "0004_providers" = @("providers")
    "0005_wallets_model_routes_and_usage" = @("api_usages", "model_routes", "wallet_entries", "wallets")
    "0006_idempotent_wallet_entries" = @()
    "0007_upstream_models_billing_groups_and_precise_usage" = @("billing_groups", "upstream_models")
    "0008_model_catalog_and_provider_routes" = @()
    "0009_seed_popular_model_catalog" = @()
}
foreach ($migration in $expectedMigrations) {
    if (-not $tablesIntroducedByMigration.ContainsKey($migration.Version)) {
        throw "Restore table manifest is missing migration $($migration.Version)"
    }
}
$optionFile = New-MySQLClientOptionFile $password
$preflightFile = Join-Path ([IO.Path]::GetTempPath()) ("novro-preflight-" + [guid]::NewGuid().ToString("N") + ".sql")
$createFile = Join-Path ([IO.Path]::GetTempPath()) ("novro-create-" + [guid]::NewGuid().ToString("N") + ".sql")
$verifyFile = Join-Path ([IO.Path]::GetTempPath()) ("novro-verify-" + [guid]::NewGuid().ToString("N") + ".sql")
try {
    $baseArguments = @(
        "--defaults-extra-file=$optionFile",
        "--protocol=TCP",
        "--host=$hostName",
        "--port=$port",
        "--user=$userName",
        "--connect-timeout=10",
        "--ssl-mode=$sslMode",
        "--default-character-set=utf8mb4"
    )

    [IO.File]::WriteAllText(
        $preflightFile,
        "SELECT COUNT(*) FROM information_schema.schemata WHERE schema_name = '$TargetDatabase';`r`n",
        [Text.UTF8Encoding]::new($false)
    )
    $queryArguments = $baseArguments + @("--batch", "--skip-column-names")
    $databaseExists = (Invoke-MySQLFile -Executable $executable -BaseArguments $queryArguments -InputPath $preflightFile).Trim()
    if ($databaseExists -ne "0") {
        throw "Refusing to restore into existing database: $TargetDatabase"
    }

    [IO.File]::WriteAllText($createFile, "CREATE DATABASE ``$TargetDatabase`` CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci;`r`n", [Text.UTF8Encoding]::new($false))
    $null = Invoke-MySQLFile -Executable $executable -BaseArguments $baseArguments -InputPath $createFile

    $restoreArguments = $baseArguments + @("--database=$TargetDatabase", "--binary-mode")
    $null = Invoke-MySQLFile -Executable $executable -BaseArguments $restoreArguments -InputPath $fullBackupPath

    [IO.File]::WriteAllText(
        $verifyFile,
        "SELECT table_name FROM information_schema.tables WHERE table_schema = '$TargetDatabase' AND table_type = 'BASE TABLE' ORDER BY table_name;`r`nSELECT CONCAT('migration:', version) FROM ``$TargetDatabase``.novro_schema_migrations ORDER BY version;`r`nSELECT CONCAT('checksum-column:', COUNT(*)) FROM information_schema.columns WHERE table_schema = '$TargetDatabase' AND table_name = 'novro_schema_migrations' AND column_name = 'checksum';`r`n",
        [Text.UTF8Encoding]::new($false)
    )
    $verifyOutput = Invoke-MySQLFile -Executable $executable -BaseArguments $queryArguments -InputPath $verifyFile
    $values = @($verifyOutput -split "`r?`n" | Where-Object { -not [string]::IsNullOrWhiteSpace($_) })
    $actualTables = @($values | Where-Object { -not $_.StartsWith("migration:") -and -not $_.StartsWith("checksum-column:") })
    $actualMigrations = @($values | Where-Object { $_.StartsWith("migration:") } | ForEach-Object { $_.Substring(10) })
    $checksumColumnValues = @($values | Where-Object { $_.StartsWith("checksum-column:") } | ForEach-Object { $_.Substring(16) })
    if ($actualMigrations.Count -eq 0) {
        throw "Restore verification failed: restored database has no migration records"
    }
    if ($actualMigrations.Count -gt $expectedMigrations.Count) {
        throw "Restore verification failed: restored database contains migrations missing from the repository"
    }
    for ($index = 0; $index -lt $actualMigrations.Count; $index++) {
        if ($actualMigrations[$index] -ne $expectedMigrations[$index].Version) {
            throw "Restore verification failed: restored migrations are not a contiguous repository prefix"
        }
    }
    if ($checksumColumnValues.Count -ne 1 -or ($checksumColumnValues[0] -ne "0" -and $checksumColumnValues[0] -ne "1")) {
        throw "Restore verification failed: migration checksum metadata is invalid"
    }

    $expectedTables = @("novro_schema_migrations")
    foreach ($version in $actualMigrations) {
        $expectedTables += $tablesIntroducedByMigration[$version]
    }
    $expectedTables = @($expectedTables | Sort-Object -Unique)
    if (($actualTables -join "`n") -ne ($expectedTables -join "`n")) {
        throw "Restore verification failed: restored table set does not match its migration history"
    }

    $migrationChecksumsVerified = $false
    if ($checksumColumnValues[0] -eq "1") {
        [IO.File]::WriteAllText(
            $verifyFile,
            "SELECT version, checksum FROM ``$TargetDatabase``.novro_schema_migrations ORDER BY version;`r`n",
            [Text.UTF8Encoding]::new($false)
        )
        $migrationChecksumOutput = Invoke-MySQLFile -Executable $executable -BaseArguments $queryArguments -InputPath $verifyFile
        $migrationChecksumLines = @($migrationChecksumOutput -split "`r?`n" | Where-Object { -not [string]::IsNullOrWhiteSpace($_) })
        if ($migrationChecksumLines.Count -ne $actualMigrations.Count) {
            throw "Restore verification failed: migration checksum count does not match migration history"
        }
        for ($index = 0; $index -lt $migrationChecksumLines.Count; $index++) {
            $parts = @($migrationChecksumLines[$index] -split "`t")
            if ($parts.Count -ne 2 -or $parts[0] -ne $expectedMigrations[$index].Version -or $parts[1] -notmatch '^[a-fA-F0-9]{64}$' -or $parts[1] -ine $expectedMigrations[$index].Checksum) {
                throw "Restore verification failed: migration checksum does not match the repository for $($actualMigrations[$index])"
            }
        }
        $migrationChecksumsVerified = $true
    }

    $comparedTableCount = 0
    if ($CompareSourceRowCounts) {
        $rowCountStatements = foreach ($table in $expectedTables) {
            "SELECT '$table', (SELECT COUNT(*) FROM ``$sourceDatabase``.``$table``), (SELECT COUNT(*) FROM ``$TargetDatabase``.``$table``);"
        }
        [IO.File]::WriteAllText($verifyFile, ($rowCountStatements -join "`r`n") + "`r`n", [Text.UTF8Encoding]::new($false))
        $rowCountOutput = Invoke-MySQLFile -Executable $executable -BaseArguments $queryArguments -InputPath $verifyFile
        $rowCountLines = @($rowCountOutput -split "`r?`n" | Where-Object { -not [string]::IsNullOrWhiteSpace($_) })
        if ($rowCountLines.Count -ne $expectedTables.Count) {
            throw "Restore row-count comparison returned an unexpected result"
        }
        foreach ($line in $rowCountLines) {
            $parts = @($line -split "`t")
            if ($parts.Count -ne 3 -or $parts[1] -ne $parts[2]) {
                $table = if ($parts.Count -gt 0) { $parts[0] } else { "unknown" }
                throw "Restore row-count comparison failed for table $table"
            }
        }
        $comparedTableCount = $rowCountLines.Count
    }
    [pscustomobject]@{
        RestoredDatabase = $TargetDatabase
        TableCount = $actualTables.Count
        MigrationCount = $actualMigrations.Count
        PendingMigrationCount = $expectedMigrations.Count - $actualMigrations.Count
        ComparedTableCount = $comparedTableCount
        ChecksumVerified = $backupChecksumVerified
        BackupChecksumVerified = $backupChecksumVerified
        MigrationChecksumsVerified = $migrationChecksumsVerified
    }
}
finally {
    foreach ($path in @($optionFile, $preflightFile, $createFile, $verifyFile)) {
        if (Test-Path -LiteralPath $path) {
            Remove-Item -LiteralPath $path -Force
        }
    }
}
