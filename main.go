package main

import (
	"fmt"
	"log"
	"os"
	"reflect"
	"strconv"
	"strings"
	"sync"

	"github.com/BurntSushi/toml"
)

type Subscription struct {
	Notification NotificationSettings
	Id           string
	WebsiteId    string
	Threshold    string
	Frequency    string
}

type Selector struct {
	Value     string
	Type      string
	Threshold string
	Frequency string
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

	jobs := make(chan job, 100)
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
		jobs <- job{website: w, subscription: &sub, scraper: scrapersFactory[w.ScrapingType]}
	}
	close(jobs)
	wg.Wait()
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
