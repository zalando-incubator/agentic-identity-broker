package helpers

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"net/url"
	"sync"
	"sync/atomic"
)

// RefreshCommitFault forwards real PostgreSQL connections. Once armed, it sends
// the next COMMIT to PostgreSQL, consumes its response through ReadyForQuery,
// and closes that connection before forwarding the acknowledgement to pgx.
// The caller must inspect the database through a separate, direct connection.
// No PostgreSQL frame, SQL parameter, or credential is logged.
type RefreshCommitFault struct {
	listener    net.Listener
	direct      *url.URL
	connections sync.Map // net.Conn (client) -> *refreshFaultConnection
	closed      atomic.Bool
	unavailable atomic.Bool
	armed       atomic.Bool
	mu          sync.Mutex
	result      chan bool
}

type refreshFaultConnection struct {
	client, server  net.Conn
	owner           *RefreshCommitFault
	dropping        atomic.Bool
	refreshMutation atomic.Bool
	closeOnce       sync.Once
	reportOnce      sync.Once
	result          chan bool
	writeStatements map[string]bool
	writePortals    map[string]bool
}

func NewRefreshCommitFault(directURL string) (*RefreshCommitFault, error) {
	direct, err := url.Parse(directURL)
	if err != nil {
		return nil, fmt.Errorf("parse direct PostgreSQL URL: %w", err)
	}
	if direct.Host == "" || direct.Query().Get("sslmode") != "disable" {
		return nil, fmt.Errorf("test protocol relay requires PostgreSQL URL with sslmode=disable")
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("listen for PostgreSQL commit fault: %w", err)
	}
	fault := &RefreshCommitFault{listener: listener, direct: direct}
	go fault.accept()
	return fault, nil
}

func (f *RefreshCommitFault) ConnectionURL() string {
	local := *f.direct
	local.Host = f.listener.Addr().String()
	return local.String()
}

// ArmCommitLoss observes exactly one COMMIT after a refresh-row mutation,
// not an unrelated signing-key or branch-key commit. The returned channel
// receives true only after PostgreSQL finished that COMMIT without an
// ErrorResponse; the application itself receives no acknowledgement.
func (f *RefreshCommitFault) ArmCommitLoss() (<-chan bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.closed.Load() || f.armed.Load() {
		return nil, fmt.Errorf("PostgreSQL commit fault is closed or already armed")
	}
	f.result = make(chan bool, 1)
	f.armed.Store(true)
	return f.result, nil
}

// SetUnavailable breaks subsequent PostgreSQL query attempts, without
// synthesizing an OAuth response; recovery uses the same production app.
func (f *RefreshCommitFault) SetUnavailable(unavailable bool) {
	f.unavailable.Store(unavailable)
}

func (f *RefreshCommitFault) Close() error {
	if !f.closed.CompareAndSwap(false, true) {
		return nil
	}
	err := f.listener.Close()
	f.connections.Range(func(_, value any) bool {
		value.(*refreshFaultConnection).close()
		return true
	})
	return err
}

func (f *RefreshCommitFault) accept() {
	for {
		client, err := f.listener.Accept()
		if err != nil {
			return
		}
		server, err := net.Dial("tcp", f.direct.Host)
		if err != nil {
			_ = client.Close()
			continue
		}
		conn := &refreshFaultConnection{client: client, server: server, owner: f}
		f.connections.Store(client, conn)
		go conn.serve()
	}
}

func (c *refreshFaultConnection) serve() {
	defer c.close()
	if err := c.forwardStartup(); err != nil {
		return
	}
	go c.toServer()
	c.toClient()
}

func (c *refreshFaultConnection) close() {
	c.closeOnce.Do(func() {
		_ = c.client.Close()
		_ = c.server.Close()
		c.owner.connections.Delete(c.client)
		if c.dropping.Load() {
			c.report(false)
		}
	})
}

func (c *refreshFaultConnection) forwardStartup() error {
	for {
		var header [4]byte
		if _, err := io.ReadFull(c.client, header[:]); err != nil {
			return err
		}
		length := binary.BigEndian.Uint32(header[:])
		if length < 8 || length > 1<<20 {
			return fmt.Errorf("invalid PostgreSQL startup frame length")
		}
		body := make([]byte, int(length)-4)
		if _, err := io.ReadFull(c.client, body); err != nil {
			return err
		}
		if length == 8 && (binary.BigEndian.Uint32(body[:4]) == 80877103 || binary.BigEndian.Uint32(body[:4]) == 80877104) {
			if _, err := c.client.Write([]byte{'N'}); err != nil {
				return err
			}
			continue
		}
		if _, err := c.server.Write(header[:]); err != nil {
			return err
		}
		_, err := c.server.Write(body)
		return err
	}
}

func (c *refreshFaultConnection) toServer() {
	defer c.close()
	for {
		var header [5]byte
		if _, err := io.ReadFull(c.client, header[:]); err != nil {
			return
		}
		length := binary.BigEndian.Uint32(header[1:])
		if length < 4 || length > 1<<26 {
			return
		}
		size := int64(length - 4)
		if c.owner.unavailable.Load() {
			return
		}
		if header[0] == 'Q' || header[0] == 'P' || header[0] == 'B' || header[0] == 'E' {
			body := make([]byte, int(size))
			if _, err := io.ReadFull(c.client, body); err != nil {
				return
			}
			switch header[0] {
			case 'P':
				name, query, _ := bytes.Cut(body, []byte{0})
				query, _, _ = bytes.Cut(query, []byte{0})
				if c.writeStatements == nil {
					c.writeStatements = make(map[string]bool)
				}
				c.writeStatements[string(name)] = isRefreshWrite(query)
			case 'B':
				portal, statement, _ := bytes.Cut(body, []byte{0})
				statement, _, _ = bytes.Cut(statement, []byte{0})
				if c.writePortals == nil {
					c.writePortals = make(map[string]bool)
				}
				c.writePortals[string(portal)] = c.writeStatements[string(statement)]
			case 'E':
				portal, _, _ := bytes.Cut(body, []byte{0})
				if c.writePortals[string(portal)] {
					c.refreshMutation.Store(true)
				}
			case 'Q':
				query, _, _ := bytes.Cut(body, []byte{0})
				if isRefreshWrite(query) {
					c.refreshMutation.Store(true)
				}
				if bytes.EqualFold(bytes.TrimSpace(query), []byte("COMMIT")) {
					if c.refreshMutation.Swap(false) && c.owner.armed.CompareAndSwap(true, false) {
						c.owner.mu.Lock()
						c.result = c.owner.result
						c.owner.result = nil
						c.owner.mu.Unlock()
						c.dropping.Store(true)
					}
				} else if bytes.EqualFold(bytes.TrimSpace(query), []byte("ROLLBACK")) {
					c.refreshMutation.Store(false)
				}
			}
			if _, err := c.server.Write(header[:]); err != nil {
				return
			}
			if _, err := c.server.Write(body); err != nil {
				return
			}
			continue
		}
		if _, err := c.server.Write(header[:]); err != nil {
			return
		}
		if _, err := io.CopyN(c.server, c.client, size); err != nil {
			return
		}
	}
}

func isRefreshWrite(sql []byte) bool {
	sql = bytes.TrimSpace(sql)
	for _, verb := range [...]string{"INSERT INTO", "UPDATE", "DELETE FROM"} {
		if len(sql) <= len(verb) || !bytes.EqualFold(sql[:len(verb)], []byte(verb)) {
			continue
		}
		table := bytes.TrimSpace(sql[len(verb):])
		return len(table) >= len("refresh_") && bytes.EqualFold(table[:len("refresh_")], []byte("refresh_"))
	}
	return false
}

func (c *refreshFaultConnection) toClient() {
	var serverError bool
	for {
		var header [5]byte
		if _, err := io.ReadFull(c.server, header[:]); err != nil {
			return
		}
		length := binary.BigEndian.Uint32(header[1:])
		if length < 4 || length > 1<<26 {
			return
		}
		if c.dropping.Load() {
			if header[0] == 'E' {
				serverError = true
			}
			if _, err := io.CopyN(io.Discard, c.server, int64(length-4)); err != nil {
				return
			}
			if header[0] == 'Z' {
				c.report(!serverError)
				return
			}
			continue
		}
		if _, err := c.client.Write(header[:]); err != nil {
			return
		}
		if _, err := io.CopyN(c.client, c.server, int64(length-4)); err != nil {
			return
		}
	}
}

func (c *refreshFaultConnection) report(committed bool) {
	c.reportOnce.Do(func() {
		if c.result != nil {
			c.result <- committed
			close(c.result)
		}
	})
}
