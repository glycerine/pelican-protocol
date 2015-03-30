package pelicantun

import (
	"bytes"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/glycerine/go-goon"
)

// A LongPoller (aka tunnel) is the server-side implementation
// of long-polling. We connect the http client (our pelican socks proxy)
// with the downstream target, typically an http server or sshd.
// For the client side implementation of long polling, see the
// file alphabeta.go and the Chaser structure and methods.
//
// Inside the reverse proxy, the LongPoller represents a 1:1, one
// client to one (downstream target) server connection,
// if you ignore the socks-proxy and reverse-proxy in the middle.
// A ReverseProxy can have many LongPollers, mirroring the number of
// connections on the client side to the socks proxy. The key
// distinguishes them. The LongerPoller is where we implement the
// server side of the long polling.
//
// http request flow (client initiating direction), http replies
// flow in the opposite direction of the arrows below.
//
//        "upstream"                               "downstream"
//           V                                         ^
//     e.g. web-browser                          e.g. web-server
//           |                                         ^
//           v                                         |
// -----------------------             -------------------------
// | TcpUpstreamReceiver |             |  net.Conn TCP connect |
// |    |                |             |               ^       |
// |    v                |             |           ServerRW    |
// | ClientRW            |             |               ^       |
// |    v                |    http     |               |       |
// | Chaser->alpha/beta->|------------>|WebServer--> LongPoller|
// -----------------------             -------------------------
//   pelican-socks-proxy                 pelican-reverse-proxy
//
//
type LongPoller struct {
	cfg LongPollerConfig

	reqStop           chan bool
	Done              chan bool
	ClientPacketRecvd chan *tunnelPacket

	rw        *ServerRW // manage the goroutines that read and write dnConn
	recvCount int
	conn      net.Conn

	// server issues a unique key for the connection, which allows multiplexing
	// of multiple client connections from this one ip if need be.
	// The ssh integrity checks inside the tunnel prevent malicious tampering.
	key     string
	pollDur time.Duration

	Dest Addr
	name string

	mut          sync.Mutex
	CloseKeyChan chan string
	lastUseTm    time.Time

	nextReplySerial             int64
	lastRequestSerialNumberSeen int64

	// save misordered requests here, to play
	// them back in the right order.
	misorderedRequests map[int64]*PelicanPacket

	// estimated time for RTT to downstream server,
	// getting this right can speed things up significantly
	fastWaitDur time.Duration

	// test reply packet re-ordering in AB by letting
	// the test request a re-numbering of the reply packets.
	// consumed until no more available, forceReplySn should
	// supply the serial numbers to be assigned replies.
	forceReplySn []int64

	tmLastSend []time.Time
	tmLastRecv []time.Time
}

type LongPollerConfig struct {
	Dest    Addr
	PollDur time.Duration
	Bufsz   int
}

// Make a new LongPoller as a part of the server (ReverseProxy is the server;
// PelicanSocksProxy is the client).
//
// If a CloseKeyChan receives a key, we return any associated client -> server
// http request immediately for that key, to facilitate quick shutdown.
//
func NewLongPoller(cfg LongPollerConfig) *LongPoller {
	key := GenPelicanKey()
	if cfg.Dest.Port == 0 {
		cfg.Dest.Port = GetAvailPort()
	}
	if cfg.Dest.Ip == "" {
		cfg.Dest.Ip = "0.0.0.0"
	}
	cfg.Dest.SetIpPort()

	po("NewLongPoller() originally, cfg.Bufsz = %d", cfg.Bufsz)
	if cfg.Bufsz == 0 {
		cfg.Bufsz = DefaultChaserConfig().BufSize
		po("NewLongPoller(): using cfg.Bufsz = %d, from DefaultChaserConfig().BufSize", cfg.Bufsz)
	}

	s := &LongPoller{
		cfg:                cfg,
		reqStop:            make(chan bool),
		Done:               make(chan bool),
		ClientPacketRecvd:  make(chan *tunnelPacket),
		tmLastSend:         make([]time.Time, 0),
		tmLastRecv:         make([]time.Time, 0),
		name:               "LongPoller",
		key:                string(key),
		Dest:               cfg.Dest,
		CloseKeyChan:       make(chan string),
		pollDur:            cfg.PollDur,
		nextReplySerial:    1,
		misorderedRequests: make(map[int64]*PelicanPacket),
		//fastWaitDur:        20 * time.Microsecond, // 0.352s with 1MB buffers
		//fastWaitDur: 5 * time.Millisecond, // locally 2.299s with 1MB buffers
		//fastWaitDur: 1 * time.Millisecond, // 0.772sec
		//fastWaitDur: 100 * time.Microsecond, // 0.374s
		fastWaitDur: 0, // 0.325s with 1MB buffers
	}

	// buffer sizes like DefaultChaserConfig().BufSize have a large impact on performance. set in DefaultChaserConfig() in alphabeta.go.
	po("\n *** using fastWaitDur: %v   DefaultChaserConfig().BufSize: %v\n\n", s.fastWaitDur, DefaultChaserConfig().BufSize)

	return s
}

func (s *LongPoller) Stop() {
	po("%p LongPoller stop received", s)
	s.RequestStop()
	<-s.Done
	po("%p LongPoller stop done", s)
}

// RequestStop makes sure we only close
// the s.reqStop channel once. Returns
// true iff we closed s.reqStop on this call.
func (s *LongPoller) RequestStop() bool {
	s.mut.Lock()
	defer s.mut.Unlock()

	select {
	case <-s.reqStop:
		return false
	default:
		close(s.reqStop)
		return true
	}
}

func (s *LongPoller) getReplySerial() int64 {
	s.mut.Lock()
	defer s.mut.Unlock()
	v := s.nextReplySerial
	s.nextReplySerial++
	return v
}

func (s *LongPoller) finish() {
	s.rw.Stop()
	close(s.Done)
}

// LongPoller::Start() implements the long-polling logic.
//
// When a new client request comes in (2nd one), we bump any
// already waiting long-poll into replying to its request.
//
//     new reader ------> bumps waiting/long-polling reader & takes its place.
//       ^                      |
//       |                      V
//       ^                      |
//       |                      V
//    client <-- returns to <---/
//
// it's a closed loop track with only one goroutine per tunnel
// actively holding on a long poll.
//
// There are only ever two client (http) requests outstanding
// at any given moment in time.
//
func (s *LongPoller) Start() error {

	skey := string(s.key[:5])

	err := s.dial()
	if err != nil {
		return fmt.Errorf("%s '%s' LongPoller could not dial '%s': '%s'", s, skey, s.Dest.IpPort, err)
	}

	// s.dial() sets s.conn on success.
	s.rw = NewServerRW("ServerRW/LongPoller", s.conn, 1024*1024, nil, nil, s)
	s.rw.Start()

	go func() {
		defer func() { s.finish() }()

		// duration of the long poll
		var longPollTimeUp *time.Timer
		if int64(s.pollDur) > 0 {
			longPollTimeUp = time.NewTimer(s.pollDur)
		} else {
			// s.pollDur is 0, so do not do the long-poll
			// timer at all. useful for tests.
			longPollTimeUp = time.NewTimer(24 * time.Hour)
			longPollTimeUp.Stop()
		}

		var pack *tunnelPacket
		var wasOutOfOrder bool

		// set this to finish re-ordering a packet. Return to nil when
		// done writing the re-ordered packet.
		coalescedSequence := make([]*Pbody, 0)

		// in cliReq and bytesFromServer, the client is upstream and the
		// server is downstream. In LongPoller, we read from the server
		// and write those bytes in Replies to the client. In LongPoller, we read
		// from the client Requests and write those bytes to the server.

		// keep at most 2 cliRequests on hand, cycle them in FIFO order.
		// they are: oldestReqPack, and waitingCliReqs[0], in that order.

		waiters := NewRequestFifo(2)

		var countForUpstream int64

		// tries to send, and does if we have
		// a waiting request to send on.
		//
		// returns false iff we got s.reqStop
		// while trying to send.
		sendReplyUpstream := func() bool {
			if waiters.Empty() {
				return true
			}

			oldest := waiters.PopRight()

			po("%p '%s' longpoller sendReplyUpstream() is sending along oldest ClientRequest with response, countForUpstream(%d) >0 || len(waitingCliReqs)==%d was > 0", s, skey, countForUpstream, waiters.Len())

			if countForUpstream != oldest.ppResp.TotalPayloadSize() {
				panic(fmt.Sprintf("should never get here: countForUpstream is out of sync with oldest.ppResp.TotalPayloadSize(): %d == countForUpstream != len(oldest.ppResp.TotalPayloadSize() == %d", countForUpstream, oldest.ppResp.TotalPayloadSize()))
			}

			if countForUpstream > 0 {
				// last thing before the reply: append reply serial number, to allow
				// correct ordering on the client end. But skip replySerialNumber
				// addition if this is an empty packet, because there will be lots of those.
				// That is why the countForUpstream > 0 condition in here.
				//
				rs := s.getReplySerial()
				oldest.ppResp.SetSerial(rs)

			} else {
				oldest.ppResp.SetSerial(-1)
			}

			// write capnp format to resp
			// already done: oldest.resp.Header().Set("Content-type", "application/octet-stream")
			err := oldest.ppResp.Save(oldest.resp)
			panicOn(err)

			// respdup is for testing
			oldest.respdup.Reset()
			err = oldest.ppResp.Save(oldest.respdup)
			panicOn(err)

			po("longpoll sending '%s'\n", string(oldest.ppResp.TotalPayload()))

			close(oldest.done) // send reply!
			countForUpstream = 0

			// debug
			if oldest.ppReq.Serialnum >= 0 {
				po("sending s.lp2ab <- oldest where oldest.respdup.Bytes() = '%s'. countForUpstream = %d. oldest.ppReq.Serialnum = %d", string(oldest.respdup.Bytes()[:countForUpstream]), countForUpstream, oldest.ppReq.Serialnum)
			} else {
				po("sending s.lp2ab <- oldest. countForUpstream = %d. oldest.ppReq.Serialnum = %d", countForUpstream, oldest.ppReq.Serialnum)
			}

			if waiters.Empty() {
				longPollTimeUp.Stop()
			}
			return true
		} // end sendReplyUpstream

		for {
			wasOutOfOrder = false
			po("%p '%s' longpoller: at top of LongPoller loop, inside Start(). len(wait)=%d", s, skey, waiters.Len())

			select {

			case <-longPollTimeUp.C:
				po("%p '%s' longPollTimeUp!!", s, skey)
				sendReplyUpstream()

			// Only receive if we have a waiting packet body to write to.
			// Otherwise let the RecvFromDownCh() do the fixed size buffering.
			case b500 := <-func() chan []byte {
				if !waiters.Empty() {
					return s.rw.RecvFromDownCh()
				} else {
					return nil
				}
			}():
				if len(b500) > 0 {
					s.lastUseTm = time.Now()
				}
				po("%p '%s' LongPoller got data from downstream <-s.rw.RecvFromDownCh() got b500='%s'\n", s, skey, string(b500))

				oldestReqPack := waiters.PeekRight()

				countForUpstream += int64(len(b500))
				oldestReqPack.ppResp.AppendPayload(b500, false)

				sendReplyUpstream()

			case pack = <-s.ClientPacketRecvd:
				s.recvCount++
				s.NoteTmRecv()
				po("%p  longPoller got client packet! recvCount now: %d", s, s.recvCount)

				if pack.ppReq.TotalPayloadSize() > 0 {
					s.lastUseTm = time.Now()
				}

				po("%p '%s' longPoller got client packet! recvCount now: %d", s, skey, s.recvCount)

				// ignore negative serials--they were just for getting
				// a server initiated reply medium. And we should never send
				// a zero serial -- they start at 1.
				if pack.ppReq.Serialnum > 0 {

					if !pack.ppReq.Verifies() {
						fmt.Printf("pack.ppReq on s.ab2lp did not verify checksum!: '%#v'\ngoon.Dump:\n", pack.ppReq)
						goon.Dump(pack.ppReq)
						panic("pack.ppReq on s.ab2lp did not verify checksum!")
					}

					if pack.ppReq.Serialnum != s.lastRequestSerialNumberSeen+1 {
						po("detected out of order pack %d, s.lastRequestSerialNumberSeen=%d",
							pack.ppReq.Serialnum, s.lastRequestSerialNumberSeen)
						// pack.ppReq.Serialnum is out of order

						// sanity check
						_, already := s.misorderedRequests[pack.ppReq.Serialnum]
						if already {
							panic(fmt.Sprintf("misordered request detected, but we already saw pack.ppReq.Serialnum =%d. Misorder because s.lastRequestSerialNumberSeen = %d which is not one less than pack.ppReq.Serialnum", pack.ppReq.Serialnum, s.lastRequestSerialNumberSeen))
						} else {
							// sanity check that we aren't too far off
							if pack.ppReq.Serialnum < s.lastRequestSerialNumberSeen {
								panic(fmt.Sprintf("duplicate request number from the past: pack.ppReq.Serialnum =%d < s.lastRequestSerialNumberSeen = %d", pack.ppReq.Serialnum, s.lastRequestSerialNumberSeen))
							}

							// the main action in the event of misorder detection:
							// store the misorder request until later, but still push onto waiters for replies below.
							s.misorderedRequests[pack.ppReq.Serialnum] = pack.ppReq
							wasOutOfOrder = true
						}
					} else {
						s.lastRequestSerialNumberSeen = pack.ppReq.Serialnum
					}
				} // end if pack.ppReq.Serialnum > 0

				// Data or note, we reset the poll timer, so that we only hold
				// this packet open on this end for at most 'dur' time.
				// Since we will be replying to oldestReqPack (if any) immediately,
				// we can reset the timer to reflect pack's arrival.
				longPollTimeUp.Reset(s.pollDur)

				// get the oldest packet, and reply using that.

				// our long-poll timer reflects the time since
				// the most recent packet arrival.

				// we save the SerReq part of pack above, so we can send along the
				// reply at any point. Thus (and become of this PushLeft) we do
				// first-Request-in-first-Response-out, although obviously not
				// necessarily waiting to transport the actual downstream response to any
				// given request.
				waiters.PushLeft(pack)

				// ===================================
				// got to here in the merge of little.go and longpoll.go
				// ===================================

				if pack.ppReq != nil && pack.ppReq.TotalPayloadSize() > 0 {
					po("%p '%s' LongPoller, just received ClientPacket with pack.ppReq.Body[0].Payload = '%s'\n", s, skey, string(pack.ppReq.Body[0].Payload))
				}

				// have to both send and receive

				pack.resp.Header().Set("Content-type", "application/octet-stream")

				po("%p '%s' just before s.rw.SendToDownCh()", s, skey)

				if !wasOutOfOrder && pack.ppReq.TotalPayloadSize() > 0 {
					// we got data from the client for server!
					// read from the request body and write to the ResponseWriter

					// append pack to where it belongs
					coalescedSequence = append(coalescedSequence, pack.ppReq.Body...)

					// *goes after* additions: check for any that can go in-order *after* pack
					lookFor := pack.ppReq.Serialnum + 1
					for {
						if oooPp, ok := s.misorderedRequests[lookFor]; ok {
							coalescedSequence = append(coalescedSequence, oooPp.Body...)
							delete(s.misorderedRequests, lookFor)
							s.lastRequestSerialNumberSeen = oooPp.Serialnum
							lookFor++
						} else {
							break
						}
					}
					// coalescedSequence will contain our buffers in order

					writeMe := pack.ppReq.Body[0].Payload

					// if we have more than pack, adjust writeMe to
					// encompass all buffers that are ready to go in-order now.
					if len(coalescedSequence) > 1 {
						// now concatenate all together for one send
						var allTogether bytes.Buffer
						for _, v := range coalescedSequence {
							allTogether.Write(v.Payload)
						}
						writeMe = allTogether.Bytes()
					}

					if len(writeMe) == 0 {
						panic("should be writing some bytes here, but len(writeMe) == 0")
					}

					select {
					// s.rw.SendToDownCh() is a 1000 buffered channel so okay to not use a timeout;
					// in fact we do want the back pressure to keep us from
					// writing too much too fast.
					case s.rw.SendToDownCh() <- writeMe:
						po("%p '%s' sent data '%s' on s.rw.SendToDownCh()", s, skey, string(writeMe))
					case <-s.reqStop:
						po("%p '%s' got reqStop, *not* returning", s, skey)
						// avoid deadlock on shutdown, but do
						// finish processing this packet, don't return yet
					}

					coalescedSequence = coalescedSequence[:0]
				} // end if len(pack.reqBody) > 0

				po("%p '%s' just after s.rw.SendToDownCh()", s, skey)

				// transfer data from server to client

				// TODO: instead of fixed 5msec, this threshold should be
				// 1x the one-way-trip time from the client-to-server, since that is
				// the expected additional alternative wait time if we have to reply
				// with an empty reply now.
				//
				// add any data from the next 10 msec to return packet to client
				// hence if the server replies quickly, we can reply quickly
				// to the client too.
				if int64(s.fastWaitDur) > 0 {

					startFastWaitTm := time.Now()
					select {
					case b500 := <-s.rw.RecvFromDownCh():
						po("%p '%s' longpoller  <-s.rw.RecvFromDownCh() got b500='%s' during fast-reply-wait (after %v)\n", s, skey, string(b500), time.Since(startFastWaitTm))

						oldest := waiters.PeekRight()

						countForUpstream += int64(len(b500))
						oldest.ppResp.AppendPayload(b500, false)

					case <-time.After(s.fastWaitDur): // slightly faster locally than 500 usec: 1.2sec.
						//case <-time.After(20 * time.Millisecond): // 1msec => 1.20s; same with 500 usec; 2msec-3msec => 1.09s, slightly faster. 4msec => 1.105sec 5msec => 1.10sec
						po("%p '%s' after 10msec of extra s.rw.RecvFromDownCh() reads", s, skey)

						// stop trying to read from server downstream, and send what
						// we got upstream to client.
					}
				}

				// key piece of logic for the long-poll is here:
				// reply immediately under two conditions: there
				// are bytes to send back upstream, or we have
				// more than one of the alpha/beta parked here.
				if countForUpstream > 0 || waiters.Len() > 1 {
					sendReplyUpstream()
				} else {
					po("%p '%s' LongPoll countForUpstream(%d); waiters.Len()==%d",
						s, skey, countForUpstream, waiters.Len())
				}

				// end case pack = <-s.ClientPacketRecvd:
			case <-s.reqStop:
				po("%p '%s' got reqStop, returning", s, skey)
				return
			case <-s.CloseKeyChan:
				po("%p '%s' LongPoller in nil packet state, got closekeychan. Shutting down.", s, skey)

				// empty out the oldest and wait queue, replying to zero, one, or both requests.
				p := waiters.PopRight()
				for p != nil {
					close(p.done)
					p = waiters.PopRight()
				}
				return
			} //end select
		} // end for

	}()

	return nil
}

func (s *LongPoller) dial() error {

	po("ReverseProxy::NewTunnel: Attempting connect to our target '%s'\n", s.Dest.IpPort)
	dialer := net.Dialer{
		Timeout:   1000 * time.Millisecond,
		KeepAlive: 30 * time.Second,
	}

	var err error
	s.conn, err = dialer.Dial("tcp", s.Dest.IpPort)
	switch err.(type) {
	case *net.OpError:
		if strings.HasSuffix(err.Error(), "connection refused") {
			// could not reach destination
			return err
		}
	default:
		panicOn(err)
	}

	return err
}

func (r *LongPoller) NoteTmRecv() {
	r.mut.Lock()
	defer r.mut.Unlock()
	r.tmLastRecv = append(r.tmLastRecv, time.Now())
}

func (r *LongPoller) NoteTmSent() {
	r.mut.Lock()
	defer r.mut.Unlock()
	r.tmLastSend = append(r.tmLastSend, time.Now())
}

func (r *LongPoller) ShowTmHistory() {
	r.mut.Lock()
	defer r.mut.Unlock()
	po("LongPoller.ShowTmHistory() called.")
	nr := len(r.tmLastRecv)
	ns := len(r.tmLastSend)
	min := nr
	if ns < min {
		min = ns
	}
	fmt.Printf("%s history: ns=%d.  nr=%d.  min=%d.\n", r.name, ns, nr, min)

	for i := 0; i < ns; i++ {
		fmt.Printf("%s history of Send from LP to AB '%v'  \n",
			r.name,
			r.tmLastSend[i])
	}

	for i := 0; i < nr; i++ {
		fmt.Printf("%s history of Recv from AB at LP '%v'  \n",
			r.name,
			r.tmLastRecv[i])
	}

	for i := 0; i < min; i++ {
		fmt.Printf("%s history: elap: '%s'    Recv '%v'   Send '%v'  \n",
			r.name,
			r.tmLastSend[i].Sub(r.tmLastRecv[i]),
			r.tmLastRecv[i], r.tmLastSend[i])
	}

	for i := 0; i < min-1; i++ {
		fmt.Printf("%s history: send-to-send elap: '%s'\n", r.name, r.tmLastSend[i+1].Sub(r.tmLastSend[i]))
	}
	for i := 0; i < min-1; i++ {
		fmt.Printf("%s history: recv-to-recv elap: '%s'\n", r.name, r.tmLastRecv[i+1].Sub(r.tmLastRecv[i]))
	}

}
