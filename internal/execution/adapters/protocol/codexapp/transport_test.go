package codexapp

import (
	"bufio"
	"context"
	"crypto/sha1"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"testing"
	"time"
)

func TestCodexAppCallCancellationInterruptsBlockedWrite(t *testing.T) {
	local, remote := net.Pipe()
	defer remote.Close()
	client := &Client{socket: &codexSocket{conn: local, read: bufio.NewReader(local)}, pending: make(map[string]chan Envelope), notifications: make(chan Envelope, 1), done: make(chan struct{}), closing: make(chan struct{})}
	go client.read()
	defer client.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	finished := make(chan error, 1)
	go func() { finished <- client.Call(ctx, "thread/read", map[string]string{"threadId": "test"}, nil) }()
	// Reading one byte proves the client is blocked partway through its write.
	if _, err := io.ReadFull(remote, make([]byte, 1)); err != nil {
		t.Fatal(err)
	}
	cancel()
	select {
	case err := <-finished:
		if err == nil {
			t.Fatal("canceled blocked RPC succeeded")
		}
	case <-time.After(time.Second):
		t.Fatal("RPC context did not interrupt socket write")
	}
}

func TestCodexSocketHandshakeHonorsCancellationWithoutDeadline(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	finished := make(chan error, 1)
	go func() {
		socket, err := dialCodexSocket(ctx, "ws://"+listener.Addr().String())
		if socket != nil {
			socket.Close()
		}
		finished <- err
	}()
	conn, err := listener.Accept()
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if _, err := http.ReadRequest(bufio.NewReader(conn)); err != nil {
		t.Fatal(err)
	}
	cancel()
	select {
	case err := <-finished:
		if err == nil {
			t.Fatal("canceled handshake succeeded")
		}
	case <-time.After(time.Second):
		t.Fatal("handshake ignored cancellation")
	}
}

func TestCodexSocketLoopbackHandshakeAndMaskedJSON(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	finished := make(chan error, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			finished <- err
			return
		}
		defer conn.Close()
		reader := bufio.NewReader(conn)
		request, err := http.ReadRequest(reader)
		if err != nil {
			finished <- err
			return
		}
		if request.Header.Get("Origin") != "" {
			finished <- fmt.Errorf("client sent Origin header")
			return
		}
		key := request.Header.Get("Sec-WebSocket-Key")
		accept := sha1.Sum([]byte(key + "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"))
		_, err = fmt.Fprintf(conn, "HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Accept: %s\r\n\r\n", base64.StdEncoding.EncodeToString(accept[:]))
		if err != nil {
			finished <- err
			return
		}
		header := make([]byte, 2)
		if _, err = io.ReadFull(reader, header); err != nil {
			finished <- err
			return
		}
		if header[0] != 0x81 || header[1]&0x80 == 0 {
			finished <- fmt.Errorf("unmasked or nontext client frame")
			return
		}
		size := int(header[1] & 0x7f)
		if size == 126 {
			var n uint16
			if err = binary.Read(reader, binary.BigEndian, &n); err != nil {
				finished <- err
				return
			}
			size = int(n)
		}
		mask := make([]byte, 4)
		if _, err = io.ReadFull(reader, mask); err != nil {
			finished <- err
			return
		}
		payload := make([]byte, size)
		if _, err = io.ReadFull(reader, payload); err != nil {
			finished <- err
			return
		}
		for i := range payload {
			payload[i] ^= mask[i%4]
		}
		var value map[string]string
		if err = json.Unmarshal(payload, &value); err != nil {
			finished <- err
			return
		}
		if value["method"] != "initialize" {
			finished <- fmt.Errorf("unexpected request %v", value)
			return
		}
		_, err = conn.Write(append([]byte{0x81, 0x0e}, []byte(`{"id":1,"x":1}`)...))
		finished <- err
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	socket, err := dialCodexSocket(ctx, "ws://"+listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer socket.Close()
	if err := socket.Send(map[string]string{"method": "initialize"}); err != nil {
		t.Fatal(err)
	}
	message, err := socket.Receive()
	if err != nil {
		t.Fatal(err)
	}
	if string(message) != `{"id":1,"x":1}` {
		t.Fatalf("unexpected server message %s", message)
	}
	if err := <-finished; err != nil {
		t.Fatal(err)
	}
}
