"""Archive raw test streams and compare one serial/full/quick measurement set."""

import argparse
import gzip
import hashlib
import json
import pathlib


OUT = pathlib.Path(__file__).resolve().parent


def read(path):
    return json.loads(pathlib.Path(path).read_text(encoding="utf-8"))


def outcomes(summary):
    return sorted((test["package"], test["name"], test["status"]) for test in summary["tests"])


def archive(label, path):
    source = pathlib.Path(path)
    raw = source.read_bytes()
    target = OUT / (label + ".jsonl.gz")
    with target.open("wb") as output:
        with gzip.GzipFile(filename="", mode="wb", fileobj=output, mtime=0) as compressed:
            compressed.write(raw)
    return {"file": target.name, "raw_sha256": hashlib.sha256(raw).hexdigest(), "raw_bytes": len(raw)}


def main():
    parser = argparse.ArgumentParser()
    for name in ("full", "serial", "quick", "quick_focused"):
        parser.add_argument("--" + name.replace("_", "-"), required=True)
    args = parser.parse_args()
    full, serial, quick, focused = (read(getattr(args, name)) for name in ("full", "serial", "quick", "quick_focused"))
    assert full["scope"] == serial["scope"] == "full"
    assert quick["scope"] == focused["scope"] == "quick"
    assert all(report["passed"] for report in (full, quick, focused)) and serial["summary"]["passed"]
    full_tests = sorted(item for group in full["groups"] for item in outcomes(group["summary"]))
    serial_tests = outcomes(serial["summary"])
    assert full_tests == serial_tests, "serial and three-process terminal outcomes differ"
    manifest = {
        "source_commit": full["environment"]["source_commit"],
        "go_version": full["environment"]["go_version"],
        "machine": full["environment"]["machine"],
        "native_switches": full["environment"]["enabled_native_acceptance_switches"],
        "serial_wall_seconds": serial["wall_seconds"],
        "full_preparation_seconds": full["preparation_seconds"],
        "full_execution_seconds": full["execution_seconds"],
        "full_to_serial_ratio": full["execution_seconds"] / serial["wall_seconds"],
        "quick_execution_seconds": quick["execution_seconds"],
        "quick_focused_execution_seconds": focused["execution_seconds"],
        "top_level_core_tests": len(full["enumerated_core_tests"]),
        "packages": len(full["enumerated_packages"]),
        "terminal_tests_and_subtests": len(full_tests),
        "skipped_tests_and_subtests": sum(item[2] == "skip" for item in full_tests),
        "terminal_outcomes_sha256": hashlib.sha256(json.dumps(full_tests, ensure_ascii=False).encode("utf-8")).hexdigest(),
        "groups": [{"id": group["id"], "assigned_tests": len(group.get("assigned_tests") or []), "wall_seconds": group["wall_seconds"]} for group in full["groups"]],
        "raw_logs": {},
    }
    manifest["raw_logs"]["serial"] = archive("serial", serial["stdout_json_log"])
    for label, report in (("full", full), ("quick", quick), ("quick-focused", focused)):
        for group in report["groups"]:
            key = label + "-" + group["id"]
            manifest["raw_logs"][key] = archive(key, group["stdout_json_log"])
    (OUT / "suite-comparison.json").write_text(json.dumps(manifest, indent=2) + "\n", encoding="utf-8")
    print(json.dumps({key: manifest[key] for key in ("serial_wall_seconds", "full_execution_seconds", "full_to_serial_ratio", "terminal_tests_and_subtests")}, indent=2))


if __name__ == "__main__":
    main()
