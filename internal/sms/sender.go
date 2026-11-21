package sms

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/godbus/dbus/v5"
)

const (
	mmService         = "org.freedesktop.ModemManager1"
	mmRoot            = "/org/freedesktop/ModemManager1"
	mmGetManagedObjs  = "org.freedesktop.DBus.ObjectManager.GetManagedObjects"
	mmModemInterface  = "org.freedesktop.ModemManager1.Modem"
	mmModemEnable     = "org.freedesktop.ModemManager1.Modem.Enable"
	mmMessagingCreate = "org.freedesktop.ModemManager1.Modem.Messaging.Create"
	mmMessagingDelete = "org.freedesktop.ModemManager1.Modem.Messaging.Delete"
	mmSmsSend         = "org.freedesktop.ModemManager1.Sms.Send"

	defaultTimeout = 30 * time.Second
)

var (
	modemSem = make(chan struct{}, 1)
	conn     *dbus.Conn
)

// getConn mantiene una conexión D-Bus activa y se reconecta si se cae.
func getConn() (*dbus.Conn, error) {
	if conn != nil && conn.Connected() {
		return conn, nil
	}
	if conn != nil {
		_ = conn.Close() // Limpiamos recursos de la conexión caída
	}

	c, err := dbus.ConnectSystemBus()
	if err != nil {
		return nil, err
	}
	conn = c
	return conn, nil
}

// Send envía un SMS respetando el contexto (timeouts/cancelaciones) y limpiando el mensaje del módem.
func Send(ctx context.Context, number, message string) error {
	if strings.TrimSpace(number) == "" {
		return fmt.Errorf("el número es obligatorio")
	}
	if strings.TrimSpace(message) == "" {
		return fmt.Errorf("el mensaje es obligatorio")
	}

	// Serializamos el acceso al módem físico para evitar colisiones
	select {
	case modemSem <- struct{}{}:
		// Obtuvimos el turno para usar el módem
		defer func() { <-modemSem }()
	case <-ctx.Done():
		// Venció el tiempo o el cliente cortó MIENTRAS esperábamos en la cola
		return fmt.Errorf("tiempo de espera agotado en la cola del módem: %w", ctx.Err())
	}

	// Si mientras esperábamos el turno el cliente cortó o venció el timeout, abortamos ya mismo
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("envío cancelado antes de acceder al módem: %w", err)
	}

	// Aseguramos un timeout máximo aunque el caller pase context.Background()
	opCtx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	c, err := getConn()
	if err != nil {
		return fmt.Errorf("conectar con el bus del sistema: %w", err)
	}

	// 1. Buscar el módem disponible
	var objects map[dbus.ObjectPath]map[string]map[string]dbus.Variant
	call := c.Object(mmService, mmRoot).CallWithContext(opCtx, mmGetManagedObjs, 0)
	if call.Err != nil {
		return fmt.Errorf("consultar dispositivos ModemManager: %w", call.Err)
	}
	if err = call.Store(&objects); err != nil {
		return fmt.Errorf("leer dispositivos ModemManager: %w", err)
	}

	var modemPath dbus.ObjectPath
	for path, interfaces := range objects {
		if _, ok := interfaces[mmModemInterface]; ok {
			modemPath = path
			break
		}
	}
	if modemPath == "" {
		return fmt.Errorf("no se encontró ningún módem")
	}

	modem := c.Object(mmService, modemPath)

	// 2. Habilitar el módem
	if err = modem.CallWithContext(opCtx, mmModemEnable, 0, true).Err; err != nil {
		return fmt.Errorf("habilitar módem: %w", err)
	}

	// 3. Crear el SMS en la memoria del módem
	var smsPath dbus.ObjectPath
	call = modem.CallWithContext(opCtx, mmMessagingCreate, 0, map[string]dbus.Variant{
		"number": dbus.MakeVariant(number),
		"text":   dbus.MakeVariant(message),
	})
	if call.Err != nil {
		return fmt.Errorf("crear SMS: %w", call.Err)
	}
	if err = call.Store(&smsPath); err != nil {
		return fmt.Errorf("leer SMS creado: %w", err)
	}

	// 4. Borrar el SMS de la memoria del módem al terminar (haya fallado o no el envío)
	defer func() {
		// Usamos un contexto breve independiente por si el contexto principal expiró justo en el envío
		delCtx, delCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer delCancel()

		if delErr := modem.CallWithContext(delCtx, mmMessagingDelete, 0, smsPath).Err; delErr != nil {
			slog.Warn("no se pudo borrar el SMS de la memoria del módem", "sms_path", smsPath, "error", delErr)
		}
	}()

	// 5. Enviar el SMS
	if err = c.Object(mmService, smsPath).CallWithContext(opCtx, mmSmsSend, 0).Err; err != nil {
		return fmt.Errorf("enviar SMS: %w", err)
	}

	return nil
}

func Close() error {
	modemSem <- struct{}{}
	defer func() { <-modemSem }()

	if conn != nil {
		err := conn.Close()
		conn = nil
		return err
	}
	return nil
}
