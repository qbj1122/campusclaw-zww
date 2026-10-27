#!/usr/bin/env python3
"""Discover and run every CampusClaw API test case.

Usage:
    python3 apitest/run_all.py
    python3 apitest/run_all.py --filter materials
    python3 apitest/run_all.py --filter api/mcp --verbose
"""

from __future__ import annotations

import argparse
import html
import importlib.util
import sys
import time
import traceback
from dataclasses import dataclass, field
from pathlib import Path

from lib import CaseFailure, Ctx, load_config


ROOT = Path(__file__).resolve().parent
CASES_DIR = ROOT / "cases"
REPORT_PATH = ROOT / "report.html"


@dataclass
class CaseResult:
    case_id: str
    path: Path
    status: str = "PASS"
    duration: float = 0.0
    error: str = ""
    traceback_text: str = ""
    output: str = ""


@dataclass
class RunSummary:
    results: list[CaseResult] = field(default_factory=list)
    warnings: list[str] = field(default_factory=list)
    cleanup_errors: list[str] = field(default_factory=list)
    started_at: float = field(default_factory=time.time)
    finished_at: float = 0.0

    @property
    def passed(self) -> int:
        return sum(1 for result in self.results if result.status == "PASS")

    @property
    def failed(self) -> int:
        return sum(1 for result in self.results if result.status == "FAIL")

    @property
    def duration(self) -> float:
        return (self.finished_at or time.time()) - self.started_at


def discover(filter_text: str | None = None) -> list[tuple[str, Path]]:
    if not CASES_DIR.is_dir():
        raise SystemExit(f"cases directory not found: {CASES_DIR}")
    found: list[tuple[str, Path]] = []
    for path in sorted(CASES_DIR.rglob("*.py")):
        if path.name.startswith("_") or "__pycache__" in path.parts:
            continue
        relative = path.relative_to(CASES_DIR).with_suffix("")
        case_id = relative.as_posix()
        if filter_text and filter_text.lower() not in case_id.lower():
            continue
        found.append((case_id, path))
    return found


def import_case(path: Path, index: int):
    module_name = f"campusclaw_case_{index}_{abs(hash(str(path)))}"
    spec = importlib.util.spec_from_file_location(module_name, path)
    if spec is None or spec.loader is None:
        raise RuntimeError(f"cannot import {path}")
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    if not hasattr(module, "run"):
        raise RuntimeError(f"{path} does not define run(ctx)")
    return module


def run_case(ctx: Ctx, case_id: str, path: Path, index: int) -> CaseResult:
    result = CaseResult(case_id=case_id, path=path)
    started = time.perf_counter()
    try:
        module = import_case(path, index)
        module.run(ctx)
    except CaseFailure as exc:
        result.status = "FAIL"
        result.error = str(exc)
    except Exception:  # noqa: BLE001 - show unexpected failures in the report
        result.status = "FAIL"
        result.error = "unexpected exception"
        result.traceback_text = traceback.format_exc()
    finally:
        result.duration = time.perf_counter() - started
    return result


def write_report(summary: RunSummary, config: dict) -> None:
    rows = []
    details = []
    for result in summary.results:
        css = "pass" if result.status == "PASS" else "fail"
        rows.append(
            "<tr class='{css}'><td>{status}</td><td>{case}</td>"
            "<td>{duration:.3f}s</td><td>{error}</td></tr>".format(
                css=css,
                status=html.escape(result.status),
                case=html.escape(result.case_id),
                duration=result.duration,
                error=html.escape(result.error.splitlines()[0] if result.error else ""),
            )
        )
        if result.error or result.traceback_text:
            body = result.error
            if result.traceback_text:
                body += "\n\n" + result.traceback_text
            details.append(
                "<details class='{css}'><summary>{status} {case}</summary>"
                "<pre>{body}</pre></details>".format(
                    css=css,
                    status=html.escape(result.status),
                    case=html.escape(result.case_id),
                    body=html.escape(body),
                )
            )

    warnings_html = ""
    if summary.warnings:
        warnings_html = "<h2>Warnings</h2><ul>" + "".join(
            f"<li>{html.escape(item)}</li>" for item in summary.warnings
        ) + "</ul>"
    cleanup_html = ""
    if summary.cleanup_errors:
        cleanup_html = "<h2>Cleanup errors</h2><ul>" + "".join(
            f"<li>{html.escape(item)}</li>" for item in summary.cleanup_errors
        ) + "</ul>"

    db = config["db"]
    generated = time.strftime("%Y-%m-%d %H:%M:%S")
    document = f"""<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<title>CampusClaw API test report</title>
<style>
body {{ font-family: system-ui, sans-serif; margin: 2rem; color: #1f2937; }}
h1 {{ margin-bottom: .25rem; }}
.meta {{ color: #4b5563; margin-bottom: 1.5rem; }}
.summary span {{ display: inline-block; margin-right: 1.5rem; font-weight: 600; }}
.summary .pass {{ color: #047857; }}
.summary .fail {{ color: #b91c1c; }}
table {{ border-collapse: collapse; width: 100%; margin-top: 1rem; }}
th, td {{ border-bottom: 1px solid #e5e7eb; padding: .45rem .6rem; text-align: left; vertical-align: top; }}
tr.pass td:first-child {{ color: #047857; font-weight: 600; }}
tr.fail td:first-child {{ color: #b91c1c; font-weight: 600; }}
tr.fail {{ background: #fef2f2; }}
pre {{ background: #f3f4f6; padding: 1rem; overflow-x: auto; white-space: pre-wrap; }}
details {{ margin: .6rem 0; }}
li {{ margin: .25rem 0; }}
</style>
</head>
<body>
<h1>CampusClaw API test report</h1>
<div class="meta">
Generated {html.escape(generated)} · base URL {html.escape(config['base_url'])} ·
DB {html.escape(db['host'])}:{db['port']}/{html.escape(db['database'])} ·
duration {summary.duration:.2f}s
</div>
<div class="summary">
<span class="pass">{summary.passed} passed</span>
<span class="fail">{summary.failed} failed</span>
<span>{len(summary.results)} total</span>
</div>
{warnings_html}
{cleanup_html}
<table>
<thead><tr><th>Status</th><th>Case</th><th>Duration</th><th>Error</th></tr></thead>
<tbody>
{''.join(rows)}
</tbody>
</table>
<h2>Failure details</h2>
{''.join(details) if details else '<p>No failures.</p>'}
</body>
</html>
"""
    REPORT_PATH.write_text(document, encoding="utf-8")


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--filter", help="only run cases whose path contains this text")
    parser.add_argument("--verbose", action="store_true", help="print full tracebacks")
    args = parser.parse_args()

    config = load_config()
    ctx = Ctx(config)
    summary = RunSummary()
    cases = discover(args.filter)

    print(f"CampusClaw API tests · {config['base_url']}")
    print(f"DB {config['db']['host']}:{config['db']['port']}/{config['db']['database']}")
    print(f"Discovered {len(cases)} case(s)")
    print("-" * 78)

    if not cases:
        print("No cases matched.")
        return 2

    try:
        health = ctx.get("/health", expect=200)
        if ctx.json(health).get("status") != "ok":
            raise CaseFailure(f"health check returned {ctx.safe_text(health)}")
        pruned = ctx.prune_orphan_vectors()
        if pruned:
            print(f"Pruned {pruned} orphan vector point(s) from previous runs")
    except Exception as exc:  # noqa: BLE001
        print(f"FATAL: app is not reachable at {config['base_url']}: {exc}")
        ctx.close()
        return 2

    try:
        for index, (case_id, path) in enumerate(cases):
            result = run_case(ctx, case_id, path, index)
            summary.results.append(result)
            status = "PASS" if result.status == "PASS" else "FAIL"
            print(f"{status:4}  {case_id:<48} {result.duration:6.2f}s")
            if result.status == "FAIL":
                print(f"      {result.error}")
                if args.verbose and result.traceback_text:
                    print(result.traceback_text)
    finally:
        summary.finished_at = time.time()
        summary.warnings = list(ctx.warnings)
        summary.cleanup_errors = ctx.cleanup()
        ctx.close()
        write_report(summary, config)

    print("-" * 78)
    print(
        f"{summary.passed} passed, {summary.failed} failed, "
        f"{len(summary.results)} total in {summary.duration:.2f}s"
    )
    print(f"HTML report: {REPORT_PATH}")
    if summary.warnings:
        print(f"Warnings ({len(summary.warnings)}):")
        for warning in summary.warnings:
            print(f"  - {warning}")
    if summary.cleanup_errors:
        print(f"Cleanup errors ({len(summary.cleanup_errors)}):")
        for error in summary.cleanup_errors:
            print(f"  - {error}")
    return 1 if summary.failed else 0


if __name__ == "__main__":
    sys.exit(main())
