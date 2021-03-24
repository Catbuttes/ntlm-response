package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"
)

type Config struct {
	Urls                []string          `json:"urls"`
	HTTPProxy           string            `json:"http_proxy"`
	Interface           string            `json:"interface"`
	ResponseTimeout     int               `json:"response_timeout"`
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
		},
		Timeout: time.Duration(config.ResponseTimeout) * time.Second,
	}
	method := strings.ToUpper(config.Method)
	req, _ := http.NewRequest(method, config.Urls[urlId], nil)
	req.SetBasicAuth(config.Username, config.Password)

	for key, value := range config.Headers {
		req.Header.Add(key, value)
	}

	res, err := client.Do(req)
	if err != nil {
		fmt.Print(err)
		fmt.Println("")
		os.Exit(-1)
	}
	defer res.Body.Close()

	fmt.Printf("%v\n", config.Password)
	fmt.Printf("%v\n", config.Username)
	fmt.Printf("%v\n", config.Workstation)
	fmt.Printf("%v\n", req.Header)

	data, err := ioutil.ReadAll(res.Body)
	_ = data
	if err != nil {
		fmt.Print(err)
		fmt.Println("")
		os.Exit(-1)
	}

	fmt.Printf("Status: %s\n", res.Status)
	//fmt.Printf("%s\n", data)

	wg.Done()
}

func loadConfig() Config {

	conf := Config{}
	help := flag.Bool("help", false, "Prints this message")
	configFile := flag.String("config", "config.json", "The config file to use")

	flag.Parse()

	if *help == true {
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
