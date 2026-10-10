#!/usr/bin/env python3
"""Check that docs/baseline.json still describes the code it was generated from.

Two cheap, deterministic checks that need no database:

1. every ``e.Router.<METHOD>("<path>"`` in apps/server/internal/app appears in
   baseline.json "routes", and every baseline route still appears in the source;
2. the toolchain recorded in baseline.json still matches the pinned versions
   used by .github/workflows/ci.yml.

Exits non-zero and prints a unified-ish diff of the drift so CI can fail on it.
"""

from __future__ import annotations

import json
import os
import re
import sys

REPO = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
BASELINE = os.path.join(REPO, "docs", "baseline.json")
WORKFLOW = os.path.join(REPO, ".github", "workflows", "ci.yml")
SERVER = os.path.join(REPO, "apps", "server", "internal", "app")

ROUTE_RE = re.compile(r'e\.Router\.(GET|POST|PUT|PATCH|DELETE|Any|OPTIONS)\("([^"]+)"')

EXPECTED_NODE = "22.23.2"
EXPECTED_PNPM = "11.8.0"
EXPECTED_GO = "1.27.1"


def source_routes() -> set[tuple[str, str]]:
    found: set[tuple[str, str]] = set()
    for name in sorted(os.listdir(SERVER)):
        if not name.endswith(".go") or name.endswith("_test.go"):
            continue
        with open(os.path.join(SERVER, name), encoding="utf-8") as fh:
            text = fh.read()
        for method, path in ROUTE_RE.findall(text):
            found.add((method, path))
    return found


def workflow_pins() -> dict[str, str]:
    with open(WORKFLOW, encoding="utf-8") as fh:
        text = fh.read()
    node = re.search(r"node-version:\s*['\"]?([\w.]+)", text)
    pnpm = re.search(r"version:\s*['\"]?([\w.]+)", text)
    go = re.search(r"go-version:\s*['\"]?([\w.]+)", text)
    return {
        "node": node.group(1) if node else "",
        "pnpm": pnpm.group(1) if pnpm else "",
        "go": go.group(1) if go else "",
    }


def main() -> int:
    with open(BASELINE, encoding="utf-8") as fh:
        baseline = json.load(fh)

    failures: list[str] = []

    recorded = {(r["method"].upper(), r["path"]) for r in baseline["routes"]}
    actual = {(method.upper(), path) for method, path in source_routes()}

    for missing in sorted(actual - recorded):
        failures.append(f"route in source but not in baseline.json: {missing[0]} {missing[1]}")
    for stale in sorted(recorded - actual):
        failures.append(f"route in baseline.json but not in source: {stale[0]} {stale[1]}")

    pins = workflow_pins()
    tools = baseline["toolVersions"]
    for key, expected, actual_pin in (
        ("node", EXPECTED_NODE, pins["node"]),
        ("pnpm", EXPECTED_PNPM, pins["pnpm"]),
        ("go", EXPECTED_GO, pins["go"]),
    ):
        if actual_pin != expected:
            failures.append(
                f"ci.yml {key} version is {actual_pin!r}, baseline expects {expected!r}"
            )
        recorded_version = re.sub(r"^go", "", str(tools[key]).lstrip("v"))
        if recorded_version != expected:
            failures.append(
                f"baseline.json toolVersions.{key} is {recorded_version!r}, expected {expected!r}"
            )

    if failures:
        print("baseline drift detected:")
        for line in failures:
            print(f"  - {line}")
        print("\nUpdate docs/baseline.json deliberately, or fix the code.")
        return 1

    print(f"baseline.json matches source: {len(actual)} routes, toolchain pinned "
          f"(go {pins['go']}, node {pins['node']}, pnpm {pins['pnpm']})")
    return 0


if __name__ == "__main__":
    sys.exit(main())