package main

import (
	"bytes"
	"net/http"
)

type tunnelPacket struct {
	resp    http.ResponseWriter
	respdup *bytes.Buffer // duplicate resp here, to enable testing

	request *http.Request

	//key  string // separate from body
	done chan bool

	// becomes ppResp.Serialnum
	//replySerial int64 // order the replies by serial number. Empty replies get serial number -1.

	ppReq  *PelicanPacket
	ppResp *PelicanPacket
}

func NewTunnelPacket() *tunnelPacket {
	p := &tunnelPacket{
		done:   make(chan bool),
		ppReq:  NewPelicanPacket(request),
		ppResp: NewPelicanPacket(response),
	}
	return p
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
