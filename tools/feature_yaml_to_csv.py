"""Convert a generated feature YAML test plan into a CSV summary.

Usage:
    python feature_yaml_to_csv.py <input.yaml> [output.csv]

If output.csv is omitted, it is written next to the input file with the
same base name (e.g. radius-server.yaml -> radius-server.csv).

Columns:
    TestCaseID, Type, Priority, Automation, Description, IsDeploymentTest
"""

from __future__ import annotations

import csv
import sys
from pathlib import Path

import yaml


COLUMNS = [
    "TestCaseID",
    "Type",
    "Priority",
    "Automation",
    "Description",
    "IsDeploymentTest",
]


def iter_test_cases(doc: dict):
    """Yield every test case dict in the document, in file order."""
    for feature in doc.get("features", []) or []:
        tests = feature.get("tests") or {}
        # tests is a mapping: category -> [test, ...]
        for category, cases in tests.items():
            if not cases:
                continue
            for tc in cases:
                # Default type to the category key when not present on the case.
                if "type" not in tc or tc.get("type") in (None, ""):
                    tc = {**tc, "type": category}
                yield tc


def to_row(tc: dict) -> dict:
    return {
        "TestCaseID": tc.get("testCaseID", ""),
        "Type": tc.get("type", ""),
        "Priority": tc.get("priority", ""),
        "Automation": tc.get("automation", ""),
        "Description": tc.get("description", ""),
        "IsDeploymentTest": tc.get("isDeploymentTest", False),
    }


def convert(input_path: Path, output_path: Path) -> int:
    with input_path.open("r", encoding="utf-8") as fh:
        doc = yaml.safe_load(fh)

    rows = [to_row(tc) for tc in iter_test_cases(doc)]

    with output_path.open("w", encoding="utf-8", newline="") as fh:
        writer = csv.DictWriter(fh, fieldnames=COLUMNS)
        writer.writeheader()
        writer.writerows(rows)

    return len(rows)


def main(argv: list[str]) -> int:
    if len(argv) < 2:
        print(__doc__)
        return 2

    input_path = Path(argv[1])
    if not input_path.is_file():
        print(f"Error: input file not found: {input_path}", file=sys.stderr)
        return 1

    output_path = Path(argv[2]) if len(argv) > 2 else input_path.with_suffix(".csv")

    count = convert(input_path, output_path)
    print(f"Wrote {count} test cases to {output_path}")
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv))
