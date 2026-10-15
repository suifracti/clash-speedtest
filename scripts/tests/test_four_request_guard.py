"""Offline lifecycle regression: real temporary SQLite, no real process signals or HTTP."""
import hashlib
import json
from pathlib import Path
import runpy
import sqlite3
import subprocess
import tempfile
import unittest
from unittest.mock import patch

SCRIPT = Path(__file__).resolve().parents[1] / 'four_request_guard.py'


class GuardTests(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self.folder = Path(self.tmp.name)
        self.root = self.folder / 'data'
        (self.root / 'history').mkdir(parents=True)
        (self.folder / 'private').mkdir()
        self.core = self.folder / 'synthetic-core'
        self.core.write_bytes(b'offline fixture, never executed')
        self.command = f'{self.core} --gui --data-dir {self.root} --no-auto-credentials'
        self.write('candidate-entry.json', {'data_root': str(self.root), 'core': str(self.core)})
        self.write('build-manifest.json', {'core_sha256': hashlib.sha256(self.core.read_bytes()).hexdigest()})
        self.write('verified-nodes.json', [dict(profile_id='p', identity_sha256=hashlib.sha256(n.encode()).hexdigest(), revision='r') for n in ['n1', 'n2']])
        self.conn = sqlite3.connect(self.root / 'history/history.db')
        self.addCleanup(self.conn.close)
        self.conn.executescript('''
        CREATE TABLE workbench_latency_tests(attempt_id TEXT, profile_id TEXT, node_identity_key TEXT, config_revision_key TEXT);
        CREATE TABLE workbench_latency_samples(attempt_id TEXT, target TEXT);
        CREATE TABLE workbench_latency_batches(batch_id TEXT, request_id TEXT, state TEXT);
        CREATE TABLE workbench_latency_batch_items(batch_id TEXT, attempt_id TEXT, profile_id TEXT, node_key TEXT, node_identity_key TEXT, config_revision_key TEXT, execution_state TEXT, persistence_state TEXT);
        CREATE TABLE workbench_public_service_attempts(attempt_id TEXT, request_id TEXT, profile_id TEXT, node_key TEXT, node_identity_key TEXT, config_revision_key TEXT, service_id TEXT, execution_state TEXT, persistence_state TEXT);
        CREATE TABLE workbench_public_service_results(attempt_id TEXT);
        CREATE TABLE measurement_rounds(round_id TEXT, state TEXT);
        CREATE TABLE measurement_round_items(round_id TEXT, request_id TEXT, project TEXT, service_id TEXT, profile_id TEXT, node_key TEXT, node_identity_key TEXT, config_revision_key TEXT, not_executed_reason TEXT);
        CREATE TABLE workbench_download_attempts(attempt_id TEXT);
        CREATE TABLE workbench_download_results(attempt_id TEXT);
        CREATE TABLE monitor_samples(sample_id TEXT);
        CREATE TABLE monitor_job_definitions(job_id TEXT);
        ''')
        self.clock = 1000.0
        self.ticks = 0
        self.signals = []

    def write(self, name, data):
        (self.folder / name).write_text(json.dumps(data))

    def results(self, *, saved=True, finished=True):
        d = self.conn
        d.execute('INSERT INTO measurement_rounds VALUES (?,?)', ('new', 'finished' if finished else 'running'))
        d.execute("INSERT INTO workbench_latency_batches VALUES ('b','lat-request','completed')")
        for n in ['n1', 'n2']:
            d.execute('INSERT INTO workbench_latency_tests VALUES (?,?,?,?)', ('lat-'+n, 'p', n, 'r'))
            d.execute('INSERT INTO workbench_latency_samples VALUES (?,?)', ('lat-'+n, 'https://speed.cloudflare.com/__down?bytes=1'))
            d.execute('INSERT INTO workbench_latency_batch_items VALUES (?,?,?,?,?,?,?,?)', ('b', 'lat-'+n, 'p', n, n, 'r', 'completed', 'saved'))
            d.execute('INSERT INTO workbench_public_service_attempts VALUES (?,?,?,?,?,?,?,?,?)', ('svc-'+n, 'req-'+n, 'p', n, n, 'r', 'cloudflare_204', 'completed', 'saved' if saved else 'saving'))
            if saved:
                d.execute('INSERT INTO workbench_public_service_results VALUES (?)', ('svc-'+n,))
            for project, request, service in [('latency', 'lat-request', ''), ('service', 'req-'+n, 'cloudflare_204')]:
                d.execute('INSERT INTO measurement_round_items VALUES (?,?,?,?,?,?,?,?,?)', ('new', request, project, service, 'p', n, n, 'r', ''))
        d.commit()

    def run_guard(self, first_tick, later_tick=None, process_exit=False):
        def sleep(_):
            self.ticks += 1
            self.clock += 1 if self.ticks == 1 else 181
            if self.ticks == 1:
                self.write('arm-live-guard.json', {'armed_at_epoch': self.clock})
                first_tick()
            elif later_tick:
                later_tick()
            if self.ticks > 5:
                raise RuntimeError('fixture exceeded bounded iterations')

        def ps(args, **kwargs):
            if process_exit and self.ticks:
                raise subprocess.CalledProcessError(1, args)
            prefix = '99999 ' if '-axww' in args else ''
            return (prefix + self.command + '\n').encode()

        with patch('sys.argv', [str(SCRIPT), str(self.folder)]), patch('subprocess.check_output', side_effect=ps), patch('subprocess.run', return_value=subprocess.CompletedProcess([], 0, stdout='')), patch('time.time', side_effect=lambda: self.clock), patch('time.sleep', side_effect=sleep), patch('os.kill', side_effect=lambda *x: self.signals.append(x)):
            runpy.run_path(str(SCRIPT), run_name='__main__')
        return json.loads((self.folder / 'run-boundary.json').read_text())

    def test_saved_round_stops_observer_without_closing_app(self):
        result = self.run_guard(self.results)
        self.assertEqual(result['stop_reason'], 'completed_saved')
        self.assertEqual(self.signals, [])
        self.assertLess(self.clock, 1181)

    def test_finished_round_does_not_hide_saving_results(self):
        result = self.run_guard(lambda: self.results(saved=False))
        self.assertEqual(result['stop_reason'], 'absolute_deadline')
        self.assertEqual(len(self.signals), 1)

    def test_saved_children_do_not_hide_running_round(self):
        result = self.run_guard(lambda: self.results(finished=False))
        self.assertEqual(result['stop_reason'], 'absolute_deadline')

    def test_missing_persisted_result_cannot_claim_completion(self):
        def first():
            self.results()
            self.conn.execute('DELETE FROM workbench_public_service_results')
            self.conn.commit()
        result = self.run_guard(first)
        self.assertEqual(result['stop_reason'], 'absolute_deadline')

    def test_process_exit_does_not_signal_replacement_or_raise_unbound_command(self):
        result = self.run_guard(lambda: None, process_exit=True)
        self.assertEqual(result['stop_reason'], 'process_exited')
        self.assertEqual(self.signals, [])

    def test_observer_sql_error_is_recorded_and_bounded(self):
        def first():
            self.conn.execute('DROP TABLE workbench_latency_samples')
            self.conn.commit()
        result = self.run_guard(first)
        self.assertIn(result['stop_reason'], ['budget_or_observer_gate', 'absolute_deadline'])
        observations = json.loads((self.folder / 'live-observations.json').read_text())
        self.assertTrue(any(x.get('observer_error') == 'OperationalError' for x in observations))

    def test_saved_failures_also_finish_without_retry_or_app_exit(self):
        def first():
            self.results()
            self.conn.execute("UPDATE workbench_public_service_attempts SET execution_state='failed'")
            self.conn.execute("UPDATE workbench_latency_batch_items SET execution_state='failed'")
            self.conn.commit()
        result = self.run_guard(first)
        self.assertEqual(result['stop_reason'], 'completed_saved')
        self.assertEqual(self.signals, [])

    def test_save_failure_stays_incomplete(self):
        def first():
            self.results(saved=False)
            self.conn.execute("UPDATE workbench_public_service_attempts SET persistence_state='failed'")
            self.conn.commit()
        self.assertEqual(self.run_guard(first)['stop_reason'], 'absolute_deadline')

    def test_old_finished_round_cannot_end_new_observation(self):
        self.results()
        self.assertEqual(self.run_guard(lambda: None)['stop_reason'], 'absolute_deadline')

    def test_wrong_round_request_cannot_borrow_other_saved_result(self):
        def first():
            self.results()
            self.conn.execute("UPDATE measurement_round_items SET request_id='unknown' WHERE project='service'")
            self.conn.commit()
        self.assertEqual(self.run_guard(first)['stop_reason'], 'absolute_deadline')

    def test_scope_drift_is_stopped_even_if_all_rows_saved(self):
        def first():
            self.results()
            self.conn.execute("UPDATE workbench_public_service_attempts SET config_revision_key='changed'")
            self.conn.commit()
        self.assertEqual(self.run_guard(first)['stop_reason'], 'budget_or_observer_gate')
        self.assertEqual(len(self.signals), 1)

    def test_budget_violation_is_not_hidden_by_completed_round(self):
        def first():
            self.results()
            self.conn.execute("INSERT INTO workbench_download_attempts VALUES ('unauthorized')")
            self.conn.commit()
        self.assertEqual(self.run_guard(first)['stop_reason'], 'budget_or_observer_gate')
        self.assertEqual(len(self.signals), 1)

    def test_three_of_four_saved_is_not_completion(self):
        def first():
            self.results()
            self.conn.execute("DELETE FROM measurement_round_items WHERE request_id='req-n2'")
            self.conn.commit()
        self.assertEqual(self.run_guard(first)['stop_reason'], 'absolute_deadline')

    def test_previous_evidence_is_not_overwritten(self):
        self.write('run-boundary.json', {'state': 'stopped', 'stop_reason': 'absolute_deadline'})
        before = (self.folder / 'run-boundary.json').read_bytes()
        with self.assertRaisesRegex(SystemExit, 'refusing to overwrite'):
            self.run_guard(lambda: None)
        self.assertEqual((self.folder / 'run-boundary.json').read_bytes(), before)
        self.assertEqual(self.signals, [])


if __name__ == '__main__':
    unittest.main()
