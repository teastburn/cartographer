// A test script to measure the speed of large amounts of HTTP requests to the server
// Given a CSV file of coordinates, parse and concurrently send coords
// to the server as fast as possible.
// Ex: make cities
// Ex: go run tools/import/main.go -f mycoords.csv
package main

import (
	"bufio"
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
	"encoding/json"

	"golang.org/x/net/http2"
)

var (
	filePath       = flag.String("f", "cities5.csv", "CSV file to import.")
	hostAddress    = flag.String("h", "https://localhost:8080", "Proto, host and port of server to bombar.")
	writeEndpoint  = flag.String("w", "/geo", "Path of write endpoint on -h option")
	configEndpoint = flag.String("i", "/info", "Path of info endpoint on -h option")
	certFile       = flag.String("c", "cert.pem", "Path to cert.pem file for TLS")
)

// Config represents the data needed to start and mark finished an http request to our server
type Config struct {
	Listeners                int    `json:"listeners"`                // Unmarshalled from server. Capital for Unmarshall.
	Protocol                 string `json:"protocol"`                 // Unmarshalled from server. Capital for Unmarshall.
	NumGoroutines            int    `json:"numGoroutines"`            // Unmarshalled from server. Capital for Unmarshall.
	NumCPU                   int    `json:"numCPU"`                   // Unmarshalled from server. Capital for Unmarshall.
	ConcurrentRequestsServer int    `json:"concurrentRequestsServer"` // Unmarshalled from server. Capital for Unmarshall.
	concurrentRequestsClient int    `json:"concurrentRequestsClient"`
	totalRequests            int64  `json:"totalRequests"`
	totalTimeMs              int64  `json:"totalTimeMs"`
	avgTimeμs                int64  `json:"avgTimeμs"`
}

// Request represents the data needed to start and mark finished an http request to our server
type Request struct {
	wg     *sync.WaitGroup // to allow the main program to know when all goroutines are finished
	num    int64           // the number/ID of the request being made (for logging purposes)
	line   string          // the text line from the CSV import file
	client http.Client     // pre-configured http2 client
	url    string          // URL to send the request to
}

func (r Request) String() string {
	return fmt.Sprintf("[%d] %s", r.num, r.line)
}

func main() {
	flag.Parse()

	// open file into scanner
	file, err := os.Open(*filePath)
	if err != nil {
		log.Fatal(err)
	}
	defer file.Close()

	client := newClient(*certFile)
	config := getServerConfig(client, fmt.Sprintf("%s%s", *hostAddress, *configEndpoint))
	config.concurrentRequestsClient = config.ConcurrentRequestsServer / 2

	var totalRequests int64
	wg := &sync.WaitGroup{}

	receiveChan := make(chan Request, config.concurrentRequestsClient)
	doneChan := make(chan bool)

	for i := 1; i <= config.concurrentRequestsClient; i++ {
		go receiveRoutine(receiveChan, doneChan)
	}

	scanner := bufio.NewScanner(file)
	startTime := time.Now()

	for scanner.Scan() {
		totalRequests++
		wg.Add(1)
		req := Request{
			wg:     wg,
			num:    totalRequests,
			line:   scanner.Text(),
			client: client,
			url:    fmt.Sprintf("%s%s", *hostAddress, *writeEndpoint),
		}
		receiveChan <- req
	}

	wg.Wait()
	doneChan <- true

	config.totalRequests = totalRequests
	timeTaken := time.Since(startTime)
	config.totalTimeMs = int64(timeTaken / time.Millisecond)
	config.avgTimeμs = int64(timeTaken/time.Microsecond) / config.totalRequests
	log.Printf("Done. %#v", config)
	// TODO maybe write results to file for safekeeping?
}

// receiveRoutine waits for Requests on receiveChan and processes them
// until a message is received on doneChan
func receiveRoutine(receiveChan chan Request, doneChan chan bool) {
	for {
		select {
		case r := <-receiveChan:
			func() {
				defer r.wg.Done()

				err := writeLocation(r)
				if err != nil {
					log.Printf("Error POSTing %#v: %s", err, err.Error())
				}

				if r.num%1000 == 0 {
					log.Printf("Completed %d", r.num)
				}
			}()
		case <-doneChan:
			return
		}
	}
}

// Write the coordinates to the server
func writeLocation(r Request) error {
	coords := strings.Split(r.line, ",")
	jsonStr := []byte(fmt.Sprintf(`{"lat":%s,"lon":%s}`, coords[0], coords[1]))
	_, err := r.client.Post(r.url, "application/json", bytes.NewBuffer(jsonStr))
	return err
}

// Return body of config endpoint to show the environment server was running with during test
func getServerConfig(client http.Client, url string) Config {
	resp, err := client.Get(url)
	if err != nil {
		log.Fatal(err)
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	config := Config{}
	json.Unmarshal(body, &config)

	return config
}

// Create a new http client with our cert added into CA pool
// If you haven't created a cert, see Installation section of README.md
func newClient(certFile string) http.Client {
	CA_Pool := x509.NewCertPool()
	severCert, err := ioutil.ReadFile(certFile)
	if err != nil {
		log.Fatal("Could not load server certificate!")
	}
	CA_Pool.AppendCertsFromPEM(severCert)

	return http.Client{
		Timeout: time.Second * 5,
		Transport: &http2.Transport{
			TLSClientConfig: &tls.Config{
				RootCAs: CA_Pool,
			},
		},
	}
}
