import unittest

import chart_latency_proof as proof


def result(pair, label, p95_ms, contaminated=False):
    return dict(pair=pair, label=label, p95_ms=p95_ms, contaminated=contaminated,
                contamination_reasons=['test contamination'] if contaminated else [],
                cpu=dict(max_cpu_psi_some_avg10_pct=0, max_cpu_psi_some_interval_pct=0,
                         max_background_busy_pct=0))


class ChartLatencyProofTests(unittest.TestCase):
    def test_cpu_psi_parser_requires_some_avg10_and_total(self):
        self.assertEqual(proof.parse_cpu_psi(
            'some avg10=12.34 avg60=1.00 avg300=0.10 total=2000\n'
            'full avg10=0.00 avg60=0.00 avg300=0.00 total=0'),
            dict(avg10=12.34, total_us=2000))
        self.assertIsNone(proof.parse_cpu_psi('full avg10=0 avg60=0 avg300=0 total=0'))
        self.assertIsNone(proof.parse_cpu_psi('some avg10=unknown total=0'))

    def test_interval_excludes_guest_time_from_busy_and_records_cpu_pressure(self):
        before = dict(time=0, cpu=[0] * 10, swaps={'pswpin': 0, 'pswpout': 0},
                      load=[0.5, 0.5, 0.5], available_cpus=4,
                      pressure={'cpu': 'some avg10=0 avg60=0 avg300=0 total=1000'})
        after = dict(time=1, cpu=[20, 0, 0, 80, 0, 0, 0, 0, 10, 0],
                     swaps=before['swaps'], load=before['load'], available_cpus=4,
                     pressure={'cpu': 'some avg10=12.5 avg60=0 avg300=0 total=11000'})
        row = proof.interval(before, after)
        self.assertEqual(row['total_ticks'], 100)
        self.assertEqual(row['busy_pct'], 20)
        self.assertEqual(row['guest_pct'], 10)
        self.assertEqual(row['cpu_psi_some_avg10_pct'], 12.5)
        self.assertEqual(row['cpu_psi_some_interval_pct'], 1.0)

    def test_pre_measurement_window_requires_low_normalized_load_and_cpu_pressure(self):
        row = dict(total_ticks=100, load_per_cpu=0.4, iowait_pct=0, steal_pct=0,
                   swap_pages=0, cpu_psi_some_avg10_pct=1, cpu_psi_some_interval_pct=0.5)
        self.assertTrue(proof.premeasurement_safe(row))
        for field, value in [('total_ticks', 0), ('load_per_cpu', 0.5), ('iowait_pct', 2),
                             ('steal_pct', 1), ('swap_pages', 1),
                             ('cpu_psi_some_avg10_pct', 10.1),
                             ('cpu_psi_some_interval_pct', 10.1),
                             ('cpu_psi_some_avg10_pct', None)]:
            with self.subTest(field=field, value=value):
                self.assertFalse(proof.premeasurement_safe(dict(row, **{field: value})))

    def test_measurement_contamination_rejects_pressure_steal_swap_and_missing_psi(self):
        row = dict(cpu_psi_some_avg10_pct=1, cpu_psi_some_interval_pct=1,
                   steal_pct=0, swap_pages=0, guest_pct=30)
        self.assertEqual(proof.measurement_contamination(row), [])
        for field, value in [('cpu_psi_some_avg10_pct', 20.1),
                             ('cpu_psi_some_interval_pct', 20.1),
                             ('steal_pct', 0.2), ('swap_pages', 1),
                             ('cpu_psi_some_avg10_pct', None)]:
            with self.subTest(field=field, value=value):
                self.assertTrue(proof.measurement_contamination(dict(row, **{field: value})))

    def test_validate_output_requires_an_executed_test_and_parses_p95(self):
        report = 'charts/workloads p50=40ms p95=55000us p99=60ms SLO=550ms\n'
        passed = report + '--- PASS: TestSLO_WorkloadCharts (1.0s)\n'
        self.assertEqual(proof.validate_output(passed, 0)['p95_ms'], 55)
        for text, status in [(report + '--- SKIP: TestSLO_WorkloadCharts (1.0s)\n', 0),
                             (passed, 1), ('PASS\n', 0), (passed + report, 0),
                             (passed + passed, 0),
                             (passed + '--- SKIP: TestSLO_WorkloadCharts (1.0s)\n', 0)]:
            with self.subTest(text=text, status=status):
                with self.assertRaises(RuntimeError):
                    proof.validate_output(text, status)

    def test_paired_run_order_alternates(self):
        self.assertEqual(proof.paired_run_order(1), ('baseline', 'candidate'))
        self.assertEqual(proof.paired_run_order(2), ('candidate', 'baseline'))
        self.assertEqual(proof.paired_run_order(3), ('baseline', 'candidate'))

    def test_one_noisy_sample_does_not_establish_a_regression(self):
        rows = []
        for pair in range(1, proof.PAIRED_RUNS + 1):
            candidate = 150 if pair == 1 else 100
            rows.extend([result(pair, 'baseline', 100), result(pair, 'candidate', candidate)])
        self.assertEqual(proof.evaluate_comparison(rows)['verdict'], 'pass')

    def test_deliberately_slow_candidate_fails_the_relative_gate(self):
        # Models an implementation that adds a sustained 20% p95 regression.
        rows = []
        for pair in range(1, proof.PAIRED_RUNS + 1):
            rows.extend([result(pair, 'baseline', 100), result(pair, 'candidate', 120)])
        comparison = proof.evaluate_comparison(rows)
        self.assertEqual(comparison['verdict'], 'regression')
        self.assertGreater(comparison['relative_difference_95pct_ci_pct'][0], 15)

    def test_contaminated_run_is_inconclusive_not_a_pass(self):
        rows = []
        for pair in range(1, proof.PAIRED_RUNS + 1):
            rows.extend([result(pair, 'baseline', 100),
                         result(pair, 'candidate', 100, contaminated=(pair == 4))])
        self.assertEqual(proof.evaluate_comparison(rows)['verdict'], 'inconclusive')

    def test_ci_spanning_regression_limit_is_inconclusive(self):
        rows = []
        values = [100, 100, 100, 100, 130, 130, 130, 130]
        for pair, candidate in enumerate(values, 1):
            rows.extend([result(pair, 'baseline', 100), result(pair, 'candidate', candidate)])
        self.assertEqual(proof.evaluate_comparison(rows)['verdict'], 'inconclusive')

    def test_cpu_summary_includes_pressure_and_background_processes(self):
        rows = [dict(idle_pct=60, busy_pct=40, background_busy_pct=20,
                     cpu_psi_some_avg10_pct=12, cpu_psi_some_interval_pct=5,
                     guest_pct=20, steal_pct=0, swap_pages=0,
                     background_processes=[dict(name='provider', cpu_pct=20)])]
        summary = proof.summarize_cpu(rows)
        self.assertEqual(summary['max_cpu_psi_some_avg10_pct'], 12)
        self.assertEqual(summary['max_background_busy_pct'], 20)
        self.assertEqual(summary['peak_pressure_processes'][0]['name'], 'provider')


if __name__ == '__main__':
    unittest.main()
