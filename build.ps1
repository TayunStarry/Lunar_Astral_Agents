# 构建LunarCore
Set-Location -Path './LunarCore'
./build.ps1
Set-Location -Path '../'

# 构建 Crystal_Astral
Set-Location -Path './subsystem\crystal_astral'
./build.ps1
Set-Location -Path '../../'

# 构建 bridge_adapter
Set-Location -Path './subsystem\bridge_adapter'
./build.ps1
Set-Location -Path '../../'

# 构建 project_archiving
Set-Location -Path './subsystem\project_archiving'
./build.ps1
Set-Location -Path '../../'