import {useState} from "react";
import {useQuery} from "@tanstack/react-query";
import {useWorkspace} from "../../app/context";
import {projectQueries} from "../../shared/api/queries";
export function TaskMaterials({id}:{id:string}){
 const {api,online}=useWorkspace();
 const [open,setOpen]=useState(false);
 const q=useQuery({...projectQueries(api).taskMaterials(id),enabled:online&&open,refetchInterval:online&&open?5000:false});
 return <section aria-label="任务最新文档">
  <button onClick={()=>setOpen(!open)} aria-expanded={open}>查看最新文档 / 测试安排</button>
  {open&&<div>
   <p className="muted">关联文档自动跟随远端；已有候选仍执行各自绑定的版本。</p>
   {q.error&&<p role="alert">{String(q.error)}</p>}
   {q.data?.commit&&<p>远端版本 {q.data.commit.slice(0,12)} · {q.data.status==="ready"?"测试安排已就绪":q.data.diagnostic||"材料或测试安排待就绪"}</p>}
   {q.data?.files?.map(f=><details key={f.path}><summary>{f.path}</summary>{f.diagnostic&&<p role="alert">{f.diagnostic}</p>}<pre>{f.body}</pre></details>)}
  </div>}
 </section>;
}
