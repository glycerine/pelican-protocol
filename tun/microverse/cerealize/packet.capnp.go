package cerealize

// AUTO GENERATED - DO NOT EDIT

import (
	C "github.com/glycerine/go-capnproto"
)

type PbodyCapn C.Struct

func NewPbodyCapn(s *C.Segment) PbodyCapn      { return PbodyCapn(s.NewStruct(40, 2)) }
func NewRootPbodyCapn(s *C.Segment) PbodyCapn  { return PbodyCapn(s.NewRootStruct(40, 2)) }
func AutoNewPbodyCapn(s *C.Segment) PbodyCapn  { return PbodyCapn(s.NewStructAR(40, 2)) }
func ReadRootPbodyCapn(s *C.Segment) PbodyCapn { return PbodyCapn(s.Root(0).ToStruct()) }
func (s PbodyCapn) IsRequest() bool            { return C.Struct(s).Get1(0) }
func (s PbodyCapn) SetIsRequest(v bool)        { C.Struct(s).Set1(0, v) }
func (s PbodyCapn) Serialnum() int64           { return int64(C.Struct(s).Get64(8)) }
func (s PbodyCapn) SetSerialnum(v int64)       { C.Struct(s).Set64(8, uint64(v)) }
func (s PbodyCapn) Paysize() int64             { return int64(C.Struct(s).Get64(16)) }
func (s PbodyCapn) SetPaysize(v int64)         { C.Struct(s).Set64(16, uint64(v)) }
func (s PbodyCapn) AbTm() int64                { return int64(C.Struct(s).Get64(24)) }
func (s PbodyCapn) SetAbTm(v int64)            { C.Struct(s).Set64(24, uint64(v)) }
func (s PbodyCapn) LpTm() int64                { return int64(C.Struct(s).Get64(32)) }
func (s PbodyCapn) SetLpTm(v int64)            { C.Struct(s).Set64(32, uint64(v)) }
func (s PbodyCapn) Paymac() C.UInt8List        { return C.UInt8List(C.Struct(s).GetObject(0)) }
func (s PbodyCapn) SetPaymac(v C.UInt8List)    { C.Struct(s).SetObject(0, C.Object(v)) }
func (s PbodyCapn) Payload() C.UInt8List       { return C.UInt8List(C.Struct(s).GetObject(1)) }
func (s PbodyCapn) SetPayload(v C.UInt8List)   { C.Struct(s).SetObject(1, C.Object(v)) }

// capn.JSON_enabled == false so we stub MarshallJSON().
func (s PbodyCapn) MarshalJSON() (bs []byte, err error) { return }

type PbodyCapn_List C.PointerList

func NewPbodyCapnList(s *C.Segment, sz int) PbodyCapn_List {
	return PbodyCapn_List(s.NewCompositeList(40, 2, sz))
}
func (s PbodyCapn_List) Len() int           { return C.PointerList(s).Len() }
func (s PbodyCapn_List) At(i int) PbodyCapn { return PbodyCapn(C.PointerList(s).At(i).ToStruct()) }
func (s PbodyCapn_List) ToArray() []PbodyCapn {
	n := s.Len()
	a := make([]PbodyCapn, n)
	for i := 0; i < n; i++ {
		a[i] = s.At(i)
	}
	return a
}
func (s PbodyCapn_List) Set(i int, item PbodyCapn) { C.PointerList(s).Set(i, C.Object(item)) }

type PelicanPacketCapn C.Struct

func NewPelicanPacketCapn(s *C.Segment) PelicanPacketCapn { return PelicanPacketCapn(s.NewStruct(8, 2)) }
func NewRootPelicanPacketCapn(s *C.Segment) PelicanPacketCapn {
	return PelicanPacketCapn(s.NewRootStruct(8, 2))
}
func AutoNewPelicanPacketCapn(s *C.Segment) PelicanPacketCapn {
	return PelicanPacketCapn(s.NewStructAR(8, 2))
}
func ReadRootPelicanPacketCapn(s *C.Segment) PelicanPacketCapn {
	return PelicanPacketCapn(s.Root(0).ToStruct())
}
func (s PelicanPacketCapn) IsRequest() bool          { return C.Struct(s).Get1(0) }
func (s PelicanPacketCapn) SetIsRequest(v bool)      { C.Struct(s).Set1(0, v) }
func (s PelicanPacketCapn) Key() string              { return C.Struct(s).GetObject(0).ToText() }
func (s PelicanPacketCapn) SetKey(v string)          { C.Struct(s).SetObject(0, s.Segment.NewText(v)) }
func (s PelicanPacketCapn) Body() PbodyCapn_List     { return PbodyCapn_List(C.Struct(s).GetObject(1)) }
func (s PelicanPacketCapn) SetBody(v PbodyCapn_List) { C.Struct(s).SetObject(1, C.Object(v)) }

// capn.JSON_enabled == false so we stub MarshallJSON().
func (s PelicanPacketCapn) MarshalJSON() (bs []byte, err error) { return }

type PelicanPacketCapn_List C.PointerList

func NewPelicanPacketCapnList(s *C.Segment, sz int) PelicanPacketCapn_List {
	return PelicanPacketCapn_List(s.NewCompositeList(8, 2, sz))
}
func (s PelicanPacketCapn_List) Len() int { return C.PointerList(s).Len() }
func (s PelicanPacketCapn_List) At(i int) PelicanPacketCapn {
	return PelicanPacketCapn(C.PointerList(s).At(i).ToStruct())
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
	C.PointerList(s).Set(i, C.Object(item))
}
