"""Owned, model-free fixtures: a redirected process must never admit tests/publication."""
import json
from contextlib import contextmanager
from copy import deepcopy
import os
from pathlib import Path
import shutil
import re
import subprocess
import tempfile
import unittest

ROOT = Path(__file__).resolve().parents[1]
PWSH = shutil.which('pwsh')


class EnvironmentTTYTests(unittest.TestCase):
    def run_entry(self, root, phase, env=None, extra=()):
        return subprocess.run([PWSH, '-NoProfile', '-File', str(root / 'scripts/verify-environment.ps1'), '-Phase', phase, *extra],
                              capture_output=True, text=True, encoding='utf-8', env=env, timeout=30)

    def test_probe_rejects_redirected_process_without_channels_or_tests(self):
        p = self.run_entry(ROOT, 'tty')
        self.assertEqual(p.returncode, 1, p.stdout + p.stderr)
        check = json.loads(p.stdout)
        self.assertEqual(check['id'], 'terminal.tty')
        self.assertEqual(check['status'], 'failed')
        self.assertIn('stdin_redirected=True', check['message'])
        self.assertNotIn('STEERLINE_TTY_OK', check['message'])

    def test_prepare_and_publish_reject_current_non_tty_even_with_old_positive(self):
        with tempfile.TemporaryDirectory(prefix='steerline-tty-fixture-') as tmp:
            parent = Path(tmp)
            root = parent / 'source'
            (root / 'scripts').mkdir(parents=True)
            (root / 'SteerlineFlow/testing').mkdir(parents=True)
            for name in ('verify-environment.ps1', 'environment-tty.ps1'):
                shutil.copyfile(ROOT / 'scripts' / name, root / 'scripts' / name)
            (root / 'SteerlineFlow/environment.md').write_text('```steerline-environment\n{"validation_base":"codex/validation-fixture"}\n```\n')
            (root / 'SteerlineFlow/testing/test-list.json').write_text(json.dumps({'tests': [{'id': 'l2.fixture', 'level': 'L2'}]}))
            subprocess.run(['git', 'init', '-b', 'codex/tty-fixture', str(root)], check=True, capture_output=True)
            subprocess.run(['git', '-C', str(root), 'add', '.'], check=True, capture_output=True)
            subprocess.run(['git', '-C', str(root), '-c', 'user.name=Fixture', '-c', 'user.email=fixture@example.invalid', 'commit', '-m', 'fixture'], check=True, capture_output=True)
            commit = subprocess.check_output(['git', '-C', str(root), 'rev-parse', 'HEAD'], text=True).strip()
            shim = parent / 'tools'
            shim.mkdir()
            marker = parent / 'unexpected-command.txt'
            # Scoped fake commands cannot install, contact providers, or run project tests.
            for name in ('go', 'gh', 'tea', 'python', 'dotnet'):
                body = f'@echo off\r\necho {name} %*>>"{marker}"\r\n'
                if name == 'go':
                    body += 'if "%~1"=="version" (echo go version go1.26.4 windows/amd64 & exit /b 0)\r\n'
                body += 'exit /b 1\r\n'
                (shim / (name + '.cmd')).write_text(body)
            env = dict(os.environ)
            env['PATH'] = str(shim) + os.pathsep + env['PATH']
            env.pop('UNITY_EDITOR_PATH', None)
            env['STEERLINE_GITEA_LOGIN'] = 'fixture-not-a-real-login'
            env['STEERLINE_RESULT_FILE'] = str(parent / 'result.json')
            env['STEERLINE_PROGRESS_FILE'] = str(parent / 'progress.json')
            for phase in ('prepare', 'publish'):
                with self.subTest(phase=phase):
                    initial = {'version': 2, 'activity_id': 'a' * 32, 'round': 1, 'source_commit': commit,
                               'script_digest': 'b' * 64, 'completed': False,
                               'previous': {'message': 'retained immutable input'},
                               'checks': [{'id': 'terminal.tty', 'status': 'passed', 'message': 'old positive is insufficient'}],
                               'l2': [{'test_id': 'l2.fixture', 'status': 'incomplete', 'message': 'not executed'}],
                               'pr': {'url': '', 'base': '', 'head': '', 'head_revision': '',
                                      'cleanup_completed': True, 'outcome_unknown': False}, 'message': ''}
                    Path(env['STEERLINE_RESULT_FILE']).write_text(json.dumps(initial))
                    Path(env['STEERLINE_PROGRESS_FILE']).write_text(json.dumps({'test.l2.fixture': False, 'pr': False}))
                    p = self.run_entry(root, phase, env)
                    self.assertEqual(p.returncode, 1, p.stdout + p.stderr)
                    result = json.loads(Path(env['STEERLINE_RESULT_FILE']).read_text(encoding='utf-8'))
                    self.assertTrue(result['completed'])
                    self.assertEqual(result['previous'], initial['previous'])
                    tty = [c for c in result['checks'] if c['id'] == 'terminal.tty']
                    self.assertEqual(len(tty), 1)
                    self.assertEqual(tty[0]['status'], 'failed')
                    self.assertFalse(result['pr']['url'])
                    self.assertEqual(result['l2'][0]['status'], 'incomplete')
                    self.assertEqual(json.loads(Path(env['STEERLINE_PROGRESS_FILE']).read_text()), {'test.l2.fixture': False, 'pr': False})
            calls = marker.read_text() if marker.exists() else ''
            self.assertNotIn('go run', calls)
            self.assertNotIn('tools/test-project.py', calls)
            self.assertNotIn('pr create', calls)
            self.assertFalse((root / 'Test-Log').exists())

    @contextmanager
    def failure_fixture(self):
        with tempfile.TemporaryDirectory(prefix='steerline-tty-report-') as tmp:
            parent = Path(tmp)
            root = parent / 'not-a-git-repository'
            (root / 'scripts').mkdir(parents=True)
            (root / 'SteerlineFlow/testing').mkdir(parents=True)
            for name in ('verify-environment.ps1', 'environment-tty.ps1'):
                shutil.copyfile(ROOT / 'scripts' / name, root / 'scripts' / name)
            (root / 'SteerlineFlow/environment.md').write_text('```steerline-environment\n{"validation_base":"codex/fixture"}\n```\n')
            (root / 'SteerlineFlow/testing/test-list.json').write_text(json.dumps({'tests': [{'id': 'l2.fixture', 'level': 'L2'}]}))
            tools = parent / 'tools'
            tools.mkdir()
            marker = parent / 'unexpected-tool.txt'
            for name in ('git', 'go', 'gh', 'tea', 'python', 'dotnet', 'Unity'):
                (tools / (name + '.cmd')).write_text(f'@echo off\r\necho {name}>>"{marker}"\r\nexit /b 99\r\n')
            env = dict(os.environ, PATH=str(tools) + os.pathsep + os.environ['PATH'],
                       STEERLINE_RESULT_FILE=str(parent / 'result.json'), STEERLINE_PROGRESS_FILE=str(parent / 'progress.json'))
            report = {'version': 2, 'activity_id': 'a' * 32, 'round': 2, 'source_commit': 'c' * 40,
                      'script_digest': 'd' * 64, 'completed': False, 'checks': [], 'l2': [], 'message': '',
                      'previous': {'round': 1, 'checks': [{'id': 'prior.only', 'status': 'passed'}]},
                      'pr': {'url': '', 'base': '', 'head': '', 'head_revision': '',
                             'cleanup_completed': False, 'outcome_unknown': False}}
            progress = {'pr': False, 'test.l2.fixture': False}
            yield root, env, report, progress, marker

    def save_channels(self, env, report, progress):
        Path(env['STEERLINE_RESULT_FILE']).write_text(json.dumps(report), encoding='utf-8')
        Path(env['STEERLINE_PROGRESS_FILE']).write_text(json.dumps(progress), encoding='utf-8')

    def declared_probe_ids(self):
        source = (ROOT / 'scripts/verify-environment.ps1').read_text(encoding='utf-8')
        # Compare with actual normal probes/direct incomplete facts, not the new ID array.
        return set(re.findall(r"Check '([^']+)'", source)) | set(re.findall(r"\bid='([^']+)'", source))

    def test_tty_failure_records_negative_without_git_tests_or_provider(self):
        with self.failure_fixture() as (root, env, initial, progress, marker):
            self.save_channels(env, initial, progress)
            reason = 'CreateProcessW failed before process creation; original error 317'
            p = self.run_entry(root, 'tty-failure', env, ('-FailureReason', reason))
            self.assertEqual(p.returncode, 1, p.stdout + p.stderr)
            result = json.loads(Path(env['STEERLINE_RESULT_FILE']).read_text(encoding='utf-8'))
            for key in ('version', 'activity_id', 'round', 'source_commit', 'script_digest', 'previous'):
                self.assertEqual(result[key], initial[key])
            self.assertTrue(result['completed'])
            self.assertEqual({c['id'] for c in result['checks']}, self.declared_probe_ids())
            for check in result['checks']:
                self.assertEqual(check['status'], 'failed' if check['id'] == 'terminal.tty' else 'incomplete')
                if check['id'] == 'terminal.tty':
                    self.assertEqual(check['message'], reason)
            self.assertEqual(result['l2'][0]['status'], 'incomplete')
            self.assertFalse(result['pr']['url'])
            self.assertFalse(result['pr']['outcome_unknown'])
            self.assertTrue(result['pr']['cleanup_completed'])
            self.assertEqual(json.loads(Path(env['STEERLINE_PROGRESS_FILE']).read_text()), progress)
            self.assertFalse(marker.exists(), 'failure recorder launched an environment tool')
            self.assertFalse(list(Path(env['STEERLINE_RESULT_FILE']).parent.glob('*.tmp')))

    def test_tty_failure_preserves_same_round_completed_facts(self):
        with self.failure_fixture() as (root, env, initial, progress, marker):
            initial['completed'] = True
            initial['pr']['cleanup_completed'] = True
            check_id = next(i for i in self.declared_probe_ids() if i != 'terminal.tty')
            verified = {'id': check_id, 'status': 'passed', 'message': 'same round original fact'}
            l2 = {'test_id': 'l2.fixture', 'status': 'passed', 'message': 'same round complete L2'}
            initial['checks'] = [verified]
            initial['l2'] = [l2]
            progress['test.l2.fixture'] = True
            self.save_channels(env, initial, progress)
            p = self.run_entry(root, 'tty-failure', env, ('-FailureReason', 'publish TTY process did not start'))
            self.assertEqual(p.returncode, 1, p.stdout + p.stderr)
            result = json.loads(Path(env['STEERLINE_RESULT_FILE']).read_text(encoding='utf-8'))
            self.assertIn(verified, result['checks'])
            self.assertEqual(result['l2'], [l2])
            self.assertEqual(result['previous'], initial['previous'])
            self.assertEqual(json.loads(Path(env['STEERLINE_PROGRESS_FILE']).read_text()), progress)
            self.assertFalse(marker.exists())

    def test_invalid_failure_input_never_rewrites_either_channel(self):
        with self.failure_fixture() as (root, env, initial, progress, marker):
            check_id = next(i for i in self.declared_probe_ids() if i != 'terminal.tty')
            cases = []
            for key, value in [('url', 'https://example.invalid/pull/1'), ('head_revision', 'a' * 40), ('outcome_unknown', True)]:
                report = deepcopy(initial); report['pr'][key] = value
                cases.append((key, report, progress, ('-FailureReason', 'original failure')))
            for name, field, value in [('old-format', 'version', 1), ('wrong-type', 'version', '2'),
                                       ('wrong-id', 'activity_id', 'invalid'), ('unknown-field', 'extra', True),
                                       ('partial', 'checks', [{'id': check_id, 'status': 'passed', 'message': 'producer may still run'}]),
                                       ('partial-l2', 'l2', [{'test_id': 'l2.fixture', 'status': 'incomplete', 'message': 'started'}]),
                                       ('invalid-status', 'checks', [{'id': check_id, 'status': 'unknown', 'message': ''}]),
                                       ('duplicate-check', 'checks', [{'id': check_id, 'status': 'failed', 'message': ''}] * 2),
                                       ('duplicate-l2', 'l2', [{'test_id': 'l2.fixture', 'status': 'incomplete', 'message': ''}] * 2)]:
                report = deepcopy(initial); report[field] = value
                cases.append((name, report, progress, ('-FailureReason', 'original failure')))
            report = deepcopy(initial); report['completed'] = True
            cases.append(('unsettled-cleanup', report, progress, ('-FailureReason', 'original failure')))
            for name, value in [('published-progress', {'pr': True, 'test.l2.fixture': False}),
                                ('wrong-progress-type', {'pr': False, 'test.l2.fixture': 'false'}),
                                ('unknown-progress', {'pr': False, 'extra': False})]:
                cases.append((name, initial, value, ('-FailureReason', 'original failure')))
            for reason in ('', ' ', 'x' * 8193):
                cases.append(('invalid-reason', initial, progress, ('-FailureReason', reason)))
            cases.append(('positive-status-forbidden', initial, progress, ('-FailureReason', 'error', '-FailureStatus', 'passed')))
            for name, report, state, extra in cases:
                with self.subTest(name=name):
                    self.save_channels(env, report, state)
                    before = [Path(env[k]).read_bytes() for k in ('STEERLINE_RESULT_FILE', 'STEERLINE_PROGRESS_FILE')]
                    p = self.run_entry(root, 'tty-failure', env, extra)
                    self.assertNotEqual(p.returncode, 0, p.stdout + p.stderr)
                    self.assertEqual([Path(env[k]).read_bytes() for k in ('STEERLINE_RESULT_FILE', 'STEERLINE_PROGRESS_FILE')], before)
            self.assertFalse(marker.exists())


if __name__ == '__main__':
    if not PWSH:
        raise SystemExit('PowerShell 7 is required; this check must not silently skip')
    unittest.main()
