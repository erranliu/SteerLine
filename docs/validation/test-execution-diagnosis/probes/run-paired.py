import pathlib,json,subprocess,os,time,statistics,collections
root=pathlib.Path.cwd();out=root/'docs/validation/test-execution-diagnosis';temp=pathlib.Path(json.loads((out/'probe-manifest.json').read_text())['temporary_directory']);folder=out/'paired-probes';folder.mkdir(exist_ok=True)
cases=[('fixture','probe-base','seed','TestDiagnosticFixtureBatch'),('git_proxy','baseline','helper','TestNativeDecisionAcceptanceIsSeparateFromApplication'),('dag_matrix','baseline','layered','TestTaskBatchRejectsDuplicateAndCrossProjectDependencies')]
reports=[]
for label,before,after,test in cases:
 records=[]
 for pair in range(3):
  for variant in ([before,after] if pair%2==0 else [after,before]):
   name=f'{label}-{pair+1}-{variant}';trace=folder/(name+'-git.jsonl');env=os.environ.copy();env['STEERLINE_DIAGNOSTIC_GIT_PROXY']=str(temp/'git-proxy.exe');env['GIT_TRACE2_EVENT']=str(trace.resolve())
   start=time.perf_counter();r=subprocess.run([str(temp/f'core-{variant}.exe'),'-test.run=^'+test+'$','-test.v','-test.count=1','-test.parallel=1','-test.timeout=120s'],cwd=root/'internal/steerline',env=env,stdout=subprocess.PIPE,stderr=subprocess.STDOUT,timeout=150)
   elapsed=time.perf_counter()-start;log=r.stdout.decode('utf-8',errors='replace');(folder/(name+'.log')).write_text(log,encoding='utf-8')
   starts=[]
   if trace.exists():
    for line in trace.read_text(encoding='utf-8').splitlines():
     e=json.loads(line)
     if e.get('event')=='start':starts.append(e)
   record={'case':label,'pair':pair+1,'variant':variant,'wall_seconds':elapsed,'exit_code':r.returncode,'git_process_starts':len(starts)};records.append(record);print(json.dumps(record),flush=True)
   if r.returncode or ('--- PASS: '+test) not in log:raise RuntimeError('probe failed: '+name+'\n'+log[-3000:])
 report={'case':label,'test':test,'before_variant':before,'after_variant':after,'before_median_seconds':statistics.median(x['wall_seconds'] for x in records if x['variant']==before),'after_median_seconds':statistics.median(x['wall_seconds'] for x in records if x['variant']==after),'runs':records}
 report['improvement_percent']=100*(1-report['after_median_seconds']/report['before_median_seconds']);reports.append(report)
 (folder/'summary.json').write_text(json.dumps(reports,indent=2)+'\n',encoding='utf-8')
print(json.dumps(reports),flush=True)
