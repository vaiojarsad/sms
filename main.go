package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/vaiojarsad/ringy/internal/sms"
)

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

	// Configuramos el servidor con timeouts de seguridad
	srv := &http.Server{
		Addr:         ":" + strconv.Itoa(port),
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 40 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Contexto raíz que se cancela cuando Docker o el SO envían SIGTERM / SIGINT
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Levantamos el servidor en una goroutine para no bloquear el apagado ordenado
	go func() {
		log.Printf("Servicio REST escuchando en %s", srv.Addr)
		if errSrv := srv.ListenAndServe(); errSrv != nil && !errors.Is(errSrv, http.ErrServerClosed) {
			log.Fatalf("Error fatal en el servidor HTTP: %v", errSrv)
		}
	}()

	// Esperamos la señal de apagado (Ctrl+C o docker stop)
	<-ctx.Done()
	log.Println("Apagando servidor, esperando que terminen los envíos en curso...")

	// Le damos hasta tiempo para que termine cualquier SMS que esté enviándose
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	if err = srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("Error durante el apagado del servidor: %v", err)
	}
	_ = sms.Close()
	log.Println("Servidor detenido correctamente")
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

	ctx, cancel := context.WithTimeout(r.Context(), 35*time.Second)
	defer cancel()

	// Pasamos r.Context() hacia la capa de SMS
	if err := sms.Send(ctx, request.Number, request.Message); err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			log.Printf("Timeout enviando SMS: %v", err)
			writeError(w, http.StatusGatewayTimeout, "tiempo de espera agotado al enviar el SMS")
			return
		}
		if errors.Is(err, context.Canceled) {
			log.Printf("Cancelación del lado cliente enviando SMS: %v", err)
			return
		}
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
