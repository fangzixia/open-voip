package media

func (m *scheduledRoomMixer) removeLeg(id string) {
	if m == nil {
		return
	}
	m.mu.Lock()
	leg, input, out := m.legs[id], m.inputs[id], m.outputs[id]
	delete(m.legs, id)
	delete(m.inputs, id)
	delete(m.outputs, id)
	delete(m.epochs, id)
	delete(m.gains, id)
	delete(m.outSeq, id)
	delete(m.outTS, id)
	delete(m.outSSRC, id)
	m.mu.Unlock()
	if input != nil {
		input.Close()
	}
	if leg != nil {
		leg.close()
	}
	if out != nil {
		out.stop()
	}
}
