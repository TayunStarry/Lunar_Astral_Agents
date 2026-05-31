cmake_minimum_required(VERSION 3.12)
project("stable-diffusion" LANGUAGES C CXX)

set(CMAKE_EXPORT_COMPILE_COMMANDS ON)

if (NOT XCODE AND NOT MSVC AND NOT CMAKE_BUILD_TYPE)
    set(CMAKE_BUILD_TYPE Release CACHE STRING "Build type" FORCE)
endif()

if (WIN32)
    add_compile_definitions(_CRT_SECURE_NO_WARNINGS)
    add_compile_definitions(NOMINMAX)
endif()

if (MSVC)
    add_compile_definitions(_SILENCE_CXX17_CODECVT_HEADER_DEPRECATION_WARNING)
    add_compile_options(
        $<$<COMPILE_LANGUAGE:C>:/MP>
        $<$<COMPILE_LANGUAGE:C>:/utf-8>
        $<$<COMPILE_LANGUAGE:CXX>:/MP>
        $<$<COMPILE_LANGUAGE:CXX>:/utf-8>
    )
endif()

set(CMAKE_LIBRARY_OUTPUT_DIRECTORY ${CMAKE_BINARY_DIR}/bin)
set(CMAKE_RUNTIME_OUTPUT_DIRECTORY ${CMAKE_BINARY_DIR}/bin)

set(CMAKE_C_STANDARD 11)
set(CMAKE_C_STANDARD_REQUIRED true)
set(CMAKE_CXX_STANDARD 17)
set(CMAKE_CXX_STANDARD_REQUIRED true)

set(SD_LIB stable-diffusion)

file(GLOB SD_LIB_SOURCES CONFIGURE_DEPENDS
    "src/*.h"
    "src/*.cpp"
    "src/*.hpp"
    "src/model_io/*.h"
    "src/model_io/*.cpp"
    "src/tokenizers/*.h"
    "src/tokenizers/*.cpp"
    "src/tokenizers/vocab/*.h"
    "src/tokenizers/vocab/*.cpp"
)

set(SDCPP_BUILD_VERSION "local")
set(SDCPP_BUILD_COMMIT "unknown")

set_property(
  SOURCE ${CMAKE_CURRENT_SOURCE_DIR}/src/version.cpp
  APPEND PROPERTY COMPILE_DEFINITIONS
  SDCPP_BUILD_COMMIT=${SDCPP_BUILD_COMMIT} SDCPP_BUILD_VERSION=${SDCPP_BUILD_VERSION}
)

# --- Vulkan backend options ---
option(SD_VULKAN "sd: enable vulkan backend" ON)
if(SD_VULKAN)
    message("-- Vulkan GPU backend enabled (pre-built ggml)")
endif()

# --- GGML (pre-built) ---

set(GGML_BUILD_DIR "${CMAKE_SOURCE_DIR}/build_ggml" CACHE PATH "Pre-built ggml library directory")

if (NOT EXISTS "${GGML_BUILD_DIR}/src/ggml.a")
    message(FATAL_ERROR "GGML not pre-built. Run 'powershell -File ../build_ggml.ps1' first.\n  Expected: ${GGML_BUILD_DIR}/src/ggml.a")
endif()

add_library(ggml STATIC IMPORTED)
set_target_properties(ggml PROPERTIES
    IMPORTED_LOCATION "${GGML_BUILD_DIR}/src/ggml.a"
)
set_property(TARGET ggml PROPERTY INTERFACE_INCLUDE_DIRECTORIES
    "${CMAKE_SOURCE_DIR}/ggml/include"
    "${CMAKE_SOURCE_DIR}/ggml/src"
)

if (EXISTS "${GGML_BUILD_DIR}/src/ggml-vulkan/ggml-vulkan.a")
    add_library(ggml-vulkan STATIC IMPORTED)
    set_target_properties(ggml-vulkan PROPERTIES
        IMPORTED_LOCATION "${GGML_BUILD_DIR}/src/ggml-vulkan/ggml-vulkan.a"
    )
    set_property(TARGET ggml APPEND PROPERTY
        INTERFACE_LINK_LIBRARIES ggml-vulkan
    )
endif()

if (EXISTS "${GGML_BUILD_DIR}/src/ggml-base.a")
    set_property(TARGET ggml APPEND PROPERTY
        INTERFACE_LINK_LIBRARIES "${GGML_BUILD_DIR}/src/ggml-base.a"
    )
endif()

if (EXISTS "${GGML_BUILD_DIR}/src/ggml-cpu.a")
    set_property(TARGET ggml APPEND PROPERTY
        INTERFACE_LINK_LIBRARIES "${GGML_BUILD_DIR}/src/ggml-cpu.a"
    )
endif()

message("-- GGML pre-built library: ${GGML_BUILD_DIR}")

# --- Build ---

message("-- Build static library")
set(BUILD_SHARED_LIBS OFF)
add_library(${SD_LIB} STATIC ${SD_LIB_SOURCES})

set(CMAKE_POLICY_DEFAULT_CMP0077 NEW)

add_definitions(-DGGML_MAX_NAME=128)

add_subdirectory(thirdparty)

target_link_libraries(${SD_LIB} PUBLIC ggml zip)
target_include_directories(${SD_LIB} PUBLIC . src include)
target_include_directories(${SD_LIB} PUBLIC . thirdparty)
target_compile_features(${SD_LIB} PUBLIC c_std_11 cxx_std_17)

# Transitive dependencies from pre-built ggml
if (SD_VULKAN)
    find_library(Vulkan_LIBRARY vulkan-1 PATHS "C:/VulkanSDK/1.4.350.0/Lib" REQUIRED)
    target_link_libraries(${SD_LIB} PUBLIC ${Vulkan_LIBRARY})
endif()
find_package(OpenMP)
if (OpenMP_FOUND)
    target_link_libraries(${SD_LIB} PUBLIC OpenMP::OpenMP_CXX)
endif()

set(SD_PUBLIC_HEADERS include/stable-diffusion.h)
set_target_properties(${SD_LIB} PROPERTIES PUBLIC_HEADER "${SD_PUBLIC_HEADERS}")

# --- CLI Executable ---

set(SD_CLI_EXE sd-cli)

add_executable(${SD_CLI_EXE} src/main.cpp)
target_link_libraries(${SD_CLI_EXE} PUBLIC ${SD_LIB} ggml zip)
target_include_directories(${SD_CLI_EXE} PUBLIC . src include thirdparty)
target_compile_features(${SD_CLI_EXE} PUBLIC c_std_11 cxx_std_17)
if (WIN32)
    target_compile_definitions(${SD_CLI_EXE} PRIVATE -D_CRT_SECURE_NO_WARNINGS -DNOMINMAX)
endif()