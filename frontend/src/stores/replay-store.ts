import { create } from 'zustand'
import { replayApi } from '../api/run-replay'
import { sonarRunApi } from '../api/sonar-run'
import type { CreateReplayBody, ReplaySnapshot, RunReplay } from '../types/run-replay'
import type { SonarRun } from '../types/sonar-run'

interface ReplayStore{
  runs:SonarRun[];replays:RunReplay[];snapshot:ReplaySnapshot|null;selectedRunId:number|null;loading:boolean
  fetchRuns:()=>Promise<void>;fetchReplays:()=>Promise<void>
  chooseRun:(id:number|null)=>void;create:(body:CreateReplayBody)=>Promise<void>;open:(id:number)=>Promise<void>;closeSnapshot:()=>void
}
export const useReplayStore=create<ReplayStore>((set)=>({
  runs:[],replays:[],snapshot:null,selectedRunId:null,loading:false,
  fetchRuns:async()=>{const response=await sonarRunApi.list();set({runs:response.data.filter(run=>run.run_state==='processed')})},
  fetchReplays:async()=>{const response=await replayApi.list();set({replays:response.data})},
  chooseRun:(selectedRunId)=>{set({selectedRunId,snapshot:null})},
  create:async(body)=>{set({loading:true});try{const response=await replayApi.create(body);set(state=>({snapshot:response.data,replays:state.replays.some(item=>item.id===response.data.replay.id)?state.replays:[response.data.replay,...state.replays]}))}finally{set({loading:false})}},
  open:async(id)=>{set({loading:true});try{const response=await replayApi.get(id);set({snapshot:response.data,selectedRunId:response.data.replay.run_id})}finally{set({loading:false})}},
  closeSnapshot:()=>set({snapshot:null})
}))
