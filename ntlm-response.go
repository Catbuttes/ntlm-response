package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"net/textproto"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Config struct {
	Urls                []string          `json:"urls"`
	HTTPProxy           string            `json:"http_proxy"`
	Interface           string            `json:"interface"`
	ResponseTimeout     float64           `json:"response_timeout"`
	Method              string            `json:"method"`
	Username            string            `json:"username"`
	Password            string            `json:"password"`
	Workstation         string            `json:"workstation"`
	Body                string            `json:"body"`
	ResponseBodyField   string            `json:"response_body_field"`
	ResponseBodyMaxSize string            `json:"response_body_max_size"`
	ResponseStringMatch string            `json:"response_string_match"`
	ResponseStatusCode  int               `json:"response_status_code"`
	Headers             map[string]string `json:"headers"`
	HTTPHeaderTags      map[string]string `json:"http_header_tags"`
	HTTPHeaderMetrics   map[string]string `json:"http_header_metrics"`
}

func main() {

	conf := loadConfig()

	var waitGroup sync.WaitGroup

	for i := 0; i < len(conf.Urls); i++ {
		waitGroup.Add(1)

		go checkUrl(i, conf, &waitGroup)
	}

	waitGroup.Wait()

}

func checkUrl(urlId int, config Config, wg *sync.WaitGroup) {
	tags := make(map[string]string)
	metrics := make(map[string]string)

	addr, err := url.Parse(config.Urls[urlId])
	if err != nil {
		fmt.Print(err)
		fmt.Println("")
		os.Exit(-1)
	}

	if addr.Scheme != "http" && addr.Scheme != "https" {
		fmt.Print("Only http and https are supported")
		fmt.Println("")
		os.Exit(-1)
	}

	tags["method"] = config.Method
	tags["server"] = addr.String()

	dialer := &net.Dialer{}

	if config.Interface != "" {
		dialer.LocalAddr, err = localAddress(config.Interface)
		if err != nil {
			fmt.Print(err)
			fmt.Println("")
			os.Exit(-1)
		}
	}

	proxyURL, err := url.Parse(config.HTTPProxy)
	if err != nil {
		fmt.Print(err)
		fmt.Println("")
		os.Exit(-1)
	}

	proxy := func(r *http.Request) (*url.URL, error) {
		return proxyURL, nil
	}

	if config.HTTPProxy == "" {
		proxy = http.ProxyFromEnvironment
	}

	client := &http.Client{
		Transport: NtlmNegotiator{
			RoundTripper: &http.Transport{
				Proxy:       proxy,
				DialContext: dialer.DialContext,
			},
			Workstation: config.Workstation,
			Username:    config.Username,
			Password:    config.Password,
		},
		Timeout: time.Duration(config.ResponseTimeout) * time.Second,
	}
	method := strings.ToUpper(config.Method)
	req, _ := http.NewRequest(method, config.Urls[urlId], nil)
	//req.SetBasicAuth(config.Username, config.Password)

	for key, value := range config.Headers {
		if strings.ToLower(key) == "host" {
			req.Host = value
		}
		req.Header.Add(key, value)

	}

	if config.Body != "" {
		req.Body = ioutil.NopCloser(bytes.NewReader([]byte(config.Body)))
	}

	start := time.Now()
	res, err := client.Do(req)
	responseTime := time.Since(start).Seconds()
	metrics["response_time"] = strconv.FormatFloat(responseTime, 'f', 5, 64)
	if err != nil {
		if timeoutError, ok := err.(net.Error); ok && timeoutError.Timeout() {
			tags["result"] = "timeout"
			metrics["result_code"] = "4"
		} else {
			urlErr, isURLErr := err.(*url.Error)
			if isURLErr {

				opErr, isNetErr := (urlErr.Err).(*net.OpError)
				if isNetErr {
					switch (opErr.Err).(type) {
					case (*net.DNSError):
						tags["result"] = "dns_error"
						metrics["result_code"] = "5"
					case (*net.ParseError):
						tags["result"] = "connection_failed"
						metrics["result_code"] = "3"
					default:
						tags["result"] = "connection_failed"
						metrics["result_code"] = "3"
					}
				}
			}
		}
	} else {
		defer res.Body.Close()

		tags["status_code"] = strconv.Itoa(res.StatusCode)
		metrics["http_response_code"] = strconv.Itoa(res.StatusCode) + "i"

		if config.ResponseStatusCode > 0 {
			if res.StatusCode != config.ResponseStatusCode {
				if tags["result"] == "" {
					tags["result"] = "response_status_code_mismatch"
					metrics["result_code"] = "6"
				}
			}
		}

		for headerName, tag := range config.HTTPHeaderTags {
			fixedHeader := textproto.CanonicalMIMEHeaderKey(headerName)
			headerValues, foundHeader := res.Header[fixedHeader]
			if foundHeader && len(headerValues) > 0 {
				tags[tag] = strings.Join(headerValues, " ")
			}
		}

		for headerName, metric := range config.HTTPHeaderMetrics {
			fixedHeader := textproto.CanonicalMIMEHeaderKey(headerName)
			headerValues, foundHeader := res.Header[fixedHeader]
			if foundHeader && len(headerValues) > 0 {
				metrics[metric] = strings.Join(headerValues, " ")
			}
		}

		data, err := ioutil.ReadAll(res.Body)
		if err == nil {
			metrics["content_length"] = strconv.Itoa(len(data)) + "i"

			if config.ResponseBodyField != "" {
				metrics[config.ResponseBodyField] = string(data)
			}

			if config.ResponseStringMatch != "" {
				matched, err := regexp.Match(config.ResponseStringMatch, data)
				if err != nil {
					tags["result"] = "body_read_error"
					metrics["result_code"] = "2"
				}

				if !matched {
					if tags["result"] == "" {
						tags["result"] = "response_string_mismatch"
						metrics["result_code"] = "1"
						metrics["response_string_match"] = "0"
					}
				}
			}

		} else {
			if tags["result"] == "" {
				tags["result"] = "body_read_error"
				metrics["result_code"] = "2"
			}
		}

	}

	if tags["result"] == "" {
		tags["result"] = "success"
		metrics["result_code"] = "0"
	}

	delimiter := ","
	fmt.Print("ntlm_response")

	for key, value := range tags {
		tagValue := strings.ReplaceAll(value, "=", "\\=")
		tagValue = strings.ReplaceAll(tagValue, " ", "\\ ")
		tagValue = strings.ReplaceAll(tagValue, ",", "\\,")

		fmt.Printf("%s%s=%s", delimiter, key, tagValue)
	}

	delimiter = " "
	for key, value := range metrics {
		metricValue := strings.ReplaceAll(value, "=", "\\=")
		metricValue = strings.ReplaceAll(metricValue, " ", "\\ ")
		metricValue = strings.ReplaceAll(metricValue, ",", "\\,")

		if strings.HasSuffix(key, "_s") {
			fmt.Printf("%s%s=\"%s\"", delimiter, strings.TrimSuffix(key, "_s"), metricValue)
		} else if strings.HasSuffix(key, "_i") {
			fmt.Printf("%s%s=%si", delimiter, strings.TrimSuffix(key, "_i"), metricValue)
		} else {
			fmt.Printf("%s%s=%s", delimiter, key, metricValue)
		}

		delimiter = ","
	}

	fmt.Print("\n")

	wg.Done()
}

func loadConfig() Config {

	conf := Config{}
	help := flag.Bool("help", false, "Prints this message")
	configFile := flag.String("config", "config.json", "The config file to use")

	flag.Parse()

	if *help {
		displayHelp()
		os.Exit(0)
	}

	configContents, err := ioutil.ReadFile(*configFile)
	if err != nil {
		fmt.Print(err)
		fmt.Println("")
		os.Exit(-1)
	}

	json.Unmarshal([]byte(configContents), &conf)

	return conf
}

func displayHelp() {
	fmt.Println("ntlm-response")
	fmt.Println("An NTLM HTTP Response metrics checker")
	fmt.Println("https://github.com/Catbuttes/ntlm-response")
	fmt.Println("")

	flag.PrintDefaults()
	fmt.Println("")
}

func localAddress(interfaceName string) (net.Addr, error) {
	i, err := net.InterfaceByName(interfaceName)
	if err != nil {
		return nil, err
	}

	addrs, err := i.Addrs()
	if err != nil {
		return nil, err
	}

	for _, addr := range addrs {
		if naddr, ok := addr.(*net.IPNet); ok {
			// leaving port set to zero to let kernel pick
			return &net.TCPAddr{IP: naddr.IP}, nil
		}
	}

	return nil, fmt.Errorf("cannot create local address for interface %q", interfaceName)
}
