package decker

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// The deck and the presenter view (-presenter) run as two processes, in
// separate windows, and talk over a Unix socket: one JSON object per line.
// The deck owns the position. It sends a linkState whenever the position
// changes, and the presenter sends linkCmds to move it. Either window's keys
// work, so the deck keeps going if the presenter view goes away.

// linkState is what the deck tells the presenter view.
type linkState struct {
	Slide   int           `json:"slide"` // 0-based
	Step    int           `json:"step"`  // 0-based
	Outline []linkOutline `json:"outline"`
	Notes   string        `json:"notes"` // for the current slide
	W       int           `json:"w"`     // the deck's size in cells, for previews
	H       int           `json:"h"`
}

// linkOutline is one slide in the deck's running order. It comes from the
// deck rather than the presenter's own build, because in dev mode the deck
// rebuilds on every save and the presenter doesn't.
type linkOutline struct {
	Title string `json:"title"`
	Steps int    `json:"steps"`
}

// linkCmd is what the presenter view asks the deck to do.
type linkCmd struct {
	Cmd   string `json:"cmd"`             // next, prev, nextSlide, prevSlide, first, last, goto, replay
	Slide int    `json:"slide,omitempty"` // 1-based, for goto
}

// linkKeys maps each command to the deck key that does the same thing.
var linkKeys = map[string]string{
	"next": "right", "prev": "left",
	"nextSlide": "]", "prevSlide": "[",
	"first": "home", "last": "end",
	"replay": "r",
}

// defaultSocket is in the per-user temp directory, named for the deck, so
// both processes find it without configuration.
func defaultSocket(name string) string { return filepath.Join(os.TempDir(), name+".sock") }

// linkServer is the deck's end of the link.
type linkServer struct {
	ln    net.Listener
	path  string
	onCmd func(linkCmd)

	mu    sync.Mutex
	conns map[net.Conn]chan []byte // each holds at most the newest state
	last  []byte
}

// listenLink starts serving the link at path. A socket file left behind by
// a deck that exited (or restarted in dev mode) is replaced; one that a
// running deck still answers is an error.
func listenLink(path string, onCmd func(linkCmd)) (*linkServer, error) {
	if c, err := net.DialTimeout("unix", path, 200*time.Millisecond); err == nil {
		c.Close()
		return nil, fmt.Errorf("another deck is already running on %s: quit it, or pass a different -socket", path)
	}
	os.Remove(path)
	ln, err := net.Listen("unix", path)
	if err != nil {
		return nil, err
	}
	s := &linkServer{ln: ln, path: path, onCmd: onCmd, conns: map[net.Conn]chan []byte{}}
	go s.accept()
	return s, nil
}

func (s *linkServer) accept() {
	for {
		c, err := s.ln.Accept()
		if err != nil {
			return // closed
		}
		out := make(chan []byte, 1)
		s.mu.Lock()
		s.conns[c] = out
		if s.last != nil {
			out <- s.last
		}
		s.mu.Unlock()
		go s.write(c, out)
		go s.read(c)
	}
}

func (s *linkServer) write(c net.Conn, out chan []byte) {
	for b := range out {
		c.SetWriteDeadline(time.Now().Add(2 * time.Second))
		if _, err := c.Write(b); err != nil {
			s.drop(c)
			return
		}
	}
}

func (s *linkServer) read(c net.Conn) {
	sc := bufio.NewScanner(c)
	for sc.Scan() {
		var cmd linkCmd
		if json.Unmarshal(sc.Bytes(), &cmd) == nil && s.onCmd != nil {
			s.onCmd(cmd)
		}
	}
	s.drop(c)
}

func (s *linkServer) drop(c net.Conn) {
	s.mu.Lock()
	if out, ok := s.conns[c]; ok {
		delete(s.conns, c)
		close(out)
	}
	s.mu.Unlock()
	c.Close()
}

// Publish sends st to every connected presenter view, and to any that
// connect later. A slow reader gets only the newest state.
func (s *linkServer) Publish(st linkState) {
	b, err := json.Marshal(st)
	if err != nil {
		return
	}
	b = append(b, '\n')
	s.mu.Lock()
	defer s.mu.Unlock()
	s.last = b
	for _, out := range s.conns {
		select {
		case <-out: // replace an unsent older state
		default:
		}
		out <- b
	}
}

// Close stops serving and removes the socket file.
func (s *linkServer) Close() {
	s.ln.Close()
	s.mu.Lock()
	for c, out := range s.conns {
		delete(s.conns, c)
		close(out)
		c.Close()
	}
	s.mu.Unlock()
	os.Remove(s.path)
}

// linkClient is the presenter view's end of the link.
type linkClient struct {
	mu   sync.Mutex
	conn net.Conn
	r    *bufio.Reader
}

// dial connects to the deck, replacing any earlier connection.
func (l *linkClient) dial(path string) error {
	c, err := net.DialTimeout("unix", path, time.Second)
	if err != nil {
		return err
	}
	l.mu.Lock()
	if l.conn != nil {
		l.conn.Close()
	}
	l.conn, l.r = c, bufio.NewReader(c)
	l.mu.Unlock()
	return nil
}

// next blocks until the deck sends its next state.
func (l *linkClient) next() (linkState, error) {
	l.mu.Lock()
	r := l.r
	l.mu.Unlock()
	var st linkState
	if r == nil {
		return st, net.ErrClosed
	}
	line, err := r.ReadBytes('\n')
	if err != nil {
		return st, err
	}
	return st, json.Unmarshal(line, &st)
}

// send asks the deck to do something. It fails quietly when not connected;
// the presenter view shows the link status.
func (l *linkClient) send(cmd linkCmd) {
	b, err := json.Marshal(cmd)
	if err != nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.conn == nil {
		return
	}
	l.conn.SetWriteDeadline(time.Now().Add(time.Second))
	l.conn.Write(append(b, '\n'))
}

func (l *linkClient) close() {
	l.mu.Lock()
	if l.conn != nil {
		l.conn.Close()
		l.conn, l.r = nil, nil
	}
	l.mu.Unlock()
}
