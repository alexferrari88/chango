package main

import (
	"log"
	"os"
	"sync"

	"github.com/BurntSushi/toml"
	"github.com/alexferrari88/chango"
)

func main() {
	var websitesParsed map[string]chango.Websites
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

	var subsParsed map[string][]chango.Subscription
	subscriptionsData, err := os.ReadFile("subscriptions.toml")
	if err != nil {
		log.Fatal(err)
	}

	_, err = toml.Decode(string(subscriptionsData), &subsParsed)
	if err != nil {
		log.Fatal(err)
	}

	jobs := make(chan chango.Job, 100)
	results := make(chan chango.Result, 100)

	go chango.Worker(jobs, results, &wg)
	go chango.ProcessResults(results, &wg)

	scrapersFactory := map[string]chango.Scraper{"json": chango.JSONScraper{}, "html": chango.HTMLScraper{}}

	subscriptions := subsParsed["subscription"]
	for _, s := range subscriptions {
		wg.Add(1)
		sub := s
		w := websites.GetById(s.WebsiteId)
		if sub.Notification.Type != "" {
			switch sub.Notification.Type {
			case "console":
				sub.Notification.Notifier = chango.ConsoleNotifier{}
			case "email":
				sub.Notification.Notifier = chango.EmailNotifier{Address: sub.Notification.Address}
			}
		}
		jobs <- chango.Job{Website: w, Subscription: &sub, Scraper: scrapersFactory[w.ScrapingType]}
	}
	close(jobs)
	wg.Wait()
}
