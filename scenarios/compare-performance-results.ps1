[CmdletBinding()]
param(
    [Parameter(Mandatory = $true)]
    [string]$Baseline,

    [Parameter(Mandatory = $true)]
    [string]$Improved
)

$ErrorActionPreference = "Stop"

function Read-Summary {
    param([string]$Path)
    if (-not (Test-Path -LiteralPath $Path)) {
        throw "Summary file not found: $Path"
    }
    return Get-Content -LiteralPath $Path -Raw | ConvertFrom-Json
}

function Find-ContainerSummary {
    param(
        $Summary,
        [string]$Suffix
    )
    return @($Summary.containers | Where-Object { $_.container -like "*$Suffix" })[0]
}

function Format-Delta {
    param(
        [double]$Before,
        [double]$After
    )
    if ($Before -eq 0) {
        return $null
    }
    return [math]::Round((($After - $Before) / $Before) * 100, 2)
}

$baselineSummary = Read-Summary $Baseline
$improvedSummary = Read-Summary $Improved
$baselineTarget = Find-ContainerSummary $baselineSummary "-target"
$improvedTarget = Find-ContainerSummary $improvedSummary "-target"
$primarySuffix = if ($null -ne $baselineTarget -and $null -ne $improvedTarget) { "-target" } else { "-cpu" }
$baselineCpu = Find-ContainerSummary $baselineSummary $primarySuffix
$improvedCpu = Find-ContainerSummary $improvedSummary $primarySuffix
$baselineMemory = Find-ContainerSummary $baselineSummary $primarySuffix
$improvedMemory = Find-ContainerSummary $improvedSummary $primarySuffix

$comparison = @(
    [pscustomobject]@{ metric = "Primary CPU mean (%)"; baseline = $baselineCpu.cpu_mean_pct; improved = $improvedCpu.cpu_mean_pct; delta_percent = Format-Delta $baselineCpu.cpu_mean_pct $improvedCpu.cpu_mean_pct; lower_is_better = $true }
    [pscustomobject]@{ metric = "Primary CPU p95 (%)"; baseline = $baselineCpu.cpu_p95_pct; improved = $improvedCpu.cpu_p95_pct; delta_percent = Format-Delta $baselineCpu.cpu_p95_pct $improvedCpu.cpu_p95_pct; lower_is_better = $true }
    [pscustomobject]@{ metric = "Primary memory p95 (MB)"; baseline = $baselineMemory.memory_p95_mb; improved = $improvedMemory.memory_p95_mb; delta_percent = Format-Delta $baselineMemory.memory_p95_mb $improvedMemory.memory_p95_mb; lower_is_better = $true }
    [pscustomobject]@{ metric = "Log output (MB)"; baseline = $baselineSummary.log_megabytes; improved = $improvedSummary.log_megabytes; delta_percent = Format-Delta $baselineSummary.log_megabytes $improvedSummary.log_megabytes; lower_is_better = $true }
    [pscustomobject]@{ metric = "Time to first restart (sec)"; baseline = $baselineSummary.time_to_first_restart_sec; improved = $improvedSummary.time_to_first_restart_sec; delta_percent = Format-Delta $baselineSummary.time_to_first_restart_sec $improvedSummary.time_to_first_restart_sec; lower_is_better = $false }
)

$comparison | Format-Table -AutoSize
