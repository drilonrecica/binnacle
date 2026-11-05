#!/usr/bin/env python3
# SPDX-License-Identifier: AGPL-3.0-only
"""Reproducible benchmark harness for Binnacle.

Two scenarios run the binnacle binary in deterministic demo mode:

* ``long-run`` (the release gate, RUN-9): a first start seeds 48 h of raw 10 s
  history plus one already expired hour; a second start on the same database
  runs with the stock retention, the check scheduler and eight SSE clients, so
  the measured run carries a realistic database, one retention and vacuum
  pass that deletes that hour, and the rollup worker on top of live writes.
* ``short``: a fresh demo database, for quick local comparisons.

Every run is checked against the targets in TARGETS (documented in
docs/operations/benchmarking.md). A missed target, or one that could not be
measured, makes the harness exit with status 1.
"""

import argparse
import http.cookiejar
import json
import os
import shutil
import signal
import socket
import sqlite3
import statistics
import subprocess
import sys
import tempfile
import threading
import time
import urllib.request
from pathlib import Path
from typing import Any

SETUP_TOKEN = "binnacle-benchmark-token-32chars-long"
MiB = 1 << 20

# Targets per container tier: a run uses the smallest tier that is at least
# its container count. Each entry is (metric, limit); every metric is "lower
# is better" and must be measured. Keep docs/operations/benchmarking.md in
# step with this table.
TARGETS: dict[int, list[tuple[str, float]]] = {
    30: [
        ("rss_max_mib", 100),
        ("cpu_avg_percent", 2.0),
        ("write_p95_ms", 50),
        ("rollup_p95_ms", 250),
        ("retention_ms", 2000),
        ("vacuum_ms", 2000),
        ("wal_max_mib", 64),
        ("sse_bytes_per_second_per_client", 20 * 1024),
        ("dropped_batches", 0),
        ("worker_failures", 0),
        ("write_transactions_per_second", 1.0),
    ],
    100: [
        ("rss_max_mib", 128),
        ("cpu_avg_percent", 3.0),
        ("write_p95_ms", 100),
        ("rollup_p95_ms", 500),
        ("retention_ms", 5000),
        ("vacuum_ms", 2000),
        ("wal_max_mib", 64),
        ("sse_bytes_per_second_per_client", 64 * 1024),
        ("dropped_batches", 0),
        ("worker_failures", 0),
        ("write_transactions_per_second", 1.0),
    ],
}
# Metrics only the long-run scenario produces; a short run reports them as
# not applicable instead of failing.
LONG_RUN_ONLY = {"retention_ms", "vacuum_ms"}


class Client:
    """HTTP client with its own cookie jar (one session per process run)."""

    def __init__(self, base_url: str) -> None:
        self.base_url = base_url
        self.opener = urllib.request.build_opener(urllib.request.HTTPCookieProcessor(http.cookiejar.CookieJar()))

    def get_json(self, path: str, timeout: float = 5.0) -> Any:
        with self.opener.open(self.base_url + path, timeout=timeout) as resp:  # noqa: S310
            return json.loads(resp.read().decode("utf-8"))

    def post_json(self, path: str, body: dict[str, Any], timeout: float = 5.0) -> None:
        req = urllib.request.Request(  # noqa: S310
            self.base_url + path,
            data=json.dumps(body).encode("utf-8"),
            headers={"Content-Type": "application/json"},
            method="POST",
        )
        with self.opener.open(req, timeout=timeout) as resp:  # noqa: S310
            resp.read()

    def claim(self) -> None:
        self.post_json("/api/v1/setup/claim", {"token": SETUP_TOKEN, "username": "benchmark", "password": "benchmark-password-32chars-long"})

    def wait_ready(self, process: subprocess.Popen[bytes], timeout: float) -> None:
        deadline = time.time() + timeout
        last: Exception | None = None
        while time.time() < deadline:
            if process.poll() is not None:
                raise RuntimeError(f"binnacle exited with status {process.returncode} before it became ready")
            try:
                self.opener.open(self.base_url + "/healthz", timeout=2.0)  # noqa: S310
                return
            except Exception as e:  # noqa: BLE001
                last = e
                time.sleep(0.5)
        raise RuntimeError(f"server did not become ready within {timeout:.0f}s: {last}")


def find_port() -> int:
    with socket.socket(socket.AF_INET, socket.SOCK_STREAM) as s:
        s.bind(("127.0.0.1", 0))
        return int(s.getsockname()[1])


def read_proc(pid: int) -> tuple[float, int] | None:
    """CPU seconds (user+system) and RSS bytes of pid."""
    try:
        with open(f"/proc/{pid}/stat", "rb") as f:  # noqa: S108
            parts = f.read().rsplit(b")", 1)[1].split()
        # After the command name: index 11/12 are utime/stime, 21 is rss pages.
        ticks = int(parts[11]) + int(parts[12])
        return ticks / os.sysconf("SC_CLK_TCK"), int(parts[21]) * os.sysconf("SC_PAGE_SIZE")
    except (OSError, IndexError, ValueError):
        return None


def file_size(path: Path) -> int:
    try:
        return path.stat().st_size
    except OSError:
        return 0


class Sampler:
    """Samples process CPU/RSS, the WAL size and monitor-health."""

    def __init__(self, pid: int, client: Client, db_path: Path, interval: float) -> None:
        self.pid, self.client, self.db_path, self.interval = pid, client, db_path, interval
        self.stopped = threading.Event()
        self.samples: list[dict[str, Any]] = []
        self.errors = 0
        self.thread = threading.Thread(target=self._loop, daemon=True)

    def start(self) -> None:
        self.thread.start()

    def stop(self) -> None:
        self.stopped.set()
        self.thread.join(timeout=self.interval + 5)

    def _loop(self) -> None:
        while True:
            sample: dict[str, Any] = {"at": time.time(), "wal_bytes": file_size(Path(str(self.db_path) + "-wal"))}
            proc = read_proc(self.pid)
            if proc is not None:
                sample["cpu_seconds"], sample["rss_bytes"] = proc
            try:
                data = self.client.get_json("/api/v1/monitor-health", timeout=5.0)
                sample["metrics"] = {m["id"]: m["value"] for m in data.get("metrics", [])}
            except Exception:  # noqa: BLE001
                self.errors += 1
            self.samples.append(sample)
            if self.stopped.wait(self.interval):
                return


class SSEClient:
    """One live stream held open for the whole run."""

    def __init__(self, client: Client, duration: float) -> None:
        self.client, self.duration = client, duration
        self.bytes_per_second: float | None = None
        self.error: str | None = None
        self.thread = threading.Thread(target=self._run, daemon=True)

    def start(self) -> None:
        self.thread.start()

    def join(self) -> None:
        self.thread.join(timeout=self.duration + 15)

    def _run(self) -> None:
        req = urllib.request.Request(self.client.base_url + "/api/v1/live", headers={"Accept": "text/event-stream"})  # noqa: S310
        start, total = time.time(), 0
        try:
            with self.client.opener.open(req, timeout=15) as resp:  # noqa: S310
                while time.time() - start < self.duration:
                    chunk = resp.read1(65536)
                    if not chunk:
                        break
                    total += len(chunk)
            elapsed = time.time() - start
            if elapsed >= self.duration * 0.9:
                self.bytes_per_second = total / elapsed
            else:
                self.error = f"stream ended after {elapsed:.0f}s"
        except Exception as e:  # noqa: BLE001
            self.error = str(e)


def latency(snapshot: dict[str, Any], op: str) -> dict[str, Any] | None:
    for entry in snapshot.get("latency", []):
        if entry.get("id") == op:
            return entry
    return None


def histogram_quantile(before: dict[str, Any] | None, after: dict[str, Any] | None, q: float) -> tuple[float | None, int]:
    """The q-quantile (ms) of the runs between two cumulative histograms.

    Linear interpolation inside the bucket, as Prometheus histogram_quantile
    does; a quantile in the unbounded bucket is infinite. Returns the value
    and the number of runs it covers (None when there were none).
    """
    if after is None or not after.get("histogram"):
        return None, 0
    counts = [b["count"] for b in after["histogram"]]
    if before is not None and before.get("histogram"):
        counts = [a - b["count"] for a, b in zip(counts, before["histogram"], strict=True)]
    total = counts[-1]
    if total <= 0:
        return None, 0
    rank = q * total
    lower_bound, lower_count = 0.0, 0
    for bucket, count in zip(after["histogram"], counts, strict=True):
        if count >= rank:
            if bucket["le"] is None:
                return float("inf"), total
            upper = float(bucket["le"])
            inside = count - lower_count
            fraction = (rank - lower_count) / inside if inside else 1.0
            return lower_bound + (upper - lower_bound) * fraction, total
        if bucket["le"] is not None:
            lower_bound = float(bucket["le"])
        lower_count = count
    return float("inf"), total


def failure_counts(snapshot: dict[str, Any]) -> dict[str, int]:
    return {f["id"]: int(f["count"]) for f in snapshot.get("failures", [])}


def agg(values: list[float]) -> dict[str, float | None]:
    if not values:
        return {"avg": None, "max": None, "p95": None}
    values = sorted(values)
    return {"avg": statistics.mean(values), "max": values[-1], "p95": values[min(len(values) - 1, int(len(values) * 0.95))]}


def raw_history(db_path: Path) -> dict[str, Any]:
    """Raw sample rows and their span, read after the process stopped."""
    with sqlite3.connect(db_path) as db:
        out: dict[str, Any] = {}
        for table in ("host_samples_10s", "resource_samples_10s", "container_instance_samples_10s"):
            count, oldest, newest = db.execute(f"SELECT COUNT(*), MIN(ts), MAX(ts) FROM {table}").fetchone()  # noqa: S608
            out[table] = {"rows": count, "span_hours": (newest - oldest) / 3_600_000 if oldest is not None else None, "oldest_ms": oldest}
        return out


def checks_run_since(db_path: Path, since: float) -> tuple[int, int]:
    """Enabled checks, and how many have a result stored after since."""
    with sqlite3.connect(db_path) as db:
        total = db.execute("SELECT COUNT(*) FROM health_checks WHERE enabled=1").fetchone()[0]
        ran = db.execute("SELECT COUNT(*) FROM health_check_state WHERE checked_at>=?", (int(since),)).fetchone()[0]
        return int(total), int(ran)


def stop(process: subprocess.Popen[bytes]) -> None:
    if process.poll() is not None:
        return
    process.send_signal(signal.SIGTERM)
    try:
        process.wait(timeout=30)
    except subprocess.TimeoutExpired:
        process.kill()
        process.wait(timeout=10)


def environment(data_dir: Path, port: int, extra: dict[str, str] | None = None) -> dict[str, str]:
    env = os.environ.copy()
    for key in [k for k in env if k.startswith("BINNACLE_")]:
        del env[key]
    env.update({
        "BINNACLE_LISTEN_ADDRESS": f"127.0.0.1:{port}",
        "BINNACLE_DATA_DIR": str(data_dir),
        "BINNACLE_RUNTIME_DIR": str(data_dir / "run"),
        "BINNACLE_DATABASE_PATH": str(data_dir / "binnacle.db"),
        "BINNACLE_SETUP_TOKEN": SETUP_TOKEN,
    })
    env.update(extra or {})
    return env


def seed(binary: Path, data_dir: Path, containers: int, checks: int, raw_hours: int, log: Any) -> float:
    """First start: seed raw_hours of retained raw history plus one expired
    hour, then stop. The long-term preset keeps a retention pass that runs
    while seeding from deleting the expired hour before the measured run."""
    port = find_port()
    env = environment(data_dir, port, {"BINNACLE_RETENTION_PRESET": "long-term"})
    cmd = [str(binary), "--demo", "--demo-seed", "1", "--demo-containers", str(containers), "--demo-checks", str(checks), "--demo-raw-history", f"{raw_hours + 1}h"]
    started = time.time()
    process = subprocess.Popen(cmd, env=env, stdout=subprocess.DEVNULL, stderr=log)  # noqa: S603
    try:
        Client(f"http://127.0.0.1:{port}").wait_ready(process, timeout=1800)
        return time.time() - started
    finally:
        stop(process)


def measure(args: argparse.Namespace, binary: Path, data_dir: Path, containers: int, log: Any) -> dict[str, Any]:
    port = find_port()
    db_path = data_dir / "binnacle.db"
    client = Client(f"http://127.0.0.1:{port}")
    cmd = [str(binary), "--demo", "--demo-seed", "1", "--demo-containers", str(containers), "--demo-checks", str(args.checks)]
    if args.scenario == "long-run":
        cmd.append("--demo-run-checks")
    started = time.time()
    process = subprocess.Popen(cmd, env=environment(data_dir, port), stdout=subprocess.DEVNULL, stderr=log)  # noqa: S603
    try:
        client.wait_ready(process, timeout=300)
        client.claim()
        if args.warmup:
            time.sleep(args.warmup)
        first = client.get_json("/api/v1/monitor-health")
        measured_from = time.time()
        sampler = Sampler(process.pid, client, db_path, args.sample_interval)
        sampler.start()
        streams = [SSEClient(client, float(args.duration)) for _ in range(args.sse_clients)]
        for stream in streams:
            stream.start()
        time.sleep(args.duration)
        for stream in streams:
            stream.join()
        sampler.stop()
        last = client.get_json("/api/v1/monitor-health")
        if process.poll() is not None:
            raise RuntimeError(f"binnacle exited with status {process.returncode} during the run")
    finally:
        stop(process)

    samples = sampler.samples
    metrics_last = {m["id"]: m["value"] for m in last.get("metrics", [])}
    cpu_avg = None
    timed = [(s["at"], s["cpu_seconds"]) for s in samples if "cpu_seconds" in s]
    if len(timed) >= 2 and timed[-1][0] > timed[0][0]:
        cpu_avg = (timed[-1][1] - timed[0][1]) / (timed[-1][0] - timed[0][0]) * 100.0
    tx = [(s["at"], s["metrics"]["write_transactions"]) for s in samples if isinstance(s.get("metrics", {}).get("write_transactions"), (int, float))]
    write_tps = (tx[-1][1] - tx[0][1]) / (tx[-1][0] - tx[0][0]) if len(tx) >= 2 and tx[-1][0] > tx[0][0] else None
    write_p95, writes = histogram_quantile(latency(first, "persistence_write"), latency(last, "persistence_write"), 0.95)
    write_p50, _ = histogram_quantile(latency(first, "persistence_write"), latency(last, "persistence_write"), 0.50)
    # Rollups count in the measured window: the passes before it include the
    # full rebuild that demo seeding runs in the short scenario. Retention
    # counts from process start, since its working pass runs at boot.
    rollup_p95, rollups = histogram_quantile(latency(first, "rollup"), latency(last, "rollup"), 0.95)
    retention = latency(last, "retention") or {}
    retention_passes = int(retention.get("count") or 0)
    before_failures, after_failures = failure_counts(first), failure_counts(last)
    # Deliveries are reported but not counted: the demo channel points at a
    # .invalid host and the benchmark has no master key, so every alert the
    # failing demo check raises ends in a failed delivery by design.
    new_failures = {k: v - before_failures.get(k, 0) for k, v in after_failures.items() if k not in ("checks_skipped", "deliveries") and v - before_failures.get(k, 0) > 0}
    sse_rates = [s.bytes_per_second for s in streams]
    sse_errors = [s.error for s in streams if s.error]
    dropped = [float(s["metrics"]["dropped"]) for s in samples if isinstance(s.get("metrics", {}).get("dropped"), (int, float))]
    rss = [float(s["rss_bytes"]) for s in samples if "rss_bytes" in s]
    wal = [float(s["wal_bytes"]) for s in samples]

    result: dict[str, Any] = {
        "containers": containers,
        "checks": args.checks,
        "duration_seconds": args.duration,
        "sse_clients": args.sse_clients,
        "startup_seconds": round(measured_from - started - args.warmup, 1),
        "samples": len(samples),
        "sampler_errors": sampler.errors,
        "process": {"cpu_avg_percent": cpu_avg, "rss_bytes": agg(rss)},
        "write_latency_ms": {"p50": write_p50, "p95": write_p95, "batches": writes, "source": "persistence_write histogram, measured window"},
        "rollup_ms": {"p95": rollup_p95, "passes": rollups, "latest": metrics_last.get("rollup_duration")},
        # Bootstrap's SetRetention wakes a pass at boot, which deletes the
        # expired hour; the boot timer's pass a minute later finds nothing
        # left. The slowest pass since start is the one that did the work.
        "retention": {
            "passes": retention_passes,
            "max_ms": retention.get("max"),
            "latest_ms": metrics_last.get("retention_duration"),
            "vacuum_max_ms": (latency(last, "vacuum") or {}).get("max"),
            "incomplete": metrics_last.get("retention_incomplete"),
        },
        "deliveries_failed": after_failures.get("deliveries", 0) - before_failures.get("deliveries", 0),
        "wal_bytes": agg(wal),
        "database_bytes": file_size(db_path),
        "queue_depth_max": max((float(s["metrics"]["queue"]) for s in samples if isinstance(s.get("metrics", {}).get("queue"), (int, float))), default=None),
        "dropped_batches": (dropped[-1] - dropped[0]) if len(dropped) >= 2 else None,
        "worker_failures": new_failures,
        "checks_skipped": after_failures.get("checks_skipped", 0) - before_failures.get("checks_skipped", 0),
        "write_transactions_per_second": write_tps,
        "sse": {"bytes_per_second": sse_rates, "errors": sse_errors},
    }
    if args.scenario == "long-run":
        total, ran = checks_run_since(db_path, measured_from)
        result["checks_run"] = {"enabled": total, "ran_during_run": ran}
        result["raw_history_hours"] = args.raw_history_hours
        result["raw_history_after"] = raw_history(db_path)
    return result


def observed(run: dict[str, Any], scenario: str) -> dict[str, float | None]:
    """The values compared with the targets; None means not measured."""
    rss = run["process"]["rss_bytes"]["max"]
    wal = run["wal_bytes"]["max"]
    rates = run["sse"]["bytes_per_second"]
    retention = run["retention"]
    values: dict[str, float | None] = {
        "rss_max_mib": rss / MiB if rss is not None else None,
        "cpu_avg_percent": run["process"]["cpu_avg_percent"],
        "write_p95_ms": run["write_latency_ms"]["p95"],
        "rollup_p95_ms": run["rollup_ms"]["p95"],
        "wal_max_mib": wal / MiB if wal is not None else None,
        "sse_bytes_per_second_per_client": max(rates) if rates and None not in rates and not run["sse"]["errors"] else None,
        "dropped_batches": run["dropped_batches"],
        "worker_failures": float(sum(run["worker_failures"].values())),
        "write_transactions_per_second": run["write_transactions_per_second"],
        "retention_ms": None,
        "vacuum_ms": None,
    }
    if scenario == "long-run" and retention["passes"] >= 1 and not retention["incomplete"]:
        values["retention_ms"] = retention["max_ms"]
        values["vacuum_ms"] = retention["vacuum_max_ms"]
    return values


def evaluate(run: dict[str, Any], scenario: str) -> list[dict[str, Any]]:
    tier = next((t for t in sorted(TARGETS) if run["containers"] <= t), None)
    if tier is None:
        return [{"metric": "targets", "status": "not_applicable", "detail": f"no targets above {max(TARGETS)} containers"}]
    values = observed(run, scenario)
    results = []
    for metric, limit in TARGETS[tier]:
        value = values.get(metric)
        if scenario != "long-run" and metric in LONG_RUN_ONLY:
            status = "not_applicable"
        elif value is None:
            status = "fail"
        else:
            status = "pass" if value <= limit else "fail"
        results.append({"metric": metric, "value": value, "target": limit, "tier": tier, "status": status})
    if scenario == "long-run":
        # Scenario integrity: the run must have done what it claims.
        checks_run = run.get("checks_run", {})
        raw = run.get("raw_history_after", {}).get("host_samples_10s", {})
        hours = run["raw_history_hours"]
        integrity = [
            ("retention pass observed", run["retention"]["passes"] >= 1 and not run["retention"]["incomplete"]),
            ("checks ran during the run", checks_run.get("enabled", 0) > 0 and checks_run.get("ran_during_run", 0) == checks_run.get("enabled", 0)),
            # The seed spans raw_hours+1 to raw_hours+2 h; deleting the
            # expired hour leaves raw_hours to raw_hours+1 (one hour less if
            # an hour boundary passed between seeding and the boot pass).
            (f"retention deleted the expired hour ({hours} h of raw history kept)", hours - 1 <= (raw.get("span_hours") or 0) < hours + 1),
            ("all SSE clients streamed for the whole run", not run["sse"]["errors"] and len(run["sse"]["bytes_per_second"]) == run["sse_clients"]),
        ]
        for name, ok in integrity:
            results.append({"metric": name, "value": ok, "target": True, "tier": tier, "status": "pass" if ok else "fail"})
    return results


def markdown(report: dict[str, Any]) -> str:
    lines = [f"Scenario `{report['scenario']}`, binary `{report['binary']}`, host {report['host']}.", "", "| Containers | Check | Value | Target | Result |", "| --- | --- | --- | --- | --- |"]
    for run in report["runs"]:
        for r in run["targets"]:
            value = r.get("value")
            shown = f"{value:.2f}" if isinstance(value, float) else str(value)
            lines.append(f"| {run['containers']} | {r['metric']} | {shown} | {r.get('target', '')} | {r['status'].upper()} |")
    lines += ["", f"**{'PASS' if report['passed'] else 'FAIL'}**", ""]
    return "\n".join(lines)


def main() -> int:
    parser = argparse.ArgumentParser(description="Binnacle reproducible benchmark harness")
    parser.add_argument("--scenario", choices=["long-run", "short"], default="long-run")
    parser.add_argument("--binary", default="bin/binnacle", help="path to binnacle binary")
    parser.add_argument("--containers", type=int, action="append", help="synthetic containers; repeat for several runs (default: 30 and 100 for long-run, 30 for short)")
    parser.add_argument("--checks", type=int, default=10, help="number of synthetic health checks")
    parser.add_argument("--duration", type=int, help="measured seconds (default: 300 long-run, 60 short)")
    parser.add_argument("--warmup", type=int, default=5, help="seconds after startup before measuring")
    parser.add_argument("--sse-clients", type=int, help="concurrent live streams (default: 8 long-run, 1 short)")
    parser.add_argument("--raw-history-hours", type=int, default=48, help="long-run: hours of raw history kept after the retention pass")
    parser.add_argument("--sample-interval", type=float, default=2.0, help="seconds between samples")
    parser.add_argument("--output", help="write the JSON report to this file")
    parser.add_argument("--markdown", help="write the target table as Markdown to this file")
    parser.add_argument("--no-build", action="store_true", help="skip building the binary")
    parser.add_argument("--keep-data", action="store_true", help="keep each run's data directory")
    args = parser.parse_args()
    long_run = args.scenario == "long-run"
    containers = args.containers or ([30, 100] if long_run else [30])
    args.duration = args.duration or (300 if long_run else 60)
    args.sse_clients = args.sse_clients if args.sse_clients is not None else (8 if long_run else 1)
    if long_run and args.duration + args.warmup < 120:
        parser.error("long-run needs at least 120 s of warmup plus duration to include the retention pass one minute after boot")

    binary = Path(args.binary).resolve()
    if not args.no_build:
        subprocess.run(["go", "build", "-o", str(binary), "./cmd/binnacle"], check=True)  # noqa: S603, S607
    if not binary.exists():
        print(f"binary not found: {binary}", file=sys.stderr)
        return 1
    version = subprocess.run([str(binary), "--version"], capture_output=True, text=True, check=False).stdout.strip()  # noqa: S603

    runs = []
    for count in containers:
        data_dir = Path(tempfile.mkdtemp(prefix=f"binnacle-benchmark-{count}-"))
        log_path = data_dir / "binnacle.log"
        print(f"==> {args.scenario}: {count} containers ({data_dir})", file=sys.stderr)
        with open(log_path, "ab") as log:
            seed_seconds = None
            if long_run:
                seed_seconds = seed(binary, data_dir, count, args.checks, args.raw_history_hours, log)
                print(f"    seeded {args.raw_history_hours + 1} h of raw history in {seed_seconds:.0f}s", file=sys.stderr)
            run = measure(args, binary, data_dir, count, log)
        run["seed_seconds"] = seed_seconds
        run["targets"] = evaluate(run, args.scenario)
        run["passed"] = all(r["status"] != "fail" for r in run["targets"])
        if args.keep_data or not run["passed"]:
            run["data_dir"] = str(data_dir)
        else:
            shutil.rmtree(data_dir)
        for r in run["targets"]:
            if r["status"] == "fail":
                print(f"    FAIL {r['metric']}: {r.get('value')} (target {r.get('target')})", file=sys.stderr)
        runs.append(run)

    report = {
        "scenario": args.scenario,
        "binary": version or str(binary),
        "host": f"{os.uname().sysname} {os.uname().machine}, {os.cpu_count()} CPUs",
        "seed": 1,
        "raw_history_hours": args.raw_history_hours if long_run else None,
        "runs": runs,
        "passed": all(r["passed"] for r in runs),
    }
    text = json.dumps(report, indent=2)
    if args.output:
        Path(args.output).write_text(text + "\n", encoding="utf-8")
    if args.markdown:
        Path(args.markdown).write_text(markdown(report), encoding="utf-8")
    print(text)
    print(f"benchmark {'PASSED' if report['passed'] else 'FAILED'}", file=sys.stderr)
    return 0 if report["passed"] else 1


if __name__ == "__main__":
    sys.exit(main())
