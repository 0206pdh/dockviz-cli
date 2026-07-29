[CmdletBinding()]
param(
    [int]$DurationSeconds = 30,
    [int]$SampleIntervalSeconds = 2,
    [string]$RunLabel = "scenario",
    [string]$OutputDirectory = (Join-Path (Get-Location) "artifacts"),
    [string]$WorkloadImage = "busybox:1.36",
    [string]$TargetImage = "",
    [string]$TargetCommand = "",
    [ValidateRange(1, 64)]
    [int]$StorageSizeGB = 2,
    [ValidateRange(8, 64)]
    [int]$StorageReserveGB = 12,
    [int]$StorageReadyTimeoutSeconds = 300,
    [switch]$UseMaxSafeStorage,
    [switch]$Keep
)

$ErrorActionPreference = "Stop"

if ($DurationSeconds -lt 5) {
    throw "DurationSeconds must be at least 5."
}
if ($SampleIntervalSeconds -lt 1) {
    throw "SampleIntervalSeconds must be at least 1."
}
if ([string]::IsNullOrWhiteSpace($TargetImage) -xor [string]::IsNullOrWhiteSpace($TargetCommand)) {
    throw "TargetImage and TargetCommand must be provided together."
}
if ($StorageReadyTimeoutSeconds -lt 30) {
    throw "StorageReadyTimeoutSeconds must be at least 30."
}

$hostFreeGB = $null
if ($UseMaxSafeStorage) {
    $workspaceDriveName = ([IO.Path]::GetPathRoot((Get-Location).Path)).TrimEnd('\').TrimEnd(':')
    $workspaceDrive = Get-PSDrive -Name $workspaceDriveName -PSProvider FileSystem -ErrorAction Stop
    $hostFreeGB = [math]::Floor($workspaceDrive.Free / 1GB)
    $safeStorageGB = [int]($hostFreeGB - $StorageReserveGB)
    if ($safeStorageGB -lt 1) {
        throw "Not enough free space for a safe storage test: free=${hostFreeGB}GB reserve=${StorageReserveGB}GB."
    }
    $StorageSizeGB = [math]::Min($safeStorageGB, 64)
    Write-Host "Max-safe storage selected: ${StorageSizeGB}GB (host free=${hostFreeGB}GB, reserved=${StorageReserveGB}GB)"
}

$safeLabel = $RunLabel -replace "[^A-Za-z0-9_-]", "_"
$scenarioId = "dockviz-perf-$((Get-Date).ToUniversalTime().ToString("yyyyMMddHHmmss"))"
$outputPath = [IO.Path]::GetFullPath($OutputDirectory)
$null = New-Item -ItemType Directory -Path $outputPath -Force

$cpuName = "$scenarioId-cpu"
$memoryName = "$scenarioId-memory"
$logName = "$scenarioId-logs"
$crashName = "$scenarioId-restart"
$containerNames = @($cpuName, $memoryName, $logName, $crashName)
$targetName = "$scenarioId-target"
if (-not [string]::IsNullOrWhiteSpace($TargetImage)) {
    $containerNames += $targetName
}
$storageName = "$scenarioId-storage"
$volumeName = "$scenarioId-volume"
$allContainerNames = @($containerNames + $storageName)
$storageMegabytes = $StorageSizeGB * 1024
$storageStarted = $false
$volumeCreated = $false
$rows = [System.Collections.Generic.List[object]]::new()

function Invoke-DockerText {
    param([string[]]$DockerArguments)

    $result = & docker @DockerArguments 2>&1
    if ($LASTEXITCODE -ne 0) {
        throw "docker $($DockerArguments -join ' ') failed: $($result -join "`n")"
    }
    return ($result -join "`n")
}

function Convert-Percent {
    param([string]$Value)

    if ([string]::IsNullOrWhiteSpace($Value)) {
        return $null
    }
    return [math]::Round([double](($Value -replace "%", "").Trim()), 2)
}

function Convert-MemoryToMB {
    param([string]$Value)

    if ([string]::IsNullOrWhiteSpace($Value)) {
        return $null
    }

    $match = [regex]::Match($Value, "([0-9]+(?:\.[0-9]+)?)\s*([BKMG]i?B)")
    if (-not $match.Success) {
        return $null
    }

    $amount = [double]$match.Groups[1].Value
    switch ($match.Groups[2].Value.ToUpperInvariant()) {
        "B"   { return [math]::Round($amount / 1MB, 3) }
        "KB"  { return [math]::Round($amount / 1KB, 3) }
        "KIB" { return [math]::Round($amount / 1KB, 3) }
        "MB"  { return [math]::Round($amount, 3) }
        "MIB" { return [math]::Round($amount, 3) }
        "GB"  { return [math]::Round($amount * 1KB, 3) }
        "GIB" { return [math]::Round($amount * 1KB, 3) }
    }
    return $null
}

function Get-RestartCount {
    param([string]$Name)

    try {
        return [int](Invoke-DockerText @("inspect", "--format", "{{.RestartCount}}", $Name))
    } catch {
        return 0
    }
}

function Get-LogBytes {
    param([string]$Name)

    try {
        $logPath = Invoke-DockerText @("inspect", "--format", "{{.LogPath}}", $Name)
        if (-not [string]::IsNullOrWhiteSpace($logPath) -and (Test-Path -LiteralPath $logPath)) {
            return [int64](Get-Item -LiteralPath $logPath).Length
        }

        # Docker Desktop keeps the daemon log path inside its VM. Fall back to
        # the bytes returned by `docker logs`, which is portable and still
        # measures the pressure visible to the daemon.
        $logText = Invoke-DockerText @("logs", $Name)
        return [Text.Encoding]::UTF8.GetByteCount($logText)
    } catch {
        return -1
    }
}

function Get-Mean {
    param([object[]]$Values)

    $numbers = @($Values | Where-Object { $null -ne $_ } | ForEach-Object { [double]$_ })
    if ($numbers.Count -eq 0) {
        return $null
    }
    return [math]::Round(($numbers | Measure-Object -Average).Average, 2)
}

function Get-Percentile {
    param(
        [object[]]$Values,
        [double]$Percentile
    )

    $numbers = @($Values | Where-Object { $null -ne $_ } | ForEach-Object { [double]$_ } | Sort-Object)
    if ($numbers.Count -eq 0) {
        return $null
    }
    $index = [math]::Ceiling($Percentile * $numbers.Count) - 1
    $index = [math]::Max(0, [math]::Min($index, $numbers.Count - 1))
    return [math]::Round($numbers[$index], 2)
}

function Save-Text {
    param(
        [string]$Path,
        [string]$Content
    )
    Set-Content -LiteralPath $Path -Value $Content -Encoding utf8
}

Invoke-DockerText @("version", "--format", "{{.Server.Version}}") | Out-Null
foreach ($image in @($WorkloadImage, $TargetImage) | Where-Object { -not [string]::IsNullOrWhiteSpace($_) } | Select-Object -Unique) {
    try {
        Invoke-DockerText @("image", "inspect", $image) | Out-Null
    } catch {
        Write-Host "Pulling $image ..."
        Invoke-DockerText @("pull", $image) | Out-Null
    }
}

$beforeDfPath = Join-Path $outputPath "$safeLabel-$scenarioId-before-system-df.txt"
$workloadDfPath = Join-Path $outputPath "$safeLabel-$scenarioId-workload-system-df.txt"
$reclaimableDfPath = Join-Path $outputPath "$safeLabel-$scenarioId-reclaimable-system-df.txt"
$afterCleanupDfPath = Join-Path $outputPath "$safeLabel-$scenarioId-after-cleanup-system-df.txt"
$csvPath = Join-Path $outputPath "$safeLabel-$scenarioId-samples.csv"
$jsonPath = Join-Path $outputPath "$safeLabel-$scenarioId-summary.json"

Save-Text $beforeDfPath (Invoke-DockerText @("system", "df"))

try {
    Write-Host "Starting dockviz scenario $scenarioId"

    Invoke-DockerText @(
        "run", "-d", "--name", $cpuName,
        "--label", "dockviz.scenario=$scenarioId",
        $WorkloadImage, "sh", "-c", "while :; do :; done"
    ) | Out-Null

    Invoke-DockerText @(
        "run", "-d", "--name", $memoryName,
        "--label", "dockviz.scenario=$scenarioId",
        $WorkloadImage, "sh", "-c", "dd if=/dev/zero of=/tmp/dockviz-memory bs=1M count=64; sleep 600"
    ) | Out-Null

    Invoke-DockerText @(
        "run", "-d", "--name", $logName,
        "--label", "dockviz.scenario=$scenarioId",
        $WorkloadImage, "sh", "-c", "i=0; while [ `$i -lt 20000 ]; do echo dockviz-scenario-log-`$i; i=`$((i+1)); done; sleep 600"
    ) | Out-Null

    Invoke-DockerText @(
        "run", "-d", "--name", $crashName,
        "--restart", "on-failure:20",
        "--label", "dockviz.scenario=$scenarioId",
        $WorkloadImage, "sh", "-c", "sleep 2; exit 42"
    ) | Out-Null

    Invoke-DockerText @(
        "volume", "create",
        "--label", "dockviz.scenario=$scenarioId",
        $volumeName
    ) | Out-Null
    $volumeCreated = $true

    Invoke-DockerText @(
        "run", "-d", "--name", $storageName,
        "--label", "dockviz.scenario=$scenarioId",
        "--volume", "$volumeName`:/data",
        $WorkloadImage, "sh", "-c", "dd if=/dev/urandom of=/data/dockviz-payload.bin bs=1M count=$storageMegabytes; touch /data/.dockviz-ready; tail -f /dev/null"
    ) | Out-Null
    $storageStarted = $true

    $storageDeadline = [datetime]::UtcNow.AddSeconds($StorageReadyTimeoutSeconds)
    $storageReady = $false
    while ([datetime]::UtcNow -lt $storageDeadline) {
        try {
            & docker exec $storageName sh -c "test -f /data/.dockviz-ready" *> $null
            if ($LASTEXITCODE -eq 0) {
                $storageReady = $true
                break
            }
        } catch {
            # The writer may still be starting or Docker Desktop may be busy.
        }
        Start-Sleep -Seconds 2
    }
    if (-not $storageReady) {
        throw "The ${StorageSizeGB}GB storage workload did not finish within $StorageReadyTimeoutSeconds seconds."
    }

    if (-not [string]::IsNullOrWhiteSpace($TargetImage)) {
        Invoke-DockerText @(
            "run", "-d", "--name", $targetName,
            "--label", "dockviz.scenario=$scenarioId",
            $TargetImage, "sh", "-c", $TargetCommand
        ) | Out-Null
    }

    $startedAt = [datetime]::UtcNow
    $deadline = [datetime]::UtcNow.AddSeconds($DurationSeconds)
    $firstRestartAt = $null
    $thirdRestartAt = $null

    while ([datetime]::UtcNow -lt $deadline) {
        $elapsed = [math]::Round(([datetime]::UtcNow - $startedAt).TotalSeconds, 2)
        try {
            $statsArguments = @("stats", "--no-stream", "--format", "{{json .}}") + $containerNames
            $statsOutput = Invoke-DockerText $statsArguments
            $statsObjects = @($statsOutput -split "`r?`n" | Where-Object { -not [string]::IsNullOrWhiteSpace($_) } | ForEach-Object { $_ | ConvertFrom-Json })

            foreach ($stats in $statsObjects) {
                $containerName = [string]$stats.Name
                if ($containerNames -notcontains $containerName) {
                    continue
                }
                $restartCount = Get-RestartCount $containerName
                $rows.Add([pscustomobject]@{
                    timestamp_utc = [datetime]::UtcNow.ToString("o")
                    elapsed_sec = $elapsed
                    container = $containerName
                    cpu_pct = Convert-Percent $stats.CPUPerc
                    memory_mb = Convert-MemoryToMB (($stats.MemUsage -split "/")[0])
                    memory_pct = Convert-Percent $stats.MemPerc
                    pids = $stats.PIDs
                    restart_count = $restartCount
                })

                if ($containerName -eq $crashName) {
                    if ($restartCount -ge 1 -and $null -eq $firstRestartAt) {
                        $firstRestartAt = $elapsed
                    }
                    if ($restartCount -ge 3 -and $null -eq $thirdRestartAt) {
                        $thirdRestartAt = $elapsed
                    }
                }
            }
        } catch {
            # A short-lived restart container can disappear between stats and inspect.
        }
        Start-Sleep -Seconds $SampleIntervalSeconds
    }

    Save-Text $workloadDfPath (Invoke-DockerText @("system", "df"))

    if (-not $Keep) {
        Invoke-DockerText @("rm", "-f", $storageName) | Out-Null
        $storageStarted = $false
        Save-Text $reclaimableDfPath (Invoke-DockerText @("system", "df"))
    }

    $perContainer = @($containerNames | ForEach-Object {
        $currentContainerName = $_
        $containerRows = @($rows | Where-Object { $_.container -eq $currentContainerName })
        [pscustomobject]@{
            container = $currentContainerName
            samples = $containerRows.Count
            cpu_mean_pct = Get-Mean @($containerRows.cpu_pct)
            cpu_p95_pct = Get-Percentile @($containerRows.cpu_pct) 0.95
            memory_mean_mb = Get-Mean @($containerRows.memory_mb)
            memory_p95_mb = Get-Percentile @($containerRows.memory_mb) 0.95
            memory_p95_pct = Get-Percentile @($containerRows.memory_pct) 0.95
            restart_count = if ($containerRows.Count -gt 0) { ($containerRows.restart_count | Measure-Object -Maximum).Maximum } else { 0 }
        }
    })

    $logBytes = Get-LogBytes $logName
    $summary = [pscustomobject]@{
        schema_version = 1
        run_label = $RunLabel
        scenario_id = $scenarioId
        workload_image = $WorkloadImage
        target_image = if (-not [string]::IsNullOrWhiteSpace($TargetImage)) { $TargetImage } else { $null }
        target_command = if (-not [string]::IsNullOrWhiteSpace($TargetCommand)) { $TargetCommand } else { $null }
        storage_size_gb = $StorageSizeGB
        storage_auto_sized = $UseMaxSafeStorage.IsPresent
        host_free_gb_before = $hostFreeGB
        storage_reserve_gb = $StorageReserveGB
        storage_volume = $volumeName
        duration_seconds = $DurationSeconds
        sample_interval_seconds = $SampleIntervalSeconds
        sample_count = $rows.Count
        time_to_first_restart_sec = $firstRestartAt
        time_to_third_restart_sec = $thirdRestartAt
        log_bytes = $logBytes
        log_megabytes = if ($logBytes -ge 0) { [math]::Round($logBytes / 1MB, 2) } else { $null }
        containers = $perContainer
        before_system_df = $beforeDfPath
        workload_system_df = $workloadDfPath
        reclaimable_system_df = if (Test-Path -LiteralPath $reclaimableDfPath) { $reclaimableDfPath } else { $null }
        after_cleanup_system_df = $afterCleanupDfPath
        samples_csv = $csvPath
    }

    $rows | Export-Csv -LiteralPath $csvPath -NoTypeInformation -Encoding utf8
    $summary | ConvertTo-Json -Depth 6 | Set-Content -LiteralPath $jsonPath -Encoding utf8
    Write-Host "Summary: $jsonPath"
    Write-Host "Samples: $csvPath"
} finally {
    if (-not $Keep) {
        foreach ($containerName in $allContainerNames) {
            try {
                Invoke-DockerText @("rm", "-f", $containerName) | Out-Null
            } catch {
                # Cleanup is idempotent: the storage container is removed
                # before the reclaimable snapshot is taken.
            }
        }
        if ($volumeCreated) {
            try {
                Invoke-DockerText @("volume", "rm", $volumeName) | Out-Null
            } catch {
                # The volume may already have been removed by the user.
            }
        }
        Save-Text $afterCleanupDfPath (Invoke-DockerText @("system", "df"))
        Write-Host "Scenario containers removed. Cleanup snapshot: $afterCleanupDfPath"
    } else {
        Write-Host "Keeping scenario containers because -Keep was specified."
    }
}
