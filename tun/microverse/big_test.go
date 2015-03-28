package main

import (
	"encoding/binary"
	"fmt"
	"testing"
	"time"

	cv "github.com/glycerine/goconvey/convey"
	sha3 "golang.org/x/crypto/sha3"
)

// generate N 8-byte-words of test data, filled with 0..N int64s
func genTestData(N int64) []byte {
	sendMe := make([]byte, N*8)
	for i := int64(0); i < N; i++ {
		binary.LittleEndian.PutUint64(sendMe[i*8:(i+1)*8], uint64(i))
	}
	return sendMe
}

// confirm the data from genTestData is still intact as prepared.
func checkTestData(N int64, sendMe []byte) {
	for i := int64(0); i < N; i++ {
		n := int64(binary.LittleEndian.Uint64(sendMe[i*8 : (i+1)*8]))
		if n != i {
			panic(fmt.Sprintf("n(%v) != i(%v)", n, i))
		}
	}
}

func TestBigMessagesInBigBuffersComeThroughWithCorrectlyOrderedData052(t *testing.T) {

	// how many int64s to transfer:
	N := int64(1e6)

	// how many to put in one packet
	//M := int64(1e5)

	sendMe := genTestData(N)
	checkTestData(N, sendMe)

	sha3_512_digest := sha3.Sum512(sendMe)
	fmt.Printf("\n okay before send !!!: '%x' \n", string(sha3_512_digest[:]))

	dn := NewBoundary("downstream")

	ab2lp := make(chan *tunnelPacket)
	lp2ab := make(chan *tunnelPacket)

	longPollDur := 2 * time.Second
	lp := NewLittlePoll(longPollDur, dn, ab2lp, lp2ab)

	dn.Start()
	defer dn.Stop()

	lp.Start()
	defer lp.Stop()

	cv.Convey("With large 16MB buffers we were seeing some corruption in the SCP tests, check for that in microverse", t, func() {

		/*
			// test *request* reorder alone (distinct from *reply* reordering):

			c2 := NewMockResponseWriter()

			// First send 2 in requestSerial 2, then send 1 in request serial 1,
			// and we should see them arrive 1 then 2 due to the re-ordering logic.
			//
			body2 := []byte("2")
			reqBody2 := bytes.NewBuffer(body2)
			r2, err := http.NewRequest("POST", "http://example.com/", reqBody2)
			panicOn(err)
			pack2 := &tunnelPacket{
				resp:    c2,
				respdup: new(bytes.Buffer),
				request: r2,
				done:    make(chan bool),
				key:     "longpoll_test_key",
				SerReq: SerReq{
					reqBody:       body2,
					requestSerial: 2,
				},
			}

			lp.ab2lp <- pack2

			c1 := NewMockResponseWriter()

			body1 := []byte("1")
			reqBody1 := bytes.NewBuffer(body1)
			r1, err := http.NewRequest("POST", "http://example.com/", reqBody1)
			panicOn(err)

			pack1 := &tunnelPacket{
				resp:    c1,
				respdup: new(bytes.Buffer),
				request: r1,
				done:    make(chan bool),
				key:     "longpoll_test_key",
				SerReq: SerReq{
					reqBody:       body1,
					requestSerial: 1,
				},
			}

			lp.ab2lp <- pack1

			// we won't get pack1 back immediately, but we will get pack2 back,
			// since it was sent first.
			//			select {
			//			case <-pack1.done:
			//				// good
			//				po("got back pack1.done")
			//			case <-time.After(1 * time.Second):
			//				dn.hist.ShowHistory()
			//				panic("should have had pack1 be done by now -- if re-ordering is in effect")
			//			}

			select {
			case <-pack2.done:
				// good
				po("got back pack2.done")
			case <-time.After(1 * time.Second):
				dn.hist.ShowHistory()
				panic("should have had pack2 be done by now -- if re-ordering is in effect")
			}

			//po("pack1 got back: '%s'", pack1.respdup.Bytes())
			po("pack2 got back: '%s'", pack2.respdup.Bytes())

			dh := dn.hist.GetHistory()
			dh.ShowHistory()

			cv.So(dh.CountAbsorbs(), cv.ShouldEqual, 1)
			cv.So(dh.CountGenerates(), cv.ShouldEqual, 0)

			cv.So(string(dh.absorbHistory[0].what), cv.ShouldEqual, "12")
		*/
	})

}
