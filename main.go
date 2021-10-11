package main

import (
	"bufio"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/signal"
	"regexp"
	"strconv"
	"strings"
	"sync"
)

// Pairs of ports for denoting where to fetch data from, and where to listen
var portPair []int

// Regex patterns for mathcing different kinds of line protocol data
var linePattern, typePattern, helpPattern *regexp.Regexp

const basePath = `/metrics`
const staleThreshold = 240 // This decides how many times a value can be unchanged before it is blocked from sending
const startStale = true

type MetricType int32

const (
	histogram MetricType = iota // Not supported
	summary                     // Not supported
	untyped
	counter
	gauge
)

var typeText = [...]string{
	`histogram`, // Not supported
	`summary`,   // Not supported
	`untyped`,
	`counter`,
	`gauge`,
}

type ScrapeTarget struct {
	queryPort int
	data      map[string]MetricData
}

type MetricData struct {
	commentType      MetricType
	commentHelp      string
	label            map[string]float64
	unchangedCounter int64
}

func (scrapeTarget *ScrapeTarget) handler(w http.ResponseWriter, r *http.Request) {
	resp, err := http.Get(`http://localhost:` + strconv.Itoa(scrapeTarget.queryPort) + basePath)
	if err != nil {
		log.Fatalln(err)
	}
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Fatalln(err)
	}

	stringBody := string(body)

	data := make(map[string]MetricData)

	scanner := bufio.NewScanner(strings.NewReader(stringBody))

	// The below loop is unoptimized. Optimization is a "to-do".
	for scanner.Scan() {

		lineResult := linePattern.FindStringSubmatch(scanner.Text())

		// Metric value?
		if len(lineResult) > 0 {
			if value, err := strconv.ParseFloat(lineResult[3], 64); err == nil {
				if len(data[lineResult[1]].label) == 0 {
					var x = data[lineResult[1]]
					x.label = make(map[string]float64)
					data[lineResult[1]] = x
				}
				data[lineResult[1]].label[lineResult[2]] = value
			}
		}

		// Type declaration?
		typeResult := typePattern.FindStringSubmatch(scanner.Text())
		if len(typeResult) > 0 {
			var metricType MetricType
			switch typeResult[2] {
			case "counter":
				metricType = counter
			case "gauge":
				metricType = gauge
			case "histogram":
				metricType = histogram
			case "summary":
				metricType = summary
			case "untyped":
				metricType = untyped
			}

			var x = data[typeResult[1]]
			x.commentType = metricType
			data[typeResult[1]] = x
		}

		// Help declaration?
		helpResult := helpPattern.FindStringSubmatch(scanner.Text())
		if len(helpResult) > 0 {
			var x = data[helpResult[1]]
			x.commentHelp = helpResult[2]
			data[helpResult[1]] = x
		}
	}

	for name, content := range data {

		// Metric name doesn't exist yet? Create it!
		if _, ok := scrapeTarget.data[name]; !ok {
			// Unchanged counter value should be initialized differently if we want
			// to start with assuming that all value are stale, or if we want to
			// start by assuming that all values are "live" and then gradually
			// put them in "stale" status.
			// * -1, assume all values are live
			// * threshold value, assume all values are stale to begin with

			if startStale {
				content.unchangedCounter = staleThreshold
			} else {
				content.unchangedCounter = -1
			}

			scrapeTarget.data[name] = content
		}

		// Check if value is unchanged compared to previous value
		unchanged := true
		for label, value := range content.label {
			if scrapeTarget.data[name].label[label] != value {
				unchanged = false
			}
		}

		if unchanged {
			// increment unchangedness counter in historical data.
			var x = scrapeTarget.data[name]
			x.unchangedCounter++
			scrapeTarget.data[name] = x
		} else {
			// copy current data to historical data and
			// reset unchangedness counter in historical data.
			scrapeTarget.data[name] = data[name]
			var x = scrapeTarget.data[name]
			x.unchangedCounter = 0
			scrapeTarget.data[name] = x
		}
	}

	var metricOutput string
	for name, content := range data {
		if content.commentType == histogram { // not supported, because complicated
			continue
		}
		if content.commentType == summary { // not supported, because complicated
			continue
		}
		if scrapeTarget.data[name].unchangedCounter > staleThreshold {
			continue
		}

		metricOutput += fmt.Sprintln(`# HELP ` + name + ` ` + content.commentHelp)
		metricOutput += fmt.Sprintln(`# TYPE ` + name + ` ` + typeText[content.commentType])
		for label, value := range content.label {
			if label != `` {
				metricOutput += fmt.Sprintln(name+`{`+label+`}`, value)
			} else {
				metricOutput += fmt.Sprintln(name, value)
			}
		}
	}
	fmt.Fprintf(w, metricOutput)
}

func main() {
	linePattern = regexp.MustCompile(`^([a-zA-Z_:][a-zA-Z0-9_:]*)(?:\{([^\}]*)\})? ([+-]Inf|NaN|-?[0-9]+(?:\.\d+)?(?:e[+-]\d+)?)(?: (-?\d+))?$`)
	typePattern = regexp.MustCompile(`^# TYPE ([a-zA-Z_:][a-zA-Z0-9_:]*(?:\{[^\}]+\})?) (counter|gauge|histogram|summary|untyped)$`)
	helpPattern = regexp.MustCompile(`^# HELP ([a-zA-Z_:][a-zA-Z0-9_:]*(?:\{[^\}]+\})?) (.*)$`)

	commandlineArguments := os.Args[1:]
	for _, element := range commandlineArguments {
		i, err := strconv.Atoi(element)
		if err != nil {
			// handle error
			fmt.Println(err)
			os.Exit(2)
		}

		portPair = append(portPair, i)
	}

	for len(portPair) >= 2 {
		fmt.Println(portPair)

		var remotePort, localPort int
		remotePort, portPair = portPair[0], portPair[1:]
		localPort, portPair = portPair[0], portPair[1:]

		go listener(remotePort, localPort)
	}

	fmt.Printf("Press Ctrl+C to end\n")
	WaitForCtrlC()
	fmt.Printf("\n")
}

func listener(queryPort, listenport int) {
	scrapeTarget := &ScrapeTarget{queryPort: queryPort}
	scrapeTarget.data = make(map[string]MetricData)

	http.HandleFunc(basePath, scrapeTarget.handler)
	log.Fatal(http.ListenAndServe(`:`+strconv.Itoa(listenport), nil))
}

func WaitForCtrlC() {
	var end_waiter sync.WaitGroup
	end_waiter.Add(1)
	var signal_channel chan os.Signal
	signal_channel = make(chan os.Signal, 1)
	signal.Notify(signal_channel, os.Interrupt)
	go func() {
		<-signal_channel
		end_waiter.Done()
	}()
	end_waiter.Wait()
}
