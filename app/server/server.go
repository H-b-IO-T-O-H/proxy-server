package server

import (
	"bufio"
	"bytes"
	"crypto/tls"
	"fmt"
	"github.com/H-b-IO-T-O-H/proxy-server/app/certificates"
	"github.com/H-b-IO-T-O-H/proxy-server/app/common/constants"
	"github.com/H-b-IO-T-O-H/proxy-server/app/common/errors"
	Log "github.com/H-b-IO-T-O-H/proxy-server/app/common/logger"
	"github.com/H-b-IO-T-O-H/proxy-server/app/database"
	"github.com/H-b-IO-T-O-H/proxy-server/app/requests"
	handlers "github.com/H-b-IO-T-O-H/proxy-server/app/requests/delivery"
	reqRepository "github.com/H-b-IO-T-O-H/proxy-server/app/requests/repository"
	"github.com/apsdehal/go-logger"
	"github.com/gin-gonic/gin"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

type DBConfig struct {
	Host     string `yaml:"host"`
	Port     int    `yaml:"port"`
	User     string `yaml:"user"`
	Password string `yaml:"password"`
	Name     string `yaml:"name"`
}

type Config struct {
	ListenProxy string   `yaml:"proxy"`
	ListenGui   string   `yaml:"gui"`
	Db          DBConfig `yaml:"db"`
	FindMethod  string
	MaxLength   uint
	SessionSave bool
}

type Server struct {
	Ca         tls.Certificate
	HttpClient *http.Client
	Log        *logger.Logger
	Db         requests.RepositoryRequest
	DoneChan   chan bool
	Config     *Config
	Router     *gin.Engine
}

func NewServer(config Config) (*Server, error) {
	var err error
	gin.SetMode(gin.ReleaseMode)

	infoLogger, _ := logger.New("Info requestLogger", 1, os.Stdout)
	server := Server{
		HttpClient: new(http.Client),
		Config:     &config,
		Log:        infoLogger,
		DoneChan:   make(chan bool, 1),
		Router:     gin.Default(),
	}

	server.HttpClient.Timeout = 5 * time.Second
	server.Ca, err = certificates.LoadCA()
	if err != nil {
		return nil, err
	}

	return &server, nil
}

func (s *Server) Run() {
	credentials := fmt.Sprintf("user=%s password=%s dbname=%s host=%s port=%d", s.Config.Db.User,
		s.Config.Db.Password, s.Config.Db.Name,
		s.Config.Db.Host, s.Config.Db.Port)
	db, err := gorm.Open(postgres.Open(credentials), &gorm.Config{})
	if err != nil {
		s.Log.Fatal("connection to postgres db failed...")
	}

	RequestsRep := reqRepository.NewPgRepository(db)
	resp := handlers.NewRest(RequestsRep)
	s.Db = RequestsRep

	s.Router.GET("/requests", func(ctx *gin.Context) {
		resp.GetRequests(ctx)
	})
	s.Router.GET("/request/:id", func(ctx *gin.Context) {
		resp.GetRequest(ctx)
	})
	s.Router.GET("/repeat/:id", func(ctx *gin.Context) {
		resp.RepeatRequest(ctx)
	})
	s.Router.GET("/scan/:id", func(ctx *gin.Context) {
		resp.ScanRequest(ctx)
	})

	go func() {
		s.Log.Infof("Gui start listening on %s", s.Config.ListenGui)
		if err := s.Router.Run(s.Config.ListenGui); err != http.ErrServerClosed {
			s.Log.Fatalf("Error in gui server run: %v\n", err)
		}
	}()

	go func() {
		s.Log.Infof("Proxy start listening on %s", s.Config.ListenProxy)
		if err := http.ListenAndServe(s.Config.ListenProxy, s); err != http.ErrServerClosed {
			s.Log.Fatalf("Error in proxy server run: %v\n", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	select {
	case <-quit:
	case <-s.DoneChan:
	}
	if !s.Config.SessionSave {
		if err := s.Db.DeleteRequests(); err != nil {
			s.Log.Fatal(err.Msg())
		} else {
			s.Log.Notice("Prepare database... Clear all requests.")
		}
	} else {
		s.Log.Notice("Prepare database... Save all new requests.")
	}
	s.Log.Info("Shutdown Server (timeout of 1 seconds) ...\nServer exiting")
}

func (s *Server) Close() {
	s.DoneChan <- true
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	defer func() {
		err := recover()
		if err != nil {
			s.Log.Fatal("unexpected error was met during server recover")
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

	leafCert, err := certificates.GenerateCert(&s.Ca, r.Host)
	if err != nil {
		s.Log.FatalF("Error while generating certificates: %s\n", err.Error())
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}

	curCert := make([]tls.Certificate, 1)
	curCert[0] = *leafCert

	curConfig := &tls.Config{
		Certificates: curCert,
		GetCertificate: func(info *tls.ClientHelloInfo) (certificate *tls.Certificate, e error) {
			return certificates.GenerateCert(&s.Ca, info.ServerName)
		},
	}

	serverConn, errServ := tls.Dial("tcp", r.Host, curConfig)
	if errServ != nil {
		s.Log.ErrorF("Service unavailable: %v", errServ)
		http.Error(w, errServ.Error(), http.StatusServiceUnavailable)
		return
	}
	hijacker, ok := w.(http.Hijacker)
	if !ok {
		msg := "Hijacking not supported"
		s.Log.ErrorF(msg)
		http.Error(w, msg, http.StatusInternalServerError)
		return
	}

	conn, _, err := hijacker.Hijack()
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}

	if _, err = conn.Write(constants.HeaderOk); err != nil {
		s.Log.ErrorF("Unable to install conn: %v", err)
		_ = conn.Close()
		return
	}
	clientConn := tls.Server(conn, curConfig)
	go transfer(serverConn, clientConn, true, s)
	go transfer(clientConn, serverConn, false, s)
}
func transfer(dest io.WriteCloser, src io.ReadCloser, save bool, s *Server) {

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
	_, _ = io.Copy(multiWriter, ioutil.NopCloser(src))

	if save {
		bufReader := bufio.NewReader(bytes.NewBuffer(buf.Bytes()))
		req, _ := http.ReadRequest(bufReader)
		HandleRequest(s, req, true)
	}
}

func (s *Server) HandleHttp(w http.ResponseWriter, req *http.Request) {
	HandleRequest(s, req, false)
	resp, err := http.DefaultTransport.RoundTrip(req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	defer func() {
		if resp.Body != nil {
			_ = resp.Body.Close()
		}
	}()
	//common.ResponseLogger(resp, s.log)
	copyHeader(w.Header(), resp.Header)
	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, resp.Body)
}

func copyHeader(dst, src http.Header) {
	for k, vv := range src {
		for _, v := range vv {
			dst.Add(k, v)
		}
	}
}

func HandleRequest(s *Server, req *http.Request, isHttps bool) {
	var id int
	var errReq errors.Err

	if req != nil && (s.Config.FindMethod == constants.AllMethods ||
		strings.Contains(req.Method, s.Config.FindMethod)) {

		if id, errReq = database.SaveRequest(s.Db, req, isHttps); errReq != nil {
			s.Log.ErrorF("Can't save request: %v", errReq)
		}
		Log.RequestLogger(req, s.Log, s.Config.MaxLength, id)
	} else {
		Log.RequestLogger(req, s.Log, s.Config.MaxLength, 0)
	}
}
