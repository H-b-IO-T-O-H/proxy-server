package app

import (
	"bufio"
	"bytes"
	"crypto/tls"
	"fmt"
	"github.com/H-b-IO-T-O-H/proxy-server/app/certificates"
	"github.com/apsdehal/go-logger"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

const (
	OKHeader = "HTTP/1.1 200 OK\r\n\r\n"
)

type Config struct {
	Listen string `yaml:"listen"`
	//Db      DBConfig `yaml:"db"`
}

type Server struct {
	ca         tls.Certificate
	httpClient *http.Client
	log        *logger.Logger
	//db         *database.DB
	doneChan chan bool
	config   *Config
}

func NewServer(config Config) (*Server, error) {
	var err error
	infoLogger, _ := logger.New("Info logger", 1, os.Stdout)
	server := Server{
		httpClient: new(http.Client),
		config:     &config,
		log:        infoLogger,
		doneChan:   make(chan bool, 1),
		//db:       db,
	}
	server.httpClient.Timeout = 5 * time.Second
	server.ca, err = certificates.LoadCA()
	if err != nil {
		return nil, err
	}
	return &server, nil
}

func (s *Server) Run() {

	go func() {
		s.log.Infof("Start listening on %s", s.config.Listen)
		if err := http.ListenAndServe(s.config.Listen, s); err != http.ErrServerClosed {
			s.log.Fatalf("listen: %s\n", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	select {
	case <-quit:
	case <-s.doneChan:
	}
	s.log.Info("Shutdown Server (timeout of 1 seconds) ...\nServer exiting")
}

func (s *Server) Close() {
	s.doneChan <- true
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	defer func() {
		err := recover()
		if err != nil {
			s.log.Fatal("unexpected error was met during server recover")
			log.Println(err)
		}
	}()
	//requestLogger(r, s.log)
	if r.Method == http.MethodConnect {
		s.HandleHttps(w, r)
	} else {
		s.HandleHttp(w, r)
	}
}

func (s *Server) HandleHttps(w http.ResponseWriter, r *http.Request) {

	leafCert, err := certificates.GenerateCert(&s.ca, r.Host)
	if err != nil {
		s.log.FatalF("Error while generating certificates: %s\n", err.Error())
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}

	curCert := make([]tls.Certificate, 1)
	curCert[0] = *leafCert

	curConfig := &tls.Config{
		Certificates: curCert,
		GetCertificate: func(info *tls.ClientHelloInfo) (certificate *tls.Certificate, e error) {
			return certificates.GenerateCert(&s.ca, info.ServerName)
		},
	}

	serverConn, errServ := tls.Dial("tcp", r.Host, curConfig)
	if errServ != nil {
		s.log.ErrorF("Service unavailable: %v", errServ)
		http.Error(w, errServ.Error(), http.StatusServiceUnavailable)
		return
	}
	hijacker, ok := w.(http.Hijacker)
	if !ok {
		msg := "Hijacking not supported"
		s.log.ErrorF(msg)
		http.Error(w, msg, http.StatusInternalServerError)
		return
	}

	//head := fmt.Sprintf("Method: %s\t Request: %s", r.Proto,  r.URL)
	//s.log.Infof(head)
	//requestLogger(r.Response, s.log)

	conn, _, err := hijacker.Hijack()
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}

	if _, err = conn.Write([]byte(OKHeader)); err != nil {
		s.log.ErrorF("Unable to install conn: %v", err)
		_ = conn.Close()
		return
	}
	clientConn := tls.Server(conn, curConfig)

	go transfer(serverConn, clientConn, true, s.log)
	go transfer(clientConn, serverConn, false, s.log)
}
func transfer(dest io.WriteCloser, src io.ReadCloser, save bool, log *logger.Logger) {
	var err error

	if src == nil || dest == nil {
		return
	}
	defer func() {
		if dest != nil {
			_ = dest.Close()
		}
		if src != nil {
			_ = src.Close()
		}
	}()

	buf := new(bytes.Buffer)
	multiWriter := io.MultiWriter(dest, buf)
	_, err = io.Copy(multiWriter, ioutil.NopCloser(src))
	if err != nil {
		//log.Println("afsdfsdffsadfasafds")
		return
	}

	if save {
		bufReader := bufio.NewReader(bytes.NewBuffer(buf.Bytes()))
		req, _ := http.ReadRequest(bufReader)
		if req != nil {
			requestLogger(req, log)
		}
	}
	//else {
	//	bufReader := bufio.NewReader(bytes.NewBuffer(buf.Bytes()))
	//	resp, _ := http.ReadResponse(bufReader, nil)
	//	if resp != nil {
	//		responseLogger(resp, log)
	//	}
	//}
}

func (s *Server) HandleHttp(w http.ResponseWriter, req *http.Request) {
	resp, err := http.DefaultTransport.RoundTrip(req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	defer resp.Body.Close()
	responseLogger(resp, s.log)
	copyHeader(w.Header(), resp.Header)
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}

func copyHeader(dst, src http.Header) {
	for k, vv := range src {
		for _, v := range vv {
			dst.Add(k, v)
		}
	}
}

func requestLogger(req *http.Request, log *logger.Logger) {
	if req == nil {
		return
	}
	head := fmt.Sprintf("Method: %s %s\t Request: %s", req.Method, req.Proto, req.RequestURI)
	log.Infof(head)
}

func responseLogger(resp *http.Response, log *logger.Logger) {
	status := resp.StatusCode
	reqUri := "none"
	if resp.Request != nil {
		reqUri = resp.Request.RequestURI
	}
	head := fmt.Sprintf("Method: %s\t Status: %s\t Request: %s", resp.Proto, resp.Status, reqUri)
	if status >= 400 && status < 500 {
		log.Warning(head)
	} else if status < 400 {
		if status >= 200 && status <= 202 {
			log.NoticeF(head)
		} else {
			log.Infof(head)
		}
	} else {
		log.Error(head)
	}
}
