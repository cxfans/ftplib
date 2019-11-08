package ftplib

import (
	"log"
	"net"
	"strconv"
	"strings"
)

type DataConn interface {
	Host() string
	Port() int
	Write(p []byte) (n int, err error)
	Read(p []byte) (n int, err error)
	Close() error
}

type PassiveConn struct {
	conn       *net.TCPConn
	host, port string
	done       chan bool
	err        error
}

func NewPassiveConn(host string) (passiveConn *PassiveConn, err error) {
	passiveConn = &PassiveConn{host: host, done: make(chan bool, 1)}
	if err := passiveConn.ListenAndServe(); err != nil {
		return nil, err
	}
	log.Println("A new passive connection created.")
	return passiveConn, nil
}

func (passiveConn *PassiveConn) Host() string {
	return "127.0.0.1"
}

func (passiveConn *PassiveConn) Port() int {
	port, _ := strconv.Atoi(passiveConn.port)
	return port
}

func (passiveConn *PassiveConn) Close() error {
	log.Println("Passive data connection closed.")
	return passiveConn.conn.Close()
}

func (passiveConn *PassiveConn) ListenAndServe() error {
	laddr, err := net.ResolveTCPAddr("tcp4", passiveConn.host+":0")
	if err != nil {
		log.Println(err)
		return err
	}
	listener, err := net.ListenTCP("tcp4", laddr)
	if err != nil {
		log.Println(err)
		return err
	}
	addr := listener.Addr()
	parts := strings.Split(addr.String(), ":")
	passiveConn.host = parts[0]
	passiveConn.port = parts[1]

	go func() {
		conn, err := listener.AcceptTCP()
		passiveConn.done <- true
		if err != nil {
			log.Println(err)
			passiveConn.err = err
			return
		}
		passiveConn.err = nil
		passiveConn.conn = conn
	}()

	return nil
}

func (passiveConn *PassiveConn) wait() bool {
	if passiveConn.conn != nil {
		return true
	}
	<-passiveConn.done
	return passiveConn.conn != nil
}

func (passiveConn *PassiveConn) Read(data []byte) (n int, err error) {
	if !passiveConn.wait() {
		return 0, passiveConn.err
	}
	return passiveConn.conn.Read(data)
}

func (passiveConn *PassiveConn) Write(data []byte) (n int, err error) {
	if !passiveConn.wait() {
		return 0, passiveConn.err
	}
	return passiveConn.conn.Write(data)
}
