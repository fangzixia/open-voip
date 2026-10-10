package media

// Linear gain transitions occupy exactly one room frame. Normalisation at
// every sample guarantees that total gain remains <= 0.85 during a join.
type routeGain struct{ previous map[string]float64 }

func (g *routeGain) mix(sources map[string][]int16) []int16 {
	if g.previous == nil {
		g.previous = map[string]float64{}
	}
	out := make([]int16, mixFrameSamples)
	n := len(sources)
	if n == 0 {
		clear(g.previous)
		return out
	}
	target := mixHeadroom / float64(n)
	for i := range out {
		progress := float64(i+1) / float64(len(out))
		var sum, total float64
		for id, pcm := range sources {
			gain := g.previous[id] + (target-g.previous[id])*progress
			total += gain
			if i < len(pcm) {
				sum += float64(pcm[i]) * gain
			}
		}
		if total > mixHeadroom {
			sum *= mixHeadroom / total
		}
		out[i] = int16(sum)
	}
	clear(g.previous)
	for id := range sources {
		g.previous[id] = target
	}
	return out
}
