import { apiClient } from './client'
import type { CreateReplayBody, ReplaySnapshot, RunReplay } from '../types/run-replay'

export const replayApi={
  list:()=>apiClient.page<RunReplay[]>('/replays?page_size=100'),
  get:(id:number)=>apiClient.get<ReplaySnapshot>(`/replays/${id}`),
  create:(body:CreateReplayBody)=>apiClient.post<ReplaySnapshot>('/replays',body)
}
