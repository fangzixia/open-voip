//go:build linux && cgo

package media

/*
#cgo pkg-config: soxr spandsp
#include <stdlib.h>
#include <soxr.h>
#include <spandsp.h>

static soxr_t ov_soxr_create(double in_rate, double out_rate, int variable, const char **error) {
    soxr_io_spec_t io = soxr_io_spec(SOXR_INT16_I, SOXR_INT16_I);
	io.flags |= SOXR_NO_DITHER;
    soxr_quality_spec_t q = soxr_quality_spec(SOXR_HQ, variable ? SOXR_VR : 0);
    soxr_runtime_spec_t rt = soxr_runtime_spec(1);
    // SOXR_VR requires the creation ratio to cover every later I/O ratio.
    double max_in_rate = variable ? in_rate * 1.0003 : in_rate;
    soxr_t state = soxr_create(max_in_rate, out_rate, 1, error, &io, &q, &rt);
    if (state && variable && !*error) {
        *error = soxr_set_io_ratio(state, in_rate / out_rate, 0);
    }
    if (state && *error) {
        soxr_delete(state);
        state = NULL;
    }
    return state;
}
*/
import "C"

import (
	"errors"
	"fmt"
	"unsafe"
)

// Native states are owned by one stream and used under that stream's mutex.
// Errors from libsoxr point to static strings; they must never be freed.
type streamingResampler struct {
	ptr             C.soxr_t
	inRate, outRate int
	variable        bool
}

func newStreamingResampler(inRate, outRate int, variable bool) (*streamingResampler, error) {
	if inRate <= 0 || outRate <= 0 {
		return nil, errors.New("invalid sample rate")
	}
	var e *C.char
	v := 0
	if variable {
		v = 1
	}
	p := C.ov_soxr_create(C.double(inRate), C.double(outRate), C.int(v), &e)
	if e != nil {
		return nil, errors.New(C.GoString(e))
	}
	if p == nil {
		return nil, errors.New("soxr_create returned nil")
	}
	return &streamingResampler{ptr: p, inRate: inRate, outRate: outRate, variable: variable}, nil
}

func (r *streamingResampler) process(in []int16, drain bool) ([]int16, error) {
	if r == nil || r.ptr == nil {
		return nil, errors.New("resampler closed")
	}
	if len(in) == 0 && !drain {
		return nil, nil
	}
	var out []int16
	for {
		buf := make([]int16, max(1024, len(in)*r.outRate/r.inRate+256))
		var consumed, produced C.size_t
		var src unsafe.Pointer
		if len(in) > 0 {
			src = unsafe.Pointer(&in[0])
		}
		e := C.soxr_process(r.ptr, C.soxr_in_t(src), C.size_t(len(in)), &consumed, C.soxr_out_t(unsafe.Pointer(&buf[0])), C.size_t(len(buf)), &produced)
		if e != nil {
			return nil, errors.New(C.GoString(e))
		}
		out = append(out, buf[:int(produced)]...)
		in = in[int(consumed):]
		if !drain && len(in) == 0 {
			break
		}
		if drain && len(in) == 0 && produced == 0 {
			break
		}
		if !drain && consumed == 0 && produced == 0 {
			return nil, errors.New("soxr made no progress")
		}
	}
	return out, nil
}

func (r *streamingResampler) setPPM(ppm float64) error {
	if r == nil || r.ptr == nil || !r.variable {
		return errors.New("variable resampler unavailable")
	}
	ppm = max(-300, min(300, ppm))
	ratio := float64(r.inRate) / float64(r.outRate) * (1 + ppm/1e6)
	if e := C.soxr_set_io_ratio(r.ptr, C.double(ratio), C.size_t(r.outRate)); e != nil {
		return fmt.Errorf("soxr ratio: %s", C.GoString(e))
	}
	return nil
}

func (r *streamingResampler) close() {
	if r != nil && r.ptr != nil {
		C.soxr_delete(r.ptr)
		r.ptr = nil
	}
}

type g711PLC struct{ ptr *C.plc_state_t }

func newG711PLC() *g711PLC { return &g711PLC{ptr: C.plc_init(nil)} }
func (p *g711PLC) receive(pcm []int16) {
	if len(pcm) > 0 && p.ptr != nil {
		C.plc_rx(p.ptr, (*C.int16_t)(unsafe.Pointer(&pcm[0])), C.int(len(pcm)))
	}
}
func (p *g711PLC) fill(pcm []int16) {
	if len(pcm) > 0 && p.ptr != nil {
		C.plc_fillin(p.ptr, (*C.int16_t)(unsafe.Pointer(&pcm[0])), C.int(len(pcm)))
	}
}
func (p *g711PLC) close() {
	if p != nil && p.ptr != nil {
		C.plc_free(p.ptr)
		p.ptr = nil
	}
}
