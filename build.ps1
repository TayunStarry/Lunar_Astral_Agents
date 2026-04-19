# 构建LunarCore
Set-Location -Path './LunarCore'
./build.ps1
Set-Location -Path '../'

# 构建image_generation
Set-Location -Path './subsystem\image_generation'
./build.ps1
Set-Location -Path '../../'

# 构建file_explorer
Set-Location -Path './subsystem\file_explorer'
./build.ps1
Set-Location -Path '../../'

# 构建qq_adapter
Set-Location -Path './subsystem\qq_adapter'
./build.ps1
Set-Location -Path '../../'

# 构建image_box
Set-Location -Path './subsystem\image_box'
./build.ps1
Set-Location -Path '../../'

# 构建project_archiving
Set-Location -Path './subsystem\project_archiving'
./build.ps1
Set-Location -Path '../../'