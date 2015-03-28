package cerealize

import (
	"io"

	capn "github.com/glycerine/go-capnproto"
)

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
	dest.Serialnum = int64(src.Serialnum())
	dest.Paysize = int64(src.Paysize())
	dest.AbTm = int64(src.AbTm())
	dest.LpTm = int64(src.LpTm())

	// Paymac
	copy(dest.Paymac[:], []byte(src.Paymac().ToArray()))

	// Payload
	// may be dangerous: refer to underlying segment to avoid a copy
	dest.Payload = src.Payload().ToArray()

	return dest
}

func PbodyGoToCapn(seg *capn.Segment, src *Pbody) PbodyCapn {
	dest := AutoNewPbodyCapn(seg)
	dest.SetSerialnum(src.Serialnum)
	dest.SetPaysize(src.Paysize)
	dest.SetAbTm(src.AbTm)
	dest.SetLpTm(src.LpTm)

	mylist1 := seg.NewUInt8List(len(src.Paymac))
	for i := range src.Paymac {
		mylist1.Set(i, uint8(src.Paymac[i]))
	}
	dest.SetPaymac(mylist1)

	mylist2 := seg.NewUInt8List(len(src.Payload))
	for i := range src.Payload {
		mylist2.Set(i, uint8(src.Payload[i]))
	}
	dest.SetPayload(mylist2)

	return dest
}

func (s *PelicanPacket) Save(w io.Writer) error {
	seg := capn.NewBuffer(nil)
	PelicanPacketGoToCapn(seg, s)
	_, err := seg.WriteTo(w)
	return err
}

func (s *PelicanPacket) Load(r io.Reader) error {
	capMsg, err := capn.ReadFromStream(r, nil)
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
	dest.Key = src.Key()

	var n int

	// Body
	n = src.Body().Len()
	dest.Body = make([]Pbody, n)
	for i := 0; i < n; i++ {
		dest.Body[i] = *PbodyCapnToGo(src.Body().At(i), nil)
	}

	return dest
}

func PelicanPacketGoToCapn(seg *capn.Segment, src *PelicanPacket) PelicanPacketCapn {
	dest := AutoNewPelicanPacketCapn(seg)
	dest.SetKey(src.Key)

	// Body -> PbodyCapn (go slice to capn list)
	if len(src.Body) > 0 {
		typedList := NewPbodyCapnList(seg, len(src.Body))
		plist := capn.PointerList(typedList)
		i := 0
		for _, ele := range src.Body {
			plist.Set(i, capn.Object(PbodyGoToCapn(seg, &ele)))
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

func SlicePbodyToPbodyCapnList(seg *capn.Segment, m []Pbody) PbodyCapn_List {
	lst := NewPbodyCapnList(seg, len(m))
	for i := range m {
		lst.Set(i, PbodyGoToCapn(seg, &m[i]))
	}
	return lst
}

func PbodyCapnListToSlicePbody(p PbodyCapn_List) []Pbody {
	v := make([]Pbody, p.Len())
	for i := range v {
		PbodyCapnToGo(p.At(i), &v[i])
	}
	return v
}
