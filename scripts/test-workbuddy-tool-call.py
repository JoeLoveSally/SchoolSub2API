#!/usr/bin/env python3
from __future__ import annotations

import argparse
import json
import sys
import urllib.error
import urllib.request
from pathlib import Path

DEFAULT_URL = "http://127.0.0.1:5001/v1/chat/completions"
DEFAULT_MODEL = "hkust-glm-5.2"


def first_config_key(path: Path) -> str:
    try:
        data = json.loads(path.read_text(encoding="utf-8"))
    except FileNotFoundError as exc:
        raise SystemExit(f"config not found: {path}") from exc
    keys = data.get("keys")
    if isinstance(keys, list):
        for value in keys:
            if isinstance(value, str) and value.strip():
                return value.strip()
    raise SystemExit(f"no usable client key in {path}")


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(
        description="Probe the OpenAI-compatible tool-call path used by WorkBuddy.",
    )
    parser.add_argument("--url", default=DEFAULT_URL)
    parser.add_argument("--model", default=DEFAULT_MODEL)
    parser.add_argument("--config", default="config.json")
    parser.add_argument("--api-key", help="defaults to config.json; avoid this option if shell history is enabled")
    parser.add_argument("--timeout", type=float, default=120.0)
    parser.add_argument("--non-stream", action="store_true", help="probe non-streaming instead of WorkBuddy-like streaming")
    return parser.parse_args()


def request_payload(model: str, stream: bool) -> dict:
    return {
        "model": model,
        "stream": stream,
        "messages": [
            {
                "role": "user",
                "content": (
                    "Call the inspect_project tool exactly once with path set to '.'. "
                    "Do not answer with prose before or after the tool call."
                ),
            }
        ],
        "tools": [
            {
                "type": "function",
                "function": {
                    "name": "inspect_project",
                    "description": "Inspect a project directory. This diagnostic tool does not execute anything.",
                    "parameters": {
                        "type": "object",
                        "properties": {
                            "path": {
                                "type": "string",
                                "description": "Project directory path",
                            }
                        },
                        "required": ["path"],
                        "additionalProperties": False,
                    },
                },
            }
        ],
        "tool_choice": "auto",
    }


def open_request(url: str, api_key: str, payload: dict, timeout: float):
    body = json.dumps(payload, ensure_ascii=False).encode("utf-8")
    req = urllib.request.Request(
        url,
        data=body,
        method="POST",
        headers={
            "Authorization": f"Bearer {api_key}",
            "Content-Type": "application/json",
            "Accept": "text/event-stream" if payload["stream"] else "application/json",
        },
    )
    return urllib.request.urlopen(req, timeout=timeout)


def append_tool_delta(tool_calls: dict[int, dict], delta: dict) -> None:
    index = int(delta.get("index", 0) or 0)
    current = tool_calls.setdefault(
        index,
        {"id": "", "type": "function", "name": "", "arguments": ""},
    )
    if isinstance(delta.get("id"), str):
        current["id"] += delta["id"]
    function = delta.get("function")
    if isinstance(function, dict):
        if isinstance(function.get("name"), str):
            current["name"] += function["name"]
        if isinstance(function.get("arguments"), str):
            current["arguments"] += function["arguments"]


def print_reasoning(reasoning: str) -> None:
    print(f"reasoning_chars: {len(reasoning)}")
    if reasoning:
        sample = reasoning[:1200]
        if len(reasoning) > len(sample):
            sample += "..."
        print("reasoning_sample:")
        print(sample)


def run_stream(response) -> int:
    reasoning_parts: list[str] = []
    content_parts: list[str] = []
    tool_calls: dict[int, dict] = {}
    error_frames: list[dict] = []
    finish_reasons: list[str] = []
    frame_count = 0

    for raw in response:
        line = raw.decode("utf-8", errors="replace").strip()
        if not line.startswith("data:"):
            continue
        data = line[5:].strip()
        if not data or data == "[DONE]":
            continue
        frame_count += 1
        try:
            obj = json.loads(data)
        except json.JSONDecodeError:
            print("unparsed_sse:", data[:500])
            continue

        if isinstance(obj.get("error"), dict) or obj.get("status_code"):
            error_frames.append(obj)

        choices = obj.get("choices")
        if not isinstance(choices, list):
            continue
        for choice in choices:
            if not isinstance(choice, dict):
                continue
            reason = choice.get("finish_reason")
            if isinstance(reason, str) and reason:
                finish_reasons.append(reason)
            delta = choice.get("delta")
            if not isinstance(delta, dict):
                continue
            if isinstance(delta.get("reasoning_content"), str):
                reasoning_parts.append(delta["reasoning_content"])
            if isinstance(delta.get("content"), str):
                content_parts.append(delta["content"])
            calls = delta.get("tool_calls")
            if isinstance(calls, list):
                for call in calls:
                    if isinstance(call, dict):
                        append_tool_delta(tool_calls, call)

    reasoning = "".join(reasoning_parts)
    content = "".join(content_parts)
    calls = [tool_calls[i] for i in sorted(tool_calls)]

    print(f"http_status: {getattr(response, 'status', 'unknown')}")
    print(f"sse_frames: {frame_count}")
    print_reasoning(reasoning)
    print("content:", repr(content))
    print("finish_reasons:", finish_reasons)
    print("tool_calls:", json.dumps(calls, ensure_ascii=False, indent=2))
    if error_frames:
        print("error_frames:")
        print(json.dumps(error_frames, ensure_ascii=False, indent=2))

    if calls:
        print("RESULT: PASS - proxy emitted an OpenAI tool_call")
        return 0
    if error_frames:
        print("RESULT: FAIL - server emitted an error after the model turn")
        return 2
    if reasoning and not content:
        print("RESULT: FAIL - reasoning-only response; no parseable tool_call")
        return 3
    print("RESULT: FAIL - no tool_call")
    return 4


def run_non_stream(response) -> int:
    raw = response.read().decode("utf-8", errors="replace")
    print(f"http_status: {getattr(response, 'status', 'unknown')}")
    try:
        obj = json.loads(raw)
    except json.JSONDecodeError:
        print(raw[:3000])
        return 4
    print(json.dumps(obj, ensure_ascii=False, indent=2))
    choices = obj.get("choices")
    if isinstance(choices, list) and choices:
        message = choices[0].get("message") if isinstance(choices[0], dict) else None
        if isinstance(message, dict) and isinstance(message.get("tool_calls"), list) and message["tool_calls"]:
            print("RESULT: PASS - proxy emitted an OpenAI tool_call")
            return 0
    print("RESULT: FAIL - no tool_call")
    return 4


def main() -> int:
    args = parse_args()
    api_key = args.api_key or first_config_key(Path(args.config).expanduser())
    payload = request_payload(args.model, stream=not args.non_stream)

    print("WorkBuddy tool-call diagnostic")
    print("endpoint:", args.url)
    print("model:", args.model)
    print("stream:", payload["stream"])
    print("tool: inspect_project(path='.')")
    print("api_key: <redacted>")
    print()

    try:
        with open_request(args.url, api_key, payload, args.timeout) as response:
            if payload["stream"]:
                return run_stream(response)
            return run_non_stream(response)
    except urllib.error.HTTPError as exc:
        body = exc.read().decode("utf-8", errors="replace")
        print("http_status:", exc.code)
        print("http_error_body:")
        try:
            print(json.dumps(json.loads(body), ensure_ascii=False, indent=2))
        except json.JSONDecodeError:
            print(body[:3000])
        return 2
    except Exception as exc:
        print(f"request_error: {type(exc).__name__}: {exc}")
        return 5


if __name__ == "__main__":
    sys.exit(main())
