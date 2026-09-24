package media

import (
	"context"
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/pion/interceptor"
	"github.com/pion/rtp"
	"github.com/pion/webrtc/v4"
	"github.com/pion/webrtc/v4/pkg/media"
	"github.com/pion/webrtc/v4/pkg/media/ivfwriter"
	"github.com/pion/webrtc/v4/pkg/media/oggwriter"

	"open-switch/internal/config"
	"open-switch/internal/errs"
	"open-switch/internal/ports"
	"open-switch/internal/ports/dto"
)

type peer struct {
	pc         *webrtc.PeerConnection
	audioOut   *webrtc.TrackLocalStaticRTP
	videoOut   *webrtc.TrackLocalStaticRTP
	audioSamp  *webrtc.TrackLocalStaticSample
	role       dto.LegRole
	audioMuted bool
	videoMuted bool
	held       bool
}

type room struct {
	mu          sync.RWMutex
	enableVideo bool
	sipAudio    bool
	sipRTP      map[*sipRTP]struct{}
	peers       map[string]*peer
	dtmf        map[string]ports.DTMFHandler
	rec         *recorder
}

type recorder struct {
	id        string
	callID    string
	path      string
	audioPath string
	videoPath string
	mode      string
	file      *os.File
	ogg       *oggwriter.OggWriter
	ivf       *ivfwriter.IVFWriter
	pcm       *pcmMix
	mu        sync.Mutex
	bytes     int64
	ended     *time.Time
	started   time.Time
	retainTo  *time.Time
}

// Options 媒体层启动选项。
type Options struct {
	ICE           config.ICEConfig
	TURN          config.TURNConfig
	RecordingsDir string
	SIP           config.SIPConfig
}

// Service 实现 MediaPort 的进程内 SFU。
type Service struct {
	api           *webrtc.API
	apiPCMU       *webrtc.API
	ice           []webrtc.ICEServer
	turn          config.TURNConfig
	recDir        string
	sip           *sipUA
	mu            sync.Mutex
	rooms         map[string]*room
	recByID       map[string]*recorder
	sipPending    map[string]bool
	sipRTPPending map[string]map[*sipRTP]struct{}
}

// NewService 根据 ICE/TURN/录音目录创建媒体服务。
func NewService(opt Options) (*Service, error) {
	me := &webrtc.MediaEngine{}
	if err := me.RegisterCodec(webrtc.RTPCodecParameters{
		RTPCodecCapability: webrtc.RTPCodecCapability{
			MimeType: webrtc.MimeTypeOpus, ClockRate: 48000, Channels: 2, SDPFmtpLine: "minptime=10;useinbandfec=1",
		},
		PayloadType: 111,
	}, webrtc.RTPCodecTypeAudio); err != nil {
		return nil, err
	}
	if err := me.RegisterCodec(webrtc.RTPCodecParameters{
		RTPCodecCapability: webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypePCMU, ClockRate: 8000},
		PayloadType:        0,
	}, webrtc.RTPCodecTypeAudio); err != nil {
		return nil, err
	}
	if err := me.RegisterCodec(webrtc.RTPCodecParameters{
		RTPCodecCapability: webrtc.RTPCodecCapability{MimeType: "audio/telephone-event", ClockRate: 8000},
		PayloadType:        101,
	}, webrtc.RTPCodecTypeAudio); err != nil {
		return nil, err
	}
	if err := me.RegisterCodec(webrtc.RTPCodecParameters{
		RTPCodecCapability: webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeVP8, ClockRate: 90000},
		PayloadType:        96,
	}, webrtc.RTPCodecTypeVideo); err != nil {
		return nil, err
	}
	_ = me.RegisterCodec(webrtc.RTPCodecParameters{
		RTPCodecCapability: webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeH264, ClockRate: 90000},
		PayloadType:        102,
	}, webrtc.RTPCodecTypeVideo)

	se := webrtc.SettingEngine{}
	minPort := opt.ICE.UDPPortMin
	maxPort := opt.ICE.UDPPortMax
	if minPort == 0 {
		minPort = 10000
	}
	if maxPort == 0 {
		maxPort = 20000
	}
	if err := se.SetEphemeralUDPPortRange(minPort, maxPort); err != nil {
		return nil, err
	}

	var iceServers []webrtc.ICEServer
	if len(opt.ICE.STUNURLs) > 0 {
		iceServers = append(iceServers, webrtc.ICEServer{URLs: opt.ICE.STUNURLs})
	}

	ir := &interceptor.Registry{}
	if err := webrtc.RegisterDefaultInterceptors(me, ir); err != nil {
		return nil, err
	}

	api := webrtc.NewAPI(webrtc.WithMediaEngine(me), webrtc.WithSettingEngine(se), webrtc.WithInterceptorRegistry(ir))

	mePCMU := &webrtc.MediaEngine{}
	if err := mePCMU.RegisterCodec(webrtc.RTPCodecParameters{
		RTPCodecCapability: webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypePCMU, ClockRate: 8000},
		PayloadType:        0,
	}, webrtc.RTPCodecTypeAudio); err != nil {
		return nil, err
	}
	if err := mePCMU.RegisterCodec(webrtc.RTPCodecParameters{
		RTPCodecCapability: webrtc.RTPCodecCapability{MimeType: "audio/telephone-event", ClockRate: 8000},
		PayloadType:        101,
	}, webrtc.RTPCodecTypeAudio); err != nil {
		return nil, err
	}
	sePCMU := webrtc.SettingEngine{}
	if err := sePCMU.SetEphemeralUDPPortRange(minPort, maxPort); err != nil {
		return nil, err
	}
	irPCMU := &interceptor.Registry{}
	if err := webrtc.RegisterDefaultInterceptors(mePCMU, irPCMU); err != nil {
		return nil, err
	}
	apiPCMU := webrtc.NewAPI(webrtc.WithMediaEngine(mePCMU), webrtc.WithSettingEngine(sePCMU), webrtc.WithInterceptorRegistry(irPCMU))

	s := &Service{
		api:           api,
		apiPCMU:       apiPCMU,
		ice:           iceServers,
		turn:          opt.TURN,
		recDir:        opt.RecordingsDir,
		rooms:         map[string]*room{},
		recByID:       map[string]*recorder{},
		sipPending:    map[string]bool{},
		sipRTPPending: map[string]map[*sipRTP]struct{}{},
	}
	s.sip = newSIPUA(opt.SIP, s)
	return s, nil
}

var _ ports.MediaPort = (*Service)(nil)

func (s *Service) CreateRoom(ctx context.Context, callID string, opts dto.RoomOptions) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if existing, ok := s.rooms[callID]; ok {
		existing.mu.Lock()
		defer existing.mu.Unlock()
		if s.sipPending[callID] {
			s.rooms[callID].sipAudio = true
			delete(s.sipPending, callID)
		}
		if rtp := s.sipRTPPending[callID]; rtp != nil {
			s.rooms[callID].sipRTP = rtp
			s.rooms[callID].sipAudio = true
			delete(s.sipRTPPending, callID)
		}
		return nil
	}
	sipAudio := s.sipPending[callID]
	rtpSess := s.sipRTPPending[callID]
	delete(s.sipPending, callID)
	delete(s.sipRTPPending, callID)
	s.rooms[callID] = &room{
		enableVideo: opts.EnableVideo || opts.SessionType == dto.SessionTypeVideo || opts.SessionType == dto.SessionTypeMixed,
		sipAudio:    sipAudio || rtpSess != nil,
		sipRTP:      rtpSess,
		peers:       map[string]*peer{},
		dtmf:        map[string]ports.DTMFHandler{},
	}
	return nil
}

func (s *Service) CloseRoom(ctx context.Context, callID string) error {
	if s.sip != nil {
		s.sip.endCall(callID, true)
	}
	s.mu.Lock()
	r := s.rooms[callID]
	delete(s.rooms, callID)
	delete(s.sipPending, callID)
	delete(s.sipRTPPending, callID)
	var rec *recorder
	if r != nil {
		r.mu.Lock()
		rec = r.rec
		r.rec = nil
		r.mu.Unlock()
	}
	for id, saved := range s.recByID {
		if saved.callID == callID {
			delete(s.recByID, id)
		}
	}
	s.mu.Unlock()
	if rec != nil {
		_ = rec.close()
	}
	if r == nil {
		return nil
	}
	r.mu.Lock()
	peers := r.peers
	r.peers = map[string]*peer{}
	r.mu.Unlock()
	for _, p := range peers {
		_ = p.pc.Close()
	}
	return nil
}

func (s *Service) LeaveRoom(ctx context.Context, callID, legID string) error {
	if s.sip != nil {
		s.sip.endLeg(callID, legID)
	}
	r := s.getRoom(callID)
	if r == nil {
		return nil
	}
	r.mu.Lock()
	for rt := range r.sipRTP {
		rt.mu.Lock()
		match := rt.legID == legID
		rt.mu.Unlock()
		if match {
			rt.close()
			delete(r.sipRTP, rt)
		}
	}
	p := r.peers[legID]
	delete(r.peers, legID)
	r.mu.Unlock()
	if p != nil {
		_ = p.pc.Close()
	}
	return nil
}

func (s *Service) JoinWebRTC(ctx context.Context, callID, legID string, role dto.LegRole) (dto.LocalOffer, error) {
	r := s.getRoom(callID)
	if r == nil {
		return dto.LocalOffer{}, errs.NotFound("媒体房间不存在")
	}

	r.mu.RLock()
	sipAudio := r.sipAudio
	enableVideo := r.enableVideo
	r.mu.RUnlock()

	api := s.api
	if sipAudio && s.apiPCMU != nil {
		api = s.apiPCMU
	}
	pc, err := api.NewPeerConnection(webrtc.Configuration{ICEServers: s.iceServers()})
	if err != nil {
		return dto.LocalOffer{}, err
	}

	audioCap := webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeOpus, ClockRate: 48000, Channels: 2}
	if sipAudio {
		audioCap = webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypePCMU, ClockRate: 8000}
	}
	audioOut, err := webrtc.NewTrackLocalStaticRTP(audioCap, "audio", "sfu-audio-"+legID)
	if err != nil {
		_ = pc.Close()
		return dto.LocalOffer{}, err
	}
	if _, err := pc.AddTrack(audioOut); err != nil {
		_ = pc.Close()
		return dto.LocalOffer{}, err
	}

	var audioSamp *webrtc.TrackLocalStaticSample
	if !sipAudio {
		audioSamp, err = webrtc.NewTrackLocalStaticSample(webrtc.RTPCodecCapability{
			MimeType: webrtc.MimeTypePCMU, ClockRate: 8000,
		}, "moh", "sfu-moh-"+legID)
		if err != nil {
			_ = pc.Close()
			return dto.LocalOffer{}, err
		}
		if _, err := pc.AddTrack(audioSamp); err != nil {
			_ = pc.Close()
			return dto.LocalOffer{}, err
		}
	}

	var videoOut *webrtc.TrackLocalStaticRTP
	if enableVideo && !sipAudio {
		videoOut, err = webrtc.NewTrackLocalStaticRTP(webrtc.RTPCodecCapability{
			MimeType: webrtc.MimeTypeVP8, ClockRate: 90000,
		}, "video", "sfu-video-"+legID)
		if err != nil {
			_ = pc.Close()
			return dto.LocalOffer{}, err
		}
		if _, err := pc.AddTrack(videoOut); err != nil {
			_ = pc.Close()
			return dto.LocalOffer{}, err
		}
	}

	recvOnly := role == dto.LegRoleSupervisor
	p := &peer{pc: pc, audioOut: audioOut, videoOut: videoOut, audioSamp: audioSamp, role: role, audioMuted: recvOnly}

	pc.OnTrack(func(remote *webrtc.TrackRemote, _ *webrtc.RTPReceiver) {
		if remote.Codec().MimeType == "audio/telephone-event" {
			go s.readDTMF(callID, legID, remote)
			return
		}
		go s.forward(callID, legID, remote)
	})

	r.mu.Lock()
	if old, ok := r.peers[legID]; ok {
		_ = old.pc.Close()
	}
	r.peers[legID] = p
	r.mu.Unlock()

	gatherComplete := webrtc.GatheringCompletePromise(pc)
	offer, err := pc.CreateOffer(nil)
	if err != nil {
		return dto.LocalOffer{}, err
	}
	if err := pc.SetLocalDescription(offer); err != nil {
		return dto.LocalOffer{}, err
	}
	select {
	case <-gatherComplete:
	case <-ctx.Done():
		return dto.LocalOffer{}, ctx.Err()
	case <-time.After(3 * time.Second):
	}
	ld := pc.LocalDescription()
	if ld == nil {
		return dto.LocalOffer{}, errs.Internal("未生成本地 SDP")
	}
	return dto.LocalOffer{SDP: ld.SDP, Type: "offer"}, nil
}

func (s *Service) AcceptAnswer(ctx context.Context, callID, legID string, answerSDP string) error {
	p := s.getPeer(callID, legID)
	if p == nil {
		return errs.NotFound("媒体腿不存在")
	}
	return p.pc.SetRemoteDescription(webrtc.SessionDescription{Type: webrtc.SDPTypeAnswer, SDP: answerSDP})
}

func (s *Service) TrickleICE(ctx context.Context, callID, legID string, cand dto.ICECandidateInit) error {
	p := s.getPeer(callID, legID)
	if p == nil {
		return errs.NotFound("媒体腿不存在")
	}
	init := webrtc.ICECandidateInit{Candidate: cand.Candidate}
	if cand.SDPMid != "" {
		init.SDPMid = new(cand.SDPMid)
	}
	if cand.SDPMLineIndex != nil {
		init.SDPMLineIndex = new(uint16(*cand.SDPMLineIndex))
	}
	return p.pc.AddICECandidate(init)
}

func (s *Service) IssueTURNCredentials(ctx context.Context, subject string, ttl time.Duration) (dto.TURNConfig, error) {
	if !s.turn.Enabled {
		return dto.TURNConfig{STUNURLs: s.stunURLs()}, nil
	}
	if ttl <= 0 {
		ttl = time.Hour
	}
	exp := time.Now().Add(ttl).Unix()
	username := fmt.Sprintf("%d:%s", exp, subject)
	mac := hmac.New(sha1.New, []byte(s.turn.AuthSecret))
	_, _ = mac.Write([]byte(username))
	cred := base64.StdEncoding.EncodeToString(mac.Sum(nil))
	return dto.TURNConfig{STUNURLs: s.stunURLs(), URLs: append([]string{}, s.turn.URLs...), Username: username, Credential: cred, TTL: ttl}, nil
}

func (s *Service) SetTrackMuted(ctx context.Context, callID, legID string, audio, video bool) error {
	r := s.getRoom(callID)
	if r == nil {
		return errs.NotFound("媒体房间不存在")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if p := r.peers[legID]; p != nil {
		p.audioMuted = audio
		p.videoMuted = video
		return nil
	}
	for rt := range r.sipRTP {
		rt.mu.Lock()
		if rt.legID == legID {
			rt.muted = audio
			rt.mu.Unlock()
			return nil
		}
		rt.mu.Unlock()
	}
	return errs.NotFound("媒体腿不存在")
}

func (s *Service) SetHold(ctx context.Context, callID, legID string, on bool) error {
	r := s.getRoom(callID)
	if r == nil {
		return errs.NotFound("媒体房间不存在")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if p := r.peers[legID]; p != nil {
		p.held = on
		return nil
	}
	for rt := range r.sipRTP {
		rt.mu.Lock()
		if rt.legID == legID {
			rt.held = on
			rt.mu.Unlock()
			return nil
		}
		rt.mu.Unlock()
	}
	return errs.NotFound("媒体腿不存在")
}

func (s *Service) RequestRenegotiation(ctx context.Context, callID, legID string, addVideo bool) error {
	r := s.getRoom(callID)
	if r == nil {
		return errs.NotFound("媒体房间不存在")
	}
	r.mu.Lock()
	r.enableVideo = addVideo
	if !addVideo {
		for _, p := range r.peers {
			p.videoMuted = true
		}
	}
	r.mu.Unlock()
	_ = ctx
	_ = legID
	return nil
}

func (s *Service) InjectAudio(ctx context.Context, callID, botLegID string, source dto.AudioSource) error {
	r := s.getRoom(callID)
	if r == nil {
		return errs.NotFound("媒体房间不存在")
	}
	_ = botLegID
	path := s.resolvePrompt(source.FilePath)
	go s.playSourceToRoom(callID, path, source.Loop)
	return nil
}

func (s *Service) SubscribeDTMF(ctx context.Context, callID, legID string, handler ports.DTMFHandler) error {
	r := s.getRoom(callID)
	if r == nil {
		return errs.NotFound("媒体房间不存在")
	}
	r.mu.Lock()
	r.dtmf[legID] = handler
	r.mu.Unlock()
	return nil
}

func (s *Service) SendDTMF(ctx context.Context, callID, legID string, digit dto.DTMFDigit) error {
	r := s.getRoom(callID)
	if r == nil {
		return errs.NotFound("媒体房间不存在")
	}
	r.mu.RLock()
	h := r.dtmf[legID]
	r.mu.RUnlock()
	if h != nil {
		h(ctx, digit)
	}
	return nil
}

func (s *Service) StartRecording(ctx context.Context, callID string, policy dto.RecordingPolicy) (string, error) {
	if policy.Mode == "" || policy.Mode == "off" {
		return "", nil
	}
	r := s.getRoom(callID)
	if r == nil {
		return "", errs.NotFound("媒体房间不存在")
	}
	if err := os.MkdirAll(s.recDir, 0o750); err != nil {
		return "", err
	}
	id := uuid.New().String()
	r.mu.RLock()
	sipAudio := r.sipAudio
	r.mu.RUnlock()
	ext := ".ogg"
	if sipAudio {
		ext = ".wav"
	}
	audioPath := filepath.Join(s.recDir, callID+"-"+id+ext)
	var until *time.Time
	if policy.RetainDays > 0 {
		until = new(time.Now().UTC().Add(time.Duration(policy.RetainDays) * 24 * time.Hour))
	}
	rec := &recorder{
		id: id, callID: callID, path: audioPath, audioPath: audioPath, mode: policy.Mode,
		started: time.Now().UTC(), retainTo: until,
	}
	if sipAudio {
		rec.pcm = newPCMMix(8000, rec.started)
		if err := rec.pcm.startFile(audioPath); err != nil {
			return "", err
		}
	} else {
		ogg, err := oggwriter.New(audioPath, 48000, 2)
		if err != nil {
			return "", err
		}
		rec.ogg = ogg
	}
	if policy.Mode == "video_composite" {
		videoPath := filepath.Join(s.recDir, callID+"-"+id+".ivf")
		ivf, err := ivfwriter.New(videoPath)
		if err == nil {
			rec.videoPath = videoPath
			rec.ivf = ivf
		}
	}
	r.mu.Lock()
	r.rec = rec
	r.mu.Unlock()
	s.mu.Lock()
	s.recByID[id] = rec
	s.mu.Unlock()
	return id, nil
}

func (s *Service) StopRecording(ctx context.Context, recordingID string) error {
	s.mu.Lock()
	rec := s.recByID[recordingID]
	s.mu.Unlock()
	if rec == nil {
		return nil
	}
	if r := s.getRoom(rec.callID); r != nil {
		r.mu.Lock()
		if r.rec != nil && r.rec.id == recordingID {
			r.rec = nil
		}
		r.mu.Unlock()
	}
	return rec.close()
}

func (s *Service) RecordingInfo(ctx context.Context, recordingID string) (ports.RecordingMeta, error) {
	s.mu.Lock()
	rec := s.recByID[recordingID]
	s.mu.Unlock()
	if rec == nil {
		return ports.RecordingMeta{}, errs.NotFound("录音不存在")
	}
	return rec.snapshot(), nil
}

func (s *Service) OriginateSIP(ctx context.Context, callID, legID, dial, trunkID string) error {
	if s.sip == nil || !s.sip.enabled {
		return errs.Unprocessable("SIP 未启用", errs.CodeSIPDisabled)
	}
	return s.sip.originate(ctx, callID, legID, dial, trunkID)
}

func (s *Service) BridgeLegs(ctx context.Context, callID, legA, legB string) error {
	if s.getPeer(callID, legA) == nil || s.getPeer(callID, legB) == nil {
		return errs.NotFound("媒体腿不存在")
	}
	return nil
}

func (s *Service) iceServers() []webrtc.ICEServer {
	out := append([]webrtc.ICEServer{}, s.ice...)
	if s.turn.Enabled && len(s.turn.URLs) > 0 {
		out = append(out, webrtc.ICEServer{URLs: s.turn.URLs})
	}
	return out
}

func (s *Service) getRoom(callID string) *room {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.rooms[callID]
}

func (s *Service) getPeer(callID, legID string) *peer {
	r := s.getRoom(callID)
	if r == nil {
		return nil
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.peers[legID]
}

func (s *Service) forward(callID, fromLeg string, remote *webrtc.TrackRemote) {
	buf := make([]byte, 1500)
	pkt := &rtp.Packet{}
	for {
		n, _, err := remote.Read(buf)
		if err != nil {
			return
		}
		r := s.getRoom(callID)
		if r == nil {
			return
		}
		r.mu.RLock()
		unmarshaled := pkt.Unmarshal(buf[:n]) == nil
		if unmarshaled && r.rec != nil {
			cp := *pkt
			cp.Payload = append([]byte{}, pkt.Payload...)
			r.rec.writeRTP(remote.Kind(), &cp)
		}
		from := r.peers[fromLeg]
		muted := false
		held := false
		if from != nil {
			held = from.held
			if remote.Kind() == webrtc.RTPCodecTypeAudio {
				muted = from.audioMuted
			} else {
				muted = from.videoMuted
			}
			if from.role == dto.LegRoleSupervisor {
				muted = true
			}
		}
		for id, p := range r.peers {
			if id == fromLeg || muted || held || p.held {
				continue
			}
			var out *webrtc.TrackLocalStaticRTP
			if remote.Kind() == webrtc.RTPCodecTypeAudio {
				out = p.audioOut
			} else {
				out = p.videoOut
			}
			if out != nil {
				_, _ = out.Write(buf[:n])
			}
		}
		if unmarshaled && !muted && !held && remote.Kind() == webrtc.RTPCodecTypeAudio && r.sipRTP != nil {
			outPkt := *pkt
			outPkt.Payload = append([]byte{}, pkt.Payload...)
			outPkt.Extension = false
			outPkt.Extensions = nil
			outPkt.Padding = false
			if raw, err := outPkt.Marshal(); err == nil {
				for rt := range r.sipRTP {
					if !rt.blocked() {
						rt.writePCMU(raw)
					}
				}
			}
		}
		r.mu.RUnlock()
	}
}

func (s *Service) readDTMF(callID, legID string, remote *webrtc.TrackRemote) {
	buf := make([]byte, 1500)
	last := byte(255)
	for {
		n, _, err := remote.Read(buf)
		if err != nil {
			return
		}
		if n < 1 {
			continue
		}
		pkt := &rtp.Packet{}
		if err := pkt.Unmarshal(buf[:n]); err != nil || len(pkt.Payload) < 1 {
			continue
		}
		event := pkt.Payload[0] & 0x7f
		end := len(pkt.Payload) > 1 && pkt.Payload[1]&0x80 != 0
		if !end || event == last {
			continue
		}
		last = event
		digit := dtmfEvent(event)
		r := s.getRoom(callID)
		if r == nil {
			return
		}
		r.mu.RLock()
		h := r.dtmf[legID]
		r.mu.RUnlock()
		if h != nil && digit != "" {
			h(context.Background(), dto.DTMFDigit(digit))
		}
	}
}

func dtmfEvent(ev byte) string {
	if ev <= 9 {
		return string('0' + ev)
	}
	switch ev {
	case 10:
		return "*"
	case 11:
		return "#"
	default:
		return ""
	}
}

func (s *Service) playTone(callID, legID string, dur time.Duration) {
	p := s.getPeer(callID, legID)
	if p == nil || p.audioSamp == nil {
		return
	}
	frames := int(dur / (20 * time.Millisecond))
	payload := mulawToneFrame()
	for i := 0; i < frames; i++ {
		if cur := s.getPeer(callID, legID); cur == nil || !cur.held {
			return
		}
		_ = p.audioSamp.WriteSample(media.Sample{Data: payload, Duration: 20 * time.Millisecond})
		time.Sleep(20 * time.Millisecond)
	}
}

func (s *Service) playToneToRoom(callID string, dur time.Duration) {
	deadline := time.Now().Add(dur)
	payload := mulawToneFrame()
	for time.Now().Before(deadline) {
		r := s.getRoom(callID)
		if r == nil {
			return
		}
		r.mu.RLock()
		for _, p := range r.peers {
			if p.audioSamp != nil {
				_ = p.audioSamp.WriteSample(media.Sample{Data: payload, Duration: 20 * time.Millisecond})
			}
		}
		r.mu.RUnlock()
		time.Sleep(20 * time.Millisecond)
	}
}

func (rec *recorder) writeRTP(kind webrtc.RTPCodecType, pkt *rtp.Packet) {
	if rec == nil || pkt == nil {
		return
	}
	rec.mu.Lock()
	defer rec.mu.Unlock()
	if rec.ended != nil {
		return
	}
	if kind == webrtc.RTPCodecTypeAudio && rec.pcm != nil && (pkt.PayloadType == 0 || pkt.PayloadType == 8) {
		rec.pcm.add(pkt.PayloadType, pkt.Payload)
		rec.bytes = rec.pcm.byteSize()
		return
	}
	if kind == webrtc.RTPCodecTypeAudio && rec.ogg != nil && pkt.PayloadType != 0 && pkt.PayloadType != 8 && pkt.PayloadType != 101 {
		if err := rec.ogg.WriteRTP(pkt); err == nil {
			rec.bytes += int64(len(pkt.Payload))
		}
	}
	if kind == webrtc.RTPCodecTypeVideo && rec.ivf != nil {
		if err := rec.ivf.WriteRTP(pkt); err == nil {
			rec.bytes += int64(len(pkt.Payload))
		}
	}
	if rec.file != nil {
		n, _ := rec.file.Write(pkt.Payload)
		rec.bytes += int64(n)
	}
}

func (rec *recorder) write(b []byte) {
	if rec == nil || rec.file == nil {
		return
	}
	rec.mu.Lock()
	defer rec.mu.Unlock()
	n, _ := rec.file.Write(b)
	rec.bytes += int64(n)
}

func (rec *recorder) close() error {
	if rec == nil {
		return nil
	}
	rec.mu.Lock()
	defer rec.mu.Unlock()
	if rec.ended != nil {
		return nil
	}
	rec.ended = new(time.Now().UTC())
	var err error
	if rec.ogg != nil {
		err = rec.ogg.Close()
		rec.ogg = nil
	}
	if rec.pcm != nil {
		if n, werr := rec.pcm.writeWAV(rec.path); werr == nil {
			rec.bytes = n
		} else if err == nil {
			err = werr
		}
		rec.pcm = nil
	}
	if rec.ivf != nil {
		_ = rec.ivf.Close()
		rec.ivf = nil
	}
	if rec.file != nil {
		_ = rec.file.Close()
		rec.file = nil
	}
	if rec.audioPath != "" && rec.videoPath != "" {
		if out, muxErr := muxWebM(rec.audioPath, rec.videoPath); muxErr == nil && out != "" {
			rec.path = out
			if st, e := os.Stat(out); e == nil {
				rec.bytes = st.Size()
			}
		}
	}
	return err
}

func (rec *recorder) snapshot() ports.RecordingMeta {
	rec.mu.Lock()
	defer rec.mu.Unlock()
	return ports.RecordingMeta{
		ID: rec.id, CallID: rec.callID, FilePath: rec.path, MediaType: rec.mode,
		StartedAt: rec.started, EndedAt: rec.ended, RetainUntil: rec.retainTo, FileSize: rec.bytes,
	}
}

func muxWebM(audioPath, videoPath string) (string, error) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		return "", err
	}
	out := strings.TrimSuffix(audioPath, filepath.Ext(audioPath)) + ".webm"
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "ffmpeg", "-y", "-i", audioPath, "-i", videoPath, "-c", "copy", out)
	if err := cmd.Run(); err != nil {
		return "", err
	}
	return out, nil
}

func mulawToneFrame() []byte {
	const n = 160
	out := make([]byte, n)
	for i := 0; i < n; i++ {
		v := int16(8000)
		if i%18 < 9 {
			v = -8000
		}
		out[i] = linearToMulaw(v)
	}
	return out
}

func linearToMulaw(sample int16) byte {
	const (
		bias = 0x84
		clip = 32635
	)
	sign := byte(0)
	if sample < 0 {
		sign = 0x80
		sample = -sample
	}
	if sample > clip {
		sample = clip
	}
	sample += bias
	exp := byte(7)
	for exp > 0 && sample&(0x4000) == 0 {
		sample <<= 1
		exp--
	}
	mant := byte((sample >> 7) & 0x0F)
	return ^(sign | (exp << 4) | mant)
}

var _ = io.EOF

func (s *Service) stunURLs() []string {
	var urls []string
	for _, server := range s.ice {
		urls = append(urls, server.URLs...)
	}
	return urls
}
