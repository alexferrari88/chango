package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/PuerkitoBio/goquery"
	"github.com/tidwall/gjson"
)

type Selector struct {
	Value     string
	Type      string
	Threshold string
	Frequency string
}

type Subscription struct {
	Notification NotificationSettings
	Id           string
	WebsiteId    string
	Threshold    string
	Frequency    string
}

type NotificationSettings struct {
	Type     string
	Address  string
	Notifier Notifier
}

type Website struct {
	Id           string
	Selector     Selector
	Url          string
	Name         string
	ScrapingType string
	JsonKey      string
	RealBrowser  bool
}

type Websites []Website

func (ws *Websites) GetById(id string) *Website {
	for _, w := range *ws {
		if w.Id == id {
			return &w
		}
	}
	return &Website{}
}

type Job struct {
	Website      *Website
	Subscription *Subscription
	Scraper      Scraper
}

type Result struct {
	Website      *Website
	Subscription *Subscription
	Error        error
	Value        string
}

type Scraper interface {
	Scrape(w *Website) (Result, error)
}

type JSONScraper struct{}

func (j JSONScraper) Scrape(w *Website) (Result, error) {
	u := w.Url
	selector := w.Selector
	req, err := http.NewRequest("GET", u, nil)
	if err != nil {
		panic(err)
	}
	client := &http.Client{
		Timeout: 3 * time.Second,
	}
	res, err := client.Do(req)
	if err != nil {
		return Result{}, err
	}
	if res.StatusCode != 200 {
		return Result{}, fmt.Errorf("status code error: %d %s", res.StatusCode, res.Status)
	}
	defer res.Body.Close()
	body, err := io.ReadAll(res.Body)
	if err != nil {
		return Result{}, err
	}
	json := string(body)
	if w.JsonKey != "" {
		json = gjson.Get(json, w.JsonKey).String()
	}
	value := gjson.Get(json, selector.Value)
	return Result{Value: value.String(), Website: w}, nil
}

type HTMLScraper struct{}

func (h HTMLScraper) Scrape(w *Website) (Result, error) {
	u := w.Url
	selector := w.Selector
	req, err := http.NewRequest("GET", u, nil)
	if err != nil {
		panic(err)
	}
	client := &http.Client{
		Timeout: time.Second * 5,
	}
	res, err := client.Do(req)
	if err != nil {
		return Result{}, err
	}
	if res.StatusCode != 200 {
		return Result{}, fmt.Errorf("status code error: %d %s", res.StatusCode, res.Status)
	}
	defer res.Body.Close()
	doc, err := goquery.NewDocumentFromReader(res.Body)
	if err != nil {
		return Result{}, err
	}
	value := doc.Find(selector.Value).Text()
	return Result{Value: value, Website: w}, nil
}

type Notifier interface {
	Write(p []byte) (n int, err error)
}

type ConsoleNotifier struct {
	Address string
}
type EmailNotifier struct {
	Address string
}

func (c ConsoleNotifier) Write(p []byte) (n int, err error) {
	fmt.Println(string(p))
	return len(p), nil
}

func (e EmailNotifier) Write(p []byte) (n int, err error) {
	fmt.Println("Sending email to", e.Address)
	fmt.Println(string(p))
	return len(p), nil
}

func worker(jobs <-chan Job, results chan<- Result, wg *sync.WaitGroup) {
	for j := range jobs {
		result, err := j.Scraper.Scrape(j.Website)
		if err != nil {
			results <- Result{Subscription: j.Subscription, Error: err}
			continue
		}
		results <- Result{Value: result.Value, Subscription: j.Subscription, Website: result.Website}
	}
	close(results)
}

func main() {
	var websitesParsed map[string]Websites
	var wg sync.WaitGroup
	websitesData, err := os.ReadFile("websites.toml")
	if err != nil {
		log.Fatal(err)
	}

	_, err = toml.Decode(string(websitesData), &websitesParsed)
	if err != nil {
		log.Fatal(err)
	}
	websites := websitesParsed["website"]

	var subsParsed map[string][]Subscription
	subscriptionsData, err := os.ReadFile("subscriptions.toml")
	if err != nil {
		log.Fatal(err)
	}

	_, err = toml.Decode(string(subscriptionsData), &subsParsed)
	if err != nil {
		log.Fatal(err)
	}

	jobs := make(chan Job, 100)
	results := make(chan Result, 100)

	go worker(jobs, results, &wg)
	go processResults(results, &wg)

	scrapersFactory := map[string]Scraper{"json": JSONScraper{}, "html": HTMLScraper{}}

	subscriptions := subsParsed["subscription"]
	for _, s := range subscriptions {
		wg.Add(1)
		sub := s
		w := websites.GetById(s.WebsiteId)
		if sub.Notification.Type != "" {
			switch sub.Notification.Type {
			case "console":
				sub.Notification.Notifier = ConsoleNotifier{}
			case "email":
				sub.Notification.Notifier = EmailNotifier{Address: sub.Notification.Address}
			}
		}
		jobs <- Job{Website: w, Subscription: &sub, Scraper: scrapersFactory[w.ScrapingType]}
	}
	close(jobs)
	wg.Wait()
}

func processResults(results <-chan Result, wg *sync.WaitGroup) {
	for result := range results {
		if result.Error != nil {
			fmt.Println(result.Error)
			wg.Done()
			continue
		} else if result.Website == nil {
			wg.Done()
			continue
		}
		if result.Subscription.Threshold != "" {
			if reachedThreshold, err := checkThreshold(result.Subscription.Threshold, result.Value); err != nil {
				log.Fatal(err)
			} else if reachedThreshold {
				notifier := result.Subscription.Notification.Notifier
				if notifier != nil {
					fmt.Fprint(notifier, result.Website.Name, "reached the threshold. The new value is: ", result.Value, ".")
				} else {
					fmt.Println("Threshold reached for", result.Website.Name)
				}
			}
		}
		wg.Done()
	}
}

func processThresholdString(threshold string) (func(string) (bool, error), error) {
	s := strings.Split(threshold, " ")
	if len(s) != 2 {
		return nil, fmt.Errorf("invalid threshold string")
	}
	cmp := s[0]
	threshValRaw := s[1]

	var threshVal interface{}
	if threshValRaw == "true" {
		threshVal = true
	} else if threshValRaw == "false" {
		threshVal = false
	} else if f, err := strconv.ParseFloat(threshValRaw, 64); err == nil {
		threshVal = f
	} else {
		threshVal = threshValRaw
	}
	return func(v string) (bool, error) {
		var vVal interface{}
		if v == "true" {
			vVal = true
		} else if v == "false" {
			vVal = false
		} else if f, err := strconv.ParseFloat(v, 64); err == nil {
			vVal = f
		} else {
			vVal = v
		}

		switch cmp {
		case "==":
			return reflect.DeepEqual(vVal, threshVal), nil
		case "!=":
			return !reflect.DeepEqual(vVal, threshVal), nil
		case ">":
			if reflect.TypeOf(vVal) == reflect.TypeOf(threshVal) {
				return vVal.(float64) > threshVal.(float64), nil
			}
			return false, nil
		case "<":
			if reflect.TypeOf(vVal) == reflect.TypeOf(threshVal) {
				return vVal.(float64) < threshVal.(float64), nil
			}
			return false, nil
		case ">=":
			if reflect.TypeOf(vVal) == reflect.TypeOf(threshVal) {
				return vVal.(float64) >= threshVal.(float64), nil
			}
			return false, nil
		case "<=":
			if reflect.TypeOf(vVal) == reflect.TypeOf(threshVal) {
				return vVal.(float64) <= threshVal.(float64), nil
			}
			return false, nil
		default:
			return false, fmt.Errorf("invalid comparison operator")
		}
	}, nil
}

func checkThreshold(threshold string, value string) (bool, error) {
	if threshOperator, err := processThresholdString(threshold); err != nil {
		return false, err
	} else {
		return threshOperator(value)
	}
}
