import { afterEach, expect, it, vi } from "vitest";
import { cleanup, fireEvent, screen } from "@testing-library/react";
import { Api } from "../../shared/api/client";
import { projectQueries } from "../../shared/api/queries";
import { identity, run, testClient, mountFeature } from "../../test/workspace";
import { PlanTests, type PlanTestsView } from "./plan-tests";

afterEach(cleanup);
const view: PlanTestsView = {
  status:"ready", plan_path:"SteerlineFlow/plans/task.plan.md", total:1, passed:0, pending:1,
  rows:[{id:"rules",name:"规则验证",level:"L1",environment:"isolated",command:"test-rules",check:"断言通过",status:"pending"}],
};
it("TM displays only completion progress and preserves the last snapshot during sync errors", async () => {
  const api=new Api(identity,"csrf","home");vi.spyOn(api,"get").mockResolvedValue({...view,passed:1,pending:0,progress_version:3,collection_status:"unavailable",diagnostic:"进度同步异常：transport_unavailable",rows:view.rows.map(r=>({...r,status:"passed"}))});
  mountFeature(<PlanTests run={run}/>,api,testClient());
  expect(await screen.findByText("共 1 项 · 上次通过 1 · 未完成 0")).toBeVisible();
  expect(screen.getByText(/进度同步异常/)).toBeVisible();
  expect(screen.queryByText("report:original")).toBeNull();
  expect(screen.queryByRole("button",{name:/运行/})).toBeNull();
});
it("TM distinguishes an empty arrangement and missing/invalid documents", async () => {
  for(const [status,text] of [["ready","未安排自动测试"],["missing","Plan 未关联测试安排"],["invalid","测试安排无效"]]) {
    const api=new Api(identity,"csrf","home");vi.spyOn(api,"get").mockResolvedValue({...view,status,total:0,rows:[]});
    mountFeature(<PlanTests run={run}/>,api,testClient());
    expect(await screen.findByText(text)).toBeVisible();
    expect(screen.queryByText(/通过 0/)).toBeNull();cleanup();
  }
});
it("TM scopes cache to the exact project/candidate and does not query while offline", () => {
  const api=new Api(identity,"csrf","home");const get=vi.spyOn(api,"get");const client=testClient();
  client.setQueryData(projectQueries(api).planTests(run).queryKey,{...view,passed:1});
  mountFeature(<PlanTests run={run}/>,api,client,false);
  expect(screen.getByText(/上次通过 1/)).toBeVisible();expect(get).not.toHaveBeenCalled();
  const other=new Api({...identity,project_id:"other"},"csrf","other-home");
  expect(projectQueries(other).planTests(run).queryKey).not.toEqual(projectQueries(api).planTests(run).queryKey);
  expect(projectQueries(api).planTests({...run,run_id:"other"}).queryKey).not.toEqual(projectQueries(api).planTests(run).queryKey);
});

it("PMS keeps expired progress as history and explains old PR completion", async () => {
 const api=new Api(identity,"csrf","home");vi.spyOn(api,"get").mockResolvedValue({...view,material_status:"outdated",association_available:false,passed:0,rows:view.rows.map(r=>({...r,status:"stale"}))});
 mountFeature(<PlanTests run={run}/>,api,testClient());
 expect(await screen.findByText(/此候选 PR 合入仍可完成任务/)).toBeVisible();
 expect(screen.getByText(/历史测试安排/)).toBeVisible();
 expect(screen.queryByText(/共 1 项 · 通过/)).toBeNull();
 fireEvent.click(screen.getByText(/规则验证 · L1/));
 expect(screen.queryByText("report:original")).toBeNull();
});
