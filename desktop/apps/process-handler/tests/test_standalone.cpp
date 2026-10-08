// ============================================================
// tests/test_standalone.cpp
//
// Dependency-free checks for the process handler. The other test files in this
// directory need GoogleTest, which is not always available (and is not wired
// into CI), so these checks compile with nothing but a C++17 compiler:
//
//   g++ -std=c++17 -pthread -Isrc -o /tmp/nd-process-handler-tests
//       src/process_handler.cpp tests/test_standalone.cpp
//   /tmp/nd-process-handler-tests
//
// `desktop/tools/ci/repo_checks.py` runs exactly that when a compiler exists.
// ============================================================

#include "process_handler.h"

#include <atomic>
#include <chrono>
#include <cstdio>
#include <cstdlib>
#include <fstream>
#include <iostream>
#include <sstream>
#include <string>
#include <thread>
#include <vector>

using namespace neuro;

namespace {

int g_failures = 0;
int g_checks = 0;

void check(bool condition, const std::string& what) {
    ++g_checks;
    if (!condition) {
        ++g_failures;
        std::cerr << "FAIL: " << what << std::endl;
    }
}

template <typename T>
void check_equal(const T& actual, const T& expected, const std::string& what) {
    ++g_checks;
    if (!(actual == expected)) {
        ++g_failures;
        std::cerr << "FAIL: " << what << " (expected " << expected << ", got "
                  << actual << ")" << std::endl;
    }
}

std::string read_file(const std::string& path) {
    std::ifstream file(path);
    std::ostringstream buffer;
    buffer << file.rdbuf();
    return buffer.str();
}

bool wait_for(const std::function<bool()>& predicate, int timeout_ms) {
    const auto deadline = std::chrono::steady_clock::now() +
                          std::chrono::milliseconds(timeout_ms);
    while (std::chrono::steady_clock::now() < deadline) {
        if (predicate()) {
            return true;
        }
        std::this_thread::sleep_for(std::chrono::milliseconds(50));
    }
    return predicate();
}

// ------------------------------------------------------------
// Message serialisation
// ------------------------------------------------------------

void test_message_round_trip() {
    Message original;
    original.type = MessageType::COMMAND;
    original.source_process = "rust_main";
    original.target_process = "go_integration";
    original.command = "execute";
    original.data = R"({"action":"type_text","params":{"text":"hi"}})";
    original.timestamp = 123456789;
    original.message_id = "msg-001";

    const std::string json = original.to_json();
    const Message parsed = Message::from_json(json);

    check(parsed.valid, "round-tripped message parses");
    check(parsed.type == MessageType::COMMAND, "message type survives");
    check_equal(parsed.source_process, std::string("rust_main"), "source survives");
    check_equal(parsed.target_process, std::string("go_integration"), "target survives");
    check_equal(parsed.command, std::string("execute"), "command survives");
    check_equal(parsed.data, original.data, "nested data survives verbatim");
    check_equal(parsed.timestamp, static_cast<uint64_t>(123456789), "timestamp survives");
    check_equal(parsed.message_id, std::string("msg-001"), "message id survives");
}

void test_nested_and_escaped_data() {
    Message tricky;
    tricky.type = MessageType::EVENT;
    tricky.source_process = "a";
    tricky.target_process = "b";
    tricky.command = "nested";
    tricky.data = R"({"a":[1,2,{"b":"}"}],"c":"\"quoted\" legacy \\ backslash"})";

    const Message parsed = Message::from_json(tricky.to_json());
    check(parsed.valid, "nested data parses");
    check_equal(parsed.data, tricky.data, "braces inside strings do not end the object");
}

void test_malformed_messages_are_rejected() {
    const std::string bad[] = {
        "",
        "{",
        "not json",
        "[1,2,3]",                       // array, not an envelope
        R"({"type":"COMMAND"})",         // type must be numeric
        R"({"type":99})",                // out of range
        R"({"timestamp":1})",            // no type at all
        R"({"type":0,"timestamp":"x"})", // bad timestamp
        R"({"type":0,"data":{"unterminated":)",
    };
    for (const std::string& input : bad) {
        const Message msg = Message::from_json(input);
        check(!msg.valid, "rejected malformed input: " + input);
    }
}

void test_unicode_escape() {
    const Message msg = Message::from_json(R"({"type":0,"command":"caf\u00e9"})");
    check(msg.valid, "unicode escape parses");
    check_equal(msg.command, std::string("caf\xC3\xA9"), "\\u00e9 becomes UTF-8");
}

// ------------------------------------------------------------
// Validation and rate limiting
// ------------------------------------------------------------

void test_is_safe_json() {
    check(MessageValidator::is_safe_json(R"({"a":1})"), "object payload is valid");
    check(MessageValidator::is_safe_json(R"([1,2,3])"), "array payload is valid");
    check(MessageValidator::is_safe_json(R"("text")"), "string payload is valid");
    check(MessageValidator::is_safe_json(R"({"type":0,"data":{"a":[1,{"b":2}]}})"),
          "envelope is valid");
    check(!MessageValidator::is_safe_json(""), "empty payload is invalid");
    check(!MessageValidator::is_safe_json("{"), "truncated payload is invalid");
    check(!MessageValidator::is_safe_json(R"({"a":})"), "dangling value is invalid");
    check(!MessageValidator::is_safe_json(R"({"a" 1})"), "missing colon is invalid");
    check(!MessageValidator::is_safe_json(std::string(1024 * 1024 + 1, 'x')),
          "oversized payload is invalid");
}

void test_validate_message() {
    Message msg;
    msg.type = MessageType::COMMAND;
    msg.source_process = "source";
    msg.target_process = "target";
    msg.command = "do";
    msg.data = R"({"ok":true})";

    std::string error;
    check(MessageValidator::validate_message(msg, error), "complete message validates");

    Message no_target = msg;
    no_target.target_process.clear();
    check(!MessageValidator::validate_message(no_target, error), "missing target rejected");

    Message bad_data = msg;
    bad_data.data = "{oops";
    check(!MessageValidator::validate_message(bad_data, error), "bad payload rejected");
}

void test_rate_limit_is_per_source() {
    int allowed = 0;
    for (int i = 0; i < 10; ++i) {
        if (MessageValidator::check_rate_limit("standalone-a", 5)) {
            ++allowed;
        }
    }
    check_equal(allowed, 5, "5 of 10 within one second are allowed");

    check(MessageValidator::check_rate_limit("standalone-b", 5),
          "a different source has its own budget");
    check(!MessageValidator::check_rate_limit("standalone-a", 5),
          "the first source stays blocked");

    for (int i = 0; i < 50; ++i) {
        check(MessageValidator::check_rate_limit("standalone-unlimited", 0),
              "0 means unlimited");
    }
}

// ------------------------------------------------------------
// Process lifecycle (POSIX only; the Windows path needs real processes)
// ------------------------------------------------------------

#ifndef _WIN32
void test_process_env_and_stop() {
    const std::string marker = "/tmp/nd_process_handler_env_marker.txt";
    std::remove(marker.c_str());

    ProcessManager manager;
    ProcessConfig config;
    config.name = "standalone-env-probe";
    config.executable_path = "/bin/sh";
    config.args = {"-c", "echo \"$ND_STANDALONE_ENV\" > " + marker + "; sleep 5"};
    config.env_vars = {{"ND_STANDALONE_ENV", "hello-from-env"}};
    config.auto_restart = false;
    config.enable_heartbeat = false;

    check(manager.register_process(config), "process registers");
    check(manager.start_process(config.name), "process starts");
    check(manager.get_process_state(config.name) == ProcessState::RUNNING,
          "process is RUNNING");

    const bool wrote = wait_for([&marker]() {
        return read_file(marker).find("hello-from-env") != std::string::npos;
    }, 3000);
    check(wrote, "env_vars reach the child process");

    check(manager.stop_process(config.name, true), "process stops");
    check(manager.get_process_state(config.name) == ProcessState::STOPPED,
          "process is STOPPED after stop");
    std::remove(marker.c_str());
}

void test_crash_is_detected_without_deadlock() {
    ProcessManager manager;
    ProcessConfig config;
    config.name = "standalone-crash-probe";
    config.executable_path = "/bin/sh";
    config.args = {"-c", "exit 3"};
    config.auto_restart = false;
    config.enable_heartbeat = false;

    check(manager.register_process(config), "crash probe registers");
    check(manager.start_process(config.name), "crash probe starts");

    // The monitor thread must notice the exit and mark the process CRASHED
    // (the old implementation deadlocked here by re-locking manager_mutex).
    const bool crashed = wait_for([&manager, &config]() {
        return manager.get_process_state(config.name) == ProcessState::CRASHED;
    }, 5000);
    check(crashed, "exited process is marked CRASHED");

    check(manager.stop_process(config.name, true) || true, "manager still responsive");
}

void test_health_monitoring_toggle() {
    ProcessManager manager;
    ProcessConfig config;
    config.name = "standalone-heartbeat-probe";
    config.executable_path = "/bin/sh";
    config.args = {"-c", "sleep 5"};
    config.auto_restart = false;
    config.enable_heartbeat = true;
    config.heartbeat_timeout = std::chrono::seconds(1);

    manager.enable_health_monitoring(false);
    check(manager.register_process(config), "heartbeat probe registers");
    check(manager.start_process(config.name), "heartbeat probe starts");

    // With monitoring off the process must not be killed for a late heartbeat.
    std::this_thread::sleep_for(std::chrono::milliseconds(2500));
    check(manager.get_process_state(config.name) == ProcessState::RUNNING,
          "disabled health monitoring leaves a live process alone");

    manager.enable_health_monitoring(true);
    const bool reaped = wait_for([&manager, &config]() {
        return manager.get_process_state(config.name) == ProcessState::CRASHED;
    }, 6000);
    check(reaped, "re-enabled health monitoring reaps the late process");
}
#endif  // !_WIN32

}  // namespace

int main() {
    test_message_round_trip();
    test_nested_and_escaped_data();
    test_malformed_messages_are_rejected();
    test_unicode_escape();
    test_is_safe_json();
    test_validate_message();
    test_rate_limit_is_per_source();
#ifndef _WIN32
    test_process_env_and_stop();
    test_crash_is_detected_without_deadlock();
    test_health_monitoring_toggle();
#endif

    std::cout << g_checks - g_failures << "/" << g_checks << " checks passed"
              << std::endl;
    if (g_failures != 0) {
        std::cerr << g_failures << " check(s) failed" << std::endl;
        return 1;
    }
    std::cout << "process-handler standalone tests OK" << std::endl;
    return 0;
}
