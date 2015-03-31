package main

import (
	"fmt"
	"io"
	"time"

	capn "github.com/glycerine/go-capnproto"
	"golang.org/x/crypto/sha3"
)

// concatenate cerealize/{packet.go saveload.go packet.capnp.go}, then
// add support for the bool IsRequest, which bambam didnn't add.
// And make Pbody refer to the same slices in the origin PbodyCapn
// segment rather than copying big Payloads around.

// PelicanPacket describes the packets exchanged between Chaser and LongPoller.
// We use capnproto for serialization and version skew management.
// The PelicanPacket struct acts as IDL, and debugging convenience.
//
// The Serial numbers should start from at
// least 1; 0 is an invalid serial number.
//
// PelicanPacket: the packets exchanged between Chaser and LongPoller.
// A packet may contain multiple Pbody that have been coalesced together
// under the Body slice.

type isReqType bool

const request isReqType = true
const response isReqType = false

//new:
type PelicanPacket struct {
	// if not Request, then is Reply
	IsRequest bool   `capid:"0"`
	Key       string `capid:"1"`

	// order the replies/requests by serial number. Empty replies get serial number -1.
	Serialnum int64    `capid:"2"` // for ab -> lp packets on the lp side
	Body      []*Pbody `capid:"3"`
}

func NewPelicanPacket(isReq isReqType, ser int64, key string) *PelicanPacket {
	return &PelicanPacket{
		IsRequest: bool(isReq),
		Serialnum: ser,
		Body:      make([]*Pbody, 0),
		Key:       key,
	}
}

func (pp *PelicanPacket) AppendPayload(work []byte, atAb bool) {
	// ignore len 0 work
	if len(work) == 0 {
		return
	}

	newPbody := newPbody(pp.IsRequest, work, pp.Serialnum)
	if atAb {
		newPbody.AbTm = time.Now().UnixNano()
	} else {
		newPbody.LpTm = time.Now().UnixNano()
	}
	pp.Body = append(pp.Body, newPbody)
}

func (pp *PelicanPacket) Verifies() bool {
	if !IsLegitPelicanKey([]byte(pp.Key)) {
		return false
	}

	if len(pp.Body) == 0 {
		return true
	}
	for _, v := range pp.Body {
		if !v.Verifies() {
			return false
		}
	}
	return true
}

func (pp *PelicanPacket) ShowPayload() {
	fmt.Printf("\n=========== begin ShowPayload() ===================\n")
	if len(pp.Body) == 0 {
		fmt.Printf("len(pp.Body) == 0, no content here.\n")
	} else {
		for i := 0; i < len(pp.Body); i++ {
			fmt.Printf("======== ShowPayload, Pbody %d of %d =========\n", i, len(pp.Body))
			fmt.Printf("%d:'%s'\n", i, string(pp.Body[i].Payload))
		}
	}
	fmt.Printf("\n=========== end of ShowPayload() ===================\n")
}

func (pp *PelicanPacket) TotalPayloadSize() int64 {
	var tot int64
	if len(pp.Body) == 0 {
		return 0
	} else {
		for i := 0; i < len(pp.Body); i++ {
			tot += int64(len(pp.Body[i].Payload))
			if int64(len(pp.Body[i].Payload)) != pp.Body[i].Paysize {
				panic("Paysize out of sync with len(pp.Body[i].Payload)!")
			}
		}
	}
	return tot
}

func (pp *PelicanPacket) TotalPayload() []byte {
	tot := pp.TotalPayloadSize()

	if tot == 0 {
		return []byte{}
	}

	if len(pp.Body) == 1 {
		// don't copy if we can avoid it
		return pp.Body[0].Payload
	}

	r := make([]byte, tot)

	w := 0

	for i := 0; i < len(pp.Body); i++ {

		// sanity checks:
		if int64(len(pp.Body[i].Payload)) != pp.Body[i].Paysize {
			panic(fmt.Sprintf("len(pp.Body[i].Payload) != pp.Body[i].Paysize; %d != %d",
				len(pp.Body[i].Payload),
				pp.Body[i].Paysize))
		}
		if !pp.Body[i].Verifies() {
			panic(fmt.Sprintf("body %d '%#v' failed to verify", i, pp.Body[i]))
		}

		w += copy(r[w:], pp.Body[i].Payload)
	}
	return r
}

func (pp *PelicanPacket) SetSerial(ser int64) {
	pp.Serialnum = ser
	for _, v := range pp.Body {
		v.Serialnum = ser
	}
}

func (pp *PelicanPacket) SetAbTm() {
	now := time.Now().UnixNano()
	for _, v := range pp.Body {
		v.AbTm = now
	}
}
func (pp *PelicanPacket) SetLpTm() {
	now := time.Now().UnixNano()
	for _, v := range pp.Body {
		v.LpTm = now
	}
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

func newPbody(isRequest bool, payload []byte, ser int64) *Pbody {
	return &Pbody{
		IsRequest: isRequest,
		Paymac:    sha3.Sum512(payload),
		Payload:   payload,
		Paysize:   int64(len(payload)),
		Serialnum: ser,
	}
}

func (pb *Pbody) Verifies() bool {
	if pb.Paymac == sha3.Sum512(pb.Payload) && pb.Paysize == int64(len(pb.Payload)) {
		return true
	}
	return false
}

// auto-generated code below (from bambam and capnpc-go), plus
// a few manual tweaks

func (s *Pbody) Save(w io.Writer) error {
	seg := capn.NewBuffer(nil)
	PbodyGoToCapn(seg, s)
	_, err := seg.WriteTo(w)
	return err
}

func (s *Pbody) Load(r io.Reader) error {
	capMsg, err := capn.ReadFromStream(r, nil)
	if err != nil {
		//panic(fmt.Errorf("capn.ReadFromStream error: %s", err))
		return err
	}
	z := ReadRootPbodyCapn(capMsg)
	PbodyCapnToGo(z, s)
	return nil
}

func PbodyCapnToGo(src PbodyCapn, dest *Pbody) *Pbody {
	if dest == nil {
		dest = &Pbody{}
	}
	dest.IsRequest = src.IsRequest()
	dest.Serialnum = src.Serialnum()
	dest.Paysize = src.Paysize()
	dest.AbTm = src.AbTm()
	dest.LpTm = src.LpTm()

	// Paymac
	copy(dest.Paymac[:], []byte(src.Paymac().ToArray())) // manually adjusted

	// Payload
	// may be dangerous: refer to underlying segment to avoid a big copy
	dest.Payload = src.Payload().ToArray() // manually added

	return dest
}

func PbodyGoToCapn(seg *capn.Segment, src *Pbody) PbodyCapn {
	dest := AutoNewPbodyCapn(seg)
	dest.SetIsRequest(src.IsRequest)
	dest.SetSerialnum(src.Serialnum)
	dest.SetPaysize(src.Paysize)
	dest.SetAbTm(src.AbTm)
	dest.SetLpTm(src.LpTm)

	mylist1 := seg.NewUInt8List(len(src.Paymac))
	copy(mylist1.ToArray(), src.Paymac[:])
	//	for i := range src.Paymac {
	//		mylist1.Set(i, uint8(src.Paymac[i]))
	//	}
	dest.SetPaymac(mylist1)

	mylist2 := seg.NewUInt8List(len(src.Payload))
	copy(mylist2.ToArray(), src.Payload)
	//	for i := range src.Payload {
	//		mylist2.Set(i, uint8(src.Payload[i]))
	//	}
	dest.SetPayload(mylist2)

	return dest
}

func (s *PelicanPacket) Save(w io.Writer) error {
	seg := capn.NewBuffer(nil)
	PelicanPacketGoToCapn(seg, s)
	_, err := seg.WriteTo(w)
	return err
}

func (s *PelicanPacket) Load(src []byte) error {

	capMsg, _, err := capn.ReadFromMemoryZeroCopy(src)
	//capMsg, err := capn.ReadFromStream(r, nil)

	if err != nil {
		//panic(fmt.Errorf("capn.ReadFromStream error: %s", err))
		return err
	}
	z := ReadRootPelicanPacketCapn(capMsg)
	PelicanPacketCapnToGo(z, s)
	return nil
}

func PelicanPacketCapnToGo(src PelicanPacketCapn, dest *PelicanPacket) *PelicanPacket {
	if dest == nil {
		dest = &PelicanPacket{}
	}
	dest.IsRequest = src.IsRequest()
	dest.Key = src.Key()
	dest.Serialnum = src.Serialnum()

	var n int

	// Body
	n = src.Body().Len()
	dest.Body = make([]*Pbody, n)
	for i := 0; i < n; i++ {
		dest.Body[i] = PbodyCapnToGo(src.Body().At(i), nil)
	}

	return dest
}

func PelicanPacketGoToCapn(seg *capn.Segment, src *PelicanPacket) PelicanPacketCapn {
	dest := AutoNewPelicanPacketCapn(seg)
	dest.SetIsRequest(src.IsRequest)
	dest.SetKey(src.Key)
	dest.SetSerialnum(src.Serialnum)

	// Body -> PbodyCapn (go slice to capn list)
	if len(src.Body) > 0 {
		typedList := NewPbodyCapnList(seg, len(src.Body))
		plist := capn.PointerList(typedList)
		i := 0
		for _, ele := range src.Body {
			plist.Set(i, capn.Object(PbodyGoToCapn(seg, ele)))
			i++
		}
		dest.SetBody(typedList)
	}

	return dest
}

func SliceByteToUInt8List(seg *capn.Segment, m []byte) capn.UInt8List {
	lst := seg.NewUInt8List(len(m))
	for i := range m {
		lst.Set(i, uint8(m[i]))
	}
	return lst
}

func UInt8ListToSliceByte(p capn.UInt8List) []byte {
	v := make([]byte, p.Len())
	for i := range v {
		v[i] = byte(p.At(i))
	}
	return v
}

type PbodyCapn capn.Struct

func NewPbodyCapn(s *capn.Segment) PbodyCapn      { return PbodyCapn(s.NewStruct(40, 2)) }
func NewRootPbodyCapn(s *capn.Segment) PbodyCapn  { return PbodyCapn(s.NewRootStruct(40, 2)) }
func AutoNewPbodyCapn(s *capn.Segment) PbodyCapn  { return PbodyCapn(s.NewStructAR(40, 2)) }
func ReadRootPbodyCapn(s *capn.Segment) PbodyCapn { return PbodyCapn(s.Root(0).ToStruct()) }
func (s PbodyCapn) IsRequest() bool               { return capn.Struct(s).Get1(0) }
func (s PbodyCapn) SetIsRequest(v bool)           { capn.Struct(s).Set1(0, v) }
func (s PbodyCapn) Serialnum() int64              { return int64(capn.Struct(s).Get64(8)) }
func (s PbodyCapn) SetSerialnum(v int64)          { capn.Struct(s).Set64(8, uint64(v)) }
func (s PbodyCapn) Paysize() int64                { return int64(capn.Struct(s).Get64(16)) }
func (s PbodyCapn) SetPaysize(v int64)            { capn.Struct(s).Set64(16, uint64(v)) }
func (s PbodyCapn) AbTm() int64                   { return int64(capn.Struct(s).Get64(24)) }
func (s PbodyCapn) SetAbTm(v int64)               { capn.Struct(s).Set64(24, uint64(v)) }
func (s PbodyCapn) LpTm() int64                   { return int64(capn.Struct(s).Get64(32)) }
func (s PbodyCapn) SetLpTm(v int64)               { capn.Struct(s).Set64(32, uint64(v)) }
func (s PbodyCapn) Paymac() capn.UInt8List        { return capn.UInt8List(capn.Struct(s).GetObject(0)) }
func (s PbodyCapn) SetPaymac(v capn.UInt8List)    { capn.Struct(s).SetObject(0, capn.Object(v)) }
func (s PbodyCapn) Payload() capn.UInt8List       { return capn.UInt8List(capn.Struct(s).GetObject(1)) }
func (s PbodyCapn) SetPayload(v capn.UInt8List)   { capn.Struct(s).SetObject(1, capn.Object(v)) }

type PbodyCapn_List capn.PointerList

func NewPbodyCapnList(s *capn.Segment, sz int) PbodyCapn_List {
	return PbodyCapn_List(s.NewCompositeList(40, 2, sz))
}
func (s PbodyCapn_List) Len() int           { return capn.PointerList(s).Len() }
func (s PbodyCapn_List) At(i int) PbodyCapn { return PbodyCapn(capn.PointerList(s).At(i).ToStruct()) }
func (s PbodyCapn_List) ToArray() []PbodyCapn {
	n := s.Len()
	a := make([]PbodyCapn, n)
	for i := 0; i < n; i++ {
		a[i] = s.At(i)
	}
	return a
}
func (s PbodyCapn_List) Set(i int, item PbodyCapn) { capn.PointerList(s).Set(i, capn.Object(item)) }

type PelicanPacketCapn capn.Struct

func NewPelicanPacketCapn(s *capn.Segment) PelicanPacketCapn {
	return PelicanPacketCapn(s.NewStruct(16, 2))
}
func NewRootPelicanPacketCapn(s *capn.Segment) PelicanPacketCapn {
	return PelicanPacketCapn(s.NewRootStruct(16, 2))
}
func AutoNewPelicanPacketCapn(s *capn.Segment) PelicanPacketCapn {
	return PelicanPacketCapn(s.NewStructAR(16, 2))
}
func ReadRootPelicanPacketCapn(s *capn.Segment) PelicanPacketCapn {
	return PelicanPacketCapn(s.Root(0).ToStruct())
}
func (s PelicanPacketCapn) IsRequest() bool          { return capn.Struct(s).Get1(0) }
func (s PelicanPacketCapn) SetIsRequest(v bool)      { capn.Struct(s).Set1(0, v) }
func (s PelicanPacketCapn) Key() string              { return capn.Struct(s).GetObject(0).ToText() }
func (s PelicanPacketCapn) SetKey(v string)          { capn.Struct(s).SetObject(0, s.Segment.NewText(v)) }
func (s PelicanPacketCapn) Serialnum() int64         { return int64(capn.Struct(s).Get64(8)) }
func (s PelicanPacketCapn) SetSerialnum(v int64)     { capn.Struct(s).Set64(8, uint64(v)) }
func (s PelicanPacketCapn) Body() PbodyCapn_List     { return PbodyCapn_List(capn.Struct(s).GetObject(1)) }
func (s PelicanPacketCapn) SetBody(v PbodyCapn_List) { capn.Struct(s).SetObject(1, capn.Object(v)) }

type PelicanPacketCapn_List capn.PointerList

func NewPelicanPacketCapnList(s *capn.Segment, sz int) PelicanPacketCapn_List {
	return PelicanPacketCapn_List(s.NewCompositeList(16, 2, sz))
}
func (s PelicanPacketCapn_List) Len() int { return capn.PointerList(s).Len() }
func (s PelicanPacketCapn_List) At(i int) PelicanPacketCapn {
	return PelicanPacketCapn(capn.PointerList(s).At(i).ToStruct())
}
func (s PelicanPacketCapn_List) ToArray() []PelicanPacketCapn {
	n := s.Len()
	a := make([]PelicanPacketCapn, n)
	for i := 0; i < n; i++ {
		a[i] = s.At(i)
	}
	return a
}
func (s PelicanPacketCapn_List) Set(i int, item PelicanPacketCapn) {
	capn.PointerList(s).Set(i, capn.Object(item))
}
