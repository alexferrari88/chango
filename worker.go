package chango

import (
	"fmt"
	"log"
	"reflect"
	"strconv"
	"strings"
	"sync"
)

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

func Worker(jobs <-chan Job, results chan<- Result, wg *sync.WaitGroup) {
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

func ProcessResults(results <-chan Result, wg *sync.WaitGroup) {
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
			if reachedThreshold, err := CheckThreshold(result.Subscription.Threshold, result.Value); err != nil {
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

func ProcessThresholdString(threshold string) (func(string) (bool, error), error) {
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

func CheckThreshold(threshold string, value string) (bool, error) {
	if threshOperator, err := ProcessThresholdString(threshold); err != nil {
		return false, err
	} else {
		return threshOperator(value)
	}
}
