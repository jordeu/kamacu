#!/usr/bin/env python3
"""Replay browser-captured prompt input through the installed Claude/Codex TUIs.

Requires Python pexpect and both CLIs on PATH. Uses temporary configuration and
local mock model endpoints; no account credentials or model service are needed.
First run prompt-execution-browser-check.cjs with an output JSON path, then:
    python3 scripts/prompt-execution-cli-check.py /tmp/prompt-frames.json
"""

import argparse
import json
import os
from pathlib import Path
import re
import tempfile
import threading
import time
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

import pexpect


class MockModel(BaseHTTPRequestHandler):
    requests = []

    def log_message(self, *_):
        pass

    def do_GET(self):
        self.send_json({"data": []})

    def send_json(self, body):
        self.send_response(200)
        self.send_header("Content-Type", "application/json")
        self.end_headers()
        self.wfile.write(json.dumps(body).encode())

    def do_POST(self):
        body = json.loads(self.rfile.read(int(self.headers.get("Content-Length", "0"))) or "{}")
        self.requests.append((self.path, body))
        if "count_tokens" in self.path:
            self.send_json({"input_tokens": 10})
            return
        self.send_response(200)
        self.send_header("Content-Type", "text/event-stream")
        self.end_headers()
        if "/messages" in self.path:
            message = {
                "id": "msg_test", "type": "message", "role": "assistant",
                "model": "claude-sonnet-4-6", "content": [], "stop_reason": None,
                "stop_sequence": None, "usage": {"input_tokens": 10, "output_tokens": 1},
            }
            events = [
                ("message_start", {"message": message}),
                ("content_block_start", {"index": 0, "content_block": {"type": "text", "text": ""}}),
                ("content_block_delta", {"index": 0, "delta": {"type": "text_delta", "text": "TEST_OK"}}),
                ("content_block_stop", {"index": 0}),
                ("message_delta", {"delta": {"stop_reason": "end_turn", "stop_sequence": None}, "usage": {"output_tokens": 3}}),
                ("message_stop", {}),
            ]
        else:
            message = {
                "id": "msg_test", "type": "message", "status": "completed", "role": "assistant",
                "content": [{"type": "output_text", "text": "TEST_OK", "annotations": []}],
            }
            response = {
                "id": "resp_test", "object": "response", "created_at": int(time.time()),
                "status": "completed", "model": "test", "output": [message],
                "usage": {"input_tokens": 10, "output_tokens": 3, "total_tokens": 13},
            }
            events = [
                ("response.created", {"response": {**response, "status": "in_progress", "output": []}}),
                ("response.output_item.added", {"output_index": 0, "item": {**message, "status": "in_progress", "content": []}}),
                ("response.output_text.delta", {"item_id": "msg_test", "output_index": 0, "content_index": 0, "delta": "TEST_OK"}),
                ("response.output_item.done", {"output_index": 0, "item": message}),
                ("response.completed", {"response": response}),
            ]
        try:
            for event, data in events:
                payload = json.dumps({"type": event, **data})
                self.wfile.write(f"event: {event}\ndata: {payload}\n\n".encode())
                self.wfile.flush()
        except BrokenPipeError:
            pass


def plain(text):
    return re.sub(r"\x1b\[[0-?]*[ -/]*[@-~]", "", text)


def check_engine(engine, fixtures, base_url, root):
    workspace = root / engine / "workspace"
    workspace.mkdir(parents=True)
    config = root / engine / "config"
    config.mkdir()
    env = dict(os.environ)
    for key in list(env):
        if key.startswith(("KAMACU_", "CLAUDE", "ANTHROPIC_", "CODEX_", "OPENAI_")):
            env.pop(key)
    env.update(TERM="xterm-256color", COLORTERM="truecolor")
    if engine == "claude":
        env.update(
            CLAUDE_CONFIG_DIR=str(config), ANTHROPIC_API_KEY="local-test-only",
            ANTHROPIC_BASE_URL=base_url, DISABLE_AUTOUPDATER="1",
            CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC="1",
        )
        (config / ".claude.json").write_text(json.dumps({
            "hasCompletedOnboarding": True, "theme": "dark", "hasAcknowledgedCost": True,
        }))
        args = ["claude", "--bare", "--model", "claude-sonnet-4-6", "--setting-sources", "",
                "--strict-mcp-config", "--mcp-config", '{"mcpServers":{}}']
    else:
        env["CODEX_HOME"] = str(config)
        (config / "config.toml").write_text(
            'model = "test"\nmodel_provider = "local_test"\n'
            '[model_providers.local_test]\nname = "Local Test"\n'
            f'base_url = "{base_url}/v1"\nwire_api = "responses"\n'
            f'[projects."{workspace}"]\ntrust_level = "trusted"\n'
        )
        args = ["codex", "--no-alt-screen", "--no-daemon", "-a", "never", "-s", "read-only"]
    child = pexpect.spawn(args[0], args[1:], cwd=str(workspace), env=env,
                          encoding="utf-8", timeout=1, dimensions=(40, 140))
    # pexpect defaults to a 50ms delay on every send; replay only the timing
    # captured from the browser so we test the production 100ms submit gap.
    child.delaybeforesend = 0
    output = ""

    def pump(seconds):
        nonlocal output
        deadline = time.monotonic() + seconds
        while time.monotonic() < deadline:
            try:
                chunk = child.read_nonblocking(65536, timeout=0.1)
                output += chunk
                if "\x1b[6n" in chunk:
                    child.send("\x1b[1;1R")
                if "\x1b[c" in chunk:
                    child.send("\x1b[?1;2c")
            except pexpect.TIMEOUT:
                pass
            except pexpect.EOF:
                return

    with (root / engine / "terminal.log").open("w") as log:
        child.logfile_read = log
        try:
            pump(5)
            # Only trust this test's empty temporary directory and dummy API key.
            if engine == "claude" and "Yes,Itrustthisfolder" in plain(output):
                child.send("\x1b[B")
                pump(0.2)
                child.send("\r")
                pump(4)
            if engine == "claude" and "DoyouwanttousethisAPIkey?" in plain(output):
                child.send("\x1b[A")
                pump(0.2)
                child.send("\r")
                pump(4)
            for fixture in fixtures:
                if fixture["engine"] != engine:
                    continue
                start = len(MockModel.requests)
                previous_ms = 0
                for frame in fixture["frames"]:
                    time.sleep(max(0, (frame["ms"] - previous_ms) / 1000))
                    child.send(frame["data"])
                    previous_ms = frame["ms"]
                deadline = time.monotonic() + 20
                matched = False
                while time.monotonic() < deadline and not matched:
                    pump(0.2)
                    for endpoint, body in MockModel.requests[start:]:
                        if "count_tokens" in endpoint:
                            continue
                        messages = body.get("messages", body.get("input", []))
                        users = [message for message in messages if message.get("role") == "user"]
                        if not users:
                            continue
                        content = users[-1]["content"]
                        text = content if isinstance(content, str) else "\n".join(
                            part.get("text", "") for part in content
                            if part.get("type") in ("text", "input_text")
                        )
                        # Claude encloses large pastes in <pasted_content>.
                        # Codex trims outer whitespace as for a normal Enter.
                        if fixture["text"].strip() in text:
                            matched = True
                            break
                assert matched, f"{engine}/{fixture['title']} not submitted; see {log.name}"
                print(f"PASS {engine}: {fixture['title']} ({len(fixture['text'])} characters)", flush=True)
                pump(2)
        finally:
            child.terminate(force=True)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("frames", type=Path)
    parser.add_argument("--engine", choices=["claude", "codex"])
    args = parser.parse_args()
    fixtures = json.loads(args.frames.read_text())
    root = Path(tempfile.mkdtemp(prefix="kamacu-prompt-cli-"))
    print(f"Isolated CLI test artifacts: {root}", flush=True)
    server = ThreadingHTTPServer(("127.0.0.1", 0), MockModel)
    threading.Thread(target=server.serve_forever, daemon=True).start()
    try:
        for engine in [args.engine] if args.engine else ["claude", "codex"]:
            assert any(fixture["engine"] == engine for fixture in fixtures), f"No fixtures for {engine}"
            check_engine(engine, fixtures, f"http://127.0.0.1:{server.server_port}", root)
    finally:
        server.shutdown()
        server.server_close()


if __name__ == "__main__":
    main()
