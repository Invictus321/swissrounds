// Command swissrounds runs Magic: The Gathering Swiss tournaments with a top
// cut, served as a web UI for the organiser and a big-screen display.
package main

import (
	"embed"
	"flag"
	"io/fs"
	"log"
	"net"
	"net/http"
	"strings"
)

//go:embed static
var staticFiles embed.FS

func main() {
	addr := flag.String("addr", "localhost:8080", "address to listen on (use :8080 to allow other devices on your network)")
	data := flag.String("data", "tournament.json", "file the tournament is saved to")
	flag.Parse()

	srv, err := NewServer(*data)
	if err != nil {
		log.Fatal(err)
	}
	static, err := fs.Sub(staticFiles, "static")
	if err != nil {
		log.Fatal(err)
	}

	ln, err := net.Listen("tcp", *addr)
	if err != nil {
		log.Fatal(err)
	}
	base := "http://" + ln.Addr().String()
	if strings.HasPrefix(*addr, ":") || strings.HasPrefix(*addr, "0.0.0.0:") {
		_, port, _ := net.SplitHostPort(ln.Addr().String())
		base = "http://localhost:" + port
	}
	log.Printf("Saving tournament to %s", *data)
	log.Printf("Organiser view:  %s/", base)
	log.Printf("Big screen view: %s/display", base)
	log.Fatal(http.Serve(ln, srv.Routes(static)))
}
