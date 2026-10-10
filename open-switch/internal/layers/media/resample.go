package media

// resamplePCM is for complete finite assets only. Real-time callers retain one
// streamingResampler per stream/generation and drain exclusively at its end.
func resamplePCM(pcm []int16, fromRate, toRate int) ([]int16, error) {
	if len(pcm) == 0 || fromRate == toRate {
		return pcm, nil
	}
	r, e := newStreamingResampler(fromRate, toRate, false)
	if e != nil {
		return nil, e
	}
	defer r.close()
	out, e := r.process(pcm, false)
	if e != nil {
		return nil, e
	}
	tail, e := r.process(nil, true)
	if e != nil {
		return nil, e
	}
	return append(out, tail...), nil
}
