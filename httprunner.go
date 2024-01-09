package httprunner

import (
	"context"
	"crypto/tls"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

type RunnerOptions interface {
	GetAddr() (string, error)
	GetTlsConfig() *tls.Config
	GetTlsEnabled() bool
	Reload() error
}

// This function is just some jank to prevent deadlocking if the user doesn't read from warn and it fills up.
func warning(warn chan<- error, err error) {
	select {
	case warn <- err:
	default:
	}
}

type Runner struct {
	wg       *sync.WaitGroup
	warn     chan error
	err      chan error
	done     chan struct{}
	stop     chan os.Signal
	reload   chan os.Signal
	exitCode int
}

func NewRunner() *Runner {
	// First set up signal handling so that we can reload and stop.
	hups := make(chan os.Signal, 1)
	stop := make(chan os.Signal, 1)

	signal.Notify(hups, syscall.SIGHUP)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)

	var waitgroup sync.WaitGroup

	done := make(chan struct{}, 1)
	err := make(chan error, 1)
	warn := make(chan error, 10)

	// If exitCode is -1 then execution has not completed yet.
	server := &Runner{&waitgroup, warn, err, done, stop, hups, -1}
	return server
}

func (r *Runner) Warnings() <-chan error {
	return r.warn
}

func (r *Runner) serveWithReload(router http.Handler, opts RunnerOptions, logger *log.Logger) {
	for {
		err := opts.Reload()
		if err != nil {
			warning(r.warn, fmt.Errorf("Error reloading server options. Old config may be used: %s", err))
		}

		addr, err := opts.GetAddr()
		if err != nil {
			r.err <- err
		}
		tlsConf := opts.GetTlsConfig()
		tlsEnabled := opts.GetTlsEnabled()

		httpServ := &http.Server{
			Addr:         addr,
			Handler:      router,
			ErrorLog:     logger,
			TLSConfig:    tlsConf,
			ReadTimeout:  5 * time.Second,
			WriteTimeout: 10 * time.Second,
			IdleTimeout:  120 * time.Second,
		}
		go func(err chan<- error) {
			if tlsEnabled {
				// Note that the certificate is already embedded in the tlsConf and that will override
				// any cert/key filenames we pass anyways.
				httpServ.ErrorLog.Printf("https server listening on %s", addr)
				if e := httpServ.ListenAndServeTLS("", ""); e != nil && e != http.ErrServerClosed {
					err <- e
				}
			} else {
				httpServ.ErrorLog.Printf("http server listening on %s", addr)
				if e := httpServ.ListenAndServe(); e != nil && e != http.ErrServerClosed {
					err <- e
				}
			}
		}(r.err)

		select {
		case <-r.reload:
			httpServ.ErrorLog.Print("SIGHUP received. Reloading...")
			begin := time.Now()
			httpServ.ErrorLog.Print("Halting HTTP Server...")
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			if err := httpServ.Shutdown(ctx); err != nil {
				warning(r.warn, fmt.Errorf("Failed to gracefully shutdown server: %v", err))
			} else {
				httpServ.ErrorLog.Printf("HTTP server halted in %v", time.Since(begin))
			}
			cancel()
		case <-r.done:
			httpServ.ErrorLog.Print("Server shutting down...")
			begin := time.Now()
			httpServ.ErrorLog.Print("Halting HTTP Server...")
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			if err := httpServ.Shutdown(ctx); err != nil {
				warning(r.warn, fmt.Errorf("Failed to gracefully shutdown server: %v", err))
			} else {
				httpServ.ErrorLog.Printf("HTTP server halted in %v", time.Since(begin))
			}

			r.wg.Done()
			cancel()
			return
		}
	}
}

func (r *Runner) Run(router http.Handler, opts RunnerOptions, logger *log.Logger) {
	r.wg.Add(1)
	go r.waitOnInterrupt()
	go r.serveWithReload(router, opts, logger)
	return
}

func (r *Runner) BlockingRun(router http.Handler, opts RunnerOptions, logger *log.Logger) (int, error) {
	r.wg.Add(1)
	go r.serveWithReload(router, opts, logger)
	err := r.waitOnInterrupt()
	return r.exitCode, err
}

func (r *Runner) waitOnInterrupt() error {
	select {
	case <-r.stop:
		warning(r.warn, fmt.Errorf("Interrupt/kill received. Exiting..."))
		r.exitCode = 0
		r.Shutdown()
	case e := <-r.err:
		warning(r.warn, fmt.Errorf("Encountered error. Exiting..."))
		r.Shutdown()
		r.exitCode = 1
		return e
	}
	return nil
}

func (r *Runner) Shutdown() {
	signal.Stop(r.reload)
	signal.Stop(r.stop)
	close(r.done)

	r.wg.Wait()

	close(r.stop)
	close(r.reload)
	close(r.warn)
	close(r.err)
	return
}
