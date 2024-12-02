package utils

import (
	"github.com/chanfun-ren/executor/pkg/logging"
	"net"
)

// Get preferred outbound ip of this machine
func GetOutboundIP() net.IP {
	var log = logging.DefaultLogger()
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		log.Fatalw("Failed to get outbound IP", "error", err)
	}
	defer conn.Close()

	localAddr := conn.LocalAddr().(*net.UDPAddr)

	return localAddr.IP
}
