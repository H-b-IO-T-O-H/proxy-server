package app

import (
	"crypto/tls"
	"fmt"
	"github.com/H-b-IO-T-O-H/proxy-server/app/certificates"
	"github.com/apsdehal/go-logger"
	"io"
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
			log.Println(err)
		}
	}()
	if r.Method == http.MethodConnect {
		fmt.Println("https")
		s.HandleTunneling(w, r)
		//s.LaunchSecureConnection(w, r)
	} else {
		s.HandleHTTP(w, r)
	}
}

func (s *Server) HandleTunneling(w http.ResponseWriter, r *http.Request) {

	leafCert, err := certificates.GenerateCert(&s.ca, r.Host)
	if err != nil {
		//log.Fatalf("Error while generating certificates: %s\n", err.Error())
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

	serverConn, err := tls.Dial("tcp", r.Host, curConfig)
	hijacker, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "Hijacking not supported", http.StatusInternalServerError)
		return
	}

	conn, _, err := hijacker.Hijack()
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	//w.WriteHeader(http.StatusOK)


	_, err = conn.Write([]byte(OKHeader))
	if err != nil {
		log.Printf("Unable to install conn: %v", err)
		_ = conn.Close()
		return
	}
	clientConn := tls.Server(conn, curConfig)
	//requestLogger()
	//err = clientConn.Handshake()
	//if err != nil {
	//	log.Printf("Unable to handshake: %v", err)
	//	_ = clientConn.Close()
	//	_ = conn.Close()
	//	return
	//}

	go transfer(serverConn, clientConn)
	go transfer(clientConn, serverConn)
}
func transfer(destination io.WriteCloser, source io.ReadCloser) {
	defer destination.Close()
	defer source.Close()
	io.Copy(destination, source)
}

func requestLogger(resp *http.Response, log *logger.Logger) {
	status := resp.StatusCode
	head := fmt.Sprintf("Method: %s\t Status: %s\t Request: %s", resp.Proto, resp.Status, resp.Request.RequestURI)
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

func (s *Server) HandleHTTP(w http.ResponseWriter, req *http.Request) {
	resp, err := http.DefaultTransport.RoundTrip(req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	defer resp.Body.Close()
	requestLogger(resp, s.log)
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
