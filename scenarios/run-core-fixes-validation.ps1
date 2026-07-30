[CmdletBinding()]
param(
    [string]$OutputDirectory = (Join-Path (Get-Location) "artifacts\core-fixes-validation"),
    [ValidateRange(1, 32)]
    [int]$VolumeSizeGB = 6,
    [ValidateRange(128, 4096)]
    [int]$ImageLayerMB = 768,
    [ValidateRange(4, 48)]
    [int]$StatsContainerCount = 12,
    [ValidateRange(1, 20)]
    [int]$StatsRuns = 5,
    [string]$WorkloadImage = "busybox:1.36",
    [switch]$Keep
)

$ErrorActionPreference = "Stop"

$scenarioId = "dockviz-core-$((Get-Date).ToUniversalTime().ToString("yyyyMMddHHmmss"))"
$labelKey = "dockviz.validation"
$outputRoot = [IO.Path]::GetFullPath($OutputDirectory)
$null = New-Item -ItemType Directory -Path $outputRoot -Force

$volumeDir = Join-Path $outputRoot "01-volumes"
$vhdxDir = Join-Path $outputRoot "02-vhdx"
$imageDir = Join-Path $outputRoot "03-image-tags"
$statsDir = Join-Path $outputRoot "04-parallel-stats"
foreach ($dir in @($volumeDir, $vhdxDir, $imageDir, $statsDir)) {
    $null = New-Item -ItemType Directory -Path $dir -Force
}

$volumeNoAll = "$scenarioId-volume-no-all"
$volumeAll = "$scenarioId-volume-all"
$imageBase = "$scenarioId-image"
$imageTagKeep = "$imageBase`:keep"
$imageTagRemove = "$imageBase`:remove"
$statsPrefix = "$scenarioId-stats"
$statsContainers = 1..$StatsContainerCount | ForEach-Object { "$statsPrefix-$_" }

function Invoke-DockerText {
    param([string[]]$DockerArguments)

    $stderrPath = [IO.Path]::GetTempFileName()
    $previousErrorActionPreference = $ErrorActionPreference
    try {
        $ErrorActionPreference = "Continue"
        $result = & docker @DockerArguments 2> $stderrPath
        $exitCode = $LASTEXITCODE
    } finally {
        $ErrorActionPreference = $previousErrorActionPreference
    }
    $stderr = ""
    if (Test-Path -LiteralPath $stderrPath) {
        $stderr = Get-Content -Path $stderrPath -Raw
        Remove-Item -LiteralPath $stderrPath -Force
    }
    if ($null -eq $stderr) {
        $stderr = ""
    }
    $combined = @()
    if ($null -ne $result) {
        $combined += $result
    }
    if ($stderr.Trim() -ne "") {
        $combined += $stderr.TrimEnd()
    }
    if ($exitCode -ne 0) {
        throw "docker $($DockerArguments -join ' ') failed: $($combined -join "`n")"
    }
    return ($combined -join "`n")
}

function Invoke-DockerBestEffort {
    param([string[]]$DockerArguments)

    $previousErrorActionPreference = $ErrorActionPreference
    try {
        $ErrorActionPreference = "SilentlyContinue"
        & docker @DockerArguments *> $null
        return $LASTEXITCODE
    } catch {
        return 1
    } finally {
        $ErrorActionPreference = $previousErrorActionPreference
        $Error.Clear()
        $global:LASTEXITCODE = 0
    }
}

function Test-DockerObject {
    param([string[]]$DockerArguments)

    return (Invoke-DockerBestEffort $DockerArguments) -eq 0
}

function Save-Text {
    param([string]$Path, [string]$Value)
    $Value | Set-Content -Encoding UTF8 -Path $Path
}

function Save-SystemDf {
    param([string]$Path)
    Save-Text $Path (Invoke-DockerText @("system", "df", "-v"))
}

function Get-PSDriveSnapshot {
    $drive = Get-PSDrive -Name C -PSProvider FileSystem
    [pscustomobject]@{
        freeBytes = [int64]$drive.Free
        freeGB = [math]::Round($drive.Free / 1GB, 3)
        usedGB = [math]::Round($drive.Used / 1GB, 3)
    }
}

function Get-VHDXSnapshots {
    $candidates = @()
    if ($env:LOCALAPPDATA) {
        $candidates += Join-Path $env:LOCALAPPDATA "Docker\wsl\disk\docker_data.vhdx"
        $candidates += Join-Path $env:LOCALAPPDATA "Docker\wsl\data\ext4.vhdx"
    }
    if ($env:USERPROFILE) {
        $candidates += Join-Path $env:USERPROFILE "AppData\Local\Docker\wsl\disk\docker_data.vhdx"
        $candidates += Join-Path $env:USERPROFILE "AppData\Local\Docker\wsl\data\ext4.vhdx"
    }
    foreach ($path in ($candidates | Select-Object -Unique)) {
        if (Test-Path -LiteralPath $path) {
            $item = Get-Item -LiteralPath $path
            [pscustomobject]@{
                path = $item.FullName
                sizeBytes = [int64]$item.Length
                sizeGB = [math]::Round($item.Length / 1GB, 3)
            }
        }
    }
}

function Cleanup-Scenario {
    if ($Keep) {
        return
    }
    foreach ($name in $statsContainers) {
        Invoke-DockerBestEffort @("rm", "-f", $name) | Out-Null
    }
    foreach ($volume in @($volumeNoAll, $volumeAll)) {
        Invoke-DockerBestEffort @("volume", "rm", "-f", $volume) | Out-Null
    }
    foreach ($tag in @($imageTagRemove, $imageTagKeep)) {
        Invoke-DockerBestEffort @("image", "rm", "-f", $tag) | Out-Null
    }
    Invoke-DockerBestEffort @("builder", "prune", "--filter", "label=$labelKey=$scenarioId", "--force") | Out-Null
}

function Run-VolumeValidation {
    Invoke-DockerText @("volume", "create", "--label", "$labelKey=$scenarioId", $volumeNoAll) | Out-Null
    Invoke-DockerText @("volume", "create", "--label", "$labelKey=$scenarioId", $volumeAll) | Out-Null

    $countMB = $VolumeSizeGB * 1024
    Invoke-DockerText @(
        "run", "--rm",
        "--label", "$labelKey=$scenarioId",
        "-v", "${volumeNoAll}:/data",
        $WorkloadImage,
        "sh", "-c", "dd if=/dev/zero of=/data/payload.bin bs=1M count=$countMB"
    ) | Out-Null
    Invoke-DockerText @(
        "run", "--rm",
        "--label", "$labelKey=$scenarioId",
        "-v", "${volumeAll}:/data",
        $WorkloadImage,
        "sh", "-c", "dd if=/dev/zero of=/data/payload.bin bs=1M count=$countMB"
    ) | Out-Null

    Save-SystemDf (Join-Path $volumeDir "before-system-df.txt")
    Save-Text (Join-Path $volumeDir "before-volume-ls.txt") (Invoke-DockerText @("volume", "ls", "--filter", "label=$labelKey=$scenarioId"))
    Save-Text (Join-Path $volumeDir "before-volume-inspect.json") (Invoke-DockerText @("volume", "inspect", $volumeNoAll, $volumeAll))

    $withoutAll = Invoke-DockerText @("volume", "prune", "--force", "--filter", "label=$labelKey=$scenarioId")
    Save-Text (Join-Path $volumeDir "prune-without-all-output.txt") $withoutAll
    $noAllStillExists = Test-DockerObject @("volume", "inspect", $volumeNoAll)

    $withAll = Invoke-DockerText @("volume", "prune", "--all", "--force", "--filter", "label=$labelKey=$scenarioId")
    Save-Text (Join-Path $volumeDir "prune-with-all-output.txt") $withAll
    $allStillExists = Test-DockerObject @("volume", "inspect", $volumeAll)

    Save-SystemDf (Join-Path $volumeDir "after-system-df.txt")
    Save-Text (Join-Path $volumeDir "after-volume-ls.txt") (Invoke-DockerText @("volume", "ls", "--filter", "label=$labelKey=$scenarioId"))

    return [ordered]@{
        volumeSizeGB = $VolumeSizeGB
        noAllVolumeStillExistsAfterPrune = [bool]$noAllStillExists
        allVolumeStillExistsAfterPrune = [bool]$allStillExists
        pruneWithoutAllOutput = $withoutAll
        pruneWithAllOutput = $withAll
    }
}

function Run-VHDXSnapshot {
    $before = Get-PSDriveSnapshot
    $vhdx = @(Get-VHDXSnapshots)
    Save-SystemDf (Join-Path $vhdxDir "docker-system-df.txt")
    $before | ConvertTo-Json -Depth 4 | Set-Content -Encoding UTF8 -Path (Join-Path $vhdxDir "host-free.json")
    $vhdx | ConvertTo-Json -Depth 4 | Set-Content -Encoding UTF8 -Path (Join-Path $vhdxDir "vhdx-sizes.json")
    Save-Text (Join-Path $vhdxDir "manual-compact-note.txt") "VHDX compaction is intentionally not automated by this validation script. Record before/after size and host free space here if you compact Docker Desktop manually."
    return [ordered]@{
        hostFreeGB = $before.freeGB
        vhdx = $vhdx
    }
}

function Run-ImageTagValidation {
    $tmp = Join-Path $imageDir "build-context"
    $null = New-Item -ItemType Directory -Path $tmp -Force
    @"
FROM busybox:1.36
ARG SCENARIO_ID
RUN dd if=/dev/zero of=/dockviz-image-payload.bin bs=1M count=$ImageLayerMB
RUN echo "`$SCENARIO_ID" > /dockviz-image-scenario.txt
CMD ["sleep", "600"]
"@ | Set-Content -Encoding UTF8 -Path (Join-Path $tmp "Dockerfile")

    Save-SystemDf (Join-Path $imageDir "before-system-df.txt")
    $buildOutput = Invoke-DockerText @("build", "--no-cache", "--build-arg", "SCENARIO_ID=$scenarioId", "--label", "$labelKey=$scenarioId", "-t", $imageTagKeep, $tmp)
    Save-Text (Join-Path $imageDir "build-output.txt") $buildOutput
    Invoke-DockerText @("tag", $imageTagKeep, $imageTagRemove) | Out-Null

    Save-Text (Join-Path $imageDir "before-image-ls.txt") (Invoke-DockerText @("image", "ls", $imageBase))
    Save-Text (Join-Path $imageDir "before-image-inspect.json") (Invoke-DockerText @("image", "inspect", $imageTagKeep, $imageTagRemove))

    $removeOutput = Invoke-DockerText @("image", "rm", $imageTagRemove)
    Save-Text (Join-Path $imageDir "remove-selected-tag-output.txt") $removeOutput

    $keepExists = Test-DockerObject @("image", "inspect", $imageTagKeep)
    $removeExists = Test-DockerObject @("image", "inspect", $imageTagRemove)

    Save-Text (Join-Path $imageDir "after-image-ls.txt") (Invoke-DockerText @("image", "ls", $imageBase))
    Save-SystemDf (Join-Path $imageDir "after-system-df.txt")

    return [ordered]@{
        imageLayerMB = $ImageLayerMB
        keptTagExistsAfterRemove = [bool]$keepExists
        removedTagExistsAfterRemove = [bool]$removeExists
        removeOutput = $removeOutput
    }
}

function Measure-DockerStats {
    param([string[]]$Names)

    $csvPath = Join-Path $statsDir "stats-runs.csv"
    $jsonPath = Join-Path $statsDir "stats-summary.json"
    $previousErrorActionPreference = $ErrorActionPreference
    try {
        $ErrorActionPreference = "Continue"
        $goOutput = & go run .\scenarios\stats_parallel_benchmark.go `
            -containers ($Names -join ",") `
            -runs $StatsRuns `
            -csv $csvPath `
            -json $jsonPath 2>&1
        $exitCode = $LASTEXITCODE
    } finally {
        $ErrorActionPreference = $previousErrorActionPreference
    }
    Save-Text (Join-Path $statsDir "benchmark-output.json") ($goOutput -join "`n")
    if ($exitCode -ne 0) {
        throw "go stats benchmark failed: $($goOutput -join "`n")"
    }
    return Get-Content -Path $jsonPath -Raw | ConvertFrom-Json
}

function Run-StatsValidation {
    foreach ($name in $statsContainers) {
        Invoke-DockerText @("run", "-d", "--name", $name, "--label", "$labelKey=$scenarioId", $WorkloadImage, "sleep", "600") | Out-Null
    }
    Save-Text (Join-Path $statsDir "containers.txt") ($statsContainers -join "`n")

    $benchmark = Measure-DockerStats -Names $statsContainers
    return [ordered]@{
        containerCount = $StatsContainerCount
        runs = $StatsRuns
        sequentialAvgSeconds = $benchmark.sequentialAvgSeconds
        parallelAvgSeconds = $benchmark.parallelAvgSeconds
        speedup = $benchmark.speedup
        csv = (Join-Path $statsDir "stats-runs.csv")
        json = (Join-Path $statsDir "stats-summary.json")
    }
}

try {
    Invoke-DockerText @("version", "--format", "{{.Server.Version}}") | Out-Null
    Invoke-DockerText @("pull", $WorkloadImage) | Out-Null

    $hostBefore = Get-PSDriveSnapshot
    $volumeResult = Run-VolumeValidation
    $vhdxResult = Run-VHDXSnapshot
    $imageResult = Run-ImageTagValidation
    $statsResult = Run-StatsValidation
    $hostAfterWorkload = Get-PSDriveSnapshot

    $summary = [ordered]@{
        scenarioId = $scenarioId
        createdAt = (Get-Date).ToUniversalTime().ToString("o")
        outputRoot = $outputRoot
        dockerServerVersion = (Invoke-DockerText @("version", "--format", "{{.Server.Version}}"))
        hostBefore = $hostBefore
        hostAfterWorkload = $hostAfterWorkload
        volumeValidation = $volumeResult
        vhdxSnapshot = $vhdxResult
        imageTagValidation = $imageResult
        parallelStatsValidation = $statsResult
    }
    $summaryPath = Join-Path $outputRoot "summary.json"
    $summary | ConvertTo-Json -Depth 8 | Set-Content -Encoding UTF8 -Path $summaryPath

    Write-Host "Core fixes validation complete."
    Write-Host "Scenario: $scenarioId"
    Write-Host "Summary: $summaryPath"
    Write-Host "Output: $outputRoot"
} finally {
    Cleanup-Scenario
    Save-SystemDf (Join-Path $outputRoot "after-cleanup-system-df.txt")
    (Get-PSDriveSnapshot) | ConvertTo-Json -Depth 4 | Set-Content -Encoding UTF8 -Path (Join-Path $outputRoot "after-cleanup-host-free.json")
}
