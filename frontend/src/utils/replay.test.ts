import { describe, expect, it } from 'vitest'
import type { ReplayFramesCollection } from '../types/run-replay'
import { REPLAY_FLAG_COLOR, REPLAY_NORMAL_COLOR, frameColor, nextFlagged, parseFrames } from './replay'

const collection:ReplayFramesCollection={
  type:'FeatureCollection',
  features:[
    {type:'Feature',properties:{seq:2,cumulative_m:20,accuracy_m:1.5,flags:['overlap']},geometry:{type:'Point',coordinates:[20,0]}},
    {type:'Feature',properties:{seq:0,cumulative_m:0,accuracy_m:1.5,flags:[]},geometry:{type:'Point',coordinates:[0,0]}},
    {type:'Feature',properties:{seq:1,cumulative_m:10,accuracy_m:12.5,flags:['accuracy','overlap']},geometry:{type:'Point',coordinates:[10,0]}},
    {type:'Feature',properties:{seq:3,cumulative_m:130,accuracy_m:1.5,flags:['dropout']},geometry:{type:'Point',coordinates:[130,0]}}
  ]
}

describe('replay playback helpers',()=>{
  it('parses frozen frames sorted by sample sequence',()=>{
    const frames=parseFrames(collection)
    expect(frames.map(frame=>frame.seq)).toEqual([0,1,2,3])
    expect(frames[3]?.x).toBe(130)
  })
  it('prioritizes dropout over accuracy over overlap for marker color',()=>{
    const frames=parseFrames(collection)
    expect(frameColor(frames[0]!)).toBe(REPLAY_NORMAL_COLOR)
    expect(frameColor(frames[1]!)).toBe(REPLAY_FLAG_COLOR.accuracy)
    expect(frameColor(frames[2]!)).toBe(REPLAY_FLAG_COLOR.overlap)
    expect(frameColor(frames[3]!)).toBe(REPLAY_FLAG_COLOR.dropout)
  })
  it('finds the next flagged sample after the current position',()=>{
    const frames=parseFrames(collection)
    expect(nextFlagged(frames,0,'accuracy')).toBe(1)
    expect(nextFlagged(frames,1,'dropout')).toBe(3)
    expect(nextFlagged(frames,1,'overlap')).toBe(2)
    expect(nextFlagged(frames,2,'overlap')).toBe(-1)
    expect(nextFlagged(frames,3,'dropout')).toBe(-1)
  })
})
