package caddylog

import (
	"log"
	"net/http"
	"time"

	"bitbucket.org/lightcodelabs/caddy2"
	"bitbucket.org/lightcodelabs/caddy2/modules/caddyhttp"
)

func init() {
	caddy2.RegisterModule(caddy2.Module{
		Name: "http.middleware.log",
		New:  func() (interface{}, error) { return new(Log), nil },
		OnLoad: func(instances []interface{}, priorState interface{}) (interface{}, error) {
			var counter int
			if priorState != nil {
				counter = priorState.(int)
			}
			counter++
			for _, inst := range instances {
				logInst := inst.(*Log)
				logInst.counter = counter
			}
			log.Println("State is now:", counter)
			return counter, nil
		},
		OnUnload: func(state interface{}) error {
			log.Println("Closing log files, state:", state)
			return nil
		},
	})
}

// Log implements a simple logging middleware.
type Log struct {
	Filename string
	counter  int
}

func (l *Log) ServeHTTP(w http.ResponseWriter, r *http.Request, next caddyhttp.Handler) error {
	start := time.Now()

	if err := next.ServeHTTP(w, r); err != nil {
		return err
	}

	log.Println("latency:", time.Now().Sub(start), l.counter)

	return nil
}

// Interface guard
var _ caddyhttp.MiddlewareHandler = &Log{}