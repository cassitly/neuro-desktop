# Neuro Desktop

> **An AI-powered desktop control system that gives Neuro-sama the ability to control a computer through natural language commands.**

[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)
[![Build Status](https://img.shields.io/badge/build-passing-brightgreen.svg)]()
[![Version](https://img.shields.io/badge/version-0.0.3b--dev-blue.svg)]()

## Table of Contents

- [Overview](#overview)
- [Features](#features)
- [Architecture](#architecture)
- [Quick Start](#quick-start)
- [Installation](#installation)
- [Usage](#usage)
- [Development](#development)
- [Documentation](#documentation)
- [Contributing](#contributing)
- [License](#license)

## Overview

Neuro Desktop is a multi-language integration system that enables [Neuro-sama](https://twitch.tv/vedal987) to interact with desktop environments through a sophisticated action scripting language. The system bridges AI decision-making with real-world computer control through a carefully designed architecture that prioritizes safety, reliability, and human-like interaction patterns.

### What Makes Neuro Desktop Special?

- **Multi-Language Architecture**: Combines Rust (system integration), Go (API communication), Python (cross-platform control), and C++ (process management) for optimal performance
- **Human-Like Mouse Movement**: Advanced algorithmic pathfinding that mimics natural human mouse movements with Bézier curves and Perlin noise
- **Powerful Script Language**: Simple yet expressive action scripting for complex automation tasks
- **Automatic Recovery**: Built-in crash detection and automatic process restart capabilities
- **Cross-Platform**: Works on Windows, Linux, and macOS with platform-specific optimizations

## Features

### Core Capabilities

- 🖱️ **Mouse Control**: Precise cursor movement with human-like motion algorithms
- ⌨️ **Keyboard Control**: Text typing, key presses, shortcuts, and complex key combinations
- 📝 **Script Execution**: Multi-command action scripts for complex workflows
- 🔄 **Action Queuing**: Build and execute macro-like action sequences
- 📊 **Telemetry**: Comprehensive action history and desktop monitoring
- 🛡️ **Safety First**: Validation, rate limiting, and bounded execution

### Action Types

#### High-Level Actions
- **Mouse**: Move, click, drag, path drawing
- **Keyboard**: Type text, press keys, shortcuts
- **Scripts**: Multi-line automation sequences

#### Low-Level Controls
- **Direct API**: Fine-grained control over individual actions
- **Queue Management**: Build complex macros programmatically
- **Execution Control**: Execute now or queue for later

## Architecture

```
┌─────────────────────────────────────────────────────────┐
│                    Neuro API                            │
│                 (WebSocket Server)                      │
└────────────────────────┬────────────────────────────────┘
                         │
                         │ WebSocket (ws://localhost:8000)
                         │
┌────────────────────────▼────────────────────────────────┐
│              Go Integration Layer                       │
│  • Connects to Neuro API                               │
│  • Registers available actions                         │
│  • Translates Neuro commands → IPC                     │
└────────────────────────┬────────────────────────────────┘
                         │
                         │ File IPC (JSON)
                         │
┌────────────────────────▼────────────────────────────────┐
│              Rust Main Process                          │
│  • IPC Command Processor                               │
│  • Python FFI Bridge (PyO3)                            │
│  • Process Lifecycle Management                        │
└────────────────────────┬────────────────────────────────┘
                         │
                         │ PyO3 FFI
                         │
┌────────────────────────▼────────────────────────────────┐
│            Python Controller Layer                      │
│  • Cross-platform input control (pyautogui)            │
│  • Action script parser                                │
│  • Mouse pathfinding algorithms                        │
│  • Desktop telemetry                                   │
└─────────────────────────────────────────────────────────┘
```

### Component Breakdown

| Component | Language | Purpose |
|-----------|----------|---------|
| **neuro-integration** | Go | WebSocket client, action registry, IPC communication |
| **neuro-desktop** | Rust | Main orchestrator, IPC handler, Python FFI bridge |
| **controller** | Python | Cross-platform input control, script parsing |
| **frontend** | TypeScript | Web-based UI (planned feature) |

## Quick Start

### Prerequisites

```bash
# Rust 1.70+
curl --proto '=https' --tlsv1.2 -sSf https://sh.rustup.rs | sh

# Go 1.22+
# Download from https://go.dev/dl/

# Python 3.10+
python --version

# Node.js 18+ (for frontend)
node --version
```

### Installation

**Option 1: Pre-built Binaries** (Recommended)

1. Download the latest release from [Releases](https://github.com/Nakashireyumi/neuro-desktop/releases)
2. Extract the archive
3. Run `neuro-desktop.exe` (Windows) or `./neuro-desktop` (Linux/macOS)

**Option 2: Build from Source**

```bash
# Clone repository
git clone https://github.com/Nakashireyumi/neuro-desktop.git
cd neuro-desktop/desktop

# Run automated build
.\scripts\build-all.ps1  # Windows
./scripts/build-all.sh   # Linux/macOS
```

### First Run

```bash
# Windows
cd apps/neuro-desktop/target/release
.\neuro-desktop.exe

# Linux/macOS
cd apps/neuro-desktop/target/release
./neuro-desktop
```

The system will automatically:
1. ✅ Initialize Python controllers
2. ✅ Start IPC handler
3. ✅ Launch Go integration
4. ✅ Connect to Neuro API (default: `ws://localhost:8000`)

## Usage

### Basic Example

Once running, Neuro can execute commands like:

```javascript
// Move mouse to center of screen
{
  "action": "move_mouse_to",
  "params": { "x": 960, "y": 540 }
}

// Type text
{
  "action": "type_text",
  "params": { "text": "Hello from Neuro!" }
}

// Execute complex script
{
  "action": "run_script",
  "params": {
    "script": `
      TYPE "notepad"
      ENTER
      WAIT 1
      TYPE "Hello World!"
    `
  }
}
```

### Action Script Language

The script language supports powerful multi-command sequences:

```text
# Open application
SHORTCUT win
TYPE "notepad"
ENTER
WAIT 1

# Type content
TYPE "Dear User,"
ENTER
TYPE "This is Neuro!"
ENTER

# Save file
SHORTCUT ctrl s
WAIT 0.5
TYPE "neuro_message.txt"
ENTER
```

See [Action Script Documentation](docs/action_script/LANGUAGE_REFERENCE.md) for complete reference.

### Configuration

Edit `config/integration-config.yml`:

```yaml
connection:
  neuro-backend: "ws://localhost:8000"
  
package:
  name: "neuro-desktop"
  version: "0.0.3b-dev"
```

Or use environment variables:

```bash
# Windows
$env:NEURO_SDK_WS_URL = "ws://localhost:8000"
$env:NEURO_IPC_FILE = "./neuro_ipc.json"
$env:NEURO_PERMISSIONS_FILE = "./desktop/apps/neuro-integration/permissions.example.json"
$env:NEURO_RELAY_ENABLED = "true"
$env:NEURO_RELAY_EMULATED_ADDR = "127.0.0.1:8001"
$env:NEURO_RELAY_NAME = "Neuro Desktop Hub"
$env:NEURO_CATALOG_FILE = "./desktop/catalog/index.json"

# Linux/macOS
export NEURO_SDK_WS_URL="ws://localhost:8000"
export NEURO_IPC_FILE="./neuro_ipc.json"
export NEURO_PERMISSIONS_FILE="./desktop/apps/neuro-integration/permissions.example.json"
export NEURO_RELAY_ENABLED="true"
export NEURO_RELAY_EMULATED_ADDR="127.0.0.1:8001"
export NEURO_RELAY_NAME="Neuro Desktop Hub"
export NEURO_CATALOG_FILE="./desktop/catalog/index.json"
```

Use [`permissions.example.json`](desktop/apps/neuro-integration/permissions.example.json) as a starting policy.
Set `NEURO_RELAY_BINARY` to an explicit relay executable path if the binary is not in the same folder as `neuro-desktop.exe`.

## Development

### Docker Modular Tests

Run modular tests in Docker:

```bash
docker compose -f docker-compose.tests.yml run --rm go-integration-tests
docker compose -f docker-compose.tests.yml run --rm python-parser-tests
```

### Optional Relay Build/Bundling

If you have Neuro Relay source locally, set:

```bash
# PowerShell
$env:NEURO_RELAY_SOURCE_DIR = "C:\\path\\to\\neuro-relay"

# bash
export NEURO_RELAY_SOURCE_DIR="/path/to/neuro-relay"
```

Then run:

```bash
cd desktop
./scripts/build-all.ps1
```

The build script will compile relay and pass it to the bundle scripts automatically.

### Project Structure

```
desktop/
├── apps/
│   ├── neuro-desktop/          # Main Rust application
│   │   └── src/
│   │       ├── main.rs          # Entry point
│   │       ├── controller.rs    # Python FFI bridge
│   │       ├── ipc_handler.rs   # IPC command processor
│   │       └── go_manager.rs    # Go process manager
│   │
│   └── neuro-integration/      # Go WebSocket client
│       ├── main.go
│       ├── action-handling.go
│       ├── action-registry.go
│       └── types.go
│
├── backend/python/controller/  # Python control drivers
│   ├── lib.py                  # Entry point
│   ├── actions.py              # Script parser
│   ├── controls/
│   │   ├── mouse.py            # Mouse controller
│   │   └── keyboard.py         # Keyboard controller
│   └── libraries/
│       └── mouse_pathfinder.py # Human-like motion
│
├── frontend/                   # Web UI (TypeScript/Vite)
├── config/                     # Configuration files
├── scripts/                    # Build and bundle scripts
└── docs/                       # Documentation
```

### Development Workflow

```bash
# 1. Setup development environment
.\scripts\setup-dev.ps1

# 2. Build all components
make all

# 3. Run in development mode
.\scripts\bundle\dev.ps1

# 4. Run tests
cargo test                      # Rust tests
go test ./...                   # Go tests
pytest backend/python/          # Python tests

# 5. Build production bundle
.\scripts\bundle\prod.ps1
```

### Adding New Actions

1. **Define action schema** in `action-registry.go`:

```go
var MyActionSchema = ActionDefinition{
    Name: "my_action",
    Description: "Does something cool",
    Schema: map[string]interface{}{
        "type": "object",
        "properties": map[string]interface{}{
            "param1": map[string]interface{}{
                "type": "string",
                "description": "A parameter",
            },
        },
        "required": []string{"param1"},
    },
}
```

2. **Handle action** in `action-handling.go`:

```go
case string(CmdMyAction):
    param1, _ := params["param1"].(string)
    cmd = IPCCommand{
        Type: CmdMyAction,
        Params: map[string]interface{}{
            "param1": param1,
        },
    }
```

3. **Implement in Rust** (`ipc_handler.rs`):

```rust
IPCCommand::MyAction { params } => {
    controller.my_action(&params.param1)
}
```

4. **Add Python implementation** if needed (`controller/`).

### Testing with Randy

Randy is a mock Neuro API server for testing:

```bash
# Terminal 1: Start Randy
cd Randy
npm install
npm start

# Terminal 2: Run Neuro Desktop
cd apps/neuro-desktop/target/release
.\neuro-desktop.exe
```

Randy will send random actions to test your integration.

## Documentation

- 📖 [Action Script Language Reference](docs/action_script/LANGUAGE_REFERENCE.md)
- 🏗️ [Architecture Deep Dive](docs/ARCHITECTURE.md)
- 🔧 [API Specification](desktop/apps/neuro-integration/integration-docs/Action Script Documentation.md)
- 🚀 [Deployment Guide](docs/DEPLOYMENT.md)
- 🧪 [Testing Guide](tests/README.md)
- 🤝 [Contributing Guidelines](CONTRIBUTING.md)
- 🧭 [Project Vision](VISION.md)
- 🗺️ [Production TODO](docs/PRODUCTION_TODO.md)
- 📝 [Changelog](CHANGELOG.md)
- 🤝 [Code of Conduct](CODE_OF_CONDUCT.md)
- 🔐 [Security Policy](SECURITY.md)

## Troubleshooting

### Common Issues

**"Go integration binary not found"**
```bash
# Rebuild Go integration
cd apps/neuro-integration
go build -o neuro-integration.exe .
cp neuro-integration.exe ../neuro-desktop/target/release/
```

**"Failed to initialize Python controller"**
```bash
# Reinstall Python dependencies
cd backend/python
python -m venv .venv
.venv\Scripts\activate
pip install -r requirements.txt
```

**"WebSocket connection failed"**
```bash
# Check if Randy or Neuro API is running
curl ws://localhost:8000
# Or start Randy
cd Randy && npm start
```

See [Troubleshooting Guide](docs/TROUBLESHOOTING.md) for more solutions.

## Contributing

We welcome contributions! Please see [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines.

### Areas We Need Help

- 🐛 Bug reports and fixes
- 📝 Documentation improvements
- ✨ New action types
- 🧪 Test coverage
- 🌐 Cross-platform testing
- 🎨 UI/UX improvements

## Roadmap

- [ ] **v0.1.0**: Core functionality (current)
- [ ] **v0.2.0**: Enhanced safety features
- [ ] **v0.3.0**: Vision system integration
- [ ] **v0.4.0**: Advanced macro system
- [ ] **v1.0.0**: Production-ready release

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

## Acknowledgments

- **Neuro-sama** - The AI that makes this all worthwhile
- **Vedal** - Creator of Neuro-sama
- The community for testing and feedback

## Support

- 💬 [Discord](https://discord.gg/neuro)
- 🐛 [Issue Tracker](https://github.com/Nakashireyumi/neuro-desktop/issues)
- 📧 Email: support@neuro-desktop.dev

---

**Made with ❤️ by the Neuro Desktop Team**

*"Giving Neuro the keys to the desktop, one action at a time."*

