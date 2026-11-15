# Copies the mod into Herdr, which reloads it. A symlink won't do: Herdr loads the code through one but not the icons.
$mod = 'better-icons'
$app = if ($env:HERDR_APP_HOME) { $env:HERDR_APP_HOME } else { Join-Path $HOME '.config\herdr-app' }
robocopy (Join-Path $PSScriptRoot "mods\$mod") (Join-Path $app "mods\$mod") /MIR /XF gpui-kit.d.ts herdr.d.ts jsconfig.json /NJH /NJS /NFL /NDL /NP | Out-Null
# robocopy: below 8 is success (1 = files copied).
if ($LASTEXITCODE -ge 8) { Write-Error "robocopy failed ($LASTEXITCODE)"; exit 1 }
exit 0
