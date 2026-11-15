package sms

import (
	"fmt"

	"github.com/godbus/dbus/v5"
)

const (
	mmService           = "org.freedesktop.ModemManager1"
	mmRoot              = "/org/freedesktop/ModemManager1"
	mmGetManagedObjects = "org.freedesktop.DBus.ObjectManager.GetManagedObjects"
	mmModemInterface    = "org.freedesktop.ModemManager1.Modem"
	mmMessagingCreate   = "org.freedesktop.ModemManager1.Modem.Messaging.Create"
	mmSmsSend           = "org.freedesktop.ModemManager1.Sms.Send"
	mmModemEnable       = "org.freedesktop.ModemManager1.Modem.Enable"
)

// Send creates and sends an SMS through the first modem registered with ModemManager.
func Send(number, message string) error {
	if number == "" {
		return fmt.Errorf("el número es obligatorio")
	}
	if message == "" {
		return fmt.Errorf("el mensaje es obligatorio")
	}

	conn, err := dbus.SystemBus()
	if err != nil {
		return fmt.Errorf("conectar con el bus del sistema: %w", err)
	}
	defer conn.Close()

	var objects map[dbus.ObjectPath]map[string]map[string]dbus.Variant
	call := conn.Object(mmService, mmRoot).Call(mmGetManagedObjects, 0)
	if call.Err != nil {
		return fmt.Errorf("consultar dispositivos ModemManager: %w", call.Err)
	}
	if err := call.Store(&objects); err != nil {
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

	modem := conn.Object(mmService, modemPath)
	if err := modem.Call(mmModemEnable, 0, true).Err; err != nil {
		return fmt.Errorf("habilitar módem: %w", err)
	}

	var smsPath dbus.ObjectPath
	call = modem.Call(mmMessagingCreate, 0, map[string]dbus.Variant{
		"number": dbus.MakeVariant(number),
		"text":   dbus.MakeVariant(message),
	})
	if call.Err != nil {
		return fmt.Errorf("crear SMS: %w", call.Err)
	}
	if err := call.Store(&smsPath); err != nil {
		return fmt.Errorf("leer SMS creado: %w", err)
	}

	if err := conn.Object(mmService, smsPath).Call(mmSmsSend, 0).Err; err != nil {
		return fmt.Errorf("enviar SMS: %w", err)
	}
	return nil
}
