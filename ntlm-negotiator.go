package main

import (
	"bytes"
	"encoding/base64"
	"io"
	"io/ioutil"
	"net/http"
	"strings"

	"github.com/Azure/go-ntlmssp"
)

// The contents of this file are shamelessly cribbed from the roundtripper at github.com/Azure/go-ntlmssp.
// Some changes have been made to support specifying the workstation

// GetDomain : parse domain name from based on slashes in the input
func GetDomain(user string) string {
	domain := ""

	if strings.Contains(user, "\\") {
		ucomponents := strings.SplitN(user, "\\", 2)
		domain = ucomponents[0]
	}
	return domain
}

func GetUsername(user string) string {
	username := user

	if strings.Contains(user, "\\") {
		ucomponents := strings.SplitN(user, "\\", 2)
		username = ucomponents[1]
	}
	return username
}

//NtlmNegotiator is a http.Roundtripper decorator that automatically
//converts basic authentication to NTLM/Negotiate authentication when appropriate.
type NtlmNegotiator struct {
	Username    string
	Password    string
	Workstation string
	http.RoundTripper
}

//RoundTrip sends the request to the server, handling any authentication
//re-sends as needed.
func (l NtlmNegotiator) RoundTrip(req *http.Request) (res *http.Response, err error) {
	// Use default round tripper if not provided
	rt := l.RoundTripper
	if rt == nil {
		rt = http.DefaultTransport
	}

	// Save request body
	body := bytes.Buffer{}
	if req.Body != nil {
		_, err = body.ReadFrom(req.Body)
		if err != nil {
			return nil, err
		}

		req.Body.Close()
		req.Body = ioutil.NopCloser(bytes.NewReader(body.Bytes()))
	}
	// first try anonymous, in case the server still finds us
	// authenticated from previous traffic
	req.Header.Del("Authorization")
	res, err = rt.RoundTrip(req)
	if err != nil {
		return nil, err
	}

	if res.StatusCode != http.StatusUnauthorized {
		return res, err
	}

	resauth := authheader(strings.Join(res.Header.Values("Www-Authenticate"), " "))

	if resauth.IsNTLM() {
		// 401 with request:Basic and response:Negotiate
		io.Copy(ioutil.Discard, res.Body)
		res.Body.Close()

		// get domain from username
		domain := GetDomain(l.Username)

		// send negotiate
		negotiateMessage, err := ntlmssp.NewNegotiateMessage(domain, l.Workstation)
		if err != nil {
			return nil, err
		}

		req.Header.Set("Authorization", "NTLM "+base64.StdEncoding.EncodeToString(negotiateMessage))

		req.Body = ioutil.NopCloser(bytes.NewReader(body.Bytes()))

		res, err = rt.RoundTrip(req)
		if err != nil {
			return nil, err
		}

		// receive challenge?
		resauth = authheader(strings.Join(res.Header.Values("Www-Authenticate"), " "))
		challengeMessage, err := resauth.GetData()
		if err != nil {
			return nil, err
		}
		if !(resauth.IsNTLM()) || len(challengeMessage) == 0 {
			// Negotiation failed, let client deal with response
			return res, nil
		}
		io.Copy(ioutil.Discard, res.Body)
		res.Body.Close()

		user := GetUsername(l.Username)

		// send authenticate
		authenticateMessage, err := ntlmssp.ProcessChallenge(challengeMessage, user, l.Password)
		if err != nil {
			return nil, err
		}

		req.Header.Set("Authorization", "NTLM "+base64.StdEncoding.EncodeToString(authenticateMessage))

		req.Body = ioutil.NopCloser(bytes.NewReader(body.Bytes()))

		res, err := rt.RoundTrip(req)

		return res, err
	}

	return res, err
}
