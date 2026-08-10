[CmdletBinding()]
param(
    [ValidateSet('linux/amd64', 'linux/arm64')]
    [string]$Platform = 'linux/amd64',

    [string]$OutputDirectory = '',

    [string]$DockerPath = ''
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

$novroImage = 'novro:offline'
$mysqlImage = 'mysql:8.4'
$repositoryDirectory = (Resolve-Path (Join-Path $PSScriptRoot '..')).Path
if (-not $OutputDirectory) {
    $OutputDirectory = Join-Path $repositoryDirectory 'dist'
}

function Resolve-DockerExecutable {
    if ($DockerPath) {
        if (-not (Test-Path -LiteralPath $DockerPath -PathType Leaf)) {
            throw "Docker CLI does not exist: $DockerPath"
        }
        return (Resolve-Path -LiteralPath $DockerPath).Path
    }

    $command = Get-Command docker -ErrorAction SilentlyContinue
    if ($command) {
        return $command.Source
    }

    $candidates = @(
        (Join-Path $env:ProgramFiles 'Docker\Docker\resources\bin\docker.exe'),
        (Join-Path $env:LOCALAPPDATA 'Programs\DockerDesktop\resources\bin\docker.exe')
    )
    foreach ($candidate in $candidates) {
        if (Test-Path -LiteralPath $candidate -PathType Leaf) {
            return $candidate
        }
    }

    throw 'Docker CLI was not found. Install and start Docker Desktop first.'
}

$docker = Resolve-DockerExecutable

function Invoke-Docker {
    param([Parameter(ValueFromRemainingArguments = $true)][string[]]$Arguments)

    & $docker @Arguments
    if ($LASTEXITCODE -ne 0) {
        throw "docker $($Arguments -join ' ') failed with exit code $LASTEXITCODE"
    }
}

function Convert-ToTrimmedText {
    param($Value)

    if ($null -eq $Value) {
        return ''
    }
    if ($Value -is [array]) {
        return (($Value -join "`n").Trim())
    }
    return ([string]$Value).Trim()
}

function Get-DockerOperatingSystem {
    $output = Convert-ToTrimmedText (& $docker info --format '{{.OSType}}' 2>&1)
    if ($LASTEXITCODE -ne 0 -or -not $output) {
        throw "Docker Desktop engine is not ready: $output"
    }
    return $output
}

$dockerOs = Get-DockerOperatingSystem
if ($dockerOs -ne 'linux') {
    throw "Docker Desktop is using $dockerOs containers. Switch it to Linux containers."
}

Invoke-Docker buildx version

$sourceCommit = Convert-ToTrimmedText (& git -C $repositoryDirectory rev-parse --short=12 HEAD)
if ($LASTEXITCODE -ne 0 -or -not $sourceCommit) {
    $sourceCommit = 'unknown'
}
$sourceTreeDirty = [bool](& git -C $repositoryDirectory status --porcelain)
$architecture = $Platform.Split('/')[1]
$timestamp = Get-Date -Format 'yyyyMMdd-HHmmss'
$bundleName = "novro-offline-$architecture-$sourceCommit-$timestamp"
$outputRoot = [System.IO.Path]::GetFullPath($OutputDirectory)
$bundleDirectory = Join-Path $outputRoot $bundleName
$imageArchiveName = 'novro-images.tar'
$imageArchive = Join-Path $bundleDirectory $imageArchiveName

New-Item -ItemType Directory -Path $bundleDirectory -Force | Out-Null
New-Item -ItemType Directory -Path (Join-Path $bundleDirectory 'scripts') -Force | Out-Null
New-Item -ItemType Directory -Path (Join-Path $bundleDirectory 'deploy') -Force | Out-Null
New-Item -ItemType Directory -Path (Join-Path $bundleDirectory 'docs') -Force | Out-Null

Write-Host "Building $novroImage for $Platform..."
Invoke-Docker buildx build `
    --platform $Platform `
    --load `
    --tag $novroImage `
    --file (Join-Path $repositoryDirectory 'Dockerfile') `
    $repositoryDirectory

Write-Host "Pulling $mysqlImage for $Platform..."
Invoke-Docker pull --platform $Platform $mysqlImage

foreach ($image in @($novroImage, $mysqlImage)) {
    $imagePlatform = Convert-ToTrimmedText (& $docker image inspect --format '{{.Os}}/{{.Architecture}}' $image)
    if ($LASTEXITCODE -ne 0) {
        throw "Unable to inspect $image"
    }
    if ($imagePlatform -ne $Platform) {
        throw "Image $image has platform $imagePlatform, expected $Platform"
    }
}

$novroImageId = Convert-ToTrimmedText (& $docker image inspect --format '{{.Id}}' $novroImage)
$mysqlImageId = Convert-ToTrimmedText (& $docker image inspect --format '{{.Id}}' $mysqlImage)

Write-Host "Saving application and database images to $imageArchiveName..."
Invoke-Docker image save --output $imageArchive $novroImage $mysqlImage

Copy-Item -LiteralPath (Join-Path $repositoryDirectory 'compose.yaml') -Destination $bundleDirectory
Copy-Item -LiteralPath (Join-Path $repositoryDirectory 'compose.http.yaml') -Destination $bundleDirectory
Copy-Item -LiteralPath (Join-Path $repositoryDirectory 'Dockerfile') -Destination $bundleDirectory
Copy-Item -LiteralPath (Join-Path $repositoryDirectory '.dockerignore') -Destination $bundleDirectory
Copy-Item -LiteralPath (Join-Path $repositoryDirectory 'scripts\deploy-docker.sh') -Destination (Join-Path $bundleDirectory 'scripts')
Copy-Item -LiteralPath (Join-Path $repositoryDirectory 'deploy\docker.env.example') -Destination (Join-Path $bundleDirectory 'deploy')
Copy-Item -LiteralPath (Join-Path $repositoryDirectory 'deploy\docker-entrypoint.sh') -Destination (Join-Path $bundleDirectory 'deploy')
Copy-Item -LiteralPath (Join-Path $repositoryDirectory 'deploy\nginx.conf') -Destination (Join-Path $bundleDirectory 'deploy')
Copy-Item -LiteralPath (Join-Path $repositoryDirectory 'deploy\nginx.http.conf') -Destination (Join-Path $bundleDirectory 'deploy')
Copy-Item -LiteralPath (Join-Path $repositoryDirectory 'deploy\supervisord.conf') -Destination (Join-Path $bundleDirectory 'deploy')
Copy-Item -LiteralPath (Join-Path $repositoryDirectory 'docs\docker-deployment.md') -Destination (Join-Path $bundleDirectory 'docs')

$imageHash = (Get-FileHash -Algorithm SHA256 -LiteralPath $imageArchive).Hash.ToLowerInvariant()
"$imageHash  $imageArchiveName" | Set-Content -LiteralPath (Join-Path $bundleDirectory 'SHA256SUMS') -Encoding ascii

$manifest = @(
    "CreatedAt=$((Get-Date).ToString('o'))"
    "Platform=$Platform"
    "SourceCommit=$sourceCommit"
    "SourceTreeDirty=$($sourceTreeDirty.ToString().ToLowerInvariant())"
    "NovroImage=$novroImage"
    "NovroImageID=$novroImageId"
    "MySQLImage=$mysqlImage"
    "MySQLImageID=$mysqlImageId"
)
$manifest | Set-Content -LiteralPath (Join-Path $bundleDirectory 'manifest.txt') -Encoding ascii

$tarCommand = Get-Command tar.exe -ErrorAction SilentlyContinue
if (-not $tarCommand) {
    throw "Images were exported to $bundleDirectory, but tar.exe is unavailable to create the single-file bundle."
}

$bundleArchive = Join-Path $outputRoot "$bundleName.tar.gz"
& $tarCommand.Source -czf $bundleArchive -C $outputRoot $bundleName
if ($LASTEXITCODE -ne 0) {
    throw "Unable to create bundle archive: $bundleArchive"
}

$bundleHash = (Get-FileHash -Algorithm SHA256 -LiteralPath $bundleArchive).Hash.ToLowerInvariant()
$bundleHashFile = "$bundleArchive.sha256"
"$bundleHash  $([System.IO.Path]::GetFileName($bundleArchive))" | Set-Content -LiteralPath $bundleHashFile -Encoding ascii

Write-Host ''
Write-Host 'Offline bundle created:'
Write-Host "  $bundleArchive"
Write-Host "  $bundleHashFile"
Write-Host "Source tree dirty: $sourceTreeDirty"
Write-Host 'Transfer both files to the server, verify the SHA-256, extract the archive, then run deploy-docker.sh with --offline-images novro-images.tar.'
