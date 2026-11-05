#!/bin/bash
# SPDX-License-Identifier: AGPL-3.0-only
set -euo pipefail

# Validates Binnacle's collectors against the reference tools on this host.
#
#   scripts/validate-host.sh [--window SECONDS]
#       Starts bin/binnacle against /proc, /sys, / and the Docker socket on a
#       throwaway data directory and loopback port, claims setup, reads its
#       API over the window and prints Binnacle next to df, free,
#       /proc/net/dev, /proc/diskstats, top and docker stats. Exits 1 when a
#       row is outside its tolerance (CHECK) or Binnacle left it unknown.
#
#   scripts/validate-host.sh --record DIR [--window SECONDS]
#       Records the raw inputs instead (two /proc snapshots, the /sys/block
#       device list, statfs of every mount, df, and the Docker Engine API
#       list, inspect and two stats reads) plus the reference values computed
#       from them. The layout is the fixture format of
#       internal/collector/production/testdata/validation. Review the files
#       for host names and paths before committing them.
#
#   scripts/validate-host.sh --reference DIR
#       Recomputes DIR/reference.json and DIR/docker/reference.json from the
#       recorded inputs, for a fixture that was edited or written by hand.
#
# Environment: BINNACLE_BIN (default bin/binnacle), BINNACLE_DOCKER_SOCKET
# (default /var/run/docker.sock), VALIDATE_PORT (default: a free loopback port),
# VALIDATE_MAX_CONTAINERS (default 25), VALIDATE_KEEP=1 to keep the raw inputs
# of a comparison run. Needs bash, python3, curl and openssl;
# df, free, top and the docker CLI are used when present. Run as a user that
# can read the Docker socket. See docs/operations/metric-validation.md.

ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
BINNACLE_BIN="${BINNACLE_BIN:-$ROOT_DIR/bin/binnacle}"
SOCKET="${BINNACLE_DOCKER_SOCKET:-/var/run/docker.sock}"
PORT="${VALIDATE_PORT:-}"
MAX_CONTAINERS="${VALIDATE_MAX_CONTAINERS:-25}"
WINDOW=10
RECORD=""
REFERENCE=""
export LC_ALL=C

usage() { sed -n '5,31p' "$0" | sed 's/^# \{0,1\}//'; }
while [ $# -gt 0 ]; do
  case "$1" in
    --window) WINDOW="${2:?--window needs seconds}"; shift 2 ;;
    --record) RECORD="${2:?--record needs a directory}"; shift 2 ;;
    --reference) REFERENCE="${2:?--reference needs a directory}"; shift 2 ;;
    -h|--help) usage; exit 0 ;;
    *) echo "unknown argument: $1" >&2; usage >&2; exit 2 ;;
  esac
done
case "$WINDOW" in ''|*[!0-9]*) echo "--window must be whole seconds" >&2; exit 2 ;; esac
[ "$WINDOW" -ge 4 ] || { echo "--window must be at least 4 seconds (two collection ticks)" >&2; exit 2; }

for tool in python3 curl; do
  command -v "$tool" >/dev/null 2>&1 || { echo "$tool is required" >&2; exit 2; }
done

have_docker=0
if [ -S "$SOCKET" ] && curl -sf --max-time 5 --unix-socket "$SOCKET" -o /dev/null http://docker/_ping; then
  have_docker=1
fi

# docker_get <path> <out>: one bounded, read-only Engine API call.
docker_get() { curl -sf --max-time 10 --unix-socket "$SOCKET" -o "$2" "http://docker$1"; }

# snapshot_proc <dir>: the files the host collector reads, as of now.
snapshot_proc() {
  mkdir -p "$1/1/net"
  for name in stat meminfo loadavg uptime diskstats; do cat "/proc/$name" >"$1/$name"; done
  cat /proc/1/net/dev >"$1/1/net/dev"
  cat /proc/1/mountinfo >"$1/1/mountinfo"
}

# snapshot_docker_stats <dir> <ids-file>: one stats read per listed container.
snapshot_docker_stats() {
  mkdir -p "$1"
  while read -r id; do
    [ -n "$id" ] || continue
    docker_get "/containers/$id/stats?stream=false&one-shot=true" "$1/$id.json" || rm -f "$1/$id.json"
  done <"$2"
}

now() { python3 -c 'import time; print(f"{time.time():.6f}")'; }

# record_window <dir>: both snapshots around a window of WINDOW seconds, with
# the elapsed time between them. Runs <dir>/during (a function name) while the
# window is open when set.
record_window() {
  local dir="$1" during="${2:-}" t0 t1 d0 d1
  mkdir -p "$dir/proc" "$dir/docker"
  : >"$dir/sys-block.txt"
  for entry in /sys/block/*; do
    [ -e "$entry" ] || continue
    if [ -e "$entry/partition" ]; then echo "$(basename "$entry") partition"; else basename "$entry"; fi
  done >"$dir/sys-block.txt"
  if [ "$have_docker" = 1 ]; then
    docker_get "/containers/json?all=1" "$dir/docker/containers.json"
    python3 -I - "$dir/docker/containers.json" "$MAX_CONTAINERS" >"$dir/docker/running.txt" <<'PY'
import json, sys
items = json.load(open(sys.argv[1]))
running = [c["Id"] for c in items if c.get("State") == "running"]
print("\n".join(running[: int(sys.argv[2])]))
PY
    mkdir -p "$dir/docker/inspect"
    python3 -I -c 'import json,sys; [print(c["Id"]) for c in json.load(open(sys.argv[1]))]' "$dir/docker/containers.json" |
      head -n "$MAX_CONTAINERS" | while read -r id; do docker_get "/containers/$id/json" "$dir/docker/inspect/$id.json" || true; done
  fi

  t0="$(now)"; snapshot_proc "$dir/proc/t0"
  d0="$(now)"; [ "$have_docker" = 1 ] && snapshot_docker_stats "$dir/docker/stats-t0" "$dir/docker/running.txt"
  if [ -n "$during" ]; then "$during" "$dir"; else sleep "$WINDOW"; fi
  t1="$(now)"; snapshot_proc "$dir/proc/t1"
  d1="$(now)"; [ "$have_docker" = 1 ] && snapshot_docker_stats "$dir/docker/stats-t1" "$dir/docker/running.txt"
  python3 -c "print(f'{$t1 - $t0:.6f}')" >"$dir/elapsed"
  python3 -c "print(f'{$d1 - $d0:.6f}')" >"$dir/docker/elapsed"
  echo "$t0 $t1" >"$dir/window"

  # statfs right before df, so the fixture's two views of each mount agree.
  if [ -n "$REFERENCE" ]; then
  python3 -I "$HELPER" reference "$REFERENCE"
  echo "Wrote $REFERENCE/reference.json and, with Docker stats present, $REFERENCE/docker/reference.json."
  exit 0
fi

if [ -n "$RECORD" ]; then timeout 30 python3 -I "$HELPER" statfs "$dir"; fi
  if command -v df >/dev/null 2>&1; then
    timeout 30 df -B1 --output=size,used,avail,itotal,iused,fstype,target >"$dir/df.txt" 2>/dev/null || true
  fi
  if command -v free >/dev/null 2>&1; then free -b >"$dir/free.txt"; fi
}

HELPER="$(mktemp /tmp/bnv-helper.XXXXXX.py)"
trap 'rm -f "$HELPER"' EXIT
cat >"$HELPER" <<'PY'
"""Reference values and the side-by-side report for scripts/validate-host.sh.

Every reference value is computed here, independently of Binnacle's Go code,
from the documented semantics of the reference tool: top's busy share, free's
"used", the byte counters of /proc/net/dev and /proc/diskstats, df's columns
and the Docker CLI's docker stats formulas.
"""
import json
import os
import sys
from datetime import datetime

SKIP_IFACE_PREFIXES = ("veth", "docker", "br-")
STACKED_PREFIXES = ("dm-", "md", "zram", "loop", "ram")


def read(path):
    with open(path) as f:
        return f.read()


def cpu_line(text):
    for line in text.splitlines():
        f = line.split()
        if f and f[0] == "cpu":
            return [int(v) for v in f[1:]]
    raise ValueError("no aggregate cpu line")


def meminfo(text):
    out = {}
    for line in text.splitlines():
        key, _, rest = line.partition(":")
        if rest.strip():
            out[key] = int(rest.split()[0]) * 1024
    return out


def net_dev(text, filtered):
    rx = tx = 0
    for line in text.splitlines():
        name, sep, rest = line.partition(":")
        if not sep or not rest.split() or not rest.split()[0].isdigit():
            continue
        name = name.strip()
        if filtered and (name == "lo" or name.startswith(SKIP_IFACE_PREFIXES)):
            continue
        f = [int(v) for v in rest.split()]
        rx, tx = rx + f[0], tx + f[8]
    return rx, tx


def whole_devices(sys_block):
    out = set()
    for line in sys_block.splitlines():
        f = line.split()
        if f and len(f) == 1 and not f[0].startswith(STACKED_PREFIXES):
            out.add(f[0])
    return out


def diskstats(text, devices):
    reads = rsec = writes = wsec = 0
    for line in text.splitlines():
        f = line.split()
        if len(f) >= 10 and f[2] in devices:
            reads, rsec, writes, wsec = reads + int(f[3]), rsec + int(f[5]), writes + int(f[7]), wsec + int(f[9])
    return reads, rsec * 512, writes, wsec * 512


def host_reference(d):
    """Reference host values over the recorded window of directory d."""
    p0, p1 = os.path.join(d, "proc", "t0"), os.path.join(d, "proc", "t1")
    elapsed = float(read(os.path.join(d, "elapsed")))
    c0, c1 = cpu_line(read(os.path.join(p0, "stat"))), cpu_line(read(os.path.join(p1, "stat")))
    # top: the first eight fields (user nice system idle iowait irq softirq
    # steal) make the total; busy is everything but idle and iowait.
    t0, t1 = sum(c0[:8]), sum(c1[:8])
    idle = (c1[3] - c0[3]) + (c1[4] - c0[4])
    ref = {"elapsed": elapsed, "cpu_busy_pct": (t1 - t0 - idle) * 100.0 / (t1 - t0) if t1 > t0 else None}
    mem = meminfo(read(os.path.join(p1, "meminfo")))
    ref["mem_total_bytes"] = mem["MemTotal"]
    ref["mem_used_bytes"] = mem["MemTotal"] - mem["MemAvailable"]
    ref["load1"] = float(read(os.path.join(p1, "loadavg")).split()[0])
    for filtered, suffix in ((True, ""), (False, "_all")):
        rx0, tx0 = net_dev(read(os.path.join(p0, "1", "net", "dev")), filtered)
        rx1, tx1 = net_dev(read(os.path.join(p1, "1", "net", "dev")), filtered)
        ref["net_rx_bps" + suffix] = (rx1 - rx0) / elapsed
        ref["net_tx_bps" + suffix] = (tx1 - tx0) / elapsed
    devices = whole_devices(read(os.path.join(d, "sys-block.txt")))
    r0 = diskstats(read(os.path.join(p0, "diskstats")), devices)
    r1 = diskstats(read(os.path.join(p1, "diskstats")), devices)
    ref["disk_read_iops"] = (r1[0] - r0[0]) / elapsed
    ref["disk_read_bps"] = (r1[1] - r0[1]) / elapsed
    ref["disk_write_iops"] = (r1[2] - r0[2]) / elapsed
    ref["disk_write_bps"] = (r1[3] - r0[3]) / elapsed
    return ref


def df_rows(d):
    """df -B1 --output=size,used,avail,itotal,iused,fstype,target, by target."""
    path = os.path.join(d, "df.txt")
    if not os.path.exists(path):
        return {}
    out = {}
    for line in read(path).splitlines()[1:]:
        f = line.split(None, 6)
        if len(f) == 7 and f[0].isdigit():
            out[f[6]] = {"size": int(f[0]), "used": int(f[1]), "avail": int(f[2]),
                         "itotal": int(f[3]) if f[3].isdigit() else None,
                         "iused": int(f[4]) if f[4].isdigit() else None, "fstype": f[5]}
    return out


def docker_time(value):
    if not value or value.startswith("0001-"):
        return None
    head, _, frac = value.rstrip("Z").partition(".")
    return datetime.fromisoformat(head + "+00:00").timestamp() + (float("0." + frac[:9]) if frac else 0.0)


def mem_usage_no_cache(mem):
    """The Docker CLI's MEM USAGE: usage minus inactive file cache, taken from
    total_inactive_file on cgroup v1 and inactive_file on cgroup v2."""
    usage = mem.get("usage", 0)
    stats = mem.get("stats") or {}
    if "total_inactive_file" in stats and stats["total_inactive_file"] < usage:
        return usage - stats["total_inactive_file"]
    v = stats.get("inactive_file", 0)
    return usage - v if v < usage else usage


def net_totals(stats):
    networks = stats.get("networks")
    if not networks:
        return None
    return sum(n.get("rx_bytes", 0) for n in networks.values()), sum(n.get("tx_bytes", 0) for n in networks.values())


def blkio_totals(stats):
    read_b = write_b = 0
    for entry in (stats.get("blkio_stats") or {}).get("io_service_bytes_recursive") or []:
        op = entry.get("op", "").lower()
        if op == "read":
            read_b += entry.get("value", 0)
        elif op == "write":
            write_b += entry.get("value", 0)
    return read_b, write_b


def docker_reference(d, use_read_times):
    """docker stats values per running container over the recorded window."""
    base = os.path.join(d, "docker")
    s0, s1 = os.path.join(base, "stats-t0"), os.path.join(base, "stats-t1")
    if not os.path.isdir(s1):
        return {}
    fallback = float(read(os.path.join(base, "elapsed")))
    out = {}
    for name in sorted(os.listdir(s1)):
        if not name.endswith(".json") or not os.path.exists(os.path.join(s0, name)):
            continue
        a, b = json.load(open(os.path.join(s0, name))), json.load(open(os.path.join(s1, name)))
        elapsed = fallback
        if use_read_times:
            ra, rb = docker_time(a.get("read")), docker_time(b.get("read"))
            if ra and rb and rb > ra:
                elapsed = rb - ra
        ca, cb = a["cpu_stats"], b["cpu_stats"]
        online = cb.get("online_cpus") or len((cb.get("cpu_usage") or {}).get("percpu_usage") or []) or None
        cpu_delta = cb["cpu_usage"]["total_usage"] - ca["cpu_usage"]["total_usage"]
        sys_delta = cb.get("system_cpu_usage", 0) - ca.get("system_cpu_usage", 0)
        cpu = cpu_delta / sys_delta * online * 100.0 if sys_delta > 0 and cpu_delta >= 0 and online else None
        n0, n1 = net_totals(a), net_totals(b)
        k0, k1 = blkio_totals(a), blkio_totals(b)
        out[name[:-5]] = {
            "cpu_percent": cpu,
            "online_cpus": online,
            "mem_usage_bytes": mem_usage_no_cache(b.get("memory_stats") or {}),
            "rx_bps": (n1[0] - n0[0]) / elapsed if n0 and n1 else None,
            "tx_bps": (n1[1] - n0[1]) / elapsed if n0 and n1 else None,
            "block_read_bps": (k1[0] - k0[0]) / elapsed,
            "block_write_bps": (k1[1] - k0[1]) / elapsed,
        }
    return out


def write_reference(d):
    with open(os.path.join(d, "reference.json"), "w") as f:
        json.dump(host_reference(d), f, indent=2, sort_keys=True)
        f.write("\n")
    containers = docker_reference(d, use_read_times=False)
    if containers:
        with open(os.path.join(d, "docker", "reference.json"), "w") as f:
            json.dump(containers, f, indent=2, sort_keys=True)
            f.write("\n")


def write_statfs(d):
    """statfs of every mount in the host mount table, for the fixture."""
    rows = []
    for line in read("/proc/1/mountinfo").splitlines():
        f = line.split()
        target = f[4].encode().decode("unicode_escape") if "\\" in f[4] else f[4]
        try:
            s = os.statvfs(target)
        except OSError:
            continue
        rows.append("\t".join(str(v) for v in (target, s.f_bsize, s.f_frsize, s.f_blocks, s.f_bfree, s.f_bavail, s.f_files, s.f_ffree)))
    with open(os.path.join(d, "statfs.tsv"), "w") as f:
        f.write("# target\tbsize\tfrsize\tblocks\tbfree\tbavail\tfiles\tffree\n")
        f.write("\n".join(rows) + "\n")


# --- report -----------------------------------------------------------------

def human_bytes(v):
    if v is None:
        return "unknown"
    for unit in ("B", "KiB", "MiB", "GiB", "TiB"):
        if abs(v) < 1024 or unit == "TiB":
            return f"{v:.1f} {unit}" if unit != "B" else f"{v:.0f} B"
        v /= 1024.0


def fmt(kind, v):
    if v is None:
        return "unknown"
    if kind == "pct":
        return f"{v:.1f} %"
    if kind == "bytes":
        return human_bytes(v)
    if kind == "bps":
        return human_bytes(v) + "/s"
    if kind == "iops":
        return f"{v:.1f} /s"
    return f"{v:.2f}"


ROWS = []


def row(metric, kind, ours, ref, source, abs_tol, rel_tol=0.0, info=False):
    if info:
        verdict = "info"
    elif ours is None and ref is None:
        verdict = "ok"
    elif ours is None or ref is None:
        verdict = "CHECK"
    else:
        verdict = "ok" if abs(ours - ref) <= max(abs_tol, abs(ref) * rel_tol) else "CHECK"
    diff = ""
    if ours is not None and ref is not None:
        if kind == "pct":
            diff = f"{ours - ref:+.1f} pp"
        elif kind == "num":
            diff = f"{ours - ref:+.2f}"
        else:
            diff = fmt(kind, ours - ref)
            diff = diff if diff.startswith("-") else "+" + diff
    ROWS.append((metric, "-" if info and ours is None else fmt(kind, ours), fmt(kind, ref), diff, verdict, source))


def mean(values):
    values = [v for v in values if v is not None]
    return sum(values) / len(values) if values else None


def parse_rfc3339(value):
    head, _, rest = value.partition(".")
    if rest:
        frac = "".join(ch for ch in rest if ch.isdigit())
        tz = rest[len(frac):]
    else:
        head, frac, tz = value[:19], "", value[19:]
    tz = "+00:00" if tz in ("Z", "") else tz
    return datetime.fromisoformat(head + tz).timestamp() + (float("0." + frac) if frac else 0.0)


def report(d, interval):
    ref = host_reference(d)
    w0, w1 = (float(v) for v in read(os.path.join(d, "window")).split())
    api = os.path.join(d, "api")
    ticks = {}
    for line in read(os.path.join(api, "server.jsonl")).splitlines():
        host = json.loads(line)["host"]
        ticks[host["at"]] = host
    # A tick's rates cover the interval before it, so only ticks whose whole
    # interval lies inside the reference window are compared.
    inside = [h for at, h in sorted(ticks.items()) if w0 + interval - 0.5 <= parse_rfc3339(at) <= w1 + 0.5]
    last = inside[-1] if inside else (list(ticks.values())[-1] if ticks else {})
    print(f"Binnacle collector validation: window {ref['elapsed']:.1f} s, {len(inside)} host ticks compared")
    print()

    top_busy = None
    top_path = os.path.join(d, "top.txt")
    if os.path.exists(top_path):
        lines = [l for l in read(top_path).splitlines() if l.startswith("%Cpu(s):")]
        if lines:
            fields = {}
            for part in lines[-1].split(":", 1)[1].split(","):
                value, _, key = part.strip().partition(" ")
                try:
                    fields[key.strip()] = float(value)
                except ValueError:
                    pass
            if "id" in fields:
                top_busy = 100.0 - fields["id"] - fields.get("wa", 0.0)

    cpu = mean(h.get("cpuPct") for h in inside)
    row("cpu busy", "pct", cpu, ref["cpu_busy_pct"], "/proc/stat delta (100 - idle - iowait)", 5)
    if top_busy is not None:
        row("cpu busy", "pct", cpu, top_busy, "top: 100 - id - wa", 5)
    row("memory total", "bytes", last.get("memoryTotalBytes"), ref["mem_total_bytes"], "/proc/meminfo MemTotal", 0)
    row("memory used", "bytes", last.get("memoryUsedBytes"), ref["mem_used_bytes"], "/proc/meminfo MemTotal - MemAvailable", 64 << 20, 0.02)
    free_path = os.path.join(d, "free.txt")
    if os.path.exists(free_path):
        for line in read(free_path).splitlines():
            f = line.split()
            if f and f[0] == "Mem:":
                row("memory used", "bytes", last.get("memoryUsedBytes"), float(f[2]), "free -b used", 64 << 20, 0.02)
    row("load 1m", "num", last.get("load1"), ref["load1"], "/proc/loadavg", 0.3, 0.1)
    for direction in ("rx", "tx"):
        ours = mean(h.get(f"network{direction.capitalize()}Bps") for h in inside)
        row(f"network {direction}", "bps", ours, ref[f"net_{direction}_bps"], "/proc/net/dev, without lo/veth/docker/br-", 128 << 10, 0.15)
        row(f"network {direction} (all)", "bps", None, ref[f"net_{direction}_bps_all"], "/proc/net/dev, every interface", 0, info=True)
    for direction, key in (("read", "Read"), ("write", "Write")):
        row(f"disk {direction}", "bps", mean(h.get(f"disk{key}Bps") for h in inside), ref[f"disk_{direction}_bps"], "/proc/diskstats, whole devices in /sys/block", 128 << 10, 0.15)
        row(f"disk {direction} IOPS", "iops", mean(h.get(f"disk{key}Iops") for h in inside), ref[f"disk_{direction}_iops"], "/proc/diskstats, whole devices in /sys/block", 10, 0.15)

    df = df_rows(d)
    filesystems = json.load(open(os.path.join(api, "filesystems.json")))
    for fs in filesystems:
        target = fs["mountPoint"]
        row_ref = df.get(target)
        for label, ours, key in (("size", fs.get("totalBytes"), "size"), ("used", fs.get("usedBytes"), "used"), ("avail", fs.get("availableBytes"), "avail")):
            row(f"fs {target} {label}", "bytes", ours, row_ref[key] if row_ref else None, "df -B1", 256 << 20, 0.01)

    containers = docker_reference(d, use_read_times=True)
    cli = {}
    cli_path = os.path.join(d, "docker", "docker-stats.jsonl")
    if os.path.exists(cli_path):
        for line in read(cli_path).splitlines():
            if line.strip():
                v = json.loads(line)
                cli[v.get("ID", "")[:12]] = v
    samples = {}
    for line in read(os.path.join(api, "resources.jsonl")).splitlines():
        for resource in json.loads(line):
            for c in resource.get("components") or []:
                samples.setdefault(c["id"], {})[resource.get("lastSeenAt")] = c
    for cid, want in containers.items():
        seen = samples.get(cid, {})
        comps = [c for at, c in sorted(seen.items()) if at and w0 + interval - 0.5 <= parse_rfc3339(at) <= w1 + 0.5]
        name = (comps or list(seen.values()) or [{"name": cid[:12]}])[-1].get("name") or cid[:12]
        online = want["online_cpus"]
        ref_host_pct = want["cpu_percent"] / online if want["cpu_percent"] is not None and online else None
        row(f"{name} cpu (host %)", "pct", mean(c.get("cpuHostPct") for c in comps), ref_host_pct, f"stats API delta: CPU % / {online} CPUs", 5)
        if cid[:12] in cli and online:
            text = cli[cid[:12]].get("CPUPerc", "").rstrip("%")
            try:
                row(f"{name} cpu (host %)", "pct", mean(c.get("cpuHostPct") for c in comps), float(text) / online, "docker stats CPU % / CPUs (short sample)", 0, info=True)
            except ValueError:
                pass
        row(f"{name} memory", "bytes", comps[-1].get("memoryBytes") if comps else None, want["mem_usage_bytes"], "docker stats MEM USAGE (usage - inactive_file)", 16 << 20, 0.05)
        for key, label in (("rx", "rxBps"), ("tx", "txBps")):
            row(f"{name} net {key}", "bps", mean(c.get(label) for c in comps), want[f"{key}_bps"], "stats API networks delta (docker stats NET I/O)", 64 << 10, 0.2)
        for key, label in (("read", "blockReadBps"), ("write", "blockWriteBps")):
            row(f"{name} block {key}", "bps", mean(c.get(label) for c in comps), want[f"block_{key}_bps"], "stats API blkio delta (docker stats BLOCK I/O)", 64 << 10, 0.2)

    health = json.load(open(os.path.join(api, "collector-health.json")))
    widths = [max(len(r[i]) for r in ROWS + [("metric", "binnacle", "reference", "diff", "verdict", "")]) for i in range(5)]
    header = ("metric", "binnacle", "reference", "diff", "verdict", "reference source")
    print("  ".join(h.ljust(widths[i]) if i < 5 else h for i, h in enumerate(header)))
    for r in ROWS:
        print("  ".join(v.ljust(widths[i]) if i < 5 else v for i, v in enumerate(r)))
    print()
    reported = {fs["mountPoint"] for fs in filesystems}
    missing = [f"{t} ({r['fstype']})" for t, r in sorted(df.items()) if t not in reported]
    if missing:
        print("df mounts Binnacle does not report (pseudo filesystems, or the same device as a shorter mount):")
        print("  " + ", ".join(missing))
        print()
    for name, h in sorted(health.items()):
        print(f"collector {name}: {h.get('state')}" + (f" ({h['reason']})" if h.get("reason") else ""))
    failed = [r for r in ROWS if r[4] == "CHECK"]
    print()
    if failed:
        print(f"{len(failed)} row(s) outside tolerance. Rates compare a mean of {interval:.0f} s ticks with one window delta; rerun under steady load before treating a rate row as a defect.")
        return 1
    print("All rows within tolerance.")
    return 0


if __name__ == "__main__":
    command, directory = sys.argv[1], sys.argv[2]
    if command == "statfs":
        write_statfs(directory)
    elif command == "reference":
        write_reference(directory)
    elif command == "report":
        sys.exit(report(directory, float(sys.argv[3])))
PY

if [ -n "$REFERENCE" ]; then
  python3 -I "$HELPER" reference "$REFERENCE"
  echo "Wrote $REFERENCE/reference.json and, with Docker stats present, $REFERENCE/docker/reference.json."
  exit 0
fi

if [ -n "$RECORD" ]; then
  mkdir -p "$RECORD"
  [ "$have_docker" = 1 ] || echo "Docker socket $SOCKET not reachable; recording host inputs only." >&2
  record_window "$RECORD"
  python3 -I "$HELPER" reference "$RECORD"
  echo "Recorded fixture in $RECORD. Review it for host names and paths before committing."
  exit 0
fi

[ -x "$BINNACLE_BIN" ] || { echo "$BINNACLE_BIN not found; run 'make build' or set BINNACLE_BIN" >&2; exit 2; }
command -v openssl >/dev/null 2>&1 || { echo "openssl is required" >&2; exit 2; }
if [ "$have_docker" != 1 ]; then
  echo "Docker socket $SOCKET is not reachable. Binnacle needs Docker Engine 29.5.1 or newer;" >&2
  echo "run as a member of the docker group or set BINNACLE_DOCKER_SOCKET." >&2
  exit 2
fi

WORK="$(mktemp -d /tmp/bnv.XXXXXX)"
BINNACLE_PID=""
cleanup() {
  if [ -n "$BINNACLE_PID" ]; then kill "$BINNACLE_PID" 2>/dev/null || true; wait "$BINNACLE_PID" 2>/dev/null || true; fi
  rm -f "$HELPER"
  if [ -n "${VALIDATE_KEEP:-}" ]; then echo "Kept $WORK"; else rm -rf "$WORK"; fi
}
trap cleanup EXIT

if [ -z "$PORT" ]; then
  PORT="$(python3 -I -c 'import socket; s=socket.socket(); s.bind(("127.0.0.1", 0)); print(s.getsockname()[1])')"
fi
BASE="http://127.0.0.1:$PORT"
TOKEN="$(openssl rand -hex 32)"
INTERVAL=2
BINNACLE_SETUP_TOKEN="$TOKEN" BINNACLE_LISTEN_ADDRESS="127.0.0.1:$PORT" BINNACLE_DATA_DIR="$WORK/data" \
  BINNACLE_HOST_PROC=/proc BINNACLE_HOST_SYS=/sys BINNACLE_HOST_ROOT=/ BINNACLE_DOCKER_SOCKET="$SOCKET" \
  BINNACLE_HOST_INTERVAL="${INTERVAL}s" BINNACLE_CONTAINER_INTERVAL="${INTERVAL}s" \
  "$BINNACLE_BIN" >"$WORK/binnacle.log" 2>&1 &
BINNACLE_PID=$!

for _ in $(seq 1 60); do
  curl -sf -o /dev/null "$BASE/healthz" && break
  kill -0 "$BINNACLE_PID" 2>/dev/null || break
  sleep 0.5
done
# The answer must come from the process started here, not another listener.
if ! kill -0 "$BINNACLE_PID" 2>/dev/null || ! curl -sf -o /dev/null "$BASE/healthz"; then
  tail -n 20 "$WORK/binnacle.log" >&2
  echo "Binnacle did not start on $BASE" >&2
  exit 1
fi

JAR="$WORK/cookies"
status="$(curl -s -o "$WORK/claim.out" -w '%{http_code}' -c "$JAR" -H 'Content-Type: application/json' -H "Origin: $BASE" \
  -d "{\"token\":\"$TOKEN\",\"username\":\"validate\",\"password\":\"$(openssl rand -hex 16)\"}" "$BASE/api/v1/setup/claim")"
[ "$status" = 204 ] || { cat "$WORK/claim.out" >&2; echo "setup claim answered $status" >&2; exit 1; }
api() { curl -sf --max-time 10 -b "$JAR" "$BASE/api/v1/$1"; }

# Two host ticks give the first rates; a third makes sure Docker has a baseline.
for _ in $(seq 1 30); do
  api server | python3 -I -c 'import json,sys; sys.exit(0 if json.load(sys.stdin)["host"].get("cpuPct") is not None else 1)' && break
  sleep 1
done
sleep "$INTERVAL"

# during <dir>: poll the API once a second and run top over the window.
during() {
  local dir="$1"
  mkdir -p "$dir/api"
  : >"$dir/api/server.jsonl"; : >"$dir/api/resources.jsonl"
  local top_pid=""
  if command -v top >/dev/null 2>&1; then
    top -b -n 2 -d "$WINDOW" >"$dir/top.txt" 2>/dev/null &
    top_pid=$!
  fi
  for _ in $(seq 1 "$((WINDOW + 1))"); do
    if api server >>"$dir/api/server.jsonl"; then echo >>"$dir/api/server.jsonl"; fi
    if api resources >>"$dir/api/resources.jsonl"; then echo >>"$dir/api/resources.jsonl"; fi
    sleep 1
  done
  if [ -n "$top_pid" ]; then wait "$top_pid" 2>/dev/null || true; fi
}

echo "Sampling for ${WINDOW} s..." >&2
record_window "$WORK" during
sed -i '/^$/d' "$WORK/api/server.jsonl" "$WORK/api/resources.jsonl"
api filesystems >"$WORK/api/filesystems.json"
api collector-health >"$WORK/api/collector-health.json"
if command -v docker >/dev/null 2>&1; then
  DOCKER_HOST="unix://$SOCKET" timeout 30 docker stats --no-stream --no-trunc --format '{{json .}}' >"$WORK/docker/docker-stats.jsonl" 2>/dev/null || true
fi

python3 -I "$HELPER" report "$WORK" "$INTERVAL"
