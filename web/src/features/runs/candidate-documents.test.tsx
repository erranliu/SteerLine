import {afterEach,expect,it,vi} from "vitest";
import {cleanup,fireEvent,screen} from "@testing-library/react";
import {Api} from "../../shared/api/client";
import {identity,run,testClient,mountFeature} from "../../test/workspace";
import {CandidateDocuments} from "./candidate-documents";
import {TaskMaterials} from "../tasks/task-materials";
afterEach(cleanup);
it("PMS loads full candidate before/after documents only on demand",async()=>{
 const api=new Api(identity,"csrf","home");const get=vi.spyOn(api,"get").mockResolvedValue({status:"outdated",commit:"old",latest_commit:"new",files:[{path:"task.plan.md",before:"Full original Plan",after:"Full latest Plan"}]});
 mountFeature(<CandidateDocuments run={{...run,material_status:"outdated"}}/>,api,testClient());
 expect(get).not.toHaveBeenCalled();
 expect(screen.getByText(/此候选 PR 合入仍可完成任务/)).toBeVisible();
 fireEvent.click(screen.getByRole("button",{name:"执行文档 / 最新文档 / 查看差异"}));
 fireEvent.click(await screen.findByText(/task.plan.md/));
 expect(screen.getByText("Full original Plan")).toBeVisible();expect(screen.getByText("Full latest Plan")).toBeVisible();
 expect(get).toHaveBeenCalledWith("/runs/"+run.run_id+"/documents",expect.any(AbortSignal));
});
it("PMS latest task documents are read-only and expose source errors",async()=>{
 const api=new Api(identity,"csrf","home");const get=vi.spyOn(api,"get").mockResolvedValue({commit:"new",status:"invalid",diagnostic:"Plan missing",files:[{path:"task.plan.md",body:"",diagnostic:"File deleted"}]});
 mountFeature(<TaskMaterials id="101"/>,api,testClient());expect(get).not.toHaveBeenCalled();
 fireEvent.click(screen.getByRole("button",{name:"查看最新文档 / 测试安排"}));
 fireEvent.click(await screen.findByText("task.plan.md"));expect(screen.getByRole("alert")).toHaveTextContent("File deleted");
 expect(get).toHaveBeenCalledWith("/maintenance/tasks/101",expect.any(AbortSignal));expect(screen.queryByRole("button",{name:/确认|应用/})).toBeNull();
});
