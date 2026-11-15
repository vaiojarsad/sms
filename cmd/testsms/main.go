// testsms is a small command-line program for manually testing SMS delivery.
package main

import (
	"flag"
	"log"

	"github.com/vaiojarsad/ringy/internal/sms"
)

func main() {
	number := flag.String("numero", "", "número de destino")
	message := flag.String("mensaje", "", "texto del SMS")
	flag.Parse()

	if err := sms.Send(*number, *message); err != nil {
		log.Fatal(err)
	}
	log.Println("SMS enviado")
}
