package ftplib

import (
	"bufio"
	"errors"
	"io"
	"log"
	"net"
	"net/textproto"
	"strconv"
	"strings"
	"time"
)

/// EntryType describes the different types of an Entry.
type EntryType int

const (
	EntryTypeFile EntryType = iota
	EntryTypeFolder
	EntryTypeLink
)

// ClientConn represents the connection to a remote FTP server.
type ClientConn struct {
	conn     *textproto.Conn
	host     string
	timeout  time.Duration
	features map[string]string
}

// response represent a data-connection
type response struct {
	conn net.Conn
	c    *ClientConn
}

// ClientConn represents the connection to a remote FTP server.
type Entry struct {
	Name string
	Type EntryType
	Size uint64
	Time time.Time
}

func (c *ClientConn) Quit() error {
	c.conn.Cmd("QUIT")
	return c.conn.Close()
}

func (r *response) Close() error {
	err := r.conn.Close()
	_, _, err2 := r.c.conn.ReadResponse(StatusClosingDataConnection)
	if err2 != nil {
		err = err2
	}
	return err
}

func (r *response) Read(buf []byte) (int, error) {
	return r.conn.Read(buf)
}

// Dial is like DialTimeout with no timeout
func Dial(addr string) (*ClientConn, error) {
	return DialTimeout(addr, 0)
}

func DialTimeout(addr string, timeout time.Duration) (*ClientConn, error) {
	tconn, err := net.DialTimeout("tcp", addr, timeout)
	if err != nil {
		return nil, err
	}
	// Use the resolved IP address in case addr contains a domain name
	// If we use the domain name, we might not resolve to the same IP.
	remoteAddr := tconn.RemoteAddr().(*net.TCPAddr)
	conn := textproto.NewConn(tconn)

	c := &ClientConn{
		conn:     conn,
		host:     remoteAddr.IP.String(),
		timeout:  timeout,
		features: make(map[string]string),
	}

	_, msg, err := c.conn.ReadResponse(StatusReady)
	//_, msg, err := c.conn.ReadResponse(StatusReady)
	log.Println(msg)
	if err != nil {
		c.Quit()
		return nil, err
	}

	err = c.feat()
	if err != nil {
		c.Quit()
		return nil, err
	}

	err = c.setUTF8()
	if err != nil {
		c.Quit()
		return nil, err
	}

	return c, nil
}

// setUTF8 issues an "OPTS UTF8 ON" command.
func (c *ClientConn) setUTF8() error {
	if _, ok := c.features["UTF8"]; !ok {
		return nil
	}

	code, message, err := c.cmd(-1, "OPTS UTF8 ON")
	if err != nil {
		return err
	}

	if code != StatusCommandOK {
		return errors.New(message)
	}

	log.Println("Set utf-8")

	return nil
}

// feat issues a FEAT FTP command to list the additional commands supported by
// the remote FTP server.
// FEAT is described in RFC 2389
func (c *ClientConn) feat() error {
	code, message, err := c.cmd(-1, "FEAT")
	if err != nil {
		return err
	}

	if code != StatusSystem {
		// The server does not support the FEAT command. This is not an
		// error: we consider that there is no additional feature.
		return nil
	}

	lines := strings.Split(message, "\n")
	for _, line := range lines {
		if !strings.HasPrefix(line, " ") {
			continue
		}

		line = strings.TrimSpace(line)
		featureElements := strings.SplitN(line, " ", 2)

		command := featureElements[0]

		var commandDesc string
		if len(featureElements) == 2 {
			commandDesc = featureElements[1]
		}

		c.features[command] = commandDesc
	}

	return nil
}

// Login authenticates the client with specified user and password.
//
// "anonymous"/"anonymous" is a common user/password scheme for FTP servers
// that allows anonymous read-only accounts.
func (c *ClientConn) Login(user, password string) error {
	code, message, err := c.cmd(-1, "USER %s", user)
	if err != nil {
		return err
	}

	switch code {
	case StatusLoggedIn:
	case StatusUserOK:
		_, _, err = c.cmd(StatusLoggedIn, "PASS %s", password)
		if err != nil {
			return err
		}
	default:
		return errors.New(message)
	}

	// Switch to binary mode
	_, _, err = c.cmd(StatusCommandOK, "TYPE I")
	if err != nil {
		return err
	}

	log.Println("User logged in.")
	return nil
}

// Logout issues a REIN FTP command to logout the current user.
func (c *ClientConn) Logout() error {
	_, _, err := c.cmd(StatusReady, "REIN")
	return err
}

func ConnectAnonymous(addr string) (*ClientConn, error) {
	return Connect(addr, "anonymous", "anonymous")
}

func Connect(addr, user, password string) (*ClientConn, error) {
	c, err := Dial(addr)
	if err != nil {
		c.Quit()
		return nil, err
	}
	return c, c.Login(user, password)
}

// epsv issues an "EPSV" command to get a port number for a data connection.
// Enter extended passive mode
func (c *ClientConn) epsv() (port int, err error) {
	_, line, err := c.cmd(StatusExtendedPassiveMode, "EPSV")
	if err != nil {
		return
	}
	// 229 Entering Extended Passive Mode (|||65202|)
	start := strings.Index(line, "|||")
	end := strings.LastIndex(line, "|")
	if start == -1 || end == -1 {
		err = errors.New("invalid EPSV response format")
		return
	}
	port, err = strconv.Atoi(line[start+3 : end])
	return
}

// Enter passive mode
func (c *ClientConn) pasv() (port int, err error) {
	_, line, err := c.cmd(StatusPassiveMode, "PASV")
	if err != nil {
		return
	}
	// 227 Entering Passive Mode (172,17,66,241,254,179)
	start := strings.Index(line, "(")
	end := strings.LastIndex(line, ")")
	if start == -1 || end == -1 {
		err = errors.New("invalid PASV response format")
		return
	}
	lst := strings.Split(string(line[start+1:end]), ",")
	n := len(lst)
	x, _ := strconv.Atoi(lst[n-2])
	y, _ := strconv.Atoi(lst[n-1])
	port, err = x*256+y, nil
	return
}

// openDataConn creates a new FTP data connection.
func (c *ClientConn) openDataConn() (net.Conn, error) {
	var (
		port int
		err  error
	)

	if port, err = c.epsv(); err != nil {
		if port, err = c.pasv(); err != nil {
		}
	}

	return net.DialTimeout("tcp", net.JoinHostPort(c.host, strconv.Itoa(port)), c.timeout)
}

// parseListLine parses the various non-standard
// format returned by the LIST FTP command.
func parseListLine(line string) (*Entry, error) {
	fields := strings.Fields(line)
	if len(fields) < 9 {
		return nil, errors.New("unsupported LIST line")
	}

	e := &Entry{}
	switch fields[0][0] {
	case '-':
		e.Type = EntryTypeFile
	case 'd':
		e.Type = EntryTypeFolder
	case 'l':
		e.Type = EntryTypeLink
	default:
		return nil, errors.New("unknown entry type")
	}

	if e.Type == EntryTypeFile {
		size, err := strconv.ParseUint(fields[4], 10, 0)
		if err != nil {
			return nil, err
		}
		e.Size = size
	}
	var timeStr string
	if strings.Contains(fields[7], ":") { // this year
		thisYear, _, _ := time.Now().Date()
		timeStr = fields[6] + " " + fields[5] + " " +
			strconv.Itoa(thisYear)[2:4] + " " + fields[7] + " GMT"
	} else { // not this year
		timeStr = fields[6] + " " + fields[5] + " " + fields[7][2:4] + " " + "00:00" + " GMT"
	}
	t, err := time.Parse("_2 Jan 06 15:04 MST", timeStr)
	if err != nil {
		return nil, err
	}
	e.Time = t

	e.Name = strings.Join(fields[8:], " ")
	return e, nil
}

// NameList issues an NLST FTP command.
func (c *ClientConn) NameList(path string) (entries []string, err error) {
	conn, err := c.cmdDataConnFrom(0, "NLST %s", path)
	if err != nil {
		return
	}

	r := &response{conn, c}
	defer r.Close()

	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		entries = append(entries, scanner.Text())
	}
	if err = scanner.Err(); err != nil {
		return entries, err
	}
	return
}

// List issues a LIST FTP command.
func (c *ClientConn) List(path string) (entries []*Entry, err error) {
	conn, err := c.cmdDataConnFrom(0, "LIST %s", path)
	if err != nil {
		return
	}
	r := &response{conn, c}
	defer r.Close()

	bio := bufio.NewReader(r)
	for {
		line, e := bio.ReadString('\n')
		if e == io.EOF {
			break
		} else if e != nil {
			return nil, e
		}
		entry, err := parseListLine(line)
		if err == nil {
			entries = append(entries, entry)
		}
	}
	return
}

func (c *ClientConn) ChangeDir(path string) error {
	_, _, err := c.cmd(StatusRequestedFileActionOK, "CWD %s", path)
	return err
}

// Changes the current directory to the parent directory.
// ChangeDir("..")
func (c *ClientConn) ChangeDirToParent() error {
	_, _, err := c.cmd(StatusRequestedFileActionOK, "CDUP")
	return err
}

// Returns the path of the current directory.
func (c *ClientConn) CurrentDir() (string, error) {
	_, msg, err := c.cmd(StatusPathCreated, "PWD")
	if err != nil {
		return "", err
	}

	start := strings.Index(msg, "\"")
	end := strings.LastIndex(msg, "\"")

	if start == -1 || end == -1 {
		return "", errors.New("unsupported PWD response format")
	}

	return msg[start+1 : end], nil
}

// Retrieves a file from the remote FTP server.
// The ReadCloser must be closed at the end of the operation.
func (c *ClientConn) Retr(path string) (io.ReadCloser, error) {
	return c.RetrFrom(path, 0)
}

// Retr issues a RETR FTP command to fetch the specified file from the remote
// FTP server, the server will not send the offset first bytes of the file.
func (c *ClientConn) RetrFrom(path string, offset uint64) (io.ReadCloser, error) {
	conn, err := c.cmdDataConnFrom(offset, "RETR %s", path)
	if err != nil {
		return nil, err
	}

	return &response{conn, c}, nil
}

// Uploads a file to the remote FTP server.
// This function gets the data from the io.Reader. Hint: io.Pipe()
func (c *ClientConn) Stor(path string, r io.Reader) error {
	return c.StorFrom(path, r, 0)
}

// Stor issues a STOR FTP command to store a file to the remote FTP server.
// Stor creates the specified file with the content of the io.Reader, writing
// on the server will start at the given file offset.
func (c *ClientConn) StorFrom(path string, r io.Reader, offset uint64) error {
	conn, err := c.cmdDataConnFrom(offset, "STOR %s", path)

	if err != nil {
		return err
	}

	_, err = io.Copy(conn, r)
	conn.Close()
	if err != nil {
		return err
	}

	_, _, err = c.conn.ReadResponse(StatusClosingDataConnection)
	return err
}

func (c *ClientConn) Rename(from, to string) error {
	_, _, err := c.cmd(StatusRequestFilePending, "RNFR %s", from)
	if err != nil {
		return err
	}

	_, _, err = c.cmd(StatusRequestedFileActionOK, "RNTO %s", to)
	return err
}

// Creates a new directory on the remote FTP server.
func (c *ClientConn) MakeDir(path string) error {
	_, _, err := c.cmd(StatusPathCreated, "MKD %s", path)
	return err
}

// Removes a directory from the remote FTP server.
func (c *ClientConn) RemoveDir(path string) error {
	_, _, err := c.cmd(StatusRequestedFileActionOK, "RMD %s", path)
	return err
}

// Deletes a file on the remote FTP server.
func (c *ClientConn) Delete(path string) error {
	_, _, err := c.cmd(StatusRequestedFileActionOK, "DELE %s", path)
	return err
}

// Sends a NOOP command. Usualy used to prevent timeouts.
func (c *ClientConn) NoOp() error {
	_, _, err := c.cmd(StatusCommandOK, "NOOP")
	return err
}

// cmd is a helper function to execute a command and
// check for the expected FTP return code
func (c *ClientConn) cmd(expected int, format string, args ...interface{}) (int, string, error) {
	_, err := c.conn.Cmd(format, args...)
	if err != nil {
		return 0, "", err
	}

	return c.conn.ReadResponse(expected)
}

// cmdDataConnFrom executes a command which require a FTP data connection.
// Issues a REST FTP command to specify the number of bytes to skip for the transfer.
func (c *ClientConn) cmdDataConnFrom(offset uint64, format string, args ...interface{}) (net.Conn, error) {
	conn, err := c.openDataConn()
	if err != nil {
		return nil, err
	}

	if offset != 0 {
		_, _, err := c.cmd(StatusRequestFilePending, "REST %d", offset)
		if err != nil {
			conn.Close()
			return nil, err
		}
	}

	_, err = c.conn.Cmd(format, args...)
	if err != nil {
		conn.Close()
		return nil, err
	}
	code, msg, err := c.conn.ReadResponse(-1)
	if err != nil {
		conn.Close()
		return nil, err
	}
	if code != StatusAlreadyOpen && code != StatusAboutToSend {
		conn.Close()
		// It easier for the client to extract the code and message with type assertions.
		return nil, &textproto.Error{code, msg}
	}
	return conn, nil
}
