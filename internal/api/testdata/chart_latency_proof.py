"""Compare workload-chart p95 latency against the same-runner base build."""

import argparse
import hashlib
import json
import math
import os
from pathlib import Path
import random
import re
import statistics
import subprocess
import time


PAIRED_RUNS = 8
ABSOLUTE_TARGET_MS = 90.0
MAX_REGRESSION_PCT = 15.0
MAX_REGRESSION_RATIO = 1 + MAX_REGRESSION_PCT / 100
QUIET_CPU_PSI_AVG10_MAX = 10.0
CONTAMINATION_CPU_PSI_AVG10 = 20.0
CONTAMINATION_CPU_PSI_INTERVAL = 20.0
PREMEASUREMENT_SAFE_SECONDS = 30
BOOTSTRAP_SAMPLES = 20_000


def parse_cpu_psi(text):
    """Return the Linux CPU PSI ``some`` avg10 and cumulative total fields."""
    for line in text.splitlines():
        if not line.startswith('some '):
            continue
        fields = dict(field.split('=', 1) for field in line.split()[1:] if '=' in field)
        try:
            return dict(avg10=float(fields['avg10']), total_us=int(fields['total']))
        except (KeyError, ValueError):
            return None
    return None


def snapshot():
    cpu = list(map(int, Path('/proc/stat').read_text().splitlines()[0].split()[1:]))
    swaps = {}
    for line in Path('/proc/vmstat').read_text().splitlines():
        key, value = line.split()
        if key in ('pswpin', 'pswpout'):
            swaps[key] = int(value)
    pressure = {}
    for kind in ('cpu', 'memory', 'io'):
        path = Path('/proc/pressure') / kind
        if path.exists():
            pressure[kind] = path.read_text().strip()
    return dict(time=time.time(), cpu=cpu, swaps=swaps, load=os.getloadavg(),
                available_cpus=max(1, len(os.sched_getaffinity(0))), pressure=pressure)


def interval(before, after):
    delta = [b - a for a, b in zip(before['cpu'], after['cpu'])]
    total = sum(delta[:8])  # Guest time is already included in user/nice time.
    scale = 100 / total if total > 0 else 0
    elapsed = after['time'] - before['time']
    before_psi = parse_cpu_psi(before['pressure'].get('cpu', ''))
    after_psi = parse_cpu_psi(after['pressure'].get('cpu', ''))
    psi_interval = None
    if total > 0 and before_psi is not None and after_psi is not None and elapsed > 0:
        psi_interval = (after_psi['total_us'] - before_psi['total_us']) / (elapsed * 1_000_000) * 100
    return dict(
        time=after['time'], seconds=elapsed, total_ticks=total,
        idle_pct=delta[3] * scale, iowait_pct=delta[4] * scale,
        steal_pct=delta[7] * scale, guest_pct=sum(delta[8:10]) * scale,
        busy_pct=(total - delta[3] - delta[4] - delta[7]) * scale,
        swap_pages=sum(after['swaps'][key] - before['swaps'][key] for key in after['swaps']),
        load=after['load'], load_per_cpu=after['load'][0] / after['available_cpus'],
        cpu_psi_some_avg10_pct=after_psi['avg10'] if after_psi is not None else None,
        cpu_psi_some_interval_pct=psi_interval, pressure=after['pressure'])


def premeasurement_safe(row):
    """Require a stable, low-pressure window before starting the paired set."""
    psi_avg10 = row['cpu_psi_some_avg10_pct']
    psi_interval = row['cpu_psi_some_interval_pct']
    return (row['total_ticks'] > 0 and row['load_per_cpu'] < 0.5
            and row['iowait_pct'] <= 1 and row['steal_pct'] <= 0.1
            and row['swap_pages'] == 0 and psi_avg10 is not None
            and psi_avg10 <= QUIET_CPU_PSI_AVG10_MAX
            and psi_interval is not None and psi_interval <= QUIET_CPU_PSI_AVG10_MAX)


def wait_for_safe_window(prefix):
    deadline = time.monotonic() + 300
    streak = 0
    before = snapshot()
    rows = []
    with prefix.with_suffix('.idle.jsonl').open('w') as receipt:
        while streak < PREMEASUREMENT_SAFE_SECONDS:
            time.sleep(5)
            after = snapshot()
            row = interval(before, after)
            streak = streak + row['seconds'] if premeasurement_safe(row) else 0
            row['quiet_streak_seconds'] = streak
            rows.append(row)
            receipt.write(json.dumps(row) + '\n')
            receipt.flush()
            before = after
            if time.monotonic() > deadline:
                raise RuntimeError('No qualifying pre-measurement window; comparison is inconclusive.')
    return rows


def ticks(pid):
    try:
        fields = Path(f'/proc/{pid}/stat').read_text().rsplit(')', 1)[1].split()
        return int(fields[11]) + int(fields[12])
    except (FileNotFoundError, ProcessLookupError):
        return None


def process_counters(root=Path('/proc')):
    counters = {}
    for path in root.iterdir():
        if not path.name.isdigit():
            continue
        try:
            name, tail = (path / 'stat').read_text().rsplit(')', 1)
            fields = tail.split()
            counters[int(path.name)] = dict(name=name.split('(', 1)[1],
                                            ticks=int(fields[11]) + int(fields[12]),
                                            start=fields[19])
        except (OSError, ValueError, IndexError):
            continue
    return counters


def background_processes(before, after, measurement_pid, total_ticks):
    rows = []
    if total_ticks <= 0:
        return rows
    for pid, current in after.items():
        if pid == measurement_pid:
            continue
        previous = before.get(pid, {})
        previous_ticks = previous.get('ticks', 0) if previous.get('start') == current['start'] else 0
        delta = current['ticks'] - previous_ticks
        if delta > 0:
            rows.append(dict(pid=pid, name=current['name'], cpu_pct=100 * delta / total_ticks))
    return sorted(rows, key=lambda row: row['cpu_pct'], reverse=True)[:5]


def measurement_contamination(row):
    reasons = []
    psi_avg10 = row.get('cpu_psi_some_avg10_pct')
    psi_interval = row.get('cpu_psi_some_interval_pct')
    if psi_avg10 is None or psi_interval is None:
        reasons.append('CPU PSI unavailable during measurement')
    elif psi_avg10 > CONTAMINATION_CPU_PSI_AVG10 or psi_interval > CONTAMINATION_CPU_PSI_INTERVAL:
        reasons.append('CPU PSI exceeded the contamination limit')
    if row.get('steal_pct', 0) > 0.1:
        reasons.append('hypervisor CPU steal exceeded 0.1%')
    if row.get('swap_pages', 0) != 0:
        reasons.append('memory swapping occurred')
    return reasons


def summarize_cpu(rows):
    if not rows:
        return dict(sample_count=0, min_idle_pct=None, max_host_busy_pct=None,
                    max_background_busy_pct=None, max_cpu_psi_some_avg10_pct=None,
                    max_cpu_psi_some_interval_pct=None, max_guest_pct=None,
                    max_steal_pct=None, max_swap_pages=None,
                    peak_pressure_processes=[], peak_background_processes=[])
    pressure_row = max(rows, key=lambda row: row.get('cpu_psi_some_avg10_pct') or 0)
    process_row = max(rows, key=lambda row: row.get('background_busy_pct') or 0)
    return dict(
        sample_count=len(rows),
        min_idle_pct=min(row['idle_pct'] for row in rows),
        max_host_busy_pct=max(row['busy_pct'] for row in rows),
        max_background_busy_pct=max(row.get('background_busy_pct', 0) for row in rows),
        max_cpu_psi_some_avg10_pct=max(row.get('cpu_psi_some_avg10_pct') or 0 for row in rows),
        max_cpu_psi_some_interval_pct=max(row.get('cpu_psi_some_interval_pct') or 0 for row in rows),
        max_guest_pct=max(row['guest_pct'] for row in rows),
        max_steal_pct=max(row['steal_pct'] for row in rows),
        max_swap_pages=max(row['swap_pages'] for row in rows),
        peak_pressure_processes=[dict(name=row['name'], cpu_pct=row['cpu_pct'])
                                 for row in pressure_row.get('background_processes', [])[:3]],
        peak_background_processes=[dict(name=row['name'], cpu_pct=row['cpu_pct'])
                                   for row in process_row.get('background_processes', [])[:3]],
    )


def digest(path):
    with path.open('rb') as source:
        return hashlib.file_digest(source, 'sha256').hexdigest()


def duration_ms(value):
    match = re.fullmatch(r'(\d+(?:\.\d+)?)(ns|us|µs|ms|s|m|h)', value)
    if not match:
        raise ValueError(f'Unsupported Go duration: {value}')
    amount = float(match.group(1))
    unit = match.group(2)
    factor = {'ns': 1e-6, 'us': 1e-3, 'µs': 1e-3,
              'ms': 1, 's': 1000, 'm': 60_000, 'h': 3_600_000}[unit]
    return amount * factor


def validate_output(text, returncode):
    if returncode != 0:
        raise RuntimeError(f'SLO process exited {returncode}')
    passes = re.findall(r'^--- PASS: TestSLO_WorkloadCharts \(', text, re.MULTILINE)
    if len(passes) != 1 or re.search(r'^--- SKIP: TestSLO_WorkloadCharts ', text, re.MULTILINE):
        raise RuntimeError('Workload chart test did not execute and pass')
    reports = re.findall(
        r'charts/workloads p50=(\S+) p95=(\S+) p99=(\S+) SLO=(\S+)', text)
    if len(reports) != 1:
        raise RuntimeError(f'Expected one explicit workload chart latency report, got {reports}')
    p50, p95, p99, go_target = reports[0]
    return dict(p50_ms=duration_ms(p50), p95_ms=duration_ms(p95),
                p99_ms=duration_ms(p99), go_target=go_target)


def measure_one(binary, label, pair, output):
    identity = digest(binary)
    env = {key: os.environ[key] for key in (
        'PATH', 'HOME', 'TMPDIR', 'TZ', 'GOMAXPROCS', 'CI',
        'GITHUB_ACTIONS', 'GITHUB_RUN_ID', 'GITHUB_RUN_ATTEMPT') if key in os.environ}
    command = [str(binary), '-test.run=^TestSLO_WorkloadCharts$', '-test.v',
               '-test.count=1', '-test.cpu=2', '-test.timeout=5m']
    stem = output / f'pair-{pair:02d}-{label}'
    cpu_rows = []
    with stem.with_suffix('.log').open('w') as log, stem.with_suffix('.cpu.jsonl').open('w') as receipt:
        before = snapshot()
        processes_before = process_counters()
        process = subprocess.Popen(command, env=env, stdout=log, stderr=subprocess.STDOUT)
        own_before = ticks(process.pid)
        while process.poll() is None:
            time.sleep(1)
            after = snapshot()
            processes_after = process_counters()
            own_after = ticks(process.pid)
            row = interval(before, after)
            row['background_processes'] = background_processes(
                processes_before, processes_after, process.pid, row['total_ticks'])
            if own_before is not None and own_after is not None and row['total_ticks'] > 0:
                row['measurement_cpu_pct'] = 100 * (own_after - own_before) / row['total_ticks']
                row['background_busy_pct'] = max(0, row['busy_pct'] - row['measurement_cpu_pct'])
                row['contamination_reasons'] = measurement_contamination(row)
                cpu_rows.append(row)
            receipt.write(json.dumps(row) + '\n')
            receipt.flush()
            before, own_before = after, own_after
            processes_before = processes_after
        returncode = process.wait()

    text = stem.with_suffix('.log').read_text()
    latency = validate_output(text, returncode)
    cpu = summarize_cpu(cpu_rows)
    reasons = sorted({reason for row in cpu_rows for reason in row.get('contamination_reasons', [])})
    if not cpu_rows:
        reasons.append('No per-second CPU samples were captured')
    result = dict(pair=pair, label=label, binary_sha256=identity, command=command,
                  returncode=returncode, **latency, cpu=cpu,
                  contaminated=bool(reasons), contamination_reasons=reasons,
                  target_90ms_met=latency['p95_ms'] <= ABSOLUTE_TARGET_MS)
    stem.with_suffix('.result.json').write_text(json.dumps(result, indent=2) + '\n')
    if digest(binary) != identity:
        raise RuntimeError(f'{label} executable changed during measurement')
    print(json.dumps(dict(pair=pair, label=label, p95_ms=round(latency['p95_ms'], 3),
                          target_90ms_met=result['target_90ms_met'], cpu=cpu,
                          contaminated=result['contaminated'],
                          contamination_reasons=reasons)), flush=True)
    print(text, flush=True)
    return result


def bootstrap_median_ratio_ci(ratios, samples=BOOTSTRAP_SAMPLES, seed=20261025):
    if len(ratios) < PAIRED_RUNS:
        raise ValueError(f'Need at least {PAIRED_RUNS} paired measurements')
    logs = [math.log(ratio) for ratio in ratios]
    rng = random.Random(seed)
    bootstrapped = []
    for _ in range(samples):
        sample = [logs[rng.randrange(len(logs))] for _ in logs]
        bootstrapped.append(math.exp(statistics.median(sample)))
    bootstrapped.sort()
    lower = bootstrapped[int(0.025 * (samples - 1))]
    upper = bootstrapped[int(0.975 * (samples - 1))]
    return lower, upper


def evaluate_comparison(results):
    by_pair = {}
    for result in results:
        by_pair.setdefault(result['pair'], {})[result['label']] = result
    if len(by_pair) != PAIRED_RUNS or any(set(pair) != {'baseline', 'candidate'} for pair in by_pair.values()):
        raise ValueError(f'Expected {PAIRED_RUNS} complete baseline/candidate pairs')
    pairs = [by_pair[index] for index in sorted(by_pair)]
    ratios = [pair['candidate']['p95_ms'] / pair['baseline']['p95_ms'] for pair in pairs]
    point_ratio = math.exp(statistics.median(math.log(ratio) for ratio in ratios))
    lower, upper = bootstrap_median_ratio_ci(ratios)
    all_results = [result for pair in pairs for result in pair.values()]
    contaminated = [result for result in all_results if result['contaminated']]
    if contaminated:
        verdict = 'inconclusive'
        reason = 'CPU contention or host-capacity indicators contaminated one or more runs'
    elif lower > MAX_REGRESSION_RATIO:
        verdict = 'regression'
        reason = '95% paired-bootstrap lower bound exceeds the 15% regression allowance'
    elif upper <= MAX_REGRESSION_RATIO:
        verdict = 'pass'
        reason = '95% paired-bootstrap upper bound is within the 15% regression allowance'
    else:
        verdict = 'inconclusive'
        reason = '95% paired-bootstrap interval overlaps the 15% regression threshold'
    baseline_values = [pair['baseline']['p95_ms'] for pair in pairs]
    candidate_values = [pair['candidate']['p95_ms'] for pair in pairs]
    return dict(
        verdict=verdict, reason=reason, paired_runs=PAIRED_RUNS,
        baseline_p95_median_ms=statistics.median(baseline_values),
        candidate_p95_median_ms=statistics.median(candidate_values),
        baseline_p95_range_ms=[min(baseline_values), max(baseline_values)],
        candidate_p95_range_ms=[min(candidate_values), max(candidate_values)],
        relative_p95_difference_pct=(point_ratio - 1) * 100,
        relative_difference_95pct_ci_pct=[(lower - 1) * 100, (upper - 1) * 100],
        max_allowed_regression_pct=MAX_REGRESSION_PCT,
        baseline_meets_90ms_target=statistics.median(baseline_values) <= ABSOLUTE_TARGET_MS,
        candidate_meets_90ms_target=statistics.median(candidate_values) <= ABSOLUTE_TARGET_MS,
        max_cpu_psi_some_avg10_pct=max(
            result['cpu']['max_cpu_psi_some_avg10_pct'] or 0 for result in all_results),
        max_cpu_psi_some_interval_pct=max(
            result['cpu']['max_cpu_psi_some_interval_pct'] or 0 for result in all_results),
        max_background_busy_pct=max(
            result['cpu']['max_background_busy_pct'] or 0 for result in all_results),
        contaminated_runs=[dict(pair=result['pair'], label=result['label'],
                                reasons=result['contamination_reasons']) for result in contaminated],
    )


def paired_run_order(pair):
    return ('baseline', 'candidate') if pair % 2 else ('candidate', 'baseline')


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--baseline', type=Path, required=True)
    parser.add_argument('--candidate', type=Path, required=True)
    parser.add_argument('--output', type=Path, required=True)
    args = parser.parse_args()
    args.output.mkdir(parents=True, exist_ok=False)
    baseline = args.baseline.resolve()
    candidate = args.candidate.resolve()
    wait_for_safe_window(args.output / 'comparison')

    results = []
    for pair in range(1, PAIRED_RUNS + 1):
        for label in paired_run_order(pair):
            binary = baseline if label == 'baseline' else candidate
            results.append(measure_one(binary, label, pair, args.output))

    comparison = evaluate_comparison(results)
    (args.output / 'comparison.json').write_text(json.dumps(comparison, indent=2) + '\n')
    print('WORKLOAD CHART COMPARISON: ' + json.dumps(comparison), flush=True)
    if comparison['verdict'] != 'pass':
        raise RuntimeError(f"Workload chart comparison {comparison['verdict']}: {comparison['reason']}")


if __name__ == '__main__':
    main()
