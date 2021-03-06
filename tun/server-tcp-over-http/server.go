package main

import (
	"flag"
	"fmt"

	tun "github.com/mailgun/pelican-protocol/tun"
)

//var destAddr = "127.0.0.1:12222" // tunnel destination
var destAddr = "127.0.0.1:22" // tunnel destination
//var destAddr = "127.0.0.1:1234" // tunnel destination

var listenAddr = flag.String("http", fmt.Sprintf("%s:%d", tun.ReverseProxyIp, tun.ReverseProxyPort), "http listen address")

func main() {
	flag.Parse()

	lis, err := tun.NewAddr1(*listenAddr)
	if err != nil {
		panic(err)
	}
	s := tun.NewReverseProxy(tun.ReverseProxyConfig{Listen: *lis})
	s.Start()
	select {}
}
