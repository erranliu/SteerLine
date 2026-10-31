"""Execute the original publication block with scoped provider doubles on PS5.1."""
import json
import os
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile
import unittest

ROOT = Path(__file__).resolve().parents[1]
POWERSHELL = shutil.which('powershell.exe')

PROVIDER = r'''
import json, os, sys
from pathlib import Path
args = sys.argv[1:]
variant = os.environ['PUBLICATION_CASE']
with Path(os.environ['PUBLICATION_CALLS']).open('a', encoding='utf-8') as f:
    f.write(json.dumps(args) + '\n')
url = 'https://example.invalid/pulls/7'
gitea = args[0] == 'api'
base, head, revision = 'codex/fixture-base', 'codex/fixture-head', 'd' * 40
pr = dict(number=7, url=url, html_url=url, state='open' if gitea else 'OPEN',
          baseRefName=base, headRefName=head, headRefOid=revision,
          base=dict(ref=base), head=dict(ref=head, sha=revision), merged=False)
listing = ('--method' in args and args[args.index('--method')+1] == 'GET'
           and '/pulls?' in args[-1]) if gitea else args[:2] == ['pr', 'list']
creating = ('--method' in args and args[args.index('--method')+1] == 'POST') if gitea else args[:2] == ['pr', 'create']
if listing:
    print(json.dumps([] if variant == 'empty' else [pr, pr] if variant == 'multiple' else [pr]))
elif creating:
    if gitea:
        body = json.load(sys.stdin)
        assert body['base'] == base and body['head'] == head
    else:
        assert args[args.index('--base')+1] == base and args[args.index('--head')+1] == head
    print(json.dumps(pr) if gitea else url)
else:
    assert args[-1].endswith('/pulls/7') if gitea else args[:3] == ['pr', 'view', url]
    if variant == 'query-error': sys.exit(13)
    if variant == 'wrong-base': pr.update(baseRefName='wrong', base=dict(ref='wrong'))
    if variant == 'wrong-head': pr.update(headRefName='wrong', head=dict(ref='wrong', sha=revision))
    if variant == 'wrong-revision': pr.update(headRefOid='e'*40, head=dict(ref=head, sha='e'*40))
    print(json.dumps(pr))
'''

HARNESS = r'''
param([string]$Entry,[string]$Result)
$ErrorActionPreference='Stop'
if($PSVersionTable.PSVersion.Major -ne 5 -or $PSVersionTable.PSVersion.Minor -ne 1){throw 'Real Windows PowerShell 5.1 required'}
$tokens=$null;$errors=$null
$ast=[Management.Automation.Language.Parser]::ParseFile($Entry,[ref]$tokens,[ref]$errors)
if($errors.Count){throw 'Original script syntax invalid'}
foreach($name in @('Invoke-GhValue','Invoke-GiteaApi')){
    $node=$ast.Find({param($n) $n -is [Management.Automation.Language.FunctionDefinitionAst] -and $n.Name -ceq $name},$true)
    if($node){Invoke-Expression $node.Extent.Text}
}
$original=[IO.File]::ReadAllText($Entry)
$start=$original.IndexOf('$report.pr.outcome_unknown=$true;')
$end=$original.IndexOf('} catch {',$start)
if($start -lt 0 -or $end -lt $start){throw 'Original publication block missing'}
$block=$original.Substring($start,$end-$start)
$script:pushes=0;$script:snapshots=@()
function Invoke-GitValue {$script:pushes++}
function Save-Result {$script:snapshots+=($report|ConvertTo-Json -Depth 8 -Compress)}
function Set-Progress {}
# tea is a native EXE in production. Forward literal arguments to the fixture
# EXE; a .cmd double would reinterpret the query's ampersands through cmd.exe.
function tea {$parameters=@($args|ForEach-Object{$_});$input|& $env:PUBLICATION_PYTHON $env:PUBLICATION_PROVIDER @parameters}
$head='codex/fixture-head';$base='codex/fixture-base';$revision='d'*40
$report=@{activity_id=('a'*32);completed=$false;pr=@{url='';head_revision='';cleanup_completed=$true;outcome_unknown=$false}}
$failure=''
try {& ([scriptblock]::Create($block))} catch {$failure=$_.Exception.Message}
$value=@{report=$report;error=$failure;pushes=$script:pushes;snapshots=$script:snapshots}
[IO.File]::WriteAllText($Result,($value|ConvertTo-Json -Depth 10),[Text.UTF8Encoding]::new($false))
'''

PROBE_HARNESS = r'''
param([string]$Entry,[string]$Fixture,[string]$Result)
$ErrorActionPreference='Stop'
if($PSVersionTable.PSVersion.Major -ne 5 -or $PSVersionTable.PSVersion.Minor -ne 1){throw 'Real PS5.1 required'}
Set-Location -LiteralPath $Fixture
$tokens=$null;$errors=$null
$ast=[Management.Automation.Language.Parser]::ParseFile($Entry,[ref]$tokens,[ref]$errors)
if($errors.Count){throw 'Original script syntax invalid'}
$node=$ast.Find({param($n) $n -is [Management.Automation.Language.FunctionDefinitionAst] -and $n.Name -ceq 'Invoke-GitValue'},$true)
Invoke-Expression $node.Extent.Text
$original=[IO.File]::ReadAllText($Entry)
$start=$original.IndexOf("    $"+'revision=Invoke-GitValue')
$end=$original.IndexOf("    $"+'changed=@(',$start)
if($start -lt 0 -or $end -lt $start){throw 'Original probe commit block missing'}
$block=$original.Substring($start,$end-$start)
$script:commits=0
function git {
    $parameters=@($args|ForEach-Object{$_})
    if($parameters[0] -ceq 'commit'){$script:commits++}
    if($env:PROBE_CASE -ceq 'check-error' -and ($parameters -join ' ') -ceq 'diff --cached --check'){$global:LASTEXITCODE=17;return}
    & $env:PROBE_GIT @parameters
    $exit=$LASTEXITCODE
    if($exit -eq 0 -and $parameters[0] -ceq 'add'){
        switch($env:PROBE_CASE){
            'extra' {[IO.File]::WriteAllText((Join-Path $Fixture 'foreign.txt'),'foreign');& $env:PROBE_GIT add -- foreign.txt|Out-Null;$exit=$LASTEXITCODE}
            'empty' {& $env:PROBE_GIT reset --quiet HEAD;$exit=$LASTEXITCODE}
            'altered' {[IO.File]::WriteAllText((Join-Path $Fixture $parameters[-1]),'altered');& $env:PROBE_GIT add -- $parameters[-1]|Out-Null;$exit=$LASTEXITCODE}
            'padded' {$path=Join-Path $Fixture $parameters[-1];[IO.File]::WriteAllText($path,(' '+[IO.File]::ReadAllText($path)));& $env:PROBE_GIT add -- $parameters[-1]|Out-Null;$exit=$LASTEXITCODE}
        }
    }
    $global:LASTEXITCODE=$exit
}
$root=$Fixture;$report=@{activity_id=('a'*32);round=1;source_commit=$env:PROBE_SOURCE}
$probe='SteerlineFlow/testing/validation/'+$report.activity_id+'-1.md'
$failure=''
try {& ([scriptblock]::Create($block))} catch {$failure=$_.Exception.Message}
$head=(& $env:PROBE_GIT rev-parse HEAD).Trim()
[IO.File]::WriteAllText($Result,(@{error=$failure;commits=$script:commits;head=$head}|ConvertTo-Json),[Text.UTF8Encoding]::new($false))
'''


class PublicationTests(unittest.TestCase):
    def test_original_probe_checks_index_before_commit(self):
        git = shutil.which('git')
        self.assertIsNotNone(git)
        for case in ('valid', 'extra', 'empty', 'altered', 'padded', 'check-error'):
            with self.subTest(case=case), tempfile.TemporaryDirectory(prefix='environment-probe-') as tmp:
                fixture = Path(tmp)
                repo = fixture / 'source'
                subprocess.run([git, 'init', '-b', 'codex/probe-fixture', str(repo)], check=True, capture_output=True)
                for key, value in (('user.name', 'Fixture'), ('user.email', 'fixture@example.invalid'), ('core.autocrlf', 'false')):
                    subprocess.run([git, '-C', str(repo), 'config', key, value], check=True, capture_output=True)
                (repo / 'base.txt').write_text('fixture\n')
                subprocess.run([git, '-C', str(repo), 'add', '.'], check=True, capture_output=True)
                subprocess.run([git, '-C', str(repo), 'commit', '-m', 'fixture'], check=True, capture_output=True)
                source = subprocess.check_output([git, '-C', str(repo), 'rev-parse', 'HEAD'], text=True).strip()
                harness, result_file = fixture / 'fixture.ps1', fixture / 'result.json'
                harness.write_text(PROBE_HARNESS, encoding='utf-8')
                env = dict(os.environ, PROBE_CASE=case, PROBE_GIT=git, PROBE_SOURCE=source)
                env['PATH'] = os.pathsep.join(p for p in env.get('PATH', '').split(os.pathsep)
                                            if p and not (Path(p) / 'pwsh.exe').exists() and not (Path(p) / 'pwsh').exists())
                self.assertIsNone(shutil.which('pwsh', path=env['PATH']))
                p = subprocess.run([POWERSHELL, '-NoProfile', '-File', str(harness),
                                    str(ROOT / 'scripts/verify-environment.ps1'), str(repo), str(result_file)],
                                   cwd=repo, capture_output=True, text=True, encoding='utf-8', env=env, timeout=20)
                self.assertEqual(p.returncode, 0, p.stdout + p.stderr)
                result = json.loads(result_file.read_text(encoding='utf-8'))
                if case == 'valid':
                    self.assertFalse(result['error'], result)
                    self.assertEqual(result['commits'], 1)
                    self.assertNotEqual(result['head'], source)
                    changed = subprocess.check_output([git, '-C', str(repo), 'diff', '--name-only', source, 'HEAD'], text=True).strip()
                    self.assertEqual(changed, 'SteerlineFlow/testing/validation/' + 'a' * 32 + '-1.md')
                else:
                    self.assertTrue(result['error'], result)
                    if case == 'check-error':
                        self.assertIn('Git failed:', result['error'])
                        self.assertIn('--check', result['error'])
                    else:
                        self.assertEqual(result['error'], 'Staged diff must contain only the exact current validation probe')
                    self.assertEqual(result['commits'], 0)
                    self.assertEqual(result['head'], source)

    def test_original_publication_list_and_identity(self):
        for case in ('empty', 'existing', 'multiple', 'wrong-base', 'wrong-head', 'wrong-revision', 'query-error'):
            with self.subTest(case=case), tempfile.TemporaryDirectory(prefix='environment-publication-') as tmp:
                fixture = Path(tmp)
                (fixture / 'provider.py').write_text(PROVIDER, encoding='utf-8')
                (fixture / 'fixture.ps1').write_text(HARNESS, encoding='utf-8')
                for name in ('gh',):
                    (fixture / (name + '.cmd')).write_text(
                        f'@echo off\r\n"{sys.executable}" "{fixture / "provider.py"}" %*\r\n', encoding='utf-8')
                env = dict(os.environ)
                env['PATH'] = str(fixture) + os.pathsep + os.pathsep.join(
                    p for p in env.get('PATH', '').split(os.pathsep)
                    if p and not (Path(p) / 'pwsh.exe').exists() and not (Path(p) / 'pwsh').exists())
                self.assertIsNone(shutil.which('pwsh', path=env['PATH']))
                calls_file, result_file = fixture / 'calls.jsonl', fixture / 'result.json'
                env.update(PUBLICATION_CASE=case, PUBLICATION_CALLS=str(calls_file), STEERLINE_GITEA_LOGIN='fixture-only',
                           PUBLICATION_PYTHON=sys.executable, PUBLICATION_PROVIDER=str(fixture / 'provider.py'))
                p = subprocess.run([POWERSHELL, '-NoProfile', '-File', str(fixture / 'fixture.ps1'),
                                    str(ROOT / 'scripts/verify-environment.ps1'), str(result_file)],
                                   capture_output=True, text=True, encoding='utf-8', env=env, timeout=20)
                self.assertEqual(p.returncode, 0, p.stdout + p.stderr)
                result = json.loads(result_file.read_text(encoding='utf-8'))
                calls = [json.loads(line) for line in calls_file.read_text(encoding='utf-8').splitlines()]
                creates = [a for a in calls if a[:2] == ['pr', 'create'] or '--method' in a and a[a.index('--method')+1] == 'POST']
                self.assertEqual(len(creates), 1 if case == 'empty' else 0, calls)
                self.assertEqual(result['pushes'], 1)
                first = json.loads(result['snapshots'][0])['pr']
                self.assertTrue(first['outcome_unknown'])
                self.assertFalse(first['cleanup_completed'])
                pr = result['report']['pr']
                if case in ('empty', 'existing'):
                    self.assertFalse(result['error'], result)
                    self.assertEqual(pr['url'], 'https://example.invalid/pulls/7')
                    self.assertEqual(pr['head_revision'], 'd' * 40)
                    self.assertTrue(pr['cleanup_completed'])
                    self.assertFalse(pr['outcome_unknown'])
                    self.assertEqual(len(calls), 3 if case == 'empty' else 2)
                else:
                    self.assertTrue(result['error'], result)
                    self.assertTrue(pr['outcome_unknown'])
                    self.assertFalse(pr['cleanup_completed'])
                    self.assertFalse(pr['head_revision'])
                    self.assertEqual(len(calls), 1 if case == 'multiple' else 2)


if __name__ == '__main__':
    if not POWERSHELL:
        raise SystemExit('Windows PowerShell 5.1 required; do not silently skip')
    unittest.main()
