package main

import (
	"bytes"
	"fmt"
	"math/rand"
	"sync"
	"time"

	"github.com/glycerine/go-goon"
)

type Chaser struct {
	reqStop chan bool
	Done    chan bool

	incoming    chan []byte
	repliesHere chan []byte

	alphaIsHome bool
	betaIsHome  bool

	alphaArrivesHome chan bool
	betaArrivesHome  chan bool

	alphaDone   chan bool
	betaDone    chan bool
	monitorDone chan bool

	closedChan chan bool
	home       *ClientHome

	key  string
	dest Addr

	notifyDone chan *Chaser
	skipNotify bool

	mut sync.Mutex
	cfg ChaserConfig

	// shutdown after a period on non-use
	shutdownInactiveDur time.Duration
	inactiveTimer       *time.Timer
	lastActiveTm        time.Time
	mutTimer            sync.Mutex

	lp2ab chan *PelicanPacket
	ab2lp chan *tunnelPacket

	tmLastRecv []time.Time
	tmLastSend []time.Time

	hist *HistoryLog
	name string

	nextSendSerialNumber     int64
	lastRecvSerialNumberSeen int64

	misorderedReplies map[int64]*PelicanPacket

	NumChasers int // used to be alpha and beta, but now just alpha0..N

}

type ChaserConfig struct {
	ConnectTimeout      time.Duration
	TransportTimeout    time.Duration
	BufSize             int
	ShutdownInactiveDur time.Duration
}

func DefaultChaserConfig() *ChaserConfig {
	return &ChaserConfig{
		ConnectTimeout:      2000 * time.Millisecond,
		TransportTimeout:    60 * time.Second,
		BufSize:             4096,
		ShutdownInactiveDur: 10 * time.Minute,
	}
}

func SetChaserConfigDefaults(cfg *ChaserConfig) {
	def := DefaultChaserConfig()
	if cfg.ConnectTimeout == 0 {
		cfg.ConnectTimeout = def.ConnectTimeout
	}
	if cfg.TransportTimeout == 0 {
		cfg.TransportTimeout = def.TransportTimeout
	}
	if cfg.BufSize == 0 {
		cfg.BufSize = def.BufSize
	}
	if cfg.ShutdownInactiveDur == 0 {
		cfg.ShutdownInactiveDur = def.ShutdownInactiveDur
	}
}

func NewChaser(
	cfg ChaserConfig,
	incoming chan []byte,
	repliesHere chan []byte,
	ab2lp chan *tunnelPacket,
	lp2ab chan *PelicanPacket) *Chaser {

	SetChaserConfigDefaults(&cfg)

	s := &Chaser{
		lp2ab:   lp2ab,
		ab2lp:   ab2lp,
		reqStop: make(chan bool),
		Done:    make(chan bool),

		alphaDone:   make(chan bool),
		betaDone:    make(chan bool),
		monitorDone: make(chan bool),
		incoming:    incoming,    // from upstream
		repliesHere: repliesHere, // to upstream
		alphaIsHome: true,
		betaIsHome:  true,
		closedChan:  make(chan bool),
		home:        NewClientHome(),
		cfg:         cfg,

		shutdownInactiveDur: cfg.ShutdownInactiveDur,
		inactiveTimer:       time.NewTimer(cfg.ShutdownInactiveDur),
		tmLastSend:          make([]time.Time, 0),
		tmLastRecv:          make([]time.Time, 0),

		hist:                 NewHistoryLog("Chaser"),
		name:                 "Chaser",
		nextSendSerialNumber: 1,
		misorderedReplies:    make(map[int64]*PelicanPacket),
	}

	// always closed
	close(s.closedChan)

	return s
}

func (s *Chaser) ResetActiveTimer() {
	s.mutTimer.Lock()
	defer s.mutTimer.Unlock()
	s.inactiveTimer.Reset(s.shutdownInactiveDur)
	s.lastActiveTm = time.Now()
}

func (s *Chaser) Start() {
	s.home.Start()
	s.startAlpha()
	s.startBeta(1, "beta")
}

// Stops without reporting anything on the
// notifyDone channel passed to NewChaser().
func (s *Chaser) StopWithoutReporting() {
	s.skipNotify = true
	s.Stop()
}

// Stop the Chaser.
func (s *Chaser) Stop() {
	//po("%p Chaser stopping.\n", s)

	s.RequestStop()

	<-s.alphaDone
	<-s.betaDone
	s.home.Stop()

	//po("%p chaser all done.\n", s)
	close(s.Done)
}

// RequestStop makes sure we only close
// the s.reqStop channel once. Returns
// true iff we closed s.reqStop on this call.
func (s *Chaser) RequestStop() bool {
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

func (s *Chaser) startAlpha() {
	go func() {
		//po("%p alpha at top of startAlpha", s)

		defer func() {
			close(s.alphaDone)
			//po("%p Alpha done.", s)
		}()

		var work []byte
		var goNow bool
		for {
			work = []byte{}

			select {
			case goNow = <-s.home.shouldAlphaGoNow:
			case <-s.reqStop:
				//po("%p Alpha got s.reqStop", s)
				return
			}
			po("%p Alpha got goNow = %v", s, goNow)

			if !goNow {

				// only I am home, so wait for an event.
				select {
				case work = <-s.incoming:
					po("%p alpha got work on s.incoming: '%s'.\n", s, string(work))

					// launch with the data in work
				case <-s.reqStop:
					//po("%p Alpha got s.reqStop", s)
					return
				case <-s.betaDone:
					//po("%p Alpha got s.betaDone", s)
					return
				case <-s.home.tellAlphaToGo:
					po("%p alpha got s.home.tellAlphaToGo.\n", s)

					// we can launch without data, but
					// make sure there isn't some data waiting,
					// check again just so the random
					// nature of select won't hurt data deliver rates.
					select {
					case work = <-s.incoming:
					default:
						// don't block on it through, go ahead with empty data
						// if we don't have any.
					}
				case <-s.inactiveTimer.C:
					po("%p alpha got <-s.inactiveTimer.C, after %v: returning/shutting down.", s, s.shutdownInactiveDur)
					return
				}
			}

			if len(work) > 0 {
				// actual bytes to send!
				s.ResetActiveTimer()
			}
			// else must send out anyway, since we may be just long-polling for
			// keep-alive and server sending purposes.

			// send request to server
			s.home.alphaDepartsHome <- true

			// ================================
			// request-response cycle here
			// ================================

			replyBytes, recvSerial, err := s.DoRequestResponse(work, "")
			if err != nil {
				po("%p alpha aborting on error from DoRequestResponse: '%s'", s, err)
				return
			}
			po("%p alpha DoRequestResponse done; recvSerial = %d.\n", s, recvSerial)

			// if Alpha is here, tell him to head out.
			s.home.alphaArrivesHome <- true

			if len(replyBytes) > 0 {
				s.ResetActiveTimer()

				by := bytes.NewBuffer(replyBytes)

				tryMe := recvSerial + 1
				for {
					if !s.addIfPresent(&tryMe, by) {
						break
					}
				}
				sendMe := by.Bytes()

				// deliver any response data (body) to our client, but only
				// bother if len(replyBytes) > 0, as checked above.
				select {
				case s.repliesHere <- sendMe:
					po("*p Alpha sent to repliesHere: '%s'", string(sendMe))
				case <-s.reqStop:
					//po("%p Alpha got s.reqStop", s)
					return
				}
			}
		}
	}()
}

// Beta is responsible for the second http
// connection.
func (s *Chaser) startBeta(id int, name string) {
	go func() {
		//po("%p beta at top of startBeta", s)
		defer func() {
			close(s.betaDone)
			//po("%p Beta done.", s)
		}()
		var work []byte
		var goNow bool
		for {
			work = []byte{}

			select {
			case goNow = <-s.home.shouldBetaGoNow:
				po("%p Beta got goNow = %v", s, goNow)
			case <-s.reqStop:
				//po("%p Beta got s.reqStop", s)
				return
			}

			if !goNow {

				select {

				case work = <-s.incoming:
					po("%p beta got work on s.incoming '%s'.\n", s, string(work))
					// launch with the data in work
				case <-s.reqStop:
					//po("%p Beta got s.reqStop", s)
					return
				case <-s.alphaDone:
					return
				case <-s.home.tellBetaToGo:
					po("%p beta got s.home.tellBetaToGo.\n", s)

					// we can launch without data, but
					// make sure there isn't some data waiting,
					// check again just so the random
					// nature of select won't hurt data deliver rates.
					select {
					case work = <-s.incoming:
					default:
						// don't block on it through, go ahead with empty data
						// if we don't have any.
					}
				}
			}

			if len(work) > 0 {
				s.ResetActiveTimer()
			}

			// send request to server
			s.home.betaDepartsHome <- true

			// ================================
			// request-response cycle here
			// ================================

			replyBytes, recvSerial, err := s.DoRequestResponse(work, "")
			if err != nil {
				po("%p beta aborting on error from DoRequestResponse: '%s'", s, err)
				return
			}
			po("%p beta DoRequestResponse done; recvSerial = %d.\n", s, recvSerial)

			// if Alpha is here, tell him to head out.
			s.home.betaArrivesHome <- true

			if len(replyBytes) > 0 {
				s.ResetActiveTimer()

				by := bytes.NewBuffer(replyBytes)

				tryMe := recvSerial + 1
				for {
					if !s.addIfPresent(&tryMe, by) {
						break
					}
					tryMe++
				}
				sendMe := by.Bytes()

				// deliver any response data (body) to our client, but only
				// bother if len(replyBytes) > 0, as checked above.
				select {
				case s.repliesHere <- sendMe:
					po("*p Beta sent to repliesHere: '%s'", string(sendMe))
				case <-s.reqStop:
					//po("%p Beta got s.reqStop", s)
					return
				}
			}
		}
	}()
}

// sleep for some random interval to simulate time to server and back.
func rsleep() {
	time.Sleep(time.Duration(rand.Intn(2000)) * time.Millisecond)
}

type who int

const Alpha who = 1
const Beta who = 2
const Both who = 3

type ClientHome struct {
	reqStop chan bool
	Done    chan bool

	alphaArrivesHome chan bool
	betaArrivesHome  chan bool

	alphaDepartsHome chan bool
	betaDepartsHome  chan bool

	//	alphaShutdown chan bool
	//	betaShutdown  chan bool

	// for measuring latency under simulation
	localWishesToSend chan bool

	shouldAlphaGoNow chan bool
	shouldBetaGoNow  chan bool

	tellBetaToGo  chan bool
	tellAlphaToGo chan bool

	alphaHome bool
	betaHome  bool

	//	alphaShut bool
	//	betaShut  bool

	shouldAlphaGoCached bool
	shouldBetaGoCached  bool

	lastHome who

	localReqArrTm  int64
	latencyHistory []int64
	mut            sync.Mutex

	alphaRTT []time.Duration
	betaRTT  []time.Duration

	lastAlphaDepart time.Time
	lastBetaDepart  time.Time
}

func NewClientHome() *ClientHome {

	s := &ClientHome{
		reqStop: make(chan bool),
		Done:    make(chan bool),

		alphaArrivesHome: make(chan bool),
		betaArrivesHome:  make(chan bool),

		alphaDepartsHome: make(chan bool),
		betaDepartsHome:  make(chan bool),

		//alphaShutdown: make(chan bool),
		//betaShutdown:  make(chan bool),

		shouldAlphaGoNow: make(chan bool),
		shouldBetaGoNow:  make(chan bool),

		tellBetaToGo:  make(chan bool),
		tellAlphaToGo: make(chan bool),

		localWishesToSend: make(chan bool),

		shouldAlphaGoCached: true,
		shouldBetaGoCached:  false,

		alphaHome: true,
		betaHome:  true,

		alphaRTT: make([]time.Duration, 0),
		betaRTT:  make([]time.Duration, 0),
	}
	return s
}

func (s *ClientHome) Stop() {
	//po("%p client home stop requested", s)
	s.RequestStop()
	<-s.Done
}

// RequestStop makes sure we only close
// the s.reqStop channel once. Returns
// true iff we closed s.reqStop on this call.
func (s *ClientHome) RequestStop() bool {
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

func (s *ClientHome) String() string {
	return fmt.Sprintf("home:{alphaHome: %v, betaHome: %v} / ptr=%p", s.alphaHome, s.betaHome, s)
}

func (s *ClientHome) Start() {
	po("%p home starting.", s)

	go func() {
		defer func() {
			po("%p home done.", s)
		}()
		for {
			select {

			case <-s.alphaArrivesHome:
				now := time.Now()
				adur := now.Sub(s.lastAlphaDepart)
				s.alphaRTT = append(s.alphaRTT, adur)

				// for latency study
				if s.localReqArrTm > 0 {
					lat := now.UnixNano() - s.localReqArrTm
					s.latencyHistory = append(s.latencyHistory, lat)
					fmt.Printf("\n latency: %v\n", lat)
					s.localReqArrTm = 0
				}

				s.alphaHome = true

				VPrintf("++++  home received alphaArrivesHome. state of Home= '%s'\n", s)

				s.lastHome = Alpha
				if s.betaHome {
					select {
					case s.tellBetaToGo <- true:
					default:
					}
				}
				s.update()
				VPrintf("++++  end of alphaArrivesHome. state of Home= '%s'\n", s)

			case <-s.betaArrivesHome:
				now := time.Now()
				adur := now.Sub(s.lastBetaDepart)
				s.betaRTT = append(s.betaRTT, adur)

				// for latency study
				if s.localReqArrTm > 0 {
					lat := time.Now().UnixNano() - s.localReqArrTm
					s.latencyHistory = append(s.latencyHistory, lat)
					fmt.Printf("\n latency: %v\n", lat)
					s.localReqArrTm = 0
				}
				s.betaHome = true
				VPrintf("++++  home received betaArrivesHome. state of Home= '%s'\n", s)

				s.lastHome = Beta
				if s.alphaHome {
					select {
					case s.tellAlphaToGo <- true:
					default:
					}
				}
				s.update()
				VPrintf("++++  end of betaArrivesHome. state of Home= '%s'\n", s)

			case <-s.alphaDepartsHome:
				s.lastAlphaDepart = time.Now()
				s.alphaHome = false
				s.update()
				VPrintf("----  home received alphaDepartsHome. state of Home= '%s'\n", s)

			case <-s.betaDepartsHome:
				s.lastBetaDepart = time.Now()
				s.betaHome = false
				s.update()
				VPrintf("----  home received betaDepartsHome. state of Home= '%s'\n", s)

			case s.shouldAlphaGoNow <- s.shouldAlphaGoCached:

			case s.shouldBetaGoNow <- s.shouldBetaGoCached:

			case <-s.reqStop:
				po("%p home got s.reqStop", s)
				close(s.Done)
				return

			case <-s.localWishesToSend:
				// for measuring latency under simulation
				s.localReqArrTm = time.Now().UnixNano()
				if s.numHome() > 0 {
					s.latencyHistory = append(s.latencyHistory, 0)
					fmt.Printf("\n latency: %v\n", time.Duration(0))
					s.localReqArrTm = 0 // send done instantly, reset to indicate no pending send.
				}
			}
		}
	}()
}

func (s *ClientHome) shouldAlphaGo() (res bool) {
	if s.numHome() == 2 {
		return true
	}
	return false
}

func (s *ClientHome) shouldBetaGo() (res bool) {
	// in case of tie, arbitrarily alpha goes first.
	return false
}

func (s *ClientHome) numHome() (res int) {
	if s.alphaHome && s.betaHome {
		return 2
	}
	if s.alphaHome || s.betaHome {
		return 1
	}
	return 0
}

func (s *ClientHome) update() {
	s.shouldAlphaGoCached = s.shouldAlphaGo()
	s.shouldBetaGoCached = s.shouldBetaGo()
}

// unsafe/racy: use only after Chaser is shutdown
func (home *ClientHome) GetAlphaRoundtripDurationHistory() (artt []time.Duration) {
	return home.alphaRTT
	/*
		select {
		case artt = <-s.CopyAlphaRTT:
		case <-home.reqStop:
		}
		return
	*/
}

// unsafe/racy: use only after Chaser is shutdown
func (home *ClientHome) GetBetaRoundtripDurationHistory() (brtt []time.Duration) {
	return home.betaRTT
	/*
		select {
		case brtt = <-s.CopyBetaRTT:
		case <-home.reqStop:
		}
		return
	*/
}

// unsafe/racy: use only after Chaser is shutdown
func (home *ClientHome) LocalSendLatencyHistory() []int64 {
	return home.latencyHistory
}

func (s *Chaser) DoRequestResponse(work []byte, urlPath string) (back []byte, recvSerial int64, err error) {

	// only assign serial numbers to client payload, not to internal zero-byte
	// alpha/beta requests that are there just to give the server a reply medium.
	reqSer := int64(-1)
	if len(work) > 0 {
		reqSer = s.getNextSendSerNum()
	}

	var ppResp *PelicanPacket
	pack := NewTunnelPacket(reqSer, -1, s.key)
	pack.resp = NewMockResponseWriter()
	pack.respdup = new(bytes.Buffer)
	pack.AddPayload(request, work)

	po("%p Chaser.DoRequestResponse() about to do initial request with packet.requestSerial: %d, work/pack.reqBody: '%s'", s, reqSer, string(work))

	select {
	case s.ab2lp <- pack:
		s.NoteTmSent()

	case <-s.reqStop:
		po("Chaser reqStop before ab2lp request to lp issued")
		return
	}

	select {
	case ppResp = <-s.lp2ab:
		fmt.Printf("\n\n ab.go: pack <- s.lp2ab got:\n")
		goon.Dump(ppResp)

		ppResp.SetAbTm()

		recvSerial = -1 // default for empty bytes in body
		//		if len(body) >= SerialLen {
		if len(ppResp.Body) > 0 {
			recvSerial = ppResp.Serialnum
		}

		po("DoRequestResponse got from lp2ab: '%s', with recvSerial=%d", string(back), ppResp.Serialnum)
		s.NoteTmRecv()
	case <-s.reqStop:
		po("Chaser reqStop before lp2ab reply received")
		return
	}

	if recvSerial >= 0 {
		// adjust s.lastRecvSerialNumberSeen and s.misorderedReplies under lock
		s.mut.Lock()
		// do 2x manually below instead of: defer s.mut.Unlock()

		if recvSerial != s.lastRecvSerialNumberSeen+1 {

			s.misorderedReplies[recvSerial] = ppResp
			// wait to send upstream: indicate this by giving back 0 length.
			back = back[:0]
			s.mut.Unlock()
		} else {
			s.lastRecvSerialNumberSeen++
			s.mut.Unlock()

			// okay to reply in back with this data:
			if len(ppResp.Body) == 1 {
				back = ppResp.Body[0].Payload
			} else {
				// put together the multiple parts in Body
				n := 0
				for i := 0; i < len(ppResp.Body); i++ {
					n += len(ppResp.Body[i].Payload)
					// sanity checks:
					if int64(len(ppResp.Body[i].Payload)) != ppResp.Body[i].Paysize {
						panic(fmt.Sprintf("len(ppResp.Body[i].Payload) != ppResp.Body[i].Paysize; %d != %d",
							len(ppResp.Body[i].Payload),
							ppResp.Body[i].Paysize))
					}
					if !ppResp.Body[i].Verifies() {
						panic(fmt.Sprintf("body %d '%#v' failed to verify", i, ppResp.Body[i]))
					}
				}
				back = make([]byte, n)
				wrote := 0
				for i := 0; i < len(ppResp.Body); i++ {
					wrote += copy(back[wrote:], ppResp.Body[i].Payload)
				}
			}
			// end reply in back with this data
		}
	}

	return
}

/// logging

func (r *Chaser) NoteTmRecv() {
	r.mut.Lock()
	defer r.mut.Unlock()
	r.tmLastRecv = append(r.tmLastRecv, time.Now())
}

func (r *Chaser) NoteTmSent() {
	r.mut.Lock()
	defer r.mut.Unlock()
	r.tmLastSend = append(r.tmLastSend, time.Now())
}

func (r *Chaser) ShowTmHistory() {
	r.mut.Lock()
	defer r.mut.Unlock()
	po("Chaser.ShowTmHistory() called.")

	nr := len(r.tmLastRecv)
	ns := len(r.tmLastSend)
	min := nr
	if ns < min {
		min = ns
	}

	fmt.Printf("%s history: ns=%d.  nr=%d.  min=%d.\n", r.name, ns, nr, min)

	for i := 0; i < ns; i++ {
		fmt.Printf("%s history of Send from AB to LP '%v'  \n",
			r.name,
			r.tmLastSend[i])
	}

	for i := 0; i < nr; i++ {
		fmt.Printf("%s history of Recv from LP at AB '%v'  \n",
			r.name,
			r.tmLastRecv[i])
	}

	for i := 0; i < min; i++ {
		fmt.Printf("%s history: elap: '%s'    Send '%v'   Recv '%v'    \n",
			r.name,
			r.tmLastSend[i].Sub(r.tmLastRecv[i]),
			r.tmLastSend[i],
			r.tmLastRecv[i])
	}

	for i := 0; i < min-1; i++ {
		fmt.Printf("%s history: send-to-send elap: '%s'\n", r.name, r.tmLastSend[i+1].Sub(r.tmLastSend[i]))
	}
	for i := 0; i < min-1; i++ {
		fmt.Printf("%s history: recv-to-recv elap: '%s'\n", r.name, r.tmLastRecv[i+1].Sub(r.tmLastRecv[i]))
	}

}

func (s *Chaser) getNextSendSerNum() int64 {
	s.mut.Lock()
	defer s.mut.Unlock()
	v := s.nextSendSerialNumber
	s.nextSendSerialNumber++
	return v
}

// Helper for startAlpha/startBeta;
// returns true iff we found and deleted tryMe from the s.misorderedReplies map.
//  Along with the delete we write the contents of the found.response to 'by'.
func (s *Chaser) addIfPresent(tryMe *int64, by *bytes.Buffer) bool {
	s.mut.Lock()
	defer s.mut.Unlock()

	ooo, ok := s.misorderedReplies[*tryMe]

	if !ok {
		return false
	}
	for i := 0; i < len(ooo.Body); i++ {
		by.Write(ooo.Body[i].Payload)
	}
	po("ab reply misordering being corrected, reply sn: %d, by is now: '%s'", *tryMe, string(by.Bytes()))

	delete(s.misorderedReplies, *tryMe)
	s.lastRecvSerialNumberSeen = *tryMe
	(*tryMe)++

	return true
}

// replace with Pbody where Pbody.IsRequest = false, e.g. with , e.g. by call to NewResponsePbody()
type SerResp struct {
	response       []byte
	responseSerial int64 // order the sends with content by serial number
	tm             time.Time
}
