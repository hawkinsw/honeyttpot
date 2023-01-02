package main

import (
	"context"
	"crypto/tls"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"reflect"
	"sync"

	"github.com/hawkinsw/honeyppot/v2/honeyttpot"
)

func generate_honeyttpot_handler(web_server honeyttpot.Server, log_get io.Writer, log_post io.Writer) func(resw http.ResponseWriter, req *http.Request) {
	return func(resw http.ResponseWriter, req *http.Request) {
		if req.Method == "POST" {
			/* Suck up (up to) 1k of data just to see what they are up to! */
			bad_buffer := make([]byte, 1024)
			if actual_length, err := io.ReadFull(req.Body, bad_buffer); err != nil && err != io.ErrUnexpectedEOF {
				bad_buffer = []byte(fmt.Sprintf("Error reading post body: %v", err))
			} else {
				bad_buffer = bad_buffer[:actual_length]
			}
			log_post.Write([]byte(fmt.Sprintf("%v:", req)))
			log_post.Write(bad_buffer)
			log_post.Write([]byte("\n"))
		} else {
			log_get.Write([]byte(fmt.Sprintf("Request: %v\n", req)))
		}
		resw.Header().Add("Server", web_server.Name())
		web_server.HandleSuccess(req, resw)
	}
}

type optionalFlag struct {
	Value string
	IsSet bool
}

func (of *optionalFlag) Set(value string) error {
	of.IsSet = true
	of.Value = value
	return nil
}
func (of *optionalFlag) String() string {
	return of.Value
}

func declareCustomFlag[FlagType flag.Value](f FlagType, v ...any) FlagType {
	paramValues := make([]reflect.Value, 1)
	paramValues[0] = reflect.ValueOf(f)
	for _, v := range v {
		paramValues = append(paramValues, reflect.ValueOf(v))
	}
	reflect.ValueOf(flag.Var).Call(paramValues)
	return f
}

var (
	cert_filename_flag = flag.String("cert", "cert.pem", "Filename of the HTTPS public certificate.")
	key_filename_flag  = flag.String("key", "key.pem", "Filename of the key for the HTTPS certificate.")
	no_ssl_flag        = flag.Bool("no-ssl", false, "Disable ssl (use HTTP 1.1)")
	listen_addr_flag   = flag.String("listen-addr", "0.0.0.0", "Address on which to listen for connections.")
	listen_port_flag   = declareCustomFlag(&optionalFlag{Value: "443"}, "listen-port", "Port on which to listen for connections (default to 80 for http and 443 for https).")
	post_log_file      = flag.String("post-log", "post.log", "Path of the file to log POST requests.")
	get_log_file       = flag.String("get-log", "get.log", "Path of the file to log GET requests.")
	data_file          = "data/tale.txt"
)

func main() {
	flag.Parse()

	server_context, server_context_cancel := context.WithCancel(context.Background())

	contents_file, contents_error := os.Open(data_file)
	if contents_error != nil {
		fmt.Fprintf(os.Stderr, "Could not open the contents file (%s): %v\n", data_file, contents_error)
		os.Exit(-1)
	}
	post_file, post_error := os.OpenFile(*post_log_file, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0755)
	if post_error != nil {
		fmt.Fprintf(os.Stderr, "Could not open the post file (%s): %v\n", *post_log_file, post_error)
		os.Exit(-1)
	}
	get_file, get_error := os.OpenFile(*get_log_file, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0755)
	if get_error != nil {
		fmt.Fprintf(os.Stderr, "Could not open the get file (%s): %v\n", *get_log_file, get_error)
		os.Exit(-1)
	}

	nginx, nginx_error := honeyttpot.NewNginx("1.23.1", contents_file)
	if nginx_error != nil {
		fmt.Fprintf(os.Stderr, "Could not create the nginx emulator: %v\n", nginx_error)
		os.Exit(-1)
	}

	wildcard_muxer := http.NewServeMux()
	wildcard_muxer.HandleFunc("/", generate_honeyttpot_handler(nginx, get_file, post_file))
	http_server := http.Server{Handler: wildcard_muxer}

	signal_channel := make(chan os.Signal, 1)

	go func() {
		for {
			select {
			case <-signal_channel:
				fmt.Fprintf(os.Stderr, "\nUser-requested server shutdown beginning ...\n")
				if err := http_server.Shutdown(server_context); err != nil {
					fmt.Fprintf(os.Stderr, "Warning: Could not Shutdown() the server: %v\n", err)
				}
				server_context_cancel()
			}
		}
	}()
	signal.Notify(signal_channel, os.Interrupt)

	if *no_ssl_flag && !listen_port_flag.IsSet {
		listen_port_flag.Set("80")
	}

	if listen_ipaddr := net.ParseIP(*listen_addr_flag); listen_ipaddr == nil {
		fmt.Fprintf(os.Stderr, "Could not parse the IP address supplied (%v)\n", *listen_addr_flag)
		server_context_cancel()
		os.Exit(-1)
	}

	http_server.Addr = fmt.Sprintf("%s:%s", *listen_addr_flag, listen_port_flag.Value)

	wg := sync.WaitGroup{}
	wg.Add(1)
	go func() {
		// In a very degenerate case, the user might have already requested that
		// the server be stopped.
		if server_context.Err() == nil {
			var err error
			if *no_ssl_flag {
				http_server.TLSNextProto = make(map[string]func(*http.Server, *tls.Conn, http.Handler), 0)
				err = http_server.ListenAndServe()
			} else {
				err = http_server.ListenAndServeTLS(*cert_filename_flag, *key_filename_flag)
			}
			if err != nil {
				if err != http.ErrServerClosed {
					fmt.Fprintf(os.Stderr, "Could not start the HTTPS webserver: %v\n", err)
				} else {
					fmt.Fprintf(os.Stderr, "Nicely shutting down the webserver.\n")
				}
			}
		}
		wg.Done()
	}()

	wg.Wait()

	get_file.Close()
	post_file.Close()

	fmt.Fprintf(os.Stderr, "Server done!\n")

	os.Stderr.Sync()
	os.Stdout.Sync()
}
