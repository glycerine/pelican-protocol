package main

import (
	"bytes"
	"fmt"
	"reflect"
	"testing"

	cv "github.com/glycerine/goconvey/convey"

	"github.com/glycerine/go-goon"
)

func TestPacketLoadSave053(t *testing.T) {

	cv.Convey("test that basic PelicanPacket serialization and deserialization work", t, func() {
		rw := NewPelicanPacket(request, 23)

		rw.Key = "mykey"
		rw.AppendPayload([]byte("wonderful1"), false)
		rw.AppendPayload([]byte("wonderful2"), false)
		rw.AppendPayload([]byte("wonderful3"), false)

		var o bytes.Buffer
		rw.Save(&o)

		rw2 := &PelicanPacket{}
		rw2.Load(o.Bytes())

		//		eq := PacketsEqual(rw, rw2)
		eq := reflect.DeepEqual(rw, rw2)

		if !eq {
			fmt.Printf("\n\n ****** rw and rw2 were not equal!\n")

			fmt.Printf("\n\n =============  rw: ====\n")
			goon.Dump(rw)
			fmt.Printf("\n\n =============  rw2: ====\n")
			goon.Dump(rw2)
			fmt.Printf("\n\n ================\n")

			panic("rw and rw2 should have been equal!")
		}

		fmt.Printf("\n yes! Load() data matched Saved() data.\n")

		cv.So(eq, cv.ShouldEqual, true)
	})
}

func PacketsEqual(a, b *PelicanPacket) bool {

	for i := 0; i < len(a.Body); i++ {
		if !reflect.DeepEqual(a.Body[i], b.Body[i]) {
			panic(fmt.Sprintf("difference at %d", i))
			return false
		}
	}

	if a.Key != b.Key {
		panic("Key")
	}
	if a.IsRequest != b.IsRequest {
		panic("IsRequest")
	}
	if a.Serialnum != b.Serialnum {
		panic("Serialnum")
	}

	return true
}
