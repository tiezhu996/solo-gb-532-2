import PauseRounded from '@mui/icons-material/PauseRounded'
import PlayArrowRounded from '@mui/icons-material/PlayArrowRounded'
import RefreshRounded from '@mui/icons-material/RefreshRounded'
import { Alert, Box, Button, Chip, IconButton, LinearProgress, MenuItem, Select, Slider, Stack, TextField, Tooltip, Typography } from '@mui/material'
import { useEffect, useMemo, useState } from 'react'
import { PageHeader } from '../components/common/PageHeader'
import { ReplayCanvas } from '../components/common/ReplayCanvas'
import { RunStateBadge } from '../components/common/RunStateBadge'
import { useAuth } from '../hooks/useAuth'
import { useReplayStore } from '../stores/replay-store'
import type { CreateReplayBody, ReplayFlag } from '../types/run-replay'
import { REPLAY_FLAG_COLOR, REPLAY_FLAG_LABEL, nextFlagged, parseFrames } from '../utils/replay'

const flagOrder:ReplayFlag[]=['dropout','overlap','accuracy']

export function ReplayPage(){
  const{hasRole}=useAuth();const{runs,replays,snapshot,selectedRunId,loading,fetchRuns,fetchReplays,chooseRun,create,open}=useReplayStore()
  const[form,setForm]=useState({expected_samples:'',dropout_factor:'',accuracy_threshold_m:'',overlap_resolution_m:''})
  const[seq,setSeq]=useState(0);const[playing,setPlaying]=useState(false);const[speed,setSpeed]=useState(1)
  useEffect(()=>{void Promise.all([fetchRuns(),fetchReplays()])},[fetchRuns,fetchReplays])
  const frames=useMemo(()=>snapshot?parseFrames(snapshot.frames):[],[snapshot])
  const snapshotId=snapshot?.replay.id
  useEffect(()=>{setSeq(0);setPlaying(false)},[snapshotId])
  useEffect(()=>{if(!playing||frames.length<2)return;const timer=window.setInterval(()=>{setSeq(value=>{const next=value+speed;if(next>=frames.length-1){setPlaying(false);return frames.length-1}return next})},180);return()=>window.clearInterval(timer)},[playing,speed,frames.length])
  const selectedRun=runs.find(run=>run.id===selectedRunId)??snapshot?.replay.run??null
  const canProcess=hasRole('admin','data_processor')
  const summary=snapshot?.summary??null
  const currentFrame=frames[seq]??null
  const submit=async()=>{
    if(!selectedRun)return
    const body:CreateReplayBody={run_id:selectedRun.id}
    if(form.expected_samples)body.expected_samples=Number(form.expected_samples)
    if(form.dropout_factor)body.dropout_factor=Number(form.dropout_factor)
    if(form.accuracy_threshold_m)body.accuracy_threshold_m=Number(form.accuracy_threshold_m)
    if(form.overlap_resolution_m)body.overlap_resolution_m=Number(form.overlap_resolution_m)
    await create(body)
  }
  const jump=(flag:ReplayFlag)=>{const target=nextFlagged(frames,seq,flag);if(target>=0){setPlaying(false);setSeq(target)}}
  return <>
    <PageHeader eyebrow="RUN QUALITY REPLAY" title="运行质量回放" description="选择已处理运行生成固定回放快照，按采样序号回放轨迹、扫幅与质量标记，并汇总丢点、重复覆盖与精度异常。" actions={<Tooltip title="刷新"><IconButton aria-label="刷新回放" onClick={()=>void Promise.all([fetchRuns(),fetchReplays()])}><RefreshRounded/></IconButton></Tooltip>}/>
    {loading&&<LinearProgress/>}
    <Box className="replay-workspace">
      <Box className="run-list">
        <Box className="section-heading"><Typography variant="h6">已处理运行</Typography><Typography variant="caption">{runs.length} 次可回放</Typography></Box>
        {runs.map(run=><button type="button" key={run.id} className={selectedRunId===run.id&&!snapshot?'run-row active':'run-row'} onClick={()=>chooseRun(run.id)}><span><strong>{run.run_code}</strong><small>{run.transect_plan?.name??`PLAN #${run.transect_plan_id}`}</small></span><span><RunStateBadge state={run.run_state}/><small>{run.actual_swath_m} m 扫幅</small></span></button>)}
        {runs.length===0&&<Alert severity="info" sx={{m:1}}>暂无已处理运行，请先在航迹处理台完成处理。</Alert>}
        <Box className="section-heading"><Typography variant="h6">回放快照</Typography><Typography variant="caption">{replays.length} 份冻结</Typography></Box>
        {replays.map(item=><button type="button" key={item.id} className={snapshot?.replay.id===item.id?'run-row active':'run-row'} onClick={()=>void open(item.id)}><span><strong>{item.run?.run_code??`RUN #${item.run_id}`}</strong><small>{item.algorithm_version} · {item.sample_count} 采样</small></span><span><small>{new Date(item.created_at).toLocaleString()}</small><small>丢 {item.dropped_points} · 重 {(item.overlap_ratio*100).toFixed(1)}% · 异 {item.accuracy_anomalies}</small></span></button>)}
        {replays.length===0&&<Typography variant="caption" color="text.secondary" sx={{display:'block',p:1}}>尚未生成回放快照。</Typography>}
      </Box>
      <Box className="replay-detail">
        {snapshot&&summary&&<>
          <Box className="section-heading"><Box><Typography variant="overline">FROZEN REPLAY SNAPSHOT</Typography><Typography variant="h6">{snapshot.replay.run?.run_code??`RUN #${snapshot.replay.run_id}`} · 回放 #{snapshot.replay.id}</Typography></Box><Stack direction="row" gap={1} flexWrap="wrap"><Chip size="small" label={`算法 ${snapshot.replay.algorithm_version}`}/><Chip size="small" label={`哈希 ${snapshot.replay.input_hash.slice(0,12)}…`}/></Stack></Box>
          <ReplayCanvas boundary={selectedRun?.transect_plan?.survey_area?.boundary_geojson} track={selectedRun?.track_geojson} frames={frames} currentSeq={seq} swathM={selectedRun?.actual_swath_m??0}/>
          <Box className="replay-player">
            <IconButton aria-label={playing?'暂停':'播放'} color="primary" onClick={()=>{if(!playing&&seq>=frames.length-1)setSeq(0);setPlaying(value=>!value)}} disabled={frames.length<2}>{playing?<PauseRounded/>:<PlayArrowRounded/>}</IconButton>
            <Select size="small" value={speed} onChange={event=>setSpeed(Number(event.target.value))} sx={{minWidth:72}}>{[1,2,4].map(value=><MenuItem key={value} value={value}>{value}×</MenuItem>)}</Select>
            <Slider value={seq} min={0} max={Math.max(0,frames.length-1)} onChange={(_,value)=>{setPlaying(false);setSeq(Number(value))}} aria-label="采样序号跳转"/>
            <span className="replay-seq">采样 {seq} / {Math.max(0,frames.length-1)}</span>
            {flagOrder.map(flag=><Button key={flag} size="small" variant="outlined" sx={{borderColor:REPLAY_FLAG_COLOR[flag],color:REPLAY_FLAG_COLOR[flag]}} disabled={nextFlagged(frames,seq,flag)<0} onClick={()=>jump(flag)}>下一{REPLAY_FLAG_LABEL[flag]}</Button>)}
          </Box>
          {currentFrame&&<Box className="metric-strip">
            <div><span>当前坐标</span><strong>{currentFrame.x.toFixed(1)}, {currentFrame.y.toFixed(1)}</strong></div>
            <div><span>累计航程</span><strong>{currentFrame.cumulative_m.toFixed(0)} m</strong></div>
            <div><span>估算精度</span><strong>{currentFrame.accuracy_m.toFixed(1)} m</strong></div>
            <div><span>质量标记</span><strong>{currentFrame.flags.length?currentFrame.flags.map(flag=>REPLAY_FLAG_LABEL[flag]).join('、'):'正常'}</strong></div>
          </Box>}
          <Box className="metric-strip">
            <div><span>丢点</span><strong>{summary.dropped_points} 点 / {summary.dropout_events} 次</strong></div>
            <div><span>重复覆盖</span><strong>{summary.overlap_cells} 格 / {(summary.overlap_ratio*100).toFixed(1)}%</strong></div>
            <div><span>精度异常</span><strong>{summary.accuracy_anomalies} 点 / 峰 {summary.max_accuracy_m.toFixed(1)} m</strong></div>
            <div><span>采样总数</span><strong>{summary.sample_count} / {summary.track_length_m.toFixed(0)} m</strong></div>
          </Box>
          {summary.dropout_gaps.length>0&&<Box className="replay-gaps"><Typography variant="subtitle2">丢点区间</Typography>{summary.dropout_gaps.map(gap=><Alert key={gap.after_seq} severity="warning" sx={{mt:1}}>采样 {gap.after_seq} 之后中断 {gap.gap_m.toFixed(1)} m，估算缺失 {gap.estimated_missing} 个采样</Alert>)}</Box>}
          <Typography variant="caption" color="text.secondary">输入哈希 {snapshot.replay.input_hash} · 算法 {snapshot.replay.algorithm_version} · 网格 {summary.resolution_m} m · 冻结于 {new Date(snapshot.replay.created_at).toLocaleString()}。回放快照为离线复核证据，不构成控制指令。</Typography>
        </>}
        {!snapshot&&selectedRun&&<Box className="replay-create">
          <Box className="section-heading"><Box><Typography variant="overline">CREATE REPLAY SNAPSHOT</Typography><Typography variant="h6">{selectedRun.run_code}</Typography></Box><RunStateBadge state={selectedRun.run_state}/></Box>
          <Alert severity="info">将以默认参数生成回放：丢点阈值 2.5× 间距中位数、精度阈值 10 m、覆盖网格取测区目标分辨率。相同输入的重复回放会被拒绝，失败不会改动运行数据。</Alert>
          <Stack direction={{xs:'column',sm:'row'}} gap={2}>
            <TextField fullWidth type="number" label="声明采样数（可选）" value={form.expected_samples} onChange={event=>setForm({...form,expected_samples:event.target.value})} helperText="填写后与航迹实际采样数严格核对"/>
            <TextField fullWidth type="number" label="丢点阈值倍数（可选）" value={form.dropout_factor} onChange={event=>setForm({...form,dropout_factor:event.target.value})} helperText="默认 2.5，范围 1.5–10"/>
          </Stack>
          <Stack direction={{xs:'column',sm:'row'}} gap={2}>
            <TextField fullWidth type="number" label="精度阈值 m（可选）" value={form.accuracy_threshold_m} onChange={event=>setForm({...form,accuracy_threshold_m:event.target.value})} helperText="默认 10 m"/>
            <TextField fullWidth type="number" label="覆盖网格 m（可选）" value={form.overlap_resolution_m} onChange={event=>setForm({...form,overlap_resolution_m:event.target.value})} helperText="默认取测区目标分辨率"/>
          </Stack>
          {canProcess?<Button variant="contained" onClick={()=>void submit()} disabled={loading}>生成回放快照</Button>:<Alert severity="warning">当前角色只读，需数据处理员或管理员生成回放。</Alert>}
        </Box>}
        {!snapshot&&!selectedRun&&<Alert severity="info">从左侧选择一次已处理运行生成回放，或打开已冻结的回放快照。</Alert>}
      </Box>
    </Box>
  </>
}
