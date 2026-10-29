"""Owned, model-free fixtures: a redirected process must never admit tests/publication."""
import json
import os
from pathlib import Path
import shutil
import subprocess
import tempfile
import unittest

ROOT = Path(__file__).resolve().parents[1]
PWSH = shutil.which('pwsh')


class EnvironmentTTYTests(unittest.TestCase):
    def run_entry(self, root, phase, env=None):
        return subprocess.run([PWSH, '-NoProfile', '-File', str(root / 'scripts/verify-environment.ps1'), '-Phase', phase],
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
                    initial = {'version': 1, 'activity_id': 'a' * 32, 'round': 1, 'source_commit': commit,
                               'script_digest': 'b' * 64, 'completed': False,
                               'previous': {'message': 'retained immutable input'},
                               'checks': [{'id': 'terminal.tty', 'status': 'passed', 'message': 'old positive is insufficient'}],
                               'l2': [{'test_id': 'l2.fixture', 'status': 'incomplete', 'message': 'not executed'}],
                               'pr': {'url': '', 'base': '', 'head': '', 'checks_passed': False, 'reviews_passed': False,
                                      'merged': False, 'merge_commit': '', 'cleanup_completed': True, 'outcome_unknown': False}, 'message': ''}
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
                    self.assertFalse(result['pr']['merged'])
                    self.assertEqual(result['l2'][0]['status'], 'incomplete')
                    self.assertEqual(json.loads(Path(env['STEERLINE_PROGRESS_FILE']).read_text()), {'test.l2.fixture': False, 'pr': False})
            calls = marker.read_text() if marker.exists() else ''
            self.assertNotIn('go run', calls)
            self.assertNotIn('pr create', calls)
            self.assertFalse((root / 'Test-Log').exists())


if __name__ == '__main__':
    if not PWSH:
        raise SystemExit('PowerShell 7 is required; this check must not silently skip')
    unittest.main()
