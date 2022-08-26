package main

import "fmt"

type NotificationSettings struct {
	Type     string
	Address  string
	Notifier Notifier
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
