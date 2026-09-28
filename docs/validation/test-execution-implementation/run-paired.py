"""Measure the two test-only reductions against the same product source.

The pre-change test files come from the recorded main commit. Go overlays keep
the checkout untouched; compilation is preparation and is excluded from the
paired execution times.
"""

import json
import pathlib
import statistics
import subprocess
import tempfile
import time


ROOT = pathlib.Path(__file__).resolve().parents[3]
PACKAGE = ROOT / "internal" / "steerline"
OUT = pathlib.Path(__file__).resolve().parent
BASE = "9877cb0"
FIXTURE_TEST = "TestDiagnosticFixtureBatch"
DAG_TEST = "TestTaskBatchRejectsDuplicateAndCrossProjectDependencies"
PROBE = """
func TestDiagnosticFixtureBatch(t *testing.T) {
    for i := 0; i < 10; i++ {
        t.Run(strings.Repeat("x", i+1), func(t *testing.T) {
            c, commit := nativeTaskFixture(t)
            if c.Repository == "" || commit == "" { t.Fatal("fixture incomplete") }
        })
    }
}
"""


def output(*args):
    return subprocess.check_output(args, cwd=ROOT, text=True, encoding="utf-8")


def build(temp, name, replacements):
    overlay = {"Replace": {}}
    for source, contents in replacements.items():
        replacement = temp / (name + "-" + pathlib.Path(source).name)
        replacement.write_text(contents, encoding="utf-8")
        overlay["Replace"][str(ROOT / source)] = str(replacement)
    path = temp / (name + ".json")
    path.write_text(json.dumps(overlay), encoding="utf-8")
    binary = temp / (name + ".exe")
    command = ["go", "test", "-c", "-overlay", str(path), "-o", str(binary), "./internal/steerline"]
    subprocess.run(command, cwd=ROOT, check=True)
    return binary


def measure(binary, case, variant, pair, test):
    command = [str(binary), "-test.run=^" + test + "$", "-test.v", "-test.count=1", "-test.parallel=1", "-test.timeout=120s"]
    started = time.perf_counter()
    run = subprocess.run(command, cwd=PACKAGE, capture_output=True, text=True, timeout=150)
    seconds = time.perf_counter() - started
    log = OUT / f"{case}-{pair}-{variant}.log"
    log.write_text(run.stdout + run.stderr, encoding="utf-8")
    if run.returncode or "--- PASS: " + test not in run.stdout:
        raise RuntimeError(f"{case} {pair} {variant} failed: {log}")
    return {"case": case, "pair": pair, "variant": variant, "seconds": seconds, "exit_code": run.returncode, "log": log.name}


def main():
    OUT.mkdir(parents=True, exist_ok=True)
    native = "internal/steerline/native_tasks_test.go"
    profiles = "internal/steerline/execution_profiles_test.go"
    dag = "internal/steerline/task_dag_test.go"
    base_native = output("git", "show", BASE + ":" + native)
    base_profiles = output("git", "show", BASE + ":" + profiles)
    base_dag = output("git", "show", BASE + ":" + dag)
    current_native = (ROOT / native).read_text(encoding="utf-8")
    report = {
        "baseline_commit": output("git", "rev-parse", BASE).strip(),
        "product_source_commit": output("git", "rev-parse", "HEAD").strip(),
        "go_version": output("go", "version").strip(),
        "method": "three alternating pairs; same product source; test-only Go overlays; build preparation excluded",
        "cases": [],
    }
    with tempfile.TemporaryDirectory(prefix="steerline-paired-") as directory:
        temp = pathlib.Path(directory)
        binaries = {
            "fixture_before": build(temp, "fixture-before", {native: base_native + PROBE, profiles: base_profiles}),
            "after": build(temp, "after", {native: current_native + PROBE}),
            "dag_before": build(temp, "dag-before", {native: current_native + PROBE, dag: base_dag}),
        }
        for case, test, before in [("fixture", FIXTURE_TEST, "fixture_before"), ("dag", DAG_TEST, "dag_before")]:
            runs = []
            for pair in range(1, 4):
                for variant in (["before", "after"] if pair % 2 else ["after", "before"]):
                    binary = binaries[before if variant == "before" else "after"]
                    record = measure(binary, case, variant, pair, test)
                    runs.append(record)
                    print(json.dumps(record), flush=True)
            before_median = statistics.median(r["seconds"] for r in runs if r["variant"] == "before")
            after_median = statistics.median(r["seconds"] for r in runs if r["variant"] == "after")
            report["cases"].append({"case": case, "test": test, "before_median_seconds": before_median, "after_median_seconds": after_median, "ratio": after_median / before_median, "runs": runs})
            (OUT / "paired-summary.json").write_text(json.dumps(report, indent=2) + "\n", encoding="utf-8")


if __name__ == "__main__":
    main()
