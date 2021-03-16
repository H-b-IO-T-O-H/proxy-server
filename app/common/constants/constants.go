package constants

import "github.com/fatih/color"

const AllowedMethods = "GET, POST, PUT, CONNECT, DELETE, OPTIONS, PATCH, PUT, TRACE"
const AllMethods = "ALL METHODS"
const ScanStatusOk = "OK"
const ScanNotFound = "No vulnerabilities found by modifying request parameters"
const ScanUnavailable = "The service being scanned is not available"

const ScanVulnerableByQuery = "Request have vulnerability in query params"
const ScanVulnerableByBody = "Request have vulnerability in body params"
const ScanNoQueryParams = "Request being scanned haven't got query params"
const TimeBasedVulnerableByBody = "Request have time-based vulnerability in body params"


var Yellow = color.New(color.FgYellow, color.Bold).SprintFunc()
var Green = color.New(color.FgGreen, color.Bold).SprintFunc()
var HeaderOk = []byte("HTTP/1.1 200 OK\r\n\r\n")
