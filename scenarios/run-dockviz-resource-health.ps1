[CmdletBinding()]
param(
    [int]$DurationSeconds = 30,
    [int]$SampleIntervalSeconds = 2,
    [string]$RunLabel = "resource-health",
    [string]$OutputDirectory = (Join-Path (Get-Location) "artifacts"),
    [string]$WorkloadImage = "busybox:1.36",
    [switch]$Keep
)

$ErrorActionPreference = "Stop"

if ($DurationSeconds -lt 30) {
    throw "DurationSeconds must be at least 30."
}
if ($SampleIntervalSeconds -lt 1) {
    throw "SampleIntervalSeconds must be at least 1."
}

$safeLabel = $RunLabel -replace "[^A-Za-z0-9_-]", "_"
$scenarioId = "dockviz-health-$((Get-Date).ToUniversalTime().ToString("yyyyMMddHHmmss"))"
$project = "${safeLabel}-${scenarioId}"
$outputPath = [IO.Path]::GetFullPath($OutputDirectory)
$null = New-Item -ItemType Directory -Path $outputPath -Force

$cpuName = "$scenarioId-cpu-hog"
$memPressureName = "$scenarioId-mem-pressure"
$memGrowthName = "$scenarioId-mem-growth"
$noLimitName = "$scenarioId-no-limit"
$containerNames = @($cpuName, $memPressureName, $memGrowthName, $noLimitName)
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

function Start-HealthContainer {
    param(
        [string]$Name,
        [string]$Service,
        [string[]]$ExtraArguments,
        [string]$Command
    )

    $args = @(
        "run", "-d", "--name", $Name,
        "--label", "dockviz.scenario=$scenarioId",
        "--label", "com.docker.compose.project=$project",
        "--label", "com.docker.compose.service=$Service"
    ) + $ExtraArguments + @($WorkloadImage, "sh", "-c", $Command)

    Invoke-DockerText $args | Out-Null
}

function Get-ContainerSamples {
    $args = @("stats", "--no-stream", "--format", "{{.Name}},{{.CPUPerc}},{{.MemUsage}}") + $containerNames
    $output = Invoke-DockerText $args
    foreach ($line in ($output -split "`n")) {
        if ([string]::IsNullOrWhiteSpace($line)) {
            continue
        }
        $parts = $line.Split(",", 3)
        $memUsage = $parts[2].Split("/", 2)[0].Trim()
        [pscustomobject]@{
            Name = $parts[0]
            CPUPercent = Convert-Percent $parts[1]
            MemoryMB = Convert-MemoryToMB $memUsage
        }
    }
}

function Stop-Scenario {
    if ($Keep) {
        return
    }
    foreach ($name in $containerNames) {
        docker rm -f $name 2>$null | Out-Null
    }
}

try {
    Invoke-DockerText @("pull", $WorkloadImage) | Out-Null

    Start-HealthContainer `
        -Name $cpuName `
        -Service "cpu-hog" `
        -ExtraArguments @("--memory", "128m", "--memory-swap", "128m") `
        -Command "while true; do :; done"

    Start-HealthContainer `
        -Name $memPressureName `
        -Service "mem-pressure" `
        -ExtraArguments @("--memory", "64m", "--memory-swap", "64m", "--tmpfs", "/dev/shm:rw,size=64m") `
        -Command "dd if=/dev/zero of=/dev/shm/fill bs=1M count=56; sleep 600"

    Start-HealthContainer `
        -Name $memGrowthName `
        -Service "mem-growth" `
        -ExtraArguments @("--memory", "96m", "--memory-swap", "96m", "--tmpfs", "/dev/shm:rw,size=96m") `
        -Command "i=0; while [ `$i -lt 10 ]; do dd if=/dev/zero of=/dev/shm/grow`$i bs=1M count=8; i=`$((i+1)); sleep 2; done; sleep 600"

    Start-HealthContainer `
        -Name $noLimitName `
        -Service "no-limit" `
        -ExtraArguments @() `
        -Command "sleep 600"

    $deadline = (Get-Date).AddSeconds($DurationSeconds)
    while ((Get-Date) -lt $deadline) {
        try {
            foreach ($sample in Get-ContainerSamples) {
                $rows.Add([pscustomobject]@{
                    Timestamp = (Get-Date).ToUniversalTime().ToString("o")
                    ScenarioId = $scenarioId
                    Project = $project
                    Container = $sample.Name
                    CPUPercent = $sample.CPUPercent
                    MemoryMB = $sample.MemoryMB
                })
            }
        } catch {
            foreach ($name in $containerNames) {
                $rows.Add([pscustomobject]@{
                    Timestamp = (Get-Date).ToUniversalTime().ToString("o")
                    ScenarioId = $scenarioId
                    Project = $project
                    Container = $name
                    CPUPercent = $null
                    MemoryMB = $null
                })
            }
        }
        Start-Sleep -Seconds $SampleIntervalSeconds
    }

    $csvPath = Join-Path $outputPath "$scenarioId-samples.csv"
    $summaryPath = Join-Path $outputPath "$scenarioId-summary.json"
    $rows | Export-Csv -NoTypeInformation -Encoding UTF8 -Path $csvPath

    $summary = [ordered]@{
        scenarioId = $scenarioId
        project = $project
        durationSeconds = $DurationSeconds
        sampleIntervalSeconds = $SampleIntervalSeconds
        containers = $containerNames
        expectedDockvizProblems = @(
            "High CPU on $cpuName",
            "Memory pressure on $memPressureName",
            "Memory growth on $memGrowthName",
            "No resource limits on $noLimitName"
        )
        expectedDockvizPanels = @(
            "Containers: project resource summary for $project",
            "Problems: resource-derived warnings",
            "Problem detail: read-only recommendations with [Enter]"
        )
        outputFiles = @{
            samplesCsv = $csvPath
            summaryJson = $summaryPath
        }
    }
    $summary | ConvertTo-Json -Depth 5 | Set-Content -Encoding UTF8 -Path $summaryPath

    Write-Host "Resource health scenario complete."
    Write-Host "Samples: $csvPath"
    Write-Host "Summary: $summaryPath"
    Write-Host "Expected dockviz project: $project"
} finally {
    Stop-Scenario
}
