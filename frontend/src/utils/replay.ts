import type { ReplayFlag, ReplayFramePoint, ReplayFramesCollection } from '../types/run-replay'

export const REPLAY_FLAG_LABEL:Record<ReplayFlag,string>={dropout:'丢点',overlap:'重复覆盖',accuracy:'精度异常'}
export const REPLAY_FLAG_COLOR:Record<ReplayFlag,string>={dropout:'#b8443c',overlap:'#d39a20',accuracy:'#7b5ea7'}
export const REPLAY_NORMAL_COLOR='#2c7873'

// parseFrames 把冻结的 GeoJSON 快照帧整理为按采样序号升序的播放帧。
export function parseFrames(collection:ReplayFramesCollection):ReplayFramePoint[]{
  return collection.features.map(feature=>({
    seq:feature.properties.seq,
    x:feature.geometry.coordinates[0],
    y:feature.geometry.coordinates[1],
    cumulative_m:feature.properties.cumulative_m,
    accuracy_m:feature.properties.accuracy_m,
    flags:feature.properties.flags??[]
  })).sort((a,b)=>a.seq-b.seq)
}

// frameColor 按 丢点 > 精度异常 > 重复覆盖 的优先级取标记色。
export function frameColor(frame:ReplayFramePoint):string{
  const priority:ReplayFlag[]=['dropout','accuracy','overlap']
  for(const flag of priority)if(frame.flags.includes(flag))return REPLAY_FLAG_COLOR[flag]
  return REPLAY_NORMAL_COLOR
}

// nextFlagged 从 from 之后查找下一个带指定质量标记的采样序号，找不到返回 -1。
export function nextFlagged(frames:ReplayFramePoint[],from:number,flag:ReplayFlag):number{
  for(let index=Math.max(0,from+1);index<frames.length;index++){const frame=frames[index];if(frame&&frame.flags.includes(flag))return index}
  return -1
}
