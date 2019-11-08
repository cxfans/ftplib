/*
	Implements file transfer protocol server based on standard library
*/

package ftplib

import (
	"bufio"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net"
	"os"
	"path"
	"strconv"
	"strings"
)

type Server struct {
	listener *net.TCPListener
	host     string
	rootDir  string
}

func NewServer(addr, rootDir string) (server *Server, err error) {
	laddr, err := net.ResolveTCPAddr("tcp4", addr)
	if err != nil {
		log.Println(err)
		return nil, err
	}
	listener, err := net.ListenTCP("tcp4", laddr)
	if err != nil {
		log.Println(err)
		return nil, err
	}
	host, _, _ := net.SplitHostPort(addr)
	return &Server{listener: listener, host: host, rootDir: rootDir}, nil
}

func (server *Server) ListenAndServe() (err error) {
	log.Println("Server start.")
	for {
		conn, err := server.listener.AcceptTCP()
		if err != nil {
			log.Println(err)
			return err
		}

		serverConn := &ServerConn{
			conn:   conn,
			reader: bufio.NewReader(conn),
			writer: bufio.NewWriter(conn),
			prefix: server.rootDir,
			host:   server.host,
		}

		log.Println(conn.RemoteAddr().String(), "connected.")

		go serverConn.Serve()
	}
}

func (server *Server) Stop() (err error) {
	if server.listener != nil {
		return server.listener.Close()
	}
	return nil
}

type ServerConn struct {
	conn             *net.TCPConn
	reader           *bufio.Reader
	writer           *bufio.Writer
	dataConn         DataConn
	prefix, host, rn string
}

func (serverConn *ServerConn) Close() {
	serverConn.conn.Close()
	if serverConn.dataConn != nil {
		serverConn.dataConn.Close()
		serverConn.dataConn = nil
	}
	log.Println("Closed one connection.")
}

func (serverConn *ServerConn) cmd(msg string, v ...interface{}) (n int) {
	n, err := serverConn.writer.WriteString(msg + "\r\n")
	if err != nil {
		log.Println(err)
		serverConn.Close()
	}
	serverConn.writer.Flush()
	log.Println(msg)
	return
}

func (serverConn *ServerConn) sendCodeLine(code int, msg string) {
	serverConn.cmd(fmt.Sprintf("%d %s", code, msg))
}

func (serverConn *ServerConn) sendStatusText(code int) {
	serverConn.sendCodeLine(code, Message(code))
}

func (serverConn *ServerConn) sendData(data []byte) {
	if serverConn.dataConn != nil {
		n, _ := serverConn.dataConn.Write(data)
		serverConn.dataConn.Close()
		msg := fmt.Sprintf("Closing data connection, sent %d bytes.", n)
		serverConn.sendCodeLine(StatusClosingDataConnection, msg)
	} else {
		serverConn.sendStatusText(StatusTransfertAborted)
	}
}

func (serverConn *ServerConn) parsingPath(params []string) string {
	p := strings.Join(params, " ")
	if strings.HasPrefix(p, "/") {
		p = path.Join(".", p)
	} else {
		p = path.Join(serverConn.prefix, p)
	}
	return p
}

func (serverConn *ServerConn) Serve() {
	log.Println("Connection established: start server.")
	serverConn.sendStatusText(StatusReady)

loop:
	for {
		cmdLine, err := serverConn.reader.ReadString('\n')
		log.Print(cmdLine)
		if err != nil {
			// When the client closes the connection, the server will read EOF.
			if err == io.EOF {
				break loop
			}
			log.Println("Loop Error:", err)
			serverConn.Close()
			break loop
		}
		params := strings.Split(strings.TrimSpace(cmdLine), " ")
		switch strings.ToUpper(params[0]) {

		case USER:
			serverConn.sendStatusText(StatusUserOK)

		case PASS:
			serverConn.sendStatusText(StatusLoggedIn)

		case PWD:
			serverConn.sendCodeLine(StatusPathCreated,
				fmt.Sprintf("\"%s\" is current directory.", serverConn.prefix))

		case CWD:
			p := serverConn.parsingPath(params[1:])
			f, err := os.Stat(p)
			if f.IsDir() && err == nil {
				serverConn.prefix = p
				serverConn.sendCodeLine(StatusRequestedFileActionOK,
					"Directory changed to "+serverConn.prefix)
			} else {
				serverConn.sendStatusText(StatusFileUnavailable)
			}

		case DELE:
			p := serverConn.parsingPath(params[1:])
			_, err := os.Stat(p)
			if err != nil {
				serverConn.sendStatusText(StatusFileUnavailable)
			} else {
				os.Remove(p)
				serverConn.sendCodeLine(StatusRequestedFileActionOK, "File deleted.")
			}

		case EPSV:
			passiveConn, err := NewPassiveConn(serverConn.host)
			if err != nil {
				serverConn.sendStatusText(StatusCanNotOpenDataConnection)
			} else {
				serverConn.dataConn = passiveConn
				serverConn.sendCodeLine(StatusExtendedPassiveMode,
					fmt.Sprintf("Entering Extended Passive Mode (|||%d|)", passiveConn.Port()))
			}

		case SIZE:
			p := serverConn.parsingPath(params[1:])
			f, err := os.Stat(p)
			if f.IsDir() && err == nil {
				serverConn.sendCodeLine(StatusFile, "1024")
			} else {
				serverConn.sendCodeLine(StatusFile, strconv.Itoa(int(f.Size())))
			}

		case LIST:
			serverConn.sendCodeLine(StatusAboutToSend,
				"Opening ASCII mode data connection for file list")
			d, _ := os.Open(serverConn.prefix)
			items, _ := d.Readdir(-1)
			info := ListDetailed(items)
			serverConn.sendData(info)

		case MKD:
			p := serverConn.parsingPath(params[1:])
			err = os.Mkdir(p, 0777)
			if err == nil {
				serverConn.sendStatusText(StatusPathCreated)
			} else {
				serverConn.sendCodeLine(StatusFileUnavailable, fmt.Sprint(err))
			}

		case NOOP:
			serverConn.sendStatusText(StatusCommandOK)

		case PASV:
			passiveConn, err := NewPassiveConn(serverConn.host)
			if err != nil {
				serverConn.sendStatusText(StatusCanNotOpenDataConnection)
			} else {
				serverConn.dataConn = passiveConn
				port := passiveConn.Port()
				x := port / 256
				y := port - x*256
				quad := strings.ReplaceAll(passiveConn.Host(), ".", ",")
				msg := fmt.Sprintf("Entering Passive Mode (%s,%d,%d)", quad, x, y)
				serverConn.sendCodeLine(227, msg)
			}

		case QUIT:
			serverConn.Close()
			break loop

		case RETR:
			p := serverConn.parsingPath(params[1:])
			data, err := ioutil.ReadFile(p)
			if err != nil {
				serverConn.sendCodeLine(StatusFileUnavailable, fmt.Sprint(err))
			} else {
				bytes := strconv.Itoa(len(data))
				serverConn.sendCodeLine(150, "Data transfer starting "+bytes+"bytes")
				serverConn.sendData([]byte(data))
			}

		case RMD, XRMD:
			p := serverConn.parsingPath(params[1:])
			f, err := os.Stat(p)
			if f.IsDir() && err == nil {
				err := os.RemoveAll(p)
				if err != nil {
					serverConn.sendCodeLine(StatusFileUnavailable, fmt.Sprint(err))
				} else {
					serverConn.sendCodeLine(StatusRequestedFileActionOK, "Directory deleted.")
				}
			} else {
				serverConn.sendStatusText(StatusFileUnavailable)
			}

		case RNFR:
			serverConn.rn = serverConn.parsingPath(params[1:])
			serverConn.sendStatusText(StatusRequestFilePending)

		case RNTO:
			p := serverConn.parsingPath(params[1:])
			err := os.Rename(serverConn.rn, p)
			if err != nil {
				serverConn.sendCodeLine(StatusFileUnavailable, fmt.Sprint(err))
			} else {
				serverConn.sendCodeLine(StatusRequestedFileActionOK, "File renamed.")
			}

		case SYST:
			serverConn.sendStatusText(StatusName)

		case STOR:
			p := serverConn.parsingPath(params[1:])
			serverConn.sendCodeLine(150, "Data transfer starting.")
			file, err := os.OpenFile(p, os.O_WRONLY|os.O_TRUNC|os.O_CREATE,
				0666)
			if err != nil {
				serverConn.sendStatusText(450)
			}
			n, _ := io.Copy(file, serverConn.dataConn)

			if n >= 0 {
				serverConn.sendCodeLine(226, "OK, received "+
					strconv.Itoa(int(n))+" bytes.")
			} else {
				serverConn.sendStatusText(550)
			}
			file.Close()

		case TYPE:
			param := strings.ToUpper(params[1])
			if param == "A" {
				serverConn.sendCodeLine(StatusCommandOK, "Type set to ASCII.")
			} else if param == "I" {
				serverConn.sendCodeLine(StatusCommandOK, "Type set to binary.")
			} else {
				serverConn.sendCodeLine(StatusBadArguments, "Invalid type.")
			}

		default:
			serverConn.sendStatusText(StatusCommandNotImplemented)

		}
	}
	log.Println("Connection terminated: stop server.")
}
