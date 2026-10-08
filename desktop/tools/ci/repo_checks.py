#!/usr/bin/env python3
"""Repository checks that do not depend on a language toolchain.

Run locally with:

    python3 desktop/tools/ci/repo_checks.py

Every check prints what it looked at, so a failure is actionable without reading
this file. The CI job runs exactly this script.
"""

from __future__ import annotations

import json
import os
import re
import shutil
import subprocess
import sys
import tempfile

REPO_ROOT = os.path.dirname(os.path.dirname(os.path.dirname(os.path.dirname(os.path.abspath(__file__)))))

SKIP_DIRS = {
    ".git",
    "node_modules",
    "dist",
    "build",
    "target",
    "__pycache__",
    ".venv",
    "venv",
}

# 2 MB: large enough for the dashboard bundle, small enough to catch a
# accidentally committed binary or dataset.
MAX_FILE_BYTES = 2 * 1024 * 1024

failures: list[str] = []


def fail(message: str) -> None:
    failures.append(message)
    print(f"FAIL  {message}")


def ok(message: str) -> None:
    print(f"ok    {message}")


def walk(root: str):
    for current, dirs, files in os.walk(root):
        dirs[:] = [d for d in dirs if d not in SKIP_DIRS]
        for name in files:
            yield os.path.join(current, name)


def _strip_jsonc(text: str) -> str:
    """Best-effort JSONC -> JSON: used by editors' config files."""
    text = re.sub(r"/\*.*?\*/", "", text, flags=re.S)
    text = re.sub(r"(?m)^\s*//.*$", "", text)
    text = re.sub(r",(\s*[}\]])", r"\1", text)
    return text


def check_json() -> None:
    """Every tracked JSON file must parse (JSONC tolerated for editor configs)."""
    count = 0
    for path in walk(REPO_ROOT):
        if not path.endswith(".json"):
            continue
        if f"{os.sep}.git{os.sep}" in path:
            continue
        count += 1
        with open(path, "r", encoding="utf-8") as handle:
            raw = handle.read()
        try:
            json.loads(raw)
        except json.JSONDecodeError as first_error:
            try:
                json.loads(_strip_jsonc(raw))
            except json.JSONDecodeError:
                fail(f"{os.path.relpath(path, REPO_ROOT)} is not valid JSON: {first_error}")
    ok(f"{count} JSON files parse")


def check_example_policies_match() -> None:
    """The two copies of the example policy must not drift apart."""
    first = os.path.join(REPO_ROOT, "desktop", "apps", "neuro-integration", "permissions.example.json")
    second = os.path.join(REPO_ROOT, "desktop", "config", "permissions.example.json")
    if not (os.path.exists(first) and os.path.exists(second)):
        fail("one of the permissions.example.json files is missing")
        return
    with open(first, "r", encoding="utf-8") as handle:
        a = json.load(handle)
    with open(second, "r", encoding="utf-8") as handle:
        b = json.load(handle)
    if a != b:
        fail("desktop/apps/neuro-integration/permissions.example.json and "
             "desktop/config/permissions.example.json have drifted apart")
        return
    ok("both example permission policies are identical")

    # A shipped example must not hand out a shell or allow launching programs.
    if "shell" in json.dumps(a).lower():
        scopes = a.get("scopes", {})
        shell = scopes.get("shell")
        if isinstance(shell, dict) and shell.get("allowed"):
            fail("the example policy enables the shell scope; it must stay off")
        else:
            ok("the example policy keeps the shell scope off")


def check_catalog() -> None:
    """Catalog entries must point at files that exist."""
    index_path = os.path.join(REPO_ROOT, "desktop", "catalog", "index.json")
    if not os.path.exists(index_path):
        fail("desktop/catalog/index.json is missing")
        return
    with open(index_path, "r", encoding="utf-8") as handle:
        index = json.load(handle)

    items = index.get("items", [])
    if not isinstance(items, list) or not items:
        fail("catalog index has no items")
        return

    seen = set()
    for item in items:
        item_id = item.get("id")
        if not item_id:
            fail(f"catalog item without an id: {item}")
        elif item_id in seen:
            fail(f"catalog id {item_id!r} appears twice")
        seen.add(item_id)

    games_dir = os.path.join(REPO_ROOT, "desktop", "catalog", "games")
    profiles = [name for name in os.listdir(games_dir) if name.endswith(".json")] if os.path.isdir(games_dir) else []
    if not profiles:
        fail("no game profiles found under desktop/catalog/games")
    else:
        for name in profiles:
            with open(os.path.join(games_dir, name), "r", encoding="utf-8") as handle:
                profile = json.load(handle)
            if not profile.get("id"):
                fail(f"game profile {name} has no id")
    ok(f"catalog has {len(items)} items and {len(profiles)} game profiles")


def check_python_syntax() -> None:
    """Every Python file in the repo must at least compile."""
    count = 0
    for path in walk(REPO_ROOT):
        if not path.endswith(".py"):
            continue
        count += 1
        with open(path, "rb") as handle:
            source = handle.read()
        try:
            compile(source, path, "exec")
        except SyntaxError as exc:
            fail(f"{os.path.relpath(path, REPO_ROOT)} does not compile: {exc}")
    ok(f"{count} Python files compile")


def _read(path: str) -> str:
    with open(path, "r", encoding="utf-8") as handle:
        return handle.read()


def check_docs() -> None:
    """Headless, weak-model and safety documentation must exist and stay linked.

    These are user-facing promises: a mode nobody documented is a mode that does
    not work, and the action surface is only usable by a small model if the
    parameter shapes are written down.
    """
    readme_path = os.path.join(REPO_ROOT, "README.md")
    readme = _read(readme_path)
    for needle in ("NEURO_HEADLESS", "shell_command"):
        if needle not in readme:
            fail(f"README.md does not mention {needle}")
            return

    for name, needles in (
        ("docs/LLM_GUIDE.md", ("game_move", "/api/actions", "get_status")),
        ("docs/SAFETY.md", ("NEURO_SHELL_ALLOWLIST", "kill switch", "default_allow")),
        # The architecture, the server↔agent protocol and the relay behaviour are
        # the three things a contributor (or a future us) needs before touching
        # either process.
        ("docs/ARCHITECTURE.md", ("agent", "Python", "server")),
        ("docs/EXECUTOR_PROTOCOL.md", ("hello", "shell_command", "file ipc")),
        ("docs/RELAY.md", ("auth_token", "register_actions", "intermediary")),
    ):
        path = os.path.join(REPO_ROOT, name)
        if not os.path.exists(path):
            fail(f"{name} is missing")
            continue
        content = _read(path).lower()
        missing = [needle for needle in needles if needle.lower() not in content]
        if missing:
            fail(f"{name} does not mention {', '.join(missing)}")
            continue
        if name not in readme:
            fail(f"README.md does not link to {name}")
            continue

    ok("README documents headless mode and links the LLM + safety guides")


def check_bundle_templates() -> None:
    """The launchers that ship inside the bundle must be valid and self-contained.

    They are templates rather than heredocs written by the bundle script, so bash
    can check them and Windows launchers can be reviewed as code.
    """
    templates = os.path.join(REPO_ROOT, "desktop", "scripts", "bundle", "templates")
    for name in ("start.sh", "start.bat", "README.txt"):
        if not os.path.exists(os.path.join(templates, name)):
            fail(f"missing bundle template scripts/bundle/templates/{name}")
            return

    sh = os.path.join(templates, "start.sh")
    bash = shutil.which("bash")
    if bash:
        result = subprocess.run([bash, "-n", sh], capture_output=True, text=True)
        if result.returncode != 0:
            fail(f"bundle template start.sh is not valid shell: {result.stderr.strip()}")
            return
    else:
        print("  - bash not found; skipped syntax check of start.sh")

    # The release bundle installs the launchers; the dev bundle only prints the
    # two commands, so it has no launcher to keep in sync.
    if "@SERVER@" not in _read(os.path.join(templates, "start.sh")):
        fail("start.sh template lost its @SERVER@ placeholder")
        return
    prod = _read(os.path.join(REPO_ROOT, "desktop", "scripts", "bundle", "prod.sh"))
    if "templates" not in prod:
        fail("scripts/bundle/prod.sh does not use the launcher templates")
        return

    ok("bundle launcher templates exist and start.sh parses")


def check_no_stale_paths() -> None:
    """Paths that were deleted must not be referenced anywhere any more.

    Deleting a placeholder directory is easy; the five documents and scripts that
    still told people to `cd` into it are the actual bug. This check is what
    keeps the cleanup honest.
    """
    stale = (
        "native/go-neuro-integration",
        "native/rust-core",
        "native/process-handler",
        "native/c_cpp",
        "tests/integration/test-",
    )
    skip_dirs = {".git", "node_modules", "dist", "__pycache__", ".venv", "target"}
    # This file contains the needles on purpose; it is the check, not a reference.
    skip_files = {os.path.relpath(__file__, REPO_ROOT)}
    hits = []
    for root, dirs, files in os.walk(REPO_ROOT):
        dirs[:] = [d for d in dirs if d not in skip_dirs]
        for name in files:
            path = os.path.join(root, name)
            if os.path.relpath(path, REPO_ROOT) in skip_files:
                continue
            if os.path.splitext(name)[1] in {".png", ".jpg", ".ico", ".woff", ".woff2", ".ttf", ".so", ".a"}:
                continue
            try:
                with open(path, "r", encoding="utf-8", errors="ignore") as handle:
                    body = handle.read()
            except OSError:
                continue
            for needle in stale:
                if needle in body:
                    hits.append(f"{os.path.relpath(path, REPO_ROOT)} -> {needle}")

    if hits:
        for hit in hits[:10]:
            fail(f"stale reference: {hit}")
        if len(hits) > 10:
            fail(f"… and {len(hits) - 10} more stale references")
        return
    ok("no references to deleted paths")


def check_process_handler_standalone() -> None:
    """Compile and run the dependency-free process-handler test suite.

    This is the suite that actually runs in CI; the GoogleTest files next to it
    need a network FetchContent of googletest and are advisory only.
    """
    app_dir = os.path.join(REPO_ROOT, "desktop", "apps", "process-handler")
    compiler = shutil.which("g++") or shutil.which("clang++") or shutil.which("c++")
    if compiler is None:
        ok("process-handler C++ tests skipped (no C++ compiler on PATH)")
        return

    test_source = os.path.join(app_dir, "tests", "test_standalone.cpp")
    if not os.path.exists(test_source):
        fail("desktop/apps/process-handler/tests/test_standalone.cpp is missing")
        return

    with tempfile.TemporaryDirectory() as tmp:
        binary = os.path.join(tmp, "process-handler-tests")
        compile_cmd = [
            compiler,
            "-std=c++17",
            "-pthread",
            "-Isrc",
            "-o",
            binary,
            os.path.join("src", "process_handler.cpp"),
            os.path.join("tests", "test_standalone.cpp"),
        ]
        compiled = subprocess.run(
            compile_cmd, cwd=app_dir, capture_output=True, text=True, timeout=300
        )
        if compiled.returncode != 0:
            fail("process-handler does not compile: "
                 + (compiled.stderr or compiled.stdout).strip().splitlines()[-1])
            return

        ran = subprocess.run([binary], capture_output=True, text=True, timeout=180)
        if ran.returncode != 0:
            tail = (ran.stdout + ran.stderr).strip().splitlines()
            fail("process-handler tests failed: " + (tail[-1] if tail else "no output"))
            return

    ok("process-handler C++ tests pass (parser, validator, lifecycle)")


def check_ollama_brain_tests() -> None:
    """The Ollama "brain" parser must pass without optional dependencies."""
    tool_dir = os.path.join(REPO_ROOT, "desktop", "tools", "ollama-neuro")
    if not os.path.isdir(tool_dir):
        fail("desktop/tools/ollama-neuro is missing")
        return
    ran = subprocess.run(
        [sys.executable, "-m", "unittest", "test_brain.py"],
        cwd=tool_dir,
        capture_output=True,
        text=True,
        timeout=180,
    )
    if ran.returncode != 0:
        tail = (ran.stdout + ran.stderr).strip().splitlines()
        fail("ollama-neuro brain tests failed: " + (tail[-1] if tail else "no output"))
        return
    ok("ollama-neuro brain tests pass (no aiohttp required)")


def check_file_sizes() -> None:
    """Nothing huge should be committed (the snapshot cap is 128 MB)."""
    largest = ("", 0)
    for path in walk(REPO_ROOT):
        try:
            size = os.path.getsize(path)
        except OSError:
            continue
        if size > largest[1]:
            largest = (path, size)
        if size > MAX_FILE_BYTES:
            fail(f"{os.path.relpath(path, REPO_ROOT)} is {size / 1024 / 1024:.1f} MB "
                 f"(limit {MAX_FILE_BYTES // 1024 // 1024} MB)")
    ok(f"largest tracked file is {os.path.relpath(largest[0], REPO_ROOT)} "
       f"({largest[1] / 1024:.0f} KB)")


def main() -> int:
    print(f"repository checks in {REPO_ROOT}\n")
    check_json()
    check_example_policies_match()
    check_catalog()
    check_python_syntax()
    check_process_handler_standalone()
    check_ollama_brain_tests()
    check_docs()
    check_bundle_templates()
    check_no_stale_paths()
    check_file_sizes()

    print()
    if failures:
        print(f"{len(failures)} check(s) failed")
        return 1
    print("all repository checks passed")
    return 0


if __name__ == "__main__":
    sys.exit(main())
