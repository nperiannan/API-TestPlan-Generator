#!/usr/bin/env python3
"""Audit all test plans for POST/PUT/PATCH steps missing request body."""
import os
import yaml
import sys

missing = []
total_steps = 0
body_steps = 0

for root, dirs, files in os.walk("Testplans"):
    for f in sorted(files):
        if not f.endswith(".yaml"):
            continue
        path = os.path.join(root, f)
        with open(path, "r", encoding="utf-8") as fh:
            data = yaml.safe_load(fh)
        if not data or "features" not in data:
            continue
        for feat in data["features"]:
            tests = feat.get("tests", {})
            for cat, cases in tests.items():
                if not isinstance(cases, list):
                    continue
                for tc in cases:
                    for step in tc.get("steps", []):
                        total_steps += 1
                        method = step.get("method", "").upper()
                        body = step.get("body")
                        if method in ("POST", "PUT", "PATCH"):
                            body_steps += 1
                            if not body:
                                missing.append({
                                    "file": os.path.relpath(path, "Testplans"),
                                    "tcid": tc.get("testCaseID", "?"),
                                    "type": cat,
                                    "step": step.get("name", "?"),
                                    "method": method,
                                    "path": step.get("path", ""),
                                    "desc": tc.get("description", "")[:80],
                                })

print(f"Total steps scanned: {total_steps}")
print(f"POST/PUT/PATCH steps: {body_steps}")
print(f"POST/PUT/PATCH steps MISSING body: {len(missing)}")
print(f"Coverage: {(body_steps - len(missing)) * 100 / body_steps:.1f}% have payload\n")

if missing:
    # Group by file
    by_file = {}
    for m in missing:
        by_file.setdefault(m["file"], []).append(m)
    
    for f, items in sorted(by_file.items()):
        print(f"=== {f} ({len(items)} missing) ===")
        for m in items:
            print(f"  {m['tcid']} [{m['type']}] {m['step']} -> {m['method']} {m['path']}")
            print(f"    {m['desc']}")
        print()
else:
    print("All POST/PUT/PATCH steps have body payloads!")
