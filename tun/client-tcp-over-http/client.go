package main

import (
	"flag"
	"fmt"
	"log"

	tun "github.com/mailgun/pelican-protocol/tun"
)

// print out shortcut
var po = tun.VPrintf

const bufSize = 1024

var (
	listenAddr   = flag.String("listen", ":2222", "local listen address")
	httpAddr     = flag.String("http", fmt.Sprintf("%s:%d", tun.ReverseProxyIp, tun.ReverseProxyPort), "remote tunnel server")
	tickInterval = flag.Int("tick", 250, "update interval (msec)") // orig: 250
)

func main() {
	flag.Parse()
	log.SetPrefix("tun.client: ")

	lis, err := tun.NewAddr1(*listenAddr)
	if err != nil {
		panic(err)
	}
	des, err := tun.NewAddr1(*httpAddr)
	if err != nil {
		panic(err)
	}

	fwd := tun.NewPelicanSocksProxy(tun.PelicanSocksProxyConfig{
		Listen: *lis,
		Dest:   *des,
	})
	fwd.Start()
	select {}
}
