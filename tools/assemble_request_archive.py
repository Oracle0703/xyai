#!/usr/bin/env python3
import argparse
import json
import sys
from pathlib import Path
from typing import Any


def iter_input_files(input_path: Path) -> list[Path]:
    if input_path.is_file():
        return [input_path]
    if input_path.is_dir():
        return sorted(input_path.glob("*.jsonl"))
    raise FileNotFoundError(f"input path not found: {input_path}")


def load_events(files: list[Path]) -> tuple[dict[str, dict[str, Any]], list[str]]:
    grouped: dict[str, dict[str, Any]] = {}
    warnings: list[str] = []

    for path in files:
        with path.open("r", encoding="utf-8") as handle:
            for line_number, raw in enumerate(handle, start=1):
                line = raw.strip()
                if not line:
                    continue
                try:
                    event = json.loads(line)
                except json.JSONDecodeError as exc:
                    warnings.append(f"{path}:{line_number}: invalid json: {exc}")
                    continue

                archive_id = str(event.get("archive_id") or "").strip()
                event_type = str(event.get("event") or "").strip()
                if not archive_id or event_type not in {"request", "response"}:
                    warnings.append(
                        f"{path}:{line_number}: missing archive_id or unsupported event"
                    )
                    continue

                item = grouped.setdefault(
                    archive_id,
                    {
                        "archive_id": archive_id,
                        "request": None,
                        "response": None,
                        "events": [],
                        "source_files": [],
                    },
                )
                item["events"].append(event)
                source = str(path)
                if source not in item["source_files"]:
                    item["source_files"].append(source)

                current = item.get(event_type)
                if current is None:
                    item[event_type] = event
                    continue

                current_ts = str(current.get("timestamp") or "")
                next_ts = str(event.get("timestamp") or "")
                if next_ts >= current_ts:
                    item[event_type] = event

    return grouped, warnings


def assemble_record(item: dict[str, Any]) -> dict[str, Any]:
    request = item.get("request")
    response = item.get("response")
    if request and response:
        status = "complete"
    elif request:
        status = "request_only"
    elif response:
        status = "response_only"
    else:
        status = "empty"

    summary_source = request or response or {}
    response_source = response or {}

    return {
        "archive_id": item["archive_id"],
        "status": status,
        "request_timestamp": value(summary_source, "timestamp"),
        "response_timestamp": value(response_source, "timestamp"),
        "method": value(summary_source, "method"),
        "path": value(summary_source, "path"),
        "endpoint": value(summary_source, "endpoint"),
        "model": value(summary_source, "model"),
        "user_id": value(summary_source, "user_id"),
        "api_key_id": value(summary_source, "api_key_id"),
        "group_id": value(summary_source, "group_id"),
        "account_id": value(response_source, "account_id")
        or value(summary_source, "account_id"),
        "client_ip": value(summary_source, "client_ip"),
        "user_agent": value(summary_source, "user_agent"),
        "http_status": value(response_source, "status"),
        "duration_ms": value(response_source, "duration_ms"),
        "request_body_size": value(request or {}, "body_size"),
        "response_body_size": value(response or {}, "body_size"),
        "request_body_truncated": bool(value(request or {}, "body_truncated")),
        "response_body_truncated": bool(value(response or {}, "body_truncated")),
        "source_files": item.get("source_files", []),
        "request": request,
        "response": response,
    }


def value(record: dict[str, Any], key: str) -> Any:
    if not isinstance(record, dict):
        return None
    return record.get(key)


def sort_key(record: dict[str, Any]) -> tuple[str, str]:
    return (
        str(record.get("request_timestamp") or record.get("response_timestamp") or ""),
        str(record.get("archive_id") or ""),
    )


def write_output(records: list[dict[str, Any]], output: Path, fmt: str) -> None:
    output.parent.mkdir(parents=True, exist_ok=True)
    with output.open("w", encoding="utf-8", newline="\n") as handle:
        if fmt == "json":
            json.dump(records, handle, ensure_ascii=False, indent=2)
            handle.write("\n")
            return
        for record in records:
            handle.write(json.dumps(record, ensure_ascii=False, separators=(",", ":")))
            handle.write("\n")


def main() -> int:
    parser = argparse.ArgumentParser(
        description="Assemble local request archive JSONL events by archive_id."
    )
    parser.add_argument(
        "--input",
        required=True,
        help="Archive JSONL file or directory containing *.jsonl files.",
    )
    parser.add_argument("--output", required=True, help="Output assembled file path.")
    parser.add_argument(
        "--format",
        choices=("jsonl", "json"),
        default="jsonl",
        help="Output format. Defaults to jsonl.",
    )
    args = parser.parse_args()

    try:
        files = iter_input_files(Path(args.input))
        grouped, warnings = load_events(files)
        records = sorted(
            (assemble_record(item) for item in grouped.values()),
            key=sort_key,
        )
        write_output(records, Path(args.output), args.format)
    except OSError as exc:
        sys.stderr.write(f"{exc}\n")
        return 1

    complete = sum(1 for record in records if record["status"] == "complete")
    request_only = sum(1 for record in records if record["status"] == "request_only")
    response_only = sum(1 for record in records if record["status"] == "response_only")
    sys.stdout.write(
        "Assembled "
        f"{len(records)} interactions from {len(files)} file(s): "
        f"complete={complete}, request_only={request_only}, response_only={response_only}\n"
    )
    for warning in warnings:
        sys.stderr.write(f"warning: {warning}\n")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())

