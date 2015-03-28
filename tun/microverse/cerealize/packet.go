package cerealize

// Package cerealize has name that is a humorous reference to the
// CapnProto serialization format used herein.
//
// PelicanPacket: the packets exchanged between Chaser and LongPoller.
// We use the underlying (bambam generated) PelicanPacketCapn capnproto
// struct directly to avoid copying Payload around too often.
type PelicanPacket struct {
	// if not Request, then is Reply
	IsRequest bool     `capid:"0"`
	Key       string   `capid:"1"`
	Serialnum int64    `capid:"2"`
	Body      []*Pbody `capid:"3"`
}

type Pbody struct {
	IsRequest bool  `capid:"0"`
	Serialnum int64 `capid:"1"`
	Paysize   int64 `capid:"2"`
	AbTm      int64 `capid:"3"`
	LpTm      int64 `capid:"4"`

	Paymac  [64]byte `capid:"5"`
	Payload []byte   `capid:"6"`
}
