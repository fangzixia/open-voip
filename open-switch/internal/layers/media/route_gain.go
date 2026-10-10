package media

type routeSourceGain struct {
	gain       float64
	lastSample int16
}

// Linear gain transitions occupy exactly one room frame, including removals.
// Removed routes fade using the current source frame when available, otherwise
// only their final sample. This does not replay buffered audio or conceal loss.
type routeGain struct{ previous map[string]routeSourceGain }

func (g *routeGain) mix(sources map[string][]int16) []int16 {
	return g.mixWithTransitions(sources, nil)
}

func (g *routeGain) mixWithTransitions(sources, currentFrames map[string][]int16) []int16 {
	if g.previous == nil {
		g.previous = map[string]routeSourceGain{}
	}
	out := make([]int16, mixFrameSamples)
	n := len(sources)
	var target float64
	if n > 0 {
		target = mixHeadroom / float64(n)
	}
	for i := range out {
		progress := float64(i+1) / float64(len(out))
		var sum, total float64
		for id, pcm := range sources {
			previous := g.previous[id].gain
			gain := previous + (target-previous)*progress
			total += gain
			if i < len(pcm) {
				sum += float64(pcm[i]) * gain
			}
		}
		for id, previous := range g.previous {
			if _, active := sources[id]; active {
				continue
			}
			gain := previous.gain * (1 - progress)
			sample := previous.lastSample
			if pcm, available := currentFrames[id]; available {
				sample = 0
				if i < len(pcm) {
					sample = pcm[i]
				}
			}
			total += gain
			sum += float64(sample) * gain
		}
		if total > mixHeadroom {
			sum *= mixHeadroom / total
		}
		out[i] = int16(sum)
	}
	clear(g.previous)
	for id, pcm := range sources {
		state := routeSourceGain{gain: target}
		if len(pcm) >= len(out) {
			state.lastSample = pcm[len(out)-1]
		}
		g.previous[id] = state
	}
	return out
}
