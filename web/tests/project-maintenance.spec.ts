import {test,expect} from "@playwright/test";
import {execFileSync} from "node:child_process";
import {readFileSync,writeFileSync,appendFileSync} from "node:fs";
import {dirname,resolve,join} from "node:path";

test("runtime.project-maintenance: automatic materials, independent candidates and old PR delivery",async({page},info)=>{
 test.setTimeout(300000);page.setDefaultTimeout(30000);
 const config=process.env.STEERLINE_WEB_CONFIG;
 if(!config)throw new Error("fresh debug configuration required");
 const root=resolve(dirname(config),"../..");
 const manifest=JSON.parse(readFileSync(join(root,"manifest.json"),"utf8"));
 const scheme=JSON.parse(readFileSync(join(root,"scheme.json"),"utf8"));
 if(!root.toLowerCase().startsWith("c:\\steerlinedebug\\")||!manifest.project_maintenance||scheme.scheme!=="simulate")throw new Error("fresh controlled package required");
 const repo=join(root,"projects","SteerlineTest"),remote=join(root,"remotes","erranliu","SteerlineTest.git");
 const git=(...args:string[])=>execFileSync("git",["-C",repo,...args],{encoding:"utf8",timeout:30000}).trim();
 const helper=(...args:string[])=>execFileSync(join(root,"bin","debug-helper.exe"),["debug",...args],{encoding:"utf8",timeout:60000});
 const opening=execFileSync(join(root,"bin","steerline-web.exe"),["open","-print-url"],{env:{...process.env,STEERLINE_HOME:join(root,"home")},encoding:"utf8",timeout:30000}).trim();
 await page.goto(opening);
 await expect(page.getByRole("combobox",{name:"当前项目"})).toBeVisible();
 const catalog=await(await page.request.get(new URL("/api/v2/application/projects",page.url()).href)).json();
 const primary=catalog.projects.find((p:any)=>p.config_id==="SteerlineTest.json");
 const other=catalog.projects.find((p:any)=>p.project_id!==primary.project_id);
 const project=primary.project_id,prefix="/api/v2/application/projects/"+project;
 await page.getByRole("combobox",{name:"当前项目"}).selectOption(project);
 const get=async(path:string)=>{const r=await page.request.get(new URL(prefix+path,page.url()).href);expect(r.ok(),await r.text()).toBeTruthy();return r.json()};
 const otherGet=async()=> (await page.request.get(new URL("/api/v2/application/projects/"+other.project_id+"/maintenance",page.url()).href)).json();
 const call=async(action:string,target:string,payload:object={})=>page.evaluate(async({prefix,action,target,payload})=>{
  const auth=await(await fetch("/auth/status")).json();
  const identity=await(await fetch(prefix+"/identity")).json();
  const r=await fetch(prefix+"/commands",{method:"POST",headers:{"Content-Type":"application/json","X-CSRF-Token":auth.csrf_token},body:JSON.stringify({request_id:crypto.randomUUID().replaceAll("-",""),project_id:identity.project_id,instance_id:identity.instance_id,action,target_id:target,expected_version:1,payload})});
  if(!r.ok)throw new Error(await r.text());return r.json();
 },{prefix,action,target,payload});
 const section=page.getByRole("region",{name:"项目维护"});
 await section.getByRole("button",{name:/项目维护/}).click();
 await section.getByRole("button",{name:"检查更新",exact:true}).click();
 const ready=async()=>{
  await expect.poll(async()=>{const v=await get("/maintenance");return v.tasks.find((t:any)=>t.task_id==="1")?.status},{timeout:90000}).toBe("ready");
  return get("/maintenance");
 };
 const initial=await ready();
 expect(initial.remote.sha).toBeTruthy();
 expect(initial.remote.time).toBeTruthy();
 expect(initial.tasks.find((t:any)=>t.task_id==="1").generation_id).toBeTruthy();
 // Establish the other project's completed initial state before changing this
 // project. Task/material observations are asynchronous after task creation.
 await expect.poll(async()=>{
  const value=await otherGet();
  return value.tasks.length===2&&value.tasks.every((t:any)=>t.status==="ready"&&t.generation_id);
 },{timeout:90000}).toBeTruthy();
 const originalOther=await otherGet();
 await expect(section.getByLabel("维护范围")).toHaveCount(0);
 await expect(section.getByRole("button",{name:"应用已审阅内容"})).toHaveCount(0);
 const arrangement=join(repo,"SteerlineFlow","plans","task.tests.json");
 const beforeArrangement=readFileSync(arrangement,"utf8");
 // The package is isolated. Advance its bare remote, leaving local one commit behind.
 git("checkout",manifest.base_branch);
 const old=git("rev-parse","HEAD");
 git("commit","--allow-empty","-qm","Source-only remote update");
 const latest=git("rev-parse","HEAD");
 git("push",remote,"HEAD:refs/heads/"+manifest.base_branch);
 git("reset","--hard",old);
 await section.getByRole("button",{name:"检查更新",exact:true}).click();
 await expect(section).toContainText("落后 1 个提交");
 await section.getByRole("button",{name:"更新本地",exact:true}).click();
 await expect.poll(()=>git("rev-parse","HEAD")).toBe(latest);
 await section.getByRole("button",{name:"查看提交日志",exact:true}).click();
 await expect(page.getByRole("region",{name:"提交日志"})).toContainText("Source-only remote update");
 helper("control","--simulate","-config",config,"-action","resume_dispatch");
 const start=(action:string)=>helper("control","--simulate","-config",config,"-action",action,"-task-id","1","-profile","simulate");
 const runs=async()=>{const v=await get("/snapshot");return v.connection.snapshot.page.runs.filter((c:any)=>(c.domain_task_id||c.task_id)==="1")};
 const waitRun=async(count:number)=>{
  await expect.poll(async()=> (await runs()).filter((r:any)=>r.execution_id&&r.pr?.url).length,{timeout:60000}).toBe(count);
  return (await runs()).sort((a:any,b:any)=>a.candidate_number-b.candidate_number);
 };
 start("start_task");
 const first=(await waitRun(1))[0];
 const original=await get("/runs/"+first.run_id+"/tests");
 expect(original.total).toBe(1);expect(original.material_status).toBe("current");
 expect(original.commit).toBe(latest);
 const changePlan=async(label:string)=>{
  git("checkout",manifest.base_branch);
  appendFileSync(join(repo,"SteerlineFlow", "plans", "task.plan.md"),"\n"+label+"\n");
  git("add","SteerlineFlow/plans/task.plan.md");git("commit","-qm",label);git("push",remote,"HEAD:refs/heads/"+manifest.base_branch);
  expect((await call("maintenance_check",project)).status).toBe("applied");
  return ready();
 };
 const secondMaterial=await changePlan("Plan version two");
 const secondGeneration=secondMaterial.tasks.find((t:any)=>t.task_id==="1").generation_id;
 expect(secondGeneration).not.toBe(initial.tasks.find((t:any)=>t.task_id==="1").generation_id);
 const expired=await get("/runs/"+first.run_id+"/tests");
 expect(expired.material_status).toBe("outdated");expect(expired.association_available).toBe(false);expect(expired.commit).toBe(latest);
 start("start_candidate");
 const second=(await waitRun(2))[1];
 start("start_candidate");
 const third=(await waitRun(3))[2];
 const secondTests=await get("/runs/"+second.run_id+"/tests"),thirdTests=await get("/runs/"+third.run_id+"/tests");
 expect(secondTests.commit).toBe(secondMaterial.observation.commit);
 expect(secondTests.association_available).toBe(true);expect(thirdTests.association_available).toBe(true);
 const diff=await get("/runs/"+first.run_id+"/documents");
 expect(diff.files.find((f:any)=>f.path==="SteerlineFlow/plans/task.plan.md").after).toContain("Plan version two");
 expect(diff.files.find((f:any)=>f.path==="SteerlineFlow/plans/task.plan.md").before).not.toContain("Plan version two");
 const thirdMaterial=await changePlan("Plan version three");
 expect(thirdMaterial.tasks.find((t:any)=>t.task_id==="1").generation_id).not.toBe(secondGeneration);
 expect((await get("/runs/"+third.run_id+"/tests")).association_available).toBe(false);
 expect(readFileSync(arrangement,"utf8")).toBe(beforeArrangement);
 // The last prepared PR now belongs to an outdated candidate. Its real merge
 // remains sufficient for TaskDelivery, even without a candidate on version three.
 await page.goto(new URL("/projects/"+project+"/runs?task=1&run="+third.run_id,page.url()).href);
 await expect(page.getByRole("region",{name:"计划测试"})).toContainText("当前测试关联不可用");
 await page.getByRole("button",{name:"执行文档 / 最新文档 / 查看差异"}).click();
 await expect(page.getByRole("region",{name:"候选文档"})).toContainText("已过期");
 await page.screenshot({path:info.outputPath("retained-candidate.png"),fullPage:true});
 helper("merge","--simulate");
 await expect.poll(async()=>{const tasks=await get("/tasks");return tasks.tasks.find((t:any)=>t.task_id==="1")?.status},{timeout:60000}).toBe("closed");
 const taskPage=await get("/tasks");
 const delivered=taskPage.tasks.find((t:any)=>t.task_id==="1");
 expect(delivered.delivery.workflow_run_id).toBe(third.run_id);
 const finalOther=await otherGet();
 expect(finalOther.observation.commit).toBe(originalOther.observation.commit);
 expect(finalOther.tasks.map((t:any)=>t.generation_id)).toEqual(originalOther.tasks.map((t:any)=>t.generation_id));
 writeFileSync(info.outputPath("facts.json"),JSON.stringify({project,first,second,third,initial,secondMaterial,thirdMaterial,original,expired,diff,delivered},null,2));
});
