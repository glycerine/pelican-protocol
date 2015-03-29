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
		rw.AppendPayload([]byte("wonderful1"))
		rw.AppendPayload([]byte("wonderful2"))
		rw.AppendPayload([]byte("wonderful3"))

		var o bytes.Buffer
		rw.Save(&o)

		rw2 := &PelicanPacket{}
		rw2.Load(o.Bytes())

		eq := reflect.DeepEqual(&rw, rw2)

		if !eq {
			fmt.Printf("rw and rw2 were not equal!\n")

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
