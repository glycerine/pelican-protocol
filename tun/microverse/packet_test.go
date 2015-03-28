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

		rw := PelicanPacket{
			IsRequest: true,
			Key:       "mykey",
			Body: []Pbody{
				Pbody{
					IsRequest: true,
					Payload:   []byte("wonderful1"),
				},
				Pbody{
					IsRequest: true,
					Payload:   []byte("wonderful2"),
				},
				Pbody{
					IsRequest: false,
					Payload:   []byte("wonderful3"),
				},
			},
		}

		var o bytes.Buffer
		rw.Save(&o)

		rw2 := &PelicanPacket{}
		rw2.Load(&o)

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
