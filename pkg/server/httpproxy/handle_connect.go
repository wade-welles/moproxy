// Copyright 2019-2020 Moritz Fain
// Moritz Fain <moritz@fain.io>
package httpproxy

import (
	"moproxy/internal"

	"net"
	"net/http"
	"strconv"
)

func handleConnectMethod(conn *httpClientConn) {
	remoteAddr := &internal.RemoteAddr{
		TCPAddr:    new(net.TCPAddr),
	}

	host, portStr, err := net.SplitHostPort(conn.request.RequestURI)
	if err != nil {
		sendReply(conn, http.StatusBadRequest, "", err)
		return
	}

	ip := net.ParseIP(host)
	if ip != nil {
		remoteAddr.IP = ip
	} else {
		remoteAddr.DomainName = host
	}

	port, err := strconv.Atoi(portStr)
	if err != nil {
		sendReply(conn, http.StatusBadRequest, "", err)
		return
	}
	remoteAddr.Port = port

	remoteTCPConn, err := internal.ConnectToRemote(conn.ProxyConn, remoteAddr)
	if err != nil {
		if rcErr, ok := err.(*internal.RemoteConnError); ok {
			switch rcErr.Type {
			case internal.ERR_NOT_ALLOWED_BY_RULESET:
				sendReply(conn, http.StatusForbidden, "", err)
			case internal.ERR_NET_UNREACHABLE:
			case internal.ERR_HOST_UNREACHABLE:
			case internal.ERR_CONN_REFUSED:
			default:
				sendReply(conn, http.StatusBadGateway, "", err)
			}
		} else {
			// should not happen
			sendReply(conn, http.StatusInternalServerError, "", err)
		}
		return
	}

	defer remoteTCPConn.Close()

	sendReply(conn, http.StatusOK, "Connection established! Go ahead!", err)

	// Start proxying
	var bytesWritten, bytesRead int64
	errCh := make(chan error, 2)
	go internal.ProxyTCP(conn.Connection.Conn.(*net.TCPConn), remoteTCPConn, &bytesRead, errCh)
	go internal.ProxyTCP(remoteTCPConn, conn.Connection.Conn.(*net.TCPConn), &bytesWritten, errCh)

	// Wait
	for i := 0; i < 2; i++ {
		e := <-errCh
		if e != nil {
			break
		}
	}

	conn.AddRead(bytesRead)
	conn.AddWritten(bytesWritten)

	return
}

