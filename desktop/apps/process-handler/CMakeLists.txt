cmake_minimum_required(VERSION 3.15)
project(neuro-process-handler CXX)

set(CMAKE_CXX_STANDARD 17)
set(CMAKE_CXX_STANDARD_REQUIRED ON)

# ============================================================
# Platform-specific settings
# ============================================================
if(WIN32)
    add_definitions(-D_WIN32_WINNT=0x0601)  # Windows 7+
    add_definitions(-DNOMINMAX)
    add_definitions(-DWIN32_LEAN_AND_MEAN)
endif()

# ============================================================
# Source files
# ============================================================
set(PROCESS_HANDLER_SOURCES
    src/process_handler.cpp
    src/main.cpp
)

set(PROCESS_HANDLER_HEADERS
    src/process_handler.h
)

# ============================================================
# Executable
# ============================================================
add_executable(process-handler 
    ${PROCESS_HANDLER_SOURCES}
    ${PROCESS_HANDLER_HEADERS}
)

# ============================================================
# Include directories
# ============================================================
target_include_directories(process-handler PRIVATE
    ${CMAKE_CURRENT_SOURCE_DIR}/src
)

# ============================================================
# Platform-specific libraries
# ============================================================
if(WIN32)
    target_link_libraries(process-handler PRIVATE
        ws2_32      # Windows Sockets
        kernel32    # Process management
    )
else()
    # Unix-like systems
    target_link_libraries(process-handler PRIVATE
        pthread     # POSIX threads
        dl          # Dynamic loading
    )
endif()

# ============================================================
# Compiler warnings
# ============================================================
if(MSVC)
    target_compile_options(process-handler PRIVATE /W4 /WX)
else()
    target_compile_options(process-handler PRIVATE 
        -Wall -Wextra -Wpedantic -Werror
    )
endif()

# ============================================================
# JSON Library (nlohmann/json) - Header-only
# ============================================================
include(FetchContent)
FetchContent_Declare(
    json
    GIT_REPOSITORY https://github.com/nlohmann/json.git
    GIT_TAG v3.11.3
)
FetchContent_MakeAvailable(json)
target_link_libraries(process-handler PRIVATE nlohmann_json::nlohmann_json)

# ============================================================
# Unit Tests (optional)
# ============================================================
option(BUILD_TESTS "Build unit tests" ON)

if(BUILD_TESTS)
    enable_testing()
    
    # Download and configure Google Test
    FetchContent_Declare(
        googletest
        GIT_REPOSITORY https://github.com/google/googletest.git
        GIT_TAG v1.14.0
    )
    
    # For Windows: Prevent overriding the parent project's compiler/linker settings
    set(gtest_force_shared_crt ON CACHE BOOL "" FORCE)
    FetchContent_MakeAvailable(googletest)
    
    # Test executable
    add_executable(process-handler-tests
        tests/test_message.cpp
        tests/test_validator.cpp
        tests/test_channels.cpp
        tests/test_process_manager.cpp
        src/process_handler.cpp  # Include implementation
    )
    
    target_include_directories(process-handler-tests PRIVATE
        ${CMAKE_CURRENT_SOURCE_DIR}/src
    )
    
    target_link_libraries(process-handler-tests PRIVATE
        gtest_main
        gmock_main
        nlohmann_json::nlohmann_json
    )
    
    if(WIN32)
        target_link_libraries(process-handler-tests PRIVATE ws2_32 kernel32)
    else()
        target_link_libraries(process-handler-tests PRIVATE pthread dl)
    endif()
    
    # Discover tests
    include(GoogleTest)
    gtest_discover_tests(process-handler-tests)
endif()

# ============================================================
# Installation
# ============================================================
install(TARGETS process-handler
    RUNTIME DESTINATION bin
)

# ============================================================
# Print configuration
# ============================================================
message(STATUS "=== Neuro Process Handler Configuration ===")
message(STATUS "Build type: ${CMAKE_BUILD_TYPE}")
message(STATUS "C++ Standard: ${CMAKE_CXX_STANDARD}")
message(STATUS "Build tests: ${BUILD_TESTS}")
if(WIN32)
    message(STATUS "Platform: Windows")
else()
    message(STATUS "Platform: Unix-like")
endif()
message(STATUS "==========================================")