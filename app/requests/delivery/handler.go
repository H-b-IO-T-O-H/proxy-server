package delivery

import (
	"bytes"
	"compress/gzip"
	"crypto/tls"
	"fmt"
	"github.com/H-b-IO-T-O-H/proxy-server/app/common/constants"
	"github.com/H-b-IO-T-O-H/proxy-server/app/common/errors"
	"github.com/H-b-IO-T-O-H/proxy-server/app/common/models"
	"github.com/H-b-IO-T-O-H/proxy-server/app/database"
	"github.com/H-b-IO-T-O-H/proxy-server/app/requests"
	"github.com/gin-gonic/gin"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type Resp struct {
	Request *models.RequestJSON `json:"request"`
}

type RespList struct {
	Requests models.Requests `json:"requests"`
}

type RespScanner struct {
	ScanStatus  string   `json:"scan_status"`
	QueryScan   string   `json:"scan_query"`
	BodyScan    string   `json:"scan_body"`
	ThreatsList []string `json:"used_attack_vectors"`
}

type RequestHandler struct {
	RequestRepository requests.RepositoryRequest
}

func NewRest(reqRep requests.RepositoryRequest) *RequestHandler {
	rest := &RequestHandler{RequestRepository: reqRep}
	return rest
}

func (r RequestHandler) GetRequest(ctx *gin.Context) {
	var id = 0

	if id, _ = strconv.Atoi(ctx.Param("id")); id == 0 {
		ctx.JSON(http.StatusBadRequest, errors.RespErr{Message: errors.EmptyFieldErr})
		return
	}

	buf, err := r.RequestRepository.GetRequest(id)
	if err != nil {
		ctx.JSON(err.StatusCode(), err)
		return
	}
	ctx.JSON(http.StatusOK, Resp{buf})
}

func (r RequestHandler) ScanRequest(ctx *gin.Context) {
	var id = 0

	if id, _ = strconv.Atoi(ctx.Param("id")); id == 0 {
		ctx.JSON(http.StatusBadRequest, errors.RespErr{Message: errors.EmptyFieldErr})
		return
	}

	buf, err := r.RequestRepository.GetRequest(id)
	if err != nil {
		ctx.JSON(err.StatusCode(), err)
		return
	}
	reqDb, errConvert := database.ConvertModelToRequest(buf.Req, buf.IsHTTPS)
	if errConvert != nil {
		ctx.JSON(http.StatusInternalServerError, "can't parse request")
		return
	}

	Scanner(ctx, reqDb, buf.IsHTTPS)
}

func (r RequestHandler) GetRequests(ctx *gin.Context) {
	var req struct {
		Start int  `form:"start"`
		Limit int  `form:"limit" binding:"required"`
		Full  bool `form:"full"`
	}
	if err := ctx.ShouldBindQuery(&req); err != nil || req.Limit <= 0 || req.Start < 0 {
		ctx.JSON(http.StatusBadRequest, errors.RespErr{Message: errors.EmptyFieldErr})
		return
	}

	buf, err := r.RequestRepository.GetRequests(req.Start, req.Limit, req.Full)
	if err != nil {
		ctx.JSON(err.StatusCode(), err)
		return
	}
	ctx.JSON(http.StatusOK, RespList{buf})
}

func (r RequestHandler) RepeatRequest(ctx *gin.Context) {
	var id = 0

	if id, _ = strconv.Atoi(ctx.Param("id")); id == 0 {
		ctx.JSON(http.StatusBadRequest, errors.RespErr{Message: errors.EmptyFieldErr})
		return
	}

	buf, err := r.RequestRepository.GetRequest(id)
	if err != nil {
		ctx.JSON(err.StatusCode(), err)
		return
	}
	reqDb, errConvert := database.ConvertModelToRequest(buf.Req, buf.IsHTTPS)
	if errConvert != nil {
		ctx.JSON(http.StatusInternalServerError, "can't parse request")
		return
	}

	if buf.IsHTTPS {
		WriteHttps(ctx.Writer, r.RequestRepository, reqDb)
	} else {
		WriteHttp(ctx.Writer, r.RequestRepository, reqDb)
	}
}

func WriteHttp(w http.ResponseWriter, db requests.RepositoryRequest, req *http.Request) {
	resp, err := SendHttps(req)

	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	//if errReq := database.SaveRequest(db, req, false); errReq != nil {
	//	fmt.Printf("Can't save request: %v\n", errReq)
	//}
	w.WriteHeader(resp.StatusCode)
	for key, values := range resp.Header {
		if key != "Location" {
			w.Header().Set(key, strings.Join(values, ", "))
		}
	}

	io.Copy(w, resp.Body)
}

func getRespBody(resp io.ReadCloser, gzipped bool) ([]byte, error) {
	var err error
	var respBody []byte

	if gzipped {
		reader, err := gzip.NewReader(resp)
		if err != nil {
			return nil, err
		}

		if respBody, err = ioutil.ReadAll(reader); err != nil {
			log.Fatal(err)
		}
	} else {
		if respBody, err = ioutil.ReadAll(resp); err != nil {
			log.Fatal(err)
		}
	}
	return respBody, err
}

func getServiceInfo(req *http.Request, isHttps bool) (bool, bool) {
	var resp *http.Response
	var err error
	var serviceUnavailable = true

	if isHttps {
		resp, err = SendHttps(req)
	} else {
		resp, err = SendHttp(req)
	}
	defer func() {
		if resp != nil {
			_ = resp.Body.Close()
		}
	}()

	if err == nil && resp.StatusCode >= http.StatusOK && resp.StatusCode <= http.StatusAccepted {
		serviceUnavailable = !serviceUnavailable
	} else {
		return serviceUnavailable, false
	}

	for key, values := range resp.Header {
		if key == "Content-Encoding" {
			for _, v := range values {
				if v == "gzip" {
					return false, true
				}
			}
		}
	}

	return false, false
}

func requestRepeater(req *http.Request, isHttps bool, gzipped bool) (bool, error) {
	var resp *http.Response
	var err error

	if isHttps {
		resp, err = SendHttps(req)
	} else {
		resp, err = SendHttp(req)
	}
	if err != nil {
		return false, err
	}
	if resp.StatusCode != http.StatusOK {
		return false, nil
	}

	body, err := getRespBody(resp.Body, gzipped)
	if err != nil {
		return false, err
	}

	if strings.Contains(string(body), "root:") {
		return true, nil
	}
	_ = resp.Body.Close()
	return false, nil
}

func Scanner(ctx *gin.Context, req *http.Request, isHttps bool) {

	var commands = []string{
		";cat /etc/passwd;",
		"|cat /etc/passwd|",
		"`cat /etc/passwd`",
		"||ping+-c+10+127.0.0.1||",
	}
	status := RespScanner{ThreatsList: commands}

	reqBodyBytes, _ := ioutil.ReadAll(req.Body)
	reqBodyString := string(reqBodyBytes)
	req.Body = ioutil.NopCloser(bytes.NewReader(reqBodyBytes))

	unavailable, gzipped := getServiceInfo(req, isHttps)
	if unavailable {
		status.ScanStatus = constants.ScanUnavailable
		ctx.JSON(http.StatusOK, status)
		return
	}

	params := strings.Split(reqBodyString, "=")
	modifiedBodyString := ""
	parsedBody := ""
	paramsCount := len(params)
	oldReqQuery := req.URL.RawQuery
	for _, command := range commands {
		// query scan
		req.RequestURI = oldReqQuery
		u, _ := url.ParseQuery(req.URL.RawQuery)
		if len(u) == 0 {
			status.QueryScan = constants.ScanNoQueryParams
		}
		for key, param := range u {
			u.Set(key, param[0]+command)
			req.URL.RawQuery = u.Encode()
			req.Body = ioutil.NopCloser(bytes.NewReader(reqBodyBytes))

			vulnerableByParams, err := requestRepeater(req, isHttps, gzipped)
			if err != nil {
				ctx.JSON(http.StatusInternalServerError, err)
			}
			if vulnerableByParams {
				status.QueryScan = constants.ScanVulnerableByQuery
				break
			}
		}
		if status.QueryScan != "" {
			break
		}
	}
	// body scan

	for _, command := range commands {
		modifiedBodyString = ""
		parsedBody = ""
		for i := 0; i < paramsCount-1; i++ {
			if parsedBody != "" {
				parsedBody = fmt.Sprintf("%s=%s", parsedBody, params[i])
			} else {
				parsedBody = params[0]
			}
			paramsDivided := strings.Split(params[i+1], "&")
			modifiedBodyString = parsedBody + "=" + paramsDivided[0] + command
			if len(paramsDivided) == 2 {
				modifiedBodyString += "&" + paramsDivided[1]
			}
			for j := i + 2; j < paramsCount; j++ {
				if i+1 != j && i != paramsCount-2 {
					modifiedBodyString = modifiedBodyString + "=" + params[j]
				} else {
					modifiedBodyString += params[j]
				}
			}
			req.Body = ioutil.NopCloser(bytes.NewReader([]byte(modifiedBodyString)))
			req.Header.Set("Content-Length", strconv.Itoa(len(modifiedBodyString)))
			req.ContentLength = int64(len(modifiedBodyString))

			vulnerableByParams, err := requestRepeater(req, isHttps, gzipped)

			if err != nil {
				if errors.TimeoutCheck(err.Error()) {
					status.BodyScan = fmt.Sprintf("%s (find in %s)", constants.TimeBasedVulnerableByBody, params[i])
				} else {
					ctx.JSON(http.StatusInternalServerError, err)
					return
				}

			}
			if vulnerableByParams {
				status.BodyScan = fmt.Sprintf("%s (find in %s)", constants.ScanVulnerableByBody, params[i])
				break
			}
		}
		if status.BodyScan != "" {
			break
		}
	}
	if status.QueryScan == "" && status.BodyScan == "" {
		status.ScanStatus = constants.ScanNotFound
	} else {
		status.ScanStatus = constants.ScanStatusOk
	}
	status.ThreatsList = commands
	ctx.JSON(http.StatusOK, status)
}

func SendHttp(req *http.Request) (*http.Response, error) {
	resp, err := http.DefaultTransport.RoundTrip(req)

	if err != nil {
		return nil, err
	}
	return resp, nil
}

func SendHttps(req *http.Request) (*http.Response, error) {
	req.RequestURI = ""
	client := &http.Client{
		Timeout: 2 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	return resp, nil
}

func WriteHttps(w http.ResponseWriter, db requests.RepositoryRequest, req *http.Request) {
	resp, err := SendHttps(req)
	defer func() {
		_ = resp.Body.Close()
	}()
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	//if errReq := database.SaveRequest(db, req, false); errReq != nil {
	//	fmt.Printf("Can't save request: %v\n", errReq)
	//}
	binary, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Println(err)
	}
	for key, values := range resp.Header {
		w.Header().Set(key, strings.Join(values, ", "))
	}

	_, err = w.Write(binary)
	if err != nil {
		log.Println(err)
	}
}
