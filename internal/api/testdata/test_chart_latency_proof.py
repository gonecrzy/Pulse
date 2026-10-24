import unittest

import chart_latency_proof as proof


class ChartLatencyProofTests(unittest.TestCase):
    def test_guest_time_is_not_counted_twice(self):
        before = dict(time=0, cpu=[0] * 10, swaps={'pswpin': 0, 'pswpout': 0})
        after = dict(time=1, cpu=[20, 0, 0, 80, 0, 0, 0, 0, 10, 0],
                     swaps=before['swaps'], load=[0.5, 0.5, 0.5], pressure={})
        row = proof.interval(before, after)
        self.assertEqual(row['total_ticks'], 100)
        self.assertEqual(row['busy_pct'], 20)
        self.assertEqual(row['guest_pct'], 10)

    def test_pre_measurement_cpu_idle_is_observational(self):
        row = dict(total_ticks=100, load=[1, 1, 1], idle_pct=95, iowait_pct=0,
                   steal_pct=0, guest_pct=0, swap_pages=0)
        self.assertTrue(proof.premeasurement_safe(row))
        for idle_pct in (0, 38, 89, 100):
            with self.subTest(idle_pct=idle_pct):
                self.assertTrue(proof.premeasurement_safe(dict(row, idle_pct=idle_pct)))
        for field, value in [('total_ticks', 0), ('load', [2, 1, 1]),
                             ('iowait_pct', 2), ('steal_pct', 1), ('guest_pct', 3), ('swap_pages', 1)]:
            with self.subTest(field=field):
                self.assertFalse(proof.premeasurement_safe(dict(row, **{field: value})))

    def test_skip_or_hosted_pass_cannot_satisfy_local_slo(self):
        report = 'charts/workloads p50=40ms p95=50ms p99=60ms SLO=90ms\n'
        passed = report + '--- PASS: TestSLO_WorkloadCharts (1.0s)\n'
        proof.validate_output(passed, 0)
        for text, status in [(report + '--- SKIP: TestSLO_WorkloadCharts (1.0s)\n', 0),
                             (passed.replace('90ms', '550ms'), 0), (passed, 1), ('PASS\n', 0),
                             (passed + report, 0), (passed + passed, 0),
                             (passed + '--- SKIP: TestSLO_WorkloadCharts (1.0s)\n', 0)]:
            with self.subTest(text=text, status=status):
                with self.assertRaises(RuntimeError):
                    proof.validate_output(text, status)

    def test_measurement_cpu_is_observed_but_guest_steal_and_swap_still_invalidate(self):
        row = dict(background_busy_pct=0, guest_pct=0, steal_pct=0, swap_pages=0,
                   iowait_pct=20)
        self.assertFalse(proof.competing_capacity(row))
        for background_busy_pct in (10, 27, 83, 100):
            with self.subTest(background_busy_pct=background_busy_pct):
                self.assertFalse(proof.competing_capacity(dict(row, background_busy_pct=background_busy_pct)))
        for field, value in [('guest_pct', 3), ('steal_pct', 1), ('swap_pages', 1)]:
            with self.subTest(field=field):
                self.assertTrue(proof.competing_capacity(dict(row, **{field: value})))

    def test_process_receipt_excludes_measurement_and_handles_new_or_reused_pids(self):
        before = {10: dict(name='test', ticks=20, start='1'),
                  20: dict(name='other', ticks=30, start='2'),
                  30: dict(name='old', ticks=100, start='3')}
        after = {10: dict(name='test', ticks=200, start='1'),
                 20: dict(name='other', ticks=55, start='2'),
                 30: dict(name='new', ticks=10, start='4'),
                 40: dict(name='new process', ticks=5, start='5')}
        self.assertEqual(proof.background_processes(before, after, 10, 100), [
            dict(pid=20, name='other', cpu_pct=25),
            dict(pid=30, name='new', cpu_pct=10),
            dict(pid=40, name='new process', cpu_pct=5)])

    def test_cpu_observation_summary_reports_values_without_classifying_them(self):
        summary = proof.summarize_cpu([
            dict(idle_pct=60, busy_pct=40, background_busy_pct=27,
                 guest_pct=0, steal_pct=0, swap_pages=0),
            dict(idle_pct=26, busy_pct=74, background_busy_pct=73,
                 guest_pct=0, steal_pct=0, swap_pages=0),
        ])
        self.assertEqual(summary, dict(
            sample_count=2, min_idle_pct=26, max_host_busy_pct=74,
            max_background_busy_pct=73, max_guest_pct=0,
            max_steal_pct=0, max_swap_pages=0))


if __name__ == '__main__':
    unittest.main()
