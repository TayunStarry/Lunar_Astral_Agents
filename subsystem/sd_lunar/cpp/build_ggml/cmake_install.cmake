# Install script for directory: D:/Lunar_Astral_Agents/subsystem/sd_lunar/cpp/ggml

# Set the install prefix
if(NOT DEFINED CMAKE_INSTALL_PREFIX)
  set(CMAKE_INSTALL_PREFIX "C:/Program Files (x86)/ggml")
endif()
string(REGEX REPLACE "/$" "" CMAKE_INSTALL_PREFIX "${CMAKE_INSTALL_PREFIX}")

# Set the install configuration name.
if(NOT DEFINED CMAKE_INSTALL_CONFIG_NAME)
  if(BUILD_TYPE)
    string(REGEX REPLACE "^[^A-Za-z0-9_]+" ""
           CMAKE_INSTALL_CONFIG_NAME "${BUILD_TYPE}")
  else()
    set(CMAKE_INSTALL_CONFIG_NAME "Release")
  endif()
  message(STATUS "Install configuration: \"${CMAKE_INSTALL_CONFIG_NAME}\"")
endif()

# Set the component getting installed.
if(NOT CMAKE_INSTALL_COMPONENT)
  if(COMPONENT)
    message(STATUS "Install component: \"${COMPONENT}\"")
    set(CMAKE_INSTALL_COMPONENT "${COMPONENT}")
  else()
    set(CMAKE_INSTALL_COMPONENT)
  endif()
endif()

# Is this installation the result of a crosscompile?
if(NOT DEFINED CMAKE_CROSSCOMPILING)
  set(CMAKE_CROSSCOMPILING "FALSE")
endif()

# Set path to fallback-tool for dependency-resolution.
if(NOT DEFINED CMAKE_OBJDUMP)
  set(CMAKE_OBJDUMP "D:/mingw64/bin/objdump.exe")
endif()

if(NOT CMAKE_INSTALL_LOCAL_ONLY)
  # Include the install script for the subdirectory.
  include("D:/Lunar_Astral_Agents/subsystem/sd_lunar/cpp/build_ggml/src/cmake_install.cmake")
endif()

if(CMAKE_INSTALL_COMPONENT STREQUAL "Unspecified" OR NOT CMAKE_INSTALL_COMPONENT)
  file(INSTALL DESTINATION "${CMAKE_INSTALL_PREFIX}/lib" TYPE STATIC_LIBRARY FILES "D:/Lunar_Astral_Agents/subsystem/sd_lunar/cpp/build_ggml/src/ggml.a")
endif()

if(CMAKE_INSTALL_COMPONENT STREQUAL "Unspecified" OR NOT CMAKE_INSTALL_COMPONENT)
  file(INSTALL DESTINATION "${CMAKE_INSTALL_PREFIX}/include" TYPE FILE FILES
    "D:/Lunar_Astral_Agents/subsystem/sd_lunar/cpp/ggml/include/ggml.h"
    "D:/Lunar_Astral_Agents/subsystem/sd_lunar/cpp/ggml/include/ggml-cpu.h"
    "D:/Lunar_Astral_Agents/subsystem/sd_lunar/cpp/ggml/include/ggml-alloc.h"
    "D:/Lunar_Astral_Agents/subsystem/sd_lunar/cpp/ggml/include/ggml-backend.h"
    "D:/Lunar_Astral_Agents/subsystem/sd_lunar/cpp/ggml/include/ggml-blas.h"
    "D:/Lunar_Astral_Agents/subsystem/sd_lunar/cpp/ggml/include/ggml-cann.h"
    "D:/Lunar_Astral_Agents/subsystem/sd_lunar/cpp/ggml/include/ggml-cpp.h"
    "D:/Lunar_Astral_Agents/subsystem/sd_lunar/cpp/ggml/include/ggml-cuda.h"
    "D:/Lunar_Astral_Agents/subsystem/sd_lunar/cpp/ggml/include/ggml-opt.h"
    "D:/Lunar_Astral_Agents/subsystem/sd_lunar/cpp/ggml/include/ggml-metal.h"
    "D:/Lunar_Astral_Agents/subsystem/sd_lunar/cpp/ggml/include/ggml-rpc.h"
    "D:/Lunar_Astral_Agents/subsystem/sd_lunar/cpp/ggml/include/ggml-virtgpu.h"
    "D:/Lunar_Astral_Agents/subsystem/sd_lunar/cpp/ggml/include/ggml-sycl.h"
    "D:/Lunar_Astral_Agents/subsystem/sd_lunar/cpp/ggml/include/ggml-vulkan.h"
    "D:/Lunar_Astral_Agents/subsystem/sd_lunar/cpp/ggml/include/ggml-webgpu.h"
    "D:/Lunar_Astral_Agents/subsystem/sd_lunar/cpp/ggml/include/ggml-zendnn.h"
    "D:/Lunar_Astral_Agents/subsystem/sd_lunar/cpp/ggml/include/ggml-openvino.h"
    "D:/Lunar_Astral_Agents/subsystem/sd_lunar/cpp/ggml/include/gguf.h"
    )
endif()

if(CMAKE_INSTALL_COMPONENT STREQUAL "Unspecified" OR NOT CMAKE_INSTALL_COMPONENT)
  file(INSTALL DESTINATION "${CMAKE_INSTALL_PREFIX}/lib" TYPE STATIC_LIBRARY FILES "D:/Lunar_Astral_Agents/subsystem/sd_lunar/cpp/build_ggml/src/ggml-base.a")
endif()

if(CMAKE_INSTALL_COMPONENT STREQUAL "Unspecified" OR NOT CMAKE_INSTALL_COMPONENT)
  file(INSTALL DESTINATION "${CMAKE_INSTALL_PREFIX}/lib/pkgconfig" TYPE FILE FILES "D:/Lunar_Astral_Agents/subsystem/sd_lunar/cpp/build_ggml/ggml.pc")
endif()

if(CMAKE_INSTALL_COMPONENT STREQUAL "Unspecified" OR NOT CMAKE_INSTALL_COMPONENT)
  file(INSTALL DESTINATION "${CMAKE_INSTALL_PREFIX}/lib/cmake/ggml" TYPE FILE FILES
    "D:/Lunar_Astral_Agents/subsystem/sd_lunar/cpp/build_ggml/ggml-config.cmake"
    "D:/Lunar_Astral_Agents/subsystem/sd_lunar/cpp/build_ggml/ggml-version.cmake"
    )
endif()

string(REPLACE ";" "\n" CMAKE_INSTALL_MANIFEST_CONTENT
       "${CMAKE_INSTALL_MANIFEST_FILES}")
if(CMAKE_INSTALL_LOCAL_ONLY)
  file(WRITE "D:/Lunar_Astral_Agents/subsystem/sd_lunar/cpp/build_ggml/install_local_manifest.txt"
     "${CMAKE_INSTALL_MANIFEST_CONTENT}")
endif()
if(CMAKE_INSTALL_COMPONENT)
  if(CMAKE_INSTALL_COMPONENT MATCHES "^[a-zA-Z0-9_.+-]+$")
    set(CMAKE_INSTALL_MANIFEST "install_manifest_${CMAKE_INSTALL_COMPONENT}.txt")
  else()
    string(MD5 CMAKE_INST_COMP_HASH "${CMAKE_INSTALL_COMPONENT}")
    set(CMAKE_INSTALL_MANIFEST "install_manifest_${CMAKE_INST_COMP_HASH}.txt")
    unset(CMAKE_INST_COMP_HASH)
  endif()
else()
  set(CMAKE_INSTALL_MANIFEST "install_manifest.txt")
endif()

if(NOT CMAKE_INSTALL_LOCAL_ONLY)
  file(WRITE "D:/Lunar_Astral_Agents/subsystem/sd_lunar/cpp/build_ggml/${CMAKE_INSTALL_MANIFEST}"
     "${CMAKE_INSTALL_MANIFEST_CONTENT}")
endif()
