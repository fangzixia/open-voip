package media

import (
	"context"
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"math"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/pion/interceptor"
	"github.com/pion/rtcp"
	"github.com/pion/rtp"
	"github.com/pion/webrtc/v4"
	"github.com/pion/webrtc/v4/pkg/media"
	"github.com/pion/webrtc/v4/pkg/media/oggwriter"

	"open-switch/internal/config"
	"open-switch/internal/errs"
	"open-switch/internal/observability"
	"open-switch/internal/ports"
	"open-switch/internal/ports/dto"
)

type peer struct {
	pc         *webrtc.PeerConnection
	iceMu      sync.Mutex
	pendingICE []webrtc.ICECandidateInit
	audioOut   *webrtc.TrackLocalStaticRTP
	videoOut   *webrtc.TrackLocalStaticRTP
	audioSamp  *webrtc.TrackLocalStaticSample
	role       dto.LegRole
	audioMuted bool
	videoMuted bool
	videoSSRC  uint32
	held       bool
}

type room struct {
	mu          sync.RWMutex
	direct      bool
	bridgeA     string
	bridgeB     string
	promptSeq   atomic.Uint64
	enableVideo bool
	sipAudio    bool
	sipRTP      map[*sipRTP]struct{}
	peers       map[string]*peer
	dtmf        map[string]ports.DTMFHandler
	rec         *recorder
}

func (r *room) canForward(from, to string) bool {
	if !r.direct {
		return true
	}
	return (from == r.bridgeA && to == r.bridgeB) || (from == r.bridgeB && to == r.bridgeA)
}

func (s *Service) UnbridgeLegs(callID string) {
	if r := s.getRoom(callID); r != nil {
		r.mu.Lock()
		r.bridgeA, r.bridgeB = "", ""
		r.mu.Unlock()
	}
}

type recorder struct {
	id        string
	callID    string
	path      string
	audioPath string
	mode      string
	videoRec  *videoRecording
	file      *os.File
	ogg       *oggwriter.OggWriter
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
	VideoFormat   string
	FFmpegPath    string
	SIP           config.SIPConfig
}

// Service 实现 MediaPort 的进程内 SFU。
type Service struct {
	api           *webrtc.API
	apiPCMU       *webrtc.API
	ice           []webrtc.ICEServer
	turn          config.TURNConfig
	recDir        string
	videoFormat   string
	ffmpegPath    string
	sip           *sipUA
	mu            sync.Mutex
	rooms         map[string]*room
	recByID       map[string]*recorder
	sipPending    map[string]bool
	sipRTPPending map[string]map[*sipRTP]struct{}
}

// NewService 根据 ICE/TURN/录音目录创建媒体服务。
// NewService 初始化 WebRTC 媒体能力及 SIP 配置，并建立房间与录音索引。
func NewService(opt Options) (*Service, error) {
	if opt.VideoFormat == "" {
		opt.VideoFormat = "webm"
	}
	if opt.FFmpegPath == "" {
		opt.FFmpegPath = "ffmpeg"
	}
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
		videoFormat:   opt.VideoFormat,
		ffmpegPath:    opt.FFmpegPath,
		rooms:         map[string]*room{},
		recByID:       map[string]*recorder{},
		sipPending:    map[string]bool{},
		sipRTPPending: map[string]map[*sipRTP]struct{}{},
	}
	s.sip = newSIPUA(opt.SIP, s)
	return s, nil
}

var _ ports.MediaPort = (*Service)(nil)

// CreateRoom 为通话建立媒体房间；重复调用沿用已有房间。
func (s *Service) CreateRoom(ctx context.Context, callID string, opts dto.RoomOptions) error {
	started := time.Now()
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
		direct:      opts.Direct,
		enableVideo: opts.EnableVideo || opts.SessionType == dto.SessionTypeVideo || opts.SessionType == dto.SessionTypeMixed,
		sipAudio:    sipAudio || rtpSess != nil,
		sipRTP:      rtpSess,
		peers:       map[string]*peer{},
		dtmf:        map[string]ports.DTMFHandler{},
	}
	observability.Event(observability.WithFields(ctx, observability.Fields{CallID: callID}), "media", "room.created", "create", "ok", "", started, "video", opts.EnableVideo)
	return nil
}

// CloseRoom 关闭通话中的 WebRTC、SIP 和录音资源。
func (s *Service) CloseRoom(ctx context.Context, callID string) error {
	started := time.Now()
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
	observability.Event(observability.WithFields(ctx, observability.Fields{CallID: callID}), "media", "room.closed", "close", "ok", "", started, "peer_count", len(peers))
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
	if legID == r.bridgeA || legID == r.bridgeB {
		r.bridgeA, r.bridgeB = "", ""
	}
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

// JoinWebRTC 将通话腿加入房间，绑定轨道并由服务端生成 Offer。
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
	peerCtx := observability.WithFields(ctx, observability.Fields{CallID: callID, LegID: legID})
	pc.OnSignalingStateChange(func(state webrtc.SignalingState) {
		observability.Event(peerCtx, "webrtc", "peer.signaling_state", "state", "ok", "", time.Time{}, "state", state.String())
	})
	pc.OnICEConnectionStateChange(func(state webrtc.ICEConnectionState) {
		observability.Event(peerCtx, "webrtc", "peer.ice_state", "state", "ok", "", time.Time{}, "state", state.String())
	})
	pc.OnConnectionStateChange(func(state webrtc.PeerConnectionState) {
		observability.Event(peerCtx, "webrtc", "peer.connection_state", "state", "ok", "", time.Time{}, "state", state.String())
		if state == webrtc.PeerConnectionStateConnected && enableVideo {
			go func() {
				for attempt := 0; attempt < 3; attempt++ {
					if attempt > 0 {
						time.Sleep(500 * time.Millisecond)
					}
					s.requestVideoKeyframes(callID, legID)
				}
			}()
		}
	})
	pc.OnICECandidate(func(candidate *webrtc.ICECandidate) {
		value := ""
		if candidate != nil {
			value = candidate.ToJSON().Candidate
		}
		observability.Event(peerCtx, "webrtc", "ice.local_candidate", "candidate", "ok", "", time.Time{}, "body", observability.Redact(value))
	})

	pc.OnTrack(func(remote *webrtc.TrackRemote, _ *webrtc.RTPReceiver) {
		observability.Event(peerCtx, "webrtc", "track.received", "track", "ok", "", time.Time{}, "kind", remote.Kind().String(), "codec", remote.Codec().MimeType, "ssrc", remote.SSRC())
		if remote.Kind() == webrtc.RTPCodecTypeVideo {
			r.mu.Lock()
			if r.peers[legID] == p {
				p.videoSSRC = uint32(remote.SSRC())
			}
			r.mu.Unlock()
		}
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
	observability.Event(peerCtx, "webrtc", "sdp.offer", "local", "ok", "", time.Time{}, "body", observability.Redact(ld.SDP))
	go samplePeerQuality(peerCtx, pc)
	return dto.LocalOffer{SDP: ld.SDP, Type: "offer"}, nil
}

// 后加入的订阅者需要新关键帧才能解码正在传输的视频流。
func (s *Service) requestVideoKeyframes(callID, joiningLeg string) {
	r := s.getRoom(callID)
	if r == nil {
		return
	}
	type source struct {
		pc   *webrtc.PeerConnection
		ssrc uint32
	}
	sources := make([]source, 0, 2)
	r.mu.RLock()
	for legID, p := range r.peers {
		if legID != joiningLeg && p.videoSSRC != 0 {
			sources = append(sources, source{pc: p.pc, ssrc: p.videoSSRC})
		}
	}
	r.mu.RUnlock()
	for _, source := range sources {
		_ = source.pc.WriteRTCP([]rtcp.Packet{&rtcp.PictureLossIndication{MediaSSRC: source.ssrc}})
	}
}

func (s *Service) AcceptAnswer(ctx context.Context, callID, legID string, answerSDP string) error {
	p := s.getPeer(callID, legID)
	if p == nil {
		return errs.NotFound("媒体腿不存在")
	}
	started := time.Now()
	p.iceMu.Lock()
	err := p.pc.SetRemoteDescription(webrtc.SessionDescription{Type: webrtc.SDPTypeAnswer, SDP: answerSDP})
	if err == nil {
		for _, candidate := range p.pendingICE {
			if addErr := p.pc.AddICECandidate(candidate); addErr != nil && err == nil {
				err = addErr
			}
		}
		p.pendingICE = nil
	}
	p.iceMu.Unlock()
	result := "ok"
	if err != nil {
		result = "error"
	}
	observability.Event(observability.WithFields(ctx, observability.Fields{CallID: callID, LegID: legID}), "webrtc", "sdp.answer", "remote", result, "", started, "body", observability.Redact(answerSDP))
	return err
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
	started := time.Now()
	p.iceMu.Lock()
	var err error
	if p.pc.RemoteDescription() == nil {
		p.pendingICE = append(p.pendingICE, init)
	} else {
		err = p.pc.AddICECandidate(init)
	}
	p.iceMu.Unlock()
	result := "ok"
	if err != nil {
		result = "error"
	}
	observability.Event(observability.WithFields(ctx, observability.Fields{CallID: callID, LegID: legID}), "webrtc", "ice.remote_candidate", "candidate", result, "", started, "body", observability.Redact(cand.Candidate))
	return err
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
	seq := r.promptSeq.Add(1)
	go s.playSourceToRoom(callID, path, source.Loop, seq)
	return nil
}

func (s *Service) StopInjectedAudio(ctx context.Context, callID string) error {
	r := s.getRoom(callID)
	if r == nil {
		return errs.NotFound("媒体房间不存在")
	}
	r.promptSeq.Add(1)
	_ = ctx
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

// StartRecording 按策略创建音视频录制器，并返回可查询的录音 ID。
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
	if policy.Mode == "video_composite" && sipAudio {
		return "", fmt.Errorf("SIP 音频通话不支持视频录像")
	}
	ext := ".ogg"
	if sipAudio {
		ext = ".wav"
	}
	audioPath := filepath.Join(s.recDir, callID+"-"+id+ext)
	var until *time.Time
	if policy.RetainDays > 0 {
		until = new(time.Now().UTC().Add(time.Duration(policy.RetainDays) * 24 * time.Hour))
	}
	startedAt := time.Now().UTC()
	rec := &recorder{
		id: id, callID: callID, path: audioPath, audioPath: audioPath, mode: policy.Mode,
		started: startedAt, retainTo: until,
	}
	if policy.Mode == "video_composite" {
		videoRec, err := newVideoRecording(s.recDir, callID, id, s.videoFormat, s.ffmpegPath, startedAt)
		if err != nil {
			return "", err
		}
		rec.videoRec = videoRec
		rec.path = videoRec.outputPath
		rec.audioPath = ""
	} else if sipAudio {
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
	r.mu.Lock()
	r.rec = rec
	r.mu.Unlock()
	s.mu.Lock()
	s.recByID[id] = rec
	s.mu.Unlock()
	observability.Event(observability.WithFields(ctx, observability.Fields{CallID: callID}), "media", "recording.opened", "start", "ok", "", rec.started, "recording_id", id, "mode", policy.Mode)
	return id, nil
}

// StopRecording 结束写盘并保留最终文件元数据。
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
	started := time.Now()
	err := rec.close()
	result := "ok"
	if err != nil {
		result = "error"
	}
	observability.Event(observability.WithFields(ctx, observability.Fields{CallID: rec.callID}), "media", "recording.closed", "stop", result, "", started, "recording_id", recordingID, "bytes", rec.bytes)
	return err
}

func samplePeerQuality(ctx context.Context, pc *webrtc.PeerConnection) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	for {
		<-ticker.C
		if pc.ConnectionState() == webrtc.PeerConnectionStateClosed {
			return
		}
		raw, err := json.Marshal(pc.GetStats())
		if err != nil {
			slog.WarnContext(ctx, "WebRTC stats 编码失败", append(observability.Attrs(ctx), "error", err)...)
			continue
		}
		observability.Event(ctx, "webrtc", "quality.sample", "sample", "ok", "", time.Time{}, "interval_sec", 10, "stats", string(raw))
	}
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
	r := s.getRoom(callID)
	if r == nil {
		return errs.NotFound("媒体房间不存在")
	}
	r.mu.RLock()
	for _, id := range []string{legA, legB} {
		if p := r.peers[id]; p != nil && p.pc != nil && p.pc.ConnectionState() == webrtc.PeerConnectionStateConnected {
			continue
		}
		found := false
		for rt := range r.sipRTP {
			rt.mu.Lock()
			found = found || rt.legID == id
			rt.mu.Unlock()
		}
		if !found {
			r.mu.RUnlock()
			return errs.NotFound("媒体腿尚未连接")
		}
	}
	r.mu.RUnlock()
	r.mu.Lock()
	if r.direct {
		r.bridgeA, r.bridgeB = legA, legB
	}
	r.mu.Unlock()
	_ = ctx
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

// forward 将输入 RTP 轨道转发给房间内其他通话腿，并写入录音。
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
			r.rec.writeRTP(fromLeg, remote.Kind(), remote.Codec().MimeType, &cp)
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
			if id == fromLeg || muted || held || p.held || !r.canForward(fromLeg, id) {
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
					if !rt.blocked() && r.canForward(fromLeg, rt.legID) {
						rt.writePCMU(raw)
					}
				}
			}
		}
		r.mu.RUnlock()
	}
}

// readDTMF 从 WebRTC RTP 轨道提取电话按键事件并通知订阅者。
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

func (s *Service) playToneToRoom(callID string, dur time.Duration, seq uint64) {
	deadline := time.Now().Add(dur)
	payload := mulawToneFrame()
	for time.Now().Before(deadline) {
		r := s.getRoom(callID)
		if r == nil || r.promptSeq.Load() != seq {
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

func (rec *recorder) writeRTP(legID string, kind webrtc.RTPCodecType, mime string, pkt *rtp.Packet) {
	if rec == nil || pkt == nil {
		return
	}
	rec.mu.Lock()
	defer rec.mu.Unlock()
	if rec.ended != nil {
		return
	}
	if rec.videoRec != nil {
		if err := rec.videoRec.writeRTP(legID, kind, mime, pkt); err != nil && rec.videoRec.writeErr == nil {
			rec.videoRec.writeErr = err
		}
		rec.bytes += int64(len(pkt.Payload))
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
	if rec.file != nil {
		_ = rec.file.Close()
		rec.file = nil
	}
	if rec.videoRec != nil {
		if out, size, muxErr := rec.videoRec.finish(rec.ended.Sub(rec.started)); muxErr != nil {
			if err == nil {
				err = muxErr
			}
		} else {
			rec.path = out
			rec.bytes = size
		}
		return err
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

func mulawToneFrame() []byte {
	const n = 160
	out := make([]byte, n)
	for i := 0; i < n; i++ {
		// 400 Hz 正弦波恰好在 20 ms 帧中完成整数个周期，避免帧边界爆音。
		out[i] = linearToMulaw(int16(1400 * math.Sin(2*math.Pi*400*float64(i)/8000)))
	}
	return out
}

func linearToMulaw(sample int16) byte {
	const (
		bias = 0x84
		clip = 32635
	)
	pcm := int(sample)
	sign := byte(0)
	if pcm < 0 {
		sign = 0x80
		pcm = -pcm
	}
	if pcm > clip {
		pcm = clip
	}
	pcm += bias
	exp := byte(7)
	mask := 0x4000
	for exp > 0 && pcm&mask == 0 {
		mask >>= 1
		exp--
	}
	mant := byte((pcm >> (exp + 3)) & 0x0F)
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
