package main

import (
	"fmt"
	"log"
	"sync"
)

type job struct {
	website      *Website
	subscription *Subscription
	scraper      Scraper
}

type Result struct {
	Website      *Website
	Subscription *Subscription
	Error        error
	Value        string
}

func worker(jobs <-chan job, results chan<- Result, wg *sync.WaitGroup) {
	for j := range jobs {
		result, err := j.scraper.Scrape(j.website)
		if err != nil {
			results <- Result{Subscription: j.subscription, Error: err}
			continue
		}
		results <- Result{Value: result.Value, Subscription: j.subscription, Website: result.Website}
	}
	close(results)
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
