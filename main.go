package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/vaiojarsad/ringy/internal/sms"
)

// Estos valores se reemplazan al compilar con el Makefile.
var version = "dev"
var commit = "unknown"

type sendSMSRequest struct {
	Number  string `json:"numero"`
	Message string `json:"mensaje"`
}

type sendSMSResponse struct {
	Message string `json:"mensaje"`
}

func main() {
	defaultPort := os.Getenv("PORT")
	if defaultPort == "" {
		defaultPort = "8080"
	}
	portFlag := flag.String("port", defaultPort, "puerto HTTP en el que escuchar (por defecto PORT o 8080)")
	versionFlag := flag.Bool("version", false, "mostrar la versión y salir")
	flag.Parse()
	if *versionFlag {
		fmt.Printf("sms %s (commit %s)\n", version, commit)
		return
	}
	port, err := strconv.Atoi(*portFlag)
	if err != nil || port < 1 || port > 65535 {
		log.Fatal("el puerto debe estar entre 1 y 65535")
	}

	mux := http.NewServeMux()
	mux.HandleFunc("POST /sms", sendSMSHandler)

	address := ":" + strconv.Itoa(port)
	log.Printf("Servicio REST escuchando en %s", address)
	log.Fatal(http.ListenAndServe(address, mux))
}

func sendSMSHandler(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()

	var request sendSMSRequest
	if err := decoder.Decode(&request); err != nil {
		writeError(w, http.StatusBadRequest, "JSON inválido: "+err.Error())
		return
	}
	if err := ensureOnlyWhitespace(r.Body); err != nil {
		writeError(w, http.StatusBadRequest, "el cuerpo debe contener un único objeto JSON")
		return
	}
	request.Number = strings.TrimSpace(request.Number)
	if request.Number == "" || strings.TrimSpace(request.Message) == "" {
		writeError(w, http.StatusBadRequest, "numero y mensaje son obligatorios")
		return
	}

	if err := sms.Send(request.Number, request.Message); err != nil {
		log.Printf("No se pudo enviar el SMS: %v", err)
		writeError(w, http.StatusInternalServerError, "no se pudo enviar el SMS")
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(sendSMSResponse{Message: "SMS enviado"})
}

func ensureOnlyWhitespace(body io.Reader) error {
	var extra [1]byte
	for {
		n, err := body.Read(extra[:])
		if n > 0 {
			if !isJSONWhitespace(extra[0]) {
				return errors.New("contenido adicional después del objeto JSON")
			}
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}
	}
}

func isJSONWhitespace(b byte) bool {
	return b == ' ' || b == '\t' || b == '\n' || b == '\r'
}

func writeError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(sendSMSResponse{Message: message})
}
