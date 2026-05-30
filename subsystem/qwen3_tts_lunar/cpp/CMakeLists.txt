cmake_minimum_required(VERSION 3.14)
project(qwen3-tts-ggml VERSION 0.1.0 LANGUAGES CXX)

set(CMAKE_CXX_STANDARD 17)
set(CMAKE_CXX_STANDARD_REQUIRED ON)

if(CMAKE_CXX_COMPILER_ID MATCHES "GNU|Clang")
    set(CMAKE_CXX_FLAGS "${CMAKE_CXX_FLAGS} -Wall -Wextra -Wpedantic")
    set(CMAKE_CXX_FLAGS_RELEASE "${CMAKE_CXX_FLAGS_RELEASE} -O3 -march=x86-64 -mtune=generic")
    set(CMAKE_CXX_FLAGS_DEBUG "${CMAKE_CXX_FLAGS_DEBUG} -g -O0")
    set(CMAKE_CXX_FLAGS "${CMAKE_CXX_FLAGS} -mno-avx512f -mno-avx512vl -mno-avx512dq -mno-avx512bw -mno-avx512cd")
endif()

option(QWEN3_TTS_TIMING "Enable detailed timing instrumentation" OFF)
if(QWEN3_TTS_TIMING)
    add_compile_definitions(QWEN3_TTS_TIMING)
endif()

find_package(Threads REQUIRED)
find_package(OpenMP REQUIRED)
if(OpenMP_CXX_FOUND)
    message(STATUS "OpenMP found - version ${OpenMP_CXX_VERSION}")
endif()

set(GGML_DIR "${CMAKE_CURRENT_SOURCE_DIR}/ggml")
set(GGML_BUILD_DIR "${GGML_DIR}/build")
set(GGML_LIB_DIR "${GGML_BUILD_DIR}/src")

function(link_ggml_libs target)
    target_include_directories(${target} PUBLIC
        ${CMAKE_CURRENT_SOURCE_DIR}/src
        ${GGML_DIR}/include
    )
    target_link_libraries(${target} PUBLIC
        "${GGML_LIB_DIR}/ggml.a"
        "${GGML_LIB_DIR}/ggml-base.a"
        "${GGML_LIB_DIR}/ggml-cpu.a"
        Threads::Threads
        OpenMP::OpenMP_CXX
    )
    if(EXISTS "${GGML_LIB_DIR}/ggml-vulkan/ggml-vulkan.a")
        find_package(Vulkan REQUIRED)
        target_link_libraries(${target} PUBLIC
            "${GGML_LIB_DIR}/ggml-vulkan/ggml-vulkan.a"
            Vulkan::Vulkan
        )
    endif()
endfunction()

add_library(text_tokenizer STATIC
    src/text_tokenizer.cpp
    src/gguf_loader.cpp
)
link_ggml_libs(text_tokenizer)

add_library(tts_transformer STATIC
    src/gguf_loader.cpp
    src/tts_transformer.cpp
    src/coreml_code_predictor_stub.cpp
)
link_ggml_libs(tts_transformer)

add_library(audio_tokenizer_encoder STATIC
    src/audio_tokenizer_encoder.cpp
    src/gguf_loader.cpp
)
link_ggml_libs(audio_tokenizer_encoder)

add_library(audio_tokenizer_decoder STATIC
    src/audio_tokenizer_decoder.cpp
    src/gguf_loader.cpp
)
link_ggml_libs(audio_tokenizer_decoder)

add_library(qwen3_tts STATIC
    src/qwen3_tts.cpp
)
target_include_directories(qwen3_tts PUBLIC
    ${CMAKE_CURRENT_SOURCE_DIR}/src
    ${GGML_DIR}/include
)
target_link_libraries(qwen3_tts PUBLIC
    text_tokenizer
    tts_transformer
    audio_tokenizer_encoder
    audio_tokenizer_decoder
    Threads::Threads
    OpenMP::OpenMP_CXX
)

add_library(qwen3tts_shared SHARED
    src/qwen3tts_c_api.cpp
)
set_target_properties(qwen3tts_shared PROPERTIES
    OUTPUT_NAME "qwen3tts"
    VERSION ${PROJECT_VERSION}
    SOVERSION 0
    CXX_VISIBILITY_PRESET hidden
    VISIBILITY_INLINES_HIDDEN ON
    PREFIX ""
)
if(WIN32)
    target_sources(qwen3tts_shared PRIVATE src/qwen3tts.def)
endif()
target_include_directories(qwen3tts_shared PUBLIC
    ${CMAKE_CURRENT_SOURCE_DIR}/src
)
target_link_libraries(qwen3tts_shared PRIVATE
    qwen3_tts
)

if(WIN32)
    set_target_properties(qwen3tts_shared PROPERTIES
        RUNTIME_OUTPUT_NAME "qwen3tts"
        ARCHIVE_OUTPUT_NAME "qwen3tts"
    )
endif()