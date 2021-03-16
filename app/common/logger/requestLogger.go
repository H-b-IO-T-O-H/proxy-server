package logger

import (
	"fmt"
	"github.com/H-b-IO-T-O-H/proxy-server/app/common/constants"
	"github.com/apsdehal/go-logger"
	"net/http"
)

func RequestLogger(req *http.Request, log *logger.Logger, uriMaxLen uint, trackedId int) {
	if req == nil {
		return
	}
	uri := req.RequestURI
	if size := len(uri); uint(size) > uriMaxLen {
		uri = uri[:uriMaxLen] + "...(trimmed)"
	}

	head := fmt.Sprintf("Method: %s %s\t Request: %s", req.Method, req.Proto, uri)
	if trackedId != 0 {
		fmt.Printf("%s %s %s\n", constants.Green("#Save request №"),
			constants.Yellow(trackedId), constants.Green("to the database"))
		log.Notice(head)
	} else {
		log.Info(head)
	}
}

func ResponseLogger(resp *http.Response, log *logger.Logger) {
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
