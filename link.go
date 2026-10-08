package decker

import (
	"encoding/json"
	"fmt"
	"maps"
	"net"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"time"
)

type linkState struct {
	Slide   int           `json:"slide"` // 0-based
	Step    int           `json:"step"`  // 0-based
	Outline []linkOutline `json:"outline"`
	Notes   string        `json:"notes"` // for the current slide
	W       int           `json:"w"`     // the deck's size in cells, for previews
	H       int           `json:"h"`
	// Blank is "black" or "white" while the deck's screen is blanked, else
	// empty (and left out, so an unblanked state is the same JSON as before).
	Blank string `json:"blank,omitempty"`
}

// linkOutline is one slide in the deck's running order. It comes from the
// deck rather than the presenter's own build, because in dev mode the deck
// rebuilds on every save and the presenter doesn't.
type linkOutline struct {
	Title string `json:"title"`
	Steps int    `json:"steps"`

	// Section is the slide's resolved section, so the presenter view needn't
	// resolve it against a build that may be out of date.
	Section string `json:"section,omitempty"`
}

// linkCmd is a presenter request: press Key (a navigation key in keyActs) or
// jump to Slide.
type linkCmd struct {
	Key   string `json:"key,omitempty"`
	Slide int    `json:"slide,omitempty"` // 1-based
}

// defaultSocket is in the per-user temp dir, named for the deck, so both
// processes find it.
func defaultSocket(name string) string { return filepath.Join(os.TempDir(), name+".sock") }

// linkServer is the deck's end of the link: a Unix socket, one JSON object per
// line. The deck sends a linkState on each position change, the presenter sends
// linkCmds. Either window's keys work, and the deck runs without the presenter.
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
		return nil, fmt.Errorf("another deck is already running on %s: quit it, or pass a different --socket", path)
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
	for dec := json.NewDecoder(c); ; {
		var cmd linkCmd
		if dec.Decode(&cmd) != nil {
			break
		}
		if s.onCmd != nil {
			s.onCmd(cmd)
		}
	}
	s.drop(c)
}

// drop forgets c and closes it; safe to call twice.
func (s *linkServer) drop(c net.Conn) {
	s.mu.Lock()
	if out, ok := s.conns[c]; ok {
		delete(s.conns, c)
		close(out)
	}
	s.mu.Unlock()
	c.Close()
}

// publish sends st to every connected presenter view, and to any that
// connect later. A slow reader gets only the newest state.
func (s *linkServer) publish(st linkState) {
	b, _ := json.Marshal(st) // plain data: can't fail
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

// close stops serving and removes the socket file.
func (s *linkServer) close() {
	s.ln.Close()
	s.mu.Lock()
	conns := slices.Collect(maps.Keys(s.conns))
	s.mu.Unlock()
	for _, c := range conns {
		s.drop(c)
	}
	os.Remove(s.path)
}

type linkClient struct {
	mu   sync.Mutex
	conn net.Conn
	enc  *json.Encoder
	dec  *json.Decoder
}

func (l *linkClient) dial(path string) error {
	c, err := net.DialTimeout("unix", path, time.Second)
	if err != nil {
		return err
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.conn != nil {
		l.conn.Close()
	}
	l.conn, l.enc, l.dec = c, json.NewEncoder(c), json.NewDecoder(c)
	return nil
}

// next blocks until the deck sends its next state.
func (l *linkClient) next() (st linkState, err error) {
	l.mu.Lock()
	dec := l.dec
	l.mu.Unlock()
	if dec == nil {
		return st, net.ErrClosed
	}
	err = dec.Decode(&st)
	return st, err
}

// send asks the deck to act; it fails quietly when not connected.
func (l *linkClient) send(cmd linkCmd) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.conn == nil {
		return
	}
	l.conn.SetWriteDeadline(time.Now().Add(time.Second))
	l.enc.Encode(cmd)
}

func (l *linkClient) close() {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.conn != nil {
		l.conn.Close()
		l.conn, l.enc, l.dec = nil, nil, nil
	}
}
