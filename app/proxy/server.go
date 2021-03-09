package proxy

import (
	"fmt"
	"io"
	"math/rand"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

const (
	MethodGet     = "GET"
	MethodHead    = "HEAD"
	MethodPost    = "POST"
	MethodPut     = "PUT"
	MethodPatch   = "PATCH" // RFC 5789
	MethodDelete  = "DELETE"
	MethodConnect = "CONNECT"
	MethodOptions = "OPTIONS"
	MethodTrace   = "TRACE"
)

func generateCerf(host string) string {
	host = host[:len(host)-4]
	root, _ := os.Getwd()
	path := fmt.Sprintf("%s/certs/%s.crt", root, host)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		if n, err := exec.Command("/bin/sh", "./gen_cert.sh", host, strconv.Itoa(rand.Int())).Output(); err != nil {
			fmt.Println(n)
			return ""
		} else {
			dst, err := os.Create(path)
			if err != nil {
				return ""
			}
			defer dst.Close()

			if _, err := io.Copy(dst, strings.NewReader(string(n))); err != nil {
				return ""
			}
		}
	}
	return path
}
