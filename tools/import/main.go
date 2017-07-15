package main

import (
	"bufio"
	"bytes"
	"crypto/tls"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"golang.org/x/net/http2"
	"io/ioutil"
)

var (
	filePath = flag.String("f", "cities5.csv", "File to import")
)

func main() {
	flag.Parse()

	file, err := os.Open(*filePath)
	if err != nil {
		log.Fatal(err)
	}
	defer file.Close()

	client := http.Client{
		Transport: &http2.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}

	resp, err := client.Get("https://localhost:8080/info")
	if err != nil {
		log.Fatal(err)
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	log.Printf("Config: %s, %v", resp.Proto, string(body))

	var statTotal int64
	statStartTime := time.Now()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		coords := strings.Split(line, ",")
		var jsonStr = []byte(fmt.Sprintf(`{"lat":%s,"lon":%s}`, coords[0], coords[1]))

		_, err := client.Post("https://localhost:8080/geo", "application/json", bytes.NewBuffer(jsonStr))
		if err != nil {
			log.Println(err)
		}

		statTotal++
	}

	statTotalTime := time.Since(statStartTime)
	avgTime := int64(statTotalTime / time.Microsecond) / statTotal
	log.Printf("Done. Total requests: %d, time: %dms, avg: %dµs. Config: %s",
		statTotal, statTotalTime / time.Millisecond, avgTime, string(body))
}
