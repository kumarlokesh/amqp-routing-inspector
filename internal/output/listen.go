package output

import "net"

// listenTCP opens a TCP listener at addr. Extracted for testability.
func listenTCP(addr string) (net.Listener, error) {
	return net.Listen("tcp", addr)
}
