package config

import "strings"

const defaultRecordingNotice = "本通话可能会被录音或录像，继续即表示您已知悉。"

func (r *RecordingsConfig) normalize() {
	if strings.TrimSpace(r.Video.Format) == "" {
		r.Video.Format = "webm"
	}
	if strings.TrimSpace(r.Audio.NotifyMessage) == "" {
		r.Audio.NotifyMessage = defaultRecordingNotice
	}
	if strings.TrimSpace(r.Video.NotifyMessage) == "" {
		r.Video.NotifyMessage = defaultRecordingNotice
	}
}

// AudioDirPath 返回纯音频录制根目录（含 IVR 提示音 prompts/）。
func (r RecordingsConfig) AudioDirPath() string {
	return strings.TrimSpace(r.Audio.Dir)
}

// VideoDirPath 返回录像成品与合成临时目录根路径。
func (r RecordingsConfig) VideoDirPath() string {
	return strings.TrimSpace(r.Video.Dir)
}

// VideoFFmpegPath 返回录像合成/转码用的 FFmpeg 路径。
func (r RecordingsConfig) VideoFFmpegPath() string {
	return strings.TrimSpace(r.Video.FFmpegPath)
}

// DirForExt 按文件扩展名选择存储根目录。
func (r RecordingsConfig) DirForExt(ext string) string {
	switch ext {
	case ".webm", ".mp4", ".ivf":
		return r.VideoDirPath()
	default:
		return r.AudioDirPath()
	}
}

// NotifyMessageForMode 返回与录制模式匹配的告知文案。
func (r RecordingsConfig) NotifyMessageForMode(mode string) string {
	if mode == "video_composite" {
		return strings.TrimSpace(r.Video.NotifyMessage)
	}
	if mode == "audio" {
		return strings.TrimSpace(r.Audio.NotifyMessage)
	}
	return ""
}
