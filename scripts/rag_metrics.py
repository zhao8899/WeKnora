#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""Lightweight RAGAS-style metrics runner for WeKnora's RAG pipeline.

Posts a fixture set of queries through the knowledge-chat SSE endpoint,
captures the streamed references and answer, and computes three simple
retrieval-quality signals suitable for CI gating:

  * context_precision  - fraction of expected keywords found in the
                         concatenated retrieved chunk contents.
  * answer_coverage    - fraction of expected keywords found in the
                         final answer text.
  * reference_count    - average number of references returned per
                         query (recall proxy).

These are heuristic — they do NOT require an LLM judge, so the script
is safe to run in CI without external API costs. The full RAGAS suite
(faithfulness, answer_relevancy) needs an LLM and is out of scope here.

Usage:
    python3 scripts/rag_metrics.py \
        --base-url http://127.0.0.1:18080/api/v1 \
        --api-key sk-... \
        --kb-id <knowledge-base-id> \
        --fixture scripts/fixtures/rag_smoke.json

Fixture format (JSON array):
    [
      {
        "query": "what is X",
        "expected_keywords": ["X", "definition"],
        "mode": "rag_deep",      // optional, default "rag_fast"
        "min_refs": 1,           // optional, fail when fewer refs returned
        "min_context_precision": 0.5
      }
    ]

Exit code is non-zero when any case falls below its per-case threshold.
"""

from __future__ import annotations

import argparse
import json
import re
import sys
import urllib.error
import urllib.request
from dataclasses import dataclass, field
from typing import Any, Iterable


# --- SSE client -----------------------------------------------------------


def _post_json(url: str, api_key: str, payload: dict) -> dict:
    body = json.dumps(payload).encode("utf-8")
    req = urllib.request.Request(
        url,
        data=body,
        headers={"Content-Type": "application/json", "X-API-Key": api_key},
        method="POST",
    )
    with urllib.request.urlopen(req, timeout=30) as resp:
        return json.loads(resp.read().decode("utf-8"))


def _stream_sse(url: str, api_key: str, payload: dict, timeout: int = 120) -> Iterable[dict]:
    body = json.dumps(payload).encode("utf-8")
    req = urllib.request.Request(
        url,
        data=body,
        headers={"Content-Type": "application/json", "X-API-Key": api_key},
        method="POST",
    )
    with urllib.request.urlopen(req, timeout=timeout) as resp:
        buf = b""
        for raw in resp:
            buf += raw
            while b"\n\n" in buf:
                event, buf = buf.split(b"\n\n", 1)
                for line in event.splitlines():
                    if line.startswith(b"data:"):
                        data = line[5:].strip()
                        if not data:
                            continue
                        try:
                            yield json.loads(data.decode("utf-8"))
                        except json.JSONDecodeError:
                            continue


# --- Metrics --------------------------------------------------------------


_TOKEN = re.compile(r"[\w\u4e00-\u9fff]+", re.UNICODE)


def _tokens(text: str) -> set[str]:
    return {t.lower() for t in _TOKEN.findall(text)}


def _keyword_hit_ratio(keywords: list[str], corpus: str) -> float:
    if not keywords:
        return 1.0
    corpus_lower = corpus.lower()
    hits = sum(1 for kw in keywords if kw.lower() in corpus_lower)
    return hits / len(keywords)


@dataclass
class CaseResult:
    query: str
    refs_count: int = 0
    context_precision: float = 0.0
    answer_coverage: float = 0.0
    answer_len: int = 0
    passed: bool = True
    reasons: list[str] = field(default_factory=list)


def run_case(
    base_url: str,
    api_key: str,
    session_id: str,
    kb_id: str,
    case: dict,
) -> CaseResult:
    mode = case.get("mode", "rag_fast")
    payload = {
        "query": case["query"],
        "mode": mode,
        "knowledge_base_ids": [kb_id],
        "channel": "api",
    }
    url = f"{base_url}/knowledge-chat/{session_id}"

    refs_content: list[str] = []
    answer_parts: list[str] = []
    for evt in _stream_sse(url, api_key, payload):
        rtype = evt.get("response_type", "")
        if rtype == "references":
            # Server emits knowledge_references (list of SearchResult).
            refs = (
                evt.get("knowledge_references")
                or evt.get("references")
                or (evt.get("data") or {}).get("references")
                or []
            )
            for ref in refs:
                text = ref.get("content") or ref.get("text") or ""
                if text:
                    refs_content.append(text)
        elif rtype == "answer":
            delta = evt.get("content", "") or (evt.get("data") or {}).get("content", "")
            if delta:
                answer_parts.append(delta)
        elif rtype == "complete":
            break

    answer = "".join(answer_parts)
    corpus = "\n".join(refs_content)
    expected = case.get("expected_keywords", []) or []

    result = CaseResult(
        query=case["query"],
        refs_count=len(refs_content),
        context_precision=_keyword_hit_ratio(expected, corpus),
        answer_coverage=_keyword_hit_ratio(expected, answer),
        answer_len=len(answer),
    )

    min_refs = case.get("min_refs", 0)
    if result.refs_count < min_refs:
        result.passed = False
        result.reasons.append(f"refs {result.refs_count} < min_refs {min_refs}")

    min_cp = case.get("min_context_precision", 0.0)
    if result.context_precision < min_cp:
        result.passed = False
        result.reasons.append(
            f"context_precision {result.context_precision:.2f} < min {min_cp:.2f}"
        )

    min_ac = case.get("min_answer_coverage", 0.0)
    if result.answer_coverage < min_ac:
        result.passed = False
        result.reasons.append(
            f"answer_coverage {result.answer_coverage:.2f} < min {min_ac:.2f}"
        )

    if case.get("require_answer", True) and result.answer_len == 0:
        result.passed = False
        result.reasons.append("empty answer")

    return result


# --- Runner ---------------------------------------------------------------


def _create_session(base_url: str, api_key: str, title: str) -> str:
    resp = _post_json(
        f"{base_url}/sessions",
        api_key,
        {"title": title},
    )
    return resp["data"]["id"]


def main(argv: list[str]) -> int:
    ap = argparse.ArgumentParser(description=__doc__)
    ap.add_argument("--base-url", required=True)
    ap.add_argument("--api-key", required=True)
    ap.add_argument("--kb-id", required=True)
    ap.add_argument("--fixture", required=True, help="JSON fixture file")
    ap.add_argument("--report", help="Optional path to write JSON report")
    args = ap.parse_args(argv)

    with open(args.fixture, encoding="utf-8") as f:
        cases: list[dict] = json.load(f)
    if not cases:
        print("empty fixture", file=sys.stderr)
        return 1

    all_results: list[CaseResult] = []
    any_failed = False
    for i, case in enumerate(cases):
        try:
            session = _create_session(args.base_url, args.api_key, f"ragas-{i}")
        except urllib.error.URLError as e:
            print(f"session create failed: {e}", file=sys.stderr)
            return 2
        try:
            r = run_case(args.base_url, args.api_key, session, args.kb_id, case)
        except Exception as e:
            r = CaseResult(query=case.get("query", "?"))
            r.passed = False
            r.reasons.append(f"exception: {e}")
        all_results.append(r)
        status = "PASS" if r.passed else "FAIL"
        print(
            f"[{status}] q={r.query!r} refs={r.refs_count} "
            f"cp={r.context_precision:.2f} ac={r.answer_coverage:.2f} "
            f"alen={r.answer_len}"
            + ("" if r.passed else f" -- {'; '.join(r.reasons)}")
        )
        if not r.passed:
            any_failed = True

    # Aggregate summary
    n = len(all_results)
    avg_cp = sum(r.context_precision for r in all_results) / n
    avg_ac = sum(r.answer_coverage for r in all_results) / n
    avg_refs = sum(r.refs_count for r in all_results) / n
    print(
        f"\nSummary: n={n} avg_context_precision={avg_cp:.2f} "
        f"avg_answer_coverage={avg_ac:.2f} avg_refs={avg_refs:.1f}"
    )

    if args.report:
        with open(args.report, "w", encoding="utf-8") as f:
            json.dump(
                {
                    "summary": {
                        "n": n,
                        "avg_context_precision": avg_cp,
                        "avg_answer_coverage": avg_ac,
                        "avg_refs": avg_refs,
                        "failed": sum(1 for r in all_results if not r.passed),
                    },
                    "cases": [r.__dict__ for r in all_results],
                },
                f,
                ensure_ascii=False,
                indent=2,
            )

    return 1 if any_failed else 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
