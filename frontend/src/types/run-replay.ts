import type { SonarRun } from './sonar-run'

export type ReplayFlag = 'dropout' | 'overlap' | 'accuracy'

export interface RunReplay {
  id:number; run_id:number; algorithm_version:string; input_hash:string
  sample_count:number; dropped_points:number; dropout_events:number
  overlap_cells:number; overlap_ratio:number; accuracy_anomalies:number
  created_by:number; created_at:string
  run?:SonarRun
}

export interface ReplayFrameFeature {
  type:'Feature'
  properties:{ seq:number; cumulative_m:number; accuracy_m:number; flags:ReplayFlag[] }
  geometry:{ type:'Point'; coordinates:[number,number] }
}
export interface ReplayFramesCollection { type:'FeatureCollection'; features:ReplayFrameFeature[] }

export interface ReplayFramePoint { seq:number; x:number; y:number; cumulative_m:number; accuracy_m:number; flags:ReplayFlag[] }

export interface DropoutGap { after_seq:number; gap_m:number; estimated_missing:number }

export interface ReplaySummary {
  sample_count:number; track_length_m:number; duration_minutes:number
  dropped_points:number; dropout_events:number; dropout_gaps:DropoutGap[]
  covered_cells:number; overlap_cells:number; overlap_ratio:number
  accuracy_anomalies:number; accuracy_threshold_m:number; max_accuracy_m:number; resolution_m:number
}

export interface ReplaySnapshot { replay:RunReplay; frames:ReplayFramesCollection; summary:ReplaySummary }

export interface CreateReplayBody {
  run_id:number; algorithm_version?:string; expected_samples?:number
  dropout_factor?:number; accuracy_threshold_m?:number; overlap_resolution_m?:number
}
