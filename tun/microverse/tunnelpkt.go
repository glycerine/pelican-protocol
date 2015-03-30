package main

import (
	"bytes"
	"net/http"
)

type tunnelPacket struct {
	resp    http.ResponseWriter
	respdup *bytes.Buffer // duplicate resp here, to enable testing

	request *http.Request

	key  string
	done chan bool

	ppReq  *PelicanPacket
	ppResp *PelicanPacket
}

//
func NewTunnelPacket(reqSer int64, respSer int64, key string) *tunnelPacket {
	p := &tunnelPacket{
		key:    key,
		done:   make(chan bool),
		ppReq:  NewPelicanPacket(request, reqSer),
		ppResp: NewPelicanPacket(response, respSer),
	}
	return p
}

func (t *tunnelPacket) AddPayload(isReq isReqType, work []byte, atAb bool) {
	// ignore len 0 work
	if len(work) == 0 {
		return
	}

	if isReq == request {
		t.ppReq.AppendPayload(work, atAb)
	} else {
		t.ppResp.AppendPayload(work, atAb)
	}
}

/*
func ToSerReq(pack *tunnelPacket) *SerReq {
	return &SerReq{
		reqBody:       pack.reqBody,
		requestSerial: pack.requestSerial,
	}
}

// replace with Pbody where Pbody.IsRequest = true, e.g. by call to NewRequestPbody()
//type SerReq struct {
//	reqBody       []byte
//	requestSerial int64 // order the sends with content by serial number
//}
*/
