package main

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// Claim is one place where a page cites an example, optionally with the
// HTTP exchange it shows for it.
type Claim struct {
	Page     string
	Line     int
	Example  string
	Exchange *DocExchange
}

// DocExchange is an HTTP exchange as written in a page.
type DocExchange struct {
	Method         string
	Target         string
	RequestHeader  []Header
	RequestBody    string
	Status         int
	ResponseHeader []Header
	ResponseBody   string
}

type Header struct{ Name, Value string }

var (
	fenceOpen    = regexp.MustCompile("^(`{3,})\\s*(\\S*)\\s*(.*)$")
	exampleAttr  = regexp.MustCompile(`\bexample="([^"]+)"`)
	inlineMarker = regexp.MustCompile("`example:([A-Za-z0-9._-]+)`")
	statusLine   = regexp.MustCompile(`^HTTP/\d(?:\.\d)? (\d{3})\b`)
	requestLine  = regexp.MustCompile(`^([A-Z]+) (\S+)(?: HTTP/\d(?:\.\d)?)?$`)
)

func parsePage(name, content string) ([]Claim, error) {
	var claims []Claim
	lines := strings.Split(content, "\n")
	for i := 0; i < len(lines); i++ {
		open := fenceOpen.FindStringSubmatch(lines[i])
		if open == nil {
			for _, marker := range inlineMarker.FindAllStringSubmatch(lines[i], -1) {
				claims = append(claims, Claim{Page: name, Line: i + 1, Example: marker[1]})
			}
			continue
		}
		start := i
		var block []string
		for i++; i < len(lines) && strings.TrimSpace(lines[i]) != open[1]; i++ {
			block = append(block, lines[i])
		}
		attr := exampleAttr.FindStringSubmatch(open[3])
		if open[2] != "http" || attr == nil {
			continue
		}
		exchange, err := parseExchange(block)
		if err != nil {
			return nil, fmt.Errorf("%s:%d: example %q: %w", name, start+1, attr[1], err)
		}
		claims = append(claims, Claim{Page: name, Line: start + 1, Example: attr[1], Exchange: exchange})
	}
	return claims, nil
}

func parseExchange(block []string) (*DocExchange, error) {
	split := -1
	for i, line := range block {
		if statusLine.MatchString(line) {
			split = i
			break
		}
	}
	if split < 0 {
		return nil, fmt.Errorf("no HTTP response status line")
	}
	start, requestHeader, requestBody := parseMessage(block[:split])
	request := requestLine.FindStringSubmatch(start)
	if request == nil {
		return nil, fmt.Errorf("invalid request line %q", start)
	}
	start, responseHeader, responseBody := parseMessage(block[split:])
	status, err := strconv.Atoi(statusLine.FindStringSubmatch(start)[1])
	if err != nil {
		return nil, fmt.Errorf("invalid status line %q", start)
	}
	return &DocExchange{
		Method: request[1], Target: request[2], RequestHeader: requestHeader, RequestBody: requestBody,
		Status: status, ResponseHeader: responseHeader, ResponseBody: responseBody,
	}, nil
}

// parseMessage splits a message into its start line, headers, and body.
// An indented line continues the previous one, so long targets can wrap.
func parseMessage(lines []string) (string, []Header, string) {
	var head []string
	body := ""
	for i, line := range lines {
		if strings.TrimSpace(line) == "" {
			body = strings.TrimSpace(strings.Join(lines[i+1:], "\n"))
			break
		}
		if len(head) > 0 && (line[0] == ' ' || line[0] == '\t') {
			head[len(head)-1] += strings.TrimSpace(line)
			continue
		}
		head = append(head, strings.TrimSpace(line))
	}
	if len(head) == 0 {
		return "", nil, body
	}
	var headers []Header
	for _, line := range head[1:] {
		name, value, _ := strings.Cut(line, ":")
		headers = append(headers, Header{Name: strings.TrimSpace(name), Value: strings.TrimSpace(value)})
	}
	return head[0], headers, body
}
