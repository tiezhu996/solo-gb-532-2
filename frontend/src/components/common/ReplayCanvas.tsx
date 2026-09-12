import { useEffect, useRef } from 'react'
import { Box } from '@mui/material'
import type { GeoJSONFeature, Position } from '../../types/api'
import type { ReplayFramePoint } from '../../types/run-replay'
import { geometryBounds, geometryLines } from '../../utils/geometry'
import { frameColor } from '../../utils/replay'

// ReplayCanvas 按采样序号渐进渲染回放帧：已播放段着色质量标记，未播放段灰显，
// 当前采样叠加扫幅圈。与 SurveyCanvas 一样只使用本地 Canvas，不依赖在线地图。
export function ReplayCanvas({boundary,track,frames,currentSeq,swathM}:{boundary?:GeoJSONFeature|null;track?:GeoJSONFeature|null;frames:ReplayFramePoint[];currentSeq:number;swathM:number}){
  const canvasRef=useRef<HTMLCanvasElement>(null);const hostRef=useRef<HTMLDivElement>(null)
  useEffect(()=>{
    const canvas=canvasRef.current,host=hostRef.current;if(!canvas||!host)return
    const draw=()=>{
      const rect=host.getBoundingClientRect();const ratio=Math.min(window.devicePixelRatio||1,2);canvas.width=Math.max(1,Math.round(rect.width*ratio));canvas.height=Math.max(1,Math.round(rect.height*ratio));canvas.style.width=`${rect.width}px`;canvas.style.height=`${rect.height}px`
      const context=canvas.getContext('2d');if(!context)return;context.setTransform(ratio,0,0,ratio,0,0);const width=rect.width,height=rect.height
      context.fillStyle='#edf4f3';context.fillRect(0,0,width,height);context.strokeStyle='#d2dfdd';context.lineWidth=1
      for(let x=24;x<width;x+=48){context.beginPath();context.moveTo(x,0);context.lineTo(x,height);context.stroke()}for(let y=24;y<height;y+=48){context.beginPath();context.moveTo(0,y);context.lineTo(width,y);context.stroke()}
      const features:GeoJSONFeature[]=[];if(boundary)features.push(boundary);if(track)features.push(track)
      let bounds=geometryBounds(features)
      if(!bounds&&frames.length>0){const xs=frames.map(frame=>frame.x),ys=frames.map(frame=>frame.y);bounds={minX:Math.min(...xs),minY:Math.min(...ys),maxX:Math.max(...xs),maxY:Math.max(...ys),pointCount:frames.length}}
      if(!bounds){context.fillStyle='#617475';context.font='14px sans-serif';context.fillText('等待回放快照',24,36);return}
      const margin=swathM/2;let minX=bounds.minX-margin,maxX=bounds.maxX+margin,minY=bounds.minY-margin,maxY=bounds.maxY+margin
      if(maxX===minX){maxX+=1;minX-=1}if(maxY===minY){maxY+=1;minY-=1}
      const padding=32,scale=Math.min((width-padding*2)/(maxX-minX),(height-padding*2)/(maxY-minY));const project=(point:Position):Position=>[padding+(point[0]-minX)*scale,height-padding-(point[1]-minY)*scale]
      if(boundary){context.strokeStyle='#185b63';context.fillStyle='rgba(34, 122, 126, 0.08)';context.lineWidth=2.2;context.setLineDash([]);geometryLines(boundary).forEach(line=>{if(!line.length)return;context.beginPath();line.forEach((point,index)=>{const[x,y]=project(point);if(index===0)context.moveTo(x,y);else context.lineTo(x,y)});context.closePath();context.fill();context.stroke()})}
      if(track){context.strokeStyle='#b9c8c6';context.lineWidth=1.5;context.setLineDash([5,4]);geometryLines(track).forEach(line=>{if(!line.length)return;context.beginPath();line.forEach((point,index)=>{const[x,y]=project(point);if(index===0)context.moveTo(x,y);else context.lineTo(x,y)});context.stroke()});context.setLineDash([])}
      const played=frames.filter(frame=>frame.seq<=currentSeq),waiting=frames.filter(frame=>frame.seq>currentSeq)
      context.fillStyle='#c3d0ce';waiting.forEach(frame=>{const[x,y]=project([frame.x,frame.y]);context.beginPath();context.arc(x,y,2.4,0,Math.PI*2);context.fill()})
      for(let index=1;index<played.length;index++){const previous=played[index-1],current=played[index];if(!previous||!current)continue;const[x1,y1]=project([previous.x,previous.y]);const[x2,y2]=project([current.x,current.y]);context.strokeStyle=frameColor(current);context.lineWidth=2.4;context.beginPath();context.moveTo(x1,y1);context.lineTo(x2,y2);context.stroke()}
      played.forEach(frame=>{const[x,y]=project([frame.x,frame.y]);context.fillStyle=frameColor(frame);context.beginPath();context.arc(x,y,3.4,0,Math.PI*2);context.fill()})
      const current=frames.find(frame=>frame.seq===currentSeq)
      if(current){const[x,y]=project([current.x,current.y]);context.strokeStyle='#123f42';context.lineWidth=1.6;context.setLineDash([4,3]);context.beginPath();context.arc(x,y,Math.max(4,(swathM/2)*scale),0,Math.PI*2);context.stroke();context.setLineDash([]);context.fillStyle='#123f42';context.beginPath();context.arc(x,y,5.6,0,Math.PI*2);context.fill();context.strokeStyle='#fbfcfa';context.lineWidth=2;context.beginPath();context.arc(x,y,5.6,0,Math.PI*2);context.stroke()}
      context.fillStyle='#415b5c';context.font='11px ui-monospace, monospace';context.fillText(`${minX.toFixed(0)} m`,padding,height-10);context.textAlign='right';context.fillText(`${maxX.toFixed(0)} m`,width-padding,height-10);context.textAlign='left'
    }
    draw();const observer=new ResizeObserver(draw);observer.observe(host);return()=>observer.disconnect()
  },[boundary,track,frames,currentSeq,swathM])
  return <Box ref={hostRef} className="survey-canvas replay-canvas" role="img" aria-label="运行质量回放画布"><canvas ref={canvasRef}/></Box>
}
