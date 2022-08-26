package main

import (
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/tidwall/gjson"
)

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
