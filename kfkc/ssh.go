package kfkc

import (
	"golang.org/x/crypto/ssh"
	"io/ioutil"
	"net"
	//"os"
	//"io"
	"time"
	"sync"
	"fmt"
)

var DefaultTimeout = 5 * time.Second
var DefaultScpCmd = "/usr/bin/scp -qrt "

type SshContext struct {
	user		string
	key			string
	timeout		time.Duration
	config		*ssh.ClientConfig
}

type SshClient struct {
	cli *ssh.Client
}

type SshConn struct {
	addr	string
	client	*ssh.Client
	lock	sync.Mutex
	conn	*net.Conn
}

func InitSshContext(path string, user string, timeout time.Duration, log *Log) (*SshContext, error) {
	sc := &SshContext{}

	key, err := ioutil.ReadFile(path)
	if err != nil {
		log.Error("Read private key from %s failed", path)
		return nil, err
	}
	sc.key = string(key)
	sc.user = user

	signer, err := ssh.ParsePrivateKey([]byte(sc.key))
	if err != nil {
		log.Error("Parse private key failed")
		return nil, err
	}

	authMethod := ssh.PublicKeys(signer)

	sc.config = &ssh.ClientConfig{
		User: sc.user,
		Auth: []ssh.AuthMethod{authMethod},
	}

	sc.timeout = timeout

	return sc, nil
}

func (sc *SshContext) InitSshConn(addr string, log *Log) (*SshConn, error) {
	sconn := &SshConn{}
	sconn.addr = addr
	err := sconn.SshConnect(sc, addr, log)
	if err != nil {
		log.Error("Init ssh connection to %s failed", addr)
		return nil, err
	}

	return sconn, nil
}

func (sconn *SshConn) SshConnect(sc *SshContext, addr string, log *Log) error {
	conn, err := net.DialTimeout("tcp", addr, sc.timeout)
	if err != nil {
		log.Error("Create ssh connection to %s failed", addr)
		return err
	}

	err = conn.SetDeadline(time.Now().Add(sc.timeout))
	if err != nil {
		log.Error("Set deadline failed")
		return err
	}

	sshConn, chans, reqs, err := ssh.NewClientConn(conn, addr, sc.config)
	if err != nil {
		return err
	}
	client := ssh.NewClient(sshConn, chans, reqs)

	sconn.client = client
	sconn.conn = &conn

	return nil
}

func (sconn *SshConn) SshExec(cmd string) ([]byte, error) {
	session, err := sconn.client.NewSession()
	if err != nil {
		return nil, err
	}

	defer session.Close()

	return session.CombinedOutput(cmd)
}

func (sconn *SshConn) SshScp(data []byte, file string, path string, right string) error {
	session, err := sconn.client.NewSession()
	if err != nil {
		return err
	}

	defer session.Close()

	go func() {
		w, _ := session.StdinPipe()
		defer w.Close()

		fmt.Fprintln(w, "C" + right, len(data), file) /* C is scp protocol */
		fmt.Fprint(w, string(data))
		fmt.Fprint(w, "\x00") /* End of transfer */
	}()

	cmd := DefaultScpCmd + path
	err = session.Run(cmd)
	if err != nil {
		fmt.Println("Run scp failed", err)
		return err
	}

	return nil
}

func (sconn *SshConn) sshLock() {
	sconn.lock.Lock()
}

func (sconn *SshConn) sshUnlock() {
	sconn.lock.Unlock()
}

func (sconn *SshConn) SshClose() {
	(*sconn.conn).Close()
}
