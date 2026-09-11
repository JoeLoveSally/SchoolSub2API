#!/usr/bin/env python3
from __future__ import annotations

import argparse
import json
import os
import platform
import shutil
import subprocess
import sys
from pathlib import Path

DEFAULT_MODEL_ID = "hkust-glm-5.2"
DEFAULT_MODEL_NAME = "HKUST GLM-5.2"
DEFAULT_URL = "http://127.0.0.1:5001/v1/chat/completions"
DEFAULT_MAX_INPUT = 1_048_576
DEFAULT_MAX_OUTPUT = 131_072


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(
        description="Generate or update a WorkBuddy models.json entry for local SchoolSub2API.",
    )
    parser.add_argument("--config", default="config.json", help="DS2API config file (default: config.json)")
    parser.add_argument("--output", help="WorkBuddy models.json path; auto-detected when omitted")
    parser.add_argument("--api-key", help="DS2API client API key; defaults to the first key in config.json")
    parser.add_argument("--url", default=DEFAULT_URL, help=f"OpenAI-compatible endpoint (default: {DEFAULT_URL})")
    parser.add_argument("--model-id", default=DEFAULT_MODEL_ID)
    parser.add_argument("--name", default=DEFAULT_MODEL_NAME)
    parser.add_argument("--max-input-tokens", type=int, default=DEFAULT_MAX_INPUT)
    parser.add_argument("--max-output-tokens", type=int, default=DEFAULT_MAX_OUTPUT)
    parser.add_argument("--no-backup", action="store_true", help="Do not create models.json.bak before updating")
    return parser.parse_args()


def first_config_key(path: Path) -> str:
    try:
        data = json.loads(path.read_text(encoding="utf-8"))
    except FileNotFoundError as exc:
        raise SystemExit(f"DS2API config not found: {path}") from exc
    except json.JSONDecodeError as exc:
        raise SystemExit(f"Invalid JSON in {path}: {exc}") from exc

    keys = data.get("keys")
    if isinstance(keys, list):
        for value in keys:
            if isinstance(value, str) and value.strip():
                return value.strip()

    api_keys = data.get("api_keys")
    if isinstance(api_keys, list):
        for item in api_keys:
            if isinstance(item, str) and item.strip():
                return item.strip()
            if isinstance(item, dict):
                for field in ("key", "api_key", "token"):
                    value = item.get(field)
                    if isinstance(value, str) and value.strip():
                        return value.strip()

    raise SystemExit(f"No usable DS2API client key found in {path}")


def wsl_windows_home() -> Path | None:
    if "microsoft" not in platform.release().lower():
        return None
    try:
        # cmd.exe follows the Windows console code page by default, which can
        # make Python's UTF-8 text decoding fail on non-English Windows.
        # `/u` forces UTF-16LE output, so decode the bytes explicitly.
        proc = subprocess.run(
            ["cmd.exe", "/u", "/d", "/c", "echo", "%USERPROFILE%"],
            check=True,
            capture_output=True,
            timeout=5,
        )
        win_home = proc.stdout.decode("utf-16le", errors="strict").strip()
        if not win_home or "%USERPROFILE%" in win_home:
            return None

        proc = subprocess.run(
            ["wslpath", "-u", win_home],
            check=True,
            capture_output=True,
            timeout=5,
        )
        linux_home = proc.stdout.decode("utf-8", errors="strict").strip()
        return Path(linux_home) if linux_home else None
    except (OSError, UnicodeError, subprocess.SubprocessError):
        return None


def default_output_path() -> Path:
    if os.name == "nt":
        return Path.home() / ".codebuddy" / "models.json"
    windows_home = wsl_windows_home()
    if windows_home is not None:
        return windows_home / ".codebuddy" / "models.json"
    return Path.home() / ".codebuddy" / "models.json"


def load_existing(path: Path) -> dict:
    if not path.exists():
        return {"models": []}
    try:
        data = json.loads(path.read_text(encoding="utf-8"))
    except json.JSONDecodeError as exc:
        raise SystemExit(f"Invalid existing WorkBuddy config {path}: {exc}") from exc
    if not isinstance(data, dict):
        raise SystemExit(f"Existing WorkBuddy config must be a JSON object: {path}")
    if not isinstance(data.get("models", []), list):
        raise SystemExit(f"Existing WorkBuddy config has non-array 'models': {path}")
    return data


def upsert_model(data: dict, model: dict) -> None:
    models = data.setdefault("models", [])
    for index, existing in enumerate(models):
        if isinstance(existing, dict) and existing.get("id") == model["id"]:
            models[index] = model
            break
    else:
        models.append(model)

    available = data.get("availableModels")
    if isinstance(available, list) and model["id"] not in available:
        available.append(model["id"])


def main() -> int:
    args = parse_args()
    if args.max_input_tokens <= 0 or args.max_output_tokens <= 0:
        raise SystemExit("Token limits must be positive integers")

    config_path = Path(args.config).expanduser()
    api_key = (args.api_key or first_config_key(config_path)).strip()
    if not api_key:
        raise SystemExit("DS2API client API key is empty")
    if api_key == "change-this-ds2api-key":
        raise SystemExit("Replace 'change-this-ds2api-key' in config.json before generating WorkBuddy config")

    output_path = Path(args.output).expanduser() if args.output else default_output_path()
    output_path.parent.mkdir(parents=True, exist_ok=True)

    data = load_existing(output_path)
    model = {
        "id": args.model_id,
        "name": args.name,
        "vendor": "Custom",
        "apiKey": api_key,
        "maxInputTokens": args.max_input_tokens,
        "maxOutputTokens": args.max_output_tokens,
        "url": args.url,
        "supportsToolCall": True,
        "supportsImages": False,
        "supportsReasoning": True,
    }
    upsert_model(data, model)

    if output_path.exists() and not args.no_backup:
        backup = output_path.with_name(output_path.name + ".bak")
        shutil.copy2(output_path, backup)
        print(f"Backup: {backup}")

    output_path.write_text(json.dumps(data, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    print(f"WorkBuddy config updated: {output_path}")
    print(f"Model: {args.model_id}")
    print(f"Endpoint: {args.url}")
    print(f"maxInputTokens: {args.max_input_tokens}")
    print(f"maxOutputTokens: {args.max_output_tokens}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
