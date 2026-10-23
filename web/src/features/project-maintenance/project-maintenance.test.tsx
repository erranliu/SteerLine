import {afterEach,expect,it,vi} from "vitest";
import {act,cleanup,fireEvent,render,screen,waitFor} from "@testing-library/react";
import {GenerationProgress} from "./generation-progress";
import {Api} from "../../shared/api/client";
import {identity,mountFeature,testClient} from "../../test/workspace";
import {ProjectMaintenance,type MaintenanceView} from "./project-maintenance";
afterEach(()=>{cleanup();vi.useRealTimers();sessionStorage.clear()});
function setup(overrides:Partial<MaintenanceView>={},online=true){
 const sha="a".repeat(40);
 const view:MaintenanceView={observation:{commit:sha,last_success:"2026-10-21T01:00:00Z"},remote_name:"origin/main",
 remote:{sha,title:"Update Plan",author:"Developer",time:"2026-10-21T00:00:00Z",url:"https://github.com/example/repo/commit/"+sha,prs:[]},
 local:{commit:"b".repeat(40),branch:"main",status:"behind",ahead:0,behind:2,can_update:true},
 tasks:[{task_id:"101",contract:{plan_ref:{path:"SteerlineFlow/plans/task.plan.md"}},status:"ready",generation_id:"generation"}],profiles:[{id:"local",name:"本机Codex"}],profile:"",...overrides};
 const api=new Api(identity,"csrf");
 const get=vi.spyOn(api,"get").mockImplementation(async(route)=>{
  if(route.startsWith("/maintenance/log"))return {commits:[view.remote],more:false} as never;
  if(route.startsWith("/maintenance/commits/"))return {...view.remote,prs:["https://github.com/example/repo/pull/9"]} as never;
  return view as never;
 });
 const submit=vi.spyOn(api,"submit").mockResolvedValue({status:"applied"} as never);
 mountFeature(<ProjectMaintenance/>,api,testClient(),online);
 return {api,get,submit,view};
}
it("PMS shows remote/local meaning and only the update workflow",async()=>{
 const {get,submit}=setup();
 fireEvent.click(await screen.findByRole("button",{name:/项目维护/}));
 expect(await screen.findByText(/远端基线 · origin\/main/)).toBeVisible();
 expect(screen.getByRole("heading",{name:/本地基线.*落后 2/})).toBeVisible();
 expect(screen.getByText(/Update Plan/)).toBeVisible();
 expect(screen.queryByLabelText("维护范围")).toBeNull();
 expect(screen.queryByRole("button",{name:"应用已审阅内容"})).toBeNull();
 expect(submit).not.toHaveBeenCalled();
 fireEvent.click(screen.getByRole("button",{name:"更新本地"}));
 await waitFor(()=>expect(submit).toHaveBeenCalledWith(expect.objectContaining({action:"maintenance_update",target_id:identity.project_id})));
 expect(get).toHaveBeenCalledWith("/maintenance");
});
it("PMS displays why update needs manual handling and disables unsafe update",async()=>{
 setup({local:{commit:"b".repeat(40),branch:"feature",status:"diverged",ahead:3,behind:2,can_update:false,reason:"本地存在独有提交，请人工处理"}});
 fireEvent.click(await screen.findByRole("button",{name:/项目维护/}));
 expect(await screen.findByRole("heading",{name:/本地基线.*已分叉 · 领先 3/})).toBeVisible();
 expect(screen.getByText(/请人工处理/)).toBeVisible();
 expect(screen.getByRole("button",{name:"更新本地"})).toBeDisabled();
});
it("PMS lets users browse log and real associated PR without mutation",async()=>{
 const {submit}=setup();
 fireEvent.click(await screen.findByRole("button",{name:/项目维护/}));
 fireEvent.click(screen.getByRole("button",{name:"查看提交日志"}));
 await screen.findByRole("region",{name:"提交日志"});
 fireEvent.click(screen.getAllByRole("button",{name:"提交详情 / 关联 PR"})[0]);
 expect(await screen.findByRole("link",{name:"https://github.com/example/repo/pull/9"})).toBeVisible();
 expect(submit).not.toHaveBeenCalled();
});
it("PMS exposes configuration and explicit failed retry only on problems",async()=>{
 const {submit}=setup({tasks:[{task_id:"101",contract:{plan_ref:{path:"SteerlineFlow/plans/task.plan.md"}},status:"configuration_required",generation_id:""}]});
 fireEvent.click(await screen.findByRole("button",{name:/项目维护/}));
 fireEvent.change(await screen.findByRole("combobox",{name:"测试生成方案"}),{target:{value:"local"}});
 fireEvent.click(screen.getByRole("button",{name:"保存测试生成方案"}));
 await waitFor(()=>expect(submit).toHaveBeenCalledWith(expect.objectContaining({action:"maintenance_profile",payload:{profile:"local"}})));
});
it("PMS offline view retains facts and does not submit or check automatically",async()=>{
 const {get,submit}=setup({},false);
 fireEvent.click(screen.getByRole("button",{name:/项目维护/}));
 expect(screen.getByRole("button",{name:"检查更新"})).toBeDisabled();
 expect(screen.getByText(/离线/)).toBeVisible();
 expect(get).not.toHaveBeenCalled();expect(submit).not.toHaveBeenCalled();
});

it("GQ keeps progress in the collapsed title and submits one bulk command",async()=>{
 const {submit}=setup({tasks:[{task_id:"101",contract:{plan_ref:{path:"SteerlineFlow/plans/task.plan.md"}},status:"accepted",generation_id:"generation"}],generation_progress:{id:"batch",total:9,succeeded:8,running:1,queued:0,failed:0,blocked:0}});
 expect(await screen.findByText("8/9 生成中")).toBeVisible();
 const toggle=screen.getByRole("button",{name:/项目维护/});
 expect(toggle).toHaveAttribute("aria-expanded","false");
 expect(screen.getByLabelText("正在生成")).toBeVisible();
 fireEvent.click(toggle);
 fireEvent.click(screen.getByRole("button",{name:"一键生成"}));
 await waitFor(()=>expect(submit).toHaveBeenCalledWith(expect.objectContaining({action:"maintenance_generate_all",target_id:identity.project_id,payload:{}})));
 fireEvent.click(toggle);
 expect(screen.getByText("8/9 生成中")).toBeVisible();
 expect(submit).toHaveBeenCalledTimes(1);
});

it("GQ hides success after five seconds using the backend completion timestamp",()=>{
 vi.useFakeTimers();vi.setSystemTime(new Date("2026-10-22T00:00:00Z"));
 const value={id:"batch",total:9,succeeded:9,queued:0,running:0,failed:0,blocked:0,completed_at:new Date().toISOString()};
 const mounted=render(<GenerationProgress value={value} online/>);
 expect(screen.getByText("9/9 生成完毕")).toBeVisible();
 act(()=>vi.advanceTimersByTime(4999));
 expect(screen.getByText("9/9 生成完毕")).toBeVisible();
 act(()=>vi.advanceTimersByTime(1));
 expect(screen.queryByRole("status")).toBeNull();
 mounted.unmount();
 render(<GenerationProgress value={value} online/>);
 expect(screen.queryByRole("status")).toBeNull();
});

it("GQ retains failures and stops the spinner for pending verification",()=>{
 vi.useFakeTimers();
 render(<GenerationProgress value={{id:"batch",total:9,succeeded:8,queued:0,running:0,failed:1,blocked:0}} online/>);
 act(()=>vi.advanceTimersByTime(60000));
 expect(screen.getByText("8/9 已生成，1 项失败")).toBeVisible();
 expect(screen.queryByLabelText("正在生成")).toBeNull();
});

it("GQ polling delay does not shorten the five-second notice or reset it on remount",()=>{
 vi.useFakeTimers();vi.setSystemTime(new Date("2026-10-22T00:00:00Z"));
 const active={id:"delayed",total:1,succeeded:0,queued:0,running:1,failed:0,blocked:0};
 const mounted=render(<GenerationProgress value={active} online/>);
 const completed={...active,succeeded:1,running:0,completed_at:new Date().toISOString()};
 act(()=>vi.advanceTimersByTime(8000));
 mounted.rerender(<GenerationProgress value={completed} online/>);
 expect(screen.getByText("1/1 生成完毕")).toBeVisible();
 act(()=>vi.advanceTimersByTime(2000));
 mounted.unmount();
 render(<GenerationProgress value={completed} online/>);
 act(()=>vi.advanceTimersByTime(2999));
 expect(screen.getByText("1/1 生成完毕")).toBeVisible();
 act(()=>vi.advanceTimersByTime(1));
 expect(screen.queryByRole("status")).toBeNull();
});
