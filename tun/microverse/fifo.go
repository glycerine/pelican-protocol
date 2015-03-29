package main

// request queue can be empty
type RequestFifo struct {
	q    []*PelicanPacket
	size int
}

func NewRequestFifo(capacity int) *RequestFifo {
	r := &RequestFifo{
		q:    make([]*PelicanPacket, 0, capacity),
		size: capacity,
	}
	return r
}

func (s *RequestFifo) Len() int {
	return len(s.q)
}

func (s *RequestFifo) Empty() bool {
	if len(s.q) == 0 {
		return true
	}
	return false
}

func (s *RequestFifo) PushLeft(by *PelicanPacket) {
	s.q = append([]*PelicanPacket{by}, s.q...)
}

func (s *RequestFifo) PushRight(by *PelicanPacket) {
	s.q = append(s.q, by)
}

func (s *RequestFifo) PopRight() *PelicanPacket {
	r := s.PeekRight()
	n := len(s.q)
	s.q = s.q[:n-1]
	return r
}

func (s *RequestFifo) PeekRight() *PelicanPacket {
	if len(s.q) == 0 {
		return nil
	}
	n := len(s.q)
	r := s.q[n-1]
	return r
}
