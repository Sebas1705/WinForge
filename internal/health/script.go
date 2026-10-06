package health

// collectScript reads the machine state and prints one JSON document. It only
// reads: no Set-*, no installs, no network. Every section is wrapped so one
// failing query (for example one that needs administrator rights) leaves the
// rest of the report intact and is listed under "errors".
const collectScript = `
$ErrorActionPreference = 'SilentlyContinue'
[Console]::OutputEncoding = [Text.Encoding]::UTF8
$errs = New-Object System.Collections.ArrayList
function Section([string]$Name, [scriptblock]$Body) {
    try { & $Body } catch { [void]$errs.Add($Name + ': ' + $_.Exception.Message); $null }
}
function Day($d) { if ($d -is [datetime]) { $d.ToString('yyyy-MM-dd') } else { '' } }
function Gb($n) { [math]::Round(([double]$n) / 1GB, 1) }

$isAdmin = ([Security.Principal.WindowsPrincipal][Security.Principal.WindowsIdentity]::GetCurrent()).IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)
$os   = Section 'os'    { Get-CimInstance Win32_OperatingSystem }
$cs   = Section 'system'{ Get-CimInstance Win32_ComputerSystem }
$bios = Section 'bios'  { Get-CimInstance Win32_BIOS }
$bb   = Section 'board' { Get-CimInstance Win32_BaseBoard }
$cpu  = Section 'cpu'   { Get-CimInstance Win32_Processor | Select-Object -First 1 }

$types = @{ 1='Desktop'; 2='Laptop'; 3='Workstation'; 4='Server'; 5='Server'; 6='Server'; 7='Server'; 8='Server'; 9='Tablet' }

$pending = $false
foreach ($k in 'HKLM:\SOFTWARE\Microsoft\Windows\CurrentVersion\Component Based Servicing\RebootPending',
               'HKLM:\SOFTWARE\Microsoft\Windows\CurrentVersion\WindowsUpdate\Auto Update\RebootRequired') {
    if (Test-Path $k) { $pending = $true }
}
if ((Get-ItemProperty 'HKLM:\SYSTEM\CurrentControlSet\Control\Session Manager' -Name PendingFileRenameOperations -ErrorAction SilentlyContinue).PendingFileRenameOperations) { $pending = $true }

# UEFI and Secure Boot state are readable without administrator rights here.
$uefi = $null; $sb = $null
$sbKey = 'HKLM:\SYSTEM\CurrentControlSet\Control\SecureBoot\State'
if (Test-Path $sbKey) {
    $uefi = $true
    $v = (Get-ItemProperty $sbKey -ErrorAction SilentlyContinue).UEFISecureBootEnabled
    if ($v -ne $null) { $sb = ([int]$v -eq 1) }
} else {
    $fw = (Get-ItemProperty 'HKLM:\SYSTEM\CurrentControlSet\Control' -Name PEFirmwareType -ErrorAction SilentlyContinue).PEFirmwareType
    if ($fw -eq 1) { $uefi = $false } elseif ($fw -eq 2) { $uefi = $true }
}
$tpm = Section 'tpm' { Get-Tpm }
$bl = $null; try { $v = Get-BitLockerVolume -MountPoint $env:SystemDrive -ErrorAction Stop; $bl = ($v.ProtectionStatus -eq 'On' -or $v.ProtectionStatus -eq 1) } catch { $bl = $null }
$def = Section 'defender' { Get-MpComputerStatus }
$avs = Section 'antivirus' { Get-CimInstance -Namespace root/SecurityCenter2 -ClassName AntiVirusProduct }

$defender = $null
if ($def) {
    $age = 999; if ($def.AntivirusSignatureLastUpdated -is [datetime]) { $age = ((Get-Date) - $def.AntivirusSignatureLastUpdated).TotalDays }
    $defender = [ordered]@{ enabled = [bool]$def.AMServiceEnabled; realTime = [bool]$def.RealTimeProtectionEnabled; signatureAgeDays = [math]::Round($age, 1) }
}

$mem = Section 'memory' { Get-CimInstance Win32_PhysicalMemory }
$gpus = Section 'gpu' { Get-CimInstance Win32_VideoController }
$disks = Section 'disks' { Get-PhysicalDisk }
$vols = Section 'volumes' { Get-Volume | Where-Object { $_.DriveType -eq 'Fixed' -and $_.DriveLetter } }
$drv = Section 'drivers' { Get-CimInstance Win32_PnPSignedDriver | Where-Object { $_.DeviceName } }
$bad = Section 'devices' { Get-CimInstance Win32_PnPEntity | Where-Object { $_.ConfigManagerErrorCode -and $_.ConfigManagerErrorCode -ne 0 -and $_.ConfigManagerErrorCode -ne 22 } }
$nic = Section 'firewall' { Get-NetFirewallProfile }

$uptime = 0; if ($os -and $os.LastBootUpTime -is [datetime]) { $uptime = [math]::Round(((Get-Date) - $os.LastBootUpTime).TotalDays, 1) }
$virt = $null; if ($cpu -and $cpu.VirtualizationFirmwareEnabled -ne $null) { $virt = [bool]$cpu.VirtualizationFirmwareEnabled }

$result = [ordered]@{
    admin   = $isAdmin
    machine = [ordered]@{ manufacturer = [string]$cs.Manufacturer; model = [string]$cs.Model; type = [string]$types[[int]$cs.PCSystemType] }
    os      = [ordered]@{ caption = [string]$os.Caption; version = [string]$os.Version; build = [string]$os.BuildNumber; arch = [string]$os.OSArchitecture; uptimeDays = $uptime; pendingReboot = $pending }
    cpu     = [ordered]@{ name = ([string]$cpu.Name).Trim(); cores = [int]$cpu.NumberOfCores; threads = [int]$cpu.NumberOfLogicalProcessors; virtualizationOn = $virt; hypervisorPresent = [bool]$cs.HypervisorPresent }
    memory  = [ordered]@{
        totalGB = (Gb $cs.TotalPhysicalMemory)
        modules = @($mem | ForEach-Object { [ordered]@{ capacityGB = (Gb $_.Capacity); speedMHz = [int]$_.Speed; configuredMHz = [int]$_.ConfiguredClockSpeed; manufacturer = ([string]$_.Manufacturer).Trim(); part = ([string]$_.PartNumber).Trim() } })
    }
    gpus    = @($gpus | ForEach-Object { [ordered]@{ name = [string]$_.Name; vendor = [string]$_.AdapterCompatibility; driverVersion = [string]$_.DriverVersion; driverDate = (Day $_.DriverDate) } })
    board   = [ordered]@{ manufacturer = [string]$bb.Manufacturer; product = [string]$bb.Product }
    bios    = [ordered]@{ vendor = [string]$bios.Manufacturer; version = [string]$bios.SMBIOSBIOSVersion; date = (Day $bios.ReleaseDate); uefi = $uefi }
    disks   = @($disks | ForEach-Object { [ordered]@{ name = [string]$_.FriendlyName; media = [string]$_.MediaType; bus = [string]$_.BusType; sizeGB = (Gb $_.Size); health = [string]$_.HealthStatus; operational = (@($_.OperationalStatus) -join ',') } })
    volumes = @($vols | ForEach-Object { [ordered]@{ letter = [string]$_.DriveLetter; sizeGB = (Gb $_.Size); freeGB = (Gb $_.SizeRemaining); fileSystem = [string]$_.FileSystem } })
    security = [ordered]@{
        antivirus = @($avs | ForEach-Object { [ordered]@{ name = [string]$_.displayName; enabled = ((([int]$_.productState) -band 0xF000) -eq 0x1000); upToDate = ((([int]$_.productState) -band 0xF0) -eq 0) } })
        defender  = $defender
        firewall  = @($nic | ForEach-Object { [ordered]@{ profile = [string]$_.Name; enabled = [bool]($_.Enabled -eq 'True' -or $_.Enabled -eq $true -or $_.Enabled -eq 1) } })
        secureBoot = $sb
        tpm        = $(if ($tpm) { [ordered]@{ present = [bool]$tpm.TpmPresent; ready = [bool]$tpm.TpmReady } } else { $null })
        bitlockerSystem = $bl
    }
    drivers  = @($drv | ForEach-Object { [ordered]@{ device = [string]$_.DeviceName; class = [string]$_.DeviceClass; manufacturer = [string]$_.Manufacturer; version = [string]$_.DriverVersion; date = (Day $_.DriverDate); signed = [bool]$_.IsSigned; inf = [string]$_.InfName } })
    problems = @($bad | ForEach-Object {
        $label = [string]$_.Name; if (-not $label) { $label = [string]$_.Description }
        $hw = [string]$_.PNPDeviceID
        [ordered]@{ device = $label; class = [string]$_.PNPClass; hardwareId = $hw; code = [int]$_.ConfigManagerErrorCode }
    })
    errors   = @($errs)
}
$result | ConvertTo-Json -Depth 6 -Compress
`

// updateScript searches Windows Update for pending items, drivers and firmware
// included. It does not download or install anything.
const updateScript = `
$ErrorActionPreference = 'Stop'
[Console]::OutputEncoding = [Text.Encoding]::UTF8
$session = New-Object -ComObject Microsoft.Update.Session
$searcher = $session.CreateUpdateSearcher()
$found = $searcher.Search("IsInstalled=0 and IsHidden=0")
$items = @()
foreach ($u in $found.Updates) {
    $cats = @($u.Categories | ForEach-Object { $_.Name })
    $items += [ordered]@{
        title = [string]$u.Title
        kind = $(if ($u.Type -eq 2) { 'Driver' } else { 'Software' })
        category = ($cats -join ', ')
        sizeMB = [math]::Round([double]$u.MaxDownloadSize / 1MB, 1)
        reboot = ($u.InstallationBehavior.RebootBehavior -ne 0)
    }
}
[ordered]@{ updates = @($items) } | ConvertTo-Json -Depth 4 -Compress
`
