$os = Get-CimInstance Win32_OperatingSystem
$totalKB = $os.TotalVisibleMemorySize
$freeKB = $os.FreePhysicalMemory
$usedKB = $totalKB - $freeKB
$totalGB = [math]::Round($totalKB / 1MB, 2)
$usedGB = [math]::Round($usedKB / 1MB, 2)
$freeGB = [math]::Round($freeKB / 1MB, 2)
$pct = [math]::Round(($usedKB / $totalKB) * 100, 1)
Write-Output "Total: $totalGB GB"
Write-Output "Used: $usedGB GB"
Write-Output "Free: $freeGB GB"
Write-Output "Usage: $pct%"
