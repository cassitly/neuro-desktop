// ============================================================
// process_handler.cpp - Implementation
// ============================================================

#include "process_handler.h"

#include <algorithm>
#include <cstdint>
#include <fstream>
#include <sstream>
#include <thread>
#include <chrono>
#include <iostream>
#include <atomic>

#ifdef _WIN32
#include <windows.h>
#include <process.h>
#else
#include <unistd.h>
#include <sys/wait.h>
#include <signal.h>
#endif

namespace neuro {

// ============================================================
// Message Implementation
// ============================================================

std::string Message::to_json() const {
    std::ostringstream oss;
    oss << "{";
    oss << "\"type\":\"" << static_cast<int>(type) << "\",";
    oss << "\"source\":\"" << source_process << "\",";
    oss << "\"target\":\"" << target_process << "\",";
    oss << "\"command\":\"" << command << "\",";
    oss << "\"data\":" << data << ",";
    oss << "\"timestamp\":" << timestamp << ",";
    oss << "\"message_id\":\"" << message_id << "\"";
    oss << "}";
    return oss.str();
}

namespace {

// ---------------------------------------------------------------------------
// Minimal strict JSON reader for the process-handler wire format.
//
// The handler exchanges flat objects produced by Message::to_json() plus the
// `data` payload, which is itself an arbitrary JSON value and is kept verbatim.
// The previous implementation returned a default-constructed Message and threw
// the input away, so no command the supervisor received was ever understood.
// ---------------------------------------------------------------------------

class JsonReader {
public:
    explicit JsonReader(const std::string& text) : text_(text) {}

    bool parse_object(std::map<std::string, std::string>& out) {
        skip_ws();
        if (!consume('{')) {
            return false;
        }
        skip_ws();
        if (consume('}')) {
            return true;
        }
        while (true) {
            skip_ws();
            std::string key;
            if (!parse_string(key)) {
                return false;
            }
            skip_ws();
            if (!consume(':')) {
                return false;
            }
            skip_ws();
            std::string value;
            if (!parse_value(value)) {
                return false;
            }
            out[key] = value;
            skip_ws();
            if (consume(',')) {
                continue;
            }
            if (consume('}')) {
                break;
            }
            return false;
        }
        skip_ws();
        return at_end();
    }

    // True when the whole text is one syntactically valid JSON value. Used by
    // MessageValidator::is_safe_json(), which sees plain payloads (objects,
    // arrays, scalars) rather than whole message envelopes.
    bool parse_document() {
        skip_ws();
        std::string ignored;
        if (!parse_value(ignored)) {
            return false;
        }
        skip_ws();
        return at_end();
    }

private:
    void skip_ws() {
        while (pos_ < text_.size()) {
            char c = text_[pos_];
            if (c == ' ' || c == '\t' || c == '\n' || c == '\r') {
                ++pos_;
            } else {
                break;
            }
        }
    }

    bool at_end() const { return pos_ >= text_.size(); }

    bool consume(char expected) {
        if (pos_ < text_.size() && text_[pos_] == expected) {
            ++pos_;
            return true;
        }
        return false;
    }

    bool parse_string(std::string& out) {
        if (!consume('"')) {
            return false;
        }
        out.clear();
        while (pos_ < text_.size()) {
            char c = text_[pos_++];
            if (c == '"') {
                return true;
            }
            if (c != '\\') {
                out.push_back(c);
                continue;
            }
            if (pos_ >= text_.size()) {
                return false;
            }
            char esc = text_[pos_++];
            switch (esc) {
                case '"': out.push_back('"'); break;
                case '\\': out.push_back('\\'); break;
                case '/': out.push_back('/'); break;
                case 'b': out.push_back('\b'); break;
                case 'f': out.push_back('\f'); break;
                case 'n': out.push_back('\n'); break;
                case 'r': out.push_back('\r'); break;
                case 't': out.push_back('\t'); break;
                case 'u': {
                    unsigned int code = 0;
                    if (!read_hex4(code)) {
                        return false;
                    }
                    // Encode the BMP code point as UTF-8; surrogate halves are
                    // replaced rather than emitting invalid UTF-8.
                    if (code >= 0xD800 && code <= 0xDFFF) {
                        out.push_back('?');
                    } else if (code < 0x80) {
                        out.push_back(static_cast<char>(code));
                    } else if (code < 0x800) {
                        out.push_back(static_cast<char>(0xC0 | (code >> 6)));
                        out.push_back(static_cast<char>(0x80 | (code & 0x3F)));
                    } else {
                        out.push_back(static_cast<char>(0xE0 | (code >> 12)));
                        out.push_back(static_cast<char>(0x80 | ((code >> 6) & 0x3F)));
                        out.push_back(static_cast<char>(0x80 | (code & 0x3F)));
                    }
                    break;
                }
                default:
                    return false;
            }
        }
        return false;
    }

    bool read_hex4(unsigned int& out) {
        out = 0;
        for (int i = 0; i < 4; ++i) {
            if (pos_ >= text_.size()) {
                return false;
            }
            char c = text_[pos_++];
            out <<= 4;
            if (c >= '0' && c <= '9') {
                out |= static_cast<unsigned int>(c - '0');
            } else if (c >= 'a' && c <= 'f') {
                out |= static_cast<unsigned int>(c - 'a' + 10);
            } else if (c >= 'A' && c <= 'F') {
                out |= static_cast<unsigned int>(c - 'A' + 10);
            } else {
                return false;
            }
        }
        return true;
    }

    // Scalars come back bare ("42", "true", "\"text\""); containers come back
    // as their original source text so `data` survives verbatim. Parsing is
    // recursive and strict, so `{"a":}` or `{"a" 1}` are rejected rather than
    // merely brace-balanced.
    bool parse_value(std::string& out) {
        if (pos_ >= text_.size()) {
            return false;
        }
        char c = text_[pos_];
        if (c == '"') {
            std::string decoded;
            if (!parse_string(decoded)) {
                return false;
            }
            out = decoded;
            return true;
        }
        if (c == '{' || c == '[') {
            return parse_container(out);
        }
        if (c == 't' || c == 'f' || c == 'n') {
            return capture_literal(out);
        }
        if (c == '-' || (c >= '0' && c <= '9')) {
            return capture_number(out);
        }
        return false;
    }

    bool parse_container(std::string& out) {
        const std::size_t start = pos_;
        const char open = text_[pos_];
        const char close = (open == '{') ? '}' : ']';
        ++pos_;

        skip_ws();
        if (consume(close)) {
            out = text_.substr(start, pos_ - start);
            return true;
        }

        while (true) {
            skip_ws();
            if (open == '{') {
                std::string key;
                if (!parse_string(key)) {
                    return false;
                }
                skip_ws();
                if (!consume(':')) {
                    return false;
                }
                skip_ws();
            }
            std::string value;
            if (!parse_value(value)) {
                return false;
            }
            skip_ws();
            if (consume(',')) {
                continue;
            }
            if (consume(close)) {
                break;
            }
            return false;
        }

        out = text_.substr(start, pos_ - start);
        return true;
    }

    bool capture_literal(std::string& out) {
        for (const char* literal : {"true", "false", "null"}) {
            const std::string token(literal);
            if (text_.compare(pos_, token.size(), token) == 0) {
                out = token;
                pos_ += token.size();
                return true;
            }
        }
        return false;
    }

    bool capture_number(std::string& out) {
        const std::size_t start = pos_;
        consume('-');

        const auto consume_digits = [this]() {
            bool any = false;
            while (pos_ < text_.size() && text_[pos_] >= '0' && text_[pos_] <= '9') {
                ++pos_;
                any = true;
            }
            return any;
        };

        if (!consume_digits()) {
            pos_ = start;
            return false;
        }
        if (consume('.')) {
            if (!consume_digits()) {
                pos_ = start;
                return false;
            }
        }
        if (pos_ < text_.size() && (text_[pos_] == 'e' || text_[pos_] == 'E')) {
            ++pos_;
            if (!consume('+')) {
                consume('-');
            }
            if (!consume_digits()) {
                pos_ = start;
                return false;
            }
        }

        out = text_.substr(start, pos_ - start);
        return true;
    }

    const std::string& text_;
    std::size_t pos_ = 0;
};

bool parse_uint64(const std::string& text, uint64_t& out) {
    if (text.empty()) {
        return false;
    }
    std::size_t index = 0;
    uint64_t value = 0;
    while (index < text.size()) {
        char c = text[index];
        if (c < '0' || c > '9') {
            return false;
        }
        value = value * 10 + static_cast<uint64_t>(c - '0');
        ++index;
    }
    out = value;
    return true;
}

}  // namespace

Message Message::from_json(const std::string& json) {
    Message msg;
    msg.valid = false;

    std::map<std::string, std::string> fields;
    JsonReader reader(json);
    if (!reader.parse_object(fields)) {
        return msg;
    }

    if (const auto it = fields.find("type"); it != fields.end()) {
        uint64_t raw = 0;
        if (parse_uint64(it->second, raw) && raw <= static_cast<uint64_t>(MessageType::ERROR)) {
            msg.type = static_cast<MessageType>(raw);
        } else {
            return msg;
        }
    } else {
        return msg;
    }

    auto take = [&fields](const char* key, std::string& target) {
        if (const auto it = fields.find(key); it != fields.end()) {
            target = it->second;
        }
    };

    take("source", msg.source_process);
    take("target", msg.target_process);
    take("command", msg.command);
    take("data", msg.data);
    take("message_id", msg.message_id);

    uint64_t timestamp = 0;
    if (const auto it = fields.find("timestamp"); it != fields.end()) {
        if (parse_uint64(it->second, timestamp)) {
            msg.timestamp = timestamp;
        } else {
            return msg;
        }
    }

    msg.valid = true;
    return msg;
}

// ============================================================
// FileIPCChannel Implementation
// ============================================================

FileIPCChannel::FileIPCChannel(const std::string& ipc_path) 
    : ipc_file_path(ipc_path),
      response_file_path(ipc_path + ".response") {
}

bool FileIPCChannel::initialize() {
    // Ensure directory exists
    return true;
}

bool FileIPCChannel::send(const Message& msg) {
    std::lock_guard<std::mutex> lock(file_mutex);
    
    std::ofstream file(ipc_file_path);
    if (!file.is_open()) {
        return false;
    }
    
    file << msg.to_json();
    file.close();
    return true;
}

bool FileIPCChannel::receive(Message& msg, int timeout_ms) {
    auto start = std::chrono::steady_clock::now();
    
    while (true) {
        std::ifstream file(response_file_path);
        if (file.is_open()) {
            std::string json((std::istreambuf_iterator<char>(file)),
                           std::istreambuf_iterator<char>());
            file.close();
            
            if (!json.empty()) {
                msg = Message::from_json(json);
                // Delete the response file either way: leaving malformed input
                // in place made the reader return the same bad bytes forever.
                std::remove(response_file_path.c_str());
                return true;
            }
        }
        
        auto elapsed = std::chrono::duration_cast<std::chrono::milliseconds>(
            std::chrono::steady_clock::now() - start
        ).count();
        
        if (elapsed >= timeout_ms) {
            return false;
        }
        
        std::this_thread::sleep_for(std::chrono::milliseconds(50));
    }
}

void FileIPCChannel::close() {
    std::remove(ipc_file_path.c_str());
    std::remove(response_file_path.c_str());
}

// ============================================================
// StdioChannel Implementation
// ============================================================

StdioChannel::StdioChannel() 
    : stdin_pipe(nullptr), stdout_pipe(nullptr), stderr_pipe(nullptr) {
}

bool StdioChannel::initialize() {
#ifdef _WIN32
    SECURITY_ATTRIBUTES sa;
    sa.nLength = sizeof(SECURITY_ATTRIBUTES);
    sa.bInheritHandle = TRUE;
    sa.lpSecurityDescriptor = NULL;
    
    HANDLE stdin_read, stdin_write;
    HANDLE stdout_read, stdout_write;
    HANDLE stderr_read, stderr_write;
    
    if (!CreatePipe(&stdin_read, &stdin_write, &sa, 0)) return false;
    if (!CreatePipe(&stdout_read, &stdout_write, &sa, 0)) return false;
    if (!CreatePipe(&stderr_read, &stderr_write, &sa, 0)) return false;
    
    // Ensure the write handle to stdin is not inherited
    SetHandleInformation(stdin_write, HANDLE_FLAG_INHERIT, 0);
    SetHandleInformation(stdout_read, HANDLE_FLAG_INHERIT, 0);
    SetHandleInformation(stderr_read, HANDLE_FLAG_INHERIT, 0);
    
    stdin_pipe = stdin_write;
    stdout_pipe = stdout_read;
    stderr_pipe = stderr_read;
    
    return true;
#else
    int stdin_fds[2], stdout_fds[2], stderr_fds[2];
    
    if (pipe(stdin_fds) != 0) return false;
    if (pipe(stdout_fds) != 0) return false;
    if (pipe(stderr_fds) != 0) return false;
    
    stdin_pipe = (void*)(intptr_t)stdin_fds[1];
    stdout_pipe = (void*)(intptr_t)stdout_fds[0];
    stderr_pipe = (void*)(intptr_t)stderr_fds[0];
    
    return true;
#endif
}

bool StdioChannel::send(const Message& msg) {
    std::string json = msg.to_json() + "\n";
    
#ifdef _WIN32
    DWORD written;
    return WriteFile(stdin_pipe, json.c_str(), static_cast<DWORD>(json.length()), &written, NULL);
#else
    int fd = (int)(intptr_t)stdin_pipe;
    return write(fd, json.c_str(), json.length()) > 0;
#endif
}

bool StdioChannel::receive(Message& msg, int timeout_ms) {
    (void)timeout_ms;
    // Non-blocking read with timeout
    char buffer[4096];
    
#ifdef _WIN32
    DWORD available;
    if (!PeekNamedPipe(stdout_pipe, NULL, 0, NULL, &available, NULL)) {
        return false;
    }
    
    if (available > 0) {
        DWORD read;
        if (ReadFile(stdout_pipe, buffer, sizeof(buffer) - 1, &read, NULL)) {
            buffer[read] = '\0';
            msg = Message::from_json(buffer);
            return true;
        }
    }
#else
    // Use select for timeout
    fd_set readfds;
    struct timeval tv;
    int fd = (int)(intptr_t)stdout_pipe;
    
    FD_ZERO(&readfds);
    FD_SET(fd, &readfds);
    
    tv.tv_sec = timeout_ms / 1000;
    tv.tv_usec = (timeout_ms % 1000) * 1000;
    
    int ret = select(fd + 1, &readfds, NULL, NULL, &tv);
    if (ret > 0) {
        ssize_t n = read(fd, buffer, sizeof(buffer) - 1);
        if (n > 0) {
            buffer[n] = '\0';
            msg = Message::from_json(buffer);
            return true;
        }
    }
#endif
    
    return false;
}

void StdioChannel::close() {
#ifdef _WIN32
    if (stdin_pipe) CloseHandle(stdin_pipe);
    if (stdout_pipe) CloseHandle(stdout_pipe);
    if (stderr_pipe) CloseHandle(stderr_pipe);
#else
    if (stdin_pipe) ::close((int)(intptr_t)stdin_pipe);
    if (stdout_pipe) ::close((int)(intptr_t)stdout_pipe);
    if (stderr_pipe) ::close((int)(intptr_t)stderr_pipe);
#endif
}

// ============================================================
// MessageValidator Implementation
// ============================================================

bool MessageValidator::validate_message(const Message& msg, std::string& error) {
    if (msg.source_process.empty()) {
        error = "Source process is empty";
        return false;
    }
    
    if (msg.target_process.empty()) {
        error = "Target process is empty";
        return false;
    }
    
    if (msg.command.empty()) {
        error = "Command is empty";
        return false;
    }
    
    if (!is_safe_json(msg.data)) {
        error = "Invalid JSON data";
        return false;
    }
    
    return true;
}

bool MessageValidator::is_safe_json(const std::string& json) {
    // Reject anything oversized, then require that it actually parses: the
    // previous implementation accepted every string, including truncated and
    // hostile payloads that later code would treat as a command.
    if (json.length() > 1024 * 1024) {  // 1MB limit
        return false;
    }
    JsonReader reader(json);
    return reader.parse_document();
}

std::map<std::string, std::vector<std::chrono::steady_clock::time_point>>&
MessageValidator::rate_history() {
    static std::map<std::string, std::vector<std::chrono::steady_clock::time_point>> history;
    return history;
}

bool MessageValidator::check_rate_limit(const std::string& source, int max_per_second) {
    if (max_per_second <= 0) {
        return true;  // unlimited, explicitly requested
    }

    static std::mutex rate_mutex;
    std::lock_guard<std::mutex> lock(rate_mutex);

    auto& history = rate_history();
    const auto now = std::chrono::steady_clock::now();
    auto& stamps = history[source];

    stamps.erase(
        std::remove_if(stamps.begin(), stamps.end(),
                       [now](const std::chrono::steady_clock::time_point& stamp) {
                           return now - stamp > std::chrono::seconds(1);
                       }),
        stamps.end());

    if (static_cast<int>(stamps.size()) >= max_per_second) {
        return false;
    }
    stamps.push_back(now);
    return true;
}

// ============================================================
// MessageRouter Implementation
// ============================================================

void MessageRouter::register_handler(const std::string& command, 
                                     std::function<void(const Message&)> handler) {
    std::lock_guard<std::mutex> lock(router_mutex);
    handlers[command].push_back(handler);
}

void MessageRouter::route_message(const Message& msg) {
    std::lock_guard<std::mutex> lock(router_mutex);
    
    auto it = handlers.find(msg.command);
    if (it != handlers.end()) {
        for (auto& handler : it->second) {
            handler(msg);
        }
    }
}

void MessageRouter::unregister_all() {
    std::lock_guard<std::mutex> lock(router_mutex);
    handlers.clear();
}

// ============================================================
// ProcessManager Implementation
// ============================================================

ProcessManager::ProcessManager() {
    router = std::make_unique<MessageRouter>();
}

ProcessManager::~ProcessManager() {
    shutdown();
}

bool ProcessManager::register_process(const ProcessConfig& config) {
    std::lock_guard<std::mutex> lock(manager_mutex);
    
    if (processes.find(config.name) != processes.end()) {
        std::cerr << "Process " << config.name << " already registered" << std::endl;
        return false;
    }
    
    ProcessInfo info;
    info.config = config;
    info.state = ProcessState::CREATED;
    info.platform_handle = nullptr;
    info.pid = 0;
    
    processes[config.name] = info;
    
    // Create communication channels
    for (auto method : config.comm_methods) {
        std::unique_ptr<ICommChannel> channel;
        
        switch (method) {
            case CommMethod::FILE_IPC:
                channel = std::make_unique<FileIPCChannel>(
                    "ipc_" + config.name + ".json"
                );
                break;
            case CommMethod::STDIO:
                channel = std::make_unique<StdioChannel>();
                break;
            default:
                // NAMED_PIPE / SHARED_MEMORY / TCP_SOCKET are declared in the
                // header but not implemented yet; report them instead of
                // silently registering no channel at all.
                std::cerr << "Communication method not implemented for "
                          << config.name << std::endl;
                break;
        }
        
        if (channel && channel->initialize()) {
            channels[config.name + "_" + std::to_string(static_cast<int>(method))] 
                = std::move(channel);
        }
    }
    
    std::cout << "Registered process: " << config.name << std::endl;
    return true;
}

bool ProcessManager::spawn_process(ProcessInfo& info) {
#ifdef _WIN32
    STARTUPINFOA si;
    PROCESS_INFORMATION pi;
    
    ZeroMemory(&si, sizeof(si));
    si.cb = sizeof(si);
    ZeroMemory(&pi, sizeof(pi));
    
    // Build command line
    std::string cmdline = info.config.executable_path;
    for (const auto& arg : info.config.args) {
        cmdline += " " + arg;
    }
    
    // Set environment variables. CreateProcessA needs a double-NUL terminated
    // "KEY=VALUE\0KEY=VALUE\0\0" block; passing NULL silently dropped every
    // configured variable (including NEURO_* settings the child needs).
    std::string env_block;
    for (const auto& [key, value] : info.config.env_vars) {
        if (key.empty()) {
            continue;
        }
        env_block += key + "=" + value;
        env_block.push_back('\0');
    }
    env_block.push_back('\0');

    if (!CreateProcessA(
        NULL,
        const_cast<char*>(cmdline.c_str()),
        NULL,
        NULL,
        FALSE,
        0,
        info.config.env_vars.empty() ? NULL : const_cast<char*>(env_block.data()),
        NULL,
        &si,
        &pi
    )) {
        info.last_error = "Failed to create process";
        return false;
    }
    
    info.platform_handle = pi.hProcess;
    info.pid = pi.dwProcessId;
    CloseHandle(pi.hThread);
    
#else
    pid_t pid = fork();
    
    if (pid < 0) {
        info.last_error = "Fork failed";
        return false;
    }
    
    if (pid == 0) {
        // Child process
        std::vector<char*> args;
        args.push_back(const_cast<char*>(info.config.executable_path.c_str()));
        for (auto& arg : info.config.args) {
            args.push_back(const_cast<char*>(arg.c_str()));
        }
        args.push_back(nullptr);

        // Apply the configured environment before exec; the old code ignored
        // env_vars entirely on POSIX and inherited the parent's environment.
        for (const auto& [key, value] : info.config.env_vars) {
            if (!key.empty()) {
                setenv(key.c_str(), value.c_str(), 1);
            }
        }
        
        execvp(args[0], args.data());
        exit(1); // If exec fails
    }
    
    // Parent process
    info.platform_handle = (void*)(intptr_t)pid;
    info.pid = pid;
#endif
    
    info.state = ProcessState::RUNNING;
    info.start_time = std::chrono::system_clock::now();
    info.last_heartbeat = std::chrono::system_clock::now();
    
    return true;
}

bool ProcessManager::start_process(const std::string& name) {
    std::lock_guard<std::mutex> lock(manager_mutex);
    
    auto it = processes.find(name);
    if (it == processes.end()) {
        std::cerr << "Process " << name << " not found" << std::endl;
        return false;
    }
    
    auto& info = it->second;
    
    // Check dependencies
    if (!check_dependencies_ready(info.config)) {
        std::cerr << "Dependencies not ready for " << name << std::endl;
        return false;
    }
    
    info.state = ProcessState::STARTING;
    
    if (!spawn_process(info)) {
        info.state = ProcessState::CRASHED;
        return false;
    }
    
    std::cout << "Started process: " << name << " (PID: " << info.pid << ")" << std::endl;
    
    // Start monitoring thread (owned; joined in shutdown()).
    monitor_threads.emplace_back([this, name]() {
        monitor_process(name);
    });
    
    return true;
}

bool ProcessManager::check_dependencies_ready(const ProcessConfig& config) {
    for (const auto& dep : config.depends_on) {
        auto it = processes.find(dep);
        if (it == processes.end() || it->second.state != ProcessState::RUNNING) {
            return false;
        }
    }
    return true;
}

void ProcessManager::monitor_process(const std::string& name) {
    while (!shutting_down) {
        std::this_thread::sleep_for(std::chrono::seconds(1));

        bool alive = true;
        bool heartbeat_late = false;
        bool track_heartbeat = false;
        std::chrono::seconds heartbeat_timeout{0};

        {
            // Only read state under the lock; recovery re-acquires it and must
            // never be called with the lock held (handle_process_crash ->
            // restart_process -> stop_process/start_process would self-deadlock).
            std::lock_guard<std::mutex> lock(manager_mutex);
            auto it = processes.find(name);
            if (it == processes.end()) {
                break;
            }

            auto& info = it->second;
            if (info.state != ProcessState::RUNNING) {
                break;  // stopped/restarted elsewhere; this monitor is done
            }

#ifdef _WIN32
            DWORD exit_code = STILL_ACTIVE;
            if (info.platform_handle != nullptr &&
                GetExitCodeProcess(info.platform_handle, &exit_code)) {
                alive = (exit_code == STILL_ACTIVE);
            }
#else
            if (info.pid > 0) {
                int status = 0;
                const pid_t result = waitpid(info.pid, &status, WNOHANG);
                if (result == static_cast<pid_t>(info.pid)) {
                    alive = false;
                } else if (result < 0) {
                    // Already reaped elsewhere (restart/stop) - not a crash.
                    alive = true;
                }
            }
#endif

            track_heartbeat = info.config.enable_heartbeat && health_monitoring;
            heartbeat_timeout = info.config.heartbeat_timeout;
            if (track_heartbeat) {
                const auto elapsed = std::chrono::duration_cast<std::chrono::seconds>(
                    std::chrono::system_clock::now() - info.last_heartbeat);
                heartbeat_late = elapsed > heartbeat_timeout;
            }
        }

        if (!alive) {
            handle_process_crash(name);
            break;
        }

        if (heartbeat_late) {
            std::cerr << "Process " << name << " heartbeat timeout" << std::endl;
            // The child is still alive; it just stopped answering. Terminate it
            // instead of leaking a running process the manager has written off.
            handle_process_crash(name, /*terminate_child=*/true);
            break;
        }
    }
}

void ProcessManager::handle_process_crash(const std::string& name, bool terminate_child) {
    bool should_restart = false;
    std::chrono::seconds restart_delay{0};

    {
        std::lock_guard<std::mutex> lock(manager_mutex);
        auto it = processes.find(name);
        if (it == processes.end()) {
            return;
        }
        auto& info = it->second;
        info.state = ProcessState::CRASHED;
        info.last_error = terminate_child ? "Heartbeat timeout"
                                          : "Process exited unexpectedly";

        if (terminate_child && info.pid > 0) {
#ifdef _WIN32
            if (info.platform_handle != nullptr) {
                TerminateProcess(info.platform_handle, 1);
                CloseHandle(info.platform_handle);
            }
#else
            kill(info.pid, SIGKILL);
            waitpid(info.pid, nullptr, 0);  // reap; no zombies
#endif
            info.platform_handle = nullptr;
            info.pid = 0;
        }

        should_restart = !shutting_down && info.config.auto_restart &&
                         info.restart_count < info.config.max_restart_attempts;
        restart_delay = info.config.restart_delay;
    }

    std::cerr << "Process " << name << " crashed!" << std::endl;

    if (!should_restart) {
        return;
    }

    std::cout << "Attempting to restart " << name << "..." << std::endl;
    std::this_thread::sleep_for(restart_delay);

    {
        std::lock_guard<std::mutex> lock(manager_mutex);
        auto it = processes.find(name);
        if (it == processes.end()) {
            return;
        }
        it->second.restart_count += 1;
    }

    // restart_process() takes the lock itself; safe because we released it.
    restart_process(name);
}

bool ProcessManager::stop_process(const std::string& name, bool force) {
    std::lock_guard<std::mutex> lock(manager_mutex);
    
    auto it = processes.find(name);
    if (it == processes.end()) return false;
    
    auto& info = it->second;
    info.state = ProcessState::STOPPING;

    // Graceful shutdown first: ask the child to exit over its own channel.
    // (Previously nothing was ever sent, so every stop was a hard kill.)
    if (!force && info.config.enable_heartbeat) {
        Message goodbye;
        goodbye.type = MessageType::SHUTDOWN;
        goodbye.source_process = "process_handler";
        goodbye.target_process = name;
        goodbye.command = "shutdown";
        goodbye.data = "{}";
        goodbye.timestamp = static_cast<uint64_t>(
            std::chrono::duration_cast<std::chrono::milliseconds>(
                std::chrono::system_clock::now().time_since_epoch()
            ).count()
        );
        goodbye.message_id = "shutdown_" + name;
        for (auto method : info.config.comm_methods) {
            const std::string channel_key = name + "_" + std::to_string(static_cast<int>(method));
            auto channel_it = channels.find(channel_key);
            if (channel_it != channels.end() && channel_it->second->send(goodbye)) {
                break;
            }
        }
    }

#ifdef _WIN32
    if (force) {
        TerminateProcess(info.platform_handle, 1);
    } else {
        WaitForSingleObject(info.platform_handle, 5000);
        TerminateProcess(info.platform_handle, 0);
    }
    CloseHandle(info.platform_handle);
#else
    if (force) {
        kill(info.pid, SIGKILL);
    } else {
        kill(info.pid, SIGTERM);
        // Wait a bit
        sleep(2);
        kill(info.pid, SIGKILL);
    }
#endif
    
    info.state = ProcessState::STOPPED;
    info.platform_handle = nullptr;
    info.pid = 0;
    
    return true;
}

bool ProcessManager::restart_process(const std::string& name) {
    stop_process(name, false);
    std::this_thread::sleep_for(std::chrono::milliseconds(500));
    return start_process(name);
}

bool ProcessManager::send_message(const std::string& target, const Message& msg) {
    std::lock_guard<std::mutex> lock(manager_mutex);

    auto proc_it = processes.find(target);
    if (proc_it == processes.end()) {
        return false;
    }

    const auto& info = proc_it->second;
    for (auto method : info.config.comm_methods) {
        const std::string channel_key = target + "_" + std::to_string(static_cast<int>(method));
        auto channel_it = channels.find(channel_key);
        if (channel_it != channels.end() && channel_it->second->send(msg)) {
            return true;
        }
    }

    return false;
}

bool ProcessManager::broadcast_message(const Message& msg) {
    std::lock_guard<std::mutex> lock(manager_mutex);

    bool sent_any = false;
    for (const auto& [name, info] : processes) {
        if (info.state != ProcessState::RUNNING) {
            continue;
        }

        Message target_msg = msg;
        target_msg.target_process = name;

        for (auto method : info.config.comm_methods) {
            const std::string channel_key = name + "_" + std::to_string(static_cast<int>(method));
            auto channel_it = channels.find(channel_key);
            if (channel_it != channels.end() && channel_it->second->send(target_msg)) {
                sent_any = true;
                break;
            }
        }
    }

    return sent_any;
}

void ProcessManager::register_message_handler(
    const std::string& command,
    std::function<void(const Message&)> handler
) {
    if (router) {
        router->register_handler(command, std::move(handler));
    }
}

void ProcessManager::send_heartbeat_check(const std::string& name) {
    Message heartbeat;
    heartbeat.type = MessageType::HEARTBEAT;
    heartbeat.source_process = "process_handler";
    heartbeat.target_process = name;
    heartbeat.command = "heartbeat";
    heartbeat.data = "{}";
    heartbeat.timestamp = static_cast<uint64_t>(
        std::chrono::duration_cast<std::chrono::milliseconds>(
            std::chrono::system_clock::now().time_since_epoch()
        ).count()
    );
    heartbeat.message_id = "hb_" + name;

    send_message(name, heartbeat);
}

void ProcessManager::enable_health_monitoring(bool enable) {
    health_monitoring = enable;
}

std::string ProcessManager::get_health_report() {
    std::lock_guard<std::mutex> lock(manager_mutex);

    std::ostringstream oss;
    oss << "Process Health Report\n";
    for (const auto& [name, info] : processes) {
        oss << "- " << name
            << " state=" << static_cast<int>(info.state)
            << " pid=" << info.pid
            << " restarts=" << info.restart_count
            << "\n";
    }
    return oss.str();
}

void ProcessManager::start_all() {
    // Start processes in dependency order
    bool progress = true;
    while (progress) {
        progress = false;
        for (auto& [name, info] : processes) {
            if (info.state == ProcessState::CREATED) {
                if (check_dependencies_ready(info.config)) {
                    start_process(name);
                    progress = true;
                }
            }
        }
    }
}

void ProcessManager::stop_all() {
    std::vector<std::string> names;
    {
        std::lock_guard<std::mutex> lock(manager_mutex);
        for (const auto& [name, info] : processes) {
            if (info.state == ProcessState::RUNNING) {
                names.push_back(name);
            }
        }
    }

    for (const auto& name : names) {
        stop_process(name);
    }
}

void ProcessManager::run() {
    running = true;
    start_all();
    
    // Main event loop
    while (running) {
        // Iterating the channel map directly raced with register_process()
        // (which inserts into it). Snapshot the raw channel pointers under the
        // lock instead: entries outlive the loop, only shutdown() clears them.
        std::vector<ICommChannel*> active;
        {
            std::lock_guard<std::mutex> lock(manager_mutex);
            active.reserve(channels.size());
            for (auto& [key, channel] : channels) {
                (void)key;
                active.push_back(channel.get());
            }
        }

        for (ICommChannel* channel : active) {
            Message msg;
            if (!channel->receive(msg, 100)) {
                continue;
            }
            if (!msg.valid) {
                std::cerr << "Dropped unparseable message" << std::endl;
                continue;
            }
            std::string error;
            if (MessageValidator::validate_message(msg, error)) {
                router->route_message(msg);
            } else {
                std::cerr << "Invalid message: " << error << std::endl;
            }
        }
        
        std::this_thread::sleep_for(std::chrono::milliseconds(10));
    }
}

void ProcessManager::shutdown() {
    running = false;
    shutting_down = true;
    stop_all();

    // Wait for every monitor thread (including any spawned by a restart while
    // we were stopping) so nothing outlives the manager.
    while (true) {
        std::thread worker;
        {
            std::lock_guard<std::mutex> lock(manager_mutex);
            if (monitor_threads.empty()) {
                break;
            }
            worker = std::move(monitor_threads.back());
            monitor_threads.pop_back();
        }
        if (worker.joinable()) {
            worker.join();
        }
    }

    std::lock_guard<std::mutex> lock(manager_mutex);
    for (auto& [name, channel] : channels) {
        (void)name;
        channel->close();
    }

    channels.clear();
    processes.clear();
}

ProcessState ProcessManager::get_process_state(const std::string& name) {
    std::lock_guard<std::mutex> lock(manager_mutex);
    auto it = processes.find(name);
    return it != processes.end() ? it->second.state : ProcessState::STOPPED;
}

std::vector<ProcessInfo> ProcessManager::get_all_processes() {
    std::lock_guard<std::mutex> lock(manager_mutex);
    std::vector<ProcessInfo> result;
    for (const auto& [name, info] : processes) {
        result.push_back(info);
    }
    return result;
}

} // namespace neuro
