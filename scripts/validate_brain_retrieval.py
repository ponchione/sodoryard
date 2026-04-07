#!/usr/bin/env python3
"""Repeatable live validation for proactive brain retrieval (V2-B4).

This script drives a real websocket turn against a running sirtopham server,
then fetches the stored context report and ordered signal stream for turn 1.
It is meant to prove the current operator-facing contract end to end:

- a brain-only fact can be answered from proactive MCP/vault-backed keyword retrieval
- the persisted context report shows semantic queries, brain results, and brain budget
- the ordered `/signals` endpoint exposes the retrieval signal flow used for debugging

Example:
  python3 scripts/validate_brain_retrieval.py \
    --base-url http://localhost:8092 \
    --expected-note notes/runtime-brain-proof-apr-07.md
"""

from __future__ import annotations

import argparse
import asyncio
import json
import sys
import urllib.error
import urllib.parse
import urllib.request
from dataclasses import dataclass
from typing import Any

import websockets

DEFAULT_PROMPT = "What is the runtime brain proof canary phrase?"
DEFAULT_EXPECTED_PHRASE = "ORBIT LANTERN 642"
DEFAULT_EXPECTED_NOTE = "notes/runtime-brain-proof-apr-07.md"


@dataclass
class ValidationResult:
    conversation_id: str
    turn_number: int
    assistant_text: str
    tool_calls: list[dict[str, Any]]
    context_report: dict[str, Any]
    signal_stream: list[dict[str, Any]]
    event_counts: dict[str, int]


async def collect_turn(ws_url: str, prompt: str, timeout_seconds: int) -> tuple[str, int, str, list[dict[str, Any]], dict[str, int], dict[str, Any] | None]:
    conversation_id = ""
    turn_number = 0
    assistant_parts: list[str] = []
    tool_calls: list[dict[str, Any]] = []
    event_counts: dict[str, int] = {}
    latest_context_debug: dict[str, Any] | None = None

    async with websockets.connect(ws_url, max_size=8 * 1024 * 1024) as ws:
        await ws.send(
            json.dumps(
                {
                    "type": "message",
                    "conversation_id": "",
                    "content": prompt,
                }
            )
        )

        while True:
            raw = await asyncio.wait_for(ws.recv(), timeout=timeout_seconds)
            event = json.loads(raw)
            event_type = event.get("type", "")
            event_counts[event_type] = event_counts.get(event_type, 0) + 1
            data = event.get("data") or {}

            if event_type == "conversation_created":
                conversation_id = data.get("conversation_id", conversation_id)

            if event_type == "context_debug":
                latest_context_debug = data.get("report") or latest_context_debug
                turn_number = int((latest_context_debug or {}).get("turn_number") or turn_number or 0)

            if event_type == "tool_call_start":
                tool_calls.append(
                    {
                        "tool_name": data.get("tool_name", ""),
                        "arguments": data.get("arguments") or {},
                    }
                )

            if event_type == "token":
                token = data.get("token")
                if isinstance(token, str):
                    assistant_parts.append(token)

            if event_type == "turn_complete":
                turn_number = int(data.get("turn_number") or turn_number or 0)
                break

            if event_type == "error":
                raise RuntimeError(f"server returned error event: {data}")

    return conversation_id, turn_number, "".join(assistant_parts).strip(), tool_calls, event_counts, latest_context_debug


def http_get_json(url: str) -> Any:
    req = urllib.request.Request(url, headers={"Accept": "application/json"})
    try:
        with urllib.request.urlopen(req, timeout=30) as resp:
            return json.load(resp)
    except urllib.error.HTTPError as exc:
        body = exc.read().decode("utf-8", "replace")
        raise RuntimeError(f"HTTP {exc.code} for {url}: {body}") from exc


def extract_assistant_text(messages: list[dict[str, Any]]) -> str:
    assistant_messages = [m for m in messages if m.get("role") == "assistant"]
    if not assistant_messages:
        return ""
    raw = assistant_messages[-1].get("content")
    if isinstance(raw, str):
        try:
            parsed = json.loads(raw)
        except json.JSONDecodeError:
            return raw
        if isinstance(parsed, list):
            text_parts = [block.get("text", "") for block in parsed if isinstance(block, dict) and block.get("type") == "text"]
            if text_parts:
                return "".join(text_parts).strip()
        return raw
    return json.dumps(raw)


async def validate(args: argparse.Namespace) -> ValidationResult:
    ws_url = args.ws_url or args.base_url.rstrip("/").replace("http://", "ws://", 1).replace("https://", "wss://", 1) + "/api/ws"
    conversation_id, turn_number, streamed_text, tool_calls, event_counts, context_debug_report = await collect_turn(ws_url, args.prompt, args.timeout)

    if not conversation_id:
        raise RuntimeError("did not receive conversation_created with a conversation_id")
    if turn_number <= 0:
        raise RuntimeError("did not receive a positive turn number")

    messages = http_get_json(f"{args.base_url.rstrip('/')}/api/conversations/{conversation_id}/messages")
    if not isinstance(messages, list):
        raise RuntimeError("messages endpoint returned non-list JSON")
    assistant_text = extract_assistant_text(messages) or streamed_text

    context_report = http_get_json(
        f"{args.base_url.rstrip('/')}/api/metrics/conversation/{conversation_id}/context/{turn_number}"
    )
    raw_signal_stream = http_get_json(
        f"{args.base_url.rstrip('/')}/api/metrics/conversation/{conversation_id}/context/{turn_number}/signals"
    )
    signal_stream = raw_signal_stream.get("stream") if isinstance(raw_signal_stream, dict) else raw_signal_stream

    failures: list[str] = []

    if args.expected_phrase not in assistant_text:
        failures.append(f"assistant response missing expected phrase {args.expected_phrase!r}")

    needs = context_report.get("needs") or {}
    semantic_queries = needs.get("semantic_queries") or []
    if not semantic_queries:
        failures.append("context report missing needs.semantic_queries")

    brain_results = context_report.get("brain_results")
    if not brain_results:
        failures.append("context report missing brain_results")
    else:
        if not any((hit or {}).get("document_path") == args.expected_note for hit in brain_results if isinstance(hit, dict)):
            failures.append(f"brain_results missing expected note {args.expected_note}")

    budget_breakdown = context_report.get("budget_breakdown") or {}
    brain_budget = budget_breakdown.get("brain", 0)
    if not isinstance(brain_budget, (int, float)) or brain_budget <= 0:
        failures.append("budget_breakdown.brain was not > 0")

    if not isinstance(signal_stream, list) or not signal_stream:
        failures.append("signal stream endpoint returned no entries")
    else:
        if not any((entry or {}).get("kind") == "semantic_query" for entry in signal_stream if isinstance(entry, dict)):
            failures.append("signal stream missing semantic_query entry")
        if not any((entry or {}).get("kind") == "flag" and (entry or {}).get("type") == "prefer_brain_context" for entry in signal_stream if isinstance(entry, dict)):
            failures.append("signal stream missing prefer_brain_context flag")

    if not args.allow_tool_calls and tool_calls:
        failures.append(f"expected zero tool calls but saw {[call.get('tool_name') for call in tool_calls]}")

    if failures:
        payload = {
            "status": "failed",
            "conversation_id": conversation_id,
            "turn_number": turn_number,
            "assistant_text": assistant_text,
            "tool_calls": tool_calls,
            "event_counts": event_counts,
            "context_debug_report": context_debug_report,
            "context_report": context_report,
            "signal_stream": signal_stream,
            "failures": failures,
        }
        print(json.dumps(payload, indent=2))
        raise SystemExit(1)

    result = ValidationResult(
        conversation_id=conversation_id,
        turn_number=turn_number,
        assistant_text=assistant_text,
        tool_calls=tool_calls,
        context_report=context_report,
        signal_stream=signal_stream,
        event_counts=event_counts,
    )

    print(
        json.dumps(
            {
                "status": "passed",
                "conversation_id": result.conversation_id,
                "turn_number": result.turn_number,
                "assistant_text": result.assistant_text,
                "tool_calls": result.tool_calls,
                "event_counts": result.event_counts,
                "semantic_queries": (result.context_report.get("needs") or {}).get("semantic_queries") or [],
                "brain_results": result.context_report.get("brain_results") or [],
                "budget_breakdown": result.context_report.get("budget_breakdown") or {},
                "signal_stream": result.signal_stream,
            },
            indent=2,
        )
    )
    return result


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="Repeatable live validation for V2-B4 proactive brain retrieval")
    parser.add_argument("--base-url", default="http://localhost:8092", help="HTTP base URL for the running sirtopham app")
    parser.add_argument("--ws-url", default="", help="Optional websocket URL override; defaults to <base-url>/api/ws")
    parser.add_argument("--prompt", default=DEFAULT_PROMPT, help="Prompt to submit as the first turn")
    parser.add_argument("--expected-phrase", default=DEFAULT_EXPECTED_PHRASE, help="Brain-only fact expected in the assistant answer")
    parser.add_argument("--expected-note", default=DEFAULT_EXPECTED_NOTE, help="Expected document_path in context_report.brain_results")
    parser.add_argument("--timeout", type=int, default=240, help="Per-event websocket timeout in seconds")
    parser.add_argument(
        "--allow-tool-calls",
        action="store_true",
        help="Allow explicit tool calls. By default the canary prompt is expected to complete without tool detours.",
    )
    return parser.parse_args()


def main() -> int:
    args = parse_args()
    try:
        asyncio.run(validate(args))
    except KeyboardInterrupt:
        return 130
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
