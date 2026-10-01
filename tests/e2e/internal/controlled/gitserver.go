package controlled

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"github.com/erranliu/SteerLine/internal/testprocess"
	"io"
	"net"
	"net/http"
	"net/textproto"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// gitServer is a loopback CGI transport for the real Git http-backend.
func gitServer(root, addressFile string) error {
	if root == "" || addressFile == "" {
		return fmt.Errorf("missing isolated Git server paths")
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return err
	}
	defer listener.Close()
	if err := Write(addressFile, map[string]string{"address": listener.Addr().String()}); err != nil {
		return err
	}
	server := http.Server{ReadHeaderTimeout: 5 * time.Second, Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		defer cancel()
		cmd := exec.Command("git", "http-backend")
		cmd.Env = append(os.Environ(), "GIT_PROJECT_ROOT="+root, "GIT_HTTP_EXPORT_ALL=1", "PATH_INFO="+r.URL.Path, "QUERY_STRING="+r.URL.RawQuery, "REQUEST_METHOD="+r.Method, "CONTENT_TYPE="+r.Header.Get("Content-Type"), "REMOTE_ADDR=127.0.0.1")
		var body io.Reader = r.Body
		if r.Header.Get("Content-Encoding") == "gzip" {
			z, err := gzip.NewReader(r.Body)
			if err != nil {
				http.Error(w, err.Error(), 400)
				return
			}
			defer z.Close()
			body = z
		}
		cmd.Stdin = body
		var stdout, stderr bytes.Buffer
		cmd.Stdout, cmd.Stderr = &stdout, &stderr
		if err := testprocess.Run(ctx, cmd); err != nil {
			http.Error(w, err.Error()+": "+stderr.String(), 500)
			return
		}
		reader := bufio.NewReader(&stdout)
		headers, err := textproto.NewReader(reader).ReadMIMEHeader()
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		status := 200
		for k, values := range headers {
			if k == "Status" {
				fields := strings.Fields(values[0])
				if len(fields) > 0 {
					status, _ = strconv.Atoi(fields[0])
				}
				continue
			}
			for _, v := range values {
				w.Header().Add(k, v)
			}
		}
		w.WriteHeader(status)
		_, _ = io.Copy(w, reader)
	})}
	return server.Serve(listener)
}
