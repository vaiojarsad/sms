package main

import (
	"fmt"
	"log"

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

func main() {
	conn, err := dbus.SystemBus()
	if err != nil {
		log.Fatal(err)
	}
	defer func(conn *dbus.Conn) {
		err2 := conn.Close()
		if err2 != nil {
			log.Printf("failed to close DBus connection: %v", err2)
		}
	}(conn)

	var objects map[dbus.ObjectPath]map[string]map[string]dbus.Variant

	call := conn.Object(mmService, mmRoot).Call(mmGetManagedObjects, 0)
	if call.Err != nil {
		log.Fatal(call.Err)
	}

	if err = call.Store(&objects); err != nil {
		log.Fatal(err)
	}

	var modemPath dbus.ObjectPath

	for path, interfaces := range objects {
		if _, ok := interfaces[mmModemInterface]; ok {
			modemPath = path
			fmt.Println("Modem encontrado:", path)
			break
		}
	}

	if modemPath == "" {
		log.Fatal("no se encontró ningún modem")
	}

	modem := conn.Object(mmService, modemPath)

	call = modem.Call(mmModemEnable, 0, true)
	if call.Err != nil {
		log.Fatal(call.Err)
	}

	var smsPath dbus.ObjectPath
	call = modem.Call(
		mmMessagingCreate,
		0,
		map[string]dbus.Variant{
			"number": dbus.MakeVariant("+5491131539070"),
			"text":   dbus.MakeVariant("Hola desde Go!"),
		},
	)

	if call.Err != nil {
		log.Fatal(call.Err)
	}
	if err = call.Store(&smsPath); err != nil {
		log.Fatal(err)
	}

	fmt.Println("SMS creado:", smsPath)

	sms := conn.Object(mmService, smsPath)
	call = sms.Call(mmSmsSend, 0)
	if call.Err != nil {
		log.Fatal(call.Err)
	}

	fmt.Println("SMS enviado")
}
