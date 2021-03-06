package main

import (
	"fmt"
	"log"
	"net"
	"time"

	pelican "github.com/mailgun/pelican-protocol"
	"golang.org/x/crypto/ssh"
)

type User struct {
	PubKey     string
	AcctId     string // 32-byte one-way crypto hash of PubKey. The 'user' who is ssh-ing in.
	Email      string
	SMS        string
	FirstName  string
	MiddleName string
	LastName   string
	FirstIp    string
	FirstTm    time.Time
	LastIp     string
	LastTm     time.Time
	Banned     bool
	Completed  bool // signup complete marked here.
}

type Users struct {
	KnownClientPubKey map[string]User
}

func NewUsers() *Users {
	u := &Users{
		KnownClientPubKey: make(map[string]User),
	}
	return u
}

// should be constant-time to avoid side-channel timing attacks.
func (u *Users) PermitClientConnection(clientUser string, clientAddr net.Addr, clientPubKey ssh.PublicKey) (bool, error) {

	pubBytes := ssh.MarshalAuthorizedKey(clientPubKey)
	strPubBytes := string(chomp(pubBytes))

	fmt.Printf("\n in PermitClientConnection(): clientUser = '%s', clientAddr = '%#v', clientPubKey = '%s'\n", clientUser, clientAddr, strPubBytes)

	if strPubBytes == pelican.GetOriginatorPublicKey() {
		fmt.Printf("PermitClientConnection detected Originator public key, returning true\n")
		return true, fmt.Errorf("new-account")
	}
	fmt.Printf("equal? %v \n pelican.GetOriginatorPublicKey() = \n'%s'\n and strPubBytes = \n'%s'\n", pelican.GetOriginatorPublicKey() == strPubBytes, pelican.GetOriginatorPublicKey(), strPubBytes)

	if clientUser == "newacct" {
		fmt.Printf("PermitClientConnection detected user 'newacct', returning true\n")
		return true, fmt.Errorf("new-account")
	}
	// the username is issued by the server upon the completion of the
	// new account protocol, and is the hmac of the client's public key
	// signed with a secret only the server knows. The secret used
	// to sign the hmac should be preserved as long as the service is
	// alive, since if you loose it none of the account names can
	// be validated.
	secretServerId, err := FetchSecretIdForService(".secret_id_for_service")
	panicOn(err)
	hmac := Sha1HMAC(pubBytes, []byte(secretServerId))
	acctid := encodeSha1HmacAsUsername(hmac)

	if clientUser != acctid {
		// somebody is trying to use a public key we've seen before, but
		// they did not receive our signature (acctid) for it, so likely
		// what happened is they didn't complete the new-account-creation
		// protocol. Hence we reject their connection.
		// The final step in completion of the new account protocol is that
		// we give the client their acctid (equivalent of a username);
		// which is an hmac of their public key, signed with a secret
		// known only to the server.
		return false, fmt.Errorf("bad-account-id")
	}

	user, ok := u.KnownClientPubKey[strPubBytes]
	if ok {
		if user.Banned {
			fmt.Printf("PermitClientConnection returning banned-user\n")
			return false, fmt.Errorf("banned-user")
		}
		now := time.Now()
		user.LastIp = clientAddr.String()
		user.LastTm = now
		if user.FirstIp == "" {
			user.FirstIp = user.LastIp
			user.FirstTm = now
		}
		fmt.Printf("PermitClientConnection true, user known.\n")
		return true, nil
	}

	fmt.Printf("PermitClientConnection false: user-unknown\n")
	return false, fmt.Errorf("user-unknown")
}

func main() {
	s := NewPelicanServer(&PelSrvCfg{PelicanListenPort: 8080, HttpDestIp: "127.0.0.1", HttpDestPort: 80})
	s.Start()
}

type PelicanServer struct {
	Users *Users
	Sshd  *ssh.ServerConfig
	Cfg   PelSrvCfg
}

type PelSrvCfg struct {
	PelicanListenIp   string
	PelicanListenPort int
	HttpDestIp        string
	HttpDestPort      int
	HttpDialTimeout   time.Duration
}

func NewPelicanServer(cfg *PelSrvCfg) *PelicanServer {

	s := &PelicanServer{
		Users: NewUsers(),
	}
	s.SetDefaults(cfg)

	config := &ssh.ServerConfig{
		// must have keys
		NoClientAuth: false,

		// pki based login only
		PublicKeyCallback: func(conn ssh.ConnMetadata, key ssh.PublicKey) (*ssh.Permissions, error) {
			ok, err := s.Users.PermitClientConnection(conn.User(), conn.RemoteAddr(), key)
			if !ok {
				return nil, err
			}
			perm := &ssh.Permissions{Extensions: map[string]string{
				"pubkey": string(key.Marshal()),
			}}
			return perm, nil
		},
		// no passwords
		PasswordCallback: nil,
	}

	err := GetOrGenServerKey("./host-key-id-rsa", config)
	panicOn(err)

	s.Sshd = config

	return s
}

func (s *PelicanServer) Start() {

	go func() {
		// Once a ServerConfig has been configured, connections can be accepted.
		addr := fmt.Sprintf("%s:%d", s.Cfg.PelicanListenIp, s.Cfg.PelicanListenPort)
		listener, err := net.Listen("tcp", addr)
		if err != nil {
			log.Fatalf("Failed to listen on '%s': %s", addr, err)
		}

		// Accept all connections
		log.Printf("pelican-server sshd component listening on '%s'...", addr)
		for {
			tcpConn, err := listener.Accept()
			if err != nil {
				log.Printf("Failed to accept incoming connection: '%s'", err)
				continue
			}
			// Before use, a handshake must be performed on the incoming net.Conn.
			sshConn, chans, reqs, err := ssh.NewServerConn(tcpConn, s.Sshd)
			if err != nil {
				log.Printf("Failed to handshake: '%s'", err)
				continue
			}

			log.Printf("New SSH connection from %s (%s)", sshConn.RemoteAddr(), sshConn.ClientVersion())

			go processRequests(sshConn, reqs)

			// Accept all channels, even if we Reject all but direct-tcpip.
			go handleChannels(chans, s.Cfg)
		}
	}()
}

func (s *PelicanServer) Stop() {
	fmt.Printf("todo: Pelican-Server:Stop() not implimented.\n")
}

func handleChannels(chans <-chan ssh.NewChannel, cfg PelSrvCfg) {
	for newChannel := range chans {
		go handleChannel(newChannel, cfg)
	}
}

func handleChannel(newChannel ssh.NewChannel, cfg PelSrvCfg) {

	fmt.Printf("\n in handleChannel() with channel type = '%s'\n", newChannel.ChannelType())

	// "direct-tcpip" is client -> server; "forwarded-tcpip" channel types is -R, server -> client
	t := newChannel.ChannelType()
	if t != "direct-tcpip" {
		newChannel.Reject(ssh.UnknownChannelType, fmt.Sprintf("only direct-tcpip allowed. channel of type '%s' not allowed.", t))
		return
	}

	// setup socket to forward this connection to port 80
	localConn, err := net.DialTimeout("tcp", fmt.Sprintf("%s:%d", cfg.HttpDestIp, cfg.HttpDestPort), cfg.HttpDialTimeout)
	if err != nil {
		newChannel.Reject(ssh.ConnectionFailed, fmt.Sprintf("failed to connect to server '%s' at port %d: %s", cfg.HttpDestIp, cfg.HttpDestPort, err))
		return
	}

	// todo: reject channels that are not specifying port 80 or port 443 traffic.
	//

	// At this point, we have the opportunity to reject the client's
	// request for another logical connection
	fromClient, requests, err := newChannel.Accept()
	if err != nil {
		log.Printf("Could not accept channel (%s)", err)
		return
	}

	fmt.Printf("\n sshd: Accept happened: fromClient = '%#v'\n  requests = '%#v'\n", fromClient, requests)

	// Sessions have out-of-band requests such as "shell", "pty-req" and "env" that
	// need to be rejected.
	go ssh.DiscardRequests(requests)
	// or, the same but with printf:
	/*
		go func() {
			for req := range requests {
				fmt.Printf("\n ignoring req.Type = '%v'\n", req.Type)
				if req.WantReply {
					req.Reply(false, nil)
				}
			}
		}()
	*/

	// here is the heart of the reverse proxy functionality:
	// reads on fromClient are forwarded to localConn
	// reads on localConn are forwarded to fromClient

	sp := pelican.NewShovelPair()
	sp.Start(fromClient, localConn, "fromClient<-localConn", "localConn<-fromClient")

	/*
		// Copy localConn.Reader to fromClient.Writer
		go func() {
			fmt.Printf("\n starting sshd Copy to fromClient from localConn\n")
			_, err := io.Copy(fromClient, localConn)
			if err != nil {
				fmt.Printf("io.Copy failed: %v\n", err)
				fromClient.Close()
				localConn.Close()
				fmt.Printf("\n returning from sshd Copy to fromClient from localConn\n")
				return
			}
		}()

		// Copy fromClient.Reader to localConn.Writer
		go func() {
			fmt.Printf("\n starting sshd Copy to localConn from fromClient\n")
			_, err := io.Copy(localConn, fromClient)
			if err != nil {
				fmt.Printf("io.Copy failed: %v\n", err)
				fromClient.Close()
				localConn.Close()
				fmt.Printf("\n returning from sshd Copy to localConn from fromClient\n")
				return
			}
		}()
	*/

	/* // older example
	       // some basic read/write testing:
	   	go func() {
	   		// fromClient is a ssh.Channel, an interface with Read() and Write() methods
	   		// that represents a bidirectional tcp stream being forwarded from the client.
	   		k := 0
	   		for {
	   			// just read a little bit
	   			by := make([]byte, 100)
	   			nbytes, err := fromClient.Read(by)
	   			if err != nil {
	   				if err.Error() == "EOF" {
	   					fmt.Printf("fromClient returned EOF on Read(): closing down.\n")
	   					fromClient.Close()
	   					return
	   				}
	   				fmt.Printf("fromClient returned error on Read(): '%s'", err)
	   			} else {
	   				fmt.Printf("sshd read over direct-tcp fromClient: %d bytes: '%s'\n", nbytes, string(by))
	   			}
	   			nw, err := fromClient.Write([]byte(fmt.Sprintf("writing this from sshd back to client: %d", k)))
	   			k++
	   			if err != nil {
	   				if err.Error() == "EOF" {
	   					fmt.Printf("fromClient returned EOF on Write(), closing down.\n")
	   					fromClient.Close()
	   					return
	   				}
	   				fmt.Printf("fromClient returned error on Write(): '%s'", err)
	   			} else {
	   				fmt.Printf("sshd write over direct-tcp fromClient succeeded: %d bytes.\n", nw)
	   			}
	   			fmt.Printf("sleeping for 2 sec.\n")
	   			time.Sleep(2 * time.Second)
	   			fmt.Printf("done with sleep.\n")
	   		}
	   	}()

	   	// write async
	   	go func() {
	   		k := -1
	   		for {

	   			nw, err := fromClient.Write([]byte(fmt.Sprintf("%d async writing this from sshd back to client.", k)))
	   			k--
	   			if err != nil {
	   				if err.Error() == "EOF" {
	   					fmt.Printf("fromClient returned EOF on Write(), closing down.")
	   					fromClient.Close()
	   					return
	   				}
	   				fmt.Printf("fromClient returned error on Write(): '%s'", err)
	   			} else {
	   				fmt.Printf("sshd write over direct-tcp fromClient succeeded: %d bytes.\n", nw)
	   			}
	   			fmt.Printf("async write sleeping for 1 sec.\n")
	   			time.Sleep(1 * time.Second)
	   		}
	   	}()
	*/

	fmt.Printf("\n sshd: returning from handleChannel() for channel type '%s'.\n", t)
}

func chomp(b []byte) []byte {
	for i := len(b) - 1; i >= 0; i-- {
		if b[i] != '\n' {
			return b[:i+1]
		}
	}
	return b[:0]
}

func processRequests(conn *ssh.ServerConn, reqs <-chan *ssh.Request) {
	fmt.Printf("\n in processRequests with conn = '%#v'\n", conn)

	for req := range reqs {
		fmt.Printf("\n in processRequests(), req.Type = '%#v'\n", req.Type)
		if req.Type != "direct-tcpip" {
			// accept only direct-tcpip requests
			if req.WantReply {
				req.Reply(false, nil)
			}
			continue
		}

		/*
			// Copy localConn.Reader to sshConn.Writer
			go func(sshConn net.Conn) {
				_, err := io.Copy(sshConn, channel)
				if err != nil {
					log.Println("io.Copy failed: %v", err)
					sshConn.Close()
					return
				}
			}(sshConn)
			// Copy sshConn.Reader to localConn.Writer
			go func(sshConn net.Conn) {
				_, err := io.Copy(channel, sshConn)
				if err != nil {
					log.Println("io.Copy failed: %v", err)
					sshConn.Close()
					return
				}
			}(sshConn)
		*/

		req.Reply(true, nil)
	}

	fmt.Printf("\n returning from sshd: processRequests\n")
}

func (s *PelicanServer) SetDefaults(cfg *PelSrvCfg) {
	if cfg != nil {
		s.Cfg = *cfg
	}

	if s.Cfg.HttpDialTimeout == 0 {
		s.Cfg.HttpDialTimeout = 5 * time.Second
	}

	if s.Cfg.PelicanListenIp == "" {
		s.Cfg.PelicanListenIp = "127.0.0.1"
	}

	if s.Cfg.PelicanListenPort == 0 {
		s.Cfg.PelicanListenPort = 8080
	}
	if s.Cfg.HttpDestIp == "" {
		s.Cfg.HttpDestIp = "127.0.0.1"
	}
	if s.Cfg.HttpDestPort == 0 {
		s.Cfg.HttpDestPort = 80
	}
}
