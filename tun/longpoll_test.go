package pelicantun

import (
	"testing"
	"time"

	cv "github.com/glycerine/goconvey/convey"
)

// 008 acts as verification that the test in 010 works without intermediate proxies.
func TestBcastCliSrvWorkStandAlone008(t *testing.T) {

	srv := NewBcastServer(Addr{})
	srv.Start()
	defer srv.Stop()

	if !PortIsBound(srv.Listen.IpPort) {
		panic("srv not up")
	}

	cli := NewBcastClient(Addr{Port: srv.Listen.Port})
	cli.Start()
	defer cli.Stop()

	<-srv.FirstHelloClient
	po("got past <-srv.FirstHelloClient\n")

	msg := "BREAKING NEWS"
	srv.Bcast(msg)

	cv.Convey("Given a BcastClient and BcastServer without any intervening communication proxies, they communicate just fine.\n", t, func() {
		cv.Convey("when the server broadcasts a message to all waiting clients, the client should receive that message.\n", func() {

			// wait for message to get from server to client.
			select {
			case <-cli.MsgRecvd:
				// excllent
				po("excellent, cli got a message!\n")
				po("client saw: '%s'", cli.LastMsgReceived())
			case <-time.After(time.Second * 5):
				po("\n\nWe waited 5 seconds, bailing.\n")
				srv.Stop()
				// should have gotten a message from the server at the client by now.

				panic("should have gotten a message from the server at the client by now!")
				cv.So(cli.LastMsgReceived(), cv.ShouldEqual, msg)
			}

			cv.So(cli.LastMsgReceived(), cv.ShouldEqual, msg)
		})
	})
}

// The actual point of longpoll_test.go is to run this 010 round-trip test for low-latency.
func TestLongPollToGetLowLatency010(t *testing.T) {

	cli, srv, rev, fwd, err := StartTestSystemWithBcast()
	panicOn(err)
	defer srv.Stop()
	defer rev.Stop()
	defer fwd.Stop()

	<-srv.FirstClient
	po("got past <-srv.FirstClient\n")

	cli.Start()
	defer cli.Stop()

	<-srv.SecondClient
	po("got past <-srv.SecondClient\n")

	// must do this, or else we'll race since there are like 3 PortIsBound checks that happen.
	<-srv.FirstHelloClient
	po("got past <-srv.FirstHelloClient\n")

	msg := "BREAKING NEWS"
	srv.Bcast(msg)

	cv.Convey("Given a ForwardProxy and a ReverseProxy communicating over http, in order to acheive low-latency sends from server to client, long-polling with two sockets (one send and one receive) should be used. So a server that has something to send (e.g. broadcast here) should be able to send back to the client immediately.\n", t, func() {

		// wait for message to get from server to client.
		select {
		case <-cli.MsgRecvd:
			// excllent
			po("excellent, cli got a message!\n")
			po("client saw: '%s'", cli.LastMsgReceived())
		case <-time.After(time.Second * 5):
			po("\n\nWe waited 5 seconds, bailing.\n")
			srv.Stop()
			// should have gotten a message from the server at the client by now.

			panic("should have gotten a message from the server at the client by now!")
			cv.So(cli.LastMsgReceived(), cv.ShouldEqual, msg)
		}

		cv.So(cli.LastMsgReceived(), cv.ShouldEqual, msg)
	})
}

func TestLongPollToGetLowLatency01a(t *testing.T) {

	// no port is bound checks! works!
	cli, srv, rev, fwd, err := StartTestSystemWithBcastNoPortIsBoundChecks()
	panicOn(err)

	defer rev.Stop()

	// a little out of order shutdown, for cleaner hang diagnostics.
	defer srv.Stop()

	defer fwd.Stop()

	<-srv.FirstClient
	po("got past <-srv.FirstClient\n")

	cli.Start()
	defer cli.Stop()

	<-srv.SecondClient
	po("got past <-srv.SecondClient\n")

	<-srv.FirstHelloClient
	po("got past <-srv.FirstHelloClient\n")

	msg := "BREAKING NEWS"
	srv.Bcast(msg)

	cv.Convey("Given a ForwardProxy and a ReverseProxy communicating over http, in order to acheive low-latency sends from server to client, long-polling with two sockets (one send and one receive) should be used. So a server that has something to send (e.g. broadcast here) should be able to send back to the client immediately.\n", t, func() {

		// wait for message to get from server to client.
		select {
		case <-cli.MsgRecvd:
			// excllent
			po("excellent, cli got a message!\n")
			po("client saw: '%s'", cli.LastMsgReceived())
		case <-time.After(time.Second * 5):
			po("\n\nWe waited 5 seconds, bailing.\n")
			srv.Stop()
			// should have gotten a message from the server at the client by now.

			panic("should have gotten a message from the server at the client by now!")
			cv.So(cli.LastMsgReceived(), cv.ShouldEqual, msg)
		}

		cv.So(cli.LastMsgReceived(), cv.ShouldEqual, msg)
	})
}

// next test to write, after 016:
// + growing number of bytes in messages get returned.
// + client sends 2, server sends 3, client sends 4, server sends 5 msgs in a row.
//
// + if there are no bytes to send, then the client and the server should remain idle
//   until the long-poll timeout (30 sec by default, can be smaller for testing).
//
func TestLongPollKeepsFifoOrdering016(t *testing.T) {

	cli, srv, rev, fwd, err := StartTestSystemWithCountingServer()
	panicOn(err)
	defer srv.Stop()
	defer rev.Stop()
	defer fwd.Stop()

	cv.Convey("Test ordering discipline for long-polling: Given a ForwardProxy and a ReverseProxy communicating over http, in order to acheive low-latency sends from server to client, long-polling with two sockets (one send and one receive) should be used. The two long polling sockets should still observe a first-to-send is first-to-get-a-reply order discipline.\n", t, func() {
		cli.Start()
		defer cli.Stop()
		<-srv.FirstCountingClientSeen

		po("got past <-srv.FirstCountingClientClient\n")

		msg := ""

		// fetch 10 messages
		N := 10
		for i := 0; i < N; i++ {
			select {
			case msg := <-cli.MsgRecvd:
				// excellent
				po("CountingTestClient got a message! client saw: '%s'", msg)
			case <-time.After(time.Second * 2):
				po("CountingTestClient: We waited 2 seconds, bailing.")
				srv.Stop()
				// should have gotten a message from the server at the client by now.

				panic("should have gotten a message from the server at the client by now!")
			}
		}

		po("TestLongPollKeepsFifoOrdering016: done with %d roundtrips", N)
		cv.So(msg, cv.ShouldEqual, msg)

		po("client history:")
		cli.ShowTmHistory()

		po("server history:")
		srv.ShowTmHistory()

		time.Sleep(10 * time.Second)

		po("client history after 10 sec sleep:")
		cli.ShowTmHistory()

		po("server history after 10 sec sleep:")
		srv.ShowTmHistory()

	})

}

// dup of 10 we can modify
func TestLongPollToGetLowLatency111dup(t *testing.T) {

	// start broadcast server (to test long-poll functionality/server initiated message)
	srv := NewBcastServer(Addr{})
	srv.Start()

	// start a reverse proxy
	rev := NewReverseProxy(ReverseProxyConfig{Dest: srv.Listen})
	rev.Start()

	// these checks actually stress out the system in a way that made it fail
	// so we maintain a with and without check version for now. (See below
	// in StartTestSystemWithBcastNoPortIsBoundChecks() for the version without
	// the PortIsBound() checks.
	//
	// this one by itself doesn't mess things up.
	//if !PortIsBound(rev.Cfg.Listen.IpPort) {
	//panic("rev proxy not up")
	//}

	// start the forward proxy, talks to the reverse proxy.
	fwd := NewPelicanSocksProxy(PelicanSocksProxyConfig{
		Dest: rev.Cfg.Listen,

		// if these are shorter than the long-poll interval, or
		// too short in general, it can make problems. like roundtrip
		// test 010 not passing.
		ChaserCfg: ChaserConfig{
			ConnectTimeout:   60000 * time.Millisecond,
			TransportTimeout: 60000 * time.Millisecond},
	})
	fwd.Start()

	// this one is sufficient to mess it up by itself
	if !PortIsBound(fwd.Cfg.Listen.IpPort) {
		panic("fwd proxy not up")
	}

	// start broadcast client (to test receipt of long-polled data from server)
	cli := NewBcastClient(Addr{Port: fwd.Cfg.Listen.Port})

	defer srv.Stop()
	defer rev.Stop()
	defer fwd.Stop()

	<-srv.FirstClient
	po("got past <-srv.FirstClient\n")

	cli.Start()
	defer cli.Stop()

	<-srv.SecondClient
	po("got past <-srv.SecondClient\n")

	<-srv.FirstHelloClient
	po("got past <-srv.FirstHelloClient\n")

	msg := "BREAKING NEWS"
	srv.Bcast(msg)

	cv.Convey("Given a ForwardProxy and a ReverseProxy communicating over http, in order to acheive low-latency sends from server to client, long-polling with two sockets (one send and one receive) should be used. So a server that has something to send (e.g. broadcast here) should be able to send back to the client immediately.\n", t, func() {

		// wait for message to get from server to client.
		select {
		case <-cli.MsgRecvd:
			// excllent
			po("excellent, cli got a message!\n")
			po("client saw: '%s'", cli.LastMsgReceived())
		case <-time.After(time.Second * 5):
			po("\n\nWe waited 5 seconds, bailing.\n")
			srv.Stop()
			// should have gotten a message from the server at the client by now.

			panic("should have gotten a message from the server at the client by now!")
			cv.So(cli.LastMsgReceived(), cv.ShouldEqual, msg)
		}

		cv.So(cli.LastMsgReceived(), cv.ShouldEqual, msg)
	})
}
