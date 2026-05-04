# 构建LunarCore
Set-Location -Path './LunarCore'
./build.ps1
Set-Location -Path '../'

# 构建 application_manager
Set-Location -Path './subsystem\application_manager'
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