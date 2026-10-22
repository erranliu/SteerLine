import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { useWorkspace } from "../../app/context";
import type { Candidate } from "../../shared/api/types";
import { projectQueries } from "../../shared/api/queries";

export function CandidateDocuments({run}:{run:Candidate}){
 const {api,online}=useWorkspace();
 const [open,setOpen]=useState(false);
 const view=useQuery({...projectQueries(api).candidateDocuments(run.run_id),enabled:online&&open,refetchInterval:online&&open?5000:false});
 return <section aria-label="候选文档" className="settings-panel">
  {run.material_status==="outdated"&&<p className="notice amber">已过期 · 保留候选。原执行继续保留，此候选 PR 合入仍可完成任务。</p>}
  {run.material_status==="unknown"&&<p className="muted">绑定文档当前性待核实</p>}
  <button onClick={()=>setOpen(!open)} aria-expanded={open}>执行文档 / 最新文档 / 查看差异</button>
  {open&&<div>
   {view.error&&<p role="alert">{String(view.error)}</p>}
   {view.data&&<p className="muted">执行版本 {view.data.commit.slice(0,12)} · 远端版本 {view.data.latest_commit.slice(0,12)}</p>}
   {view.data?.files.map(f=><details key={f.path}><summary>{f.path} · {f.before===f.after?"内容一致":"内容已变化"}</summary>
    {f.diagnostic&&<p role="alert">{f.diagnostic}</p>}
    <h4>执行绑定内容</h4><pre>{f.before||"此版本无内容"}</pre>
    <h4>远端最新内容</h4><pre>{f.after||"此版本无内容"}</pre>
   </details>)}
  </div>}
 </section>;
}
