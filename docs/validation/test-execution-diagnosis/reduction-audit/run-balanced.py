import os,sys,pathlib,json,subprocess,time,hashlib,concurrent.futures,tempfile
root=pathlib.Path.cwd(); out=root/'docs/validation/test-execution-diagnosis'; meta=json.loads((out/'probe-manifest.json').read_text());temp=pathlib.Path(meta['temporary_directory']); exe=pathlib.Path('C:\\Users\\lmas1\\AppData\\Local\\Temp\\steerline-test-reduction-gs36oadj\\combined.exe')
package='github.com/erranliu/SteerLine/internal/steerline'
listing=subprocess.check_output([str(exe),'-test.list=.'],cwd=root/'internal/steerline').decode('utf-8')
names=[s.strip() for s in listing.splitlines() if s.startswith(('Test','Example','Fuzz'))]
assert len(names)==len(set(names)) and names
bins=[[] for _ in range(3)]
baseline=json.loads((out/'baseline-serial/summary.json').read_text(encoding='utf-8'))
weights={t['name']:max(t['elapsed_seconds'],0.05) for t in baseline['summary']['tests'] if t['package']==package and '/' not in t['name']}
loads=[0.0,0.0,0.0]
for name in sorted(names,key=lambda n:(-weights.get(n,1.0),n)):
 index=min(range(3),key=lambda i:(loads[i],i));bins[index].append(name);loads[index]+=weights.get(name,1.0)
folder=out/'reduced-balanced-full';folder.mkdir(exist_ok=True)
(folder/'assignment.json').write_text(json.dumps({'algorithm':'Fixed recorded baseline weights, LPT descending then name, least-loaded shard then index; unknown test=1 second','source_commit':subprocess.check_output(['git','rev-parse','HEAD'],text=True).strip(),'shards':bins,'predicted_loads':loads,'weights':weights},indent=2)+'\n')
def run(item):
 idx,group=item;env=os.environ.copy();env['STEERLINE_HOME']=tempfile.mkdtemp(prefix=f'balanced-shard-{idx}-',dir=temp);pathlib.Path(env['STEERLINE_HOME']).mkdir(exist_ok=True)
 cmd=['go','tool','test2json','-t','-p',package,str(exe),'-test.v=test2json','-test.count=1','-test.parallel=1','-test.timeout=45m','-test.run=^('+'|'.join(group)+')$']
 started=time.perf_counter()
 with (folder/f'shard-{idx}.jsonl').open('wb') as stdout,(folder/f'shard-{idx}.stderr.txt').open('wb') as stderr:r=subprocess.run(cmd,cwd=root/'internal/steerline',env=env,stdout=stdout,stderr=stderr)
 return {'shard':idx,'exit_code':r.returncode,'wall_seconds':time.perf_counter()-started,'assigned':len(group)}
started=time.perf_counter()
with concurrent.futures.ThreadPoolExecutor(max_workers=3) as pool: results=list(pool.map(run,enumerate(bins)))
core_elapsed=time.perf_counter()-started
packages=subprocess.check_output(['go','list','./...'],cwd=root).decode('utf-8').splitlines();others=[p for p in packages if p!=package]
with (folder/'other-packages.jsonl').open('wb') as stdout,(folder/'other-packages.stderr.txt').open('wb') as stderr:
 other_result=subprocess.run(['go','test',*others,'-json','-count=1','-timeout=45m','-p','1','-parallel','1'],cwd=root,stdout=stdout,stderr=stderr)
elapsed=time.perf_counter()-started
observed=[];terminals=[]
for idx in range(3):
 for line in (folder/f'shard-{idx}.jsonl').read_text(encoding='utf-8').splitlines():
  e=json.loads(line)
  if e.get('Test') and '/' not in e['Test'] and e['Action']=='run':observed.append(e['Test'])
  if e.get('Test') and e['Action'] in ('pass','fail','skip'):terminals.append({'test':e['Test'],'status':e['Action'],'seconds':e.get('Elapsed',0)})
assert len(observed)==len(set(observed)) and set(observed)==set(names)
base=json.loads((out/'baseline-serial/summary.json').read_text(encoding='utf-8'))
original=[t for t in base['summary']['tests'] if t['package']==package]
original_names={t['name'] for t in original};new_names={t['test'] for t in terminals}
report={'wall_seconds':elapsed,'core_wall_seconds':core_elapsed,'other_packages_exit_code':other_result.returncode,'shards':results,'top_level_tests':len(names),'terminal_events':len(terminals),'failures':[t for t in terminals if t['status']=='fail'],'skipped':len([t for t in terminals if t['status']=='skip']),'same_top_level_set':True,'same_terminal_set':original_names==new_names,'missing':sorted(original_names-new_names),'extra':sorted(new_names-original_names),'serial_core_seconds':next(p['elapsed_seconds'] for p in base['summary']['packages'] if p['package']==package)}
others_seen=[]
for line in (folder/'other-packages.jsonl').read_text(encoding='utf-8').splitlines():
 e=json.loads(line)
 if e.get('Test') and e['Action'] in ('pass','fail','skip'):others_seen.append((e['Package'],e['Test'],e['Action']))
expected_other={(t['package'],t['name'],t['status']) for t in base['summary']['tests'] if t['package']!=package}
report['same_other_test_outcomes']=set(others_seen)==expected_other
report['same_core_test_outcomes']={(t['test'],t['status']) for t in terminals}=={(t['name'],t['status']) for t in original}
report['total_terminal_events']=len(terminals)+len(others_seen)
report['serial_full_seconds']=base['wall_seconds']
(folder/'summary.json').write_text(json.dumps(report,indent=2)+'\n');print(json.dumps(report),flush=True)
sys.exit(not report['same_core_test_outcomes'] or other_result.returncode or not report['same_other_test_outcomes'] or any(r['exit_code'] for r in results) or bool(report['failures']) or not report['same_terminal_set'])
