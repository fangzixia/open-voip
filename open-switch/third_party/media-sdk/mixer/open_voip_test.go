package mixer

import (
 "testing"
 msdk "github.com/livekit/media-sdk"
)
func TestOwnedStemsAndReset(t *testing.T){
 var output msdk.PCM16Sample
 var stems map[*Input]msdk.PCM16Sample
 m:=newMixer(newTestWriter(&output,8000),160,WithInputBufferFrames(10),WithInputBufferMin(0),WithFrameHandler(nil,func(frame map[*Input]msdk.PCM16Sample){stems=frame}))
 defer m.Stop()
 a,b:=m.NewInput(),m.NewInput()
 short,full:=make(msdk.PCM16Sample,80),make(msdk.PCM16Sample,160)
 for i:=range short{short[i]=100};for i:=range full{full[i]=200}
 if e:=a.WriteSample(short);e!=nil{t.Fatal(e)};if e:=b.WriteSample(full);e!=nil{t.Fatal(e)}
 m.mixOnce()
 if len(stems[a])!=160||len(stems[b])!=160||stems[a][0]!=100||stems[a][159]!=0||stems[b][159]!=200{t.Fatal("stems not from the same complete tick")}
 saved:=stems[b];saved[0]=999
 _=a.WriteSample(full);a.Reset();m.mixOnce()
 if stems[a][0]!=0||stems[b][0]!=0||saved[0]!=999{t.Fatal("reset or frame ownership leaked samples")}
}
