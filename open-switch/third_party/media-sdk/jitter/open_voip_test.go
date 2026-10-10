package jitter

import (
 "testing"
 "time"
 "github.com/pion/rtp"
)
func TestAudioBatchStartupWrapAndDuplicate(t *testing.T){
 delivered:=make(chan []ExtPacket,4)
 b:=NewBuffer(&testDepacketizer{},time.Second,func(p []ExtPacket){delivered<-p},WithStartupDelay(),WithBatchDelivery())
 defer b.Close()
 packet:=func(seq uint16)*rtp.Packet{return &rtp.Packet{Header:rtp.Header{SSRC:1,SequenceNumber:seq,Marker:true},Payload:append([]byte(nil),headerBytes...)}}
 b.Push(packet(0));b.Push(packet(65535));b.Push(packet(0))
 b.mu.Lock();b.startupUntil=time.Now().Add(-time.Second);b.mu.Unlock()
 b.Push(packet(1))
 select{case batch:=<-delivered:if len(batch)!=3||batch[0].SequenceNumber!=65535||batch[1].SequenceNumber!=0||batch[2].SequenceNumber!=1{t.Fatalf("audio batch lost order: %+v",batch)};default:t.Fatal("deadline did not deliver ready audio")}
 if b.Stats().PacketsDuplicate!=1{t.Fatal("duplicate not counted")}
 b.Close();b.Push(packet(2));if b.Size()!=0{t.Fatal("closed buffer accepted a packet")}
}
