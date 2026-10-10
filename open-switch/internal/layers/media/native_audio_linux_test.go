//go:build linux && cgo

package media

import (
	"fmt"
	"math"
	"slices"
	"testing"
)

func TestSoxrStreamingMatchesContinuous(t *testing.T) {
	input := make([]int16, 48000)
	for i := range input {
		input[i] = int16(10000 * math.Sin(2*math.Pi*1700*float64(i)/48000))
	}
	convert := func(chunk int) []int16 {
		r, err := newStreamingResampler(48000, 8000, false)
		if err != nil {
			t.Fatal(err)
		}
		defer r.close()
		var result []int16
		for at := 0; at < len(input); at += chunk {
			out, e := r.process(input[at:min(at+chunk, len(input))], false)
			if e != nil {
				t.Fatal(e)
			}
			result = append(result, out...)
		}
		out, e := r.process(nil, true)
		if e != nil {
			t.Fatal(e)
		}
		return append(result, out...)
	}
	whole, split := convert(len(input)), convert(721)
	if len(whole) != 8000 || len(split) != 8000 {
		t.Fatalf("sample count %d/%d", len(whole), len(split))
	}
	// libsoxr dithering can differ by one integer unit between instances.
	for i, v := range whole {
		if math.Abs(float64(int(v)-int(split[i]))) > 2 {
			t.Fatalf("boundary discontinuity at %d", i)
		}
	}
}

func TestSoxrPassband(t *testing.T) {
	for _, hz := range []float64{100, 1000, 2000, 3000} {
		r, e := newStreamingResampler(48000, 8000, false)
		if e != nil {
			t.Fatal(e)
		}
		in := make([]int16, 48000)
		for i := range in {
			in[i] = int16(12000 * math.Sin(2*math.Pi*hz*float64(i)/48000))
		}
		out, e := r.process(in, false)
		if e != nil {
			t.Fatal(e)
		}
		tail, e := r.process(nil, true)
		r.close()
		if e != nil {
			t.Fatal(e)
		}
		out = append(out, tail...)
		var power float64
		for _, v := range out[1000:7000] {
			power += float64(v) * float64(v)
		}
		db := 20 * math.Log10(math.Sqrt(power/6000)/(12000/math.Sqrt2))
		if math.Abs(db) > 1 {
			t.Fatalf("%.0f Hz: %.3f dB", hz, db)
		}
	}
}

func TestSpanDSPSilenceAndRecovery(t *testing.T) {
	p := newG711PLC()
	defer p.close()
	if p.ptr == nil {
		t.Fatal("PLC allocation failed")
	}
	pcm := make([]int16, 160)
	p.receive(pcm)
	if slices.ContainsFunc(pcm, func(v int16) bool { return v != 0 }) {
		t.Fatal("silence altered")
	}
	for range 3 {
		p.fill(pcm)
	}
	for i := range pcm {
		pcm[i] = int16(1000 * math.Sin(2*math.Pi*400*float64(i)/8000))
	}
	p.receive(pcm)
	if !slices.ContainsFunc(pcm, func(v int16) bool { return v != 0 }) {
		t.Fatal("did not recover")
	}
}

func TestVariableSoxrLatencyBudget(t *testing.T) {
	r, e := newStreamingResampler(8000, 8000, true)
	if e != nil {
		t.Fatal(e)
	}
	defer r.close()
	for _, ppm := range []float64{-300, 0, 300} {
		if e = r.setPPM(ppm); e != nil {
			t.Fatal(e)
		}
	}
	consumed, produced := 0, 0
	for range 10 {
		out, e := r.process(make([]int16, 160), false)
		if e != nil {
			t.Fatal(e)
		}
		consumed += 160
		produced += len(out)
		if produced > 0 {
			t.Logf("variable resampler startup delay samples=%d", consumed-produced)
			if consumed-produced > 240 {
				t.Fatalf("native delay exceeds shared 30 ms budget: %d samples", consumed-produced)
			}
			return
		}
	}
	t.Fatal("variable resampler did not produce within 200 ms")
}

func TestVariableSoxrExtremesMatchContinuous(t *testing.T) {
	for _, ppm := range []float64{-300, 0, 300} {
		t.Run(fmt.Sprintf("%gppm", ppm), func(t *testing.T) {
			const samples = 80000
			input := make([]int16, samples)
			for i := range input {
				input[i] = int16(12000 * math.Sin(2*math.Pi*1700*float64(i)/8000))
			}
			convert := func(chunk int) []int16 {
				r, err := newStreamingResampler(8000, 8000, true)
				if err != nil {
					t.Fatal(err)
				}
				defer r.close()
				if err = r.setPPM(ppm); err != nil {
					t.Fatal(err)
				}
				var result []int16
				for at := 0; at < len(input); at += chunk {
					out, e := r.process(input[at:min(at+chunk, len(input))], false)
					if e != nil {
						t.Fatal(e)
					}
					result = append(result, out...)
				}
				out, e := r.process(nil, true)
				if e != nil {
					t.Fatal(e)
				}
				return append(result, out...)
			}
			whole, split := convert(samples), convert(137)
			// The one-second native slew interpolates from unity to the target.
			want := float64(samples)/(1+ppm/1e6) + 4000*ppm/1e6
			if math.Abs(float64(len(whole))-want) > 3 || len(whole) != len(split) {
				t.Fatalf("rate/sample count: whole=%d split=%d expected=%.3f", len(whole), len(split), want)
			}
			for i, v := range whole {
				if math.Abs(float64(int(v)-int(split[i]))) > 2 {
					t.Fatalf("chunk discontinuity at %d", i)
				}
			}
			var power float64
			for _, v := range whole[8000:72000] {
				power += float64(v) * float64(v)
			}
			db := 20 * math.Log10(math.Sqrt(power/64000)/(12000/math.Sqrt2))
			if math.Abs(db) > 1 {
				t.Fatalf("amplitude error %.3f dB", db)
			}
		})
	}
}
